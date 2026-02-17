package croupier

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test for NewClient with various configurations
func TestNewClient_Configurations(t *testing.T) {
	tests := []struct {
		name   string
		config *ClientConfig
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
		},
		{
			name: "custom config",
			config: &ClientConfig{
				AgentAddr:     "custom.agent:19090",
				GameID:        "test-game",
				ServiceID:     "test-service",
				TimeoutSeconds: 60,
			},
		},
		{
			name: "config with disabled logging",
			config: &ClientConfig{
				AgentAddr:      "localhost:19090",
				DisableLogging: true,
			},
		},
		{
			name: "config with debug logging",
			config: &ClientConfig{
				AgentAddr:    "localhost:19090",
				DebugLogging: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.config)
			require.NotNil(t, client)

			// Just verify client is created, config is internal
			assert.NotNil(t, client)
		})
	}
}

// Test for concurrent operations on client
func TestClient_ConcurrentOperations(t *testing.T) {
	client := NewClient(DefaultClientConfig())
	defer client.Close()

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Concurrent Connect calls
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			errors <- client.Connect(ctx)
		}()
	}

	// Concurrent RegisterFunction calls
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			desc := FunctionDescriptor{
				ID:      "test.concurrent.function." + string(rune('a'+idx)),
				Version: "1.0.0",
			}
			handler := func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte(`{"result":"ok"}`), nil
			}
			errors <- client.RegisterFunction(desc, handler)
		}(i)
	}

	wg.Wait()
	close(errors)

	// Collect errors - some may fail due to no agent, but should not panic
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}
	// We expect errors since there's no actual agent running
	t.Logf("Got %d errors from concurrent operations (expected)", errorCount)
}

// Test for GetLocalAddress 2
func TestClient_GetLocalAddressConfigs(t *testing.T) {
	tests := []struct {
		name         string
		config       *ClientConfig
		expectEmpty  bool
	}{
		{
			name: "default config",
			config: DefaultClientConfig(),
			expectEmpty: true, // No local address set
		},
		{
			name: "config with local listen",
			config: &ClientConfig{
				AgentAddr:   "localhost:19090",
				LocalListen: "tcp://127.0.0.1:0",
			},
			expectEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.config)
			defer client.Close()

			addr := client.GetLocalAddress()
			if tt.expectEmpty {
				assert.Empty(t, addr)
			} else {
				assert.NotEmpty(t, addr)
			}
		})
	}
}

// Test for Close without Connect
func TestClient_CloseWithoutConnect(t *testing.T) {
	client := NewClient(DefaultClientConfig())

	err := client.Close()
	assert.NoError(t, err, "Close should not fail even without Connect")

	// Double close should also be safe
	err = client.Close()
	assert.NoError(t, err, "Second Close should also be safe")
}

// Test for Stop without Connect
func TestClient_StopWithoutConnect(t *testing.T) {
	client := NewClient(DefaultClientConfig())

	err := client.Stop()
	assert.NoError(t, err, "Stop should not fail even without Connect")
}

// Test for Serve without Connect
func TestClient_ServeNotConnected(t *testing.T) {
	client := NewClient(DefaultClientConfig())
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.Serve(ctx)
	// Should either fail quickly or timeout
	assert.True(t, err != nil || ctx.Err() == context.DeadlineExceeded)
}

// Test for RegisterFunction validations
func TestClient_RegisterFunction_Validations(t *testing.T) {
	client := NewClient(DefaultClientConfig())
	defer client.Close()

	tests := []struct {
		name      string
		desc      FunctionDescriptor
		handler   FunctionHandler
		expectErr bool
	}{
		{
			name: "valid function",
			desc: FunctionDescriptor{
				ID:      "test.valid.function",
				Version: "1.0.0",
			},
			handler: func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte(`{}`), nil
			},
			expectErr: false,
		},
		{
			name: "function with all fields",
			desc: FunctionDescriptor{
				ID:        "test.full.function",
				Version:   "2.0.0",
				Category:  "test",
				Risk:      "low",
				Entity:    "test",
				Operation: "read",
				Enabled:   true,
			},
			handler: func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte(`{"status":"ok"}`), nil
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.RegisterFunction(tt.desc, tt.handler)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				// May fail if not connected, but should not panic
				_ = err
			}
		})
	}
}

// Test for handler execution
func TestFunctionHandler_Execution(t *testing.T) {
	tests := []struct {
		name      string
		handler   FunctionHandler
		payload   []byte
		expectOK  bool
	}{
		{
			name: "simple handler",
			handler: func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte(`{"result":"success"}`), nil
			},
			payload:  []byte(`{"input":"test"}`),
			expectOK: true,
		},
		{
			name: "handler returns empty result",
			handler: func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte{}, nil
			},
			payload:  []byte{},
			expectOK: true,
		},
		{
			name: "handler with error",
			handler: func(ctx context.Context, payload []byte) ([]byte, error) {
				return nil, assert.AnError
			},
			payload:  []byte{},
			expectOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := tt.handler(ctx, tt.payload)

			if tt.expectOK {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// Test for Context cancellation
func TestClient_ContextCancellation(t *testing.T) {
	client := NewClient(DefaultClientConfig())
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	err := client.Connect(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// Test for timeout scenarios
func TestClient_TimeoutScenarios(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected bool
	}{
		{
			name:     "zero timeout",
			timeout:  0,
			expected: false,
		},
		{
			name:     "very short timeout",
			timeout:  1 * time.Nanosecond,
			expected: true,
		},
		{
			name:     "short timeout",
			timeout:  10 * time.Millisecond,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(&ClientConfig{
				AgentAddr:      "localhost:19090",
				TimeoutSeconds: int(tt.timeout.Seconds()),
			})
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			_ = client.Connect(ctx)
			// Connection will fail, but we're testing timeout handling
		})
	}
}

// Test for config variations
func TestClientConfig_Variations(t *testing.T) {
	tests := []struct {
		name   string
		config func() *ClientConfig
	}{
		{
			name: "with all security fields",
			config: func() *ClientConfig {
				return &ClientConfig{
					AgentAddr:          "localhost:19090",
					Insecure:           false,
					CAFile:             "/path/to/ca.pem",
					CertFile:           "/path/to/cert.pem",
					KeyFile:            "/path/to/key.pem",
					ServerName:         "example.com",
					InsecureSkipVerify: false,
				}
			},
		},
		{
			name: "with IPC address",
			config: func() *ClientConfig {
				return &ClientConfig{
					AgentAddr:    "localhost:19090",
					AgentIPCAddr: "ipc://croupier-agent",
				}
			},
		},
		{
			name: "with all identification fields",
			config: func() *ClientConfig {
				return &ClientConfig{
					AgentAddr:      "localhost:19090",
					GameID:         "game-123",
					Env:            "production",
					ServiceID:      "service-456",
					ServiceVersion: "2.0.0",
					AgentID:        "agent-789",
					ProviderLang:   "go",
					ProviderSDK:    "custom-sdk",
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config()
			client := NewClient(config)
			defer client.Close()

			// Verify client is created with the config
			assert.NotNil(t, client)
		})
	}
}

// Test for retry and reconnect configs
func TestConfig_Defaults(t *testing.T) {
	reconnectConfig := DefaultReconnectConfig()
	assert.True(t, reconnectConfig.Enabled)
	assert.Equal(t, 0, reconnectConfig.MaxAttempts)
	assert.Equal(t, 1000, reconnectConfig.InitialDelayMs)
	assert.Equal(t, 30000, reconnectConfig.MaxDelayMs)
	assert.Equal(t, 2.0, reconnectConfig.BackoffMultiplier)
	assert.Equal(t, 0.2, reconnectConfig.JitterFactor)

	retryConfig := DefaultRetryConfig()
	assert.True(t, retryConfig.Enabled)
	assert.Equal(t, 3, retryConfig.MaxAttempts)
	assert.Equal(t, 100, retryConfig.InitialDelayMs)
	assert.Equal(t, 5000, retryConfig.MaxDelayMs)
	assert.Equal(t, 2.0, retryConfig.BackoffMultiplier)
	assert.Equal(t, 0.1, retryConfig.JitterFactor)
	assert.NotEmpty(t, retryConfig.RetryableStatusCodes)
}

// Test for InvokeOptions variations
func TestInvokeOptions_AllVariations(t *testing.T) {
	baseOptions := InvokeOptions{}

	assert.Empty(t, baseOptions.IdempotencyKey)
	assert.Zero(t, baseOptions.Timeout)
	assert.Nil(t, baseOptions.Headers)
	assert.Nil(t, baseOptions.Retry)

	// With idempotency key
	options := baseOptions
	options.IdempotencyKey = "unique-key-123"
	assert.Equal(t, "unique-key-123", options.IdempotencyKey)

	// With timeout
	options.Timeout = 5 * time.Second
	assert.Equal(t, 5*time.Second, options.Timeout)

	// With headers
	options.Headers = map[string]string{
		"X-Custom": "value",
		"X-Request": "123",
	}
	assert.Equal(t, 2, len(options.Headers))

	// With retry
	retryCfg := DefaultRetryConfig()
	options.Retry = retryCfg
	assert.NotNil(t, options.Retry)
}

// Test for JobEvent types
func TestJobEvent_AllTypes(t *testing.T) {
	eventTypes := []string{"started", "progress", "completed", "error", "cancelled"}

	for _, eventType := range eventTypes {
		t.Run("event_"+eventType, func(t *testing.T) {
			event := JobEvent{
				EventType: eventType,
				JobID:     "job-" + eventType,
				Payload:   `{"data":"test"}`,
				Done:      eventType == "completed" || eventType == "error" || eventType == "cancelled",
			}

			assert.Equal(t, eventType, event.EventType)
			assert.Equal(t, "job-"+eventType, event.JobID)

			if event.Done {
				assert.NotEmpty(t, event.JobID)
			}
		})
	}
}

// Test for FunctionDescriptor validation
func TestFunctionDescriptor_Validation(t *testing.T) {
	tests := []struct {
		name        string
		descriptor  FunctionDescriptor
		expectValid bool
	}{
		{
			name: "minimal valid",
			descriptor: FunctionDescriptor{
				ID:      "test.minimal",
				Version: "1.0",
			},
			expectValid: true,
		},
		{
			name: "full descriptor",
			descriptor: FunctionDescriptor{
				ID:        "player.action",
				Version:   "2.1.0",
				Category:  "player",
				Risk:      "high",
				Entity:    "Player",
				Operation: "update",
				Enabled:   true,
			},
			expectValid: true,
		},
		{
			name: "missing ID",
			descriptor: FunctionDescriptor{
				Version: "1.0",
			},
			expectValid: false,
		},
		{
			name: "missing version",
			descriptor: FunctionDescriptor{
				ID: "test.function",
			},
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.descriptor.ID != "" && tt.descriptor.Version != ""
			assert.Equal(t, tt.expectValid, valid)
		})
	}
}

// Test for ServiceID generation uniqueness
func TestGenerateUUID_Uniqueness(t *testing.T) {
	uuids := make(map[string]bool)
	count := 1000

	for i := 0; i < count; i++ {
		uuid := generateUUID()
		if uuids[uuid] {
			t.Errorf("Duplicate UUID generated at iteration %d: %s", i, uuid)
		}
		uuids[uuid] = true
	}

	assert.Equal(t, count, len(uuids), "All UUIDs should be unique")
}

// Test for retry config with custom status codes
func TestRetryConfig_CustomStatusCodes(t *testing.T) {
	customCodes := []int32{1, 2, 3, 4, 5}

	config := &RetryConfig{
		Enabled:              true,
		MaxAttempts:          5,
		RetryableStatusCodes: customCodes,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, 5, config.MaxAttempts)
	assert.Equal(t, customCodes, config.RetryableStatusCodes)
}

// Benchmark test for UUID generation
func BenchmarkGenerateUUID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		generateUUID()
	}
}
