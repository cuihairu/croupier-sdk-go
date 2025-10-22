package localv1

import (
    "context"
    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

type LocalFunctionDescriptor struct { Id string `json:"id,omitempty"`; Version string `json:"version,omitempty"` }
type RegisterLocalRequest struct {
    ServiceId string `json:"service_id,omitempty"`
    Version   string `json:"version,omitempty"`
    RpcAddr   string `json:"rpc_addr,omitempty"`
    Functions []*LocalFunctionDescriptor `json:"functions,omitempty"`
}
type RegisterLocalResponse struct { SessionId string `json:"session_id,omitempty"` }
type HeartbeatRequest struct { ServiceId string `json:"service_id,omitempty"`; SessionId string `json:"session_id,omitempty"` }
type HeartbeatResponse struct{}

type LocalControlServiceServer interface {
    RegisterLocal(context.Context, *RegisterLocalRequest) (*RegisterLocalResponse, error)
    Heartbeat(context.Context, *HeartbeatRequest) (*HeartbeatResponse, error)
}
type UnimplementedLocalControlServiceServer struct{}
func (*UnimplementedLocalControlServiceServer) RegisterLocal(context.Context, *RegisterLocalRequest) (*RegisterLocalResponse, error) { return nil, status.Errorf(codes.Unimplemented, "method RegisterLocal not implemented") }
func (*UnimplementedLocalControlServiceServer) Heartbeat(context.Context, *HeartbeatRequest) (*HeartbeatResponse, error) { return nil, status.Errorf(codes.Unimplemented, "method Heartbeat not implemented") }

func RegisterLocalControlServiceServer(s *grpc.Server, srv LocalControlServiceServer) {
    s.RegisterService(&grpc.ServiceDesc{ServiceName: "croupier.agent.local.v1.LocalControlService", HandlerType: (*LocalControlServiceServer)(nil), Methods: []grpc.MethodDesc{{MethodName: "RegisterLocal", Handler: _LocalControlService_RegisterLocal_Handler}, {MethodName: "Heartbeat", Handler: _LocalControlService_Heartbeat_Handler}}}, srv)
}
func _LocalControlService_RegisterLocal_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) { in := new(RegisterLocalRequest); if err := dec(in); err != nil { return nil, err }; if interceptor == nil { return srv.(LocalControlServiceServer).RegisterLocal(ctx, in) }; info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/croupier.agent.local.v1.LocalControlService/RegisterLocal"}; handler := func(ctx context.Context, req interface{}) (interface{}, error) { return srv.(LocalControlServiceServer).RegisterLocal(ctx, req.(*RegisterLocalRequest)) }; return interceptor(ctx, in, info, handler) }
func _LocalControlService_Heartbeat_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) { in := new(HeartbeatRequest); if err := dec(in); err != nil { return nil, err }; if interceptor == nil { return srv.(LocalControlServiceServer).Heartbeat(ctx, in) }; info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/croupier.agent.local.v1.LocalControlService/Heartbeat"}; handler := func(ctx context.Context, req interface{}) (interface{}, error) { return srv.(LocalControlServiceServer).Heartbeat(ctx, req.(*HeartbeatRequest)) }; return interceptor(ctx, in, info, handler) }

// Client
type LocalControlServiceClient interface {
    RegisterLocal(context.Context, *RegisterLocalRequest, ...grpc.CallOption) (*RegisterLocalResponse, error)
    Heartbeat(context.Context, *HeartbeatRequest, ...grpc.CallOption) (*HeartbeatResponse, error)
}
type localControlServiceClient struct { cc grpc.ClientConnInterface }
func NewLocalControlServiceClient(cc grpc.ClientConnInterface) LocalControlServiceClient { return &localControlServiceClient{cc} }
func (c *localControlServiceClient) RegisterLocal(ctx context.Context, in *RegisterLocalRequest, opts ...grpc.CallOption) (*RegisterLocalResponse, error) { out := new(RegisterLocalResponse); if err := c.cc.Invoke(ctx, "/croupier.agent.local.v1.LocalControlService/RegisterLocal", in, out, opts...); err != nil { return nil, err }; return out, nil }
func (c *localControlServiceClient) Heartbeat(ctx context.Context, in *HeartbeatRequest, opts ...grpc.CallOption) (*HeartbeatResponse, error) { out := new(HeartbeatResponse); if err := c.cc.Invoke(ctx, "/croupier.agent.local.v1.LocalControlService/Heartbeat", in, out, opts...); err != nil { return nil, err }; return out, nil }

