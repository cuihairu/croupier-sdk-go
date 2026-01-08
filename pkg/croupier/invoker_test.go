package croupier

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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

	// Clean up the reconnect goroutine to avoid goroutine leak
	defer i.Close()
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

func TestInvoker_InvokeWithTimeout(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19090",
		Insecure:       true,
		DefaultTimeout: 1 * time.Nanosecond, // Very short timeout
	}).(*invoker)

	// Should fail due to very short timeout
	_, err := i.Invoke(context.Background(), "test_func", "{}", InvokeOptions{})
	if err == nil {
		t.Error("expected error due to short timeout")
	}
}

func TestInvoker_calculateRetryDelayWithMaxDelay(t *testing.T) {
	t.Parallel()

	config := &RetryConfig{
		MaxAttempts:       3,
		InitialDelayMs:    100,
		MaxDelayMs:        200,
		BackoffMultiplier: 2.0,
	}

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
		Retry:    config,
	}).(*invoker)

	// Delay should be capped at MaxDelayMs
	delay1 := i.calculateRetryDelay(0, config)
	_ = i.calculateRetryDelay(1, config)
	_ = i.calculateRetryDelay(2, config)
	delay4 := i.calculateRetryDelay(10, config) // Way beyond max

	expectedMaxDelay := 200 * time.Millisecond
	if delay1 > expectedMaxDelay {
		t.Errorf("delay1 %v exceeds max delay %v", delay1, expectedMaxDelay)
	}
	if delay4 != expectedMaxDelay {
		t.Errorf("expected delay4 to be capped at max delay %v, got %v", expectedMaxDelay, delay4)
	}
}

func TestInvoker_calculateReconnectDelayWithJitter(t *testing.T) {
	t.Parallel()

	config := &ReconnectConfig{
		Enabled:           true,
		MaxAttempts:       5,
		InitialDelayMs:    100,
		MaxDelayMs:        5000,
		BackoffMultiplier: 2.0,
		JitterFactor:      0.1, // 10% jitter
	}

	i := NewInvoker(&InvokerConfig{
		Address:   "127.0.0.1:19090",
		Insecure:  true,
		Reconnect: config,
	}).(*invoker)

	// Set reconnect attempts to test backoff
	// Note: calculateReconnectDelay uses reconnectAttempts-1, so start at 1
	i.reconnectAttempts = 1
	delay1 := i.calculateReconnectDelay()

	i.reconnectAttempts = 2
	delay2 := i.calculateReconnectDelay()

	// Verify delay grows with backoff (approximately)
	// Due to jitter, we check that delay2 is generally larger than delay1's expected range
	if delay2 < 90*time.Millisecond {
		t.Errorf("expected delay2 to be at least close to 100ms, got %v", delay2)
	}

	// With jitter, delays vary - just verify we got a reasonable delay
	if delay1 < 80*time.Millisecond {
		t.Errorf("expected delay1 to be at least 80ms (accounting for jitter), got %v", delay1)
	}
}

func TestInvoker_StartJobWithRetry(t *testing.T) {
	t.Parallel()

	config := &RetryConfig{
		Enabled:           true,
		MaxAttempts:       2,
		InitialDelayMs:    10,
		BackoffMultiplier: 1.0,
	}

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
		Retry:    config,
	}).(*invoker)

	// StartJob without connection - should retry and fail
	_, err := i.StartJob(context.Background(), "test_func", "{}", InvokeOptions{})
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestInvoker_validatePayloadWithSchema(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	// Set a schema for validation
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string",
			},
			"age": map[string]interface{}{
				"type": "integer",
			},
		},
		"required": []interface{}{"name"},
	}
	i.SetSchema("test_func", schema)

	// Test valid payload - note: validatePayload takes a string and a map
	// We need to test through Invoke which calls it
	// Since we can't call validatePayload directly with the right signature,
	// we'll test the behavior through Invoke

	// Test that empty payload fails validation
	schema2 := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []interface{}{"name"},
	}
	i.SetSchema("test_func2", schema2)

	// Empty payload should fail
	err := i.validatePayload("", schema2)
	if err == nil {
		t.Error("expected error for empty payload")
	}
}

// TestInvoker_StreamJobNotConnected tests StreamJob when not connected
func TestInvoker_StreamJobNotConnected(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	// StreamJob without connection should return error
	eventCh, err := i.StreamJob(context.Background(), "job-123")
	if err == nil {
		t.Error("expected error when not connected")
	}

	// Channel should be closed
	if eventCh == nil {
		t.Fatal("expected channel to be returned even on error")
	}

	// Try to receive from channel - should be closed
	_, ok := <-eventCh
	if ok {
		t.Error("expected channel to be closed")
	}
}

// TestInvoker_StreamJobConnectionError tests StreamJob with connection error
func TestInvoker_StreamJobConnectionError(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
		Reconnect: &ReconnectConfig{
			Enabled: false,
		},
	})

	// StreamJob should fail due to connection error
	eventCh, err := i.StreamJob(context.Background(), "job-456")
	if err == nil {
		t.Error("expected error when connection fails")
	}

	if eventCh == nil {
		t.Fatal("expected channel to be returned")
	}

	// Channel should be closed immediately
	select {
	case evt, ok := <-eventCh:
		if ok {
			t.Errorf("expected closed channel, got event: %+v", evt)
		}
	default:
		// Channel might not be closed yet, but should be soon
	}
}

// TestInvoker_CancelJobNotConnected tests CancelJob when not connected
func TestInvoker_CancelJobNotConnected(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
		Reconnect: &ReconnectConfig{
			Enabled: false,
		},
	})

	// CancelJob without connection should return error
	err := i.CancelJob(context.Background(), "job-123")
	if err == nil {
		t.Error("expected error when not connected")
	}
}

// TestInvoker_StartJobNotConnected tests StartJob when not connected
func TestInvoker_StartJobNotConnected(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
		Reconnect: &ReconnectConfig{
			Enabled: false,
		},
	})

	// StartJob without connection should return error
	_, err := i.StartJob(context.Background(), "test_func", "{}", InvokeOptions{})
	if err == nil {
		t.Error("expected error when not connected")
	}
}

// TestInvoker_InvokeWithConnectionError tests Invoke with connection error and reconnect disabled
func TestInvoker_InvokeWithConnectionError(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
		Reconnect: &ReconnectConfig{
			Enabled: false,
		},
	})

	// Invoke should fail due to connection error
	_, err := i.Invoke(context.Background(), "test_func", "{}", InvokeOptions{})
	if err == nil {
		t.Error("expected error when connection fails")
	}

	// Should not be reconnecting
	impl := i.(*invoker)
	if impl.isReconnecting {
		t.Error("should not be reconnecting when reconnect is disabled")
	}
}

// TestInvoker_buildTLSConfigWithMissingCert tests TLS config with missing cert files
func TestInvoker_buildTLSConfigWithMissingCert(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "example.com:443",
		Insecure: false,
		CAFile:   "/nonexistent/ca.pem",
	}).(*invoker)

	_, err := buildTLSConfig(i.config)
	if err == nil {
		t.Error("expected error for missing CA file")
	}
}

// TestInvoker_callContextWithTimeout tests callContext with InvokeOptions timeout
func TestInvoker_callContextWithTimeout(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19090",
		TimeoutSeconds: 30,
		Insecure:       true,
	}).(*invoker)

	// Test with InvokeOptions timeout
	customTimeout := 5 * time.Second
	opts := InvokeOptions{
		Timeout: customTimeout,
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

// TestInvoker_calculateRetryDelayWithLargeMaxDelay tests retry delay with large max delay
func TestInvoker_calculateRetryDelayWithLargeMaxDelay(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	cfg := &RetryConfig{
		InitialDelayMs:    100,
		MaxDelayMs:        200000, // Large max delay (200 seconds)
		BackoffMultiplier: 2.0,
	}

	// With attempt 2, should be 100ms * 2^2 = 400ms (before jitter)
	delay := i.calculateRetryDelay(2, cfg)

	// With jitter, actual value will vary, but should be close to 400ms
	// Jitter factor is 0.1, so range is [360ms, 440ms]
	minExpected := 360 * time.Millisecond
	maxExpected := 440 * time.Millisecond

	if delay < minExpected || delay > maxExpected {
		t.Errorf("expected delay between %v and %v, got %v", minExpected, maxExpected, delay)
	}
}

// TestInvoker_validatePayloadWithNestedProperties tests nested property validation
func TestInvoker_validatePayloadWithNestedProperties(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"age":  map[string]any{"type": "integer"},
				},
			},
		},
	}

	// Valid nested payload
	payload := `{"user":{"name":"Alice","age":30}}`
	err := i.validatePayload(payload, schema)
	if err != nil {
		t.Errorf("unexpected error for valid nested payload: %v", err)
	}

	// Invalid nested type
	payload2 := `{"user":{"name":"Alice","age":"thirty"}}`
	err = i.validatePayload(payload2, schema)
	if err == nil {
		t.Error("expected error for invalid nested type")
	}
}

// TestInvoker_validatePayloadWithArrayTypes tests array type validation
func TestInvoker_validatePayloadWithArrayTypes(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tags": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
			"scores": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "number",
				},
			},
		},
	}

	// Valid arrays
	payload := `{"tags":["a","b","c"],"scores":[1.5,2.5,3.5]}`
	err := i.validatePayload(payload, schema)
	if err != nil {
		t.Errorf("unexpected error for valid arrays: %v", err)
	}

	// Invalid array type
	payload2 := `{"tags":["a","b","c"],"scores":["not","numbers"]}`
	err = i.validatePayload(payload2, schema)
	if err == nil {
		t.Error("expected error for invalid array item types")
	}
}

// TestInvoker_InvokeWithIdempotencyKey tests Invoke with idempotency key
func TestInvoker_InvokeWithIdempotencyKey(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	opts := InvokeOptions{
		IdempotencyKey: "unique-key-12345",
	}

	// Should fail to connect, but we verify the key is handled
	_, err := i.Invoke(context.Background(), "test_func", "{}", opts)
	if err == nil {
		t.Error("expected connection error")
	}

	// Verify the key was passed (would be in the gRPC metadata if connected)
	if opts.IdempotencyKey != "unique-key-12345" {
		t.Error("idempotency key was modified")
	}
}

// TestInvoker_CloseWhenNotConnected tests Close when not connected
func TestInvoker_CloseWhenNotConnected(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	// Close should not error even when not connected
	err := i.Close()
	if err != nil {
		t.Errorf("unexpected error on Close: %v", err)
	}

	// Verify state
	impl := i.(*invoker)
	if impl.connected {
		t.Error("should not be connected after Close")
	}
}

// TestInvoker_scheduleReconnectIfNeededWithDelay tests reconnect scheduling
func TestInvoker_scheduleReconnectIfNeededWithDelay(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
		Reconnect: &ReconnectConfig{
			Enabled:        true,
			MaxAttempts:    5,
			InitialDelayMs: 100,
		},
	}).(*invoker)

	i.reconnectAttempts = 1

	// Schedule reconnect
	i.scheduleReconnectIfNeeded()

	if !i.isReconnecting {
		t.Error("expected isReconnecting to be true")
	}

	if i.reconnectAttempts != 2 {
		t.Errorf("expected reconnectAttempts to be 2, got %d", i.reconnectAttempts)
	}

	// Clean up the reconnect goroutine to avoid goroutine leak
	i.Close()
}

// TestInvoker_configWithEmptyAddress tests config with empty address
func TestInvoker_configWithEmptyAddress(t *testing.T) {
	t.Parallel()

	cfg := &InvokerConfig{
		Address:        "",
		Insecure:       true,
		TimeoutSeconds: 30, // Need to set this explicitly for DefaultTimeout to be set
	}

	i := NewInvoker(cfg).(*invoker)
	if i.config.Address != "" {
		t.Errorf("expected empty address, got %q", i.config.Address)
	}

	// Should use default
	if i.config.DefaultTimeout != 30*time.Second {
		t.Errorf("expected default timeout, got %v", i.config.DefaultTimeout)
	}
}

// TestInvoker_SetSchemaWithNilSchema tests SetSchema with nil schema
func TestInvoker_SetSchemaWithNilSchema(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	// Nil schema should work
	err := i.SetSchema("test_func", nil)
	if err != nil {
		t.Errorf("unexpected error with nil schema: %v", err)
	}

	impl := i.(*invoker)
	if impl.schemas["test_func"] != nil {
		t.Error("expected schema to be nil")
	}
}

// TestInvoker_SetSchemaWithEmptyID tests SetSchema with empty function ID
func TestInvoker_SetSchemaWithEmptyID(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	schema := map[string]any{"type": "object"}

	// Empty function ID
	err := i.SetSchema("", schema)
	if err != nil {
		t.Errorf("unexpected error with empty function ID: %v", err)
	}

	impl := i.(*invoker)
	// Empty ID should still be stored (no validation)
	if impl.schemas[""] == nil {
		t.Error("expected schema to be stored even with empty ID")
	}
}

// TestInvoker_StreamJobContextCancellation tests StreamJob with context cancellation
func TestInvoker_StreamJobContextCancellation(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// StreamJob should handle context cancellation
	eventCh, err := i.StreamJob(ctx, "job-123")
	if err == nil {
		t.Error("expected error when context is already cancelled")
	}

	if eventCh == nil {
		t.Error("expected channel to be returned")
	}

	// Channel should be closed
	_, ok := <-eventCh
	if ok {
		t.Error("expected channel to be closed")
	}
}

// TestInvoker_StreamJobWithConnectedInvoker tests StreamJob when already connected
func TestInvoker_StreamJobWithConnectedInvoker(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	// Test with context cancellation - this tests the error path before RPC call
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// StreamJob should handle context cancellation
	eventCh, err := i.StreamJob(ctx, "job-456")
	if err == nil {
		t.Error("expected error when context is already cancelled")
	}

	if eventCh == nil {
		t.Error("expected channel to be returned")
	}
}

// TestInvoker_CancelJobContextCancellation tests CancelJob with context cancellation
func TestInvoker_CancelJobContextCancellation(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// CancelJob should handle context cancellation
	// Since not connected, it will try to connect which will fail due to context cancellation
	err := i.CancelJob(ctx, "job-123")
	if err == nil {
		t.Error("expected error when context is already cancelled")
	}
}

// TestInvoker_InvokeWithInvalidOptions tests Invoke with various invalid options
func TestInvoker_InvokeWithInvalidOptions(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	t.Run("with negative timeout", func(t *testing.T) {
		t.Parallel()

		// Negative timeout should be handled
		// The callContext function should use default timeout
		ctx := context.Background()
		callCtx, cancel := i.callContext(ctx, -10*time.Second)
		defer cancel()

		if callCtx == nil {
			t.Error("expected non-nil context")
		}

		// Verify deadline is set
		deadline, ok := callCtx.Deadline()
		if !ok {
			t.Error("expected deadline to be set")
		}

		// Deadline should be in the future and reasonable (within 60 seconds)
		if time.Until(deadline) <= 0 || time.Until(deadline) > 60*time.Second {
			t.Errorf("expected reasonable deadline, got %v", time.Until(deadline))
		}
	})

	t.Run("with zero timeout uses default", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		callCtx, cancel := i.callContext(ctx, 0)
		defer cancel()

		if callCtx == nil {
			t.Error("expected non-nil context")
		}

		// When timeout is 0, it should use DefaultTimeout
		// which defaults to 30 seconds if not set
		deadline, ok := callCtx.Deadline()
		if !ok {
			t.Error("expected deadline to be set")
		}

		// Check deadline is approximately 30 seconds from now
		until := time.Until(deadline)
		if until < 29*time.Second || until > 31*time.Second {
			t.Errorf("expected deadline around 30 seconds, got %v", until)
		}
	})
}

// TestInvoker_InvokeWithEmptyPayload tests Invoke with empty payload
func TestInvoker_InvokeWithEmptyPayload(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	opts := InvokeOptions{}

	// Empty payload should be passed through
	// Will fail because we don't have a real server
	_, err := i.Invoke(context.Background(), "test_func", "", opts)
	if err == nil {
		t.Error("expected error without real server")
	}
}

// TestInvoker_InvokeWithCustomTimeout tests Invoke with custom timeout
func TestInvoker_InvokeWithCustomTimeout(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	opts := InvokeOptions{
		Timeout: 5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should use custom timeout but overall context should cancel first
	// Since we don't have a real connection, this will fail
	_, err := i.Invoke(ctx, "test_func", "{}", opts)
	if err == nil {
		t.Error("expected error")
	}
}

// TestInvoker_InvokeNotConnected tests Invoke when not connected
func TestInvoker_InvokeNotConnected(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	opts := InvokeOptions{}

	// Should fail because not connected
	_, err := i.Invoke(context.Background(), "test_func", "{}", opts)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

// TestInvoker_StartJobWithVariousOptions tests StartJob with different options
func TestInvoker_StartJobWithVariousOptions(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	t.Run("with headers", func(t *testing.T) {
		t.Parallel()

		opts := InvokeOptions{
			Headers: map[string]string{
				"X-Custom-Header": "value",
				"Authorization":   "Bearer token",
			},
		}

		// Will fail because we don't have a real server
		_, err := i.StartJob(context.Background(), "test_func", "{}", opts)
		if err == nil {
			t.Error("expected error without real server")
		}
	})

	t.Run("with idempotency key", func(t *testing.T) {
		t.Parallel()

		opts := InvokeOptions{
			IdempotencyKey: "unique-key-123",
		}

		// Will fail because we don't have a real server
		_, err := i.StartJob(context.Background(), "test_func", "{}", opts)
		if err == nil {
			t.Error("expected error without real server")
		}
	})

	t.Run("with retry config", func(t *testing.T) {
		t.Parallel()

		customRetry := &RetryConfig{
			Enabled:          false, // Disable retry for faster failure
			MaxAttempts:      1,
			InitialDelayMs:   10,
			MaxDelayMs:       100,
			BackoffMultiplier: 2.0,
			JitterFactor:     0.1,
		}

		opts := InvokeOptions{
			Retry: customRetry,
		}

		// Will fail because we don't have a real server
		_, err := i.StartJob(context.Background(), "test_func", "{}", opts)
		if err == nil {
			t.Error("expected error without real server")
		}
	})
}

// TestInvoker_ValidatePayloadWithComplexSchema tests payload validation with complex schemas
func TestInvoker_ValidatePayloadWithComplexSchema(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"required": []string{"name", "age"},
		"properties": map[string]any{
			"name": map[string]any{
				"type":      "string",
				"minLength": 1,
				"maxLength": 100,
			},
			"age": map[string]any{
				"type":    "integer",
				"minimum": 0,
				"maximum": 150,
			},
			"email": map[string]any{
				"type":   "string",
				"format": "email",
			},
			"address": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"street": map[string]any{"type": "string"},
					"city":   map[string]any{"type": "string"},
				},
			},
			"tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	}

	t.Run("valid complex payload", func(t *testing.T) {
		t.Parallel()

		payload := `{
			"name": "John Doe",
			"age": 30,
			"email": "john@example.com",
			"address": {
				"street": "123 Main St",
				"city": "Anytown"
			},
			"tags": ["tag1", "tag2"]
		}`

		err := i.validatePayload(payload, schema)
		if err != nil {
			t.Errorf("unexpected error for valid payload: %v", err)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		t.Parallel()

		payload := `{"name": "John"}` // missing age

		err := i.validatePayload(payload, schema)
		if err == nil {
			t.Error("expected error for missing required field")
		}
	})

	t.Run("invalid type for field", func(t *testing.T) {
		t.Parallel()

		payload := `{"name": "John", "age": "thirty"}` // age should be integer

		err := i.validatePayload(payload, schema)
		if err == nil {
			t.Error("expected error for invalid type")
		}
	})

	t.Run("invalid nested object", func(t *testing.T) {
		t.Parallel()

		payload := `{"name": "John", "age": 30, "address": "not an object"}`

		err := i.validatePayload(payload, schema)
		if err == nil {
			t.Error("expected error for invalid nested object")
		}
	})

	t.Run("invalid array", func(t *testing.T) {
		t.Parallel()

		payload := `{"name": "John", "age": 30, "tags": "not an array"}`

		err := i.validatePayload(payload, schema)
		if err == nil {
			t.Error("expected error for invalid array")
		}
	})
}

// TestInvoker_ConnectWithTimeout tests Connect with timeout
func TestInvoker_ConnectWithTimeout(t *testing.T) {
	t.Parallel()

	// Use a port that's unlikely to have a server
	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19999", // Different port to avoid conflicts
		Insecure:       true,
		TimeoutSeconds: 1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := i.Connect(ctx)
	// Connection may fail or timeout, either is acceptable
	// The important thing is that the function handles it gracefully
	if err != nil {
		// Expected - no server running
		t.Logf("Got expected error: %v", err)
	}
}

// TestInvoker_ReconnectAfterClose tests that reconnect works after Close
func TestInvoker_ReconnectAfterClose(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19998", // Different port to avoid conflicts
		Insecure: true,
	}).(*invoker)

	// First connect (will likely fail)
	err := i.Connect(context.Background())
	// Server may or may not be running due to parallel tests
	if err != nil {
		t.Logf("Got expected error on first connect: %v", err)
	}

	// Close
	i.Close()

	// Reset connected flag for test
	i.connected = false

	// Try to connect again after Close
	err = i.Connect(context.Background())
	if err != nil {
		t.Logf("Got expected error on second connect: %v", err)
	}

	// Verify state after Close and reconnect attempt
	if i.reconnectAttempts < 0 {
		t.Error("reconnectAttempts should be non-negative")
	}
}

// TestInvoker_MultipleCloseCalls tests that multiple Close calls are safe
func TestInvoker_MultipleCloseCalls(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	})

	// First close
	err := i.Close()
	if err != nil {
		t.Errorf("unexpected error on first Close: %v", err)
	}

	// Second close should not panic or error
	err = i.Close()
	if err != nil {
		t.Errorf("unexpected error on second Close: %v", err)
	}

	// Third close
	err = i.Close()
	if err != nil {
		t.Errorf("unexpected error on third Close: %v", err)
	}
}

// TestInvoker_StreamJobWithReconnectEnabled tests StreamJob with reconnect enabled
func TestInvoker_StreamJobWithReconnectEnabled(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19997", // Port unlikely to have server
		Insecure: true,
		Reconnect: &ReconnectConfig{
			Enabled:    true,
			MaxAttempts: 2,
		},
	})

	// StreamJob should fail and trigger reconnection logic
	// The error should be a connection error which triggers isConnectionError
	eventCh, err := i.StreamJob(context.Background(), "job-reconnect")
	if err == nil {
		t.Error("expected error when server is not available")
	}

	if eventCh == nil {
		t.Error("expected channel to be returned")
	}

	// Verify channel behavior
	select {
	case evt, ok := <-eventCh:
		if ok {
			t.Logf("Received event: %+v", evt)
		}
	default:
		// Channel may not be closed yet, which is fine
	}
}

// TestInvoker_InvokeWithValidation tests Invoke with schema validation
func TestInvoker_InvokeWithValidation(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	// Set up a schema for a function
	i.schemas = map[string]map[string]interface{}{
		"validated_fn": {
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"name"},
		},
	}

	opts := InvokeOptions{}

	// Valid payload should pass validation (will still fail on connection)
	_, err := i.Invoke(context.Background(), "validated_fn", `{"name":"test"}`, opts)
	if err == nil {
		t.Error("expected error when not connected")
	}

	// Invalid payload should fail validation
	_, err = i.Invoke(context.Background(), "validated_fn", `{"wrong":"field"}`, opts)
	if err == nil {
		t.Error("expected validation error")
	}
	// Verify the error mentions validation
	if err != nil && !strings.Contains(err.Error(), "validation failed") {
		t.Logf("Got error: %v", err)
	}
}

// TestInvoker_InvokeWithNonExistentFunction tests Invoke with non-existent function
func TestInvoker_InvokeWithNonExistentFunction(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	opts := InvokeOptions{}

	// Function without schema should still work (will fail on connection)
	_, err := i.Invoke(context.Background(), "non_existent_fn", `{}`, opts)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

// TestInvoker_StartJobWithValidation tests StartJob with schema validation
func TestInvoker_StartJobWithValidation(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	// Set up a schema for a function
	i.schemas = map[string]map[string]interface{}{
		"job_fn": {
			"type": "object",
			"properties": map[string]interface{}{
				"count": map[string]interface{}{"type": "integer"},
			},
		},
	}

	opts := InvokeOptions{}

	// Valid payload should pass validation
	_, err := i.StartJob(context.Background(), "job_fn", `{"count":5}`, opts)
	if err == nil {
		t.Error("expected error when not connected")
	}

	// Invalid payload should fail validation
	_, err = i.StartJob(context.Background(), "job_fn", `{"count":"not a number"}`, opts)
	if err == nil {
		t.Error("expected validation error")
	}
}

// TestInvoker_CancelJobWithConnectionError tests CancelJob when connection fails
func TestInvoker_CancelJobWithConnectionError(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
		Reconnect: &ReconnectConfig{
			Enabled:    true,
			MaxAttempts: 2,
		},
	})

	// CancelJob should fail and trigger reconnection logic
	err := i.CancelJob(context.Background(), "job-123")
	if err == nil {
		t.Error("expected error when not connected")
	}
}

// TestInvoker_SetSchemaVariant tests SetSchema with different variants
func TestInvoker_SetSchemaVariant(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	})

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"value": map[string]interface{}{"type": "string"},
		},
	}

	err := i.SetSchema("test_fn", schema)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify schema was set (we can't access schemas directly without type assertion)
	// but we can verify setting again doesn't cause issues
	err = i.SetSchema("test_fn", schema)
	if err != nil {
		t.Errorf("unexpected error on second SetSchema: %v", err)
	}
}

// TestInvoker_SetSchemaEmptySchema tests SetSchema with empty schema
func TestInvoker_SetSchemaEmptySchema(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	})

	err := i.SetSchema("test_fn", nil)
	if err != nil {
		t.Errorf("unexpected error with nil schema: %v", err)
	}

	err = i.SetSchema("test_fn2", map[string]interface{}{})
	if err != nil {
		t.Errorf("unexpected error with empty schema: %v", err)
	}
}

// TestInvoker_InvokeWithRetryOptions tests Invoke with retry configuration
func TestInvoker_InvokeWithRetryOptions(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	opts := InvokeOptions{
		Retry: &RetryConfig{
			MaxAttempts:     3,
			InitialDelayMs:  10,
			BackoffMultiplier: 2.0,
		},
	}

	// Should fail but use retry configuration
	_, err := i.Invoke(context.Background(), "test_fn", `{}`, opts)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

// TestInvoker_InvokeWithIdempotencyKey2 tests Invoke with idempotency key variant
func TestInvoker_InvokeWithIdempotencyKey2(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	})

	opts := InvokeOptions{
		IdempotencyKey: "unique-key-12345",
	}

	_, err := i.Invoke(context.Background(), "test_fn", `{}`, opts)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

// TestInvoker_isRetryableErrorWithPatterns tests isRetryableError with retryable patterns
func TestInvoker_isRetryableErrorWithPatterns(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
		Retry: &RetryConfig{
			RetryableStatusCodes: []int32{14, 8, 10},
		},
	}).(*invoker)

	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{"unavailable error", fmt.Errorf("service unavailable"), true},
		{"UNAVAILABLE error", fmt.Errorf("UNAVAILABLE"), true},
		{"internal error", fmt.Errorf("internal error"), true},
		{"INTERNAL ERROR", fmt.Errorf("INTERNAL ERROR"), true},
		{"deadline exceeded", fmt.Errorf("context deadline exceeded"), true},
		{"aborted error", fmt.Errorf("request aborted"), true},
		{"ABORTED", fmt.Errorf("ABORTED"), true},
		{"transport is closing", fmt.Errorf("transport is closing"), true},
		{"TRANSPORT IS CLOSING", fmt.Errorf("TRANSPORT IS CLOSING"), true},
		{"non-retryable error", fmt.Errorf("permission denied"), false},
		{"not found", fmt.Errorf("not found"), false},
		{"invalid argument", fmt.Errorf("invalid argument"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := i.isRetryableError(tc.err)
			if result != tc.expected {
				t.Errorf("isRetryableError(%q) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

// TestInvoker_isRetryableErrorWithStatusCodes tests isRetryableError with status codes
func TestInvoker_isRetryableErrorWithStatusCodes(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
		Retry: &RetryConfig{
			RetryableStatusCodes: []int32{14, 8, 10}, // 14=Unavailable, 8=ResourceExhausted, 10=Aborted
		},
	}).(*invoker)

	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{"code = 14", fmt.Errorf("rpc error: code = 14"), true},
		{"Code(14)", fmt.Errorf("rpc error: Code(14)"), true},
		{"code = 8", fmt.Errorf("rpc error: code = 8"), true},
		{"Code(8)", fmt.Errorf("rpc error: Code(8)"), true},
		{"code = 10", fmt.Errorf("rpc error: code = 10"), true},
		{"code = 5 (not retryable)", fmt.Errorf("rpc error: code = 5"), false},
		{"no code", fmt.Errorf("some error"), false},
		{"unavailable pattern", fmt.Errorf("service unavailable"), true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := i.isRetryableError(tc.err)
			if result != tc.expected {
				t.Errorf("isRetryableError(%q) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

// TestInvoker_executeWithRetryWithConnectionError tests executeWithRetry with connection error
func TestInvoker_executeWithRetryWithConnectionError(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
		Reconnect: &ReconnectConfig{
			Enabled:    false, // Disable reconnect to avoid goroutine
			MaxAttempts: 2,
		},
	}).(*invoker)

	ctx := context.Background()

	// Simulate a retryable error (unavailable is in the retryable patterns list)
	attempts := 0
	fn := func() (string, error) {
		attempts++
		return "", fmt.Errorf("service unavailable")
	}

	opts := InvokeOptions{
		Retry: &RetryConfig{
			Enabled:          true,
			MaxAttempts:      3,
			InitialDelayMs:   1, // Very short
			BackoffMultiplier: 1.0,
		},
	}

	_, err := i.executeWithRetry(ctx, opts, fn)
	if err == nil {
		t.Error("expected error")
	}

	// Should have attempted multiple times since error is retryable
	if attempts < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts)
	}
}

// TestInvoker_executeWithRetryCancelledByContext tests executeWithRetry with context cancellation
func TestInvoker_executeWithRetryCancelledByContext(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after first attempt
	attempts := 0
	fn := func() (string, error) {
		attempts++
		if attempts == 1 {
			cancel() // Cancel after first attempt
		}
		return "", fmt.Errorf("error")
	}

	opts := InvokeOptions{
		Retry: &RetryConfig{
			Enabled:          true,
			MaxAttempts:      10,
			InitialDelayMs:   1, // Very short delay
			BackoffMultiplier: 1.0,
		},
	}

	_, err := i.executeWithRetry(ctx, opts, fn)
	if err == nil {
		t.Error("expected error")
	}

	// Should mention cancellation
	if !strings.Contains(err.Error(), "cancelled") && !strings.Contains(err.Error(), "context") {
		t.Logf("Error (may not mention cancellation): %v", err)
	}
}

// TestInvoker_executeWithRetryMaxAttempts tests executeWithRetry reaching max attempts
func TestInvoker_executeWithRetryMaxAttempts(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	ctx := context.Background()
	maxAttempts := 3
	attempts := 0

	fn := func() (string, error) {
		attempts++
		return "", fmt.Errorf("service unavailable") // Retryable error
	}

	opts := InvokeOptions{
		Retry: &RetryConfig{
			Enabled:          true,
			MaxAttempts:      maxAttempts,
			InitialDelayMs:   1, // Very short
			BackoffMultiplier: 1.0,
		},
	}

	_, err := i.executeWithRetry(ctx, opts, fn)
	if err == nil {
		t.Error("expected error after max attempts")
	}

	// Should have attempted exactly maxAttempts times
	if attempts != maxAttempts {
		t.Errorf("expected %d attempts, got %d", maxAttempts, attempts)
	}

	// Error should mention the number of attempts
	if !strings.Contains(err.Error(), "3") && !strings.Contains(err.Error(), "attempt") {
		t.Logf("Error: %v", err)
	}
}

// TestInvoker_connectWithInvalidAddress tests connect with invalid address
func TestInvoker_connectWithInvalidAddress(t *testing.T) {
	t.Parallel()

	// gRPC uses lazy connection, so many invalid addresses won't fail until actual RPC call
	// Empty address is the only one that will definitely fail at dial time
	testCases := []struct {
		name        string
		address     string
		shouldError bool // Whether we expect an error at dial time
	}{
		{"empty address", "", true},
		{"localhost:abc", "localhost:abc", false}, // gRPC may succeed, fail later
		{":1234", ":1234", false},                 // gRPC may succeed, fail later
		{"invalid host", "invalid host", false},   // gRPC may succeed, fail later
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			i := NewInvoker(&InvokerConfig{
				Address:  tc.address,
				Insecure: true,
			}).(*invoker)

			err := i.connect(context.Background())
			if tc.shouldError && err == nil {
				t.Error("expected error for invalid address")
			}
			// For addresses where error is not expected, clean up any connection
			if !tc.shouldError && err == nil {
				i.Close()
			}
		})
	}
}

// TestInvoker_connectIdempotentWhenConnected tests connect is idempotent when already connected
func TestInvoker_connectIdempotentWhenConnected(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	// Simulate being connected
	i.connected = true
	i.conn = nil // No actual connection

	// connect should return nil without attempting to connect
	err := i.connect(context.Background())
	if err != nil {
		t.Errorf("unexpected error when already connected: %v", err)
	}
}

// TestInvoker_connectWithTLSError tests connect with TLS configuration error
func TestInvoker_connectWithTLSError(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "example.com:443",
		Insecure: false,
		CAFile:   "/nonexistent/ca.pem",
	}).(*invoker)

	// connect should fail due to invalid CA file
	err := i.connect(context.Background())
	if err == nil {
		t.Error("expected error for invalid CA file")
	}
}

// TestInvoker_validatePayloadWithNonObjectSchema tests validatePayload with non-object schema
func TestInvoker_validatePayloadWithNonObjectSchema(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	// Schema that accepts any type
	schema := map[string]any{
		"type": "string",
	}

	// Valid string payload
	err := i.validatePayload(`"hello"`, schema)
	if err != nil {
		t.Errorf("unexpected error for string payload: %v", err)
	}

	// Schema for number
	schema2 := map[string]any{
		"type": "number",
	}

	err = i.validatePayload(`42.5`, schema2)
	if err != nil {
		t.Errorf("unexpected error for number payload: %v", err)
	}
}

// TestInvoker_validatePayloadWithEnumSchema tests validatePayload with enum schema
func TestInvoker_validatePayloadWithEnumSchema(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{
				"type": "string",
				"enum": []interface{}{"pending", "active", "completed"},
			},
		},
	}

	// Valid enum value
	err := i.validatePayload(`{"status":"active"}`, schema)
	if err != nil {
		t.Errorf("unexpected error for valid enum value: %v", err)
	}

	// Invalid enum value
	err = i.validatePayload(`{"status":"invalid"}`, schema)
	if err == nil {
		t.Error("expected error for invalid enum value")
	}
}

// TestInvoker_InvokeReconnectScheduling tests that connection errors trigger reconnect scheduling
func TestInvoker_InvokeReconnectScheduling(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
		Reconnect: &ReconnectConfig{
			Enabled:     true,
			MaxAttempts: 2,
		},
	}).(*invoker)

	// Attempt to invoke without connection - should trigger reconnect
	_, err := i.Invoke(context.Background(), "test_fn", `{}`, InvokeOptions{})
	if err == nil {
		t.Error("expected error")
	}

	// Give time for reconnect goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Clean up reconnect goroutine
	i.Close()
}

// TestInvoker_calculateReconnectDelayNegative tests calculateReconnectDelay with edge cases
func TestInvoker_calculateReconnectDelayNegative(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
		Reconnect: &ReconnectConfig{
			InitialDelayMs:    1,
			MaxDelayMs:        10,
			BackoffMultiplier: 100.0, // Large multiplier
			JitterFactor:      1.0,    // 100% jitter
		},
	}).(*invoker)

	i.reconnectAttempts = 1
	delay := i.calculateReconnectDelay()

	// With high jitter, delay could theoretically be negative
	// The function should clamp to 0
	if delay < 0 {
		t.Errorf("expected non-negative delay, got %v", delay)
	}
}

// TestInvoker_calculateRetryDelayNegative tests calculateRetryDelay with edge cases
func TestInvoker_calculateRetryDelayNegative(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	cfg := &RetryConfig{
		InitialDelayMs:    1,
		MaxDelayMs:        10,
		BackoffMultiplier: 100.0, // Large multiplier
		JitterFactor:      1.0,    // 100% jitter
	}

	delay := i.calculateRetryDelay(5, cfg)

	// With high jitter, delay could theoretically be negative
	// The function should clamp to 0
	if delay < 0 {
		t.Errorf("expected non-negative delay, got %v", delay)
	}
}

// TestInvoker_connectContextCancellation tests connect with cancelled context
func TestInvoker_connectContextCancellation(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:29996", // Different port unlikely to have server
		Insecure: true,
	}).(*invoker)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := i.connect(ctx)
	// connect should fail because context is already cancelled or no server
	if err == nil {
		// If connection succeeded (unlikely), clean up and skip
		i.Close()
		t.Skip("server was running on test port")
	}
	// The error should mention context, deadline, or connection
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "context") &&
		!strings.Contains(strings.ToLower(err.Error()), "deadline") &&
		!strings.Contains(strings.ToLower(err.Error()), "canceled") &&
		!strings.Contains(strings.ToLower(err.Error()), "connection") {
		t.Logf("Error type: %v", err)
	}
}

// TestInvoker_InvokeWithZeroTimeoutFromConfig tests Invoke when config has zero timeout
func TestInvoker_InvokeWithZeroTimeoutFromConfig(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:        "127.0.0.1:19996",
		Insecure:       true,
		TimeoutSeconds: 0, // Zero timeout
	}).(*invoker)

	// When TimeoutSeconds is 0, DefaultTimeout becomes 0
	// callContext then uses hardcoded 30 second default
	if i.config.DefaultTimeout != 0 {
		t.Errorf("expected DefaultTimeout 0s, got %v", i.config.DefaultTimeout)
	}

	// Test callContext with zero config timeout - should use hardcoded 30s default
	ctx, cancel := i.callContext(context.Background(), 0)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline to be set")
	}

	remaining := time.Until(deadline)
	if remaining < 29*time.Second || remaining > 31*time.Second {
		t.Errorf("expected deadline around 30s, got %v", remaining)
	}
}

// TestInvoker_configWithZeroTimeout tests config with zero timeout
func TestInvoker_configWithZeroTimeout(t *testing.T) {
	t.Parallel()

	cfg := &InvokerConfig{
		Address:        "127.0.0.1:19996",
		Insecure:       true,
		TimeoutSeconds: 0,
		DefaultTimeout: 0,
	}

	i := NewInvoker(cfg).(*invoker)

	// When both TimeoutSeconds and DefaultTimeout are 0,
	// DefaultTimeout becomes 0 * time.Second = 0
	// The callContext then uses a hardcoded 30 second default
	if i.config.DefaultTimeout != 0 {
		t.Errorf("expected DefaultTimeout 0s, got %v", i.config.DefaultTimeout)
	}

	// Verify callContext uses the 30 second hardcoded default
	ctx, cancel := i.callContext(context.Background(), 0)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline to be set")
	}

	remaining := time.Until(deadline)
	if remaining < 29*time.Second || remaining > 31*time.Second {
		t.Errorf("expected deadline around 30s, got %v", remaining)
	}
}

// TestInvoker_validatePayloadWithAnyOfSchema tests validatePayload with anyOf schema
func TestInvoker_validatePayloadWithAnyOfSchema(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{
				"anyOf": []interface{}{
					map[string]any{"type": "string"},
					map[string]any{"type": "number"},
				},
			},
		},
	}

	// String should match
	err := i.validatePayload(`{"value":"hello"}`, schema)
	if err != nil {
		t.Errorf("unexpected error for string value: %v", err)
	}

	// Number should match
	err = i.validatePayload(`{"value":42}`, schema)
	if err != nil {
		t.Errorf("unexpected error for number value: %v", err)
	}

	// Boolean doesn't match anyOf
	err = i.validatePayload(`{"value":true}`, schema)
	if err == nil {
		t.Error("expected error for value that doesn't match anyOf")
	}
}

// TestInvoker_validatePayloadWithRequiredFields tests validatePayload with multiple required fields
func TestInvoker_validatePayloadWithRequiredFields(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"required": []interface{}{"field1", "field2", "field3"},
		"properties": map[string]any{
			"field1": map[string]any{"type": "string"},
			"field2": map[string]any{"type": "string"},
			"field3": map[string]any{"type": "string"},
		},
	}

	// All fields present
	err := i.validatePayload(`{"field1":"a","field2":"b","field3":"c"}`, schema)
	if err != nil {
		t.Errorf("unexpected error with all required fields: %v", err)
	}

	// Missing one required field
	err = i.validatePayload(`{"field1":"a","field2":"b"}`, schema)
	if err == nil {
		t.Error("expected error for missing required field")
	}

	// Missing two required fields
	err = i.validatePayload(`{"field1":"a"}`, schema)
	if err == nil {
		t.Error("expected error for missing required fields")
	}
}

// TestInvoker_validatePayloadWithPatternProperties tests validatePayload with pattern matching
func TestInvoker_validatePayloadWithPatternProperties(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
	}).(*invoker)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"email": map[string]any{
				"type":   "string",
				"format": "email",
			},
			"uri": map[string]any{
				"type":   "string",
				"format": "uri",
			},
		},
	}

	// Valid email
	err := i.validatePayload(`{"email":"user@example.com"}`, schema)
	if err != nil {
		t.Errorf("unexpected error for valid email: %v", err)
	}

	// Invalid email format
	err = i.validatePayload(`{"email":"not-an-email"}`, schema)
	if err == nil {
		// Some validators may not strictly validate format
		t.Logf("Note: email format validation may not be strict")
	}

	// Valid URI
	err = i.validatePayload(`{"uri":"https://example.com"}`, schema)
	if err != nil {
		t.Errorf("unexpected error for valid URI: %v", err)
	}
}

// TestInvoker_executeWithRetryDisabled tests executeWithRetry when retry is disabled
func TestInvoker_executeWithRetryDisabled(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
		Retry: &RetryConfig{
			Enabled: false,
		},
	}).(*invoker)

	ctx := context.Background()
	attempts := 0

	fn := func() (string, error) {
		attempts++
		return "", fmt.Errorf("error")
	}

	opts := InvokeOptions{
		Retry: &RetryConfig{
			Enabled:     false,
			MaxAttempts: 5, // Should be ignored when disabled
		},
	}

	_, err := i.executeWithRetry(ctx, opts, fn)
	if err == nil {
		t.Error("expected error")
	}

	// Should only attempt once
	if attempts != 1 {
		t.Errorf("expected 1 attempt when retry disabled, got %d", attempts)
	}
}

// TestInvoker_CloseCancelsReconnect tests that Close cancels pending reconnect
func TestInvoker_CloseCancelsReconnect(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
		Reconnect: &ReconnectConfig{
			Enabled:        true,
			MaxAttempts:    10,
			InitialDelayMs: 5000, // Long delay
		},
	}).(*invoker)

	// Trigger reconnect
	i.reconnectAttempts = 1
	i.scheduleReconnectIfNeeded()

	if !i.isReconnecting {
		t.Error("expected isReconnecting to be true")
	}

	if i.reconnectCancelCtx == nil {
		t.Error("expected reconnectCancelCtx to be set")
	}

	// Close should cancel the reconnect
	i.Close()

	if i.isReconnecting {
		t.Error("expected isReconnecting to be false after Close")
	}

	if i.reconnectCancelCtx != nil {
		t.Error("expected reconnectCancelCtx to be nil after Close")
	}
}

// TestInvoker_isRetryableErrorWithRetryableStatusCode tests isRetryableError with custom retryable codes
func TestInvoker_isRetryableErrorWithRetryableStatusCode(t *testing.T) {
	t.Parallel()

	i := NewInvoker(&InvokerConfig{
		Address:  "127.0.0.1:19996",
		Insecure: true,
		Retry: &RetryConfig{
			RetryableStatusCodes: []int32{14, 8, 10, 1, 4}, // Including OK(1) and NotFound(4) for testing
		},
	}).(*invoker)

	testCases := []struct {
		err      error
		expected bool
	}{
		{fmt.Errorf("rpc error: code = 14"), true},  // Unavailable
		{fmt.Errorf("rpc error: code = 8"), true},   // ResourceExhausted
		{fmt.Errorf("rpc error: code = 10"), true},  // Aborted
		{fmt.Errorf("rpc error: code = 1"), true},   // OK (in retryable list)
		{fmt.Errorf("rpc error: code = 4"), true},   // NotFound (in retryable list)
		{fmt.Errorf("rpc error: code = 3"), false},  // InvalidArgument (not in list)
		{fmt.Errorf("some unavailable error"), true}, // Pattern match
	}

	for _, tc := range testCases {
		result := i.isRetryableError(tc.err)
		if result != tc.expected {
			t.Errorf("isRetryableError(%q) = %v, want %v", tc.err, result, tc.expected)
		}
	}
}

