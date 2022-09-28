// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.20.2
// source: unique.proto

package proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// UniqueValueServiceClient is the client API for UniqueValueService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type UniqueValueServiceClient interface {
	StoreUniqueValue(ctx context.Context, in *StoreUniqueValueRequest, opts ...grpc.CallOption) (*StoreUniqueValueResponse, error)
}

type uniqueValueServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewUniqueValueServiceClient(cc grpc.ClientConnInterface) UniqueValueServiceClient {
	return &uniqueValueServiceClient{cc}
}

func (c *uniqueValueServiceClient) StoreUniqueValue(ctx context.Context, in *StoreUniqueValueRequest, opts ...grpc.CallOption) (*StoreUniqueValueResponse, error) {
	out := new(StoreUniqueValueResponse)
	err := c.cc.Invoke(ctx, "/proto.UniqueValueService/StoreUniqueValue", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UniqueValueServiceServer is the server API for UniqueValueService service.
// All implementations must embed UnimplementedUniqueValueServiceServer
// for forward compatibility
type UniqueValueServiceServer interface {
	StoreUniqueValue(context.Context, *StoreUniqueValueRequest) (*StoreUniqueValueResponse, error)
	mustEmbedUnimplementedUniqueValueServiceServer()
}

// UnimplementedUniqueValueServiceServer must be embedded to have forward compatible implementations.
type UnimplementedUniqueValueServiceServer struct {
}

func (UnimplementedUniqueValueServiceServer) StoreUniqueValue(context.Context, *StoreUniqueValueRequest) (*StoreUniqueValueResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StoreUniqueValue not implemented")
}
func (UnimplementedUniqueValueServiceServer) mustEmbedUnimplementedUniqueValueServiceServer() {}

// UnsafeUniqueValueServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to UniqueValueServiceServer will
// result in compilation errors.
type UnsafeUniqueValueServiceServer interface {
	mustEmbedUnimplementedUniqueValueServiceServer()
}

func RegisterUniqueValueServiceServer(s grpc.ServiceRegistrar, srv UniqueValueServiceServer) {
	s.RegisterService(&UniqueValueService_ServiceDesc, srv)
}

func _UniqueValueService_StoreUniqueValue_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StoreUniqueValueRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(UniqueValueServiceServer).StoreUniqueValue(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.UniqueValueService/StoreUniqueValue",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(UniqueValueServiceServer).StoreUniqueValue(ctx, req.(*StoreUniqueValueRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// UniqueValueService_ServiceDesc is the grpc.ServiceDesc for UniqueValueService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var UniqueValueService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.UniqueValueService",
	HandlerType: (*UniqueValueServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "StoreUniqueValue",
			Handler:    _UniqueValueService_StoreUniqueValue_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "unique.proto",
}