package functionv1

import (
    "context"
    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

type InvokeRequest struct {
    FunctionId string `json:"function_id,omitempty"`
    IdempotencyKey string `json:"idempotency_key,omitempty"`
    Payload []byte `json:"payload,omitempty"`
    Metadata map[string]string `json:"metadata,omitempty"`
}
func (x *InvokeRequest) GetFunctionId() string { if x==nil {return ""}; return x.FunctionId }
func (x *InvokeRequest) GetPayload() []byte { if x==nil {return nil}; return x.Payload }

type InvokeResponse struct { Payload []byte `json:"payload,omitempty"` }
func (x *InvokeResponse) GetPayload() []byte { if x==nil {return nil}; return x.Payload }

type StartJobResponse struct { JobId string `json:"job_id,omitempty"` }
func (x *StartJobResponse) GetJobId() string { if x==nil {return ""}; return x.JobId }

type JobStreamRequest struct { JobId string `json:"job_id,omitempty"` }

type JobEvent struct { Type string `json:"type,omitempty"`; Message string `json:"message,omitempty"`; Progress int32 `json:"progress,omitempty"`; Payload []byte `json:"payload,omitempty"` }

type CancelJobRequest struct { JobId string `json:"job_id,omitempty"` }
func (x *CancelJobRequest) GetJobId() string { if x==nil {return ""}; return x.JobId }

// Service
type FunctionServiceServer interface {
    Invoke(context.Context, *InvokeRequest) (*InvokeResponse, error)
    StartJob(context.Context, *InvokeRequest) (*StartJobResponse, error)
    StreamJob(*JobStreamRequest, FunctionService_StreamJobServer) error
    CancelJob(context.Context, *CancelJobRequest) (*StartJobResponse, error)
}

type UnimplementedFunctionServiceServer struct{}
func (*UnimplementedFunctionServiceServer) Invoke(context.Context, *InvokeRequest) (*InvokeResponse, error) { return nil, status.Errorf(codes.Unimplemented, "method Invoke not implemented") }
func (*UnimplementedFunctionServiceServer) StartJob(context.Context, *InvokeRequest) (*StartJobResponse, error) { return nil, status.Errorf(codes.Unimplemented, "method StartJob not implemented") }
func (*UnimplementedFunctionServiceServer) StreamJob(*JobStreamRequest, FunctionService_StreamJobServer) error { return status.Errorf(codes.Unimplemented, "method StreamJob not implemented") }
func (*UnimplementedFunctionServiceServer) CancelJob(context.Context, *CancelJobRequest) (*StartJobResponse, error) { return nil, status.Errorf(codes.Unimplemented, "method CancelJob not implemented") }

func RegisterFunctionServiceServer(s *grpc.Server, srv FunctionServiceServer) {
    s.RegisterService(&grpc.ServiceDesc{
        ServiceName: "croupier.function.v1.FunctionService",
        HandlerType: (*FunctionServiceServer)(nil),
        Methods: []grpc.MethodDesc{{MethodName: "Invoke", Handler: _FunctionService_Invoke_Handler}, {MethodName: "StartJob", Handler: _FunctionService_StartJob_Handler}, {MethodName: "CancelJob", Handler: _FunctionService_CancelJob_Handler}},
        Streams: []grpc.StreamDesc{{StreamName: "StreamJob", Handler: _FunctionService_StreamJob_Handler, ServerStreams: true}},
    }, srv)
}
func _FunctionService_Invoke_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
    in := new(InvokeRequest); if err := dec(in); err != nil { return nil, err }
    if interceptor == nil { return srv.(FunctionServiceServer).Invoke(ctx, in) }
    info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/croupier.function.v1.FunctionService/Invoke"}
    handler := func(ctx context.Context, req interface{}) (interface{}, error) { return srv.(FunctionServiceServer).Invoke(ctx, req.(*InvokeRequest)) }
    return interceptor(ctx, in, info, handler)
}
func _FunctionService_StartJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
    in := new(InvokeRequest); if err := dec(in); err != nil { return nil, err }
    if interceptor == nil { return srv.(FunctionServiceServer).StartJob(ctx, in) }
    info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/croupier.function.v1.FunctionService/StartJob"}
    handler := func(ctx context.Context, req interface{}) (interface{}, error) { return srv.(FunctionServiceServer).StartJob(ctx, req.(*InvokeRequest)) }
    return interceptor(ctx, in, info, handler)
}
type FunctionService_StreamJobServer interface { Send(*JobEvent) error; grpc.ServerStream }
func _FunctionService_StreamJob_Handler(srv interface{}, stream grpc.ServerStream) error { m := new(JobStreamRequest); if err := stream.RecvMsg(m); err != nil { return err }; return srv.(FunctionServiceServer).StreamJob(m, &functionServiceStreamJobServer{stream}) }
type functionServiceStreamJobServer struct{ grpc.ServerStream }
func (x *functionServiceStreamJobServer) Send(m *JobEvent) error { return x.ServerStream.SendMsg(m) }

func _FunctionService_CancelJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
    in := new(CancelJobRequest); if err := dec(in); err != nil { return nil, err }
    if interceptor == nil { return srv.(FunctionServiceServer).CancelJob(ctx, in) }
    info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/croupier.function.v1.FunctionService/CancelJob"}
    handler := func(ctx context.Context, req interface{}) (interface{}, error) { return srv.(FunctionServiceServer).CancelJob(ctx, req.(*CancelJobRequest)) }
    return interceptor(ctx, in, info, handler)
}

// Client
type FunctionServiceClient interface {
    Invoke(ctx context.Context, in *InvokeRequest, opts ...grpc.CallOption) (*InvokeResponse, error)
    StartJob(ctx context.Context, in *InvokeRequest, opts ...grpc.CallOption) (*StartJobResponse, error)
    StreamJob(ctx context.Context, in *JobStreamRequest, opts ...grpc.CallOption) (FunctionService_StreamJobClient, error)
    CancelJob(ctx context.Context, in *CancelJobRequest, opts ...grpc.CallOption) (*StartJobResponse, error)
}
type functionServiceClient struct { cc grpc.ClientConnInterface }
func NewFunctionServiceClient(cc grpc.ClientConnInterface) FunctionServiceClient { return &functionServiceClient{cc} }
func (c *functionServiceClient) Invoke(ctx context.Context, in *InvokeRequest, opts ...grpc.CallOption) (*InvokeResponse, error) {
    out := new(InvokeResponse)
    err := c.cc.Invoke(ctx, "/croupier.function.v1.FunctionService/Invoke", in, out, opts...)
    if err != nil { return nil, err }
    return out, nil
}
func (c *functionServiceClient) StartJob(ctx context.Context, in *InvokeRequest, opts ...grpc.CallOption) (*StartJobResponse, error) {
    out := new(StartJobResponse)
    err := c.cc.Invoke(ctx, "/croupier.function.v1.FunctionService/StartJob", in, out, opts...)
    if err != nil { return nil, err }
    return out, nil
}
type FunctionService_StreamJobClient interface { Recv() (*JobEvent, error); grpc.ClientStream }
func (c *functionServiceClient) StreamJob(ctx context.Context, in *JobStreamRequest, opts ...grpc.CallOption) (FunctionService_StreamJobClient, error) {
    sd := &grpc.StreamDesc{ServerStreams: true}
    stream, err := c.cc.NewStream(ctx, sd, "/croupier.function.v1.FunctionService/StreamJob", opts...)
    if err != nil { return nil, err }
    if err := stream.SendMsg(in); err != nil { return nil, err }
    if err := stream.CloseSend(); err != nil { return nil, err }
    return &functionServiceStreamJobClient{stream}, nil
}
type functionServiceStreamJobClient struct { grpc.ClientStream }
func (x *functionServiceStreamJobClient) Recv() (*JobEvent, error) { m := new(JobEvent); if err := x.ClientStream.RecvMsg(m); err != nil { return nil, err }; return m, nil }
func (c *functionServiceClient) CancelJob(ctx context.Context, in *CancelJobRequest, opts ...grpc.CallOption) (*StartJobResponse, error) {
    out := new(StartJobResponse)
    err := c.cc.Invoke(ctx, "/croupier.function.v1.FunctionService/CancelJob", in, out, opts...)
    if err != nil { return nil, err }
    return out, nil
}
