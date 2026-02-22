// Package croupier provides a Go SDK for Croupier game function registration and execution.
package croupier

import (
	"compress/gzip"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	agentv1 "github.com/cuihairu/croupier/sdks/go/pkg/pb/croupier/agent/v1"
	sdkv1 "github.com/cuihairu/croupier/sdks/go/pkg/pb/croupier/sdk/v1"
	"google.golang.org/protobuf/proto"

	"github.com/cuihairu/croupier/sdks/go/pkg/croupier/protocol"
	"github.com/cuihairu/croupier/sdks/go/pkg/croupier/transport"
)

// nngManagerConfig holds configuration for NNGManager
type nngManagerConfig struct {
	AgentAddr          string
	AgentIPCAddr       string
	LocalListen        string
	TimeoutSeconds     int
	Insecure           bool
	CAFile             string
	CertFile           string
	KeyFile            string
	ServerName         string
	InsecureSkipVerify bool
}

// NNGManager implements Manager interface using NNG transport
type NNGManager struct {
	config   nngManagerConfig
	handlers map[string]FunctionHandler

	// Transport layer
	client   *transport.Client
	server   *transport.Server
	serverMu sync.Mutex

	// Connection state
	mu        sync.RWMutex
	connected bool
	localAddr string

	// Session management
	sessionID      string
	serviceID      string
	serviceVersion string
	heartbeatStop  context.CancelFunc

	// Server for handling incoming RPC calls
	rpcHandler *rpcHandler

	// Job management
	jobs      map[string]*Job
	jobsMutex sync.RWMutex
	jobsSeq   uint64
}

// Job represents an async job execution
type Job struct {
	ID         string
	FunctionID string
	Payload    []byte
	Status     agentv1.JobStatus
	CreatedAt  int64
	UpdatedAt  int64
	Result     []byte
	Error      string
	Progress   int32
	Cancel     context.CancelFunc
}

// rpcHandler implements the transport.Handler interface for NNG
type rpcHandler struct {
	manager *NNGManager
	methods map[uint32]func(ctx context.Context, reqID uint32, body []byte) (respBody []byte, err error)
}

// NewNNGManager creates a new NNG-based manager
func NewNNGManager(config ClientConfig, handlers map[string]FunctionHandler) (Manager, error) {
	n := &NNGManager{
		config: nngManagerConfig{
			AgentAddr:          config.AgentAddr,
			AgentIPCAddr:       config.AgentIPCAddr,
			LocalListen:        config.LocalListen,
			TimeoutSeconds:     config.TimeoutSeconds,
			Insecure:           config.Insecure,
			CAFile:             config.CAFile,
			CertFile:           config.CertFile,
			KeyFile:            config.KeyFile,
			ServerName:         config.ServerName,
			InsecureSkipVerify: config.InsecureSkipVerify,
		},
		handlers: handlers,
		jobs:     make(map[string]*Job),
	}
	n.rpcHandler = newRPCHandler(n)
	return n, nil
}

// newRPCHandler creates a new RPC handler
func newRPCHandler(manager *NNGManager) *rpcHandler {
	handler := &rpcHandler{
		manager: manager,
		methods: make(map[uint32]func(ctx context.Context, reqID uint32, body []byte) (respBody []byte, err error)),
	}
	// Register Invoke method
	handler.methods[protocol.MsgInvokeRequest] = handler.invoke
	// Register StartJob method
	handler.methods[protocol.MsgStartJobRequest] = handler.startJob
	// Register CancelJob method
	handler.methods[protocol.MsgCancelJobRequest] = handler.cancelJob
	// Register StreamJob method
	handler.methods[protocol.MsgStreamJobRequest] = handler.streamJob
	return handler
}

// Connect implements Manager.Connect
func (n *NNGManager) Connect(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.connected {
		return nil
	}

	logInfof("Connecting to Croupier Agent: %s", n.config.AgentAddr)

	// Create client connection to agent with IPC support
	transportCfg := &transport.Config{
		Address:     n.config.AgentAddr,
		IPCAddress:  n.config.AgentIPCAddr,
		Insecure:    n.config.Insecure,
		CAFile:      n.config.CAFile,
		CertFile:    n.config.CertFile,
		KeyFile:     n.config.KeyFile,
		ServerName:  n.config.ServerName,
		SendTimeout: time.Duration(n.config.TimeoutSeconds) * time.Second,
	}

	client, err := transport.NewClient(transportCfg)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	n.client = client
	n.connected = true

	logInfof("Successfully connected to Agent")
	return nil
}

// Disconnect implements Manager.Disconnect
func (n *NNGManager) Disconnect() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.connected = false
	n.sessionID = ""
	n.serviceID = ""
	n.serviceVersion = ""
	n.stopHeartbeat()

	n.stopLocalServer()

	if n.client != nil {
		n.client.Close()
		n.client = nil
	}

	logInfof("Disconnected from Agent")
}

// GetLocalAddress implements Manager.GetLocalAddress
func (n *NNGManager) GetLocalAddress() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.localAddr
}

// IsConnected implements Manager.IsConnected
func (n *NNGManager) IsConnected() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.connected
}

// StartServer implements Manager.StartServer
func (n *NNGManager) StartServer(ctx context.Context) error {
	return n.startLocalServer(ctx)
}

// RegisterWithAgent implements Manager.RegisterWithAgent
func (n *NNGManager) RegisterWithAgent(ctx context.Context, serviceID, serviceVersion string, functions []LocalFunctionDescriptor) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.client == nil {
		return "", fmt.Errorf("not connected to agent. Call Connect() first")
	}

	n.serviceID = serviceID
	n.serviceVersion = serviceVersion

	// Build RegisterClientRequest
	req := &sdkv1.RegisterLocalRequest{
		ServiceId: serviceID,
		Version:   serviceVersion,
		RpcAddr:   n.localAddr,
		Functions: convertToProtoFunctions(functions),
	}

	// Marshal request
	reqBytes, err := proto.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	// Send to agent using transport layer
	_, respBody, err := n.client.Call(ctx, protocol.MsgRegisterClientRequest, reqBytes)
	if err != nil {
		return "", err
	}

	// Parse response
	resp := &sdkv1.RegisterLocalResponse{}
	if err := proto.Unmarshal(respBody, resp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	sessionID := resp.GetSessionId()

	n.mu.Lock()
	n.sessionID = sessionID
	n.mu.Unlock()

	// Start heartbeat after successful registration
	n.startHeartbeat()

	// Register capabilities (optional, non-blocking)
	go func() {
		if err := n.registerCapabilities(context.Background()); err != nil {
			logDebugf("Failed to register capabilities: %v", err)
		}
	}()

	return sessionID, nil
}

// startLocalServer starts the local NNG server for handling RPC calls
func (n *NNGManager) startLocalServer(ctx context.Context) error {
	n.serverMu.Lock()
	defer n.serverMu.Unlock()

	if n.server != nil {
		return nil // Already running
	}

	// Parse listen address - use port 0 for auto-assign or config-specified
	listenAddr := n.config.LocalListen
	if listenAddr == "" {
		listenAddr = "127.0.0.1:0" // Auto-assign port
	}

	serverCfg := &transport.Config{
		Address:     listenAddr,
		Insecure:    n.config.Insecure,
		RecvTimeout: 30 * time.Second,
	}

	server, err := transport.NewServer(serverCfg, n.rpcHandler)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	// Start server in background
	go func() {
		_ = server.Serve(ctx)
	}()

	// Get the actual address from the server (resolves port 0 to assigned port)
	n.localAddr = server.GetActualAddress()
	if n.localAddr == "" {
		// Fallback to config address if server doesn't provide actual address
		n.localAddr = listenAddr
	}

	n.server = server
	return nil
}

// stopLocalServer stops the local NNG server
func (n *NNGManager) stopLocalServer() {
	n.serverMu.Lock()
	defer n.serverMu.Unlock()

	if n.server != nil {
		n.server.Close()
		n.server = nil
	}
}

// registerCapabilities registers capabilities with the control service
func (n *NNGManager) registerCapabilities(ctx context.Context) error {
	// Check if we have a client connection
	if n.client == nil {
		return fmt.Errorf("not connected to agent")
	}

	// Build provider metadata
	provider := &agentv1.ProviderMeta{
		Id:      n.serviceID,
		Version: n.serviceVersion,
		Lang:    "go",
		Sdk:     "croupier-go",
	}

	// Build capabilities manifest from registered functions
	manifest, err := n.buildCapabilitiesManifest()
	if err != nil {
		return fmt.Errorf("build capabilities manifest: %w", err)
	}

	// Compress manifest with gzip
	var manifestBuf bytes.Buffer
	gzipWriter := gzip.NewWriter(&manifestBuf)
	if err := json.NewEncoder(gzipWriter).Encode(manifest); err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("compress manifest: %w", err)
	}

	// Build request
	req := &agentv1.RegisterCapabilitiesRequest{
		Provider:       provider,
		ManifestJsonGz: manifestBuf.Bytes(),
	}

	// Marshal request
	reqBytes, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Send to agent using transport layer
	_, respBody, err := n.client.Call(ctx, protocol.MsgRegisterCapabilitiesReq, reqBytes)
	if err != nil {
		return fmt.Errorf("send capabilities registration: %w", err)
	}

	// Parse response
	resp := &agentv1.RegisterCapabilitiesResponse{}
	if err := proto.Unmarshal(respBody, resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	logInfof("Successfully registered capabilities for service: %s", n.serviceID)
	return nil
}

// buildCapabilitiesManifest builds a capabilities manifest from registered functions
func (n *NNGManager) buildCapabilitiesManifest() (map[string]interface{}, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	manifest := map[string]interface{}{
		"type":       "function-provider",
		"version":    "1.0",
		"serviceID":  n.serviceID,
		"functions":  []map[string]interface{}{},
	}

	// Add function descriptors
	var functions []map[string]interface{}
	for funcID := range n.handlers {
		funcDesc := map[string]interface{}{
			"id":      funcID,
			"version": "1.0.0",
		}
		functions = append(functions, funcDesc)
	}

	manifest["functions"] = functions
	return manifest, nil
}

func (n *NNGManager) startHeartbeat() {
	n.stopHeartbeat()

	ctx, cancel := context.WithCancel(context.Background())
	n.heartbeatStop = cancel

	sessionID := n.sessionID
	serviceID := n.serviceID

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				req := &sdkv1.HeartbeatRequest{
					ServiceId: serviceID,
					SessionId: sessionID,
				}
				reqBytes, _ := proto.Marshal(req)
				_, _, err := n.client.Call(context.Background(), protocol.MsgClientHeartbeatRequest, reqBytes)
				if err != nil {
					logDebugf("Agent heartbeat failed: %v", err)
				}
			}
		}
	}()
}

func (n *NNGManager) stopHeartbeat() {
	if n.heartbeatStop != nil {
		n.heartbeatStop()
		n.heartbeatStop = nil
	}
}

// convertToLocalFunctions converts handlers to LocalFunctionDescriptors
func (n *NNGManager) convertToLocalFunctions() []LocalFunctionDescriptor {
	var localFuncs []LocalFunctionDescriptor

	n.mu.RLock()
	defer n.mu.RUnlock()

	for funcID := range n.handlers {
		localFuncs = append(localFuncs, LocalFunctionDescriptor{
			ID:      funcID,
			Version: "1.0.0", // Default version
		})
	}
	return localFuncs
}

// Handle implements transport.Handler for rpcHandler
func (h *rpcHandler) Handle(ctx context.Context, msgID uint32, reqID uint32, body []byte) (respBody []byte, err error) {
	// Look up method by MsgID
	method, ok := h.methods[msgID]
	if !ok {
		return nil, fmt.Errorf("unknown MsgID: 0x%06X", msgID)
	}

	return method(ctx, reqID, body)
}

// methodMap maps MsgID to RPC method names
var methodMap = map[uint32]string{
	protocol.MsgInvokeRequest:    "Invoke",
	protocol.MsgStartJobRequest:  "StartJob",
	protocol.MsgCancelJobRequest: "CancelJob",
	protocol.MsgStreamJobRequest: "StreamJob",
}

func (h *rpcHandler) invoke(ctx context.Context, reqID uint32, body []byte) ([]byte, error) {
	req := &sdkv1.InvokeRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		return nil, err
	}

	// Look up handler from manager
	h.manager.mu.RLock()
	handler, ok := h.manager.handlers[req.GetFunctionId()]
	h.manager.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("function not found: %s", req.GetFunctionId())
	}

	// Call handler
	resp, err := handler(ctx, req.GetPayload())
	if err != nil {
		return nil, err
	}

	// Build response
	respMsg := &sdkv1.InvokeResponse{
		Payload: resp,
	}

	return proto.Marshal(respMsg)
}

func (h *rpcHandler) startJob(ctx context.Context, reqID uint32, body []byte) ([]byte, error) {
	req := &sdkv1.InvokeRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		return nil, err
	}

	// Look up handler from manager
	h.manager.mu.RLock()
	handler, ok := h.manager.handlers[req.GetFunctionId()]
	h.manager.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("function not found: %s", req.GetFunctionId())
	}

	// Generate job ID
	h.manager.jobsMutex.Lock()
	h.manager.jobsSeq++
	jobID := fmt.Sprintf("job-%d", h.manager.jobsSeq)
	h.manager.jobsMutex.Unlock()

	// Create job context with cancellation
	jobCtx, jobCancel := context.WithCancel(context.Background())

	// Create job record
	now := time.Now().Unix()
	job := &Job{
		ID:         jobID,
		FunctionID: req.GetFunctionId(),
		Payload:    req.GetPayload(),
		Status:     agentv1.JobStatus_JOB_STATUS_PENDING,
		CreatedAt:  now,
		UpdatedAt:  now,
		Cancel:     jobCancel,
	}

	// Store job
	h.manager.jobsMutex.Lock()
	h.manager.jobs[jobID] = job
	h.manager.jobsMutex.Unlock()

	// Start job execution in background
	go h.executeJob(jobCtx, job, handler)

	// Return job ID immediately
	respMsg := &sdkv1.StartJobResponse{
		JobId: jobID,
	}

	return proto.Marshal(respMsg)
}

// executeJob executes a job asynchronously
func (h *rpcHandler) executeJob(ctx context.Context, job *Job, handler FunctionHandler) {
	// Update status to running
	h.manager.jobsMutex.Lock()
	job.Status = agentv1.JobStatus_JOB_STATUS_RUNNING
	job.UpdatedAt = time.Now().Unix()
	h.manager.jobsMutex.Unlock()

	// Execute the handler
	result, err := handler(ctx, job.Payload)

	// Update job with result
	h.manager.jobsMutex.Lock()
	defer h.manager.jobsMutex.Unlock()

	if err != nil {
		job.Status = agentv1.JobStatus_JOB_STATUS_FAILED
		job.Error = err.Error()
	} else {
		job.Status = agentv1.JobStatus_JOB_STATUS_COMPLETED
		job.Result = result
	}
	job.UpdatedAt = time.Now().Unix()
}

func (h *rpcHandler) cancelJob(ctx context.Context, reqID uint32, body []byte) ([]byte, error) {
	req := &sdkv1.CancelJobRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		return nil, err
	}

	jobID := req.GetJobId()

	// Find job
	h.manager.jobsMutex.Lock()
	job, ok := h.manager.jobs[jobID]
	if !ok {
		h.manager.jobsMutex.Unlock()
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	// Check if job can be cancelled
	if job.Status != agentv1.JobStatus_JOB_STATUS_PENDING &&
	   job.Status != agentv1.JobStatus_JOB_STATUS_RUNNING {
		h.manager.jobsMutex.Unlock()
		return nil, fmt.Errorf("job cannot be cancelled, current status: %v", job.Status)
	}

	// Cancel the job context
	if job.Cancel != nil {
		job.Cancel()
	}

	// Update job status
	job.Status = agentv1.JobStatus_JOB_STATUS_CANCELLED
	job.UpdatedAt = time.Now().Unix()
	h.manager.jobsMutex.Unlock()

	// Return empty success response
	return proto.Marshal(&sdkv1.InvokeResponse{})
}

func (h *rpcHandler) streamJob(ctx context.Context, reqID uint32, body []byte) ([]byte, error) {
	req := &sdkv1.JobStreamRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		return nil, err
	}

	jobID := req.GetJobId()

	// Find job
	h.manager.jobsMutex.Lock()
	job, ok := h.manager.jobs[jobID]
	if !ok {
		h.manager.jobsMutex.Unlock()
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	// Create job event based on current status
	event := &sdkv1.JobEvent{}

	switch job.Status {
	case agentv1.JobStatus_JOB_STATUS_PENDING:
		event.Type = "progress"
		event.Message = "Job is pending"
		event.Progress = 0

	case agentv1.JobStatus_JOB_STATUS_RUNNING:
		event.Type = "progress"
		event.Message = "Job is running"
		event.Progress = job.Progress

	case agentv1.JobStatus_JOB_STATUS_COMPLETED:
		event.Type = "done"
		event.Message = "Job completed successfully"
		event.Payload = job.Result

	case agentv1.JobStatus_JOB_STATUS_FAILED:
		event.Type = "error"
		event.Message = job.Error

	case agentv1.JobStatus_JOB_STATUS_CANCELLED:
		event.Type = "error"
		event.Message = "Job was cancelled"

	default:
		event.Type = "error"
		event.Message = fmt.Sprintf("Unknown job status: %v", job.Status)
	}

	h.manager.jobsMutex.Unlock()

	// Return event
	return proto.Marshal(event)
}

// convertToProtoFunctions converts LocalFunctionDescriptor to protobuf
func convertToProtoFunctions(funcs []LocalFunctionDescriptor) []*sdkv1.LocalFunctionDescriptor {
	result := make([]*sdkv1.LocalFunctionDescriptor, len(funcs))
	for i, f := range funcs {
		result[i] = &sdkv1.LocalFunctionDescriptor{
			Id:           f.ID,
			Version:      f.Version,
			Tags:         f.Tags,
			Summary:      f.Summary,
			Description:  f.Description,
			OperationId:  f.OperationID,
			Deprecated:   f.Deprecated,
			InputSchema:  f.InputSchema,
			OutputSchema: f.OutputSchema,
			Category:     f.Category,
			Risk:         f.Risk,
			Entity:       f.Entity,
			Operation:    f.Operation,
		}
	}
	return result
}

// createTLSConfig creates TLS configuration from client config
func createNNGTLSConfig(cfg ClientConfig) (*tls.Config, error) {
	if cfg.Insecure {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Load CA certificate
	if cfg.CAFile != "" {
		pemBytes, err := os.ReadFile(cfg.CAFile)
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
		if err == nil {
			tlsConfig.RootCAs = systemCAs
		}
	}

	// Load client certificate for mTLS
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Configure server name verification
	if cfg.ServerName != "" {
		tlsConfig.ServerName = cfg.ServerName
	} else if !cfg.InsecureSkipVerify && cfg.AgentAddr != "" {
		host, _, err := net.SplitHostPort(cfg.AgentAddr)
		if err == nil && host != "" {
			tlsConfig.ServerName = host
		}
	}

	// Skip verification if explicitly requested
	if cfg.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	return tlsConfig, nil
}
