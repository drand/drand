//
// Protobuf file containing empty message definition

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        v3.14.0
// source: drand/common.proto

package drand

import (
	common "github.com/drand/drand/protobuf/common"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Empty struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Context *common.Context `protobuf:"bytes,1,opt,name=context,proto3" json:"context,omitempty"`
}

func (x *Empty) Reset() {
	*x = Empty{}
	if protoimpl.UnsafeEnabled {
		mi := &file_drand_common_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Empty) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Empty) ProtoMessage() {}

func (x *Empty) ProtoReflect() protoreflect.Message {
	mi := &file_drand_common_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Empty.ProtoReflect.Descriptor instead.
func (*Empty) Descriptor() ([]byte, []int) {
	return file_drand_common_proto_rawDescGZIP(), []int{0}
}

func (x *Empty) GetContext() *common.Context {
	if x != nil {
		return x.Context
	}
	return nil
}

// REMINDER: This fields should be kept in sync with IdentityResponse message
type Identity struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Address string `protobuf:"bytes,1,opt,name=address,proto3" json:"address,omitempty"`
	Key     []byte `protobuf:"bytes,2,opt,name=key,proto3" json:"key,omitempty"`
	Tls     bool   `protobuf:"varint,3,opt,name=tls,proto3" json:"tls,omitempty"`
	// BLS signature over the identity to prove possession of the private key
	Signature []byte `protobuf:"bytes,4,opt,name=signature,proto3" json:"signature,omitempty"`
}

func (x *Identity) Reset() {
	*x = Identity{}
	if protoimpl.UnsafeEnabled {
		mi := &file_drand_common_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Identity) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Identity) ProtoMessage() {}

func (x *Identity) ProtoReflect() protoreflect.Message {
	mi := &file_drand_common_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Identity.ProtoReflect.Descriptor instead.
func (*Identity) Descriptor() ([]byte, []int) {
	return file_drand_common_proto_rawDescGZIP(), []int{1}
}

func (x *Identity) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

func (x *Identity) GetKey() []byte {
	if x != nil {
		return x.Key
	}
	return nil
}

func (x *Identity) GetTls() bool {
	if x != nil {
		return x.Tls
	}
	return false
}

func (x *Identity) GetSignature() []byte {
	if x != nil {
		return x.Signature
	}
	return nil
}

// Node holds the information related to a server in a group that forms a drand
// network
type Node struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Public *Identity `protobuf:"bytes,1,opt,name=public,proto3" json:"public,omitempty"`
	Index  uint32    `protobuf:"varint,2,opt,name=index,proto3" json:"index,omitempty"`
}

func (x *Node) Reset() {
	*x = Node{}
	if protoimpl.UnsafeEnabled {
		mi := &file_drand_common_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Node) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Node) ProtoMessage() {}

func (x *Node) ProtoReflect() protoreflect.Message {
	mi := &file_drand_common_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Node.ProtoReflect.Descriptor instead.
func (*Node) Descriptor() ([]byte, []int) {
	return file_drand_common_proto_rawDescGZIP(), []int{2}
}

func (x *Node) GetPublic() *Identity {
	if x != nil {
		return x.Public
	}
	return nil
}

func (x *Node) GetIndex() uint32 {
	if x != nil {
		return x.Index
	}
	return 0
}

// GroupPacket represents a group that is running a drand network (or is in the
// process of creating one or performing a resharing).
type GroupPacket struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Nodes     []*Node `protobuf:"bytes,1,rep,name=nodes,proto3" json:"nodes,omitempty"`
	Threshold uint32  `protobuf:"varint,2,opt,name=threshold,proto3" json:"threshold,omitempty"`
	// period in seconds
	Period         uint32   `protobuf:"varint,3,opt,name=period,proto3" json:"period,omitempty"`
	GenesisTime    uint64   `protobuf:"varint,4,opt,name=genesis_time,json=genesisTime,proto3" json:"genesis_time,omitempty"`
	TransitionTime uint64   `protobuf:"varint,5,opt,name=transition_time,json=transitionTime,proto3" json:"transition_time,omitempty"`
	GenesisSeed    []byte   `protobuf:"bytes,6,opt,name=genesis_seed,json=genesisSeed,proto3" json:"genesis_seed,omitempty"`
	DistKey        [][]byte `protobuf:"bytes,7,rep,name=dist_key,json=distKey,proto3" json:"dist_key,omitempty"`
	// catchup_period in seconds
	CatchupPeriod uint32          `protobuf:"varint,8,opt,name=catchup_period,json=catchupPeriod,proto3" json:"catchup_period,omitempty"`
	Context       *common.Context `protobuf:"bytes,10,opt,name=context,proto3" json:"context,omitempty"`
}

func (x *GroupPacket) Reset() {
	*x = GroupPacket{}
	if protoimpl.UnsafeEnabled {
		mi := &file_drand_common_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GroupPacket) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GroupPacket) ProtoMessage() {}

func (x *GroupPacket) ProtoReflect() protoreflect.Message {
	mi := &file_drand_common_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GroupPacket.ProtoReflect.Descriptor instead.
func (*GroupPacket) Descriptor() ([]byte, []int) {
	return file_drand_common_proto_rawDescGZIP(), []int{3}
}

func (x *GroupPacket) GetNodes() []*Node {
	if x != nil {
		return x.Nodes
	}
	return nil
}

func (x *GroupPacket) GetThreshold() uint32 {
	if x != nil {
		return x.Threshold
	}
	return 0
}

func (x *GroupPacket) GetPeriod() uint32 {
	if x != nil {
		return x.Period
	}
	return 0
}

func (x *GroupPacket) GetGenesisTime() uint64 {
	if x != nil {
		return x.GenesisTime
	}
	return 0
}

func (x *GroupPacket) GetTransitionTime() uint64 {
	if x != nil {
		return x.TransitionTime
	}
	return 0
}

func (x *GroupPacket) GetGenesisSeed() []byte {
	if x != nil {
		return x.GenesisSeed
	}
	return nil
}

func (x *GroupPacket) GetDistKey() [][]byte {
	if x != nil {
		return x.DistKey
	}
	return nil
}

func (x *GroupPacket) GetCatchupPeriod() uint32 {
	if x != nil {
		return x.CatchupPeriod
	}
	return 0
}

func (x *GroupPacket) GetContext() *common.Context {
	if x != nil {
		return x.Context
	}
	return nil
}

type GroupRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Context *common.Context `protobuf:"bytes,1,opt,name=context,proto3" json:"context,omitempty"`
}

func (x *GroupRequest) Reset() {
	*x = GroupRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_drand_common_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GroupRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GroupRequest) ProtoMessage() {}

func (x *GroupRequest) ProtoReflect() protoreflect.Message {
	mi := &file_drand_common_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GroupRequest.ProtoReflect.Descriptor instead.
func (*GroupRequest) Descriptor() ([]byte, []int) {
	return file_drand_common_proto_rawDescGZIP(), []int{4}
}

func (x *GroupRequest) GetContext() *common.Context {
	if x != nil {
		return x.Context
	}
	return nil
}

type ChainInfoRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Context *common.Context `protobuf:"bytes,1,opt,name=context,proto3" json:"context,omitempty"`
}

func (x *ChainInfoRequest) Reset() {
	*x = ChainInfoRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_drand_common_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ChainInfoRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ChainInfoRequest) ProtoMessage() {}

func (x *ChainInfoRequest) ProtoReflect() protoreflect.Message {
	mi := &file_drand_common_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ChainInfoRequest.ProtoReflect.Descriptor instead.
func (*ChainInfoRequest) Descriptor() ([]byte, []int) {
	return file_drand_common_proto_rawDescGZIP(), []int{5}
}

func (x *ChainInfoRequest) GetContext() *common.Context {
	if x != nil {
		return x.Context
	}
	return nil
}

type ChainInfoPacket struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// marshalled public key
	PublicKey []byte `protobuf:"bytes,1,opt,name=public_key,json=publicKey,proto3" json:"public_key,omitempty"`
	// period in seconds
	Period uint32 `protobuf:"varint,2,opt,name=period,proto3" json:"period,omitempty"`
	// genesis time of the chain
	GenesisTime int64 `protobuf:"varint,3,opt,name=genesis_time,json=genesisTime,proto3" json:"genesis_time,omitempty"`
	// hash is included for ease of use - not needing to have a drand client to
	// compute its hash
	Hash []byte `protobuf:"bytes,4,opt,name=hash,proto3" json:"hash,omitempty"`
	// hash of the genesis group
	GroupHash []byte          `protobuf:"bytes,5,opt,name=groupHash,proto3" json:"groupHash,omitempty"`
	Context   *common.Context `protobuf:"bytes,7,opt,name=context,proto3" json:"context,omitempty"`
}

func (x *ChainInfoPacket) Reset() {
	*x = ChainInfoPacket{}
	if protoimpl.UnsafeEnabled {
		mi := &file_drand_common_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ChainInfoPacket) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ChainInfoPacket) ProtoMessage() {}

func (x *ChainInfoPacket) ProtoReflect() protoreflect.Message {
	mi := &file_drand_common_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ChainInfoPacket.ProtoReflect.Descriptor instead.
func (*ChainInfoPacket) Descriptor() ([]byte, []int) {
	return file_drand_common_proto_rawDescGZIP(), []int{6}
}

func (x *ChainInfoPacket) GetPublicKey() []byte {
	if x != nil {
		return x.PublicKey
	}
	return nil
}

func (x *ChainInfoPacket) GetPeriod() uint32 {
	if x != nil {
		return x.Period
	}
	return 0
}

func (x *ChainInfoPacket) GetGenesisTime() int64 {
	if x != nil {
		return x.GenesisTime
	}
	return 0
}

func (x *ChainInfoPacket) GetHash() []byte {
	if x != nil {
		return x.Hash
	}
	return nil
}

func (x *ChainInfoPacket) GetGroupHash() []byte {
	if x != nil {
		return x.GroupHash
	}
	return nil
}

func (x *ChainInfoPacket) GetContext() *common.Context {
	if x != nil {
		return x.Context
	}
	return nil
}

var File_drand_common_proto protoreflect.FileDescriptor

var file_drand_common_proto_rawDesc = []byte{
	0x0a, 0x12, 0x64, 0x72, 0x61, 0x6e, 0x64, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05, 0x64, 0x72, 0x61, 0x6e, 0x64, 0x1a, 0x13, 0x63, 0x6f, 0x6d,
	0x6d, 0x6f, 0x6e, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x22, 0x32, 0x0a, 0x05, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x29, 0x0a, 0x07, 0x63, 0x6f, 0x6e,
	0x74, 0x65, 0x78, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x63, 0x6f, 0x6d,
	0x6d, 0x6f, 0x6e, 0x2e, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x52, 0x07, 0x63, 0x6f, 0x6e,
	0x74, 0x65, 0x78, 0x74, 0x22, 0x66, 0x0a, 0x08, 0x49, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79,
	0x12, 0x18, 0x0a, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65,
	0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x10, 0x0a, 0x03,
	0x74, 0x6c, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x03, 0x74, 0x6c, 0x73, 0x12, 0x1c,
	0x0a, 0x09, 0x73, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x0c, 0x52, 0x09, 0x73, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x22, 0x45, 0x0a, 0x04,
	0x4e, 0x6f, 0x64, 0x65, 0x12, 0x27, 0x0a, 0x06, 0x70, 0x75, 0x62, 0x6c, 0x69, 0x63, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x64, 0x72, 0x61, 0x6e, 0x64, 0x2e, 0x49, 0x64, 0x65,
	0x6e, 0x74, 0x69, 0x74, 0x79, 0x52, 0x06, 0x70, 0x75, 0x62, 0x6c, 0x69, 0x63, 0x12, 0x14, 0x0a,
	0x05, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x05, 0x69, 0x6e,
	0x64, 0x65, 0x78, 0x22, 0xc2, 0x02, 0x0a, 0x0b, 0x47, 0x72, 0x6f, 0x75, 0x70, 0x50, 0x61, 0x63,
	0x6b, 0x65, 0x74, 0x12, 0x21, 0x0a, 0x05, 0x6e, 0x6f, 0x64, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03,
	0x28, 0x0b, 0x32, 0x0b, 0x2e, 0x64, 0x72, 0x61, 0x6e, 0x64, 0x2e, 0x4e, 0x6f, 0x64, 0x65, 0x52,
	0x05, 0x6e, 0x6f, 0x64, 0x65, 0x73, 0x12, 0x1c, 0x0a, 0x09, 0x74, 0x68, 0x72, 0x65, 0x73, 0x68,
	0x6f, 0x6c, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x09, 0x74, 0x68, 0x72, 0x65, 0x73,
	0x68, 0x6f, 0x6c, 0x64, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x65, 0x72, 0x69, 0x6f, 0x64, 0x18, 0x03,
	0x20, 0x01, 0x28, 0x0d, 0x52, 0x06, 0x70, 0x65, 0x72, 0x69, 0x6f, 0x64, 0x12, 0x21, 0x0a, 0x0c,
	0x67, 0x65, 0x6e, 0x65, 0x73, 0x69, 0x73, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01,
	0x28, 0x04, 0x52, 0x0b, 0x67, 0x65, 0x6e, 0x65, 0x73, 0x69, 0x73, 0x54, 0x69, 0x6d, 0x65, 0x12,
	0x27, 0x0a, 0x0f, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x74, 0x69,
	0x6d, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x04, 0x52, 0x0e, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x69,
	0x74, 0x69, 0x6f, 0x6e, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x67, 0x65, 0x6e, 0x65,
	0x73, 0x69, 0x73, 0x5f, 0x73, 0x65, 0x65, 0x64, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0b,
	0x67, 0x65, 0x6e, 0x65, 0x73, 0x69, 0x73, 0x53, 0x65, 0x65, 0x64, 0x12, 0x19, 0x0a, 0x08, 0x64,
	0x69, 0x73, 0x74, 0x5f, 0x6b, 0x65, 0x79, 0x18, 0x07, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x07, 0x64,
	0x69, 0x73, 0x74, 0x4b, 0x65, 0x79, 0x12, 0x25, 0x0a, 0x0e, 0x63, 0x61, 0x74, 0x63, 0x68, 0x75,
	0x70, 0x5f, 0x70, 0x65, 0x72, 0x69, 0x6f, 0x64, 0x18, 0x08, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0d,
	0x63, 0x61, 0x74, 0x63, 0x68, 0x75, 0x70, 0x50, 0x65, 0x72, 0x69, 0x6f, 0x64, 0x12, 0x29, 0x0a,
	0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f,
	0x2e, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x52,
	0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x22, 0x39, 0x0a, 0x0c, 0x47, 0x72, 0x6f, 0x75,
	0x70, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x29, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x74,
	0x65, 0x78, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x63, 0x6f, 0x6d, 0x6d,
	0x6f, 0x6e, 0x2e, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x52, 0x07, 0x63, 0x6f, 0x6e, 0x74,
	0x65, 0x78, 0x74, 0x22, 0x3d, 0x0a, 0x10, 0x43, 0x68, 0x61, 0x69, 0x6e, 0x49, 0x6e, 0x66, 0x6f,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x29, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65,
	0x78, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x63, 0x6f, 0x6d, 0x6d, 0x6f,
	0x6e, 0x2e, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x52, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65,
	0x78, 0x74, 0x22, 0xc8, 0x01, 0x0a, 0x0f, 0x43, 0x68, 0x61, 0x69, 0x6e, 0x49, 0x6e, 0x66, 0x6f,
	0x50, 0x61, 0x63, 0x6b, 0x65, 0x74, 0x12, 0x1d, 0x0a, 0x0a, 0x70, 0x75, 0x62, 0x6c, 0x69, 0x63,
	0x5f, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x70, 0x75, 0x62, 0x6c,
	0x69, 0x63, 0x4b, 0x65, 0x79, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x65, 0x72, 0x69, 0x6f, 0x64, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x06, 0x70, 0x65, 0x72, 0x69, 0x6f, 0x64, 0x12, 0x21, 0x0a,
	0x0c, 0x67, 0x65, 0x6e, 0x65, 0x73, 0x69, 0x73, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x03, 0x52, 0x0b, 0x67, 0x65, 0x6e, 0x65, 0x73, 0x69, 0x73, 0x54, 0x69, 0x6d, 0x65,
	0x12, 0x12, 0x0a, 0x04, 0x68, 0x61, 0x73, 0x68, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04,
	0x68, 0x61, 0x73, 0x68, 0x12, 0x1c, 0x0a, 0x09, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x48, 0x61, 0x73,
	0x68, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x48, 0x61,
	0x73, 0x68, 0x12, 0x29, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x18, 0x07, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x43, 0x6f, 0x6e,
	0x74, 0x65, 0x78, 0x74, 0x52, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x78, 0x74, 0x42, 0x27, 0x5a,
	0x25, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x64, 0x72, 0x61, 0x6e,
	0x64, 0x2f, 0x64, 0x72, 0x61, 0x6e, 0x64, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2f, 0x64, 0x72, 0x61, 0x6e, 0x64, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_drand_common_proto_rawDescOnce sync.Once
	file_drand_common_proto_rawDescData = file_drand_common_proto_rawDesc
)

func file_drand_common_proto_rawDescGZIP() []byte {
	file_drand_common_proto_rawDescOnce.Do(func() {
		file_drand_common_proto_rawDescData = protoimpl.X.CompressGZIP(file_drand_common_proto_rawDescData)
	})
	return file_drand_common_proto_rawDescData
}

var file_drand_common_proto_msgTypes = make([]protoimpl.MessageInfo, 7)
var file_drand_common_proto_goTypes = []interface{}{
	(*Empty)(nil),            // 0: drand.Empty
	(*Identity)(nil),         // 1: drand.Identity
	(*Node)(nil),             // 2: drand.Node
	(*GroupPacket)(nil),      // 3: drand.GroupPacket
	(*GroupRequest)(nil),     // 4: drand.GroupRequest
	(*ChainInfoRequest)(nil), // 5: drand.ChainInfoRequest
	(*ChainInfoPacket)(nil),  // 6: drand.ChainInfoPacket
	(*common.Context)(nil),   // 7: common.Context
}
var file_drand_common_proto_depIdxs = []int32{
	7, // 0: drand.Empty.context:type_name -> common.Context
	1, // 1: drand.Node.public:type_name -> drand.Identity
	2, // 2: drand.GroupPacket.nodes:type_name -> drand.Node
	7, // 3: drand.GroupPacket.context:type_name -> common.Context
	7, // 4: drand.GroupRequest.context:type_name -> common.Context
	7, // 5: drand.ChainInfoRequest.context:type_name -> common.Context
	7, // 6: drand.ChainInfoPacket.context:type_name -> common.Context
	7, // [7:7] is the sub-list for method output_type
	7, // [7:7] is the sub-list for method input_type
	7, // [7:7] is the sub-list for extension type_name
	7, // [7:7] is the sub-list for extension extendee
	0, // [0:7] is the sub-list for field type_name
}

func init() { file_drand_common_proto_init() }
func file_drand_common_proto_init() {
	if File_drand_common_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_drand_common_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Empty); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_drand_common_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Identity); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_drand_common_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Node); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_drand_common_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GroupPacket); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_drand_common_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GroupRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_drand_common_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ChainInfoRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_drand_common_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ChainInfoPacket); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_drand_common_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   7,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_drand_common_proto_goTypes,
		DependencyIndexes: file_drand_common_proto_depIdxs,
		MessageInfos:      file_drand_common_proto_msgTypes,
	}.Build()
	File_drand_common_proto = out.File
	file_drand_common_proto_rawDesc = nil
	file_drand_common_proto_goTypes = nil
	file_drand_common_proto_depIdxs = nil
}