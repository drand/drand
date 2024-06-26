//
// This protobuf file contains the services and message definitions of all
// methods used by drand nodes to produce distributed randomness.
//

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v4.25.3
// source: drand/protocol.proto

package drand

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

const (
	Protocol_GetIdentity_FullMethodName   = "/drand.Protocol/GetIdentity"
	Protocol_PartialBeacon_FullMethodName = "/drand.Protocol/PartialBeacon"
	Protocol_SyncChain_FullMethodName     = "/drand.Protocol/SyncChain"
	Protocol_Status_FullMethodName        = "/drand.Protocol/Status"
)

// ProtocolClient is the client API for Protocol service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ProtocolClient interface {
	// GetIdentity returns the identity of the drand node
	GetIdentity(ctx context.Context, in *IdentityRequest, opts ...grpc.CallOption) (*IdentityResponse, error)
	// PartialBeacon sends its partial beacon to another node
	PartialBeacon(ctx context.Context, in *PartialBeaconPacket, opts ...grpc.CallOption) (*Empty, error)
	// SyncRequest forces a daemon to sync up its chain with other nodes
	SyncChain(ctx context.Context, in *SyncRequest, opts ...grpc.CallOption) (Protocol_SyncChainClient, error)
	// Status responds with the actual status of drand process
	Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error)
}

type protocolClient struct {
	cc grpc.ClientConnInterface
}

func NewProtocolClient(cc grpc.ClientConnInterface) ProtocolClient {
	return &protocolClient{cc}
}

func (c *protocolClient) GetIdentity(ctx context.Context, in *IdentityRequest, opts ...grpc.CallOption) (*IdentityResponse, error) {
	out := new(IdentityResponse)
	err := c.cc.Invoke(ctx, Protocol_GetIdentity_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *protocolClient) PartialBeacon(ctx context.Context, in *PartialBeaconPacket, opts ...grpc.CallOption) (*Empty, error) {
	out := new(Empty)
	err := c.cc.Invoke(ctx, Protocol_PartialBeacon_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *protocolClient) SyncChain(ctx context.Context, in *SyncRequest, opts ...grpc.CallOption) (Protocol_SyncChainClient, error) {
	stream, err := c.cc.NewStream(ctx, &Protocol_ServiceDesc.Streams[0], Protocol_SyncChain_FullMethodName, opts...)
	if err != nil {
		return nil, err
	}
	x := &protocolSyncChainClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Protocol_SyncChainClient interface {
	Recv() (*BeaconPacket, error)
	grpc.ClientStream
}

type protocolSyncChainClient struct {
	grpc.ClientStream
}

func (x *protocolSyncChainClient) Recv() (*BeaconPacket, error) {
	m := new(BeaconPacket)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *protocolClient) Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error) {
	out := new(StatusResponse)
	err := c.cc.Invoke(ctx, Protocol_Status_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ProtocolServer is the server API for Protocol service.
// All implementations should embed UnimplementedProtocolServer
// for forward compatibility
type ProtocolServer interface {
	// GetIdentity returns the identity of the drand node
	GetIdentity(context.Context, *IdentityRequest) (*IdentityResponse, error)
	// PartialBeacon sends its partial beacon to another node
	PartialBeacon(context.Context, *PartialBeaconPacket) (*Empty, error)
	// SyncRequest forces a daemon to sync up its chain with other nodes
	SyncChain(*SyncRequest, Protocol_SyncChainServer) error
	// Status responds with the actual status of drand process
	Status(context.Context, *StatusRequest) (*StatusResponse, error)
}

// UnimplementedProtocolServer should be embedded to have forward compatible implementations.
type UnimplementedProtocolServer struct {
}

func (UnimplementedProtocolServer) GetIdentity(context.Context, *IdentityRequest) (*IdentityResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetIdentity not implemented")
}
func (UnimplementedProtocolServer) PartialBeacon(context.Context, *PartialBeaconPacket) (*Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PartialBeacon not implemented")
}
func (UnimplementedProtocolServer) SyncChain(*SyncRequest, Protocol_SyncChainServer) error {
	return status.Errorf(codes.Unimplemented, "method SyncChain not implemented")
}
func (UnimplementedProtocolServer) Status(context.Context, *StatusRequest) (*StatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}

// UnsafeProtocolServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ProtocolServer will
// result in compilation errors.
type UnsafeProtocolServer interface {
	mustEmbedUnimplementedProtocolServer()
}

func RegisterProtocolServer(s grpc.ServiceRegistrar, srv ProtocolServer) {
	s.RegisterService(&Protocol_ServiceDesc, srv)
}

func _Protocol_GetIdentity_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(IdentityRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProtocolServer).GetIdentity(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Protocol_GetIdentity_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProtocolServer).GetIdentity(ctx, req.(*IdentityRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Protocol_PartialBeacon_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PartialBeaconPacket)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProtocolServer).PartialBeacon(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Protocol_PartialBeacon_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProtocolServer).PartialBeacon(ctx, req.(*PartialBeaconPacket))
	}
	return interceptor(ctx, in, info, handler)
}

func _Protocol_SyncChain_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(SyncRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(ProtocolServer).SyncChain(m, &protocolSyncChainServer{stream})
}

type Protocol_SyncChainServer interface {
	Send(*BeaconPacket) error
	grpc.ServerStream
}

type protocolSyncChainServer struct {
	grpc.ServerStream
}

func (x *protocolSyncChainServer) Send(m *BeaconPacket) error {
	return x.ServerStream.SendMsg(m)
}

func _Protocol_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProtocolServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Protocol_Status_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProtocolServer).Status(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Protocol_ServiceDesc is the grpc.ServiceDesc for Protocol service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Protocol_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "drand.Protocol",
	HandlerType: (*ProtocolServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetIdentity",
			Handler:    _Protocol_GetIdentity_Handler,
		},
		{
			MethodName: "PartialBeacon",
			Handler:    _Protocol_PartialBeacon_Handler,
		},
		{
			MethodName: "Status",
			Handler:    _Protocol_Status_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "SyncChain",
			Handler:       _Protocol_SyncChain_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "drand/protocol.proto",
}
