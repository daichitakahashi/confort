// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.0
// 	protoc        v3.20.1
// source: beacon.proto

package beacon

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type LockOp int32

const (
	LockOp_LOCK_OP_LOCK   LockOp = 0
	LockOp_LOCK_OP_UNLOCK LockOp = 1
)

// Enum value maps for LockOp.
var (
	LockOp_name = map[int32]string{
		0: "LOCK_OP_LOCK",
		1: "LOCK_OP_UNLOCK",
	}
	LockOp_value = map[string]int32{
		"LOCK_OP_LOCK":   0,
		"LOCK_OP_UNLOCK": 1,
	}
)

func (x LockOp) Enum() *LockOp {
	p := new(LockOp)
	*p = x
	return p
}

func (x LockOp) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (LockOp) Descriptor() protoreflect.EnumDescriptor {
	return file_beacon_proto_enumTypes[0].Descriptor()
}

func (LockOp) Type() protoreflect.EnumType {
	return &file_beacon_proto_enumTypes[0]
}

func (x LockOp) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use LockOp.Descriptor instead.
func (LockOp) EnumDescriptor() ([]byte, []int) {
	return file_beacon_proto_rawDescGZIP(), []int{0}
}

type AcquireOp int32

const (
	AcquireOp_ACQUIRE_OP_LOCK             AcquireOp = 0
	AcquireOp_ACQUIRE_OP_UNLOCK           AcquireOp = 1
	AcquireOp_ACQUIRE_OP_SHARED_LOCK      AcquireOp = 2
	AcquireOp_ACQUIRE_OP_INIT_SHARED_LOCK AcquireOp = 3
	AcquireOp_ACQUIRE_OP_DOWNGRADE        AcquireOp = 4
)

// Enum value maps for AcquireOp.
var (
	AcquireOp_name = map[int32]string{
		0: "ACQUIRE_OP_LOCK",
		1: "ACQUIRE_OP_UNLOCK",
		2: "ACQUIRE_OP_SHARED_LOCK",
		3: "ACQUIRE_OP_INIT_SHARED_LOCK",
		4: "ACQUIRE_OP_DOWNGRADE",
	}
	AcquireOp_value = map[string]int32{
		"ACQUIRE_OP_LOCK":             0,
		"ACQUIRE_OP_UNLOCK":           1,
		"ACQUIRE_OP_SHARED_LOCK":      2,
		"ACQUIRE_OP_INIT_SHARED_LOCK": 3,
		"ACQUIRE_OP_DOWNGRADE":        4,
	}
)

func (x AcquireOp) Enum() *AcquireOp {
	p := new(AcquireOp)
	*p = x
	return p
}

func (x AcquireOp) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (AcquireOp) Descriptor() protoreflect.EnumDescriptor {
	return file_beacon_proto_enumTypes[1].Descriptor()
}

func (AcquireOp) Type() protoreflect.EnumType {
	return &file_beacon_proto_enumTypes[1]
}

func (x AcquireOp) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use AcquireOp.Descriptor instead.
func (AcquireOp) EnumDescriptor() ([]byte, []int) {
	return file_beacon_proto_rawDescGZIP(), []int{1}
}

type LockState int32

const (
	LockState_LOCK_STATE_LOCKED        LockState = 0
	LockState_LOCK_STATE_SHARED_LOCKED LockState = 1
	LockState_LOCK_STATE_UNLOCKED      LockState = 2
)

// Enum value maps for LockState.
var (
	LockState_name = map[int32]string{
		0: "LOCK_STATE_LOCKED",
		1: "LOCK_STATE_SHARED_LOCKED",
		2: "LOCK_STATE_UNLOCKED",
	}
	LockState_value = map[string]int32{
		"LOCK_STATE_LOCKED":        0,
		"LOCK_STATE_SHARED_LOCKED": 1,
		"LOCK_STATE_UNLOCKED":      2,
	}
)

func (x LockState) Enum() *LockState {
	p := new(LockState)
	*p = x
	return p
}

func (x LockState) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (LockState) Descriptor() protoreflect.EnumDescriptor {
	return file_beacon_proto_enumTypes[2].Descriptor()
}

func (LockState) Type() protoreflect.EnumType {
	return &file_beacon_proto_enumTypes[2]
}

func (x LockState) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use LockState.Descriptor instead.
func (LockState) EnumDescriptor() ([]byte, []int) {
	return file_beacon_proto_rawDescGZIP(), []int{2}
}

type LockRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Operation LockOp `protobuf:"varint,1,opt,name=operation,proto3,enum=beacon.LockOp" json:"operation,omitempty"`
}

func (x *LockRequest) Reset() {
	*x = LockRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_beacon_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LockRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LockRequest) ProtoMessage() {}

func (x *LockRequest) ProtoReflect() protoreflect.Message {
	mi := &file_beacon_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LockRequest.ProtoReflect.Descriptor instead.
func (*LockRequest) Descriptor() ([]byte, []int) {
	return file_beacon_proto_rawDescGZIP(), []int{0}
}

func (x *LockRequest) GetOperation() LockOp {
	if x != nil {
		return x.Operation
	}
	return LockOp_LOCK_OP_LOCK
}

type LockResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	State LockState `protobuf:"varint,1,opt,name=state,proto3,enum=beacon.LockState" json:"state,omitempty"`
}

func (x *LockResponse) Reset() {
	*x = LockResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_beacon_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LockResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LockResponse) ProtoMessage() {}

func (x *LockResponse) ProtoReflect() protoreflect.Message {
	mi := &file_beacon_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LockResponse.ProtoReflect.Descriptor instead.
func (*LockResponse) Descriptor() ([]byte, []int) {
	return file_beacon_proto_rawDescGZIP(), []int{1}
}

func (x *LockResponse) GetState() LockState {
	if x != nil {
		return x.State
	}
	return LockState_LOCK_STATE_LOCKED
}

type KeyedLockRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Key       string `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	Operation LockOp `protobuf:"varint,2,opt,name=operation,proto3,enum=beacon.LockOp" json:"operation,omitempty"`
}

func (x *KeyedLockRequest) Reset() {
	*x = KeyedLockRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_beacon_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *KeyedLockRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*KeyedLockRequest) ProtoMessage() {}

func (x *KeyedLockRequest) ProtoReflect() protoreflect.Message {
	mi := &file_beacon_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use KeyedLockRequest.ProtoReflect.Descriptor instead.
func (*KeyedLockRequest) Descriptor() ([]byte, []int) {
	return file_beacon_proto_rawDescGZIP(), []int{2}
}

func (x *KeyedLockRequest) GetKey() string {
	if x != nil {
		return x.Key
	}
	return ""
}

func (x *KeyedLockRequest) GetOperation() LockOp {
	if x != nil {
		return x.Operation
	}
	return LockOp_LOCK_OP_LOCK
}

type AcquireLockRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Key       string    `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	Operation AcquireOp `protobuf:"varint,2,opt,name=operation,proto3,enum=beacon.AcquireOp" json:"operation,omitempty"`
}

func (x *AcquireLockRequest) Reset() {
	*x = AcquireLockRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_beacon_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AcquireLockRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AcquireLockRequest) ProtoMessage() {}

func (x *AcquireLockRequest) ProtoReflect() protoreflect.Message {
	mi := &file_beacon_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AcquireLockRequest.ProtoReflect.Descriptor instead.
func (*AcquireLockRequest) Descriptor() ([]byte, []int) {
	return file_beacon_proto_rawDescGZIP(), []int{3}
}

func (x *AcquireLockRequest) GetKey() string {
	if x != nil {
		return x.Key
	}
	return ""
}

func (x *AcquireLockRequest) GetOperation() AcquireOp {
	if x != nil {
		return x.Operation
	}
	return AcquireOp_ACQUIRE_OP_LOCK
}

var File_beacon_proto protoreflect.FileDescriptor

var file_beacon_proto_rawDesc = []byte{
	0x0a, 0x0c, 0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06,
	0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x22, 0x3b, 0x0a, 0x0b, 0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x12, 0x2c, 0x0a, 0x09, 0x6f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x0e, 0x2e, 0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x2e, 0x4c,
	0x6f, 0x63, 0x6b, 0x4f, 0x70, 0x52, 0x09, 0x6f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e,
	0x22, 0x37, 0x0a, 0x0c, 0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x27, 0x0a, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32,
	0x11, 0x2e, 0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x2e, 0x4c, 0x6f, 0x63, 0x6b, 0x53, 0x74, 0x61,
	0x74, 0x65, 0x52, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x22, 0x52, 0x0a, 0x10, 0x4b, 0x65, 0x79,
	0x65, 0x64, 0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x10, 0x0a,
	0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12,
	0x2c, 0x0a, 0x09, 0x6f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x0e, 0x32, 0x0e, 0x2e, 0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x2e, 0x4c, 0x6f, 0x63, 0x6b,
	0x4f, 0x70, 0x52, 0x09, 0x6f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x57, 0x0a,
	0x12, 0x41, 0x63, 0x71, 0x75, 0x69, 0x72, 0x65, 0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x2f, 0x0a, 0x09, 0x6f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x11, 0x2e, 0x62, 0x65, 0x61, 0x63, 0x6f,
	0x6e, 0x2e, 0x41, 0x63, 0x71, 0x75, 0x69, 0x72, 0x65, 0x4f, 0x70, 0x52, 0x09, 0x6f, 0x70, 0x65,
	0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2a, 0x2e, 0x0a, 0x06, 0x4c, 0x6f, 0x63, 0x6b, 0x4f, 0x70,
	0x12, 0x10, 0x0a, 0x0c, 0x4c, 0x4f, 0x43, 0x4b, 0x5f, 0x4f, 0x50, 0x5f, 0x4c, 0x4f, 0x43, 0x4b,
	0x10, 0x00, 0x12, 0x12, 0x0a, 0x0e, 0x4c, 0x4f, 0x43, 0x4b, 0x5f, 0x4f, 0x50, 0x5f, 0x55, 0x4e,
	0x4c, 0x4f, 0x43, 0x4b, 0x10, 0x01, 0x2a, 0x8e, 0x01, 0x0a, 0x09, 0x41, 0x63, 0x71, 0x75, 0x69,
	0x72, 0x65, 0x4f, 0x70, 0x12, 0x13, 0x0a, 0x0f, 0x41, 0x43, 0x51, 0x55, 0x49, 0x52, 0x45, 0x5f,
	0x4f, 0x50, 0x5f, 0x4c, 0x4f, 0x43, 0x4b, 0x10, 0x00, 0x12, 0x15, 0x0a, 0x11, 0x41, 0x43, 0x51,
	0x55, 0x49, 0x52, 0x45, 0x5f, 0x4f, 0x50, 0x5f, 0x55, 0x4e, 0x4c, 0x4f, 0x43, 0x4b, 0x10, 0x01,
	0x12, 0x1a, 0x0a, 0x16, 0x41, 0x43, 0x51, 0x55, 0x49, 0x52, 0x45, 0x5f, 0x4f, 0x50, 0x5f, 0x53,
	0x48, 0x41, 0x52, 0x45, 0x44, 0x5f, 0x4c, 0x4f, 0x43, 0x4b, 0x10, 0x02, 0x12, 0x1f, 0x0a, 0x1b,
	0x41, 0x43, 0x51, 0x55, 0x49, 0x52, 0x45, 0x5f, 0x4f, 0x50, 0x5f, 0x49, 0x4e, 0x49, 0x54, 0x5f,
	0x53, 0x48, 0x41, 0x52, 0x45, 0x44, 0x5f, 0x4c, 0x4f, 0x43, 0x4b, 0x10, 0x03, 0x12, 0x18, 0x0a,
	0x14, 0x41, 0x43, 0x51, 0x55, 0x49, 0x52, 0x45, 0x5f, 0x4f, 0x50, 0x5f, 0x44, 0x4f, 0x57, 0x4e,
	0x47, 0x52, 0x41, 0x44, 0x45, 0x10, 0x04, 0x2a, 0x59, 0x0a, 0x09, 0x4c, 0x6f, 0x63, 0x6b, 0x53,
	0x74, 0x61, 0x74, 0x65, 0x12, 0x15, 0x0a, 0x11, 0x4c, 0x4f, 0x43, 0x4b, 0x5f, 0x53, 0x54, 0x41,
	0x54, 0x45, 0x5f, 0x4c, 0x4f, 0x43, 0x4b, 0x45, 0x44, 0x10, 0x00, 0x12, 0x1c, 0x0a, 0x18, 0x4c,
	0x4f, 0x43, 0x4b, 0x5f, 0x53, 0x54, 0x41, 0x54, 0x45, 0x5f, 0x53, 0x48, 0x41, 0x52, 0x45, 0x44,
	0x5f, 0x4c, 0x4f, 0x43, 0x4b, 0x45, 0x44, 0x10, 0x01, 0x12, 0x17, 0x0a, 0x13, 0x4c, 0x4f, 0x43,
	0x4b, 0x5f, 0x53, 0x54, 0x41, 0x54, 0x45, 0x5f, 0x55, 0x4e, 0x4c, 0x4f, 0x43, 0x4b, 0x45, 0x44,
	0x10, 0x02, 0x32, 0xe4, 0x02, 0x0a, 0x0d, 0x42, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x53, 0x65, 0x72,
	0x76, 0x69, 0x63, 0x65, 0x12, 0x3e, 0x0a, 0x0d, 0x4e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63,
	0x65, 0x4c, 0x6f, 0x63, 0x6b, 0x12, 0x13, 0x2e, 0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x2e, 0x4c,
	0x6f, 0x63, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x14, 0x2e, 0x62, 0x65, 0x61,
	0x63, 0x6f, 0x6e, 0x2e, 0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x28, 0x01, 0x30, 0x01, 0x12, 0x3f, 0x0a, 0x09, 0x42, 0x75, 0x69, 0x6c, 0x64, 0x4c, 0x6f, 0x63,
	0x6b, 0x12, 0x18, 0x2e, 0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x2e, 0x4b, 0x65, 0x79, 0x65, 0x64,
	0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x14, 0x2e, 0x62, 0x65,
	0x61, 0x63, 0x6f, 0x6e, 0x2e, 0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x28, 0x01, 0x30, 0x01, 0x12, 0x47, 0x0a, 0x11, 0x49, 0x6e, 0x69, 0x74, 0x43, 0x6f, 0x6e,
	0x74, 0x61, 0x69, 0x6e, 0x65, 0x72, 0x4c, 0x6f, 0x63, 0x6b, 0x12, 0x18, 0x2e, 0x62, 0x65, 0x61,
	0x63, 0x6f, 0x6e, 0x2e, 0x4b, 0x65, 0x79, 0x65, 0x64, 0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x1a, 0x14, 0x2e, 0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x2e, 0x4c, 0x6f,
	0x63, 0x6b, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x28, 0x01, 0x30, 0x01, 0x12, 0x4c,
	0x0a, 0x14, 0x41, 0x63, 0x71, 0x75, 0x69, 0x72, 0x65, 0x43, 0x6f, 0x6e, 0x74, 0x61, 0x69, 0x6e,
	0x65, 0x72, 0x4c, 0x6f, 0x63, 0x6b, 0x12, 0x1a, 0x2e, 0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x2e,
	0x41, 0x63, 0x71, 0x75, 0x69, 0x72, 0x65, 0x4c, 0x6f, 0x63, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x1a, 0x14, 0x2e, 0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x2e, 0x4c, 0x6f, 0x63, 0x6b,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x28, 0x01, 0x30, 0x01, 0x12, 0x3b, 0x0a, 0x09,
	0x49, 0x6e, 0x74, 0x65, 0x72, 0x72, 0x75, 0x70, 0x74, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74,
	0x79, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x42, 0x0e, 0x5a, 0x0c, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x2f, 0x62, 0x65, 0x61, 0x63, 0x6f, 0x6e, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
}

var (
	file_beacon_proto_rawDescOnce sync.Once
	file_beacon_proto_rawDescData = file_beacon_proto_rawDesc
)

func file_beacon_proto_rawDescGZIP() []byte {
	file_beacon_proto_rawDescOnce.Do(func() {
		file_beacon_proto_rawDescData = protoimpl.X.CompressGZIP(file_beacon_proto_rawDescData)
	})
	return file_beacon_proto_rawDescData
}

var file_beacon_proto_enumTypes = make([]protoimpl.EnumInfo, 3)
var file_beacon_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_beacon_proto_goTypes = []interface{}{
	(LockOp)(0),                // 0: beacon.LockOp
	(AcquireOp)(0),             // 1: beacon.AcquireOp
	(LockState)(0),             // 2: beacon.LockState
	(*LockRequest)(nil),        // 3: beacon.LockRequest
	(*LockResponse)(nil),       // 4: beacon.LockResponse
	(*KeyedLockRequest)(nil),   // 5: beacon.KeyedLockRequest
	(*AcquireLockRequest)(nil), // 6: beacon.AcquireLockRequest
	(*emptypb.Empty)(nil),      // 7: google.protobuf.Empty
}
var file_beacon_proto_depIdxs = []int32{
	0, // 0: beacon.LockRequest.operation:type_name -> beacon.LockOp
	2, // 1: beacon.LockResponse.state:type_name -> beacon.LockState
	0, // 2: beacon.KeyedLockRequest.operation:type_name -> beacon.LockOp
	1, // 3: beacon.AcquireLockRequest.operation:type_name -> beacon.AcquireOp
	3, // 4: beacon.BeaconService.NamespaceLock:input_type -> beacon.LockRequest
	5, // 5: beacon.BeaconService.BuildLock:input_type -> beacon.KeyedLockRequest
	5, // 6: beacon.BeaconService.InitContainerLock:input_type -> beacon.KeyedLockRequest
	6, // 7: beacon.BeaconService.AcquireContainerLock:input_type -> beacon.AcquireLockRequest
	7, // 8: beacon.BeaconService.Interrupt:input_type -> google.protobuf.Empty
	4, // 9: beacon.BeaconService.NamespaceLock:output_type -> beacon.LockResponse
	4, // 10: beacon.BeaconService.BuildLock:output_type -> beacon.LockResponse
	4, // 11: beacon.BeaconService.InitContainerLock:output_type -> beacon.LockResponse
	4, // 12: beacon.BeaconService.AcquireContainerLock:output_type -> beacon.LockResponse
	7, // 13: beacon.BeaconService.Interrupt:output_type -> google.protobuf.Empty
	9, // [9:14] is the sub-list for method output_type
	4, // [4:9] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_beacon_proto_init() }
func file_beacon_proto_init() {
	if File_beacon_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_beacon_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LockRequest); i {
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
		file_beacon_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LockResponse); i {
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
		file_beacon_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*KeyedLockRequest); i {
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
		file_beacon_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AcquireLockRequest); i {
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
			RawDescriptor: file_beacon_proto_rawDesc,
			NumEnums:      3,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_beacon_proto_goTypes,
		DependencyIndexes: file_beacon_proto_depIdxs,
		EnumInfos:         file_beacon_proto_enumTypes,
		MessageInfos:      file_beacon_proto_msgTypes,
	}.Build()
	File_beacon_proto = out.File
	file_beacon_proto_rawDesc = nil
	file_beacon_proto_goTypes = nil
	file_beacon_proto_depIdxs = nil
}
