// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

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

// ProtocolClient is the client API for Protocol service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ProtocolClient interface {
	// GetIdentity returns the identity of the drand node
	GetIdentity(ctx context.Context, in *IdentityRequest, opts ...grpc.CallOption) (*IdentityResponse, error)
	// SignalDKGParticipant is called by non-coordinators nodes that sends their
	// public keys and secret proof they have to the coordinator so that he can
	// create the group.
	SignalDKGParticipant(ctx context.Context, in *SignalDKGPacket, opts ...grpc.CallOption) (*Empty, error)
	// PushDKGInfo is called by the coordinator to push the group he created
	// from all received keys and as well other information such as the time of
	// starting the DKG.
	PushDKGInfo(ctx context.Context, in *DKGInfoPacket, opts ...grpc.CallOption) (*Empty, error)
	// BroadcastPacket is used during DKG phases
	BroadcastDKG(ctx context.Context, in *DKGPacket, opts ...grpc.CallOption) (*Empty, error)
	// PartialBeacon sends its partial beacon to another node
	PartialBeacon(ctx context.Context, in *PartialBeaconPacket, opts ...grpc.CallOption) (*Empty, error)
	// SyncRequest forces a daemon to sync up its chain with other nodes
	SyncChain(ctx context.Context, in *SyncRequest, opts ...grpc.CallOption) (Protocol_SyncChainClient, error)
}

type protocolClient struct {
	cc grpc.ClientConnInterface
}

func NewProtocolClient(cc grpc.ClientConnInterface) ProtocolClient {
	return &protocolClient{cc}
}

func (c *protocolClient) GetIdentity(ctx context.Context, in *IdentityRequest, opts ...grpc.CallOption) (*IdentityResponse, error) {
	out := new(IdentityResponse)
	err := c.cc.Invoke(ctx, "/drand.Protocol/GetIdentity", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *protocolClient) SignalDKGParticipant(ctx context.Context, in *SignalDKGPacket, opts ...grpc.CallOption) (*Empty, error) {
	out := new(Empty)
	err := c.cc.Invoke(ctx, "/drand.Protocol/SignalDKGParticipant", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *protocolClient) PushDKGInfo(ctx context.Context, in *DKGInfoPacket, opts ...grpc.CallOption) (*Empty, error) {
	out := new(Empty)
	err := c.cc.Invoke(ctx, "/drand.Protocol/PushDKGInfo", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *protocolClient) BroadcastDKG(ctx context.Context, in *DKGPacket, opts ...grpc.CallOption) (*Empty, error) {
	out := new(Empty)
	err := c.cc.Invoke(ctx, "/drand.Protocol/BroadcastDKG", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *protocolClient) PartialBeacon(ctx context.Context, in *PartialBeaconPacket, opts ...grpc.CallOption) (*Empty, error) {
	out := new(Empty)
	err := c.cc.Invoke(ctx, "/drand.Protocol/PartialBeacon", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *protocolClient) SyncChain(ctx context.Context, in *SyncRequest, opts ...grpc.CallOption) (Protocol_SyncChainClient, error) {
	stream, err := c.cc.NewStream(ctx, &Protocol_ServiceDesc.Streams[0], "/drand.Protocol/SyncChain", opts...)
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

// ProtocolServer is the server API for Protocol service.
// All implementations should embed UnimplementedProtocolServer
// for forward compatibility
type ProtocolServer interface {
	// GetIdentity returns the identity of the drand node
	GetIdentity(context.Context, *IdentityRequest) (*IdentityResponse, error)
	// SignalDKGParticipant is called by non-coordinators nodes that sends their
	// public keys and secret proof they have to the coordinator so that he can
	// create the group.
	SignalDKGParticipant(context.Context, *SignalDKGPacket) (*Empty, error)
	// PushDKGInfo is called by the coordinator to push the group he created
	// from all received keys and as well other information such as the time of
	// starting the DKG.
	PushDKGInfo(context.Context, *DKGInfoPacket) (*Empty, error)
	// BroadcastPacket is used during DKG phases
	BroadcastDKG(context.Context, *DKGPacket) (*Empty, error)
	// PartialBeacon sends its partial beacon to another node
	PartialBeacon(context.Context, *PartialBeaconPacket) (*Empty, error)
	// SyncRequest forces a daemon to sync up its chain with other nodes
	SyncChain(*SyncRequest, Protocol_SyncChainServer) error
}

// UnimplementedProtocolServer should be embedded to have forward compatible implementations.
type UnimplementedProtocolServer struct {
}

func (UnimplementedProtocolServer) GetIdentity(context.Context, *IdentityRequest) (*IdentityResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetIdentity not implemented")
}
func (UnimplementedProtocolServer) SignalDKGParticipant(context.Context, *SignalDKGPacket) (*Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SignalDKGParticipant not implemented")
}
func (UnimplementedProtocolServer) PushDKGInfo(context.Context, *DKGInfoPacket) (*Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PushDKGInfo not implemented")
}
func (UnimplementedProtocolServer) BroadcastDKG(context.Context, *DKGPacket) (*Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method BroadcastDKG not implemented")
}
func (UnimplementedProtocolServer) PartialBeacon(context.Context, *PartialBeaconPacket) (*Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PartialBeacon not implemented")
}
func (UnimplementedProtocolServer) SyncChain(*SyncRequest, Protocol_SyncChainServer) error {
	return status.Errorf(codes.Unimplemented, "method SyncChain not implemented")
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
		FullMethod: "/drand.Protocol/GetIdentity",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProtocolServer).GetIdentity(ctx, req.(*IdentityRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Protocol_SignalDKGParticipant_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SignalDKGPacket)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProtocolServer).SignalDKGParticipant(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/drand.Protocol/SignalDKGParticipant",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProtocolServer).SignalDKGParticipant(ctx, req.(*SignalDKGPacket))
	}
	return interceptor(ctx, in, info, handler)
}

func _Protocol_PushDKGInfo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DKGInfoPacket)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProtocolServer).PushDKGInfo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/drand.Protocol/PushDKGInfo",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProtocolServer).PushDKGInfo(ctx, req.(*DKGInfoPacket))
	}
	return interceptor(ctx, in, info, handler)
}

func _Protocol_BroadcastDKG_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DKGPacket)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ProtocolServer).BroadcastDKG(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/drand.Protocol/BroadcastDKG",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ProtocolServer).BroadcastDKG(ctx, req.(*DKGPacket))
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
		FullMethod: "/drand.Protocol/PartialBeacon",
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
			MethodName: "SignalDKGParticipant",
			Handler:    _Protocol_SignalDKGParticipant_Handler,
		},
		{
			MethodName: "PushDKGInfo",
			Handler:    _Protocol_PushDKGInfo_Handler,
		},
		{
			MethodName: "BroadcastDKG",
			Handler:    _Protocol_BroadcastDKG_Handler,
		},
		{
			MethodName: "PartialBeacon",
			Handler:    _Protocol_PartialBeacon_Handler,
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