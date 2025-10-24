package sdk

import (
    "context"
    "crypto/tls"
    "crypto/x509"
    "errors"
    "io/ioutil"
    "encoding/json"
    "time"
    "os"
    "strings"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/metadata"

    functionv1 "github.com/cuihairu/croupier-sdk-go/stubs/function/v1"
)

// InvokerConfig configures a client for calling Core/Agent FunctionService.
type InvokerConfig struct {
    Address   string
    Insecure  bool
    // mTLS (if not insecure)
    CertFile  string
    KeyFile   string
    CAFile    string
    ServerName string
    // Defaults
    Timeout   time.Duration
    // Optional auth header
    AuthToken string
    // Default metadata
    GameID string
    Env    string
    // Default outgoing headers (metadata) and a dynamic provider
    Headers   map[string]string
    HeaderFunc func(ctx context.Context) map[string]string
}

// Invoker wraps a FunctionService client connection.
type Invoker struct {
    conn *grpc.ClientConn
    cli  functionv1.FunctionServiceClient
    cfg  InvokerConfig
    schemas map[string]map[string]any
}

// NewInvoker dials the given address and returns an invoker.
func NewInvoker(ctx context.Context, cfg InvokerConfig) (*Invoker, error) {
    if cfg.Address == "" { return nil, errors.New("missing address") }
    var credOpt grpc.DialOption
    if cfg.Insecure {
        credOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
    } else {
        if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
            return nil, errors.New("missing TLS cert/key/ca for secure connection; set Insecure=true for dev")
        }
        c, err := loadClientTLS(cfg.CertFile, cfg.KeyFile, cfg.CAFile, cfg.ServerName)
        if err != nil { return nil, err }
        credOpt = grpc.WithTransportCredentials(c)
    }
    if cfg.Timeout <= 0 { cfg.Timeout = 5 * time.Second }
    // default to JSON codec via content subtype
    dialOpts := []grpc.DialOption{ credOpt, grpc.WithDefaultCallOptions(grpc.CallContentSubtype("json")) }
    ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
    defer cancel()
    cc, err := grpc.DialContext(ctx, cfg.Address, dialOpts...)
    if err != nil { return nil, err }
    return &Invoker{conn: cc, cli: functionv1.NewFunctionServiceClient(cc), cfg: cfg, schemas: map[string]map[string]any{}}, nil
}

// Close closes the underlying connection.
func (i *Invoker) Close() error { if i.conn != nil { return i.conn.Close() }; return nil }

// InvokeOption controls per-call metadata.
type InvokeOption func(*functionv1.InvokeRequest)

// WithIdempotency sets idempotency key.
func WithIdempotency(k string) InvokeOption { return func(r *functionv1.InvokeRequest){ r.IdempotencyKey = k } }
// WithRoute sets route semantics: lb|broadcast|targeted|hash
func WithRoute(v string) InvokeOption { return func(r *functionv1.InvokeRequest){ meta(r)["route"]=v } }
// WithTargetService sets targeted routing service id.
func WithTargetService(id string) InvokeOption { return func(r *functionv1.InvokeRequest){ meta(r)["target_service_id"]=id } }
// WithHashKey sets routing hash key.
func WithHashKey(k string) InvokeOption { return func(r *functionv1.InvokeRequest){ meta(r)["hash_key"]=k } }
// WithTrace sets trace id.
func WithTrace(t string) InvokeOption { return func(r *functionv1.InvokeRequest){ meta(r)["trace_id"]=t } }
// WithMeta merges custom metadata.
func WithMeta(k, v string) InvokeOption { return func(r *functionv1.InvokeRequest){ meta(r)[k]=v } }

func meta(r *functionv1.InvokeRequest) map[string]string {
    if r.Metadata == nil { r.Metadata = map[string]string{} }
    return r.Metadata
}

// Invoke calls a function and returns raw payload.
func (i *Invoker) Invoke(ctx context.Context, functionID string, payload []byte, opts ...InvokeOption) ([]byte, error) {
    req := &functionv1.InvokeRequest{FunctionId: functionID, Payload: payload}
    // defaults
    if i.cfg.GameID != "" { meta(req)["game_id"]=i.cfg.GameID }
    if i.cfg.Env != "" { meta(req)["env"]=i.cfg.Env }
    // apply options
    for _, o := range opts { o(req) }
    // optional client-side validation
    if sch := i.schemas[functionID]; sch != nil {
        var val any
        if len(payload) > 0 {
            if err := json.Unmarshal(payload, &val); err != nil { return nil, err }
        }
        if err := ValidateJSON(sch, val); err != nil { return nil, err }
    }
    // set auth header if provided (best-effort via context)
    ctx = i.attachHeaders(ctx, nil)
    resp, err := i.cli.Invoke(ctx, req)
    if err != nil { return nil, err }
    return resp.GetPayload(), nil
}

// StartJob starts a long-running job and returns job id.
func (i *Invoker) StartJob(ctx context.Context, functionID string, payload []byte, opts ...InvokeOption) (string, error) {
    req := &functionv1.InvokeRequest{FunctionId: functionID, Payload: payload}
    if i.cfg.GameID != "" { meta(req)["game_id"]=i.cfg.GameID }
    if i.cfg.Env != "" { meta(req)["env"]=i.cfg.Env }
    for _, o := range opts { o(req) }
    if sch := i.schemas[functionID]; sch != nil {
        var val any
        if len(payload) > 0 {
            if err := json.Unmarshal(payload, &val); err != nil { return "", err }
        }
        if err := ValidateJSON(sch, val); err != nil { return "", err }
    }
    ctx = i.attachHeaders(ctx, nil)
    resp, err := i.cli.StartJob(ctx, req)
    if err != nil { return "", err }
    return resp.GetJobId(), nil
}

// StreamJob subscribes events for a job id. Caller should read from the returned channel until closed.
func (i *Invoker) StreamJob(ctx context.Context, jobID string) (<-chan *functionv1.JobEvent, error) {
    if jobID == "" { return nil, errors.New("missing job id") }
    ctx = i.attachHeaders(ctx, nil)
    stream, err := i.cli.StreamJob(ctx, &functionv1.JobStreamRequest{JobId: jobID})
    if err != nil { return nil, err }
    ch := make(chan *functionv1.JobEvent, 16)
    go func(){ defer close(ch); for { ev, err := stream.Recv(); if err != nil { return }; ch <- ev } }()
    return ch, nil
}

// CancelJob cancels a job by id.
func (i *Invoker) CancelJob(ctx context.Context, jobID string) error {
    if jobID == "" { return errors.New("missing job id") }
    ctx = i.attachHeaders(ctx, nil)
    _, err := i.cli.CancelJob(ctx, &functionv1.CancelJobRequest{JobId: jobID})
    return err
}

// SetSchema sets a JSON schema for a function id; used for client-side validation.
func (i *Invoker) SetSchema(functionID string, schema map[string]any) { if i.schemas == nil { i.schemas = map[string]map[string]any{} }; i.schemas[functionID] = schema }

// helper: build TransportCredentials for TLS
func loadClientTLS(certFile, keyFile, caFile string, serverName string) (credentials.TransportCredentials, error) {
    cert, err := tls.LoadX509KeyPair(certFile, keyFile)
    if err != nil { return nil, err }
    caPEM, err := ioutil.ReadFile(caFile)
    if err != nil { return nil, err }
    pool := x509.NewCertPool()
    if !pool.AppendCertsFromPEM(caPEM) { return nil, errors.New("failed to append CA") }
    cfg := &tls.Config{ Certificates: []tls.Certificate{cert}, RootCAs: pool, ServerName: serverName }
    return credentials.NewTLS(cfg), nil
}

// attach bearer auth to context (using simple outgoing metadata via grpc), best-effort without importing grpc/metadata to keep stubs light
func withAuth(ctx context.Context, token string) context.Context {
    md := metadata.Pairs("authorization", "Bearer "+token)
    return metadata.NewOutgoingContext(ctx, md)
}

// attachHeaders composes default headers, dynamic headers, auth token and extra headers into outgoing metadata.
func (i *Invoker) attachHeaders(ctx context.Context, extra map[string]string) context.Context {
    headers := map[string]string{}
    // defaults
    for k, v := range i.cfg.Headers { headers[strings.ToLower(k)] = v }
    if i.cfg.AuthToken != "" { headers["authorization"] = "Bearer "+i.cfg.AuthToken }
    if i.cfg.HeaderFunc != nil {
        if m := i.cfg.HeaderFunc(ctx); m != nil { for k, v := range m { headers[strings.ToLower(k)] = v } }
    }
    for k, v := range extra { headers[strings.ToLower(k)] = v }
    if len(headers) == 0 { return ctx }
    pairs := []string{}
    for k, v := range headers { pairs = append(pairs, k, v) }
    md := metadata.Pairs(pairs...)
    return metadata.NewOutgoingContext(ctx, md)
}

// SetSchemasFromDir loads descriptor JSON files under dir and registers schema to invoker for client-side validation.
func (i *Invoker) SetSchemasFromDir(dir string) error {
    entries, err := os.ReadDir(dir)
    if err != nil { return err }
    for _, e := range entries {
        if e.IsDir() { continue }
        if !strings.HasSuffix(e.Name(), ".json") { continue }
        d, err := LoadDescriptor(dir+"/"+e.Name())
        if err != nil { return err }
        if d != nil && d.Params != nil { i.SetSchema(d.ID, d.Params) }
    }
    return nil
}
