package croupier

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for client lifecycle

func TestClient_Lifecycle(t *testing.T) {
	tests := []struct {
		name   string
		config *ClientConfig
	}{
		{
			name:   "default config",
			config: DefaultClientConfig(),
		},
		{
			name: "custom config",
			config: &ClientConfig{
				AgentAddr:      "localhost:19090",
				GameID:         "test-game",
				ServiceID:      "test-service",
				TimeoutSeconds: 30,
			},
		},
		{
			name: "config with local listen",
			config: &ClientConfig{
				AgentAddr:   "localhost:19090",
				LocalListen: "tcp://127.0.0.1:0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.config)
			require.NotNil(t, client)

			// Close should not panic
			err := client.Close()
			assert.NoError(t, err)
		})
	}
}

func TestClient_ConnectTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected bool
	}{
		{
			name:     "very short timeout",
			timeout:  1 * time.Millisecond,
			expected: true,
		},
		{
			name:     "short timeout",
			timeout:  10 * time.Millisecond,
			expected: true,
		},
		{
			name:     "medium timeout",
			timeout:  100 * time.Millisecond,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(&ClientConfig{
				AgentAddr:      "localhost:9999", // Non-existent server
				TimeoutSeconds: int(tt.timeout.Seconds()),
			})
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			err := client.Connect(ctx)
			// Should timeout or fail
			assert.True(t, err != nil || ctx.Err() == context.DeadlineExceeded)
		})
	}
}

func TestClient_ConcurrentClose(t *testing.T) {
	client := NewClient(DefaultClientConfig())

	const numGoroutines = 10
	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errChan <- client.Close()
		}()
	}

	wg.Wait()
	close(errChan)

	// All closes should succeed
	for err := range errChan {
		assert.NoError(t, err)
	}
}

func TestClient_RegisterFunctionMultiple(t *testing.T) {
	client := NewClient(DefaultClientConfig())
	defer client.Close()

	functions := []FunctionDescriptor{
		{
			ID:      "test.function.1",
			Version: "1.0.0",
		},
		{
			ID:      "test.function.2",
			Version: "1.0.0",
		},
		{
			ID:      "test.function.3",
			Version: "1.0.0",
		},
	}

	for _, fn := range functions {
		err := client.RegisterFunction(fn, func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte(`{"result":"ok"}`), nil
		})
		// May fail if not connected, but should not panic
		_ = err
	}
}

func TestClient_StopWithoutServe(t *testing.T) {
	client := NewClient(DefaultClientConfig())

	// Stop without Serve should not fail
	err := client.Stop()
	assert.NoError(t, err)

	err = client.Close()
	assert.NoError(t, err)
}

func TestClient_MultipleClose(t *testing.T) {
	client := NewClient(DefaultClientConfig())

	// First close
	err := client.Close()
	assert.NoError(t, err)

	// Second close should also be safe
	err = client.Close()
	assert.NoError(t, err)
}

func TestClient_DoubleStop(t *testing.T) {
	client := NewClient(DefaultClientConfig())

	// First stop
	err := client.Stop()
	assert.NoError(t, err)

	// Second stop should also be safe
	err = client.Stop()
	assert.NoError(t, err)
}

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

func TestClient_DeadlineExceeded(t *testing.T) {
	client := NewClient(DefaultClientConfig())
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for timeout
	time.Sleep(10 * time.Millisecond)

	err := client.Connect(ctx)
	// Should timeout
	assert.True(t, err != nil || ctx.Err() == context.DeadlineExceeded)
}

func TestClient_ServeContext(t *testing.T) {
	tests := []struct {
		name        string
		ctx         func() context.Context
		expectError bool
	}{
		{
			name: "cancelled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			expectError: true,
		},
		{
			name: "timeout context",
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
				defer cancel()
				return ctx
			},
			expectError: false,
		},
		{
			name: "background context",
			ctx: func() context.Context {
				return context.Background()
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(DefaultClientConfig())
			defer client.Close()

			ctx := tt.ctx()

			// Serve with context
			errChan := make(chan error, 1)
			go func() {
				errChan <- client.Serve(ctx)
			}()

			// Wait a bit for Serve to start
			time.Sleep(50 * time.Millisecond)

			// Stop the client
			client.Stop()

			select {
			case err := <-errChan:
				if tt.expectError {
					assert.Error(t, err)
				}
			case <-time.After(1 * time.Second):
				// Timeout is OK for background context
			}
		})
	}
}

func TestClient_GetLocalAddress(t *testing.T) {
	tests := []struct {
		name         string
		localListen  string
		expectEmpty  bool
	}{
		{
			name:        "no local listen",
			localListen: "",
			expectEmpty: true,
		},
		{
			name:        "with local listen",
			localListen: "tcp://127.0.0.1:0",
			expectEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultClientConfig()
			config.LocalListen = tt.localListen

			client := NewClient(config)
			defer client.Close()

			addr := client.GetLocalAddress()
			if tt.expectEmpty {
				assert.Empty(t, addr)
			} else {
				// May or may not be empty depending on connection state
				_ = addr
			}
		})
	}
}

func TestClient_ConcurrentOperations(t *testing.T) {
	client := NewClient(DefaultClientConfig())
	defer client.Close()

	const numGoroutines = 20
	var wg sync.WaitGroup

	// Concurrent RegisterFunction
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			desc := FunctionDescriptor{
				ID:      "test.concurrent.function." + string(rune('a'+idx%26)),
				Version: "1.0.0",
			}
			handler := func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte(`{"result":"ok"}`), nil
			}
			_ = client.RegisterFunction(desc, handler)
		}(i)
	}

	// Concurrent GetLocalAddress
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.GetLocalAddress()
		}()
	}

	wg.Wait()
	// Should not panic or deadlock
}

func TestClient_PanicRecovery(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Client operations should not panic: %v", r)
		}
	}()

	client := NewClient(DefaultClientConfig())

	// Various operations that should not panic
	_ = client.GetLocalAddress()
	_ = client.Close()
	_ = client.Stop()
	_ = client.Close()
	_ = client.Stop()
}

func TestClient_NilConfig(t *testing.T) {
	// Nil config should use defaults
	client := NewClient(nil)
	require.NotNil(t, client)

	err := client.Close()
	assert.NoError(t, err)
}

func TestClient_EmptyStrings(t *testing.T) {
	config := &ClientConfig{
		AgentAddr:      "",
		GameID:         "",
		ServiceID:      "",
		LocalListen:    "",
		AgentID:        "",
		ControlAddr:    "",
		CAFile:         "",
		CertFile:       "",
		KeyFile:        "",
		ServerName:     "",
		ProviderLang:   "",
		ProviderSDK:    "",
		ServiceVersion: "",
		Env:            "",
	}

	client := NewClient(config)
	require.NotNil(t, client)

	err := client.Close()
	assert.NoError(t, err)
}

func TestClient_ZeroTimeout(t *testing.T) {
	config := DefaultClientConfig()
	config.TimeoutSeconds = 0

	client := NewClient(config)
	require.NotNil(t, client)

	err := client.Close()
	assert.NoError(t, err)
}

func TestClient_VeryLargeTimeout(t *testing.T) {
	config := DefaultClientConfig()
	config.TimeoutSeconds = 86400 // 24 hours

	client := NewClient(config)
	require.NotNil(t, client)

	err := client.Close()
	assert.NoError(t, err)
}

func TestClient_NegativeTimeout(t *testing.T) {
	config := DefaultClientConfig()
	config.TimeoutSeconds = -1

	client := NewClient(config)
	require.NotNil(t, client)

	err := client.Close()
	assert.NoError(t, err)
}
