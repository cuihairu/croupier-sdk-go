package croupier

import (
	"context"
	"errors"
	"fmt"
	"os"
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

func TestInvoker_StartJob(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	// StartJob should fail without connection
	_, err := i.StartJob(context.Background(), "test.fn", "{}", InvokeOptions{})
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestInvoker_StreamJob(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	// StreamJob should fail without connection
	stream, err := i.StreamJob(context.Background(), "test-job-id")
	if err == nil {
		t.Error("expected error when not connected")
	}
	// StreamJob always returns a channel (even on error)
	if stream == nil {
		t.Error("expected non-nil channel")
	}
}

func TestInvoker_CancelJob(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	// CancelJob should fail without connection
	err := i.CancelJob(context.Background(), "test-job-id")
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestInvoker_StartJobWithContext(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19090",
		TimeoutSeconds: 5,
		Insecure:       true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// StartJob should fail quickly with short timeout
	_, err := i.StartJob(ctx, "test.fn", "{}", InvokeOptions{})
	if err == nil {
		t.Error("expected error with short timeout")
	}
}

func TestInvoker_StreamJobWithInvalidJobID(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	// StreamJob with empty job ID - will try to connect and fail
	stream, err := i.StreamJob(context.Background(), "")
	if err == nil {
		t.Error("expected error with empty job ID")
	}
	// StreamJob always returns a channel (even on error)
	if stream == nil {
		t.Error("expected non-nil channel")
	}
}

func TestInvoker_validatePayloadWithNumberTypes(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"int_val":   map[string]any{"type": "integer"},
			"num_val":   map[string]any{"type": "number"},
			"bool_val":  map[string]any{"type": "boolean"},
			"str_val":   map[string]any{"type": "string"},
			"array_val": map[string]any{"type": "array"},
			"obj_val":   map[string]any{"type": "object"},
		},
	}

	// Valid integer
	err := i.validatePayload(`{"int_val":42}`, schema)
	if err != nil {
		t.Errorf("unexpected error for integer: %v", err)
	}

	// Valid number (float)
	err = i.validatePayload(`{"num_val":3.14}`, schema)
	if err != nil {
		t.Errorf("unexpected error for number: %v", err)
	}

	// Zero values
	err = i.validatePayload(`{"int_val":0,"num_val":0.0}`, schema)
	if err != nil {
		t.Errorf("unexpected error for zero values: %v", err)
	}

	// Negative numbers
	err = i.validatePayload(`{"int_val":-10,"num_val":-3.14}`, schema)
	if err != nil {
		t.Errorf("unexpected error for negative numbers: %v", err)
	}
}

func TestInvoker_validatePayloadEdgeCases(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{},
		},
	}

	// Missing type in property schema should pass (no type validation)
	err := i.validatePayload(`{"value":"anything"}`, schema)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Extra properties should be allowed
	err = i.validatePayload(`{"value":"test","extra":"data"}`, schema)
	if err != nil {
		t.Errorf("unexpected error for extra properties: %v", err)
	}
}

func TestInvoker_callContextWithCustomTimeout(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19090",
		TimeoutSeconds: 30,
		Insecure:       true,
	}).(*invoker)

	// Custom timeout in options
	opts := InvokeOptions{
		Timeout: 5 * time.Second,
	}

	ctx, cancel := i.callContext(context.Background(), opts.Timeout)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline to be set")
	}

	remaining := time.Until(deadline)
	if remaining < 4*time.Second || remaining > 6*time.Second {
		t.Errorf("expected deadline around 5s, got %v", remaining)
	}
}

func TestInvoker_callContextWithZeroTimeout(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19090",
		TimeoutSeconds: 30,
		Insecure:       true,
	}).(*invoker)

	opts := InvokeOptions{
		Timeout: 0,
	}

	ctx, cancel := i.callContext(context.Background(), opts.Timeout)
	defer cancel()

	// Should use default timeout from config
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline to be set")
	}

	remaining := time.Until(deadline)
	if remaining < 29*time.Second || remaining > 31*time.Second {
		t.Errorf("expected deadline around 30s, got %v", remaining)
	}
}

func TestInvoker_configDefaults(t *testing.T) {
	t.Parallel()

	i := NewInvoker(nil).(*invoker)

	// Check default reconnect config
	if i.config.Reconnect == nil {
		t.Fatal("expected reconnect config")
	}
	if !i.config.Reconnect.Enabled {
		t.Error("expected reconnect to be enabled by default")
	}
	if i.config.Reconnect.InitialDelayMs != 1000 {
		t.Errorf("expected initial delay 1000ms, got %d", i.config.Reconnect.InitialDelayMs)
	}

	// Check default retry config
	if i.config.Retry == nil {
		t.Fatal("expected retry config")
	}
	if !i.config.Retry.Enabled {
		t.Error("expected retry to be enabled by default")
	}
	if i.config.Retry.MaxAttempts != 3 {
		t.Errorf("expected max attempts 3, got %d", i.config.Retry.MaxAttempts)
	}
}

func TestInvoker_isConnectionErrorCases(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	testCases := []struct {
		err      error
		expected bool
	}{
		{errors.New("connection refused"), true},
		{errors.New("CONNECTION REFUSED"), true},
		{errors.New("some other error"), false},
		{nil, false},
		{errors.New("i/o timeout"), true},
		{errors.New("TLS handshake timeout"), true},
		{errors.New("network is unreachable"), true},
		{errors.New("broken pipe"), true},
		{errors.New("connection reset"), true},
		{errors.New("no such host"), true},
		{errors.New("transport is closing"), true},
		{errors.New("connection unavailable"), true},
	}

	for _, tc := range testCases {
		result := i.isConnectionError(tc.err)
		if result != tc.expected {
			t.Errorf("isConnectionError(%q) = %v, want %v", tc.err, result, tc.expected)
		}
	}
}

func TestInvoker_configWithCustomValues(t *testing.T) {
	t.Parallel()

	customReconnect := &ReconnectConfig{
		Enabled:           false,
		MaxAttempts:       5,
		InitialDelayMs:    2000,
		MaxDelayMs:        60000,
		BackoffMultiplier: 3.0,
		JitterFactor:      0.5,
	}

	customRetry := &RetryConfig{
		Enabled:           false,
		MaxAttempts:       1,
		InitialDelayMs:    500,
		MaxDelayMs:        10000,
		BackoffMultiplier: 1.5,
		JitterFactor:      0.1,
	}

	i := NewInvoker(&InvokerConfig{
		Address:        "example.com:9999",
		TimeoutSeconds: 60,
		Insecure:       false,
		Reconnect:      customReconnect,
		Retry:          customRetry,
	}).(*invoker)

	if i.config.Address != "example.com:9999" {
		t.Errorf("expected address example.com:9999, got %s", i.config.Address)
	}
	if i.config.TimeoutSeconds != 60 {
		t.Errorf("expected timeout 60s, got %d", i.config.TimeoutSeconds)
	}
	if i.config.Insecure {
		t.Error("expected insecure to be false")
	}
	if i.config.Reconnect != customReconnect {
		t.Error("expected custom reconnect config")
	}
	if i.config.Retry != customRetry {
		t.Error("expected custom retry config")
	}
}

func TestInvoker_InvokeOptionsWithAllFields(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	opts := InvokeOptions{
		IdempotencyKey: "test-key-123",
		Headers: map[string]string{
			"X-Request-ID": "abc-456",
			"X-Auth":       "token",
		},
		Timeout: 10 * time.Second,
	}

	// Just test that options are passed (will fail to connect)
	_, err := i.Invoke(context.Background(), "test", "{}", opts)
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestInvoker_CloseWithoutConnect(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	// Close without connect should not panic
	err := i.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInvoker_CloseTwice(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	// First close
	err := i.Close()
	if err != nil {
		t.Errorf("unexpected error on first close: %v", err)
	}

	// Second close should also be fine
	err = i.Close()
	if err != nil {
		t.Errorf("unexpected error on second close: %v", err)
	}
}

func TestInvoker_InvokeWithEmptyFunctionID(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	_, err := i.Invoke(context.Background(), "", "{}", InvokeOptions{})
	if err == nil {
		t.Error("expected error for empty function ID")
	}
}

func TestInvoker_StartJobWithEmptyFunctionID(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	_, err := i.StartJob(context.Background(), "", "{}", InvokeOptions{})
	if err == nil {
		t.Error("expected error for empty function ID")
	}
}

func TestInvoker_StartJobWithEmptyPayload(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	_, err := i.StartJob(context.Background(), "test_func", "", InvokeOptions{})
	if err == nil {
		t.Error("expected error for empty payload")
	}
}

func TestInvoker_StreamJobWithEmptyJobID(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	_, err := i.StreamJob(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty job ID")
	}
}

func TestInvoker_CancelJobWithEmptyJobID(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	err := i.CancelJob(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty job ID")
	}
}

func TestInvoker_ConnectWithEmptyAddress(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "",
		Insecure: true,
	}).(*invoker)

	err := i.Connect(context.Background())
	if err == nil {
		t.Error("expected error for empty address")
	}
}

func TestInvoker_calculateRetryDelayNoBackoff(t *testing.T) {
	t.Parallel()

	config := &RetryConfig{
		MaxAttempts:     3,
		InitialDelayMs:   10,
		BackoffMultiplier: 1.0, // No backoff - same delay
	}

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
		Retry:    config,
	}).(*invoker)

	// All delays should be the same when backoff multiplier is 1.0
	delay1 := i.calculateRetryDelay(0, config)
	delay2 := i.calculateRetryDelay(1, config)
	delay3 := i.calculateRetryDelay(2, config)

	if delay1 != delay2 || delay2 != delay3 {
		t.Errorf("expected same delay without backoff, got %v, %v, %v", delay1, delay2, delay3)
	}
}

func TestInvoker_InvokeWithInvalidJSONPayload(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}

	if err := i.SetSchema("test_func", schema); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}

	// Invalid JSON payload
	_, err := i.Invoke(context.Background(), "test_func", "not json", InvokeOptions{})
	if err == nil {
		t.Error("expected error for invalid JSON payload")
	}
}

func TestInvoker_scheduleReconnectIfNeededZeroMaxAttempts(t *testing.T) {
	t.Parallel()

	config := &ReconnectConfig{
		Enabled:        true,
		MaxAttempts:    0, // No max
		InitialDelayMs: 10,
	}

	i := NewInvoker(&InvokerConfig{
		Address:   "127.0.0.1:19090",
		Insecure:  true,
		Reconnect: config,
	}).(*invoker)

	// With MaxAttempts = 0, should always schedule
	i.reconnectAttempts = 100
	i.scheduleReconnectIfNeeded()
}

func TestInvoker_buildTLSConfigWithCAFile(t *testing.T) {
	t.Parallel()

	// Create a temporary CA file for testing
	tmpfile, err := os.CreateTemp("", "ca-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// For testing error path, use an invalid CA file
	tmpfile.WriteString("invalid ca content")
	tmpfile.Close()

	cfg := &InvokerConfig{
		Address:  "example.com:443",
		CAFile:   tmpfile.Name(),
		Insecure: false,
	}

	_, err = buildTLSConfig(cfg)
	// Should fail because the CA file content is not valid PEM
	if err == nil {
		t.Error("expected error for invalid CA file content")
	}
}

func TestInvoker_buildTLSConfigWithoutCAFile(t *testing.T) {
	t.Parallel()

	cfg := &InvokerConfig{
		Address:  "example.com:443",
		Insecure: false,
	}

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("buildTLSConfig failed: %v", err)
	}

	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}

	// System cert pool should be used
	if tlsCfg.RootCAs == nil {
		t.Error("expected RootCAs to be set")
	}
}

func TestInvoker_buildTLSConfigWithInvalidCAFile(t *testing.T) {
	t.Parallel()

	cfg := &InvokerConfig{
		Address:  "example.com:443",
		CAFile:   "/nonexistent/ca.pem",
		Insecure: false,
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error for nonexistent CA file")
	}
}

func TestInvoker_buildTLSConfigWithCertAndKey(t *testing.T) {
	t.Parallel()

	// Create temporary cert and key files
	certFile, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(certFile.Name())

	keyFile, err := os.CreateTemp("", "key-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(keyFile.Name())

	// Write dummy PEM content
	certFile.WriteString("-----BEGIN CERTIFICATE-----\ndummy\n-----END CERTIFICATE-----")
	keyFile.WriteString("-----BEGIN PRIVATE KEY-----\ndummy\n-----END PRIVATE KEY-----")

	certFile.Close()
	keyFile.Close()

	cfg := &InvokerConfig{
		Address:  "example.com:443",
		CertFile: certFile.Name(),
		KeyFile:  keyFile.Name(),
		Insecure: false,
	}

	_, err = buildTLSConfig(cfg)
	// This should fail because the cert/key are invalid
	if err == nil {
		t.Error("expected error for invalid cert/key pair")
	}
}

func TestInvoker_buildTLSConfigWithHostPort(t *testing.T) {
	t.Parallel()

	cfg := &InvokerConfig{
		Address:  "example.com:1234",
		Insecure: false,
	}

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("buildTLSConfig failed: %v", err)
	}

	if tlsCfg.ServerName != "example.com" {
		t.Errorf("expected ServerName to be example.com, got %s", tlsCfg.ServerName)
	}
}
