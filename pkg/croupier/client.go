package croupier

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	controlv1 "github.com/cuihairu/croupier/pkg/pb/croupier/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// NewGRPCManager is provided in this package (moved from internal)

// Client represents a Croupier client for function registration and execution
type Client interface {
	// RegisterFunction registers a single function with the agent
	RegisterFunction(desc FunctionDescriptor, handler FunctionHandler) error

	// Connect establishes connection to the agent
	Connect(ctx context.Context) error

	// Serve starts the local gRPC server and maintains the connection
	Serve(ctx context.Context) error

	// Stop gracefully stops the client
	Stop() error

	// Close closes the client and cleans up resources
	Close() error

	// GetLocalAddress returns the local server address
	GetLocalAddress() string
}

// Invoker represents a Croupier invoker for calling functions
type Invoker interface {
	// Connect establishes connection to the server/agent
	Connect(ctx context.Context) error

	// Invoke synchronously calls a function
	Invoke(ctx context.Context, functionID, payload string, options InvokeOptions) (string, error)

	// StartJob starts an asynchronous job
	StartJob(ctx context.Context, functionID, payload string, options InvokeOptions) (string, error)

	// StreamJob streams events from a running job
	StreamJob(ctx context.Context, jobID string) (<-chan JobEvent, error)

	// CancelJob cancels a running job
	CancelJob(ctx context.Context, jobID string) error

	// SetSchema sets validation schema for a function
	SetSchema(functionID string, schema map[string]interface{}) error

	// Close closes the invoker
	Close() error
}

// client implements the Client interface
type client struct {
	config      *ClientConfig
	handlers    map[string]FunctionHandler
	descriptors map[string]FunctionDescriptor
	mu          sync.RWMutex

	// gRPC related fields
	grpcManager GRPCManager
	sessionID   string
	localAddr   string

	// State management
	connected bool
	running   bool
	stopCh    chan struct{}

	// Logging
	logger Logger
}

// NewClient creates a new Croupier client
func NewClient(config *ClientConfig) Client {
	if config == nil {
		config = DefaultClientConfig()
	}

	// Set up logger based on config
	var logger Logger
	if config.DisableLogging {
		logger = &NoOpLogger{}
	} else {
		logger = NewDefaultLogger(config.DebugLogging, os.Stdout)
	}

	return &client{
		config:      config,
		handlers:    make(map[string]FunctionHandler),
		descriptors: make(map[string]FunctionDescriptor),
		stopCh:      make(chan struct{}),
		logger:      logger,
	}
}

// RegisterFunction implements Client.RegisterFunction
func (c *client) RegisterFunction(desc FunctionDescriptor, handler FunctionHandler) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("cannot register functions while client is running")
	}

	// Validate function descriptor
	if desc.ID == "" {
		return fmt.Errorf("function ID cannot be empty")
	}
	if desc.Version == "" {
		desc.Version = "1.0.0"
	}

	c.handlers[desc.ID] = handler
	c.descriptors[desc.ID] = desc
	c.logger.Infof("Registered function: %s (version: %s)", desc.ID, desc.Version)
	return nil
}

// Connect implements Client.Connect
func (c *client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	c.logger.Infof("Connecting to Croupier Agent: %s", c.config.AgentAddr)

	// Initialize gRPC manager
	grpcConfig := GRPCConfig{
		AgentAddr:          c.config.AgentAddr,
		LocalListen:        c.config.LocalListen,
		TimeoutSeconds:     c.config.TimeoutSeconds,
		Insecure:           c.config.Insecure,
		CAFile:             c.config.CAFile,
		CertFile:           c.config.CertFile,
		KeyFile:            c.config.KeyFile,
		ServerName:         c.config.ServerName,
		InsecureSkipVerify: c.config.InsecureSkipVerify,
	}

	var err error
	c.grpcManager, err = NewGRPCManager(grpcConfig, c.handlers)
	if err != nil {
		return fmt.Errorf("failed to create gRPC manager: %w", err)
	}

	// Connect to agent
	if err := c.grpcManager.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to agent: %w", err)
	}

	// Ensure local server is running before registration
	if err := c.grpcManager.StartServer(ctx); err != nil {
		c.grpcManager.Disconnect()
		return fmt.Errorf("failed to start local server: %w", err)
	}

	// Register functions with agent
	localFunctions := c.convertToLocalFunctions()
	sessionID, err := c.grpcManager.RegisterWithAgent(ctx, c.config.ServiceID, c.config.ServiceVersion, localFunctions)
	if err != nil {
		c.grpcManager.Disconnect()
		return fmt.Errorf("failed to register with agent: %w", err)
	}

	c.sessionID = sessionID
	c.localAddr = c.grpcManager.GetLocalAddress()
	c.connected = true

	c.logger.Infof("Successfully connected and registered with Agent")
	c.logger.Infof("Local service address: %s", c.localAddr)
	c.logger.Infof("Session ID: %s", c.sessionID)

	if err := c.registerCapabilities(ctx); err != nil {
		c.logger.Warnf("Failed to register capabilities: %v", err)
	}

	return nil
}

// Serve implements Client.Serve
func (c *client) Serve(ctx context.Context) error {
	if !c.connected {
		if err := c.Connect(ctx); err != nil {
			return fmt.Errorf("connection failed, cannot start service: %w", err)
		}
	}

	c.running = true
	c.logger.Infof("Croupier client service started")
	c.logger.Infof("Local service address: %s", c.localAddr)
	c.logger.Infof("Registered functions: %d", len(c.handlers))
	c.logger.Infof("Use Stop() to stop the service")
	c.logger.Infof("===============================================")

	// Start local gRPC server
	if err := c.grpcManager.StartServer(ctx); err != nil {
		return fmt.Errorf("failed to start local server: %w", err)
	}

	// Wait for stop signal or context cancellation
	select {
	case <-c.stopCh:
		c.logger.Infof("Service stopped by Stop() call")
	case <-ctx.Done():
		c.logger.Infof("Service stopped by context cancellation")
	}

	c.running = false
	c.logger.Infof("Service has stopped")
	return nil
}

// Stop implements Client.Stop
func (c *client) Stop() error {
	c.running = false
	c.connected = false

	c.logger.Infof("Stopping Croupier client...")

	if c.grpcManager != nil {
		c.grpcManager.Disconnect()
	}

	close(c.stopCh)
	c.logger.Infof("Client stopped successfully")
	return nil
}

// Close implements Client.Close
func (c *client) Close() error {
	c.Stop()
	c.handlers = nil
	return nil
}

// GetLocalAddress implements Client.GetLocalAddress
func (c *client) GetLocalAddress() string {
	return c.localAddr
}

// convertToLocalFunctions converts FunctionDescriptors to LocalFunctionDescriptors
// Note: This method must be called while holding c.mu (either read or write lock)
func (c *client) convertToLocalFunctions() []LocalFunctionDescriptor {
	var localFuncs []LocalFunctionDescriptor
	for funcID := range c.handlers {
		desc, ok := c.descriptors[funcID]
		if !ok {
			continue
		}
		version := desc.Version
		if version == "" {
			version = "1.0.0"
		}
		localFuncs = append(localFuncs, LocalFunctionDescriptor{
			ID:      funcID,
			Version: version,
		})
	}
	return localFuncs
}

func (c *client) registerCapabilities(ctx context.Context) error {
	if c.config.ControlAddr == "" {
		return nil
	}

	manifestBytes, err := c.buildManifest()
	if err != nil {
		return fmt.Errorf("build manifest: %w", err)
	}
	compressed, err := gzipBytes(manifestBytes)
	if err != nil {
		return fmt.Errorf("compress manifest: %w", err)
	}

	conn, err := c.dialControl(ctx)
	if err != nil {
		return fmt.Errorf("connect to control service: %w", err)
	}
	defer conn.Close()

	client := controlv1.NewControlServiceClient(conn)
	request := &controlv1.RegisterCapabilitiesRequest{
		Provider: &controlv1.ProviderMeta{
			Id:      c.config.ServiceID,
			Version: c.config.ServiceVersion,
			Lang:    c.config.ProviderLang,
			Sdk:     c.config.ProviderSDK,
		},
		ManifestJsonGz: compressed,
	}

	callCtx := ctx
	var cancel context.CancelFunc
	if c.config.TimeoutSeconds > 0 {
		timeout := time.Duration(c.config.TimeoutSeconds) * time.Second
		callCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if _, err := client.RegisterCapabilities(callCtx, request); err != nil {
		return err
	}

	c.logger.Infof("Uploaded provider capabilities manifest")
	return nil
}

func (c *client) dialControl(ctx context.Context) (*grpc.ClientConn, error) {
	if c.config.ControlAddr == "" {
		return nil, fmt.Errorf("control address not configured")
	}

	dialCtx := ctx
	var cancel context.CancelFunc
	if c.config.TimeoutSeconds > 0 {
		timeout := time.Duration(c.config.TimeoutSeconds) * time.Second
		dialCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var opts []grpc.DialOption
	if c.config.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		creds, err := c.controlCredentials()
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}

	conn, err := grpc.DialContext(dialCtx, c.config.ControlAddr, opts...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (c *client) controlCredentials() (credentials.TransportCredentials, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Load CA certificate
	if c.config.CAFile != "" {
		pemBytes, err := os.ReadFile(c.config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("failed to append CA certificate")
		}
		tlsConfig.RootCAs = pool
	} else {
		// Use system root CAs
		systemCAs, err := x509.SystemCertPool()
		if err != nil {
			// Fallback to empty pool if system CAs unavailable
			systemCAs = x509.NewCertPool()
		}
		tlsConfig.RootCAs = systemCAs
	}

	// Load client certificate for mTLS
	if c.config.CertFile != "" && c.config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.config.CertFile, c.config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Configure server name verification
	if c.config.ServerName != "" {
		tlsConfig.ServerName = c.config.ServerName
	} else if !c.config.InsecureSkipVerify {
		// Extract server name from address
		host, _, err := net.SplitHostPort(c.config.ControlAddr)
		if err == nil && host != "" {
			tlsConfig.ServerName = host
		}
	}

	// Skip verification if explicitly requested
	if c.config.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	return credentials.NewTLS(tlsConfig), nil
}

func (c *client) buildManifest() ([]byte, error) {
	type manifestFunction struct {
		ID        string `json:"id"`
		Version   string `json:"version"`
		Category  string `json:"category,omitempty"`
		Risk      string `json:"risk,omitempty"`
		Entity    string `json:"entity,omitempty"`
		Operation string `json:"operation,omitempty"`
		Enabled   bool   `json:"enabled,omitempty"`
	}
	type manifest struct {
		Provider struct {
			ID      string `json:"id"`
			Version string `json:"version"`
			Lang    string `json:"lang"`
			SDK     string `json:"sdk"`
		} `json:"provider"`
		Functions []manifestFunction `json:"functions,omitempty"`
	}

	out := manifest{}
	out.Provider.ID = c.config.ServiceID
	out.Provider.Version = c.config.ServiceVersion
	out.Provider.Lang = c.config.ProviderLang
	out.Provider.SDK = c.config.ProviderSDK

	c.mu.RLock()
	for _, desc := range c.descriptors {
		version := desc.Version
		if version == "" {
			version = "1.0.0"
		}
		out.Functions = append(out.Functions, manifestFunction{
			ID:        desc.ID,
			Version:   version,
			Category:  desc.Category,
			Risk:      desc.Risk,
			Entity:    desc.Entity,
			Operation: desc.Operation,
			Enabled:   desc.Enabled,
		})
	}
	c.mu.RUnlock()

	return json.Marshal(out)
}

func gzipBytes(payload []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(payload); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
