package croupier

import (
	"context"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	t.Parallel()

	config := ManagerConfig{
		AgentAddr:   "localhost:19090",
		LocalListen: "127.0.0.1:0",
	}

	handlers := map[string]FunctionHandler{
		"test.fn": func(ctx context.Context, payload []byte) ([]byte, error) {
			return payload, nil
		},
	}

	mgr, err := NewManager(config, handlers)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
}

func TestManager_GetDispatcher(t *testing.T) {
	t.Parallel()

	dispatcher := GetDispatcher()
	if dispatcher == nil {
		t.Error("GetDispatcher returned nil")
	}
}

func TestNewNNGManager(t *testing.T) {
	t.Parallel()

	config := ClientConfig{
		ServiceID:      "test-service",
		ServiceVersion: "1.0.0",
		AgentAddr:      "localhost:19090",
		Insecure:       true,
	}

	handlers := map[string]FunctionHandler{
		"test.fn": func(ctx context.Context, payload []byte) ([]byte, error) {
			return payload, nil
		},
	}

	mgr, err := NewNNGManager(config, handlers)
	if err != nil {
		t.Fatalf("NewNNGManager returned error: %v", err)
	}
	if mgr == nil {
		t.Fatal("NewNNGManager returned nil")
	}
}

func TestNewNNGManager_EmptyHandlers(t *testing.T) {
	t.Parallel()

	config := ClientConfig{
		ServiceID:      "test-service",
		ServiceVersion: "1.0.0",
		AgentAddr:      "localhost:19090",
		Insecure:       true,
	}

	// Empty handlers should be valid
	mgr, err := NewNNGManager(config, nil)
	if err != nil {
		t.Fatalf("NewNNGManager with nil handlers returned error: %v", err)
	}
	if mgr == nil {
		t.Fatal("NewNNGManager returned nil")
	}
}

func TestManager_Disconnect_NotStarted(t *testing.T) {
	t.Parallel()

	config := ClientConfig{
		ServiceID: "test-service",
	}

	handlers := map[string]FunctionHandler{
		"test.fn": func(ctx context.Context, payload []byte) ([]byte, error) {
			return payload, nil
		},
	}

	mgr, _ := NewNNGManager(config, handlers)

	// Should not panic when disconnecting without starting
	mgr.Disconnect()
}

func TestClient_Serve_ContextCancellation(t *testing.T) {
	t.Parallel()

	cli := NewClient(&ClientConfig{
		ServiceID:      "test-service",
		ServiceVersion: "1.0.0",
	})

	handler := func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	}
	cli.RegisterFunction(FunctionDescriptor{ID: "fn1", Version: "1.0.0"}, handler)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Serve should return when context is cancelled
	cli.Serve(ctx)
}

func TestManagerConfig_Defaults(t *testing.T) {
	t.Parallel()

	config := ManagerConfig{}

	// Verify default values
	if config.TimeoutSeconds != 0 {
		t.Errorf("expected default TimeoutSeconds 0, got %d", config.TimeoutSeconds)
	}
}

func TestClientConfig_DefaultValues(t *testing.T) {
	t.Parallel()

	config := DefaultClientConfig()

	if config.ServiceID == "" {
		t.Error("expected ServiceID to be set")
	}

	if config.ServiceVersion == "" {
		t.Error("expected ServiceVersion to be set")
	}

	if config.AgentAddr == "" {
		t.Error("expected AgentAddr to be set")
	}
}
