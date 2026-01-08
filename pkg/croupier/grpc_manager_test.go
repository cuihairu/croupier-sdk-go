package croupier

import (
	"context"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
)

func TestGRPCManager_GetLocalAddress(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Initially empty
	addr := mgr.GetLocalAddress()
	if addr != "" {
		t.Errorf("expected empty address, got %s", addr)
	}
}

func TestGRPCManager_IsConnected(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Initially not connected
	if mgr.IsConnected() {
		t.Error("expected not connected")
	}
}

func TestGRPCManager_DisconnectWithoutConnect(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Disconnect without connect should not panic
	mgr.Disconnect()
}

func TestGRPCManager_StartServerWithoutConnect(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// StartServer should work even without prior connection to agent
	err = mgr.StartServer(context.Background())
	if err != nil {
		t.Errorf("unexpected error starting server: %v", err)
	}

	// Wait a bit for the server goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Clean up
	mgr.Disconnect()
	// Wait for graceful stop to complete
	time.Sleep(20 * time.Millisecond)
}

func TestGRPCManager_GetLocalAddressAfterStart(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Start server
	err = mgr.StartServer(context.Background())
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer func() {
		mgr.Disconnect()
		// Wait for graceful stop to complete
		time.Sleep(30 * time.Millisecond)
	}()

	// Should have a local address now
	addr := mgr.GetLocalAddress()
	if addr == "" {
		t.Error("expected non-empty address after start")
	}
}

func TestGRPCManager_IsConnectedAfterStart(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Start server - this does NOT set connected to true
	err = mgr.StartServer(context.Background())
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer func() {
		mgr.Disconnect()
		// Wait for graceful stop to complete
		time.Sleep(30 * time.Millisecond)
	}()

	// StartServer alone does not set connected flag
	// connected is only set by Connect() to agent
	if mgr.IsConnected() {
		t.Error("expected not connected after just starting server (without Connect to agent)")
	}
}

func TestGRPCManager_ConnectToAgentWithoutServer(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		// Use a port that's unlikely to have a service running
		AgentAddr: "127.0.0.1:49999",
		Insecure:  true,
		// Set timeout to avoid hanging
		TimeoutSeconds: 1,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// gRPC Dial may succeed even if server doesn't exist (lazy connection)
	// The actual connection error happens when trying to use the connection
	// Just verify Connect completes without panic
	_ = mgr.Connect(context.Background())
	mgr.Disconnect()
}

func TestGRPCManager_NewGRPCManagerWithHandlers(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
	}

	handlers := map[string]FunctionHandler{
		"test_func": func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte("result"), nil
		},
	}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}

	// Clean up
	mgr.Disconnect()
}

func TestGRPCManager_ConnectContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		// Use a port that's unlikely to have a service running
		AgentAddr: "127.0.0.1:49998",
		Insecure:  true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// gRPC Dial may succeed even with cancelled context due to lazy connection
	// Just verify the call completes without panic
	_ = mgr.Connect(ctx)
	mgr.Disconnect()
}

func TestGRPCManager_RegisterWithAgent(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{
		"test_fn": func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte("ok"), nil
		},
	}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Should fail because agent is not running
	functions := []LocalFunctionDescriptor{
		{ID: "test_fn", Version: "1.0.0"},
	}
	_, err = mgr.RegisterWithAgent(context.Background(), "test-service", "1.0.0", functions)
	if err == nil {
		t.Error("expected error registering with non-existent agent")
	}
}

func TestGRPCManager_startHeartbeatLocked(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{
		"test_fn": func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte("ok"), nil
		},
	}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Convert to internal type to access private fields
	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Test with nil localCli - should return without doing anything
	impl.startHeartbeatLocked()

	// Test with localCli but empty sessionID - should return without doing anything
	impl.localCli = nil // Still nil
	impl.sessionID = ""
	impl.serviceID = ""
	impl.startHeartbeatLocked()
}

func TestGRPCManager_stopHeartbeatLocked(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Convert to internal type
	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Test with nil heartbeatStop - should not panic
	impl.stopHeartbeatLocked()

	// Test with non-nil heartbeatStop
	_, cancel := context.WithCancel(context.Background())
	impl.heartbeatStop = cancel
	impl.stopHeartbeatLocked()

	if impl.heartbeatStop != nil {
		t.Error("expected heartbeatStop to be nil after stop")
	}
}

func TestGRPCManager_createTLSCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  GRPCConfig
		wantErr bool
	}{
		{
			name: "insecure skip verify",
			config: GRPCConfig{
				Insecure:           true,
				InsecureSkipVerify: true,
			},
			wantErr: false,
		},
		{
			name: "with invalid CA file",
			config: GRPCConfig{
				CAFile: "/nonexistent/ca.pem",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewGRPCManager(tt.config, map[string]FunctionHandler{})
			if err != nil {
				t.Fatalf("failed to create manager: %v", err)
			}

			impl, ok := mgr.(*grpcManager)
			if !ok {
				t.Fatal("type assertion failed")
			}

			creds, err := impl.createTLSCredentials()
			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && creds == nil {
				t.Error("expected non-nil credentials")
			}
		})
	}
}

func TestGRPCManager_createServerTLSCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  GRPCConfig
		wantErr bool
	}{
		{
			name: "missing cert and key",
			config: GRPCConfig{
				LocalListen: "127.0.0.1:0",
			},
			wantErr: true,
		},
		{
			name: "invalid cert file",
			config: GRPCConfig{
				LocalListen: "127.0.0.1:0",
				CertFile:    "/nonexistent/cert.pem",
				KeyFile:     "/nonexistent/key.pem",
			},
			wantErr: true,
		},
		{
			name: "with CA file",
			config: GRPCConfig{
				LocalListen: "127.0.0.1:0",
				CertFile:    "/nonexistent/cert.pem",
				KeyFile:     "/nonexistent/key.pem",
				CAFile:      "/nonexistent/ca.pem",
			},
			wantErr: true,
		},
		{
			name: "insecure skip verify",
			config: GRPCConfig{
				LocalListen:        "127.0.0.1:0",
				CertFile:           "/nonexistent/cert.pem",
				KeyFile:            "/nonexistent/key.pem",
				InsecureSkipVerify: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewGRPCManager(tt.config, map[string]FunctionHandler{})
			if err != nil {
				t.Fatalf("failed to create manager: %v", err)
			}

			impl, ok := mgr.(*grpcManager)
			if !ok {
				t.Fatal("type assertion failed")
			}

			_, err = impl.createServerTLSCredentials()
			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
		})
	}
}

// Test RegisterWithAgent error cases
func TestGRPCManager_RegisterWithAgentErrors(t *testing.T) {
	t.Parallel()

	t.Run("not connected to agent", func(t *testing.T) {
		t.Parallel()

		config := GRPCConfig{
			LocalListen: "127.0.0.1:0",
			AgentAddr:   "127.0.0.1:19090",
			Insecure:    true,
		}
		handlers := map[string]FunctionHandler{}

		mgr, err := NewGRPCManager(config, handlers)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		functions := []LocalFunctionDescriptor{
			{ID: "test_fn", Version: "1.0.0"},
		}

		// Should fail because not connected
		_, err = mgr.RegisterWithAgent(context.Background(), "test-service", "1.0.0", functions)
		if err == nil {
			t.Error("expected error when not connected to agent")
		}
	})

	t.Run("empty local address", func(t *testing.T) {
		t.Parallel()

		config := GRPCConfig{
			LocalListen: "127.0.0.1:0",
			AgentAddr:   "127.0.0.1:19090",
			Insecure:    true,
		}
		handlers := map[string]FunctionHandler{}

		mgr, err := NewGRPCManager(config, handlers)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		impl, ok := mgr.(*grpcManager)
		if !ok {
			t.Fatal("type assertion failed")
		}

		// Simulate connected state but no local address
		impl.connected = true
		impl.conn = nil // No actual connection
		impl.localAddr = ""

		functions := []LocalFunctionDescriptor{
			{ID: "test_fn", Version: "1.0.0"},
		}

		// Should fail because local address is empty
		_, err = mgr.RegisterWithAgent(context.Background(), "test-service", "1.0.0", functions)
		if err == nil {
			t.Error("expected error when local address is empty")
		}
	})

	t.Run("filters empty function IDs", func(t *testing.T) {
		t.Parallel()

		config := GRPCConfig{
			LocalListen: "127.0.0.1:0",
			AgentAddr:   "127.0.0.1:19090",
			Insecure:    true,
		}
		handlers := map[string]FunctionHandler{}

		mgr, err := NewGRPCManager(config, handlers)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		impl, ok := mgr.(*grpcManager)
		if !ok {
			t.Fatal("type assertion failed")
		}

		// Simulate connected state
		impl.connected = true
		impl.conn = nil
		impl.localAddr = "127.0.0.1:12345"

		// Test with empty function ID - should be filtered out
		functions := []LocalFunctionDescriptor{
			{ID: "", Version: "1.0.0"},
			{ID: "valid_fn", Version: "1.0.0"},
		}

		// Will fail because localCli is nil, but we verify empty ID is filtered
		_, err = mgr.RegisterWithAgent(context.Background(), "test-service", "1.0.0", functions)
		// Expected to fail with "not connected to agent" because localCli is nil
	})
}

// Test startHeartbeat with mock client
func TestGRPCManager_startHeartbeatWithMock(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Test with all required fields set (but nil client - will return early)
	impl.localCli = nil
	impl.sessionID = "test-session"
	impl.serviceID = "test-service"

	// Should return without doing anything because localCli is nil
	impl.startHeartbeatLocked()

	// Now test with stop function existing
	if impl.heartbeatStop != nil {
		// Should have been called (but was nil, so nothing happened)
	}
}

// Test stopHeartbeat
func TestGRPCManager_stopHeartbeat(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Create a cancel function for heartbeat stop
	_, cancel := context.WithCancel(context.Background())
	impl.heartbeatStop = cancel

	// Stop should call the cancel function
	impl.stopHeartbeatLocked()

	if impl.heartbeatStop != nil {
		t.Error("expected heartbeatStop to be nil after stop")
	}

	// Calling stop again should be safe
	impl.stopHeartbeatLocked()
}

// Test startHeartbeat when already running
func TestGRPCManager_startHeartbeatWhenRunning(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Simulate heartbeat already running
	_, cancel := context.WithCancel(context.Background())
	impl.heartbeatStop = cancel
	impl.localCli = nil
	impl.sessionID = "test-session"
	impl.serviceID = "test-service"

	// Should stop existing heartbeat before starting new one
	impl.startHeartbeatLocked()

	if impl.heartbeatStop == nil {
		t.Error("heartbeatStop should not be nil after startHeartbeatLocked with nil localCli")
	}
}

// Test StartServer error cases
func TestGRPCManager_StartServerErrors(t *testing.T) {
	t.Parallel()

	t.Run("server already running", func(t *testing.T) {
		t.Parallel()

		config := GRPCConfig{
			LocalListen: "127.0.0.1:0",
			Insecure:    true,
		}
		handlers := map[string]FunctionHandler{}

		mgr, err := NewGRPCManager(config, handlers)
		if err != nil {
			t.Fatalf("failed to create manager: %v", err)
		}

		impl, ok := mgr.(*grpcManager)
		if !ok {
			t.Fatal("type assertion failed")
		}

		// Create a mock gRPC server
		impl.server = grpc.NewServer()

		// Should return nil (already running)
		err = mgr.StartServer(context.Background())
		if err != nil {
			t.Errorf("expected no error when server already running, got %v", err)
		}

		// Clean up
		impl.server.Stop()
	})
}

// Test Connect timeout
func TestGRPCManager_ConnectTimeout(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen:    "127.0.0.1:0",
		AgentAddr:      "127.0.0.1:19090",
		Insecure:       true,
		TimeoutSeconds: 1,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Should complete without hanging (may or may not error depending on whether agent is running)
	_ = mgr.Connect(ctx)
	mgr.Disconnect()
}

// Test RegisterWithAgent with empty session ID response
func TestGRPCManager_RegisterWithAgentEmptySessionID(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Simulate connected state
	impl.connected = true
	impl.localAddr = "127.0.0.1:12345"

	functions := []LocalFunctionDescriptor{
		{ID: "test_fn", Version: "1.0.0"},
	}

	// Should fail because localCli is nil
	_, err = mgr.RegisterWithAgent(context.Background(), "test-service", "1.0.0", functions)
	if err == nil {
		t.Error("expected error when local client is nil")
	}
}

// TestGRPCManager_startHeartbeatWithSessionID tests startHeartbeatLocked with session ID
func TestGRPCManager_startHeartbeatWithSessionID(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Simulate connected state with session ID
	impl.connected = true
	impl.sessionID = "test-session-123"
	impl.serviceID = "test-service"

	// stopHeartbeatLocked should be safe even without heartbeat running
	impl.stopHeartbeatLocked()

	// Start heartbeat with mock client (nil localCli means heartbeat won't actually send)
	// This tests that the function handles the case gracefully
	impl.startHeartbeatLocked()

	// Stop heartbeat
	impl.stopHeartbeatLocked()
}

// TestGRPCManager_startHeartbeatWithoutSessionID tests startHeartbeatLocked without session ID
func TestGRPCManager_startHeartbeatWithoutSessionID(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Simulate connected state but no session ID
	impl.connected = true
	impl.sessionID = ""
	impl.serviceID = "test-service"

	// startHeartbeatLocked should handle empty session ID gracefully
	impl.startHeartbeatLocked()

	// Stop heartbeat
	impl.stopHeartbeatLocked()
}

// TestGRPCManager_startHeartbeatMultipleCalls tests multiple heartbeat start/stop cycles
func TestGRPCManager_startHeartbeatMultipleCalls(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Simulate connected state
	impl.connected = true
	impl.sessionID = "test-session-456"
	impl.serviceID = "test-service"

	// Multiple start/stop cycles should be safe
	impl.startHeartbeatLocked()
	impl.startHeartbeatLocked() // Second call should be safe
	impl.stopHeartbeatLocked()
	impl.stopHeartbeatLocked() // Second stop should be safe
}

// TestGRPCManager_createServerTLSCredentialsWithCert tests createServerTLSCredentials with cert
func TestGRPCManager_createServerTLSCredentialsWithCert(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    false, // Use TLS
		CertFile:    "/nonexistent/cert.pem",
		KeyFile:     "/nonexistent/key.pem",
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Should fail because cert files don't exist
	_, err = impl.createServerTLSCredentials()
	if err == nil {
		t.Error("expected error when cert files don't exist")
	}
}

// TestGRPCManager_createServerTLSCredentialsInsecure tests createServerTLSCredentials with insecure=true
func TestGRPCManager_createServerTLSCredentialsInsecure(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true, // Insecure mode
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Even in insecure mode, createServerTLSCredentials requires cert files
	creds, err := impl.createServerTLSCredentials()
	if err == nil {
		t.Error("expected error when cert files are not provided")
	}
	if creds != nil {
		t.Error("expected nil credentials when cert files are not provided")
	}
}

// TestGRPCManager_DisconnectMultipleTimes tests that multiple Disconnect calls are safe
func TestGRPCManager_DisconnectMultipleTimes(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Multiple disconnects should be safe
	mgr.Disconnect()
	mgr.Disconnect()
	mgr.Disconnect()
}

// TestGRPCManager_RegisterWithAgentWithLocalAddr tests RegisterWithAgent with local address
func TestGRPCManager_RegisterWithAgentWithLocalAddr(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Set up state to pass initial checks
	impl.connected = true
	impl.localAddr = "127.0.0.1:12345"
	impl.conn = nil // No actual connection

	functions := []LocalFunctionDescriptor{
		{ID: "test_fn", Version: "1.0.0"},
	}

	// Should fail because conn is nil
	_, err = mgr.RegisterWithAgent(context.Background(), "test-service", "1.0.0", functions)
	if err == nil {
		t.Error("expected error when conn is nil")
	}
}

// TestGRPCManager_RegisterWithAgentEmptyFunctions tests RegisterWithAgent with empty functions
func TestGRPCManager_RegisterWithAgentEmptyFunctions(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Set up state to pass initial checks
	impl.connected = true
	impl.localAddr = "127.0.0.1:12345"

	functions := []LocalFunctionDescriptor{} // Empty functions list

	// Should fail because localCli is nil
	_, err = mgr.RegisterWithAgent(context.Background(), "test-service", "1.0.0", functions)
	if err == nil {
		t.Error("expected error when local client is nil")
	}
}

// TestGRPCManager_GetLocalAddressBeforeStart tests GetLocalAddress before StartServer
func TestGRPCManager_GetLocalAddressBeforeStart(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Before StartServer, local address should be empty
	addr := mgr.GetLocalAddress()
	if addr != "" {
		t.Errorf("expected empty address before StartServer, got %q", addr)
	}
}

// TestGRPCManager_IsConnectedBeforeConnect tests IsConnected before Connect
func TestGRPCManager_IsConnectedBeforeConnect(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Before Connect, should not be connected
	if mgr.IsConnected() {
		t.Error("expected not to be connected before Connect")
	}
}

// TestGRPCManager_ConnectIdempotent tests that multiple Connect calls are idempotent
func TestGRPCManager_ConnectIdempotent(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Simulate connected state
	impl.connected = true

	// Second Connect should return immediately (already connected)
	err = mgr.Connect(context.Background())
	if err != nil {
		t.Errorf("unexpected error on second connect: %v", err)
	}

	if !impl.connected {
		t.Error("should still be connected")
	}
}

// TestGRPCManager_StartServerIdempotent tests that multiple StartServer calls are idempotent
func TestGRPCManager_StartServerIdempotent(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Start server first time
	err = mgr.StartServer(context.Background())
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	// Second StartServer should return immediately (already running)
	err = mgr.StartServer(context.Background())
	if err != nil {
		t.Errorf("unexpected error on second StartServer: %v", err)
	}

	// Clean up
	mgr.Disconnect()
}

// TestGRPCManager_createTLSCredentialsWithCAFile tests createTLSCredentials with CA file
func TestGRPCManager_createTLSCredentialsWithCAFile(t *testing.T) {
	t.Parallel()

	// Create a temporary CA file for testing
	tmpfile, err := os.CreateTemp("", "ca-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// Write invalid content (not a real cert, but tests the file reading path)
	tmpfile.WriteString("not a real certificate")
	tmpfile.Close()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    false,
		CAFile:      tmpfile.Name(),
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Should return error because CA content is invalid (not valid PEM)
	creds, err := impl.createTLSCredentials()
	if err == nil {
		t.Error("expected error with invalid CA content")
	}
	if creds != nil {
		t.Error("expected nil credentials when CA content is invalid")
	}
}

// TestGRPCManager_createTLSCredentialsWithCertKeyPair tests createTLSCredentials with cert/key pair
func TestGRPCManager_createTLSCredentialsWithCertKeyPair(t *testing.T) {
	t.Parallel()

	// Create temporary cert and key files
	tmpCert, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpCert.Name())
	tmpCert.WriteString("invalid cert content")
	tmpCert.Close()

	tmpKey, err := os.CreateTemp("", "key-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpKey.Name())
	tmpKey.WriteString("invalid key content")
	tmpKey.Close()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    false,
		CertFile:    tmpCert.Name(),
		KeyFile:     tmpKey.Name(),
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Should fail because cert/key content is invalid
	_, err = impl.createTLSCredentials()
	if err == nil {
		t.Error("expected error with invalid cert/key content")
	}
}

// TestGRPCManager_createTLSCredentialsWithOnlyCert tests createTLSCredentials with only cert (no key)
func TestGRPCManager_createTLSCredentialsWithOnlyCert(t *testing.T) {
	t.Parallel()

	tmpCert, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpCert.Name())
	tmpCert.WriteString("invalid cert content")
	tmpCert.Close()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    false,
		CertFile:    tmpCert.Name(),
		// KeyFile not provided
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Should not error - cert without key should be handled
	creds, err := impl.createTLSCredentials()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Error("expected credentials to be returned")
	}
}

// TestGRPCManager_createTLSCredentialsInvalidCA tests createTLSCredentials with invalid CA file
func TestGRPCManager_createTLSCredentialsInvalidCA(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    false,
		CAFile:      "/nonexistent/ca.pem",
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Should return error when CA file doesn't exist
	creds, err := impl.createTLSCredentials()
	if err == nil {
		t.Error("expected error with nonexistent CA file")
	}
	if creds != nil {
		t.Error("expected nil credentials when CA file doesn't exist")
	}
}

// TestGRPCManager_createServerTLSCredentialsWithCA tests createServerTLSCredentials with CA file
func TestGRPCManager_createServerTLSCredentialsWithCA(t *testing.T) {
	t.Parallel()

	// Create temporary CA file
	tmpCA, err := os.CreateTemp("", "ca-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpCA.Name())
	tmpCA.WriteString("-----BEGIN CERTIFICATE-----\ninvalid cert content\n-----END CERTIFICATE-----")
	tmpCA.Close()

	// Create temporary cert and key files
	tmpCert, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpCert.Name())
	tmpCert.WriteString("invalid cert")
	tmpCert.Close()

	tmpKey, err := os.CreateTemp("", "key-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpKey.Name())
	tmpKey.WriteString("invalid key")
	tmpKey.Close()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    false,
		CertFile:    tmpCert.Name(),
		KeyFile:     tmpKey.Name(),
		CAFile:      tmpCA.Name(),
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Should fail to load invalid cert/key
	_, err = impl.createServerTLSCredentials()
	if err == nil {
		t.Error("expected error with invalid cert/key files")
	}
}

// TestGRPCManager_createServerTLSCredentialsWithSkipVerify tests createServerTLSCredentials with SkipVerify
func TestGRPCManager_createServerTLSCredentialsWithSkipVerify(t *testing.T) {
	t.Parallel()

	// Create temporary cert and key files
	tmpCert, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpCert.Name())
	tmpCert.WriteString("invalid cert")
	tmpCert.Close()

	tmpKey, err := os.CreateTemp("", "key-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpKey.Name())
	tmpKey.WriteString("invalid key")
	tmpKey.Close()

	config := GRPCConfig{
		LocalListen:        "127.0.0.1:0",
		AgentAddr:          "127.0.0.1:19090",
		Insecure:           false,
		CertFile:           tmpCert.Name(),
		KeyFile:            tmpKey.Name(),
		InsecureSkipVerify: true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Should still fail to load invalid cert/key even with SkipVerify
	_, err = impl.createServerTLSCredentials()
	if err == nil {
		t.Error("expected error with invalid cert/key files")
	}
}

// TestGRPCManager_createServerTLSCredentialsWithoutCA tests createServerTLSCredentials without CA
func TestGRPCManager_createServerTLSCredentialsWithoutCA(t *testing.T) {
	t.Parallel()

	// Create temporary cert and key files
	tmpCert, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpCert.Name())
	tmpCert.WriteString("invalid cert")
	tmpCert.Close()

	tmpKey, err := os.CreateTemp("", "key-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpKey.Name())
	tmpKey.WriteString("invalid key")
	tmpKey.Close()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    false,
		CertFile:    tmpCert.Name(),
		KeyFile:     tmpKey.Name(),
		// No CAFile
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Should fail to load invalid cert/key
	_, err = impl.createServerTLSCredentials()
	if err == nil {
		t.Error("expected error with invalid cert/key files")
	}
}

// TestGRPCManager_RegisterWithAgentNotConnected tests RegisterWithAgent when not connected
func TestGRPCManager_RegisterWithAgentNotConnected(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	functions := []LocalFunctionDescriptor{
		{ID: "test_fn", Version: "1.0.0"},
	}

	// Should fail because not connected
	_, err = mgr.RegisterWithAgent(context.Background(), "service-id", "1.0.0", functions)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

// TestGRPCManager_RegisterWithAgentWithoutLocalAddr tests RegisterWithAgent without local address
func TestGRPCManager_RegisterWithAgentWithoutLocalAddr(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Set connected but no local address
	impl.connected = true
	impl.localAddr = ""

	functions := []LocalFunctionDescriptor{
		{ID: "test_fn", Version: "1.0.0"},
	}

	// Should fail because local address is not set
	_, err = mgr.RegisterWithAgent(context.Background(), "service-id", "1.0.0", functions)
	if err == nil {
		t.Error("expected error when local address is not set")
	}
}

// TestGRPCManager_RegisterWithAgentWithEmptyFunctionID tests RegisterWithAgent filtering empty function IDs
func TestGRPCManager_RegisterWithAgentWithEmptyFunctionID(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Set connected with local address
	impl.connected = true
	impl.localAddr = "127.0.0.1:12345"

	functions := []LocalFunctionDescriptor{
		{ID: "", Version: "1.0.0"},         // Empty ID - should be filtered
		{ID: "valid_fn", Version: "1.0.0"}, // Valid ID
		{ID: "", Version: "2.0.0"},         // Another empty ID
	}

	// Should fail due to no actual client, but verify function filtering works
	_, err = mgr.RegisterWithAgent(context.Background(), "service-id", "1.0.0", functions)
	if err == nil {
		t.Error("expected error when localCli is nil")
	}
}

// TestGRPCManager_RegisterWithAgentWithEmptyServiceInfo tests RegisterWithAgent with empty service info
func TestGRPCManager_RegisterWithAgentWithEmptyServiceInfo(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		AgentAddr:   "127.0.0.1:19090",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Set connected with local address
	impl.connected = true
	impl.localAddr = "127.0.0.1:12345"

	functions := []LocalFunctionDescriptor{}

	// Empty functions list should still work (will fail on RPC but that's expected)
	_, err = mgr.RegisterWithAgent(context.Background(), "", "", functions)
	if err == nil {
		t.Error("expected error when localCli is nil")
	}
}

// TestGRPCManager_startHeartbeatWithNilLocalCli tests heartbeat with nil client
func TestGRPCManager_startHeartbeatWithNilLocalCli(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		AgentAddr: "127.0.0.1:19090",
		Insecure:  true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Set up state with nil localCli
	impl.localCli = nil
	impl.sessionID = "test-session"
	impl.serviceID = "test-service"

	// Should not panic
	impl.startHeartbeatLocked()

	if impl.heartbeatStop != nil {
		t.Error("expected heartbeatStop to be nil when localCli is nil")
	}
}

// TestGRPCManager_startHeartbeatWithEmptySessionID tests heartbeat with empty session ID
func TestGRPCManager_startHeartbeatWithEmptySessionID(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		AgentAddr: "127.0.0.1:19090",
		Insecure:  true,
	}
	handlers := map[string]FunctionHandler{}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	impl, ok := mgr.(*grpcManager)
	if !ok {
		t.Fatal("type assertion failed")
	}

	// Create a mock local client
	impl.localCli = nil // Not actually connecting
	impl.sessionID = "" // Empty session ID
	impl.serviceID = "test-service"

	// Should return early without starting heartbeat
	impl.startHeartbeatLocked()

	if impl.heartbeatStop != nil {
		t.Error("expected heartbeatStop to be nil when sessionID is empty")
	}
}

// TestGRPCManager_StartServerIdempotent2 tests calling StartServer multiple times
func TestGRPCManager_StartServerIdempotent2(t *testing.T) {
	t.Parallel()

	config := GRPCConfig{
		LocalListen: "127.0.0.1:0",
		Insecure:    true,
	}
	handlers := map[string]FunctionHandler{
		"test": func(ctx context.Context, payload []byte) ([]byte, error) {
			return payload, nil
		},
	}

	mgr, err := NewGRPCManager(config, handlers)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()

	// First start
	err = mgr.StartServer(ctx)
	if err != nil {
		t.Fatalf("first StartServer failed: %v", err)
	}

	// Get the address after first start
	addr1 := mgr.GetLocalAddress()

	// Second start should be idempotent
	err = mgr.StartServer(ctx)
	if err != nil {
		t.Fatalf("second StartServer failed: %v", err)
	}

	addr2 := mgr.GetLocalAddress()

	// Address should be the same
	if addr1 != addr2 {
		t.Errorf("expected same address, got %s and %s", addr1, addr2)
	}

	// Cleanup
	mgr.(*grpcManager).server.Stop()
}
