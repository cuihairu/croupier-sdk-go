package sdk

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "net"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/keepalive"

    functionv1 "github.com/cuihairu/croupier-sdk-go/stubs/function/v1"
    localv1 "github.com/cuihairu/croupier-sdk-go/stubs/agent/local/v1"
    "github.com/cuihairu/croupier-sdk-go/transport/interceptors"
)

// ClientConfig defines SDK client options.
type ClientConfig struct {
    Addr        string // agent address
    LocalListen string // local listener, e.g. 127.0.0.1:0
    ServiceID   string // service identifier (for targeted routing)
    ServiceVersion string // service version (optional)
}

// Client hosts handlers locally and registers to Agent.
type Client struct {
    cfg  ClientConfig
    conn *grpc.ClientConn
    l    *localServer
}

func NewClient(cfg ClientConfig) *Client { return &Client{cfg: cfg} }

// RegisterFunction registers a handler and optional schema.
func (c *Client) RegisterFunction(desc Function, h Handler) error {
    if c.l == nil {
        if c.cfg.LocalListen == "" { c.cfg.LocalListen = "127.0.0.1:0" }
        c.l = &localServer{
            listen: c.cfg.LocalListen,
            functions: map[string]string{},
            handlers: map[string]Handler{},
            schemas: map[string]map[string]any{},
            serviceID: func(){ if c.cfg.ServiceID=="" { c.cfg.ServiceID = "svc-1" } }();
        }
        // set defaults after struct literal
        c.l.serviceID = c.cfg.ServiceID
        c.l.version = c.cfg.ServiceVersion
        if c.l.version == "" { c.l.version = "0.1.0" }
    }
    c.l.functions[desc.ID] = desc.Version
    c.l.handlers[desc.ID] = h
    if desc.Schema != nil { c.l.schemas[desc.ID] = desc.Schema }
    return nil
}

func (c *Client) Connect(ctx context.Context) error {
    // connect to Agent
    base := []grpc.DialOption{
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 30 * time.Second}),
        grpc.WithDefaultCallOptions(grpc.CallContentSubtype("json")),
    }
    opts := append(base, interceptors.Chain(nil)...)
    cc, err := grpc.DialContext(ctx, c.cfg.Addr, opts...)
    if err != nil { return err }
    c.conn = cc
    // start local server
    if c.l != nil {
        if err := c.l.start(); err != nil { return err }
        // register to Agent
        cli := localv1.NewLocalControlServiceClient(c.conn)
        var fns []*localv1.LocalFunctionDescriptor
        for fid, ver := range c.l.functions { fns = append(fns, &localv1.LocalFunctionDescriptor{Id: fid, Version: ver}) }
        if _, err := cli.RegisterLocal(ctx, &localv1.RegisterLocalRequest{ServiceId: c.l.serviceID, Version: c.l.version, RpcAddr: c.l.addr, Functions: fns}); err != nil { return fmt.Errorf("register local: %w", err) }
    }
    return nil
}

func (c *Client) Close() error { if c.conn != nil { return c.conn.Close() }; return nil }

// Types
type Handler func(ctx context.Context, payload []byte) ([]byte, error)
type Function struct { ID, Version string; Schema map[string]any }

// idempotency helper
func NewIdempotencyKey() string { b:=make([]byte,16);_,_ = rand.Read(b); return hex.EncodeToString(b) }

// local server hosting handlers
type localServer struct {
    listen string
    addr   string
    functions map[string]string
    handlers  map[string]Handler
    schemas   map[string]map[string]any
    serviceID string
    version   string
}

func (s *localServer) start() error {
    ln, err := net.Listen("tcp", s.listen)
    if err != nil { return err }
    s.addr = ln.Addr().String()
    srv := grpc.NewServer()
    functionv1.RegisterFunctionServiceServer(srv, s)
    go func(){ _ = srv.Serve(ln) }()
    return nil
}

// implement function service
func (s *localServer) Invoke(ctx context.Context, req *functionv1.InvokeRequest) (*functionv1.InvokeResponse, error) {
    h, ok := s.handlers[req.GetFunctionId()]
    if !ok { return nil, fmt.Errorf("unknown function: %s", req.GetFunctionId()) }
    out, err := h(ctx, req.GetPayload())
    if err != nil { return nil, err }
    return &functionv1.InvokeResponse{Payload: out}, nil
}
func (s *localServer) StartJob(ctx context.Context, req *functionv1.InvokeRequest) (*functionv1.StartJobResponse, error) {
    return &functionv1.StartJobResponse{JobId: "job-"+req.GetFunctionId()}, nil
}
func (s *localServer) StreamJob(req *functionv1.JobStreamRequest, stream functionv1.FunctionService_StreamJobServer) error { return nil }
