package croupier

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	functionv1 "github.com/cuihairu/croupier/pkg/pb/croupier/function/v1"
	"github.com/xeipuuv/gojsonschema"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// invoker implements the Invoker interface
type invoker struct {
	config *InvokerConfig
	conn   *grpc.ClientConn
	client functionv1.FunctionServiceClient
	mu     sync.RWMutex

	// Function schemas for validation
	schemas map[string]map[string]interface{}

	// Connection state
	connected bool

	// Reconnection state
	isReconnecting      bool
	reconnectAttempts   int
	reconnectCancelCtx  context.CancelFunc
	reconnectCancelFlag bool // flag to track if cancellation is active
}

// NewInvoker creates a new Croupier invoker
func NewInvoker(config *InvokerConfig) Invoker {
	if config == nil {
		config = &InvokerConfig{
			Address:        "localhost:8080",
			TimeoutSeconds: 30,
			Insecure:       true,
			Reconnect:      DefaultReconnectConfig(),
			Retry:          DefaultRetryConfig(),
		}
	}

	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = time.Duration(config.TimeoutSeconds) * time.Second
	}

	// Set default reconnect config if not provided
	if config.Reconnect == nil {
		config.Reconnect = DefaultReconnectConfig()
	}

	// Set default retry config if not provided
	if config.Retry == nil {
		config.Retry = DefaultRetryConfig()
	}

	return &invoker{
		config:  config,
		schemas: make(map[string]map[string]interface{}),
	}
}

// Connect implements Invoker.Connect
func (i *invoker) Connect(ctx context.Context) error {
	err := i.connect(ctx)
	if err != nil {
		// Trigger reconnection if connection error and reconnection is enabled
		if i.isConnectionError(err) {
			i.scheduleReconnectIfNeeded()
		}
		return err
	}
	return nil
}

// connect performs the actual connection without triggering reconnection
func (i *invoker) connect(ctx context.Context) error {
	// Fast path: read lock to check if already connected
	i.mu.RLock()
	if i.connected {
		i.mu.RUnlock()
		return nil
	}
	i.mu.RUnlock()

	// Slow path: need to connect, acquire write lock
	i.mu.Lock()
	defer i.mu.Unlock()

	// Double-check after acquiring write lock
	if i.connected {
		return nil
	}

	return i.connectLocked(ctx)
}

// connectLocked performs the actual connection. Caller must hold i.mu.
func (i *invoker) connectLocked(ctx context.Context) error {
	logInfof("Connecting to server/agent at: %s", i.config.Address)

	// Setup connection options
	var opts []grpc.DialOption

	if i.config.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		tlsCfg, err := buildTLSConfig(i.config)
		if err != nil {
			return err
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	}

	// Create connection with timeout
	ctx, cancel := context.WithTimeout(ctx, i.config.DefaultTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, i.config.Address, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", i.config.Address, err)
	}

	i.conn = conn
	i.client = functionv1.NewFunctionServiceClient(conn)
	i.connected = true

	// Reset reconnection attempts on successful connection
	i.reconnectAttempts = 0
	i.isReconnecting = false

	logInfof("Connected to: %s\n", i.config.Address)
	return nil
}

// Invoke implements Invoker.Invoke
func (i *invoker) Invoke(ctx context.Context, functionID, payload string, options InvokeOptions) (string, error) {
	// connect() is now thread-safe with double-checked locking
	if err := i.connect(ctx); err != nil {
		if i.isConnectionError(err) {
			i.scheduleReconnectIfNeeded()
		}
		return "", fmt.Errorf("not connected to server: %w", err)
	}

	// Client-side validation
	if schema, exists := i.schemas[functionID]; exists {
		if err := i.validatePayload(payload, schema); err != nil {
			return "", fmt.Errorf("payload validation failed for function %s: %w", functionID, err)
		}
	}

	return i.executeWithRetry(ctx, options, func() (string, error) {
		req := &functionv1.InvokeRequest{
			FunctionId:     functionID,
			IdempotencyKey: options.IdempotencyKey,
			Payload:        []byte(payload),
			Metadata:       map[string]string{},
		}
		for k, v := range options.Headers {
			req.Metadata[k] = v
		}

		callCtx, cancel := i.callContext(ctx, options.Timeout)
		defer cancel()

		resp, err := i.client.Invoke(callCtx, req)
		if err != nil {
			return "", err
		}

		return string(resp.GetPayload()), nil
	})
}

// StartJob implements Invoker.StartJob
func (i *invoker) StartJob(ctx context.Context, functionID, payload string, options InvokeOptions) (string, error) {
	// connect() is now thread-safe with double-checked locking
	if err := i.connect(ctx); err != nil {
		if i.isConnectionError(err) {
			i.scheduleReconnectIfNeeded()
		}
		return "", fmt.Errorf("not connected to server: %w", err)
	}

	return i.executeWithRetry(ctx, options, func() (string, error) {
		req := &functionv1.InvokeRequest{
			FunctionId:     functionID,
			IdempotencyKey: options.IdempotencyKey,
			Payload:        []byte(payload),
			Metadata:       map[string]string{},
		}
		for k, v := range options.Headers {
			req.Metadata[k] = v
		}

		callCtx, cancel := i.callContext(ctx, options.Timeout)
		defer cancel()

		resp, err := i.client.StartJob(callCtx, req)
		if err != nil {
			return "", err
		}

		return resp.GetJobId(), nil
	})
}

// StreamJob implements Invoker.StreamJob
func (i *invoker) StreamJob(ctx context.Context, jobID string) (<-chan JobEvent, error) {
	eventCh := make(chan JobEvent, 10)

	// connect() is now thread-safe with double-checked locking
	if err := i.connect(ctx); err != nil {
		if i.isConnectionError(err) {
			i.scheduleReconnectIfNeeded()
		}
		close(eventCh)
		return eventCh, fmt.Errorf("not connected to server: %w", err)
	}

	req := &functionv1.JobStreamRequest{JobId: jobID}

	stream, err := i.client.StreamJob(ctx, req)
	if err != nil {
		close(eventCh)
		return eventCh, fmt.Errorf("stream job RPC failed: %w", err)
	}

	go func() {
		defer close(eventCh)
		for {
			evt, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					eventCh <- JobEvent{
						EventType: "error",
						JobID:     jobID,
						Error:     err.Error(),
						Done:      true,
					}
				}
				return
			}

			event := JobEvent{
				EventType: evt.GetType(),
				JobID:     jobID,
				Payload:   string(evt.GetPayload()),
				Done:      strings.EqualFold(evt.GetType(), "done"),
			}
			if evt.GetType() == "error" {
				event.Error = evt.GetMessage()
				event.Done = true
			} else if msg := evt.GetMessage(); msg != "" {
				event.Payload = msg
			}

			select {
			case eventCh <- event:
				if event.Done {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventCh, nil
}

// CancelJob implements Invoker.CancelJob
func (i *invoker) CancelJob(ctx context.Context, jobID string) error {
	// connect() is now thread-safe with double-checked locking
	if err := i.connect(ctx); err != nil {
		if i.isConnectionError(err) {
			i.scheduleReconnectIfNeeded()
		}
		return fmt.Errorf("not connected to server: %w", err)
	}

	req := &functionv1.CancelJobRequest{JobId: jobID}
	callCtx, cancel := i.callContext(ctx, 0)
	defer cancel()

	if _, err := i.client.CancelJob(callCtx, req); err != nil {
		return fmt.Errorf("cancel job RPC failed: %w", err)
	}
	return nil
}

// SetSchema implements Invoker.SetSchema
func (i *invoker) SetSchema(functionID string, schema map[string]interface{}) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.schemas[functionID] = schema
	logDebugf("Set schema for function: %s", functionID)
	return nil
}

// Close implements Invoker.Close
func (i *invoker) Close() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Cancel any pending reconnection
	if i.reconnectCancelCtx != nil {
		i.reconnectCancelCtx()
		i.reconnectCancelCtx = nil
	}

	i.connected = false
	i.client = nil
	i.isReconnecting = false
	i.reconnectAttempts = 0

	if i.conn != nil {
		i.conn.Close()
		i.conn = nil
	}

	i.schemas = make(map[string]map[string]interface{})
	logInfof("Invoker closed")
	return nil
}

// validatePayload validates payload against schema
func (i *invoker) validatePayload(payload string, schema map[string]interface{}) error {
	if len(schema) == 0 {
		if payload == "" {
			return fmt.Errorf("payload cannot be empty")
		}
		return nil
	}

	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
	payloadLoader := gojsonschema.NewStringLoader(payload)

	result, err := gojsonschema.Validate(schemaLoader, payloadLoader)
	if err != nil {
		return fmt.Errorf("schema validation error: %w", err)
	}

	if result.Valid() {
		return nil
	}

	var errs []string
	for _, desc := range result.Errors() {
		errs = append(errs, desc.String())
	}

	return fmt.Errorf("payload validation failed: %s", strings.Join(errs, "; "))
}

func (i *invoker) callContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = i.config.DefaultTimeout
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func buildTLSConfig(cfg *InvokerConfig) (*tls.Config, error) {
	tlsCfg := &tls.Config{}

	// Root CA handling
	if cfg.CAFile != "" {
		caBytes, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, fmt.Errorf("failed to append CA certificates")
		}
		tlsCfg.RootCAs = pool
	} else {
		systemPool, err := x509.SystemCertPool()
		if err == nil {
			tlsCfg.RootCAs = systemPool
		}
	}

	// Client certificate (optional)
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	// Infer server name for TLS verification
	if cfg.Address != "" {
		host := cfg.Address
		if h, _, err := net.SplitHostPort(cfg.Address); err == nil && h != "" {
			host = h
		}
		tlsCfg.ServerName = host
	}

	return tlsCfg, nil
}

// isConnectionError determines if an error is a connection-related error
func (i *invoker) isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Check for common connection error patterns
	connectionPatterns := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"network is unreachable",
		"no such host",
		"timeout",
		"transport is closing",
		"connection unavailable",
	}
	for _, pattern := range connectionPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}
	return false
}

// scheduleReconnectIfNeeded schedules a reconnection attempt if enabled
func (i *invoker) scheduleReconnectIfNeeded() {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Check if reconnection is enabled
	if !i.config.Reconnect.Enabled {
		return
	}

	// Check if already reconnecting
	if i.isReconnecting {
		return
	}

	// Check max attempts
	if i.config.Reconnect.MaxAttempts > 0 && i.reconnectAttempts >= i.config.Reconnect.MaxAttempts {
		logInfof("Max reconnection attempts (%d) reached, giving up", i.config.Reconnect.MaxAttempts)
		return
	}

	i.isReconnecting = true
	i.reconnectAttempts++

	delay := i.calculateReconnectDelay()
	logInfof("Scheduling reconnection attempt %d in %v", i.reconnectAttempts, delay)

	// Create a cancellable context for the reconnection
	reconnectCtx, cancel := context.WithCancel(context.Background())
	i.reconnectCancelCtx = cancel
	i.reconnectCancelFlag = true

	go func() {
		defer func() {
			i.mu.Lock()
			if i.reconnectCancelFlag {
				i.reconnectCancelCtx = nil
				i.reconnectCancelFlag = false
			}
			i.mu.Unlock()
			cancel()
		}()

		select {
		case <-time.After(delay):
			// Attempt reconnection
			logInfof("Reconnecting... (attempt %d)", i.reconnectAttempts)
			if err := i.connect(context.Background()); err != nil {
				logInfof("Reconnection attempt %d failed: %v", i.reconnectAttempts, err)
				// Schedule next attempt
				i.scheduleReconnectIfNeeded()
			} else {
				logInfof("Reconnection successful")
			}
		case <-reconnectCtx.Done():
			// Reconnection was cancelled
			logInfof("Reconnection cancelled")
		}
	}()
}

// calculateReconnectDelay calculates the delay before next reconnection attempt
func (i *invoker) calculateReconnectDelay() time.Duration {
	cfg := i.config.Reconnect

	// Calculate base delay using exponential backoff
	baseDelay := time.Duration(cfg.InitialDelayMs) * time.Millisecond
	exponentialDelay := baseDelay * time.Duration(math.Pow(float64(cfg.BackoffMultiplier), float64(i.reconnectAttempts-1)))

	// Cap at max delay
	maxDelay := time.Duration(cfg.MaxDelayMs) * time.Millisecond
	if exponentialDelay > maxDelay {
		exponentialDelay = maxDelay
	}

	// Add jitter to prevent thundering herd
	jitter := time.Duration(float64(exponentialDelay) * cfg.JitterFactor)
	// Random jitter in range [-jitter, +jitter]
	randomJitter := time.Duration(float64(jitter) * (2*rand.Float64() - 1))

	finalDelay := exponentialDelay + randomJitter
	if finalDelay < 0 {
		finalDelay = 0
	}

	return finalDelay
}

// executeWithRetry executes a function with retry logic
func (i *invoker) executeWithRetry(ctx context.Context, options InvokeOptions, fn func() (string, error)) (string, error) {
	// Use options retry if provided, otherwise use config retry
	retryConfig := options.Retry
	if retryConfig == nil {
		retryConfig = i.config.Retry
	}

	// If retry is disabled, execute directly
	if !retryConfig.Enabled {
		return fn()
	}

	var lastErr error
	maxAttempts := retryConfig.MaxAttempts

	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if this is a retryable error
		if !i.isRetryableError(err) || attempt >= maxAttempts-1 {
			return "", fmt.Errorf("invoke failed after %d attempts: %w", attempt+1, err)
		}

		// Connection errors should trigger reconnection
		if i.isConnectionError(err) && i.config.Reconnect.Enabled {
			i.connected = false
			i.scheduleReconnectIfNeeded()
		}

		// Calculate delay and wait
		delay := i.calculateRetryDelay(attempt, retryConfig)
		logInfof("Invocation attempt %d failed, retrying in %v: %v", attempt+1, delay, err)

		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return "", fmt.Errorf("retry cancelled: %w", ctx.Err())
		}
	}

	return "", fmt.Errorf("invoke failed after %d attempts: %w", maxAttempts, lastErr)
}

// isRetryableError checks if an error should trigger a retry based on gRPC status code
func (i *invoker) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Extract gRPC status code from error
	// The error may contain status code information in its message
	// For a production implementation, you would use status.FromError
	errStr := err.Error()

	for _, code := range i.config.Retry.RetryableStatusCodes {
		// Check if error message contains status code indicators
		// Common patterns: "code = XX", "Code: XX", etc.
		if strings.Contains(errStr, fmt.Sprintf("code = %d", code)) ||
			strings.Contains(errStr, fmt.Sprintf("Code(%d)", code)) {
			return true
		}
	}

	// Also check for common retryable error patterns
	retryablePatterns := []string{
		"unavailable",
		"internal error",
		"deadline exceeded",
		"aborted",
		"transport is closing",
	}

	errLower := strings.ToLower(errStr)
	for _, pattern := range retryablePatterns {
		if strings.Contains(errLower, pattern) {
			return true
		}
	}

	return false
}

// calculateRetryDelay calculates the delay before next retry attempt
func (i *invoker) calculateRetryDelay(attempt int, cfg *RetryConfig) time.Duration {
	// Calculate base delay using exponential backoff
	baseDelay := time.Duration(cfg.InitialDelayMs) * time.Millisecond
	exponentialDelay := baseDelay * time.Duration(math.Pow(float64(cfg.BackoffMultiplier), float64(attempt)))

	// Cap at max delay
	maxDelay := time.Duration(cfg.MaxDelayMs) * time.Millisecond
	if exponentialDelay > maxDelay {
		exponentialDelay = maxDelay
	}

	// Add jitter to prevent thundering herd
	jitter := time.Duration(float64(exponentialDelay) * cfg.JitterFactor)
	// Random jitter in range [-jitter, +jitter]
	randomJitter := time.Duration(float64(jitter) * (2*rand.Float64() - 1))

	finalDelay := exponentialDelay + randomJitter
	if finalDelay < 0 {
		finalDelay = 0
	}

	return finalDelay
}
