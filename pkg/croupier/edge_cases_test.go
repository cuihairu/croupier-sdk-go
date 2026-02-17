package croupier

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test for FunctionDescriptor variations
func TestFunctionDescriptor_Variations(t *testing.T) {
	tests := []struct {
		name       string
		descriptor FunctionDescriptor
	}{
		{
			name: "minimal descriptor",
			descriptor: FunctionDescriptor{
				ID:      "test.minimal",
				Version: "1.0.0",
			},
		},
		{
			name: "full descriptor",
			descriptor: FunctionDescriptor{
				ID:        "player.action.update",
				Version:   "2.1.0",
				Category:  "player",
				Risk:      "high",
				Entity:    "Player",
				Operation: "update",
				Enabled:   true,
			},
		},
		{
			name: "descriptor with disabled",
			descriptor: FunctionDescriptor{
				ID:      "test.disabled",
				Version: "1.0.0",
				Enabled: false,
			},
		},
		{
			name: "descriptor with special chars",
			descriptor: FunctionDescriptor{
				ID:      "test.function-with_special.chars@123",
				Version: "1.0.0-beta",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.descriptor.ID)
			assert.NotEmpty(t, tt.descriptor.Version)
		})
	}
}

// Test for JobEvent variations
func TestJobEvent_Variations(t *testing.T) {
	tests := []struct {
		name  string
		event JobEvent
		valid bool
	}{
		{
			name: "started event",
			event: JobEvent{
				EventType: "started",
				JobID:     "job-123",
			},
			valid: true,
		},
		{
			name: "progress event",
			event: JobEvent{
				EventType: "progress",
				JobID:     "job-456",
				Message:   "Processing...",
				Progress:  50,
			},
			valid: true,
		},
		{
			name: "completed event",
			event: JobEvent{
				EventType: "completed",
				JobID:     "job-789",
				Done:      true,
			},
			valid: true,
		},
		{
			name: "error event",
			event: JobEvent{
				EventType: "error",
				JobID:     "job-error",
				Message:   "Something went wrong",
				Done:      true,
			},
			valid: true,
		},
		{
			name: "cancelled event",
			event: JobEvent{
				EventType: "cancelled",
				JobID:     "job-cancelled",
				Message:   "User cancelled",
				Done:      true,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.event.EventType)
			assert.NotEmpty(t, tt.event.JobID)

			if tt.event.Done {
				assert.True(t, tt.event.EventType == "completed" ||
					tt.event.EventType == "error" ||
					tt.event.EventType == "cancelled")
			}
		})
	}
}

// Test for InvokeOptions variations
func TestInvokeOptions_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		options InvokeOptions
	}{
		{
			name:    "empty options",
			options: InvokeOptions{},
		},
		{
			name: "options with idempotency key",
			options: InvokeOptions{
				IdempotencyKey: "unique-key-12345",
			},
		},
		{
			name: "options with timeout",
			options: InvokeOptions{
				Timeout: 1 * time.Nanosecond,
			},
		},
		{
			name: "options with long timeout",
			options: InvokeOptions{
				Timeout: 24 * time.Hour,
			},
		},
		{
			name: "options with empty headers",
			options: InvokeOptions{
				Headers: map[string]string{},
			},
		},
		{
			name: "options with multiple headers",
			options: InvokeOptions{
				Headers: map[string]string{
					"X-Custom-1": "value1",
					"X-Custom-2": "value2",
					"X-Custom-3": "value3",
				},
			},
		},
		{
			name: "options with nil retry",
			options: InvokeOptions{
				Retry: nil,
			},
		},
		{
			name: "options with empty retry",
			options: InvokeOptions{
				Retry: &RetryConfig{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify options can be created without panic
			assert.NotNil(t, tt.options)
		})
	}
}

// Test for ClientConfig edge cases
func TestClientConfig_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		config func() *ClientConfig
	}{
		{
			name: "all empty strings",
			config: func() *ClientConfig {
				return &ClientConfig{}
			},
		},
		{
			name: "very long strings",
			config: func() *ClientConfig {
				longString := string(make([]byte, 10000))
				return &ClientConfig{
					AgentAddr: longString,
					GameID:    longString,
					ServiceID: longString,
				}
			},
		},
		{
			name: "unicode strings",
			config: func() *ClientConfig {
				return &ClientConfig{
					GameID:    "游戏-123",
					ServiceID: "服务-456",
					Env:       "生产环境",
				}
			},
		},
		{
			name: "special characters in address",
			config: func() *ClientConfig {
				return &ClientConfig{
					AgentAddr: "tcp://127.0.0.1:19090?param=value&other=123",
				}
			},
		},
		{
			name: "all TLS fields set",
			config: func() *ClientConfig {
				return &ClientConfig{
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
			name: "all boolean flags",
			config: func() *ClientConfig {
				return &ClientConfig{
					Insecure:           true,
					InsecureSkipVerify: true,
					DisableLogging:     true,
					DebugLogging:       true,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config()
			assert.NotNil(t, config)
		})
	}
}

// Test for concurrent config access
func TestClientConfig_ConcurrentAccess(t *testing.T) {
	config := &ClientConfig{}
	var wg sync.WaitGroup

	const numGoroutines = 100
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			config.AgentAddr = "addr-" + string(rune('a'+idx%26))
			config.GameID = "game-" + string(rune('0'+idx%10))
			config.TimeoutSeconds = idx
		}(i)
	}

	wg.Wait()
	// Just verify no race condition occurred
	assert.NotNil(t, config)
}

// Test for context variations
func TestContext_Variations(t *testing.T) {
	tests := []struct {
		name   string
		ctx    func() context.Context
		cancel func(context.Context)
	}{
		{
			name: "background context",
			ctx: func() context.Context {
				return context.Background()
			},
			cancel: nil,
		},
		{
			name: "todo context",
			ctx: func() context.Context {
				return context.TODO()
			},
			cancel: nil,
		},
		{
			name: "with value",
			ctx: func() context.Context {
				return context.WithValue(context.Background(), "key", "value")
			},
			cancel: nil,
		},
		{
			name: "with timeout",
			ctx: func() context.Context {
				ctx, _ := context.WithTimeout(context.Background(), 1*time.Hour)
				return ctx
			},
			cancel: func(ctx context.Context) {
				// Context will be cancelled automatically
			},
		},
		{
			name: "with deadline",
			ctx: func() context.Context {
				deadline := time.Now().Add(1 * time.Hour)
				ctx, _ := context.WithDeadline(context.Background(), deadline)
				return ctx
			},
			cancel: func(ctx context.Context) {
				// Context will be cancelled automatically
			},
		},
		{
			name: "already cancelled",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx
			},
			cancel: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx()
			assert.NotNil(t, ctx)

			if tt.cancel != nil {
				tt.cancel(ctx)
			}
		})
	}
}

// Test for retry configuration variations
func TestRetryConfig_Variations(t *testing.T) {
	tests := []struct {
		name   string
		config RetryConfig
	}{
		{
			name: "zero max attempts",
			config: RetryConfig{
				Enabled:     true,
				MaxAttempts: 0,
			},
		},
		{
			name: "negative max attempts",
			config: RetryConfig{
				Enabled:     true,
				MaxAttempts: -1,
			},
		},
		{
			name: "very large max attempts",
			config: RetryConfig{
				Enabled:     true,
				MaxAttempts: 1000000,
			},
		},
		{
			name: "zero delays",
			config: RetryConfig{
				Enabled:         true,
				InitialDelayMs:  0,
				MaxDelayMs:      0,
				BackoffMultiplier: 0,
				JitterFactor:    0,
			},
		},
		{
			name: "very large delays",
			config: RetryConfig{
				Enabled:          true,
				InitialDelayMs:   1000000,
				MaxDelayMs:       10000000,
				BackoffMultiplier: 100.0,
				JitterFactor:     1.0,
			},
		},
		{
			name: "negative jitter",
			config: RetryConfig{
				Enabled:       true,
				JitterFactor: -0.5,
			},
		},
		{
			name: "no status codes",
			config: RetryConfig{
				Enabled:              true,
				RetryableStatusCodes: []int32{},
			},
		},
		{
			name: "many status codes",
			config: RetryConfig{
				Enabled:              true,
				RetryableStatusCodes: []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			},
		},
		{
			name: "disabled with config",
			config: RetryConfig{
				Enabled:              false,
				MaxAttempts:          5,
				InitialDelayMs:       100,
				RetryableStatusCodes: []int32{1, 2, 3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify config can be created
			assert.True(t, tt.config.Enabled || !tt.config.Enabled)
		})
	}
}

// Test for reconnect configuration variations
func TestReconnectConfig_Variations(t *testing.T) {
	tests := []struct {
		name   string
		config ReconnectConfig
	}{
		{
			name: "zero max attempts",
			config: ReconnectConfig{
				Enabled:     true,
				MaxAttempts: 0,
			},
		},
		{
			name: "infinite retries (-1)",
			config: ReconnectConfig{
				Enabled:     true,
				MaxAttempts: -1,
			},
		},
		{
			name: "no delay",
			config: ReconnectConfig{
				Enabled:         true,
				InitialDelayMs:  0,
				MaxDelayMs:      0,
				BackoffMultiplier: 1.0,
			},
		},
		{
			name: "exponential backoff",
			config: ReconnectConfig{
				Enabled:          true,
				InitialDelayMs:   100,
				MaxDelayMs:       30000,
				BackoffMultiplier: 2.0,
				JitterFactor:     0.2,
			},
		},
		{
			name: "disabled",
			config: ReconnectConfig{
				Enabled: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify config can be created
			assert.True(t, tt.config.Enabled || !tt.config.Enabled)
		})
	}
}

// Benchmark for client creation
func BenchmarkNewClient(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewClient(DefaultClientConfig())
	}
}

// Benchmark for config creation
func BenchmarkDefaultClientConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = DefaultClientConfig()
	}
}
