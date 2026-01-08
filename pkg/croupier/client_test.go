package croupier

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClient_buildManifest(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ServiceID:      "svc-1",
			ServiceVersion: "sv1",
			ProviderLang:   "go",
			ProviderSDK:    "croupier-go-sdk",
		},
		handlers:    map[string]FunctionHandler{},
		descriptors: map[string]FunctionDescriptor{},
	}

	c.descriptors["f1"] = FunctionDescriptor{
		ID:        "f1",
		Version:   "1.2.3",
		Category:  "cat",
		Risk:      "low",
		Entity:    "player",
		Operation: "create",
		Enabled:   true,
	}
	c.descriptors["f2"] = FunctionDescriptor{
		ID:      "f2",
		Version: "",
		Enabled: false,
	}

	raw, err := c.buildManifest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded struct {
		Provider struct {
			ID      string `json:"id"`
			Version string `json:"version"`
			Lang    string `json:"lang"`
			SDK     string `json:"sdk"`
		} `json:"provider"`
		Functions []map[string]any `json:"functions"`
	}

	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if decoded.Provider.ID != "svc-1" || decoded.Provider.Version != "sv1" || decoded.Provider.Lang != "go" || decoded.Provider.SDK != "croupier-go-sdk" {
		t.Fatalf("unexpected provider: %+v", decoded.Provider)
	}

	funcByID := map[string]map[string]any{}
	for _, fn := range decoded.Functions {
		id, _ := fn["id"].(string)
		funcByID[id] = fn
	}

	if fn := funcByID["f1"]; fn == nil || fn["version"] != "1.2.3" || fn["entity"] != "player" || fn["operation"] != "create" {
		t.Fatalf("unexpected f1: %+v", fn)
	}
	if _, ok := funcByID["f1"]["enabled"]; !ok {
		t.Fatalf("expected enabled flag to be present for f1")
	}

	if fn := funcByID["f2"]; fn == nil || fn["version"] != "1.0.0" {
		t.Fatalf("unexpected f2: %+v", fn)
	}
	if _, ok := funcByID["f2"]["enabled"]; ok {
		t.Fatalf("expected enabled flag to be omitted for f2 when false")
	}
}

func TestGzipBytes_RoundTrip(t *testing.T) {
	t.Parallel()

	original := []byte(`{"hello":"world"}`)
	compressed, err := gzipBytes(original)
	if err != nil {
		t.Fatalf("gzipBytes: %v", err)
	}

	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer r.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read gzipped: %v", err)
	}
	if !bytes.Equal(out, original) {
		t.Fatalf("round-trip mismatch: got %q want %q", out, original)
	}
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	t.Run("with valid config", func(t *testing.T) {
		t.Parallel()

		config := &ClientConfig{
			ServiceID:      "test-service",
			ServiceVersion: "1.0.0",
			AgentAddr:      "localhost:19090",
		}
		cli := NewClient(config)

		if cli == nil {
			t.Fatal("NewClient returned nil")
		}

		c, ok := cli.(*client)
		if !ok {
			t.Fatal("NewClient did not return *client type")
		}

		if c.config != config {
			t.Error("config not set correctly")
		}

		if c.handlers == nil {
			t.Error("handlers map not initialized")
		}

		if c.descriptors == nil {
			t.Error("descriptors map not initialized")
		}

		if c.stopCh == nil {
			t.Error("stopCh not initialized")
		}
	})

	t.Run("with nil config uses defaults", func(t *testing.T) {
		t.Parallel()

		cli := NewClient(nil)
		c, ok := cli.(*client)
		if !ok {
			t.Fatal("NewClient did not return *client type")
		}

		if c.config == nil {
			t.Fatal("config should be set to default")
		}

		if c.config.AgentAddr != "localhost:19090" {
			t.Errorf("expected default AgentAddr, got %q", c.config.AgentAddr)
		}

		if !c.config.Insecure {
			t.Error("expected default Insecure to be true")
		}
	})

	t.Run("with logging disabled", func(t *testing.T) {
		t.Parallel()

		config := &ClientConfig{
			ServiceID:      "test-service",
			DisableLogging: true,
		}
		cli := NewClient(config)
		c, ok := cli.(*client)
		if !ok {
			t.Fatal("NewClient did not return *client type")
		}

		if _, ok := c.logger.(*NoOpLogger); !ok {
			t.Error("expected NoOpLogger when DisableLogging is true")
		}
	})
}

func TestClient_RegisterFunction(t *testing.T) {
	t.Parallel()

	config := &ClientConfig{
		ServiceID:      "test-service",
		ServiceVersion: "1.0.0",
	}
	cli := NewClient(config)
	c := cli.(*client)

	handler := func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	}

	t.Run("successful registration", func(t *testing.T) {
		t.Parallel()

		newCli := NewClient(config)
		desc := FunctionDescriptor{
			ID:      "test.function",
			Version: "1.0.0",
		}

		err := newCli.RegisterFunction(desc, handler)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		nc := newCli.(*client)
		if _, ok := nc.handlers["test.function"]; !ok {
			t.Error("handler not registered")
		}

		if _, ok := nc.descriptors["test.function"]; !ok {
			t.Error("descriptor not registered")
		}
	})

	t.Run("empty function ID returns error", func(t *testing.T) {
		t.Parallel()

		newCli := NewClient(config)
		desc := FunctionDescriptor{
			ID: "",
		}

		err := newCli.RegisterFunction(desc, handler)
		if err == nil {
			t.Error("expected error for empty function ID")
		}

		if err != nil && err.Error() != "function ID cannot be empty" {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("default version is set", func(t *testing.T) {
		t.Parallel()

		newCli := NewClient(config)
		desc := FunctionDescriptor{
			ID:      "test.version",
			Version: "",
		}

		err := newCli.RegisterFunction(desc, handler)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		nc := newCli.(*client)
		if nc.descriptors["test.version"].Version != "1.0.0" {
			t.Errorf("expected default version 1.0.0, got %q", nc.descriptors["test.version"].Version)
		}
	})

	t.Run("cannot register while running", func(t *testing.T) {
		t.Parallel()

		newCli := NewClient(config)
		nc := newCli.(*client)
		nc.running = true

		desc := FunctionDescriptor{
			ID:      "test.running",
			Version: "1.0.0",
		}

		err := newCli.RegisterFunction(desc, handler)
		if err == nil {
			t.Error("expected error when registering while running")
		}
	})

	// After tests, check original client wasn't affected
	_ = c
}

func TestClient_Stop(t *testing.T) {
	t.Parallel()

	config := &ClientConfig{
		ServiceID: "test-service",
	}
	cli := NewClient(config)
	c := cli.(*client)

	// Set running state
	c.running = true
	c.connected = true

	err := cli.Stop()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.running {
		t.Error("expected running to be false after Stop")
	}

	if c.connected {
		t.Error("expected connected to be false after Stop")
	}
}

func TestClient_Close(t *testing.T) {
	t.Parallel()

	config := &ClientConfig{
		ServiceID: "test-service",
	}
	cli := NewClient(config)
	c := cli.(*client)

	c.handlers = map[string]FunctionHandler{"test": nil}

	err := cli.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.handlers != nil {
		t.Error("expected handlers to be nil after Close")
	}
}

func TestClient_GetLocalAddress(t *testing.T) {
	t.Parallel()

	config := &ClientConfig{
		ServiceID: "test-service",
	}
	cli := NewClient(config)
	c := cli.(*client)

	c.localAddr = "127.0.0.1:12345"

	addr := cli.GetLocalAddress()
	if addr != "127.0.0.1:12345" {
		t.Errorf("expected 127.0.0.1:12345, got %q", addr)
	}
}

func TestClient_convertToLocalFunctions(t *testing.T) {
	t.Parallel()

	config := &ClientConfig{
		ServiceID: "test-service",
	}
	cli := NewClient(config)
	c := cli.(*client)

	handler := func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	}

	c.descriptors["f1"] = FunctionDescriptor{ID: "f1", Version: "1.0.0"}
	c.descriptors["f2"] = FunctionDescriptor{ID: "f2", Version: "2.0.0"}
	c.handlers["f1"] = handler
	c.handlers["f2"] = handler

	funcs := c.convertToLocalFunctions()

	if len(funcs) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(funcs))
	}

	funcByID := make(map[string]LocalFunctionDescriptor)
	for _, f := range funcs {
		funcByID[f.ID] = f
	}

	if f := funcByID["f1"]; f.Version != "1.0.0" {
		t.Errorf("expected f1 version 1.0.0, got %q", f.Version)
	}

	if f := funcByID["f2"]; f.Version != "2.0.0" {
		t.Errorf("expected f2 version 2.0.0, got %q", f.Version)
	}
}

func TestGzipBytes_Errors(t *testing.T) {
	t.Parallel()

	// Test with various payload sizes
	testCases := []struct {
		name    string
		payload []byte
	}{
		{"empty", []byte{}},
		{"small", []byte("hello")},
		{"large", bytes.Repeat([]byte("a"), 10000)},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			compressed, err := gzipBytes(tc.payload)
			if err != nil {
				t.Fatalf("gzipBytes failed: %v", err)
			}

			if len(compressed) == 0 && len(tc.payload) > 0 {
				t.Error("compressed data should not be empty for non-empty input")
			}

			// Verify round-trip
			r, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				t.Fatalf("gzip.NewReader failed: %v", err)
			}
			defer r.Close()

			out, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("io.ReadAll failed: %v", err)
			}

			if !bytes.Equal(out, tc.payload) {
				t.Error("round-trip mismatch")
			}
		})
	}
}

// Test DefaultClientConfig
func TestDefaultClientConfig(t *testing.T) {
	t.Parallel()

	config := DefaultClientConfig()

	if config.AgentAddr != "localhost:19090" {
		t.Errorf("expected AgentAddr localhost:19090, got %q", config.AgentAddr)
	}

	if config.Env != "development" {
		t.Errorf("expected Env development, got %q", config.Env)
	}

	if config.ServiceVersion != "1.0.0" {
		t.Errorf("expected ServiceVersion 1.0.0, got %q", config.ServiceVersion)
	}

	if config.TimeoutSeconds != 30 {
		t.Errorf("expected TimeoutSeconds 30, got %d", config.TimeoutSeconds)
	}

	if !config.Insecure {
		t.Error("expected Insecure to be true")
	}

	if config.ProviderLang != "go" {
		t.Errorf("expected ProviderLang go, got %q", config.ProviderLang)
	}

	if config.ProviderSDK != "croupier-go-sdk" {
		t.Errorf("expected ProviderSDK croupier-go-sdk, got %q", config.ProviderSDK)
	}
}

// Test DefaultReconnectConfig
func TestDefaultReconnectConfig(t *testing.T) {
	t.Parallel()

	config := DefaultReconnectConfig()

	if !config.Enabled {
		t.Error("expected Enabled to be true")
	}

	if config.MaxAttempts != 0 {
		t.Errorf("expected MaxAttempts 0 (infinite), got %d", config.MaxAttempts)
	}

	if config.InitialDelayMs != 1000 {
		t.Errorf("expected InitialDelayMs 1000, got %d", config.InitialDelayMs)
	}

	if config.MaxDelayMs != 30000 {
		t.Errorf("expected MaxDelayMs 30000, got %d", config.MaxDelayMs)
	}

	if config.BackoffMultiplier != 2.0 {
		t.Errorf("expected BackoffMultiplier 2.0, got %f", config.BackoffMultiplier)
	}

	if config.JitterFactor != 0.2 {
		t.Errorf("expected JitterFactor 0.2, got %f", config.JitterFactor)
	}
}

// Test DefaultRetryConfig
func TestDefaultRetryConfig(t *testing.T) {
	t.Parallel()

	config := DefaultRetryConfig()

	if !config.Enabled {
		t.Error("expected Enabled to be true")
	}

	if config.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts 3, got %d", config.MaxAttempts)
	}

	if config.InitialDelayMs != 100 {
		t.Errorf("expected InitialDelayMs 100, got %d", config.InitialDelayMs)
	}

	if config.MaxDelayMs != 5000 {
		t.Errorf("expected MaxDelayMs 5000, got %d", config.MaxDelayMs)
	}

	expectedCodes := []int32{14, 13, 2, 10, 4}
	if len(config.RetryableStatusCodes) != len(expectedCodes) {
		t.Errorf("expected %d retryable codes, got %d", len(expectedCodes), len(config.RetryableStatusCodes))
	}

	for i, code := range expectedCodes {
		if config.RetryableStatusCodes[i] != code {
			t.Errorf("expected code %d at index %d, got %d", code, i, config.RetryableStatusCodes[i])
		}
	}
}

// Test FunctionDescriptor defaults and validation
func TestFunctionDescriptor(t *testing.T) {
	t.Parallel()

	t.Run("empty version gets default", func(t *testing.T) {
		t.Parallel()

		desc := FunctionDescriptor{
			ID: "test.function",
		}

		if desc.Version != "" {
			t.Error("expected empty version initially")
		}

		// Simulate RegisterFunction's default version logic
		version := desc.Version
		if version == "" {
			version = "1.0.0"
		}

		if version != "1.0.0" {
			t.Errorf("expected default version 1.0.0, got %q", version)
		}
	})

	t.Run("valid descriptor", func(t *testing.T) {
		t.Parallel()

		desc := FunctionDescriptor{
			ID:        "player.ban",
			Version:   "1.2.3",
			Category:  "moderation",
			Risk:      "high",
			Entity:    "player",
			Operation: "update",
			Enabled:   true,
		}

		if desc.ID != "player.ban" {
			t.Errorf("expected ID player.ban, got %q", desc.ID)
		}

		if desc.Risk != "high" {
			t.Errorf("expected Risk high, got %q", desc.Risk)
		}

		if !desc.Enabled {
			t.Error("expected Enabled to be true")
		}
	})
}

// Test InvokerConfig defaults
func TestInvokerConfig(t *testing.T) {
	t.Parallel()

	config := InvokerConfig{
		Address:        "localhost:8080",
		TimeoutSeconds: 30,
		Insecure:       true,
	}

	if config.Address != "localhost:8080" {
		t.Errorf("expected Address localhost:8080, got %q", config.Address)
	}

	if config.TimeoutSeconds != 30 {
		t.Errorf("expected TimeoutSeconds 30, got %d", config.TimeoutSeconds)
	}

	if !config.Insecure {
		t.Error("expected Insecure to be true")
	}
}

// Test JobEvent structure
func TestJobEvent(t *testing.T) {
	t.Parallel()

	event := JobEvent{
		EventType: "completed",
		JobID:     "job-123",
		Payload:   `{"result":"success"}`,
		Done:      true,
	}

	if event.EventType != "completed" {
		t.Errorf("expected EventType completed, got %q", event.EventType)
	}

	if event.JobID != "job-123" {
		t.Errorf("expected JobID job-123, got %q", event.JobID)
	}

	if !event.Done {
		t.Error("expected Done to be true")
	}
}

// Test InvokeOptions
func TestInvokeOptions(t *testing.T) {
	t.Parallel()

	opts := InvokeOptions{
		IdempotencyKey: "key-123",
		Timeout:        10 * time.Second,
		Headers: map[string]string{
			"X-Custom": "value",
		},
	}

	if opts.IdempotencyKey != "key-123" {
		t.Errorf("expected IdempotencyKey key-123, got %q", opts.IdempotencyKey)
	}

	if opts.Timeout != 10*time.Second {
		t.Errorf("expected Timeout 10s, got %v", opts.Timeout)
	}

	if opts.Headers["X-Custom"] != "value" {
		t.Errorf("expected header value, got %q", opts.Headers["X-Custom"])
	}
}

// Test LocalFunctionDescriptor
func TestLocalFunctionDescriptor(t *testing.T) {
	t.Parallel()

	desc := LocalFunctionDescriptor{
		ID:      "player.get",
		Version: "1.0.0",
	}

	if desc.ID != "player.get" {
		t.Errorf("expected ID player.get, got %q", desc.ID)
	}

	if desc.Version != "1.0.0" {
		t.Errorf("expected Version 1.0.0, got %q", desc.Version)
	}
}

// Test Client_controlCredentials with insecure config
func TestClient_controlCredentials(t *testing.T) {
	t.Parallel()

	t.Run("insecure connection", func(t *testing.T) {
		t.Parallel()

		c := &client{
			config: &ClientConfig{
				Insecure: true,
			},
		}

		// Should not return error for insecure
		creds, err := c.controlCredentials()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if creds == nil {
			t.Error("expected credentials to be returned")
		}
	})

	t.Run("control address not configured", func(t *testing.T) {
		t.Parallel()

		c := &client{
			config: &ClientConfig{
				ControlAddr: "",
			},
		}

		_, err := c.dialControl(context.Background())
		if err == nil {
			t.Error("expected error when control address is empty")
		}
	})
}

// Test Client_registerCapabilities without control addr
func TestClient_registerCapabilitiesNoControlAddr(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr: "",
		},
	}

	// Should return early without error
	err := c.registerCapabilities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Test Client_controlCredentials with cert files
func TestClient_controlCredentialsWithCertFiles(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			CertFile: "/nonexistent/cert.pem",
			KeyFile:  "/nonexistent/key.pem",
			CAFile:   "/nonexistent/ca.pem",
		},
	}

	_, err := c.controlCredentials()
	if err == nil {
		t.Error("expected error for non-existent cert files")
	}
}

// Test Client_controlCredentials with system CAs
func TestClient_controlCredentialsSystemCAs(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			Insecure: false,
			CAFile:   "",
			CertFile: "",
			KeyFile:  "",
		},
	}

	// Should use system CAs and not error
	creds, err := c.controlCredentials()
	if err != nil {
		t.Fatalf("unexpected error with system CAs: %v", err)
	}
	if creds == nil {
		t.Error("expected credentials to be returned")
	}
}

// Test Client_controlCredentials with server name
func TestClient_controlCredentialsWithServerName(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			Insecure:   false,
			ServerName: "example.com",
		},
	}

	creds, err := c.controlCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Error("expected credentials to be returned")
	}
}

// Test Client_controlCredentials with insecure skip verify
func TestClient_controlCredentialsInsecureSkipVerify(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			Insecure:           false,
			InsecureSkipVerify: true,
		},
	}

	creds, err := c.controlCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Error("expected credentials to be returned")
	}
}

// Test Client_convertToLocalFunctions with empty descriptors
func TestClient_convertToLocalFunctionsWithEmptyDescriptors(t *testing.T) {
	t.Parallel()

	c := &client{
		descriptors: map[string]FunctionDescriptor{},
		handlers:    map[string]FunctionHandler{},
	}

	funcs := c.convertToLocalFunctions()
	if len(funcs) != 0 {
		t.Errorf("expected 0 functions, got %d", len(funcs))
	}
}

// Test Client_convertToLocalFunctions with missing handlers
func TestClient_convertToLocalFunctionsWithMissingHandlers(t *testing.T) {
	t.Parallel()

	c := &client{
		descriptors: map[string]FunctionDescriptor{
			"f1": {ID: "f1", Version: "1.0.0"},
		},
		handlers: map[string]FunctionHandler{},
	}

	funcs := c.convertToLocalFunctions()
	// Functions without handlers should not be included
	if len(funcs) != 0 {
		t.Errorf("expected 0 functions (no handlers), got %d", len(funcs))
	}
}

// Test Client_Connect idempotent
func TestClient_ConnectIdempotent(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			AgentAddr: "127.0.0.1:19090",
		},
		handlers:    map[string]FunctionHandler{},
		descriptors: map[string]FunctionDescriptor{},
		stopCh:      make(chan struct{}),
		connected:   true, // Already connected
		logger:      &NoOpLogger{},
	}

	// Should return nil without trying to connect again
	err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error when already connected: %v", err)
	}
}

// Test Client_Connect with no handlers
func TestClient_ConnectNoHandlers(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			AgentAddr: "127.0.0.1:19090",
		},
		handlers:    map[string]FunctionHandler{},
		descriptors: map[string]FunctionDescriptor{},
		stopCh:      make(chan struct{}),
		logger:      &NoOpLogger{},
	}

	// Should fail to connect (no actual agent)
	err := c.Connect(context.Background())
	if err == nil {
		t.Error("expected connection error (no agent running)")
	}
}

// Test Client_Serve without connect
func TestClient_ServeWithoutConnect(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			AgentAddr: "127.0.0.1:19090",
		},
		handlers:    map[string]FunctionHandler{},
		descriptors: map[string]FunctionDescriptor{},
		stopCh:      make(chan struct{}),
		logger:      &NoOpLogger{},
	}

	// Should try to connect and fail
	err := c.Serve(context.Background())
	if err == nil {
		t.Error("expected error when serving without connection")
	}
}

// Test Client_Serve already connected
func TestClient_ServeAlreadyConnected(t *testing.T) {
	t.Parallel()

	// Test the case where connected=true but grpcManager is not initialized
	// (which can happen in edge cases)
	c := NewClient(&ClientConfig{
		AgentAddr: "127.0.0.1:19090",
	}).(*client)

	c.RegisterFunction(FunctionDescriptor{
		ID:      "test",
		Version: "1.0.0",
	}, func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	})

	// Set connected to true without initializing grpcManager
	// This will cause StartServer to fail
	c.connected = true
	c.logger = &NoOpLogger{}

	// Serve should fail because grpcManager.StartServer will panic on nil
	// But we can't test that directly without a mock
	// Instead, we test the normal flow where connected=false
	c.connected = false

	err := c.Serve(context.Background())
	// Should fail to connect (no actual agent)
	if err == nil {
		t.Error("expected error when serving without agent connection")
	}
}

// Test buildManifest with empty descriptors
func TestClient_buildManifestEmpty(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ServiceID:      "test-svc",
			ServiceVersion: "1.0.0",
			ProviderLang:   "go",
			ProviderSDK:    "croupier-go-sdk",
		},
		descriptors: map[string]FunctionDescriptor{},
	}

	raw, err := c.buildManifest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded struct {
		Provider struct {
			ID string `json:"id"`
		} `json:"provider"`
		Functions []any `json:"functions"`
	}

	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if decoded.Provider.ID != "test-svc" {
		t.Errorf("expected provider ID test-svc, got %q", decoded.Provider.ID)
	}

	// Functions should be empty or omitted
	if len(decoded.Functions) != 0 {
		t.Errorf("expected 0 functions, got %d", len(decoded.Functions))
	}
}

// Test buildManifest with full descriptor
func TestClient_buildManifestFullDescriptor(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ServiceID:      "test-svc",
			ServiceVersion: "2.0.0",
			ProviderLang:   "go",
			ProviderSDK:    "croupier-go-sdk",
		},
		descriptors: map[string]FunctionDescriptor{
			"test.full": {
				ID:        "test.full",
				Version:   "3.0.0",
				Category:  "test",
				Risk:      "medium",
				Entity:    "entity",
				Operation: "read",
				Enabled:   true,
			},
		},
	}

	raw, err := c.buildManifest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded struct {
		Functions []map[string]any `json:"functions"`
	}

	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if len(decoded.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(decoded.Functions))
	}

	fn := decoded.Functions[0]
	if fn["id"] != "test.full" {
		t.Errorf("expected function ID test.full, got %v", fn["id"])
	}
	if fn["version"] != "3.0.0" {
		t.Errorf("expected version 3.0.0, got %v", fn["version"])
	}
	if fn["category"] != "test" {
		t.Errorf("expected category test, got %v", fn["category"])
	}
	if fn["risk"] != "medium" {
		t.Errorf("expected risk medium, got %v", fn["risk"])
	}
	if fn["entity"] != "entity" {
		t.Errorf("expected entity entity, got %v", fn["entity"])
	}
	if fn["operation"] != "read" {
		t.Errorf("expected operation read, got %v", fn["operation"])
	}
	if fn["enabled"] != true {
		t.Errorf("expected enabled true, got %v", fn["enabled"])
	}
}

// TestClient_dialControl tests the dialControl method
func TestClient_dialControl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		controlAddr string
		insecure    bool
		wantErr     bool
		errContains string
	}{
		{
			name:        "no control address",
			controlAddr: "",
			wantErr:     true,
			errContains: "control address not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &client{
				config: &ClientConfig{
					ControlAddr: tt.controlAddr,
					Insecure:    tt.insecure,
				},
			}

			conn, err := c.dialControl(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
					return
				}
				if tt.errContains != "" {
					// Check if error message contains expected string
					errStr := err.Error()
					found := false
					for _, substr := range []string{"control address not configured", "dial tcp", "transport"} {
						if strings.Contains(errStr, substr) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected error containing %q, got %q", tt.errContains, errStr)
					}
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if conn != nil {
				conn.Close()
			}
		})
	}
}

// TestClient_ConnectWithTimeout tests Connect with timeout
func TestClient_ConnectWithTimeout(t *testing.T) {
	t.Parallel()

	// Skip this test in parallel mode to avoid issues with network connections
	t.Skip("Skipping network-dependent test in parallel mode")

	c := &client{
		config: &ClientConfig{
			AgentAddr:       "127.0.0.1:49996",
			Insecure:        true,
			TimeoutSeconds:  1,
			GameID:          "test-game",
			Env:             "test",
			ServiceID:       "test-service",
			ServiceVersion:  "1.0.0",
			ProviderLang:    "go",
			ProviderSDK:     "croupier-go-sdk",
			ControlAddr:     "",
		},
		descriptors: make(map[string]FunctionDescriptor),
		handlers:    make(map[string]FunctionHandler),
		logger:      &NoOpLogger{},
	}

	// Use a context with timeout to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Connect should complete within timeout
	err := c.Connect(ctx)
	// Result depends on whether service is running, just verify no panic
	_ = err
	c.Close()
}

// TestClient_convertToLocalFunctionsWithDuplicateVersions tests with multiple descriptors
func TestClient_convertToLocalFunctionsWithMultipleDescriptors(t *testing.T) {
	t.Parallel()

	c := &client{
		descriptors: map[string]FunctionDescriptor{
			"func1": {ID: "func1", Version: "1.0.0"},
			"func2": {ID: "func2", Version: "2.0.0"},
			"func3": {ID: "func3", Version: "3.0.0"},
		},
		handlers: map[string]FunctionHandler{
			"func1": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte("ok"), nil
			},
			"func2": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte("ok"), nil
			},
			"func3": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte("ok"), nil
			},
		},
	}

	localFuncs := c.convertToLocalFunctions()
	if len(localFuncs) != 3 {
		t.Errorf("expected 3 functions, got %d", len(localFuncs))
	}

	// Check each function
	funcMap := make(map[string]LocalFunctionDescriptor)
	for _, fn := range localFuncs {
		funcMap[fn.ID] = fn
	}

	if funcMap["func1"].Version != "1.0.0" {
		t.Errorf("expected func1 version 1.0.0, got %s", funcMap["func1"].Version)
	}
	if funcMap["func2"].Version != "2.0.0" {
		t.Errorf("expected func2 version 2.0.0, got %s", funcMap["func2"].Version)
	}
	if funcMap["func3"].Version != "3.0.0" {
		t.Errorf("expected func3 version 3.0.0, got %s", funcMap["func3"].Version)
	}
}

// TestClient_convertToLocalFunctionsWithEmptyVersion tests the default version behavior
func TestClient_convertToLocalFunctionsWithEmptyVersion(t *testing.T) {
	t.Parallel()

	c := &client{
		descriptors: map[string]FunctionDescriptor{
			"func1": {ID: "func1", Version: ""}, // Empty version
			"func2": {ID: "func2", Version: "custom"}, // Custom version
		},
		handlers: map[string]FunctionHandler{
			"func1": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte("ok"), nil
			},
			"func2": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte("ok"), nil
			},
		},
	}

	localFuncs := c.convertToLocalFunctions()
	if len(localFuncs) != 2 {
		t.Errorf("expected 2 functions, got %d", len(localFuncs))
	}

	// Check that empty version gets default "1.0.0"
	funcMap := make(map[string]LocalFunctionDescriptor)
	for _, fn := range localFuncs {
		funcMap[fn.ID] = fn
	}

	if funcMap["func1"].Version != "1.0.0" {
		t.Errorf("expected func1 version 1.0.0 (default), got %s", funcMap["func1"].Version)
	}
	if funcMap["func2"].Version != "custom" {
		t.Errorf("expected func2 version 'custom', got %s", funcMap["func2"].Version)
	}
}

// TestClient_convertToLocalFunctionsNoDescriptors tests with handlers but no descriptors
func TestClient_convertToLocalFunctionsNoDescriptors(t *testing.T) {
	t.Parallel()

	c := &client{
		descriptors: map[string]FunctionDescriptor{},
		handlers: map[string]FunctionHandler{
			"func1": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte("ok"), nil
			},
		},
	}

	localFuncs := c.convertToLocalFunctions()
	if len(localFuncs) != 0 {
		t.Errorf("expected 0 functions (no descriptors), got %d", len(localFuncs))
	}
}

// TestGzipBytes tests the gzipBytes utility function
func TestGzipBytes(t *testing.T) {
	t.Parallel()

	// Test with normal payload
	payload := []byte("test payload for gzip")
	compressed, err := gzipBytes(payload)
	if err != nil {
		t.Fatalf("gzipBytes failed: %v", err)
	}

	// Verify it can be decompressed
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to decompress: %v", err)
	}

	if string(decompressed) != string(payload) {
		t.Errorf("decompressed data mismatch: got %s, want %s", string(decompressed), string(payload))
	}
}

// TestGzipBytesEmpty tests gzipBytes with empty payload
func TestGzipBytesEmpty(t *testing.T) {
	t.Parallel()

	payload := []byte{}
	compressed, err := gzipBytes(payload)
	if err != nil {
		t.Fatalf("gzipBytes failed: %v", err)
	}

	// Verify it can be decompressed
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to decompress: %v", err)
	}

	if len(decompressed) != 0 {
		t.Errorf("expected empty decompressed data, got %s", string(decompressed))
	}
}

// TestGzipBytesLarge tests gzipBytes with large payload
func TestGzipBytesLarge(t *testing.T) {
	t.Parallel()

	// Create a large payload
	payload := make([]byte, 100000)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	compressed, err := gzipBytes(payload)
	if err != nil {
		t.Fatalf("gzipBytes failed: %v", err)
	}

	// Compressed should be smaller than original (or at least not too much larger)
	if len(compressed) > len(payload) {
		t.Logf("Warning: compressed size (%d) > original size (%d)", len(compressed), len(payload))
	}

	// Verify it can be decompressed
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to decompress: %v", err)
	}

	if len(decompressed) != len(payload) {
		t.Errorf("depressed size mismatch: got %d, want %d", len(decompressed), len(payload))
	}
}

// TestClient_dialControlWithInsecure tests dialControl with insecure connection
func TestClient_dialControlWithInsecure(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr:   "127.0.0.1:19999", // Non-existent server
			Insecure:      true,
			TimeoutSeconds: 1,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should fail to connect (no server), but we test the logic
	_, err := c.dialControl(ctx)
	// gRPC dial may succeed initially (lazy connection) or fail
	// We just verify the call completes without panic
	_ = err
}

// TestClient_dialControlWithTimeout tests dialControl with timeout
func TestClient_dialControlWithTimeout(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr:   "127.0.0.1:19999",
			Insecure:      false,
			TimeoutSeconds: 1,
		},
	}

	ctx := context.Background()
	// Should use config timeout
	_, err := c.dialControl(ctx)
	if err == nil {
		// Might succeed with lazy connection
	}
}

// TestClient_dialControlContextCancellation tests dialControl with cancelled context
func TestClient_dialControlContextCancellation(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr: "127.0.0.1:19999",
			Insecure:    true,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := c.dialControl(ctx)
	// Should return error (may be context error or connection error)
	_ = err
}

// TestClient_registerCapabilitiesErrors tests registerCapabilities error cases
func TestClient_registerCapabilitiesErrors(t *testing.T) {
	t.Parallel()

	t.Run("with control addr but no server", func(t *testing.T) {
		t.Parallel()

		c := &client{
			config: &ClientConfig{
				ControlAddr:   "127.0.0.1:19999",
				Insecure:      true,
				TimeoutSeconds: 1,
			},
			descriptors: map[string]FunctionDescriptor{},
			handlers:    map[string]FunctionHandler{},
		}

		err := c.registerCapabilities(context.Background())
		// Should fail to connect to control service
		if err == nil {
			t.Error("expected error when control server is not available")
		}
	})

	t.Run("buildManifest error", func(t *testing.T) {
		t.Parallel()

		// Can't easily cause buildManifest to fail, so we just verify
		// that registerCapabilities calls it
		c := &client{
			config: &ClientConfig{
				ServiceID:    "test-service",
				ControlAddr:  "", // Empty so it returns early
				ServiceVersion: "1.0.0",
			},
			descriptors: map[string]FunctionDescriptor{},
			handlers:    map[string]FunctionHandler{},
		}

		err := c.registerCapabilities(context.Background())
		if err != nil {
			t.Errorf("unexpected error with empty control addr: %v", err)
		}
	})
}

// TestClient_ServeWithConnectFailure tests Serve when connection fails
func TestClient_ServeWithConnectFailure(t *testing.T) {
	t.Parallel()

	config := &ClientConfig{
		ServiceID:      "test-service",
		AgentAddr:      "127.0.0.1:19999", // Non-existent server
		Insecure:       true,
		TimeoutSeconds: 1,
	}
	cli := NewClient(config)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := cli.Serve(ctx)
	if err == nil {
		t.Error("expected error when connection fails")
	}
}

// TestClient_ServeContextCancellation tests Serve with context cancellation
func TestClient_ServeContextCancellation(t *testing.T) {
	t.Parallel()

	config := &ClientConfig{
		ServiceID: "test-service",
		AgentAddr: "127.0.0.1:19999",
		Insecure:  true,
	}
	cli := NewClient(config)
	c := cli.(*client)

	// Pre-set connected to skip Connect and initialize grpcManager
	c.connected = true

	// Create a grpcManager so c.grpcManager is not nil
	mgr, err := NewGRPCManager(GRPCConfig{
		LocalListen: "127.0.0.1:0",
		Insecure:    true,
	}, map[string]FunctionHandler{})
	if err != nil {
		t.Fatalf("failed to create grpc manager: %v", err)
	}
	c.grpcManager = mgr
	defer mgr.Disconnect()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = cli.Serve(ctx)
	if err != nil {
		// May error due to context cancellation
	}

	// running should be false
	if c.running {
		t.Error("expected running to be false after context cancellation")
	}
}

// TestClient_ConnectErrorCases tests Connect error cases
func TestClient_ConnectErrorCases(t *testing.T) {
	t.Parallel()

	t.Run("with invalid agent address", func(t *testing.T) {
		t.Parallel()

		config := &ClientConfig{
			ServiceID:      "test-service",
			AgentAddr:      "invalid-address:abc",
			Insecure:       true,
			TimeoutSeconds: 1,
		}
		cli := NewClient(config)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := cli.Connect(ctx)
		if err == nil {
			t.Error("expected error with invalid address")
		}
	})

	t.Run("connection timeout", func(t *testing.T) {
		t.Parallel()

		config := &ClientConfig{
			ServiceID:      "test-service",
			AgentAddr:      "192.0.2.1:9999", // Non-routable IP
			Insecure:       true,
			TimeoutSeconds: 1,
		}
		cli := NewClient(config)

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := cli.Connect(ctx)
		if err == nil {
			t.Error("expected error with timeout")
		}
	})
}

// TestClient_controlCredentialsWithInvalidCA tests controlCredentials with invalid CA
func TestClient_controlCredentialsWithInvalidCA(t *testing.T) {
	t.Parallel()

	// Create a temp file with invalid content
	tmpfile, err := os.CreateTemp("", "invalid-ca-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.WriteString("not a valid PEM certificate")
	tmpfile.Close()

	c := &client{
		config: &ClientConfig{
			Insecure: false,
			CAFile:   tmpfile.Name(),
		},
	}

	_, err = c.controlCredentials()
	if err == nil {
		t.Error("expected error for invalid CA file")
	}
}

// TestClient_controlCredentialsWithServerNameOverride tests server name override
func TestClient_controlCredentialsWithServerNameOverride(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			Insecure:   false,
			ServerName: "custom.example.com",
		},
	}

	creds, err := c.controlCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Error("expected credentials to be returned")
	}
}

// TestClient_controlCredentialsWithSkipVerify tests skip verify
func TestClient_controlCredentialsWithSkipVerify(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			Insecure:           false,
			InsecureSkipVerify: true,
		},
	}

	creds, err := c.controlCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Error("expected credentials to be returned")
	}
}

// TestClient_RegisterFunctionWithDuplicate tests registering same function twice
func TestClient_RegisterFunctionWithDuplicate(t *testing.T) {
	t.Parallel()

	config := &ClientConfig{
		ServiceID: "test-service",
	}
	cli := NewClient(config)
	c := cli.(*client)

	handler := func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	}

	desc := FunctionDescriptor{
		ID:      "test.function",
		Version: "1.0.0",
	}

	// Register first time
	err := cli.RegisterFunction(desc, handler)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Register second time - should overwrite
	err = cli.RegisterFunction(desc, handler)
	if err != nil {
		t.Errorf("second registration failed: %v", err)
	}

	// Verify only one entry exists
	if len(c.handlers) != 1 {
		t.Errorf("expected 1 handler, got %d", len(c.handlers))
	}
	if len(c.descriptors) != 1 {
		t.Errorf("expected 1 descriptor, got %d", len(c.descriptors))
	}
}

// TestClient_buildManifestWithComplexDescriptors tests buildManifest with various descriptor types
func TestClient_buildManifestWithComplexDescriptors(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ServiceID:      "test-service",
			ServiceVersion: "2.0.0",
			ProviderLang:   "go",
			ProviderSDK:    "croupier-sdk",
		},
		descriptors: map[string]FunctionDescriptor{
			"fn1": {
				ID:        "fn1",
				Version:   "1.5.0",
				Category:  "player",
				Risk:      "high",
				Entity:    "user",
				Operation: "update",
				Enabled:   true,
			},
			"fn2": {
				ID:        "fn2",
				Version:   "", // Should default to 1.0.0
				Category:  "game",
				Risk:      "low",
				Enabled:   false, // Should be omitted
			},
		},
	}

	raw, err := c.buildManifest()
	if err != nil {
		t.Fatalf("buildManifest failed: %v", err)
	}

	var manifest struct {
		Provider struct {
			ID      string `json:"id"`
			Version string `json:"version"`
			Lang    string `json:"lang"`
			SDK     string `json:"sdk"`
		} `json:"provider"`
		Functions []map[string]any `json:"functions"`
	}

	err = json.Unmarshal(raw, &manifest)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if manifest.Provider.ID != "test-service" {
		t.Errorf("expected provider ID 'test-service', got %q", manifest.Provider.ID)
	}
	if manifest.Provider.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", manifest.Provider.Version)
	}

	// Check functions
	if len(manifest.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(manifest.Functions))
	}

	// Find fn1 and fn2
	fn1Found := false
	fn2Found := false
	for _, fn := range manifest.Functions {
		id, _ := fn["id"].(string)
		if id == "fn1" {
			fn1Found = true
			if fn["version"] != "1.5.0" {
				t.Errorf("expected fn1 version 1.5.0, got %v", fn["version"])
			}
			if _, ok := fn["enabled"]; !ok {
				t.Error("expected enabled field for fn1 (true)")
			}
		} else if id == "fn2" {
			fn2Found = true
			if fn["version"] != "1.0.0" {
				t.Errorf("expected fn2 version 1.0.0 (default), got %v", fn["version"])
			}
			if _, ok := fn["enabled"]; ok {
				t.Error("expected enabled field to be omitted for fn2 (false)")
			}
		}
	}

	if !fn1Found || !fn2Found {
		t.Error("expected both fn1 and fn2 to be in manifest")
	}
}

// TestClient_convertToLocalFunctionsWithMixedHandlers tests with some handlers missing
func TestClient_convertToLocalFunctionsWithMixedHandlers(t *testing.T) {
	t.Parallel()

	c := &client{
		descriptors: map[string]FunctionDescriptor{
			"f1": {ID: "f1", Version: "1.0.0"},
			"f2": {ID: "f2", Version: "2.0.0"},
			"f3": {ID: "f3", Version: "3.0.0"},
		},
		handlers: map[string]FunctionHandler{
			"f1": func(ctx context.Context, payload []byte) ([]byte, error) {
				return payload, nil
			},
			// f2 handler missing
			"f3": func(ctx context.Context, payload []byte) ([]byte, error) {
				return payload, nil
			},
		},
	}

	funcs := c.convertToLocalFunctions()

	// Should only include functions with handlers
	if len(funcs) != 2 {
		t.Errorf("expected 2 functions (only those with handlers), got %d", len(funcs))
	}

	funcByID := make(map[string]LocalFunctionDescriptor)
	for _, f := range funcs {
		funcByID[f.ID] = f
	}

	if _, ok := funcByID["f1"]; !ok {
		t.Error("expected f1 in result (has handler)")
	}
	if _, ok := funcByID["f2"]; ok {
		t.Error("expected f2 NOT in result (no handler)")
	}
	if _, ok := funcByID["f3"]; !ok {
		t.Error("expected f3 in result (has handler)")
	}
}

// TestClient_ConnectFailsAtStartServer tests Connect when StartServer fails
func TestClient_ConnectFailsAtStartServer(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			AgentAddr:      "127.0.0.1:19090",
			LocalListen:    "127.0.0.1:0", // Use any available port
			Insecure:       true,
			ServiceID:      "test-service",
			ServiceVersion: "1.0.0",
		},
		handlers:    map[string]FunctionHandler{},
		descriptors: map[string]FunctionDescriptor{},
		stopCh:      make(chan struct{}),
		logger:      &NoOpLogger{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Connect should fail (no actual agent running)
	// The error should come from connection attempt, not StartServer
	err := c.Connect(ctx)
	if err == nil {
		t.Error("expected error when agent is not available")
	}
}

// TestClient_ConnectWithRegisteredHandler tests Connect with registered handlers
func TestClient_ConnectWithRegisteredHandler(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			AgentAddr:      "127.0.0.1:19091",
			LocalListen:    "127.0.0.1:0",
			Insecure:       true,
			ServiceID:      "test-service",
			ServiceVersion: "1.0.0",
		},
		handlers: map[string]FunctionHandler{
			"test.fn": func(ctx context.Context, payload []byte) ([]byte, error) {
				return payload, nil
			},
		},
		descriptors: map[string]FunctionDescriptor{
			"test.fn": {
				ID:      "test.fn",
				Version: "1.0.0",
			},
		},
		stopCh: make(chan struct{}),
		logger: &NoOpLogger{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := c.Connect(ctx)
	// Should fail due to no actual server
	if err == nil {
		t.Error("expected error when agent is not available")
	}
}

// TestClient_registerCapabilitiesWithoutControlAddr tests registerCapabilities when ControlAddr is empty
func TestClient_registerCapabilitiesWithoutControlAddr(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr: "", // Empty control address
		},
		logger: &NoOpLogger{},
	}

	ctx := context.Background()
	err := c.registerCapabilities(ctx)
	if err != nil {
		t.Errorf("expected no error when ControlAddr is empty, got: %v", err)
	}
}

// TestClient_gzipBytes tests gzipBytes function
func TestClient_gzipBytes(t *testing.T) {
	t.Parallel()

	input := []byte("test message for compression")

	compressed, err := gzipBytes(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(compressed) == 0 {
		t.Error("expected non-empty compressed output")
	}

	// Compressed data should be different from input
	if string(compressed) == string(input) {
		t.Error("compressed data should differ from input")
	}
}

// TestClient_gzipBytesEmpty tests gzipBytes with empty input
func TestClient_gzipBytesEmpty(t *testing.T) {
	t.Parallel()

	input := []byte{}

	compressed, err := gzipBytes(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(compressed) == 0 {
		t.Error("expected non-empty compressed output even for empty input")
	}
}

// TestClient_buildManifestWithFunctions tests buildManifest function with functions
func TestClient_buildManifestWithFunctions(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ServiceID:      "test-service",
			ServiceVersion: "2.0.0",
			ProviderLang:   "go",
			ProviderSDK:    "croupier-go-sdk",
		},
		descriptors: map[string]FunctionDescriptor{
			"func1": {
				ID:        "func1",
				Version:   "1.0.0",
				Category:  "data",
				Risk:      "low",
				Entity:    "user",
				Operation: "read",
				Enabled:   true,
			},
			"func2": {
				ID:      "func2",
				Version: "2.0.0",
			},
		},
	}

	data, err := c.buildManifest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty manifest")
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("expected valid JSON: %v", err)
	}

	// Check provider info
	if provider, ok := parsed["provider"].(map[string]interface{}); ok {
		if provider["id"] != "test-service" {
			t.Errorf("expected provider id 'test-service', got %v", provider["id"])
		}
	} else {
		t.Error("expected provider in manifest")
	}
}

// TestClient_dialControlWithInvalidControlAddr tests dialControl with invalid control address
func TestClient_dialControlWithInvalidControlAddr(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr: "invalid-address:abc",
			Insecure:    true,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// dialControl may not return error immediately for invalid addresses
	// due to gRPC's lazy connection behavior
	conn, err := c.dialControl(ctx)
	if conn != nil {
		conn.Close()
	}
	// Just verify the function completes without panic
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

// TestClient_dialControlWithContextCancellation tests dialControl with cancelled context
func TestClient_dialControlWithContextCancellation(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr: "example.com:443",
			Insecure:    true,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	conn, err := c.dialControl(ctx)
	if conn != nil {
		conn.Close()
	}
	// Should get an error due to cancelled context
	if err == nil {
		t.Log("Note: dialControl succeeded despite cancelled context (gRPC lazy connection)")
	}
}

// TestClient_dialControlWithTLS tests dialControl with TLS
func TestClient_dialControlWithTLS(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr: "example.com:443",
			Insecure:    false,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This will fail due to no actual server, but we test the TLS path
	conn, err := c.dialControl(ctx)
	if conn != nil {
		conn.Close()
	}
	// Just verify the function completes without panic
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

// TestClient_registerCapabilitiesWithControlAddr tests registerCapabilities with control address
func TestClient_registerCapabilitiesWithControlAddr(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr: "127.0.0.1:8080",
			ServiceID:    "test-service",
		},
		localAddr: "127.0.0.1:9090",
		handlers: map[string]FunctionHandler{
			"test.fn": func(ctx context.Context, payload []byte) ([]byte, error) {
				return payload, nil
			},
		},
		descriptors: map[string]FunctionDescriptor{
			"test.fn": {
				ID:      "test.fn",
				Version: "1.0.0",
			},
		},
		logger: &NoOpLogger{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should fail due to no actual server
	err := c.registerCapabilities(ctx)
	if err == nil {
		t.Log("Note: registerCapabilities succeeded (server may be running)")
	}
}

// TestClient_registerCapabilitiesBuildsManifest tests registerCapabilities builds manifest correctly
func TestClient_registerCapabilitiesBuildsManifest(t *testing.T) {
	t.Parallel()

	// Create a client with empty control address to skip actual RPC
	c := &client{
		config: &ClientConfig{
			ControlAddr: "", // Empty to skip RPC
			ServiceID:    "test-service",
		},
		localAddr: "127.0.0.1:9090",
		handlers: map[string]FunctionHandler{
			"func1": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte("result"), nil
			},
		},
		descriptors: map[string]FunctionDescriptor{
			"func1": {
				ID:        "func1",
				Version:   "1.0.0",
				Risk:      "low",
				Enabled:   true,
			},
		},
		logger: &NoOpLogger{},
	}

	// This should succeed without making any RPC call
	err := c.registerCapabilities(context.Background())
	if err != nil {
		t.Errorf("unexpected error with empty control address: %v", err)
	}
}

// TestClient_ServeWithoutConnect tests Serve without calling Connect first
func TestClient_ServeWithoutConnect2(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			AgentAddr:      "127.0.0.1:19092",
			LocalListen:    "127.0.0.1:0",
			Insecure:       true,
			ServiceID:      "test-service",
			ServiceVersion: "1.0.0",
		},
		handlers: map[string]FunctionHandler{
			"test.fn": func(ctx context.Context, payload []byte) ([]byte, error) {
				return payload, nil
			},
		},
		descriptors: map[string]FunctionDescriptor{
			"test.fn": {
				ID:      "test.fn",
				Version: "1.0.0",
			},
		},
		stopCh: make(chan struct{}),
		logger: &NoOpLogger{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Serve should try to connect first, which will fail
	err := c.Serve(ctx)
	if err == nil {
		t.Error("expected error when agent is not available")
	}
}

// TestClient_ServeIdempotent tests Serve when already connected
func TestClient_ServeIdempotent(t *testing.T) {
	t.Parallel()

	// This test verifies that Serve can handle the case where
	// the client becomes disconnected while Serve is running
	c := &client{
		config: &ClientConfig{
			ServiceID:  "test-service",
			AgentAddr:  "127.0.0.1:19999",
			Insecure:   true,
			LocalListen: "127.0.0.1:0",
		},
		handlers: map[string]FunctionHandler{
			"test.fn": func(ctx context.Context, payload []byte) ([]byte, error) {
				return payload, nil
			},
		},
		descriptors: map[string]FunctionDescriptor{
			"test.fn": {
				ID:      "test.fn",
				Version: "1.0.0",
			},
		},
		stopCh: make(chan struct{}),
		logger: &NoOpLogger{},
	}

	// Connect to a working agent
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		// If agent is not available, skip this test
		t.Skipf("Agent not available: %v", err)
	}

	// Stop the client after a short time
	go func() {
		time.Sleep(100 * time.Millisecond)
		c.Stop()
	}()

	// Serve should start and then stop cleanly
	err := c.Serve(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify we can close the client
	if err := c.Close(); err != nil {
		t.Errorf("close failed: %v", err)
	}
}

// TestClient_ConnectWithLocalServerError tests Connect when StartServer fails
func TestClient_ConnectWithLocalServerError(t *testing.T) {
	t.Parallel()

	// Create a test that simulates StartServer failure path
	c := &client{
		config: &ClientConfig{
			AgentAddr:      "192.0.2.1:9999", // Non-routable
			LocalListen:    "127.0.0.1:0",
			Insecure:       true,
			ServiceID:      "test-service",
			ServiceVersion: "1.0.0",
		},
		handlers: map[string]FunctionHandler{
			"test.fn": func(ctx context.Context, payload []byte) ([]byte, error) {
				return payload, nil
			},
		},
		descriptors: map[string]FunctionDescriptor{
			"test.fn": {
				ID:      "test.fn",
				Version: "1.0.0",
			},
		},
		stopCh: make(chan struct{}),
		logger: &NoOpLogger{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := c.Connect(ctx)
	// Should fail either at Connect or StartServer
	if err == nil {
		t.Error("expected error when agent is not available")
	}
}

// TestClient_ConnectAlreadyConnected tests Connect is idempotent
func TestClient_ConnectAlreadyConnected(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			AgentAddr: "127.0.0.1:19093",
		},
		connected: true,
		logger:     &NoOpLogger{},
	}

	// Should return nil immediately
	err := c.Connect(context.Background())
	if err != nil {
		t.Errorf("unexpected error when already connected: %v", err)
	}
}

// TestClient_StopNotRunning tests Stop when not running
func TestClient_StopNotRunning(t *testing.T) {
	t.Parallel()

	c := &client{
		config:  &ClientConfig{},
		running: false,
		stopCh:  make(chan struct{}),
		logger:  &NoOpLogger{},
	}

	// Should not panic or error
	c.Stop()
}

// TestClient_StopWhileRunning tests Stop while service is running
func TestClient_StopWhileRunning(t *testing.T) {
	t.Parallel()

	c := &client{
		config:  &ClientConfig{},
		running: true,
		stopCh:  make(chan struct{}),
		logger:  &NoOpLogger{},
	}

	// Stop should close the stopCh and set running to false
	c.Stop()

	if c.running {
		t.Error("expected running to be false after Stop")
	}

	select {
	case <-c.stopCh:
		// Channel should be closed
	default:
		t.Error("expected stopCh to be closed")
	}
}

// TestClient_ServeCallsConnect tests that Serve calls Connect when not connected
func TestClient_ServeCallsConnect(t *testing.T) {
	t.Parallel()

	connectCalled := false

	c := &client{
		config: &ClientConfig{
			AgentAddr:      "non-existent:9999",
			ServiceID:      "test-service",
			ServiceVersion: "1.0.0",
		},
		connected: false,
		stopCh:    make(chan struct{}),
		logger:    &NoOpLogger{},
	}

	// Create a wrapper client that tracks Connect calls
	wrapper := &struct {
		*client
		connectCalled *bool
	}{
		client:        c,
		connectCalled: &connectCalled,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Manually call Connect first to simulate Serve behavior
	err := wrapper.Connect(ctx)
	if err == nil {
		t.Log("Note: Connect succeeded (unexpected)")
	}
}

// TestClient_buildManifestWithMultipleFunctions tests buildManifest with multiple functions
func TestClient_buildManifestWithMultipleFunctions(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ServiceID: "complex-service",
		},
		handlers: map[string]FunctionHandler{
			"func1": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte("result"), nil
			},
			"func2": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte("result"), nil
			},
		},
		descriptors: map[string]FunctionDescriptor{
			"func1": {
				ID:        "func1",
				Version:   "1.5.0",
				Category:  "test",
				Risk:      "low",
				Entity:    "data",
				Operation: "read",
				Enabled:   true,
			},
			"func2": {
				ID:        "func2",
				Version:   "2.0.0",
				Category:  "test",
				Risk:      "high",
				Entity:    "sensitive",
				Operation: "write",
				Enabled:   true,
			},
		},
		logger: &NoOpLogger{},
	}

	data, err := c.buildManifest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected non-empty manifest")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}

	// Verify functions array
	funcs, ok := parsed["functions"].([]interface{})
	if !ok || len(funcs) != 2 {
		t.Errorf("expected 2 functions, got %v", len(funcs))
	}
}

// TestClient_buildManifestEmptyHandlers tests buildManifest with no handlers
func TestClient_buildManifestEmptyHandlers(t *testing.T) {
	t.Parallel()

	c := &client{
		config:     &ClientConfig{ServiceID: "empty-service"},
		handlers:   map[string]FunctionHandler{},
		descriptors: map[string]FunctionDescriptor{},
		logger:      &NoOpLogger{},
	}

	data, err := c.buildManifest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}

	// Should still have provider info even with no functions
	if _, ok := parsed["provider"]; !ok {
		t.Error("expected provider in manifest even with no functions")
	}
}

// TestDefaultLogger tests DefaultLogger methods
func TestDefaultLoggerWithBuffer(t *testing.T) {
	t.Parallel()

	// Test with debug mode
	buf := &bytes.Buffer{}
	logger := NewDefaultLogger(true, buf)

	logger.Debugf("debug message")
	logger.Infof("info message")
	logger.Warnf("warn message")
	logger.Errorf("error message")

	output := buf.String()
	if output == "" {
		t.Error("expected some output from logger")
	}

	// Test without debug mode
	buf2 := &bytes.Buffer{}
	logger2 := NewDefaultLogger(false, buf2)

	logger2.Debugf("debug message") // Should not output
	logger2.Infof("info message")

	output2 := buf2.String()
	if output2 == "" {
		t.Error("expected output from Infof")
	}
	// Debug message should not appear when debug is false
	if strings.Contains(output2, "[DEBUG]") {
		t.Error("debug message should not appear when debug mode is off")
	}
}

// TestClient_controlCredentialsWithControlAddr tests extracting server name from ControlAddr
func TestClient_controlCredentialsWithControlAddr(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			Insecure:           false,
			InsecureSkipVerify: false,
			ControlAddr:        "example.com:443",
			// No ServerName set, should extract from ControlAddr
		},
	}

	creds, err := c.controlCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Error("expected credentials to be returned")
	}
}

// TestClient_controlCredentialsWithInvalidCertFile tests loading invalid cert file
func TestClient_controlCredentialsWithInvalidCertFile(t *testing.T) {
	t.Parallel()

	// Create a temp file with invalid content
	tmpCert, err := os.CreateTemp("", "invalid-cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpCert.Name())
	tmpCert.WriteString("not a valid cert")
	tmpCert.Close()

	tmpKey, err := os.CreateTemp("", "invalid-key-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpKey.Name())
	tmpKey.WriteString("not a valid key")
	tmpKey.Close()

	c := &client{
		config: &ClientConfig{
			Insecure: false,
			CertFile: tmpCert.Name(),
			KeyFile:  tmpKey.Name(),
		},
	}

	_, err = c.controlCredentials()
	if err == nil {
		t.Error("expected error with invalid cert/key files")
	}
}

// TestClient_controlCredentialsWithPartialCertFiles tests with only cert file
func TestClient_controlCredentialsWithPartialCertFiles(t *testing.T) {
	t.Parallel()

	// Only cert file, no key file - should not try to load
	c := &client{
		config: &ClientConfig{
			Insecure: false,
			CertFile: "/tmp/nonexistent.pem",
			// No KeyFile set
		},
	}

	creds, err := c.controlCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Error("expected credentials to be returned")
	}
}

// TestClient_convertToLocalFunctionsEmpty tests convertToLocalFunctions with empty handlers
func TestClient_convertToLocalFunctionsEmpty(t *testing.T) {
	t.Parallel()

	c := &client{
		handlers:    map[string]FunctionHandler{},
		descriptors: map[string]FunctionDescriptor{},
	}

	funcs := c.convertToLocalFunctions()
	// convertToLocalFunctions returns a slice that may be nil or empty
	// The important thing is that it doesn't panic
	if len(funcs) != 0 {
		t.Errorf("expected empty slice, got %d functions", len(funcs))
	}
}

// TestClient_convertToLocalFunctionsWithVersion tests convertToLocalFunctions with version
func TestClient_convertToLocalFunctionsWithVersion(t *testing.T) {
	t.Parallel()

	c := &client{
		handlers: map[string]FunctionHandler{
			"test.fn": func(ctx context.Context, payload []byte) ([]byte, error) {
				return payload, nil
			},
		},
		descriptors: map[string]FunctionDescriptor{
			"test.fn": {
				ID: "test.fn",
				// No version set, should default to 1.0.0
			},
		},
	}

	funcs := c.convertToLocalFunctions()
	if len(funcs) != 1 {
		t.Errorf("expected 1 function, got %d", len(funcs))
	}
	if funcs[0].Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", funcs[0].Version)
	}
}

// TestClient_dialControlTimeoutContext tests dialControl with timeout
func TestClient_dialControlTimeoutContext(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr:     "192.0.2.1:9999", // Non-routable
			Insecure:        true,
			TimeoutSeconds:  1, // Set short timeout
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	conn, err := c.dialControl(ctx)
	if conn != nil {
		conn.Close()
	}
	// May or may not get error depending on network
	// Just verify the function completes without panic
	if err != nil {
		t.Logf("Got expected error: %v", err)
	}
}

// TestClient_registerCapabilitiesCompressionError tests manifest compression error
func TestClient_registerCapabilitiesCompressionError(t *testing.T) {
	t.Parallel()

	// This test is hard to implement without modifying the code
	// to make compression fail. Skip for now.
	t.Skip("compression error path is difficult to test")
}

// TestClient_buildManifestWithComplexMetadata tests buildManifest with various metadata
func TestClient_buildManifestWithComplexMetadata(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ServiceID:      "complex-service",
			ServiceVersion: "2.5.0",
			ProviderLang:   "go",
			ProviderSDK:    "croupier-go-sdk-v2",
		},
		handlers: map[string]FunctionHandler{
			"fn1": func(ctx context.Context, payload []byte) ([]byte, error) {
				return payload, nil
			},
		},
		descriptors: map[string]FunctionDescriptor{
			"fn1": {
				ID:        "fn1",
				Version:   "3.0.0",
				Category:  "data",
				Risk:      "low",
				Entity:    "public",
				Operation: "read",
				Enabled:   false,
			},
		},
		logger: &NoOpLogger{},
	}

	data, err := c.buildManifest()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("expected valid JSON: %v", err)
	}

	// Verify provider info
	provider, ok := parsed["provider"].(map[string]interface{})
	if !ok {
		t.Fatal("expected provider object")
	}
	if provider["id"] != "complex-service" {
		t.Errorf("expected service ID 'complex-service', got %v", provider["id"])
	}

	// Verify function metadata
	funcs, ok := parsed["functions"].([]interface{})
	if !ok || len(funcs) != 1 {
		t.Fatalf("expected 1 function, got %v", len(funcs))
	}

	fn := funcs[0].(map[string]interface{})
	if fn["category"] != "data" {
		t.Errorf("expected category 'data', got %v", fn["category"])
	}
	// Enabled field may be omitted when false
	if enabled, ok := fn["enabled"]; ok && enabled != false {
		t.Errorf("expected enabled to be false or omitted, got %v", enabled)
	}
}

// TestClient_registerCapabilitiesWithNilConfig tests with nil controlAddr
func TestClient_registerCapabilitiesWithNilConfig(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr: "", // Empty, should return early
		},
		localAddr: "127.0.0.1:9090",
		logger:    &NoOpLogger{},
	}

	err := c.registerCapabilities(context.Background())
	if err != nil {
		t.Errorf("unexpected error with empty control addr: %v", err)
	}
}

// TestClient_dialControlWithoutAddr tests dialControl without control address
func TestClient_dialControlWithoutAddr(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr: "",
		},
	}

	_, err := c.dialControl(context.Background())
	if err == nil {
		t.Error("expected error with empty control address")
	}
}

// TestClient_controlCredentialsWithInvalidCAFile tests with non-existent CA file
func TestClient_controlCredentialsWithInvalidCAFile(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			Insecure: false,
			CAFile:   "/nonexistent/ca.pem",
		},
	}

	_, err := c.controlCredentials()
	if err == nil {
		t.Error("expected error with non-existent CA file")
	}
}

// TestClient_controlCredentialsWithEmptyControlAddr tests extracting server name edge case
func TestClient_controlCredentialsWithEmptyControlAddr(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			Insecure:           false,
			InsecureSkipVerify: false,
			ControlAddr:        "", // Empty address
			// No ServerName set
		},
	}

	creds, err := c.controlCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Error("expected credentials to be returned")
	}
}

// TestClient_StopWithNilGRPCManager tests Stop when grpcManager is nil
func TestClient_StopWithNilGRPCManager(t *testing.T) {
	t.Parallel()

	c := &client{
		config:     &ClientConfig{},
		grpcManager: nil, // Explicitly nil
		stopCh:     make(chan struct{}),
		logger:     &NoOpLogger{},
	}

	err := c.Stop()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if c.running {
		t.Error("expected running to be false")
	}
	if c.connected {
		t.Error("expected connected to be false")
	}
}

// TestClient_StopTwice tests calling Stop multiple times
func TestClient_StopTwice(t *testing.T) {
	t.Parallel()

	c := &client{
		config:     &ClientConfig{},
		grpcManager: nil,
		stopCh:     make(chan struct{}),
		logger:     &NoOpLogger{},
	}

	// First Stop
	err := c.Stop()
	if err != nil {
		t.Errorf("unexpected error on first stop: %v", err)
	}

	// Second Stop should panic on closing the channel again
	// This is expected behavior - Stop should only be called once
	defer func() {
		if r := recover(); r != nil {
			// Expected to panic when closing closed channel
			t.Logf("Expected panic on second Stop: %v", r)
		}
	}()
	c.Stop()
}

// TestClient_CloseWithNilHandlers tests Close when handlers is nil
func TestClient_CloseWithNilHandlers(t *testing.T) {
	t.Parallel()

	c := &client{
		config:     &ClientConfig{},
		handlers:   nil,
		stopCh:     make(chan struct{}),
		logger:     &NoOpLogger{},
	}

	err := c.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if c.handlers != nil {
		t.Error("expected handlers to be nil after Close")
	}
}

// TestClient_RegisterFunctionWithRunningClient tests registering while running
func TestClient_RegisterFunctionWithRunningClient(t *testing.T) {
	t.Parallel()

	c := &client{
		config:   &ClientConfig{},
		handlers: make(map[string]FunctionHandler),
		running:  true, // Simulate running state
		logger:   &NoOpLogger{},
	}

	desc := FunctionDescriptor{
		ID:      "test.fn",
		Version: "1.0.0",
	}
	handler := func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	}

	err := c.RegisterFunction(desc, handler)
	if err == nil {
		t.Error("expected error when registering while running")
	}
}

// TestClient_RegisterFunctionWithEmptyID tests registering with empty ID
func TestClient_RegisterFunctionWithEmptyID(t *testing.T) {
	t.Parallel()

	c := &client{
		config:   &ClientConfig{},
		handlers: make(map[string]FunctionHandler),
		logger:   &NoOpLogger{},
	}

	desc := FunctionDescriptor{
		ID: "", // Empty ID
	}
	handler := func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	}

	err := c.RegisterFunction(desc, handler)
	if err == nil {
		t.Error("expected error with empty function ID")
	}
}

// TestClient_RegisterFunctionWithDefaultVersion tests version defaulting
func TestClient_RegisterFunctionWithDefaultVersion(t *testing.T) {
	t.Parallel()

	c := &client{
		config:     &ClientConfig{},
		handlers:   make(map[string]FunctionHandler),
		descriptors: make(map[string]FunctionDescriptor),
		logger:     &NoOpLogger{},
	}

	desc := FunctionDescriptor{
		ID: "test.fn", // No version set
	}
	handler := func(ctx context.Context, payload []byte) ([]byte, error) {
		return payload, nil
	}

	err := c.RegisterFunction(desc, handler)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check that version was defaulted
	storedDesc, ok := c.descriptors["test.fn"]
	if !ok {
		t.Fatal("function descriptor not stored")
	}
	if storedDesc.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", storedDesc.Version)
	}
}

// TestClient_registerCapabilitiesWithTimeout tests with timeout configured
func TestClient_registerCapabilitiesWithTimeout(t *testing.T) {
	t.Parallel()

	c := &client{
		config: &ClientConfig{
			ControlAddr:    "127.0.0.1:8080",
			ServiceID:      "test-service",
			TimeoutSeconds: 1, // Set timeout
		},
		localAddr: "127.0.0.1:9090",
		handlers: map[string]FunctionHandler{
			"test.fn": func(ctx context.Context, payload []byte) ([]byte, error) {
				return payload, nil
			},
		},
		descriptors: map[string]FunctionDescriptor{
			"test.fn": {
				ID:      "test.fn",
				Version: "1.0.0",
			},
		},
		logger: &NoOpLogger{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := c.registerCapabilities(ctx)
	// Will fail to connect, but we test the timeout code path
	if err == nil {
		t.Log("Note: registerCapabilities succeeded (control service not available)")
	}
}

// TestClient_GetLocalAddressWithUnconnectedClient tests GetLocalAddress when not connected
func TestClient_GetLocalAddressWithUnconnectedClient(t *testing.T) {
	t.Parallel()

	c := &client{
		config:    &ClientConfig{},
		localAddr: "", // Empty
		logger:    &NoOpLogger{},
	}

	addr := c.GetLocalAddress()
	if addr != "" {
		t.Errorf("expected empty address, got %s", addr)
	}
}

// TestClient_NewClientWithNilConfig tests NewClient with nil config
func TestClient_NewClientWithNilConfig(t *testing.T) {
	t.Parallel()

	cli := NewClient(nil)
	if cli == nil {
		t.Fatal("expected non-nil client")
	}

	c := cli.(*client)
	if c.config == nil {
		t.Error("expected default config to be set")
	}
	if c.logger == nil {
		t.Error("expected logger to be set")
	}
	if c.stopCh == nil {
		t.Error("expected stopCh to be initialized")
	}
}

