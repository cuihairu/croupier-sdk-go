package croupier

import (
	"context"
	"testing"
	"time"
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
		time.Sleep(10 * time.Millisecond)
		mgr.Disconnect()
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
		time.Sleep(10 * time.Millisecond)
		mgr.Disconnect()
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

