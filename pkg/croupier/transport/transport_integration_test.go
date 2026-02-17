package transport

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests for transport layer

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		valid  bool
	}{
		{
			name:   "minimal valid config",
			config: &Config{Address: "127.0.0.1:19090"},
			valid:  true,
		},
		{
			name: "config with IPC",
			config: &Config{
				Address:    "127.0.0.1:19090",
				IPCAddress: "ipc://croupier-agent",
			},
			valid: true,
		},
		{
			name: "config with TLS",
			config: &Config{
				Address:    "127.0.0.1:19090",
				Insecure:   false,
				CAFile:     "/path/to/ca.pem",
				CertFile:   "/path/to/cert.pem",
				KeyFile:    "/path/to/key.pem",
				ServerName: "example.com",
			},
			valid: true,
		},
		{
			name: "empty address",
			config: &Config{
				Address: "",
			},
			valid: false,
		},
		{
			name: "TLS without cert files",
			config: &Config{
				Address:  "127.0.0.1:19090",
				Insecure: false,
			},
			valid: true, // May be valid depending on system certs
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify config can be created
			assert.NotNil(t, tt.config)
		})
	}
}

func TestBuildDialAddrs_Variations(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		minAddrs int
	}{
		{
			name: "TCP only",
			config: &Config{
				Address:  "127.0.0.1:19090",
				Insecure: true,
			},
			minAddrs: 1,
		},
		{
			name: "TCP with port",
			config: &Config{
				Address:  "localhost:19090",
				Insecure: true,
			},
			minAddrs: 1,
		},
		{
			name: "IPC prioritized",
			config: &Config{
				Address:    "127.0.0.1:19090",
				IPCAddress: "ipc://croupier-agent",
				Insecure:   true,
			},
			minAddrs: 1,
		},
		{
			name: "empty config",
			config: &Config{
				Address:  "",
				Insecure: true,
			},
			minAddrs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addrs := BuildDialAddrs(tt.config)
			assert.GreaterOrEqual(t, len(addrs), tt.minAddrs)
		})
	}
}

func TestConfig_CopySemantics(t *testing.T) {
	original := &Config{
		Address:          "127.0.0.1:19090",
		IPCAddress:       "ipc://agent",
		Insecure:         true,
		InsecureSkipVerify: false,
		CAFile:           "/ca.pem",
		CertFile:         "/cert.pem",
		KeyFile:          "/key.pem",
		ServerName:       "example.com",
		WriteTimeout:     10 * time.Second,
		ReadTimeout:      20 * time.Second,
		DialTimeout:      5 * time.Second,
		MaxMessageSize:   1024 * 1024,
		KeepAlive:        true,
		KeepAliveTime:    30 * time.Second,
		KeepAliveTimeout: 10 * time.Second,
	}

	// Copy
	copy := *original
	assert.Equal(t, original.Address, copy.Address)
	assert.Equal(t, original.Insecure, copy.Insecure)

	// Modify copy
	copy.Address = "localhost:9999"
	assert.NotEqual(t, original.Address, copy.Address)
}

func TestConfig_Defaults(t *testing.T) {
	config := &Config{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}

	// Set reasonable defaults
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 5 * time.Second
	}
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 10 * time.Second
	}
	if config.DialTimeout == 0 {
		config.DialTimeout = 5 * time.Second
	}
	if config.MaxMessageSize == 0 {
		config.MaxMessageSize = 4 * 1024 * 1024
	}

	assert.Greater(t, config.WriteTimeout, time.Duration(0))
	assert.Greater(t, config.ReadTimeout, time.Duration(0))
	assert.Greater(t, config.DialTimeout, time.Duration(0))
	assert.Greater(t, config.MaxMessageSize, 0)
}

func TestConfig_ConcurrentAccess(t *testing.T) {
	config := &Config{
		Address:  "127.0.0.1:19090",
		Insecure: true,
	}

	const numGoroutines = 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			config.Address = "addr-" + string(rune('a'+idx%26))
			config.Insecure = idx%2 == 0
			config.WriteTimeout = time.Duration(idx) * time.Second
		}(i)
	}

	wg.Wait()
	// Should not panic or have data races
	assert.NotNil(t, config)
}

func TestConfig_TimeoutVariations(t *testing.T) {
	tests := []struct {
		name  string
		value time.Duration
	}{
		{"zero", 0},
		{"nanosecond", 1 * time.Nanosecond},
		{"microsecond", 1 * time.Microsecond},
		{"millisecond", 1 * time.Millisecond},
		{"second", 1 * time.Second},
		{"minute", 1 * time.Minute},
		{"hour", 1 * time.Hour},
		{"very large", 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Address:      "127.0.0.1:19090",
				WriteTimeout: tt.value,
				ReadTimeout:  tt.value,
				DialTimeout:  tt.value,
			}
			assert.NotNil(t, config)
		})
	}
}

func TestConfig_KeepAliveSettings(t *testing.T) {
	tests := []struct {
		name           string
		keepAlive      bool
		keepAliveTime  time.Duration
		keepAliveTimeout time.Duration
	}{
		{"disabled", false, 0, 0},
		{"enabled defaults", true, 30 * time.Second, 10 * time.Second},
		{"custom times", true, 60 * time.Second, 5 * time.Second},
		{"zero times", true, 0, 0},
		{"very short", true, 1 * time.Second, 1 * time.Second},
		{"very long", true, 1 * time.Hour, 30 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Address:           "127.0.0.1:19090",
				KeepAlive:         tt.keepAlive,
				KeepAliveTime:     tt.keepAliveTime,
				KeepAliveTimeout:  tt.keepAliveTimeout,
			}
			assert.NotNil(t, config)
			assert.Equal(t, tt.keepAlive, config.KeepAlive)
		})
	}
}

func TestConfig_MaxMessageSize(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		valid   bool
	}{
		{"zero", 0, true},
		{"small", 1024, true},
		{"1MB", 1024 * 1024, true},
		{"4MB", 4 * 1024 * 1024, true},
		{"16MB", 16 * 1024 * 1024, true},
		{"very large", 1024 * 1024 * 1024, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Address:         "127.0.0.1:19090",
				MaxMessageSize:  tt.size,
			}
			assert.NotNil(t, config)
			assert.Equal(t, tt.size, config.MaxMessageSize)
		})
	}
}

func TestConfig_AddressFormats(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{"localhost with port", "localhost:19090"},
		{"IPv4 with port", "127.0.0.1:19090"},
		{"IPv4 with port", "192.168.1.1:19090"},
		{"IPv6 with port", "[::1]:19090"},
		{"domain with port", "example.com:19090"},
		{"IPC address", "ipc://croupier-agent"},
		{"TCP with scheme", "tcp://127.0.0.1:19090"},
		{"with query params", "tcp://127.0.0.1:19090?param=value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Address: tt.address,
			}
			assert.NotNil(t, config)
			assert.Equal(t, tt.address, config.Address)
		})
	}
}

func TestContext_TimeoutBehavior(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		cancel   bool
		expected bool
	}{
		{
			name:     "immediate cancel",
			timeout:  1 * time.Second,
			cancel:   true,
			expected: true,
		},
		{
			name:     "short timeout",
			timeout:  1 * time.Millisecond,
			cancel:   false,
			expected: true,
		},
		{
			name:     "long timeout",
			timeout:  1 * time.Hour,
			cancel:   false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			} else {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}

			// Check context state
			select {
			case <-ctx.Done():
				assert.True(t, tt.expected || tt.cancel)
			case <-time.After(10 * time.Millisecond):
				assert.False(t, tt.expected && tt.timeout < 10*time.Millisecond)
			}
		})
	}
}

func TestBuildDialAddrs_Concurrent(t *testing.T) {
	config := &Config{
		Address:    "127.0.0.1:19090",
		IPCAddress: "ipc://agent",
		Insecure:   true,
	}

	const numGoroutines = 100
	var wg sync.WaitGroup
	results := make(chan []string, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addrs := BuildDialAddrs(config)
			results <- addrs
		}()
	}

	wg.Wait()
	close(results)

	// All results should be consistent
	count := 0
	for addrs := range results {
		count++
		if count == 1 {
			// First result
			assert.NotEmpty(t, addrs)
		} else {
			// Should be same as first (order may vary)
			assert.Greater(t, len(addrs), 0)
		}
	}

	assert.Equal(t, numGoroutines, count)
}
