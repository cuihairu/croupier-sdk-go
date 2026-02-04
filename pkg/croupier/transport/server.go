// Package transport provides NNG transport layer for Croupier SDK.
package transport

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/rep"
	_ "go.nanomsg.org/mangos/v3/transport/inproc"
	_ "go.nanomsg.org/mangos/v3/transport/tcp"
	_ "go.nanomsg.org/mangos/v3/transport/tlstcp"

	"github.com/cuihairu/croupier/sdks/go/pkg/croupier/protocol"
)

// Handler handles incoming requests and produces responses.
// The Handle method is called for each received request with the
// message ID, request ID, and request body. It should return the response
// body or an error.
type Handler interface {
	// Handle handles an incoming request and returns the response body.
	Handle(ctx context.Context, msgID uint32, reqID uint32, body []byte) (respBody []byte, err error)
}

// HandlerFunc is an adapter to allow the use of ordinary functions as Handlers.
type HandlerFunc func(ctx context.Context, msgID uint32, reqID uint32, body []byte) (respBody []byte, err error)

// Handle calls f(ctx, msgID, reqID, body).
func (f HandlerFunc) Handle(ctx context.Context, msgID uint32, reqID uint32, body []byte) (respBody []byte, err error) {
	return f(ctx, msgID, reqID, body)
}

// Server represents a NNG transport server.
// It uses the Rep protocol for handling request/response communication.
type Server struct {
	sock    mangos.Socket
	config  *Config
	handler Handler
	mu      sync.RWMutex
	closing chan struct{}
	ready   chan struct{} // Closed when server is ready to accept connections
	once    sync.Once
}

// NewServer creates a new NNG server with the given configuration.
func NewServer(config *Config, handler Handler) (*Server, error) {
	sock, err := rep.NewSocket()
	if err != nil {
		return nil, fmt.Errorf("create rep socket: %w", err)
	}

	// Apply TLS configuration
	if !config.Insecure {
		tlsConfig, err := createTLSConfig(config)
		if err != nil {
			sock.Close()
			return nil, fmt.Errorf("create tls config: %w", err)
		}
		if err := sock.SetOption(mangos.OptionTLSConfig, tlsConfig); err != nil {
			sock.Close()
			return nil, fmt.Errorf("set tls config: %w", err)
		}
	}

	// Configure receive timeout
	if config.RecvTimeout > 0 {
		if err := sock.SetOption(mangos.OptionRecvDeadline, config.RecvTimeout); err != nil {
			sock.Close()
			return nil, fmt.Errorf("set receive deadline: %w", err)
		}
	}

	// Listen on address
	listenAddr := listenAddrServer(config)
	if err := sock.Listen(listenAddr); err != nil {
		sock.Close()
		return nil, fmt.Errorf("listen %s: %w", listenAddr, err)
	}

	return &Server{
		sock:    sock,
		config:  config,
		handler: handler,
		closing: make(chan struct{}),
		ready:   make(chan struct{}), // Don't close yet - close in Serve()
	}, nil
}

// Serve starts the server's receive loop.
// It blocks until the context is cancelled or an error occurs.
func (s *Server) Serve(ctx context.Context) error {
	// Close ready channel to signal that Serve is running
	close(s.ready)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.closing:
			return fmt.Errorf("server is closing")
		default:
		}

		// Receive request
		msg, err := s.sock.RecvMsg()
		if err != nil {
			// Check if server is closing
			select {
			case <-s.closing:
				return nil
			default:
			}
			// Continue on temporary errors
			continue
		}

		// Parse request - protocol header is in Body prefix
		_, msgID, reqID, body, err := protocol.ParseMessageFromBody(msg.Body)
		msg.Free()

		if err != nil {
			s.sendError(0, protocol.GetResponseMsgID(msgID), err)
			continue
		}

		// Handle request synchronously (Rep protocol requires response in same goroutine)
		ctx := context.Background()
		respBody, err := s.handler.Handle(ctx, msgID, reqID, body)

		// Create response body with protocol header in Body (mangos REP doesn't preserve custom Header)
		respMsgID := protocol.GetResponseMsgID(msgID)
		var respBodyWithHeader []byte
		if err != nil {
			errorBody := []byte(fmt.Sprintf("{\"error\": \"%s\"}", err.Error()))
			respBodyWithHeader = protocol.NewMessageBody(respMsgID, reqID, errorBody)
		} else {
			respBodyWithHeader = protocol.NewMessageBody(respMsgID, reqID, respBody)
		}

		respMsg := mangos.NewMessage(0)
		respMsg.Body = respBodyWithHeader

		// Send response
		if err := s.sock.SendMsg(respMsg); err != nil {
			// Log error but continue serving
			continue
		}
	}
}

// sendError sends an error response.
func (s *Server) sendError(reqID uint32, msgID uint32, err error) {
	// Create error body
	errorBody := []byte(fmt.Sprintf("{\"error\": \"%s\"}", err.Error()))

	// Create response body with protocol header as prefix
	respBody := protocol.NewMessageBody(msgID, reqID, errorBody)

	respMsg := mangos.NewMessage(0)
	respMsg.Body = respBody
	s.sock.SendMsg(respMsg)
}

// Close closes the server and stops accepting new connections.
func (s *Server) Close() error {
	var closeErr error
	s.once.Do(func() {
		close(s.closing)
		closeErr = s.sock.Close()
	})
	return closeErr
}

// IsClosed returns true if the server has been closed.
func (s *Server) IsClosed() bool {
	select {
	case <-s.closing:
		return true
	default:
		return false
	}
}

// Ready returns a channel that is closed when the server is ready to accept connections.
func (s *Server) Ready() <-chan struct{} {
	return s.ready
}

// listenAddrServer returns the appropriate listen address string for the server.
func listenAddrServer(cfg *Config) string {
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
