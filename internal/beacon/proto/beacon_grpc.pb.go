// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.20.2
// source: beacon.proto

package proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// BeaconServiceClient is the client API for BeaconService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type BeaconServiceClient interface {
	LockForNamespace(ctx context.Context, opts ...grpc.CallOption) (BeaconService_LockForNamespaceClient, error)
	LockForBuild(ctx context.Context, opts ...grpc.CallOption) (BeaconService_LockForBuildClient, error)
	LockForContainerSetup(ctx context.Context, opts ...grpc.CallOption) (BeaconService_LockForContainerSetupClient, error)
	AcquireContainerLock(ctx context.Context, opts ...grpc.CallOption) (BeaconService_AcquireContainerLockClient, error)
	Interrupt(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type beaconServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewBeaconServiceClient(cc grpc.ClientConnInterface) BeaconServiceClient {
	return &beaconServiceClient{cc}
}

func (c *beaconServiceClient) LockForNamespace(ctx context.Context, opts ...grpc.CallOption) (BeaconService_LockForNamespaceClient, error) {
	stream, err := c.cc.NewStream(ctx, &BeaconService_ServiceDesc.Streams[0], "/proto.BeaconService/LockForNamespace", opts...)
	if err != nil {
		return nil, err
	}
	x := &beaconServiceLockForNamespaceClient{stream}
	return x, nil
}

type BeaconService_LockForNamespaceClient interface {
	Send(*LockRequest) error
	Recv() (*LockResponse, error)
	grpc.ClientStream
}

type beaconServiceLockForNamespaceClient struct {
	grpc.ClientStream
}

func (x *beaconServiceLockForNamespaceClient) Send(m *LockRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *beaconServiceLockForNamespaceClient) Recv() (*LockResponse, error) {
	m := new(LockResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *beaconServiceClient) LockForBuild(ctx context.Context, opts ...grpc.CallOption) (BeaconService_LockForBuildClient, error) {
	stream, err := c.cc.NewStream(ctx, &BeaconService_ServiceDesc.Streams[1], "/proto.BeaconService/LockForBuild", opts...)
	if err != nil {
		return nil, err
	}
	x := &beaconServiceLockForBuildClient{stream}
	return x, nil
}

type BeaconService_LockForBuildClient interface {
	Send(*KeyedLockRequest) error
	Recv() (*LockResponse, error)
	grpc.ClientStream
}

type beaconServiceLockForBuildClient struct {
	grpc.ClientStream
}

func (x *beaconServiceLockForBuildClient) Send(m *KeyedLockRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *beaconServiceLockForBuildClient) Recv() (*LockResponse, error) {
	m := new(LockResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *beaconServiceClient) LockForContainerSetup(ctx context.Context, opts ...grpc.CallOption) (BeaconService_LockForContainerSetupClient, error) {
	stream, err := c.cc.NewStream(ctx, &BeaconService_ServiceDesc.Streams[2], "/proto.BeaconService/LockForContainerSetup", opts...)
	if err != nil {
		return nil, err
	}
	x := &beaconServiceLockForContainerSetupClient{stream}
	return x, nil
}

type BeaconService_LockForContainerSetupClient interface {
	Send(*KeyedLockRequest) error
	Recv() (*LockResponse, error)
	grpc.ClientStream
}

type beaconServiceLockForContainerSetupClient struct {
	grpc.ClientStream
}

func (x *beaconServiceLockForContainerSetupClient) Send(m *KeyedLockRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *beaconServiceLockForContainerSetupClient) Recv() (*LockResponse, error) {
	m := new(LockResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *beaconServiceClient) AcquireContainerLock(ctx context.Context, opts ...grpc.CallOption) (BeaconService_AcquireContainerLockClient, error) {
	stream, err := c.cc.NewStream(ctx, &BeaconService_ServiceDesc.Streams[3], "/proto.BeaconService/AcquireContainerLock", opts...)
	if err != nil {
		return nil, err
	}
	x := &beaconServiceAcquireContainerLockClient{stream}
	return x, nil
}

type BeaconService_AcquireContainerLockClient interface {
	Send(*AcquireLockRequest) error
	Recv() (*AcquireLockResponse, error)
	grpc.ClientStream
}

type beaconServiceAcquireContainerLockClient struct {
	grpc.ClientStream
}

func (x *beaconServiceAcquireContainerLockClient) Send(m *AcquireLockRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *beaconServiceAcquireContainerLockClient) Recv() (*AcquireLockResponse, error) {
	m := new(AcquireLockResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *beaconServiceClient) Interrupt(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/proto.BeaconService/Interrupt", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BeaconServiceServer is the server API for BeaconService service.
// All implementations must embed UnimplementedBeaconServiceServer
// for forward compatibility
type BeaconServiceServer interface {
	LockForNamespace(BeaconService_LockForNamespaceServer) error
	LockForBuild(BeaconService_LockForBuildServer) error
	LockForContainerSetup(BeaconService_LockForContainerSetupServer) error
	AcquireContainerLock(BeaconService_AcquireContainerLockServer) error
	Interrupt(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
	mustEmbedUnimplementedBeaconServiceServer()
}

// UnimplementedBeaconServiceServer must be embedded to have forward compatible implementations.
type UnimplementedBeaconServiceServer struct {
}

func (UnimplementedBeaconServiceServer) LockForNamespace(BeaconService_LockForNamespaceServer) error {
	return status.Errorf(codes.Unimplemented, "method LockForNamespace not implemented")
}
func (UnimplementedBeaconServiceServer) LockForBuild(BeaconService_LockForBuildServer) error {
	return status.Errorf(codes.Unimplemented, "method LockForBuild not implemented")
}
func (UnimplementedBeaconServiceServer) LockForContainerSetup(BeaconService_LockForContainerSetupServer) error {
	return status.Errorf(codes.Unimplemented, "method LockForContainerSetup not implemented")
}
func (UnimplementedBeaconServiceServer) AcquireContainerLock(BeaconService_AcquireContainerLockServer) error {
	return status.Errorf(codes.Unimplemented, "method AcquireContainerLock not implemented")
}
func (UnimplementedBeaconServiceServer) Interrupt(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Interrupt not implemented")
}
func (UnimplementedBeaconServiceServer) mustEmbedUnimplementedBeaconServiceServer() {}

// UnsafeBeaconServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to BeaconServiceServer will
// result in compilation errors.
type UnsafeBeaconServiceServer interface {
	mustEmbedUnimplementedBeaconServiceServer()
}

func RegisterBeaconServiceServer(s grpc.ServiceRegistrar, srv BeaconServiceServer) {
	s.RegisterService(&BeaconService_ServiceDesc, srv)
}

func _BeaconService_LockForNamespace_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(BeaconServiceServer).LockForNamespace(&beaconServiceLockForNamespaceServer{stream})
}

type BeaconService_LockForNamespaceServer interface {
	Send(*LockResponse) error
	Recv() (*LockRequest, error)
	grpc.ServerStream
}

type beaconServiceLockForNamespaceServer struct {
	grpc.ServerStream
}

func (x *beaconServiceLockForNamespaceServer) Send(m *LockResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *beaconServiceLockForNamespaceServer) Recv() (*LockRequest, error) {
	m := new(LockRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _BeaconService_LockForBuild_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(BeaconServiceServer).LockForBuild(&beaconServiceLockForBuildServer{stream})
}

type BeaconService_LockForBuildServer interface {
	Send(*LockResponse) error
	Recv() (*KeyedLockRequest, error)
	grpc.ServerStream
}

type beaconServiceLockForBuildServer struct {
	grpc.ServerStream
}

func (x *beaconServiceLockForBuildServer) Send(m *LockResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *beaconServiceLockForBuildServer) Recv() (*KeyedLockRequest, error) {
	m := new(KeyedLockRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _BeaconService_LockForContainerSetup_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(BeaconServiceServer).LockForContainerSetup(&beaconServiceLockForContainerSetupServer{stream})
}

type BeaconService_LockForContainerSetupServer interface {
	Send(*LockResponse) error
	Recv() (*KeyedLockRequest, error)
	grpc.ServerStream
}

type beaconServiceLockForContainerSetupServer struct {
	grpc.ServerStream
}

func (x *beaconServiceLockForContainerSetupServer) Send(m *LockResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *beaconServiceLockForContainerSetupServer) Recv() (*KeyedLockRequest, error) {
	m := new(KeyedLockRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _BeaconService_AcquireContainerLock_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(BeaconServiceServer).AcquireContainerLock(&beaconServiceAcquireContainerLockServer{stream})
}

type BeaconService_AcquireContainerLockServer interface {
	Send(*AcquireLockResponse) error
	Recv() (*AcquireLockRequest, error)
	grpc.ServerStream
}

type beaconServiceAcquireContainerLockServer struct {
	grpc.ServerStream
}

func (x *beaconServiceAcquireContainerLockServer) Send(m *AcquireLockResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *beaconServiceAcquireContainerLockServer) Recv() (*AcquireLockRequest, error) {
	m := new(AcquireLockRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _BeaconService_Interrupt_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BeaconServiceServer).Interrupt(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.BeaconService/Interrupt",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BeaconServiceServer).Interrupt(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

// BeaconService_ServiceDesc is the grpc.ServiceDesc for BeaconService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var BeaconService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.BeaconService",
	HandlerType: (*BeaconServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Interrupt",
			Handler:    _BeaconService_Interrupt_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "LockForNamespace",
			Handler:       _BeaconService_LockForNamespace_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "LockForBuild",
			Handler:       _BeaconService_LockForBuild_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "LockForContainerSetup",
			Handler:       _BeaconService_LockForContainerSetup_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "AcquireContainerLock",
			Handler:       _BeaconService_AcquireContainerLock_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "beacon.proto",
}
