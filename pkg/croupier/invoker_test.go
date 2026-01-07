package croupier

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestInvoker_validatePayload(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19090",
		TimeoutSeconds: 1,
		Insecure:       true,
	})

	impl, ok := i.(*invoker)
	if !ok {
		t.Fatalf("expected *invoker, got %T", i)
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []any{"name"},
	}

	if err := impl.SetSchema("f1", schema); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}

	if _, err := impl.Invoke(context.Background(), "f1", `{}`, InvokeOptions{}); err == nil {
		t.Fatalf("expected validation error")
	}

	if _, err := impl.Invoke(context.Background(), "f1", `{"name":"alice"}`, InvokeOptions{}); err == nil {
		t.Fatalf("expected connect error (no server), got nil")
	}
}

func TestInvoker_isConnectionError(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"broken pipe", errors.New("broken pipe"), true},
		{"network unreachable", errors.New("network is unreachable"), true},
		{"no such host", errors.New("no such host"), true},
		{"timeout", errors.New("context timeout exceeded"), true},
		{"transport closing", errors.New("transport is closing"), true},
		{"connection unavailable", errors.New("connection unavailable"), true},
		{"other error", errors.New("some other error"), false},
		{"mixed case", errors.New("Connection Refused"), true},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := i.isConnectionError(tc.err)
			if result != tc.expected {
				t.Errorf("isConnectionError(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestInvoker_calculateReconnectDelay(t *testing.T) {
	t.Parallel()

	t.Run("with default config", func(t *testing.T) {
		t.Parallel()

		i := NewInvoker(&InvokerConfig{
			Address:  "127.0.0.1:19090",
			Insecure: true,
			Reconnect: &ReconnectConfig{
				Enabled:           true,
				InitialDelayMs:    1000,
				MaxDelayMs:        30000,
				BackoffMultiplier: 2.0,
				JitterFactor:      0.2,
			},
		}).(*invoker)

		// First attempt (reconnectAttempts = 1, exponent = 0)
		i.reconnectAttempts = 1
		delay := i.calculateReconnectDelay()
		if delay < 800*time.Millisecond || delay > 1200*time.Millisecond {
			t.Errorf("expected delay around 1s with jitter, got %v", delay)
		}
	})

	t.Run("exponential backoff", func(t *testing.T) {
		t.Parallel()

		i := NewInvoker(&InvokerConfig{
			Address:  "127.0.0.1:19090",
			Insecure: true,
			Reconnect: &ReconnectConfig{
				Enabled:           true,
				InitialDelayMs:    1000,
				MaxDelayMs:        10000,
				BackoffMultiplier: 2.0,
				JitterFactor:      0.0, // No jitter for predictable testing
			},
		}).(*invoker)

		// reconnectAttempts is incremented before calling calculateReconnectDelay
		// So attempts = 1 means exponent = 0, delay = 1s * 2^0 = 1s
		// attempts = 2 means exponent = 1, delay = 1s * 2^1 = 2s
		i.reconnectAttempts = 1
		delay1 := i.calculateReconnectDelay()

		i.reconnectAttempts = 2
		delay2 := i.calculateReconnectDelay()

		i.reconnectAttempts = 3
		delay3 := i.calculateReconnectDelay()

		// With exponential backoff: 1s, 2s, 4s (capped at max 10s)
		if delay1 != 1000*time.Millisecond {
			t.Errorf("expected 1s, got %v", delay1)
		}
		if delay2 != 2000*time.Millisecond {
			t.Errorf("expected 2s, got %v", delay2)
		}
		if delay3 != 4000*time.Millisecond {
			t.Errorf("expected 4s, got %v", delay3)
		}
	})

	t.Run("max delay cap", func(t *testing.T) {
		t.Parallel()

		i := NewInvoker(&InvokerConfig{
			Address:  "127.0.0.1:19090",
			Insecure: true,
			Reconnect: &ReconnectConfig{
				Enabled:           true,
				InitialDelayMs:    1000,
				MaxDelayMs:        3000,
				BackoffMultiplier: 10.0,
				JitterFactor:      0.0,
			},
		}).(*invoker)

		// Large number of attempts should still be capped
		i.reconnectAttempts = 10
		delay := i.calculateReconnectDelay()
		if delay != 3000*time.Millisecond {
			t.Errorf("expected max delay 3s, got %v", delay)
		}
	})
}

func TestInvoker_calculateRetryDelay(t *testing.T) {
	t.Parallel()

	cfg := &RetryConfig{
		Enabled:           true,
		MaxAttempts:       3,
		InitialDelayMs:    100,
		MaxDelayMs:        5000,
		BackoffMultiplier: 2.0,
		JitterFactor:      0.2,
	}

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
		Retry:    cfg,
	}).(*invoker)

	// Test first retry (attempt = 0 means first retry, exponent = 0)
	delay := i.calculateRetryDelay(0, cfg)
	if delay < 80*time.Millisecond || delay > 120*time.Millisecond {
		t.Errorf("expected delay around 100ms with jitter, got %v", delay)
	}

	// Test exponential backoff
	cfg.JitterFactor = 0.0
	delay = i.calculateRetryDelay(0, cfg)
	if delay != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", delay)
	}

	delay = i.calculateRetryDelay(1, cfg)
	if delay != 200*time.Millisecond {
		t.Errorf("expected 200ms, got %v", delay)
	}
}

func TestInvoker_isRetryableError(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"not retryable status", fmt.Errorf("rpc error: code = Internal desc = something"), false},
		{"not found", fmt.Errorf("rpc error: code = NotFound desc = not found"), false},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := i.isRetryableError(tc.err)
			if result != tc.expected {
				t.Errorf("isRetryableError(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestInvoker_callContext(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19090",
		TimeoutSeconds: 30,
		Insecure:       true,
	}).(*invoker)

	ctx, cancel := i.callContext(context.Background(), 0)
	defer cancel()

	// Should use default timeout
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline to be set")
	}

	// Should be approximately 30 seconds from now
	remaining := time.Until(deadline)
	if remaining < 29*time.Second || remaining > 31*time.Second {
		t.Errorf("expected deadline around 30s, got %v", remaining)
	}
}

func TestInvoker_validatePayloadEmpty(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	// Empty schema with empty payload should fail
	err := i.validatePayload("", map[string]any{})
	if err == nil {
		t.Error("expected error for empty payload with empty schema")
	}

	// Empty schema with non-empty payload should succeed
	err = i.validatePayload("test", map[string]any{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Non-empty schema with valid payload should pass validation
	err = i.validatePayload(`{"name":"test"}`, map[string]any{"type": "object"})
	if err != nil {
		t.Errorf("unexpected error for valid payload: %v", err)
	}
}

func TestInvoker_SetSchema(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}

	err := i.SetSchema("test.fn", schema)
	if err != nil {
		t.Fatalf("SetSchema: %v", err)
	}

	// Verify schema was set
	impl := i.(*invoker)
	if impl.schemas["test.fn"] == nil {
		t.Error("schema was not set")
	}
}

func TestInvoker_Close(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	// Set some state
	impl := i.(*invoker)
	impl.connected = true
	impl.schemas["test"] = map[string]any{}

	err := i.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify state was cleared
	if impl.connected {
		t.Error("expected connected to be false")
	}
	if len(impl.schemas) != 0 {
		t.Error("expected schemas to be cleared")
	}
}

func TestInvoker_scheduleReconnectIfNeeded(t *testing.T) {
	t.Parallel()

	t.Run("disabled reconnect", func(t *testing.T) {
		t.Parallel()

		i := NewInvoker(&InvokerConfig{
			Address:  "127.0.0.1:19090",
			Insecure: true,
			Reconnect: &ReconnectConfig{
				Enabled: false,
			},
		}).(*invoker)

		// Should not schedule reconnect
		i.scheduleReconnectIfNeeded()
		if i.isReconnecting {
			t.Error("expected isReconnecting to be false")
		}
	})

	t.Run("max attempts reached", func(t *testing.T) {
		t.Parallel()

		i := NewInvoker(&InvokerConfig{
			Address:  "127.0.0.1:19090",
			Insecure: true,
			Reconnect: &ReconnectConfig{
				Enabled:     true,
				MaxAttempts: 3,
			},
		}).(*invoker)

		i.reconnectAttempts = 3
		i.scheduleReconnectIfNeeded()
		if i.isReconnecting {
			t.Error("expected isReconnecting to be false when max attempts reached")
		}
	})

	t.Run("already reconnecting", func(t *testing.T) {
		t.Parallel()

		i := NewInvoker(&InvokerConfig{
			Address:  "127.0.0.1:19090",
			Insecure: true,
			Reconnect: &ReconnectConfig{
				Enabled:     true,
				MaxAttempts: 0,
			},
		}).(*invoker)

		i.isReconnecting = true
		i.scheduleReconnectIfNeeded()
		// Should not increment attempts
		if i.reconnectAttempts != 0 {
			t.Error("expected reconnectAttempts to remain 0")
		}
	})
}

func TestInvoker_ConnectIdempotent(t *testing.T) {
	t.Parallel()

	inv := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	// Set connected to true
	impl, ok := inv.(*invoker)
	if !ok {
		t.Fatal("type assertion failed")
	}
	impl.connected = true

	// Should not return error even though server doesn't exist
	err := inv.Connect(context.Background())
	if err != nil {
		t.Errorf("expected no error when already connected, got %v", err)
	}
}

func TestNewInvokerDefaults(t *testing.T) {
	t.Parallel()

	i := NewInvoker(nil)
	if i == nil {
		t.Fatal("expected non-nil invoker")
	}

	impl := i.(*invoker)
	if impl.config.Address != "localhost:8080" {
		t.Errorf("expected default address localhost:8080, got %s", impl.config.Address)
	}
	if impl.config.TimeoutSeconds != 30 {
		t.Errorf("expected default timeout 30s, got %d", impl.config.TimeoutSeconds)
	}
	if !impl.config.Insecure {
		t.Error("expected default insecure to be true")
	}
	if impl.config.Reconnect == nil {
		t.Error("expected default reconnect config")
	}
	if impl.config.Retry == nil {
		t.Error("expected default retry config")
	}
}

func TestInvoker_validatePayloadWithTypeValidation(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":     map[string]any{"type": "string"},
			"age":      map[string]any{"type": "integer"},
			"active":   map[string]any{"type": "boolean"},
			"score":    map[string]any{"type": "number"},
			"tags":     map[string]any{"type": "array"},
			"metadata": map[string]any{"type": "object"},
		},
	}

	// Test type validation
	tests := []struct {
		name      string
		payload   string
		shouldErr bool
	}{
		{"valid string", `{"name":"test"}`, false},
		{"wrong type for string", `{"name":123}`, true},
		{"wrong type for integer", `{"age":"not_a_number"}`, true},
		{"wrong type for boolean", `{"active":"yes"}`, true},
		{"wrong type for number", `{"score":"high"}`, true},
		{"valid integer", `{"age":25}`, false},
		{"valid boolean", `{"active":true}`, false},
		{"valid number", `{"score":99.5}`, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := i.validatePayload(tt.payload, schema)
			if (err != nil) != tt.shouldErr {
				t.Errorf("validatePayload(%q) error = %v, shouldErr %v", tt.payload, err, tt.shouldErr)
			}
		})
	}
}

func TestInvoker_validatePayloadWithInvalidJSON(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
	}

	err := i.validatePayload("not json", schema)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestInvoker_validatePayloadWithEmptyObject(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	// Empty object with empty schema should fail
	err := i.validatePayload("", map[string]any{})
	if err == nil {
		t.Error("expected error for empty payload with empty schema")
	}
}

func TestInvoker_InvokeWithOptions(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19090",
		TimeoutSeconds: 30,
		Insecure:       true,
	})

	opts := InvokeOptions{
		IdempotencyKey: "test-key",
		Headers:        map[string]string{"key": "value"},
	}

	// Should fail to connect (no server), but we test options handling
	_, err := i.Invoke(context.Background(), "test", "{}", opts)
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestInvoker_buildTLSConfig(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	// buildTLSConfig is a private method that creates TLS credentials
	// For insecure connections, it's not called, so we can't directly test it
	// But we can verify that an invoker with insecure=true doesn't require certs
	if !i.config.Insecure {
		t.Error("expected insecure to be true")
	}
}

func TestInvoker_InvokeWithoutConnect(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	_, err := i.Invoke(context.Background(), "test", "{}", InvokeOptions{})
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestInvoker_configDefaultTimeout(t *testing.T) {
	t.Parallel()

	cfg := &InvokerConfig{
		Address:        "127.0.0.1:19090",
		TimeoutSeconds: 60,
		Insecure:       true,
	}

	i := NewInvoker(cfg).(*invoker)
	if i.config.DefaultTimeout != 60*time.Second {
		t.Errorf("expected DefaultTimeout 60s, got %v", i.config.DefaultTimeout)
	}
}

func TestInvoker_configWithProvidedTimeout(t *testing.T) {
	t.Parallel()

	cfg := &InvokerConfig{
		Address:        "127.0.0.1:19090",
		TimeoutSeconds: 30,
		Insecure:       true,
		DefaultTimeout: 45 * time.Second,
	}

	i := NewInvoker(cfg).(*invoker)
	if i.config.DefaultTimeout != 45*time.Second {
		t.Errorf("expected DefaultTimeout 45s, got %v", i.config.DefaultTimeout)
	}
}

func TestInvoker_InvokeWithSchemaValidation(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	impl := i.(*invoker)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
		"required": []any{"value"},
	}

	if err := impl.SetSchema("test.fn", schema); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}

	// Missing required field
	_, err := i.Invoke(context.Background(), "test.fn", `{"other":"data"}`, InvokeOptions{})
	if err == nil {
		t.Error("expected validation error for missing required field")
	}
}

func TestInvoker_executeWithRetryNoRetry(t *testing.T) {
	t.Parallel()

	cfg := &RetryConfig{
		Enabled:     false,
		MaxAttempts: 3,
	}

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
		Retry:    cfg,
	}).(*invoker)

	attempts := 0
	ctx := context.Background()
	fn := func() (string, error) {
		attempts++
		return "", errors.New("test error")
	}

	_, err := i.executeWithRetry(ctx, InvokeOptions{}, fn)
	if err == nil {
		t.Error("expected error")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt when retry disabled, got %d", attempts)
	}
}

func TestInvoker_executeWithRetrySuccess(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	ctx := context.Background()
	expectedResult := "success"
	fn := func() (string, error) {
		return expectedResult, nil
	}

	result, err := i.executeWithRetry(ctx, InvokeOptions{}, fn)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != expectedResult {
		t.Errorf("expected %s, got %s", expectedResult, result)
	}
}
