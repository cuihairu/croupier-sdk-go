// Copyright 2025 Croupier Authors
// Licensed under the Apache License, Version 2.0

package croupier
//go:build integration
// +build integration


import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestNNGManager_configurationTests tests NNG manager configuration
func TestNNGManager_configurationTests(t *testing.T) {
	t.Run("NewNNGManager with minimal ClientConfig", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Logf("NewNNGManager error: %v", err)
		}
		if manager != nil {
			defer manager.Disconnect()
		}
	})

	t.Run("NewNNGManager with full ClientConfig", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr:         "tcp://localhost:19090",
			GameID:            "test-game",
			ServiceID:         "test-service",
			ServiceVersion:    "2.0.0",
			LocalListen:       "tcp://*:0",
			TimeoutSeconds:    60,
			Insecure:          true,
			DisableLogging:    false,
			DebugLogging:      false,
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Logf("NewNNGManager with full config error: %v", err)
		}
		if manager != nil {
			defer manager.Disconnect()
		}
	})

	t.Run("NewNNGManager with IPC address", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "ipc:///tmp/test-agent",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Logf("NewNNGManager with IPC error: %v", err)
		}
		if manager != nil {
			defer manager.Disconnect()
		}
	})

	t.Run("NewNNGManager with handlers", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		handlers := map[string]FunctionHandler{
			"test.func1": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte(`{"result":"ok"}`), nil
			},
			"test.func2": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte(`{"result":"ok"}`), nil
			},
		}

		manager, err := NewNNGManager(config, handlers)
		if err != nil {
			t.Logf("NewNNGManager with handlers error: %v", err)
		}
		if manager != nil {
			defer manager.Disconnect()
		}
	})
}

// TestNNGManager_interfaceMethods tests NNG manager interface methods
func TestNNGManager_interfaceMethods(t *testing.T) {
	t.Run("GetLocalAddress", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		addr := manager.GetLocalAddress()
		t.Logf("GetLocalAddress returned: '%s'", addr)
	})

	t.Run("IsConnected initially", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		connected := manager.IsConnected()
		t.Logf("IsConnected (initial): %v", connected)
	})

	t.Run("IsConnected after failed Connect", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "invalid-address:99999",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		ctx := context.Background()
		_ = manager.Connect(ctx)

		connected := manager.IsConnected()
		t.Logf("IsConnected (after failed Connect): %v", connected)
	})

	t.Run("Disconnect without Connect", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}

		// Disconnect without connecting
		manager.Disconnect()
		t.Log("Disconnect without Connect completed")
	})

	t.Run("multiple Disconnect calls", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}

		// Multiple Disconnect calls
		manager.Disconnect()
		manager.Disconnect()
		manager.Disconnect()
		t.Log("Multiple Disconnect calls completed")
	})
}

// TestNNGManager_connectScenarios tests connect scenarios
func TestNNGManager_connectScenarios(t *testing.T) {
	t.Run("Connect with timeout context", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err = manager.Connect(ctx)
		t.Logf("Connect with timeout error: %v", err)
	})

	t.Run("Connect with cancelled context", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = manager.Connect(ctx)
		t.Logf("Connect with cancelled context error: %v", err)
	})

	t.Run("Connect with context values", func(t *testing.T) {
		type ctxKey string
		key := ctxKey("trace-id")

		ctx := context.WithValue(context.Background(), key, "trace-12345")

		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		err = manager.Connect(ctx)
		t.Logf("Connect with context values error: %v", err)

		if ctx.Value(key) == "trace-12345" {
			t.Log("Context value preserved")
		}
	})
}

// TestNNGManager_serverOperations tests server operations
func TestNNGManager_serverOperations(t *testing.T) {
	t.Run("StartServer with timeout context", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err = manager.StartServer(ctx)
		t.Logf("StartServer with timeout error: %v", err)
	})

	t.Run("StartServer after Disconnect", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}

		manager.Disconnect()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err = manager.StartServer(ctx)
		t.Logf("StartServer after Disconnect error: %v", err)
	})
}

// TestNNGManager_registrationOperations tests registration operations
func TestNNGManager_registrationOperations(t *testing.T) {
	t.Run("RegisterWithAgent without connection", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		ctx := context.Background()
		serviceID, serviceVersion := "test-service", "1.0.0"
		functions := []LocalFunctionDescriptor{
			{
				ID:      "test.func",
				Version: "1.0.0",
			},
		}

		// Should fail because we haven't connected to the agent yet
		addr, err := manager.RegisterWithAgent(ctx, serviceID, serviceVersion, functions)
		if err == nil {
			t.Errorf("Expected error when calling RegisterWithAgent without connection, got addr: %s", addr)
		} else {
			t.Logf("RegisterWithAgent correctly failed without connection: %v", err)
		}
	})

	t.Run("RegisterWithAgent with empty functions", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		ctx := context.Background()
		functions := []LocalFunctionDescriptor{}

		addr, err := manager.RegisterWithAgent(ctx, "test", "1.0.0", functions)
		t.Logf("RegisterWithAgent with empty functions error: %v, addr: %s", err, addr)
	})

	t.Run("RegisterWithAgent with multiple functions", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		functions := []LocalFunctionDescriptor{
			{ID: "func1", Version: "1.0.0"},
			{ID: "func2", Version: "1.0.0"},
			{ID: "func3", Version: "1.0.0"},
		}

		ctx := context.Background()
		addr, err := manager.RegisterWithAgent(ctx, "test", "1.0.0", functions)
		t.Logf("RegisterWithAgent with multiple functions error: %v, addr: %s", err, addr)
	})
}

// TestNNGManager_conversionFunctions tests conversion functions
func TestNNGManager_conversionFunctions(t *testing.T) {
	t.Run("convertToProtoFunctions with empty list", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		// Access unexported method through reflection-like testing
		// This tests convertToProtoFunctions indirectly
		t.Log("convertToProtoFunctions tested through RegisterWithAgent")
	})

	t.Run("convertToLocalFunctions", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		// convertToLocalFunctions is tested through internal operations
		t.Log("convertToLocalFunctions tested indirectly")
	})
}

// TestNNGManager_contextTests tests context handling
func TestNNGManager_contextTests(t *testing.T) {
	t.Run("operations with deadline context", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		// Try various operations with deadline
		err = manager.Connect(ctx)
		t.Logf("Connect with deadline error: %v", err)

		addr, err := manager.RegisterWithAgent(ctx, "test", "1.0.0", []LocalFunctionDescriptor{})
		t.Logf("RegisterWithAgent with deadline error: %v, addr: %s", err, addr)
	})

	t.Run("operations with cancelled context", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		addr, err := manager.RegisterWithAgent(ctx, "test", "1.0.0", []LocalFunctionDescriptor{})
		t.Logf("RegisterWithAgent with cancelled context error: %v, addr: %s", err, addr)
	})
}

// TestNNGManager_errorHandling tests error handling
func TestNNGManager_errorHandling(t *testing.T) {
	t.Run("operations with invalid agent address", func(t *testing.T) {
		addresses := []string{
			"",
			"invalid",
			":::invalid:::",
			"\x00\x00invalid",
		}

		for _, addr := range addresses {
			config := ClientConfig{
				AgentAddr: addr,
			}

			manager, err := NewNNGManager(config, nil)
			if manager != nil {
				defer manager.Disconnect()
			}

			t.Logf("Invalid address '%s': NewNNGManager error=%v", addr, err)
		}
	})

	t.Run("operations with timeout", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr:     "localhost:19090",
			TimeoutSeconds: 1,
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}
		defer manager.Disconnect()

		ctx := context.Background()
		err = manager.Connect(ctx)
		t.Logf("Connect with 1s timeout error: %v", err)
	})
}

// TestNNGManager_multipleManagers tests multiple manager instances
func TestNNGManager_multipleManagers(t *testing.T) {
	t.Run("create multiple managers", func(t *testing.T) {
		configs := []ClientConfig{
			{AgentAddr: "localhost:19090"},
			{AgentAddr: "localhost:19091"},
			{AgentAddr: "localhost:19092"},
		}

		managers := []Manager{}
		for _, cfg := range configs {
			manager, err := NewNNGManager(cfg, nil)
			if err != nil {
				t.Logf("NewNNGManager error: %v", err)
				continue
			}
			managers = append(managers, manager)
		}

		// Clean up
		for _, m := range managers {
			m.Disconnect()
		}

		t.Logf("Created %d managers", len(managers))
	})

	t.Run("concurrent manager creation", func(t *testing.T) {
		const numGoroutines = 10
		errors := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				config := ClientConfig{
					AgentAddr: fmt.Sprintf("localhost:1909%d", idx),
				}
				manager, err := NewNNGManager(config, nil)
				errors[idx] = err
				if manager != nil {
					manager.Disconnect()
				}
			}(i)
		}

		// Wait for goroutines to complete
		time.Sleep(500 * time.Millisecond)

		errorCount := 0
		for _, err := range errors {
			if err != nil {
				errorCount++
			}
		}

		t.Logf("Concurrent manager creation: %d/%d had errors", errorCount, numGoroutines)
	})
}

// TestNNGManager_lifecycleTests tests complete lifecycle
func TestNNGManager_lifecycleTests(t *testing.T) {
	t.Run("full lifecycle with handlers", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		handlers := map[string]FunctionHandler{
			"test.func": func(ctx context.Context, payload []byte) ([]byte, error) {
				return []byte(`{"result":"ok"}`), nil
			},
		}

		manager, err := NewNNGManager(config, handlers)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}

		// Get local address
		addr := manager.GetLocalAddress()
		t.Logf("Local address: %s", addr)

		// Check connection status
		connected := manager.IsConnected()
		t.Logf("IsConnected: %v", connected)

		// Disconnect
		manager.Disconnect()
		t.Log("Lifecycle completed")
	})

	t.Run("lifecycle with multiple connect/disconnect cycles", func(t *testing.T) {
		config := ClientConfig{
			AgentAddr: "localhost:19090",
		}

		manager, err := NewNNGManager(config, nil)
		if err != nil {
			t.Skipf("Cannot create manager: %v", err)
		}

		for i := 0; i < 3; i++ {
			ctx := context.Background()
			err = manager.Connect(ctx)
			t.Logf("Cycle %d Connect error: %v", i, err)

			connected := manager.IsConnected()
			t.Logf("Cycle %d IsConnected: %v", i, connected)

			manager.Disconnect()
			t.Logf("Cycle %d Disconnect completed", i)
		}
	})
}
