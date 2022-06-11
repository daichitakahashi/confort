// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.20.1
// source: beacon.proto

package beacon

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
	Register(ctx context.Context, in *RegisterRequest, opts ...grpc.CallOption) (*RegisterResponse, error)
	Deregister(ctx context.Context, in *DeregisterRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	BuildImage(ctx context.Context, in *BuildImageRequest, opts ...grpc.CallOption) (BeaconService_BuildImageClient, error)
	CreateContainer(ctx context.Context, in *CreateContainerRequest, opts ...grpc.CallOption) (BeaconService_CreateContainerClient, error)
	AcquireContainerEndpoint(ctx context.Context, in *AcquireContainerEndpointRequest, opts ...grpc.CallOption) (BeaconService_AcquireContainerEndpointClient, error)
	ReleaseContainer(ctx context.Context, in *ReleaseContainerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type beaconServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewBeaconServiceClient(cc grpc.ClientConnInterface) BeaconServiceClient {
	return &beaconServiceClient{cc}
}

func (c *beaconServiceClient) Register(ctx context.Context, in *RegisterRequest, opts ...grpc.CallOption) (*RegisterResponse, error) {
	out := new(RegisterResponse)
	err := c.cc.Invoke(ctx, "/beacon.BeaconService/Register", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *beaconServiceClient) Deregister(ctx context.Context, in *DeregisterRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/beacon.BeaconService/Deregister", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *beaconServiceClient) BuildImage(ctx context.Context, in *BuildImageRequest, opts ...grpc.CallOption) (BeaconService_BuildImageClient, error) {
	stream, err := c.cc.NewStream(ctx, &BeaconService_ServiceDesc.Streams[0], "/beacon.BeaconService/BuildImage", opts...)
	if err != nil {
		return nil, err
	}
	x := &beaconServiceBuildImageClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type BeaconService_BuildImageClient interface {
	Recv() (*BuildImageResponse, error)
	grpc.ClientStream
}

type beaconServiceBuildImageClient struct {
	grpc.ClientStream
}

func (x *beaconServiceBuildImageClient) Recv() (*BuildImageResponse, error) {
	m := new(BuildImageResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *beaconServiceClient) CreateContainer(ctx context.Context, in *CreateContainerRequest, opts ...grpc.CallOption) (BeaconService_CreateContainerClient, error) {
	stream, err := c.cc.NewStream(ctx, &BeaconService_ServiceDesc.Streams[1], "/beacon.BeaconService/CreateContainer", opts...)
	if err != nil {
		return nil, err
	}
	x := &beaconServiceCreateContainerClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type BeaconService_CreateContainerClient interface {
	Recv() (*CreateContainerResponse, error)
	grpc.ClientStream
}

type beaconServiceCreateContainerClient struct {
	grpc.ClientStream
}

func (x *beaconServiceCreateContainerClient) Recv() (*CreateContainerResponse, error) {
	m := new(CreateContainerResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *beaconServiceClient) AcquireContainerEndpoint(ctx context.Context, in *AcquireContainerEndpointRequest, opts ...grpc.CallOption) (BeaconService_AcquireContainerEndpointClient, error) {
	stream, err := c.cc.NewStream(ctx, &BeaconService_ServiceDesc.Streams[2], "/beacon.BeaconService/AcquireContainerEndpoint", opts...)
	if err != nil {
		return nil, err
	}
	x := &beaconServiceAcquireContainerEndpointClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type BeaconService_AcquireContainerEndpointClient interface {
	Recv() (*AcquireContainerEndpointResponse, error)
	grpc.ClientStream
}

type beaconServiceAcquireContainerEndpointClient struct {
	grpc.ClientStream
}

func (x *beaconServiceAcquireContainerEndpointClient) Recv() (*AcquireContainerEndpointResponse, error) {
	m := new(AcquireContainerEndpointResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *beaconServiceClient) ReleaseContainer(ctx context.Context, in *ReleaseContainerRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/beacon.BeaconService/ReleaseContainer", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BeaconServiceServer is the server API for BeaconService service.
// All implementations must embed UnimplementedBeaconServiceServer
// for forward compatibility
type BeaconServiceServer interface {
	Register(context.Context, *RegisterRequest) (*RegisterResponse, error)
	Deregister(context.Context, *DeregisterRequest) (*emptypb.Empty, error)
	BuildImage(*BuildImageRequest, BeaconService_BuildImageServer) error
	CreateContainer(*CreateContainerRequest, BeaconService_CreateContainerServer) error
	AcquireContainerEndpoint(*AcquireContainerEndpointRequest, BeaconService_AcquireContainerEndpointServer) error
	ReleaseContainer(context.Context, *ReleaseContainerRequest) (*emptypb.Empty, error)
	mustEmbedUnimplementedBeaconServiceServer()
}

// UnimplementedBeaconServiceServer must be embedded to have forward compatible implementations.
type UnimplementedBeaconServiceServer struct {
}

func (UnimplementedBeaconServiceServer) Register(context.Context, *RegisterRequest) (*RegisterResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Register not implemented")
}
func (UnimplementedBeaconServiceServer) Deregister(context.Context, *DeregisterRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Deregister not implemented")
}
func (UnimplementedBeaconServiceServer) BuildImage(*BuildImageRequest, BeaconService_BuildImageServer) error {
	return status.Errorf(codes.Unimplemented, "method BuildImage not implemented")
}
func (UnimplementedBeaconServiceServer) CreateContainer(*CreateContainerRequest, BeaconService_CreateContainerServer) error {
	return status.Errorf(codes.Unimplemented, "method CreateContainer not implemented")
}
func (UnimplementedBeaconServiceServer) AcquireContainerEndpoint(*AcquireContainerEndpointRequest, BeaconService_AcquireContainerEndpointServer) error {
	return status.Errorf(codes.Unimplemented, "method AcquireContainerEndpoint not implemented")
}
func (UnimplementedBeaconServiceServer) ReleaseContainer(context.Context, *ReleaseContainerRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReleaseContainer not implemented")
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

func _BeaconService_Register_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RegisterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BeaconServiceServer).Register(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/beacon.BeaconService/Register",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BeaconServiceServer).Register(ctx, req.(*RegisterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BeaconService_Deregister_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeregisterRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BeaconServiceServer).Deregister(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/beacon.BeaconService/Deregister",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BeaconServiceServer).Deregister(ctx, req.(*DeregisterRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BeaconService_BuildImage_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(BuildImageRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(BeaconServiceServer).BuildImage(m, &beaconServiceBuildImageServer{stream})
}

type BeaconService_BuildImageServer interface {
	Send(*BuildImageResponse) error
	grpc.ServerStream
}

type beaconServiceBuildImageServer struct {
	grpc.ServerStream
}

func (x *beaconServiceBuildImageServer) Send(m *BuildImageResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _BeaconService_CreateContainer_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(CreateContainerRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(BeaconServiceServer).CreateContainer(m, &beaconServiceCreateContainerServer{stream})
}

type BeaconService_CreateContainerServer interface {
	Send(*CreateContainerResponse) error
	grpc.ServerStream
}

type beaconServiceCreateContainerServer struct {
	grpc.ServerStream
}

func (x *beaconServiceCreateContainerServer) Send(m *CreateContainerResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _BeaconService_AcquireContainerEndpoint_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(AcquireContainerEndpointRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(BeaconServiceServer).AcquireContainerEndpoint(m, &beaconServiceAcquireContainerEndpointServer{stream})
}

type BeaconService_AcquireContainerEndpointServer interface {
	Send(*AcquireContainerEndpointResponse) error
	grpc.ServerStream
}

type beaconServiceAcquireContainerEndpointServer struct {
	grpc.ServerStream
}

func (x *beaconServiceAcquireContainerEndpointServer) Send(m *AcquireContainerEndpointResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _BeaconService_ReleaseContainer_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReleaseContainerRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BeaconServiceServer).ReleaseContainer(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/beacon.BeaconService/ReleaseContainer",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BeaconServiceServer).ReleaseContainer(ctx, req.(*ReleaseContainerRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// BeaconService_ServiceDesc is the grpc.ServiceDesc for BeaconService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var BeaconService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "beacon.BeaconService",
	HandlerType: (*BeaconServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Register",
			Handler:    _BeaconService_Register_Handler,
		},
		{
			MethodName: "Deregister",
			Handler:    _BeaconService_Deregister_Handler,
		},
		{
			MethodName: "ReleaseContainer",
			Handler:    _BeaconService_ReleaseContainer_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "BuildImage",
			Handler:       _BeaconService_BuildImage_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "CreateContainer",
			Handler:       _BeaconService_CreateContainer_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "AcquireContainerEndpoint",
			Handler:       _BeaconService_AcquireContainerEndpoint_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "beacon.proto",
}