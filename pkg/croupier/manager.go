// Package croupier provides a Go SDK for Croupier game function registration and execution.
package croupier

import (
	"context"
)

// Manager handles communication with the agent using NNG transport
type Manager interface {
	// Connect establishes connection to the agent
	Connect(ctx context.Context) error

	// Disconnect closes the connection
	Disconnect()

	// RegisterWithAgent registers functions with the agent
	RegisterWithAgent(ctx context.Context, serviceID, serviceVersion string, functions []LocalFunctionDescriptor) (string, error)

	// StartServer starts the local server for handling incoming RPC calls
	StartServer(ctx context.Context) error

	// GetLocalAddress returns the local server address
	GetLocalAddress() string

	// IsConnected returns true if connected to agent
	IsConnected() bool
}

// ManagerConfig holds configuration for creating a Manager
type ManagerConfig struct {
	// AgentAddr is the address of the Croupier agent
	AgentAddr string

	// LocalListen is the address to listen on for local RPC calls
	LocalListen string

	// TimeoutSeconds is the timeout for RPC calls
	TimeoutSeconds int

	// Insecure disables TLS
	Insecure bool

	// CAFile is the path to the CA certificate
	CAFile string

	// CertFile is the path to the client certificate
	CertFile string

	// KeyFile is the path to the client key
	KeyFile string

	// ServerName is the server name for TLS verification
	ServerName string

	// InsecureSkipVerify skips TLS verification (not recommended)
	InsecureSkipVerify bool
}

// NewManager creates a new NNG-based Manager
func NewManager(config ManagerConfig, handlers map[string]FunctionHandler) (Manager, error) {
	clientConfig := ClientConfig{
		AgentAddr:          config.AgentAddr,
		LocalListen:        config.LocalListen,
		TimeoutSeconds:     config.TimeoutSeconds,
		Insecure:           config.Insecure,
		CAFile:             config.CAFile,
		CertFile:           config.CertFile,
		KeyFile:            config.KeyFile,
		ServerName:         config.ServerName,
		InsecureSkipVerify: config.InsecureSkipVerify,
	}
	return NewNNGManager(clientConfig, handlers)
}
