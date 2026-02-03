// Package transport provides NNG transport layer for Croupier SDK.
// This package wraps mangos functionality for sending/receiving messages
// with the Croupier protocol.
package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds the configuration for the transport layer.
type Config struct {
	// Address is the server address to connect to.
	Address string

	// TLS configuration
	Insecure   bool
	CAFile     string
	CertFile   string
	KeyFile    string
	ServerName string

	// Timeouts
	RecvTimeout time.Duration
	SendTimeout time.Duration

	// Backpressure
	ReadQLen  int // receive queue length (default 128)
	WriteQLen int // send queue length (default 64)
}

// DefaultConfig returns a default configuration for the transport layer.
func DefaultConfig() *Config {
	return &Config{
		Address:     "127.0.0.1:19090",
		Insecure:    true,
		RecvTimeout: 30 * time.Second,
		SendTimeout: 5 * time.Second,
		ReadQLen:    128,
		WriteQLen:   64,
	}
}

// createTLSConfig creates a tls.Config from the given configuration.
func createTLSConfig(cfg *Config) (*tls.Config, error) {
	if cfg.Insecure {
		return nil, nil // insecure mode, no TLS config needed
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Load CA certificate
	if cfg.CAFile != "" {
		caPEM, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to append CA certificate")
		}
		tlsConfig.RootCAs = caPool
	}

	// Load client certificate for mTLS
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Configure server name verification
	if cfg.ServerName != "" {
		tlsConfig.ServerName = cfg.ServerName
	}

	return tlsConfig, nil
}

// dialAddr returns the appropriate address string for dialing.
func dialAddr(cfg *Config) string {
	// If address already has a scheme prefix, use it as-is
	if strings.HasPrefix(cfg.Address, "inproc://") ||
		strings.HasPrefix(cfg.Address, "ipc:") ||
		strings.HasPrefix(cfg.Address, "tcp:") ||
		strings.HasPrefix(cfg.Address, "tls+tcp:") ||
		strings.HasPrefix(cfg.Address, "ws:") {
		return cfg.Address
	}

	if !cfg.Insecure {
		return "tls+tcp://" + cfg.Address
	}
	return "tcp://" + cfg.Address
}
