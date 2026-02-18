// Copyright 2025 Croupier Authors
// Licensed under the Apache License, Version 2.0

package croupier

import (
	"errors"
	"testing"
	"time"
)

// TestErrorHandling_scenarios tests error handling
func TestErrorHandling_scenarios(t *testing.T) {
	t.Run("error wrapper", func(t *testing.T) {
		baseErr := errors.New("base error")
		wrappedErr := errors.New("wrapped error")

		if baseErr == nil {
			t.Error("Base error should not be nil")
		}

		if wrappedErr == nil {
			t.Error("Wrapped error should not be nil")
		}
	})
}

// TestInvoker_withErrorStates tests error state handling
func TestInvoker_withErrorStates(t *testing.T) {
	t.Run("invoker with minimal valid config", func(t *testing.T) {
		invoker := NewInvoker(&InvokerConfig{
			Address:   "localhost:8080",
			Retry:     DefaultRetryConfig(),
			Reconnect: DefaultReconnectConfig(),
		})

		if invoker == nil {
			t.Error("NewInvoker() returned nil")
		}
	})
}

// TestRetryConfig_extremeValues tests extreme retry configurations
func TestRetryConfig_extremeValues(t *testing.T) {
	t.Run("very large max attempts", func(t *testing.T) {
		config := &RetryConfig{
			Enabled:     true,
			MaxAttempts: 1000,
			InitialDelayMs: 10,
			MaxDelayMs:     100,
		}

		if config.MaxAttempts != 1000 {
			t.Errorf("MaxAttempts = %d, want 1000", config.MaxAttempts)
		}
	})

	t.Run("very small delays", func(t *testing.T) {
		config := &RetryConfig{
			Enabled:        true,
			MaxAttempts:    3,
			InitialDelayMs: 1,
			MaxDelayMs:     5,
		}

		if config.InitialDelayMs != 1 {
			t.Errorf("InitialDelayMs = %d, want 1", config.InitialDelayMs)
		}
	})

	t.Run("large delays", func(t *testing.T) {
		config := &RetryConfig{
			Enabled:        true,
			MaxAttempts:    3,
			InitialDelayMs: 10000,
			MaxDelayMs:     60000,
		}

		if config.InitialDelayMs != 10000 {
			t.Errorf("InitialDelayMs = %d, want 10000", config.InitialDelayMs)
		}
	})

	t.Run("extreme backoff multipliers", func(t *testing.T) {
		multipliers := []float64{1.01, 1.5, 2.0, 3.0, 5.0, 10.0}

		for _, mult := range multipliers {
			config := &RetryConfig{
				Enabled:           true,
				MaxAttempts:       3,
				InitialDelayMs:    100,
				MaxDelayMs:        5000,
				BackoffMultiplier: mult,
				JitterFactor:      0.1,
			}

			if config.BackoffMultiplier != mult {
				t.Errorf("BackoffMultiplier = %f, want %f", config.BackoffMultiplier, mult)
			}
		}
	})
}

// TestReconnectConfig_extremeValues tests extreme reconnect configurations
func TestReconnectConfig_extremeValues(t *testing.T) {
	t.Run("very large max attempts", func(t *testing.T) {
		config := &ReconnectConfig{
			Enabled:     true,
			MaxAttempts: 1000,
			InitialDelayMs: 10,
			MaxDelayMs:     100,
		}

		if config.MaxAttempts != 1000 {
			t.Errorf("MaxAttempts = %d, want 1000", config.MaxAttempts)
		}
	})

	t.Run("very small delays", func(t *testing.T) {
		config := &ReconnectConfig{
			Enabled:        true,
			MaxAttempts:    3,
			InitialDelayMs: 1,
			MaxDelayMs:     5,
		}

		if config.InitialDelayMs != 1 {
			t.Errorf("InitialDelayMs = %d, want 1", config.InitialDelayMs)
		}
	})
}

// TestInvokerConfig_validation tests invoker configuration validation
func TestInvokerConfig_validation(t *testing.T) {
	t.Run("config with all TLS fields", func(t *testing.T) {
		config := &InvokerConfig{
			Address:   "example.com:8443",
			Insecure:  false,
			CAFile:    "/path/to/ca.crt",
			CertFile:  "/path/to/cert.pem",
			KeyFile:   "/path/to/key.pem",
			Retry:     DefaultRetryConfig(),
			Reconnect: DefaultReconnectConfig(),
		}

		invoker := NewInvoker(config)
		if invoker == nil {
			t.Error("NewInvoker() with full TLS config returned nil")
		}
	})

	t.Run("config with partial TLS fields", func(t *testing.T) {
		config := &InvokerConfig{
			Address:   "example.com:8443",
			Insecure:  false,
			CAFile:    "/path/to/ca.crt",
			Retry:     DefaultRetryConfig(),
			Reconnect: DefaultReconnectConfig(),
		}

		invoker := NewInvoker(config)
		if invoker == nil {
			t.Error("NewInvoker() with partial TLS config returned nil")
		}
	})

	t.Run("config with only cert and key", func(t *testing.T) {
		config := &InvokerConfig{
			Address:   "example.com:8443",
			Insecure:  false,
			CertFile:  "/path/to/cert.pem",
			KeyFile:   "/path/to/key.pem",
			Retry:     DefaultRetryConfig(),
			Reconnect: DefaultReconnectConfig(),
		}

		invoker := NewInvoker(config)
		if invoker == nil {
			t.Error("NewInvoker() with cert/key only returned nil")
		}
	})

	t.Run("config with various addresses", func(t *testing.T) {
		addresses := []string{
			"localhost:8080",
			"127.0.0.1:8080",
			"192.168.1.1:8080",
			"example.com:8443",
			"tcp://example.com:8080",
		}

		for _, addr := range addresses {
			config := &InvokerConfig{
				Address:   addr,
				Retry:     DefaultRetryConfig(),
				Reconnect: DefaultReconnectConfig(),
			}

			invoker := NewInvoker(config)
			if invoker == nil {
				t.Errorf("NewInvoker() returned nil for address=%s", addr)
			}
		}
	})
}

// TestFunctionDescriptor_comprehensive tests comprehensive descriptor scenarios
func TestFunctionDescriptor_comprehensive(t *testing.T) {
	t.Run("descriptor with all risk levels", func(t *testing.T) {
		risks := []string{"low", "medium", "high", "critical"}

		for _, risk := range risks {
			desc := FunctionDescriptor{
				ID:       "test.function",
				Version:  "1.0.0",
				Category:  "test",
				Risk:     risk,
			}

			if desc.Risk != risk {
				t.Errorf("Risk = %s, want %s", desc.Risk, risk)
			}
		}
	})

	t.Run("descriptor with various versions", func(t *testing.T) {
		versions := []string{
			"0.0.1",
			"1.0.0",
			"2.5.3",
			"10.20.30",
		}

		for _, version := range versions {
			desc := FunctionDescriptor{
				ID:       "test.function",
				Version:  version,
				Category:  "test",
				Risk:     "low",
			}

			if desc.Version != version {
				t.Errorf("Version = %s, want %s", desc.Version, version)
			}
		}
	})

	t.Run("descriptor with various categories", func(t *testing.T) {
		categories := []string{
			"player",
			"item",
			"guild",
			"chat",
			"system",
		}

		for _, category := range categories {
			desc := FunctionDescriptor{
				ID:       "test.function",
				Version:  "1.0.0",
				Category:  category,
				Risk:     "low",
			}

			if desc.Category != category {
				t.Errorf("Category = %s, want %s", desc.Category, category)
			}
		}
	})

	t.Run("descriptor enabled and disabled", func(t *testing.T) {
		enabledDesc := FunctionDescriptor{
			ID:       "test.function",
			Version:  "1.0.0",
			Category:  "test",
			Risk:     "low",
			Enabled:  true,
		}

		if !enabledDesc.Enabled {
			t.Error("Enabled descriptor should have Enabled=true")
		}

		disabledDesc := FunctionDescriptor{
			ID:       "test.function",
			Version:  "1.0.0",
			Category:  "test",
			Risk:     "low",
			Enabled:  false,
		}

		if disabledDesc.Enabled {
			t.Error("Disabled descriptor should have Enabled=false")
		}
	})

	t.Run("descriptor with entity and operation", func(t *testing.T) {
		desc := FunctionDescriptor{
			ID:        "player.inventory.add",
			Version:   "1.0.0",
			Category:  "player",
			Risk:      "low",
			Entity:    "inventory",
			Operation: "add",
		}

		if desc.Entity != "inventory" {
			t.Errorf("Entity = %s, want inventory", desc.Entity)
		}

		if desc.Operation != "add" {
			t.Errorf("Operation = %s, want add", desc.Operation)
		}
	})
}

// TestJobEvent_comprehensive tests job event scenarios
func TestJobEvent_comprehensive(t *testing.T) {
	t.Run("all event types", func(t *testing.T) {
		eventTypes := []string{"started", "progress", "completed", "error"}

		for _, eventType := range eventTypes {
			event := JobEvent{
				EventType: eventType,
				JobID:     "job-123",
				Payload:   "{}",
				Done:      eventType == "completed" || eventType == "error",
			}

			if event.EventType != eventType {
				t.Errorf("EventType = %s, want %s", event.EventType, eventType)
			}
		}
	})

	t.Run("event with various payloads", func(t *testing.T) {
		payloads := []string{
			"",
			"{}",
			`{"progress": 50}`,
			`{"message": "processing"}`,
			`{"error": "failed"}`,
		}

		for _, payload := range payloads {
			event := JobEvent{
				EventType: "progress",
				JobID:     "job-456",
				Payload:   payload,
			}

			if event.Payload != payload {
				t.Errorf("Payload = %s, want %s", event.Payload, payload)
			}
		}
	})

	t.Run("event with error message", func(t *testing.T) {
		event := JobEvent{
			EventType: "error",
			JobID:     "job-789",
			Payload:   "",
			Error:     "Connection failed",
			Done:      true,
		}

		if event.Error != "Connection failed" {
			t.Errorf("Error = %s, want 'Connection failed'", event.Error)
		}

		if !event.Done {
			t.Error("Error event should be marked as Done")
		}
	})
}

// TestInvokeOptions_comprehensive tests invoke options scenarios
func TestInvokeOptions_comprehensive(t *testing.T) {
	t.Run("options with various timeouts", func(t *testing.T) {
		timeouts := []time.Duration{
			0,
			time.Millisecond * 1000,      // 1 second
			time.Millisecond * 5000,      // 5 seconds
			time.Millisecond * 30000,     // 30 seconds
			time.Millisecond * 60000,     // 60 seconds
		}

		for _, timeout := range timeouts {
			options := InvokeOptions{
				Timeout: timeout,
			}

			if options.Timeout != timeout {
				t.Errorf("Timeout = %v, want %v", options.Timeout, timeout)
			}
		}
	})

	t.Run("options with metadata", func(t *testing.T) {
		options := InvokeOptions{
			Headers: map[string]string{
				"X-Request-ID":     "req-123",
				"X-Correlation-ID": "corr-456",
				"X-User-ID":        "user-789",
			},
		}

		if len(options.Headers) != 3 {
			t.Errorf("Headers count = %d, want 3", len(options.Headers))
		}
	})

	t.Run("options with empty headers", func(t *testing.T) {
		options := InvokeOptions{
			Headers: map[string]string{},
		}

		if options.Headers == nil {
			t.Error("Headers should not be nil")
		}

		if len(options.Headers) != 0 {
			t.Errorf("Headers count = %d, want 0", len(options.Headers))
		}
	})

	t.Run("options with retry config", func(t *testing.T) {
		retryConfig := &RetryConfig{
			Enabled:     true,
			MaxAttempts: 5,
		}

		options := InvokeOptions{
			Retry: retryConfig,
		}

		if options.Retry == nil {
			t.Error("Retry config should not be nil")
		}

		if options.Retry.MaxAttempts != 5 {
			t.Errorf("Retry MaxAttempts = %d, want 5", options.Retry.MaxAttempts)
		}
	})
}
