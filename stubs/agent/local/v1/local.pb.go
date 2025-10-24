package localv1

import (
    "context"
    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

// Messages
type LocalFunctionDescriptor struct {
    Id       string `json:"id,omitempty"`
    Version  string `json:"version,omitempty"`
}

type RegisterLocalRequest struct {
    ServiceId string `json:"service_id,omitempty"`
    Version   string `json:"version,omitempty"`
    RpcAddr   string `json:"rpc_addr,omitempty"`
    Functions []*LocalFunctionDescriptor `json:"functions,omitempty"`
}
type RegisterLocalResponse struct { SessionId string `json:"session_id,omitempty"` }

type HeartbeatRequest struct { ServiceId string `json:"service_id,omitempty"`; SessionId string `json:"session_id,omitempty"` }
type HeartbeatResponse struct{}

// Service
type LocalControlServiceServer interface {
    RegisterLocal(ctx context.Context, in *RegisterLocalRequest) (*RegisterLocalResponse, error)
    Heartbeat(ctx context.Context, in *HeartbeatRequest) (*HeartbeatResponse, error)
    ListLocal(ctx context.Context, in *ListLocalRequest) (*ListLocalResponse, error)
}
type UnimplementedLocalControlServiceServer struct{}
func (*UnimplementedLocalControlServiceServer) RegisterLocal(context.Context, *RegisterLocalRequest) (*RegisterLocalResponse, error) { return nil, status.Errorf(codes.Unimplemented, "method RegisterLocal not implemented") }
func (*UnimplementedLocalControlServiceServer) Heartbeat(context.Context, *HeartbeatRequest) (*HeartbeatResponse, error) { return nil, status.Errorf(codes.Unimplemented, "method Heartbeat not implemented") }

func RegisterLocalControlServiceServer(s *grpc.Server, srv LocalControlServiceServer) {
    s.RegisterService(&grpc.ServiceDesc{
        ServiceName: "croupier.agent.local.v1.LocalControlService",
        HandlerType: (*LocalControlServiceServer)(nil),
        Methods: []grpc.MethodDesc{{MethodName: "RegisterLocal", Handler: _LocalControlService_RegisterLocal_Handler}, {MethodName: "Heartbeat", Handler: _LocalControlService_Heartbeat_Handler}, {MethodName: "ListLocal", Handler: _LocalControlService_ListLocal_Handler}},
        Streams: []grpc.StreamDesc{},
        Metadata: "local.proto",
    }, srv)
}

func _LocalControlService_RegisterLocal_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
    in := new(RegisterLocalRequest)
    if err := dec(in); err != nil { return nil, err }
    if interceptor == nil { return srv.(LocalControlServiceServer).RegisterLocal(ctx, in) }
    info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/croupier.agent.local.v1.LocalControlService/RegisterLocal"}
    handler := func(ctx context.Context, req interface{}) (interface{}, error) { return srv.(LocalControlServiceServer).RegisterLocal(ctx, req.(*RegisterLocalRequest)) }
    return interceptor(ctx, in, info, handler)
}
func _LocalControlService_Heartbeat_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
    in := new(HeartbeatRequest)
    if err := dec(in); err != nil { return nil, err }
    if interceptor == nil { return srv.(LocalControlServiceServer).Heartbeat(ctx, in) }
    info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/croupier.agent.local.v1.LocalControlService/Heartbeat"}
    handler := func(ctx context.Context, req interface{}) (interface{}, error) { return srv.(LocalControlServiceServer).Heartbeat(ctx, req.(*HeartbeatRequest)) }
    return interceptor(ctx, in, info, handler)
}

// List local instances
type LocalInstance struct { ServiceId string `json:"service_id,omitempty"`; Addr string `json:"addr,omitempty"`; Version string `json:"version,omitempty"`; LastSeen string `json:"last_seen,omitempty"` }
type LocalFunction struct { Id string `json:"id,omitempty"`; Instances []*LocalInstance `json:"instances,omitempty"` }
type ListLocalRequest struct{}
type ListLocalResponse struct { Functions []*LocalFunction `json:"functions,omitempty"` }

func _LocalControlService_ListLocal_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
    in := new(ListLocalRequest)
    if err := dec(in); err != nil { return nil, err }
    if interceptor == nil { return srv.(LocalControlServiceServer).ListLocal(ctx, in) }
    info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/croupier.agent.local.v1.LocalControlService/ListLocal"}
    handler := func(ctx context.Context, req interface{}) (interface{}, error) { return srv.(LocalControlServiceServer).ListLocal(ctx, req.(*ListLocalRequest)) }
    return interceptor(ctx, in, info, handler)
}

// Client
type LocalControlServiceClient interface {
    RegisterLocal(ctx context.Context, in *RegisterLocalRequest, opts ...grpc.CallOption) (*RegisterLocalResponse, error)
    Heartbeat(ctx context.Context, in *HeartbeatRequest, opts ...grpc.CallOption) (*HeartbeatResponse, error)
    ListLocal(ctx context.Context, in *ListLocalRequest, opts ...grpc.CallOption) (*ListLocalResponse, error)
}
type localControlServiceClient struct { cc grpc.ClientConnInterface }
func NewLocalControlServiceClient(cc grpc.ClientConnInterface) LocalControlServiceClient { return &localControlServiceClient{cc} }
func (c *localControlServiceClient) RegisterLocal(ctx context.Context, in *RegisterLocalRequest, opts ...grpc.CallOption) (*RegisterLocalResponse, error) {
    out := new(RegisterLocalResponse)
    if err := c.cc.Invoke(ctx, "/croupier.agent.local.v1.LocalControlService/RegisterLocal", in, out, opts...); err != nil { return nil, err }
    return out, nil
}
func (c *localControlServiceClient) Heartbeat(ctx context.Context, in *HeartbeatRequest, opts ...grpc.CallOption) (*HeartbeatResponse, error) {
    out := new(HeartbeatResponse)
    if err := c.cc.Invoke(ctx, "/croupier.agent.local.v1.LocalControlService/Heartbeat", in, out, opts...); err != nil { return nil, err }
    return out, nil
}
func (c *localControlServiceClient) ListLocal(ctx context.Context, in *ListLocalRequest, opts ...grpc.CallOption) (*ListLocalResponse, error) {
    out := new(ListLocalResponse)
    if err := c.cc.Invoke(ctx, "/croupier.agent.local.v1.LocalControlService/ListLocal", in, out, opts...); err != nil { return nil, err }
    return out, nil
}
