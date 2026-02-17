package transport

import (
	"crypto/tls"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "127.0.0.1:19090", cfg.Address)
	assert.True(t, cfg.Insecure)
	assert.Equal(t, 30*time.Second, cfg.RecvTimeout)
	assert.Equal(t, 5*time.Second, cfg.SendTimeout)
	assert.Equal(t, 128, cfg.ReadQLen)
	assert.Equal(t, 64, cfg.WriteQLen)
}

func TestCreateTLSConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "insecure mode returns nil",
			config: &Config{
				Insecure: true,
			},
			wantErr: false,
		},
		{
			name: "secure mode without files",
			config: &Config{
				Insecure: false,
			},
			wantErr: false,
		},
		{
			name: "invalid CA file path",
			config: &Config{
				Insecure: false,
				CAFile:   "/nonexistent/ca.pem",
			},
			wantErr: true,
		},
		{
			name: "invalid cert file path",
			config: &Config{
				Insecure: false,
				CertFile: "/nonexistent/cert.pem",
				KeyFile:  "/nonexistent/key.pem",
			},
			wantErr: true,
		},
		{
			name: "cert without key",
			config: &Config{
				Insecure: false,
				CertFile: "/nonexistent/cert.pem",
			},
			wantErr: false, // Only creates config, doesn't load files
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsConfig, err := createTLSConfig(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, tlsConfig)
			} else {
				assert.NoError(t, err)
				if tt.config.Insecure {
					assert.Nil(t, tlsConfig)
				} else {
					assert.NotNil(t, tlsConfig)
				}
			}
		})
	}
}

func TestBuildDialAddrs(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected []string
	}{
		{
			name: "default config",
			config: &Config{
				Address:  "127.0.0.1:19090",
				Insecure: true,
			},
			expected: []string{"tcp://127.0.0.1:19090"},
		},
		{
			name: "explicit addresses",
			config: &Config{
				Addresses: []string{"addr1:19090", "addr2:19090"},
				Insecure:  true,
			},
			expected: []string{"tcp://addr1:19090", "tcp://addr2:19090"},
		},
		{
			name: "IPC address prioritized",
			config: &Config{
				Address:    "127.0.0.1:19090",
				IPCAddress: "croupier-agent",
				Insecure:   true,
			},
			expected: func() []string {
				// IPC is supported on most platforms
				if isIPCSupported() {
					return []string{"ipc://croupier-agent", "tcp://127.0.0.1:19090"}
				}
				return []string{"tcp://127.0.0.1:19090"}
			}(),
		},
		{
			name: "multiple comma-separated addresses",
			config: &Config{
				Address:  "addr1:19090, addr2:19090, addr3:19090",
				Insecure: true,
			},
			expected: []string{"tcp://addr1:19090", "tcp://addr2:19090", "tcp://addr3:19090"},
		},
		{
			name: "empty addresses defaults to TCP",
			config: &Config{
				Address:  "",
				Insecure: true,
			},
			expected: []string{"tcp://127.0.0.1:19090"},
		},
		{
			name: "TLS address for secure mode",
			config: &Config{
				Address:  "example.com:19090",
				Insecure: false,
			},
			expected: []string{"tls+tcp://example.com:19090"},
		},
		{
			name: "addresses with whitespace trimmed",
			config: &Config{
				Addresses: []string{" addr1:19090 ", " addr2:19090"},
				Insecure:  true,
			},
			expected: []string{"tcp://addr1:19090", "tcp://addr2:19090"},
		},
		{
			name: "skip empty addresses",
			config: &Config{
				Addresses: []string{"addr1:19090", "", "addr2:19090"},
				Insecure:  true,
			},
			expected: []string{"tcp://addr1:19090", "tcp://addr2:19090"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildDialAddrs(tt.config)

			// For most cases, check that we get the expected addresses
			if tt.expected != nil {
				assert.Equal(t, tt.expected, result)
			} else {
				assert.NotEmpty(t, result)
			}
		})
	}
}

func TestNormalizeAddress(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		insecure bool
		expected string
	}{
		{
			name:     "plain address with insecure",
			addr:     "127.0.0.1:19090",
			insecure: true,
			expected: "tcp://127.0.0.1:19090",
		},
		{
			name:     "plain address with secure",
			addr:     "example.com:19090",
			insecure: false,
			expected: "tls+tcp://example.com:19090",
		},
		{
			name:     "already has tcp://",
			addr:     "tcp://127.0.0.1:19090",
			insecure: true,
			expected: "tcp://127.0.0.1:19090",
		},
		{
			name:     "already has tls+tcp://",
			addr:     "tls+tcp://example.com:19090",
			insecure: false,
			expected: "tls+tcp://example.com:19090",
		},
		{
			name:     "IPC address",
			addr:     "ipc://croupier-agent",
			insecure: true,
			expected: "ipc://croupier-agent",
		},
		{
			name:     "inproc address",
			addr:     "inproc://test",
			insecure: true,
			expected: "inproc://test",
		},
		{
			name:     "ws address",
			addr:     "ws://localhost:8080",
			insecure: true,
			expected: "ws://localhost:8080",
		},
		{
			name:     "wss address",
			addr:     "wss://localhost:8080",
			insecure: false,
			expected: "wss://localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeAddress(tt.addr, tt.insecure)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsIPCSupported(t *testing.T) {
	// This should return true for most platforms
	result := isIPCSupported()
	// We can't assert the exact value since it depends on the platform
	// but we can verify it returns a boolean without panicking
	assert.IsType(t, true, result)
}

func TestDialAddr(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "simple address",
			config: &Config{
				Address:  "localhost:19090",
				Insecure: true,
			},
		},
		{
			name: "IPC address",
			config: &Config{
				IPCAddress: "croupier-agent",
				Address:    "localhost:19090",
				Insecure:   true,
			},
		},
		{
			name: "multiple addresses",
			config: &Config{
				Address:  "addr1:19090,addr2:19090",
				Insecure: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dialAddr(tt.config)
			assert.NotEmpty(t, result)
			assert.True(t, strings.HasPrefix(result, "tcp://") ||
				strings.HasPrefix(result, "ipc://") ||
				strings.HasPrefix(result, "tls+tcp://"))
		})
	}
}

func TestConfigWithAllFields(t *testing.T) {
	cfg := &Config{
		Address:     "example.com:19090",
		IPCAddress:  "croupier-agent",
		Insecure:    false,
		CAFile:      "/path/to/ca.pem",
		CertFile:    "/path/to/cert.pem",
		KeyFile:     "/path/to/key.pem",
		ServerName:  "example.com",
		RecvTimeout: 60 * time.Second,
		SendTimeout: 10 * time.Second,
		ReadQLen:    256,
		WriteQLen:   128,
	}

	assert.Equal(t, "example.com:19090", cfg.Address)
	assert.Equal(t, "croupier-agent", cfg.IPCAddress)
	assert.False(t, cfg.Insecure)
	assert.Equal(t, "/path/to/ca.pem", cfg.CAFile)
	assert.Equal(t, "/path/to/cert.pem", cfg.CertFile)
	assert.Equal(t, "/path/to/key.pem", cfg.KeyFile)
	assert.Equal(t, "example.com", cfg.ServerName)
	assert.Equal(t, 60*time.Second, cfg.RecvTimeout)
	assert.Equal(t, 10*time.Second, cfg.SendTimeout)
	assert.Equal(t, 256, cfg.ReadQLen)
	assert.Equal(t, 128, cfg.WriteQLen)
}

func TestBuildDialAddrs_IPCNotDuplicated(t *testing.T) {
	cfg := &Config{
		Address:    "croupier-agent,127.0.0.1:19090",
		IPCAddress: "croupier-agent",
		Insecure:   true,
	}

	addrs := buildDialAddrs(cfg)

	// Should not duplicate the IPC address
	ipcCount := 0
	for _, addr := range addrs {
		if strings.Contains(addr, "croupier-agent") {
			ipcCount++
		}
	}

	assert.LessOrEqual(t, ipcCount, 1)
}

func TestNormalizeAddress_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		insecure bool
	}{
		{
			name:     "empty string defaults to TCP",
			addr:     "",
			insecure: true,
		},
		{
			name:     "address with port only",
			addr:     ":19090",
			insecure: true,
		},
		{
			name:     "IPv6 address",
			addr:     "[::1]:19090",
			insecure: true,
		},
		{
			name:     "IPv6 address with brackets",
			addr:     "[2001:db8::1]:19090",
			insecure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeAddress(tt.addr, tt.insecure)
			assert.NotEmpty(t, result)
		})
	}
}

func TestCreateTLSConfig_ServerName(t *testing.T) {
	cfg := &Config{
		Insecure:   false,
		ServerName: "custom.example.com",
	}

	tlsConfig, err := createTLSConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, tlsConfig)

	assert.Equal(t, "custom.example.com", tlsConfig.ServerName)
}

func TestCreateTLSConfig_MinVersion(t *testing.T) {
	cfg := &Config{
		Insecure: false,
	}

	tlsConfig, err := createTLSConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, tlsConfig)

	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
}
