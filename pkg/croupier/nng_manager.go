// Package croupier provides a Go SDK for Croupier game function registration and execution.
package croupier

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

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
	n.serviceID = serviceID
	n.serviceVersion = serviceVersion
	n.mu.Unlock()

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

	// Get the actual address from the server
	// For now, we'll use the config address or wait for it to be set
	n.localAddr = listenAddr
	if listenAddr == "127.0.0.1:0" {
		// TODO: Get actual address from server
		n.localAddr = "127.0.0.1:0" // Placeholder
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
	// For now, skip capability registration via NNG
	// This would require creating a separate client to the control service
	// TODO: Implement NNG-based control service client
	return nil
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
	// TODO: Implement async job support
	return nil, fmt.Errorf("StartJob not yet implemented")
}

func (h *rpcHandler) cancelJob(ctx context.Context, reqID uint32, body []byte) ([]byte, error) {
	// TODO: Implement job cancellation
	return nil, fmt.Errorf("CancelJob not yet implemented")
}

func (h *rpcHandler) streamJob(ctx context.Context, reqID uint32, body []byte) ([]byte, error) {
	// TODO: Implement job streaming
	return nil, fmt.Errorf("StreamJob not yet implemented")
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
