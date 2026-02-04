// Package transport provides NNG transport layer for Croupier SDK.
package transport

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/req"
	_ "go.nanomsg.org/mangos/v3/transport/inproc"
	_ "go.nanomsg.org/mangos/v3/transport/ipc"
	_ "go.nanomsg.org/mangos/v3/transport/tcp"
	_ "go.nanomsg.org/mangos/v3/transport/tlstcp"

	"github.com/cuihairu/croupier/sdks/go/pkg/croupier/protocol"
)

// Client represents a NNG transport client.
// It uses the Req/Rep protocol for request/response communication.
type Client struct {
	sock      mangos.Socket
	config    *Config
	addrs     []string // Addresses to try (in order)
	mu        sync.RWMutex
	pending   map[uint32]chan *mangos.Message
	nextReqID uint32
	closing   chan struct{}
	once      sync.Once
}

// NewClient creates a new NNG client with the given configuration.
// It will try to connect to each address in order until one succeeds.
func NewClient(config *Config) (*Client, error) {
	// Build list of addresses to try
	addrs := buildDialAddrs(config)

	var lastErr error
	var sock mangos.Socket
	var err error

	for _, addr := range addrs {
		// Create new socket for each attempt
		sock, err = req.NewSocket()
		if err != nil {
			lastErr = fmt.Errorf("create req socket: %w", err)
			continue
		}

		// Apply TLS configuration
		if !config.Insecure {
			tlsConfig, err := createTLSConfig(config)
			if err != nil {
				sock.Close()
				lastErr = fmt.Errorf("create tls config: %w", err)
				continue
			}
			if err := sock.SetOption(mangos.OptionTLSConfig, tlsConfig); err != nil {
				sock.Close()
				// TLS might not be supported for IPC, skip to next address
				if strings.HasPrefix(addr, "ipc://") {
					lastErr = fmt.Errorf("set tls config: %w", err)
					continue
				}
				lastErr = fmt.Errorf("set tls config: %w", err)
				sock.Close()
				continue
			}
		}

		// Configure send timeout
		if config.SendTimeout > 0 {
			if err := sock.SetOption(mangos.OptionSendDeadline, config.SendTimeout); err != nil {
				sock.Close()
				lastErr = fmt.Errorf("set send deadline: %w", err)
				continue
			}
		}

		// Try to connect
		if err := sock.Dial(addr); err != nil {
			sock.Close()
			lastErr = fmt.Errorf("dial %s: %w", addr, err)
			continue
		}

		// Successfully connected
		client := &Client{
			sock:      sock,
			config:    config,
			addrs:     addrs,
			pending:   make(map[uint32]chan *mangos.Message),
			nextReqID: 1,
			closing:   make(chan struct{}),
		}

		// Start receive loop
		go client.receiveLoop()

		return client, nil
	}

	// All addresses failed
	return nil, fmt.Errorf("failed to connect to any address (last error: %w)", lastErr)
}

// Call sends a request and waits for the response.
// It uses the given msgID to identify the message type and includes
// the request body. The response body is unmarshaled into responseMsg.
func (c *Client) Call(ctx context.Context, msgID uint32, reqBody []byte) (respMsgID uint32, respBody []byte, err error) {
	// Allocate request ID
	c.mu.Lock()
	reqID := c.nextReqID
	c.nextReqID++
	c.mu.Unlock()

	// Create response channel
	respCh := make(chan *mangos.Message, 1)
	c.pending[reqID] = respCh
	defer delete(c.pending, reqID)

	// Create request body with protocol header as prefix
	reqBodyWithHeader := protocol.NewMessageBody(msgID, reqID, reqBody)

	// Create request message (mangos header is managed by the protocol, we use Body for our header)
	reqMsg := mangos.NewMessage(0)
	reqMsg.Body = reqBodyWithHeader

	// Send request
	if err := c.sock.SendMsg(reqMsg); err != nil {
		return 0, nil, fmt.Errorf("send: %w", err)
	}

	// Wait for response
	select {
	case resp := <-respCh:
		_, respMsgID, respReqID, respData, err := protocol.ParseMessageFromBody(resp.Body)
		resp.Free()
		if err != nil {
			return 0, nil, fmt.Errorf("parse response: %w", err)
		}
		if respReqID != reqID {
			return 0, nil, fmt.Errorf("request ID mismatch: expected %d, got %d", reqID, respReqID)
		}
		return respMsgID, respData, nil
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	case <-c.closing:
		return 0, nil, fmt.Errorf("client is closing")
	}
}

// Send sends a message without waiting for a response.
// This is useful for fire-and-forget scenarios or one-way messages.
func (c *Client) Send(msgID uint32, reqID uint32, body []byte) error {
	msg := mangos.NewMessage(0)
	msg.Header = make([]byte, protocol.HeaderSize)
	msg.Header[0] = protocol.Version1
	protocol.PutMsgID(msg.Header[1:4], msgID)
	binary.BigEndian.PutUint32(msg.Header[4:8], reqID)
	msg.Body = body

	if err := c.sock.SendMsg(msg); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

// receiveLoop receives messages from the socket and routes them to pending requests.
func (c *Client) receiveLoop() {
	for {
		select {
		case <-c.closing:
			return
		default:
		}

		msg, err := c.sock.RecvMsg()
		if err != nil {
			// Connection error or closed
			select {
			case <-c.closing:
				return
			default:
			}
			continue
		}

		// Parse protocol header from Body prefix
		_, _, reqID, _, err := protocol.ParseMessageFromBody(msg.Body)
		if err != nil {
			msg.Free()
			continue
		}

		// Route to pending request
		c.mu.RLock()
		ch, ok := c.pending[reqID]
		c.mu.RUnlock()

		if ok {
			select {
			case ch <- msg:
				// Delivered to waiting goroutine
			case <-c.closing:
				msg.Free()
				return
			}
		} else {
			// No pending request for this RequestID, unexpected response
			msg.Free()
		}
	}
}

// Close closes the client connection.
func (c *Client) Close() error {
	var closeErr error
	c.once.Do(func() {
		close(c.closing)
		closeErr = c.sock.Close()
	})
	return closeErr
}

// IsClosed returns true if the client has been closed.
func (c *Client) IsClosed() bool {
	select {
	case <-c.closing:
		return true
	default:
		return false
	}
}

// SetReadDeadline sets the deadline for the next RecvMsg operation.
func (c *Client) SetReadDeadline(deadline time.Duration) error {
	return c.sock.SetOption(mangos.OptionRecvDeadline, deadline)
}
