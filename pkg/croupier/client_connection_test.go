// Copyright 2025 Croupier Authors
// Licensed under the Apache License, Version 2.0

package croupier

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestClient_connectionScenarios tests client connection scenarios
func TestClient_connectionScenarios(t *testing.T) {
	t.Run("Connect with invalid address", func(t *testing.T) {
		config := &ClientConfig{
			AgentAddr: "invalid-address-99999:99999",
		}

		client := NewClient(config)
		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		ctx := context.Background()
		err := client.Connect(ctx)

		t.Logf("Connect with invalid address error: %v", err)
	})

	t.Run("Connect with timeout context", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		time.Sleep(10 * time.Millisecond) // Ensure timeout has passed

		err := client.Connect(ctx)
		t.Logf("Connect with timeout context error: %v", err)
	})

	t.Run("Connect with cancelled context", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := client.Connect(ctx)
		t.Logf("Connect with cancelled context error: %v", err)
	})

	t.Run("Connect with context values", func(t *testing.T) {
		type ctxKey string
		key := ctxKey("trace-id")

		ctx := context.WithValue(context.Background(), key, "trace-12345")

		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		err := client.Connect(ctx)
		t.Logf("Connect with context values error: %v", err)
	})
}

// TestClient_connectWithVariousConfigs tests Connect with various configurations
func TestClient_connectWithVariousConfigs(t *testing.T) {
	t.Run("Connect with IPC address", func(t *testing.T) {
		config := &ClientConfig{
			AgentAddr: "ipc:///tmp/test-agent",
		}

		client := NewClient(config)
		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		ctx := context.Background()
		err := client.Connect(ctx)
		t.Logf("Connect with IPC address error: %v", err)
	})

	t.Run("Connect with TCP address", func(t *testing.T) {
		config := &ClientConfig{
			AgentAddr: "tcp://localhost:19090",
		}

		client := NewClient(config)
		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		ctx := context.Background()
		err := client.Connect(ctx)
		t.Logf("Connect with TCP address error: %v", err)
	})

	t.Run("Connect with various timeouts", func(t *testing.T) {
		timeouts := []int{1, 10, 30, 60, 300}

		for _, timeout := range timeouts {
			config := &ClientConfig{
				AgentAddr:      "localhost:19090",
				TimeoutSeconds: timeout,
			}

			client := NewClient(config)
			if client == nil {
				t.Fatalf("Timeout %d: NewClient returned nil", timeout)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			err := client.Connect(ctx)
			t.Logf("Timeout %d: Connect error: %v", timeout, err)
		}
	})

	t.Run("Connect with insecure flag", func(t *testing.T) {
		config := &ClientConfig{
			AgentAddr: "localhost:19090",
			Insecure:  true,
		}

		client := NewClient(config)
		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		ctx := context.Background()
		err := client.Connect(ctx)
		t.Logf("Connect with insecure=true error: %v", err)
	})

	t.Run("Connect with TLS config", func(t *testing.T) {
		config := &ClientConfig{
			AgentAddr: "localhost:19090",
			CAFile:    "/path/to/ca.crt",
			CertFile:  "/path/to/cert.pem",
			KeyFile:   "/path/to/key.pem",
		}

		client := NewClient(config)
		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		ctx := context.Background()
		err := client.Connect(ctx)
		t.Logf("Connect with TLS config error: %v", err)
	})
}

// TestClient_serveOperations tests Serve operations
func TestClient_serveOperations(t *testing.T) {
	t.Run("Serve without Connect", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		ctx := context.Background()
		err := client.Serve(ctx)

		t.Logf("Serve without Connect error: %v", err)
	})

	t.Run("Serve with timeout context", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err := client.Serve(ctx)
		t.Logf("Serve with timeout error: %v", err)
	})

	t.Run("Serve with cancelled context", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := client.Serve(ctx)
		t.Logf("Serve with cancelled context error: %v", err)
	})

	t.Run("Serve after RegisterFunction", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		desc := FunctionDescriptor{
			ID:      "test.serve",
			Version: "1.0.0",
		}
		handler := func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte(`{"result":"ok"}`), nil
		}

		err := client.RegisterFunction(desc, handler)
		t.Logf("RegisterFunction error: %v", err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err = client.Serve(ctx)
		t.Logf("Serve after RegisterFunction error: %v", err)
	})

	t.Run("Serve with multiple functions", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		// Register multiple functions
		for i := 0; i < 5; i++ {
			desc := FunctionDescriptor{
				ID:      fmt.Sprintf("test.func%d", i),
				Version: "1.0.0",
			}
			handler := func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte(`{"result":"ok"}`), nil
			}

			err := client.RegisterFunction(desc, handler)
			t.Logf("Register function %d error: %v", i, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		err := client.Serve(ctx)
		t.Logf("Serve with multiple functions error: %v", err)
	})
}

// TestClient_lifecycleSequences tests complete lifecycle sequences
func TestClient_lifecycleSequences(t *testing.T) {
	t.Run("full lifecycle: register, connect, serve, stop", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		// Register function
		desc := FunctionDescriptor{
			ID:      "test.lifecycle",
			Version: "1.0.0",
		}
		handler := func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte(`{"result":"ok"}`), nil
		}

		err := client.RegisterFunction(desc, handler)
		t.Logf("RegisterFunction error: %v", err)

		// Connect
		ctx := context.Background()
		err = client.Connect(ctx)
		t.Logf("Connect error: %v", err)

		// Get local address
		addr := client.GetLocalAddress()
		t.Logf("Local address: %s", addr)

		// Stop
		err = client.Stop()
		t.Logf("Stop error: %v", err)

		// Close
		err = client.Close()
		t.Logf("Close error: %v", err)
	})

	t.Run("connect and stop multiple times", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		for i := 0; i < 3; i++ {
			ctx := context.Background()
			err := client.Connect(ctx)
			t.Logf("Iteration %d Connect error: %v", i, err)

			err = client.Stop()
			t.Logf("Iteration %d Stop error: %v", i, err)
		}

		err := client.Close()
		t.Logf("Final Close error: %v", err)
	})

	t.Run("serve and stop multiple times", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		desc := FunctionDescriptor{
			ID:      "test.multi",
			Version: "1.0.0",
		}
		handler := func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte(`{"result":"ok"}`), nil
		}

		_ = client.RegisterFunction(desc, handler)

		for i := 0; i < 3; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
			defer cancel()

			err := client.Serve(ctx)
			t.Logf("Iteration %d Serve error: %v", i, err)

			err = client.Stop()
			t.Logf("Iteration %d Stop error: %v", i, err)
		}

		err := client.Close()
		t.Logf("Final Close error: %v", err)
	})
}

// TestClient_concurrentOperations tests concurrent client operations
func TestClient_concurrentOperations(t *testing.T) {
	t.Run("concurrent RegisterFunction calls", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		const numGoroutines = 10
		errors := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				desc := FunctionDescriptor{
					ID:      fmt.Sprintf("test.func%d", idx),
					Version: "1.0.0",
				}
				handler := func(ctx context.Context, payload []byte) ([]byte, error) {
					return []byte(`{"result":"ok"}`), nil
				}

				errors[idx] = client.RegisterFunction(desc, handler)
			}(i)
		}

		// Wait a bit for goroutines to complete
		time.Sleep(500 * time.Millisecond)

		errorCount := 0
		for i, err := range errors {
			if err != nil {
				errorCount++
				t.Logf("Goroutine %d error: %v", i, err)
			}
		}

		t.Logf("Concurrent RegisterFunction: %d/%d had errors", errorCount, numGoroutines)
	})

	t.Run("concurrent GetLocalAddress calls", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		const numGoroutines = 100
		addresses := make([]string, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				addresses[idx] = client.GetLocalAddress()
			}(i)
		}

		// Wait a bit for goroutines to complete
		time.Sleep(500 * time.Millisecond)

		for i, addr := range addresses {
			t.Logf("Goroutine %d: GetLocalAddress = %s", i, addr)
		}
	})
}

// TestClient_errorHandling tests client error handling
func TestClient_errorHandling(t *testing.T) {
	t.Run("RegisterFunction after Close", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		// Close immediately
		err := client.Close()
		t.Logf("Close error: %v", err)

		// Try to register after close
		desc := FunctionDescriptor{
			ID:      "test.afterclose",
			Version: "1.0.0",
		}
		handler := func(ctx context.Context, payload []byte) ([]byte, error) {
			return []byte(`{"result":"ok"}`), nil
		}

		err = client.RegisterFunction(desc, handler)
		t.Logf("RegisterFunction after Close error: %v", err)
	})

	t.Run("Connect after Close", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		// Close immediately
		err := client.Close()
		t.Logf("Close error: %v", err)

		// Try to connect after close
		ctx := context.Background()
		err = client.Connect(ctx)
		t.Logf("Connect after Close error: %v", err)
	})

	t.Run("multiple Close calls", func(t *testing.T) {
		config := DefaultClientConfig()
		client := NewClient(config)

		if client == nil {
			t.Fatal("NewClient returned nil")
		}

		// Close multiple times
		for i := 0; i < 3; i++ {
			err := client.Close()
			t.Logf("Close call %d error: %v", i+1, err)
		}
	})
}
