package croupier

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
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
