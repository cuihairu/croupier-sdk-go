package croupier

import (
	"context"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	t.Parallel()

	config := ManagerConfig{
		AgentAddr:    "localhost:19090",
		ControlAddr:  "localhost:19100",
		LocalListen:  "127.0.0.1:0",
		ProviderLang: "golang",
		ProviderSDK:  "custom-go-sdk",
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
		ServiceID:         "test-service",
		ServiceVersion:    "1.0.0",
		AgentAddr:         "localhost:19090",
		ControlAddr:       "localhost:19100",
		HeartbeatInterval: 15,
		ProviderLang:      "golang",
		ProviderSDK:       "custom-go-sdk",
		Insecure:          true,
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

	nngMgr, ok := mgr.(*NNGManager)
	if !ok {
		t.Fatal("NewNNGManager did not return *NNGManager")
	}
	if nngMgr.config.HeartbeatInterval != 15 {
		t.Fatalf("expected HeartbeatInterval 15, got %d", nngMgr.config.HeartbeatInterval)
	}
	if nngMgr.config.ControlAddr != "localhost:19100" {
		t.Fatalf("expected ControlAddr localhost:19100, got %q", nngMgr.config.ControlAddr)
	}
	if nngMgr.config.ProviderLang != "golang" {
		t.Fatalf("expected ProviderLang golang, got %q", nngMgr.config.ProviderLang)
	}
	if nngMgr.config.ProviderSDK != "custom-go-sdk" {
		t.Fatalf("expected ProviderSDK custom-go-sdk, got %q", nngMgr.config.ProviderSDK)
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
	if config.HeartbeatInterval != 0 {
		t.Errorf("expected default HeartbeatInterval 0, got %d", config.HeartbeatInterval)
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

func TestNNGManager_CapabilitiesAddress(t *testing.T) {
	t.Parallel()

	manager := &NNGManager{
		config: nngManagerConfig{
			AgentAddr:   "localhost:19090",
			ControlAddr: "localhost:19100",
		},
	}

	if got := manager.capabilitiesAddress(); got != "localhost:19100" {
		t.Fatalf("expected control address, got %q", got)
	}

	manager.config.ControlAddr = ""
	if got := manager.capabilitiesAddress(); got != "localhost:19090" {
		t.Fatalf("expected agent address fallback, got %q", got)
	}
}

func TestNNGManager_BuildProviderMeta(t *testing.T) {
	t.Parallel()

	manager := &NNGManager{
		config: nngManagerConfig{
			ProviderLang: "golang",
			ProviderSDK:  "custom-go-sdk",
		},
		serviceID:      "svc-1",
		serviceVersion: "2.0.0",
	}

	provider := manager.buildProviderMeta()
	if provider.GetId() != "svc-1" {
		t.Fatalf("expected provider id svc-1, got %q", provider.GetId())
	}
	if provider.GetVersion() != "2.0.0" {
		t.Fatalf("expected provider version 2.0.0, got %q", provider.GetVersion())
	}
	if provider.GetLang() != "golang" {
		t.Fatalf("expected provider lang golang, got %q", provider.GetLang())
	}
	if provider.GetSdk() != "custom-go-sdk" {
		t.Fatalf("expected provider sdk custom-go-sdk, got %q", provider.GetSdk())
	}
}

func TestNNGManager_BuildProviderMeta_Defaults(t *testing.T) {
	t.Parallel()

	manager := &NNGManager{
		config:         nngManagerConfig{},
		serviceID:      "svc-1",
		serviceVersion: "1.0.0",
	}

	provider := manager.buildProviderMeta()
	if provider.GetLang() != "go" {
		t.Fatalf("expected default provider lang go, got %q", provider.GetLang())
	}
	if provider.GetSdk() != "croupier-go-sdk" {
		t.Fatalf("expected default provider sdk croupier-go-sdk, got %q", provider.GetSdk())
	}
}
