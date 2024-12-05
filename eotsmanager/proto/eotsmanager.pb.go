// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        (unknown)
// source: eotsmanager.proto

package proto

import (
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

type PingRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *PingRequest) Reset() {
	*x = PingRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PingRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PingRequest) ProtoMessage() {}

func (x *PingRequest) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PingRequest.ProtoReflect.Descriptor instead.
func (*PingRequest) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{0}
}

type PingResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *PingResponse) Reset() {
	*x = PingResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PingResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PingResponse) ProtoMessage() {}

func (x *PingResponse) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PingResponse.ProtoReflect.Descriptor instead.
func (*PingResponse) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{1}
}

type CreateKeyRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// name is the identifier key in keyring
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// passphrase is used to encrypt the EOTS key
	Passphrase string `protobuf:"bytes,2,opt,name=passphrase,proto3" json:"passphrase,omitempty"`
	// hd_path is the hd path for private key derivation
	HdPath string `protobuf:"bytes,3,opt,name=hd_path,json=hdPath,proto3" json:"hd_path,omitempty"`
}

func (x *CreateKeyRequest) Reset() {
	*x = CreateKeyRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CreateKeyRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateKeyRequest) ProtoMessage() {}

func (x *CreateKeyRequest) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateKeyRequest.ProtoReflect.Descriptor instead.
func (*CreateKeyRequest) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{2}
}

func (x *CreateKeyRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *CreateKeyRequest) GetPassphrase() string {
	if x != nil {
		return x.Passphrase
	}
	return ""
}

func (x *CreateKeyRequest) GetHdPath() string {
	if x != nil {
		return x.HdPath
	}
	return ""
}

type CreateKeyResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// pk is the EOTS public key following BIP-340 spec
	Pk []byte `protobuf:"bytes,1,opt,name=pk,proto3" json:"pk,omitempty"`
}

func (x *CreateKeyResponse) Reset() {
	*x = CreateKeyResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CreateKeyResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateKeyResponse) ProtoMessage() {}

func (x *CreateKeyResponse) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateKeyResponse.ProtoReflect.Descriptor instead.
func (*CreateKeyResponse) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{3}
}

func (x *CreateKeyResponse) GetPk() []byte {
	if x != nil {
		return x.Pk
	}
	return nil
}

type CreateRandomnessPairListRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// uid is the identifier of an EOTS key, i.e., public key following BIP-340 spec
	Uid []byte `protobuf:"bytes,1,opt,name=uid,proto3" json:"uid,omitempty"`
	// chain_id is the identifier of the consumer chain that the randomness is committed to
	ChainId []byte `protobuf:"bytes,2,opt,name=chain_id,json=chainId,proto3" json:"chain_id,omitempty"`
	// start_height is the start height of the randomness pair list
	StartHeight uint64 `protobuf:"varint,3,opt,name=start_height,json=startHeight,proto3" json:"start_height,omitempty"`
	// num is the number of randomness pair list
	Num uint32 `protobuf:"varint,4,opt,name=num,proto3" json:"num,omitempty"`
	// passphrase is used to decrypt the EOTS key
	Passphrase string `protobuf:"bytes,5,opt,name=passphrase,proto3" json:"passphrase,omitempty"`
}

func (x *CreateRandomnessPairListRequest) Reset() {
	*x = CreateRandomnessPairListRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CreateRandomnessPairListRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateRandomnessPairListRequest) ProtoMessage() {}

func (x *CreateRandomnessPairListRequest) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateRandomnessPairListRequest.ProtoReflect.Descriptor instead.
func (*CreateRandomnessPairListRequest) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{4}
}

func (x *CreateRandomnessPairListRequest) GetUid() []byte {
	if x != nil {
		return x.Uid
	}
	return nil
}

func (x *CreateRandomnessPairListRequest) GetChainId() []byte {
	if x != nil {
		return x.ChainId
	}
	return nil
}

func (x *CreateRandomnessPairListRequest) GetStartHeight() uint64 {
	if x != nil {
		return x.StartHeight
	}
	return 0
}

func (x *CreateRandomnessPairListRequest) GetNum() uint32 {
	if x != nil {
		return x.Num
	}
	return 0
}

func (x *CreateRandomnessPairListRequest) GetPassphrase() string {
	if x != nil {
		return x.Passphrase
	}
	return ""
}

type CreateRandomnessPairListResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// pub_rand_list is a list of Schnorr public randomness
	PubRandList [][]byte `protobuf:"bytes,1,rep,name=pub_rand_list,json=pubRandList,proto3" json:"pub_rand_list,omitempty"`
}

func (x *CreateRandomnessPairListResponse) Reset() {
	*x = CreateRandomnessPairListResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CreateRandomnessPairListResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateRandomnessPairListResponse) ProtoMessage() {}

func (x *CreateRandomnessPairListResponse) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateRandomnessPairListResponse.ProtoReflect.Descriptor instead.
func (*CreateRandomnessPairListResponse) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{5}
}

func (x *CreateRandomnessPairListResponse) GetPubRandList() [][]byte {
	if x != nil {
		return x.PubRandList
	}
	return nil
}

type KeyRecordRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// uid is the identifier of an EOTS key, i.e., public key following BIP-340 spec
	Uid []byte `protobuf:"bytes,1,opt,name=uid,proto3" json:"uid,omitempty"`
	// passphrase is used to decrypt the EOTS key
	Passphrase string `protobuf:"bytes,2,opt,name=passphrase,proto3" json:"passphrase,omitempty"`
}

func (x *KeyRecordRequest) Reset() {
	*x = KeyRecordRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *KeyRecordRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*KeyRecordRequest) ProtoMessage() {}

func (x *KeyRecordRequest) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use KeyRecordRequest.ProtoReflect.Descriptor instead.
func (*KeyRecordRequest) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{6}
}

func (x *KeyRecordRequest) GetUid() []byte {
	if x != nil {
		return x.Uid
	}
	return nil
}

func (x *KeyRecordRequest) GetPassphrase() string {
	if x != nil {
		return x.Passphrase
	}
	return ""
}

type KeyRecordResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// name is the identifier key in keyring
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// private_key is the private EOTS key encoded in secp256k1 spec
	PrivateKey []byte `protobuf:"bytes,2,opt,name=private_key,json=privateKey,proto3" json:"private_key,omitempty"`
}

func (x *KeyRecordResponse) Reset() {
	*x = KeyRecordResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *KeyRecordResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*KeyRecordResponse) ProtoMessage() {}

func (x *KeyRecordResponse) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use KeyRecordResponse.ProtoReflect.Descriptor instead.
func (*KeyRecordResponse) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{7}
}

func (x *KeyRecordResponse) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *KeyRecordResponse) GetPrivateKey() []byte {
	if x != nil {
		return x.PrivateKey
	}
	return nil
}

type SignEOTSRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// uid is the identifier of an EOTS key, i.e., public key following BIP-340 spec
	Uid []byte `protobuf:"bytes,1,opt,name=uid,proto3" json:"uid,omitempty"`
	// chain_id is the identifier of the consumer chain that the randomness is committed to
	ChainId []byte `protobuf:"bytes,2,opt,name=chain_id,json=chainId,proto3" json:"chain_id,omitempty"`
	// the message which the EOTS signs
	Msg []byte `protobuf:"bytes,3,opt,name=msg,proto3" json:"msg,omitempty"`
	// the block height which the EOTS signs
	Height uint64 `protobuf:"varint,4,opt,name=height,proto3" json:"height,omitempty"`
	// passphrase is used to decrypt the EOTS key
	Passphrase string `protobuf:"bytes,5,opt,name=passphrase,proto3" json:"passphrase,omitempty"`
}

func (x *SignEOTSRequest) Reset() {
	*x = SignEOTSRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SignEOTSRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SignEOTSRequest) ProtoMessage() {}

func (x *SignEOTSRequest) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SignEOTSRequest.ProtoReflect.Descriptor instead.
func (*SignEOTSRequest) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{8}
}

func (x *SignEOTSRequest) GetUid() []byte {
	if x != nil {
		return x.Uid
	}
	return nil
}

func (x *SignEOTSRequest) GetChainId() []byte {
	if x != nil {
		return x.ChainId
	}
	return nil
}

func (x *SignEOTSRequest) GetMsg() []byte {
	if x != nil {
		return x.Msg
	}
	return nil
}

func (x *SignEOTSRequest) GetHeight() uint64 {
	if x != nil {
		return x.Height
	}
	return 0
}

func (x *SignEOTSRequest) GetPassphrase() string {
	if x != nil {
		return x.Passphrase
	}
	return ""
}

type SignEOTSResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// sig is the EOTS signature
	Sig []byte `protobuf:"bytes,1,opt,name=sig,proto3" json:"sig,omitempty"`
}

func (x *SignEOTSResponse) Reset() {
	*x = SignEOTSResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SignEOTSResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SignEOTSResponse) ProtoMessage() {}

func (x *SignEOTSResponse) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[9]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SignEOTSResponse.ProtoReflect.Descriptor instead.
func (*SignEOTSResponse) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{9}
}

func (x *SignEOTSResponse) GetSig() []byte {
	if x != nil {
		return x.Sig
	}
	return nil
}

type SignSchnorrSigRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// uid is the identifier of an EOTS key, i.e., public key following BIP-340 spec
	Uid []byte `protobuf:"bytes,1,opt,name=uid,proto3" json:"uid,omitempty"`
	// the message which the Schnorr signature signs
	Msg []byte `protobuf:"bytes,2,opt,name=msg,proto3" json:"msg,omitempty"`
	// passphrase is used to decrypt the EOTS key
	Passphrase string `protobuf:"bytes,3,opt,name=passphrase,proto3" json:"passphrase,omitempty"`
}

func (x *SignSchnorrSigRequest) Reset() {
	*x = SignSchnorrSigRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[10]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SignSchnorrSigRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SignSchnorrSigRequest) ProtoMessage() {}

func (x *SignSchnorrSigRequest) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[10]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SignSchnorrSigRequest.ProtoReflect.Descriptor instead.
func (*SignSchnorrSigRequest) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{10}
}

func (x *SignSchnorrSigRequest) GetUid() []byte {
	if x != nil {
		return x.Uid
	}
	return nil
}

func (x *SignSchnorrSigRequest) GetMsg() []byte {
	if x != nil {
		return x.Msg
	}
	return nil
}

func (x *SignSchnorrSigRequest) GetPassphrase() string {
	if x != nil {
		return x.Passphrase
	}
	return ""
}

type SignSchnorrSigResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// sig is the Schnorr signature
	Sig []byte `protobuf:"bytes,1,opt,name=sig,proto3" json:"sig,omitempty"`
}

func (x *SignSchnorrSigResponse) Reset() {
	*x = SignSchnorrSigResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_eotsmanager_proto_msgTypes[11]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SignSchnorrSigResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SignSchnorrSigResponse) ProtoMessage() {}

func (x *SignSchnorrSigResponse) ProtoReflect() protoreflect.Message {
	mi := &file_eotsmanager_proto_msgTypes[11]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SignSchnorrSigResponse.ProtoReflect.Descriptor instead.
func (*SignSchnorrSigResponse) Descriptor() ([]byte, []int) {
	return file_eotsmanager_proto_rawDescGZIP(), []int{11}
}

func (x *SignSchnorrSigResponse) GetSig() []byte {
	if x != nil {
		return x.Sig
	}
	return nil
}

var File_eotsmanager_proto protoreflect.FileDescriptor

var file_eotsmanager_proto_rawDesc = []byte{
	0x0a, 0x11, 0x65, 0x6f, 0x74, 0x73, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x12, 0x05, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x0d, 0x0a, 0x0b, 0x50, 0x69,
	0x6e, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x0e, 0x0a, 0x0c, 0x50, 0x69, 0x6e,
	0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x5f, 0x0a, 0x10, 0x43, 0x72, 0x65,
	0x61, 0x74, 0x65, 0x4b, 0x65, 0x79, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a,
	0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d,
	0x65, 0x12, 0x1e, 0x0a, 0x0a, 0x70, 0x61, 0x73, 0x73, 0x70, 0x68, 0x72, 0x61, 0x73, 0x65, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x70, 0x61, 0x73, 0x73, 0x70, 0x68, 0x72, 0x61, 0x73,
	0x65, 0x12, 0x17, 0x0a, 0x07, 0x68, 0x64, 0x5f, 0x70, 0x61, 0x74, 0x68, 0x18, 0x03, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x06, 0x68, 0x64, 0x50, 0x61, 0x74, 0x68, 0x22, 0x23, 0x0a, 0x11, 0x43, 0x72,
	0x65, 0x61, 0x74, 0x65, 0x4b, 0x65, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12,
	0x0e, 0x0a, 0x02, 0x70, 0x6b, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x02, 0x70, 0x6b, 0x22,
	0xa3, 0x01, 0x0a, 0x1f, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x52, 0x61, 0x6e, 0x64, 0x6f, 0x6d,
	0x6e, 0x65, 0x73, 0x73, 0x50, 0x61, 0x69, 0x72, 0x4c, 0x69, 0x73, 0x74, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c,
	0x52, 0x03, 0x75, 0x69, 0x64, 0x12, 0x19, 0x0a, 0x08, 0x63, 0x68, 0x61, 0x69, 0x6e, 0x5f, 0x69,
	0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x63, 0x68, 0x61, 0x69, 0x6e, 0x49, 0x64,
	0x12, 0x21, 0x0a, 0x0c, 0x73, 0x74, 0x61, 0x72, 0x74, 0x5f, 0x68, 0x65, 0x69, 0x67, 0x68, 0x74,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x04, 0x52, 0x0b, 0x73, 0x74, 0x61, 0x72, 0x74, 0x48, 0x65, 0x69,
	0x67, 0x68, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x6e, 0x75, 0x6d, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0d,
	0x52, 0x03, 0x6e, 0x75, 0x6d, 0x12, 0x1e, 0x0a, 0x0a, 0x70, 0x61, 0x73, 0x73, 0x70, 0x68, 0x72,
	0x61, 0x73, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x70, 0x61, 0x73, 0x73, 0x70,
	0x68, 0x72, 0x61, 0x73, 0x65, 0x22, 0x46, 0x0a, 0x20, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x52,
	0x61, 0x6e, 0x64, 0x6f, 0x6d, 0x6e, 0x65, 0x73, 0x73, 0x50, 0x61, 0x69, 0x72, 0x4c, 0x69, 0x73,
	0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x22, 0x0a, 0x0d, 0x70, 0x75, 0x62,
	0x5f, 0x72, 0x61, 0x6e, 0x64, 0x5f, 0x6c, 0x69, 0x73, 0x74, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0c,
	0x52, 0x0b, 0x70, 0x75, 0x62, 0x52, 0x61, 0x6e, 0x64, 0x4c, 0x69, 0x73, 0x74, 0x22, 0x44, 0x0a,
	0x10, 0x4b, 0x65, 0x79, 0x52, 0x65, 0x63, 0x6f, 0x72, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03,
	0x75, 0x69, 0x64, 0x12, 0x1e, 0x0a, 0x0a, 0x70, 0x61, 0x73, 0x73, 0x70, 0x68, 0x72, 0x61, 0x73,
	0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x70, 0x61, 0x73, 0x73, 0x70, 0x68, 0x72,
	0x61, 0x73, 0x65, 0x22, 0x48, 0x0a, 0x11, 0x4b, 0x65, 0x79, 0x52, 0x65, 0x63, 0x6f, 0x72, 0x64,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x1f, 0x0a, 0x0b,
	0x70, 0x72, 0x69, 0x76, 0x61, 0x74, 0x65, 0x5f, 0x6b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0c, 0x52, 0x0a, 0x70, 0x72, 0x69, 0x76, 0x61, 0x74, 0x65, 0x4b, 0x65, 0x79, 0x22, 0x88, 0x01,
	0x0a, 0x0f, 0x53, 0x69, 0x67, 0x6e, 0x45, 0x4f, 0x54, 0x53, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03,
	0x75, 0x69, 0x64, 0x12, 0x19, 0x0a, 0x08, 0x63, 0x68, 0x61, 0x69, 0x6e, 0x5f, 0x69, 0x64, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x63, 0x68, 0x61, 0x69, 0x6e, 0x49, 0x64, 0x12, 0x10,
	0x0a, 0x03, 0x6d, 0x73, 0x67, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x6d, 0x73, 0x67,
	0x12, 0x16, 0x0a, 0x06, 0x68, 0x65, 0x69, 0x67, 0x68, 0x74, 0x18, 0x04, 0x20, 0x01, 0x28, 0x04,
	0x52, 0x06, 0x68, 0x65, 0x69, 0x67, 0x68, 0x74, 0x12, 0x1e, 0x0a, 0x0a, 0x70, 0x61, 0x73, 0x73,
	0x70, 0x68, 0x72, 0x61, 0x73, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x70, 0x61,
	0x73, 0x73, 0x70, 0x68, 0x72, 0x61, 0x73, 0x65, 0x22, 0x24, 0x0a, 0x10, 0x53, 0x69, 0x67, 0x6e,
	0x45, 0x4f, 0x54, 0x53, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x10, 0x0a, 0x03,
	0x73, 0x69, 0x67, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x73, 0x69, 0x67, 0x22, 0x5b,
	0x0a, 0x15, 0x53, 0x69, 0x67, 0x6e, 0x53, 0x63, 0x68, 0x6e, 0x6f, 0x72, 0x72, 0x53, 0x69, 0x67,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x69, 0x64, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x75, 0x69, 0x64, 0x12, 0x10, 0x0a, 0x03, 0x6d, 0x73, 0x67,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x6d, 0x73, 0x67, 0x12, 0x1e, 0x0a, 0x0a, 0x70,
	0x61, 0x73, 0x73, 0x70, 0x68, 0x72, 0x61, 0x73, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0a, 0x70, 0x61, 0x73, 0x73, 0x70, 0x68, 0x72, 0x61, 0x73, 0x65, 0x22, 0x2a, 0x0a, 0x16, 0x53,
	0x69, 0x67, 0x6e, 0x53, 0x63, 0x68, 0x6e, 0x6f, 0x72, 0x72, 0x53, 0x69, 0x67, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x10, 0x0a, 0x03, 0x73, 0x69, 0x67, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0c, 0x52, 0x03, 0x73, 0x69, 0x67, 0x32, 0xfa, 0x03, 0x0a, 0x0b, 0x45, 0x4f, 0x54, 0x53,
	0x4d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x12, 0x2f, 0x0a, 0x04, 0x50, 0x69, 0x6e, 0x67, 0x12,
	0x12, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x50, 0x69, 0x6e, 0x67, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x13, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x50, 0x69, 0x6e, 0x67,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x3e, 0x0a, 0x09, 0x43, 0x72, 0x65, 0x61,
	0x74, 0x65, 0x4b, 0x65, 0x79, 0x12, 0x17, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x43, 0x72,
	0x65, 0x61, 0x74, 0x65, 0x4b, 0x65, 0x79, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x4b, 0x65, 0x79,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x6b, 0x0a, 0x18, 0x43, 0x72, 0x65, 0x61,
	0x74, 0x65, 0x52, 0x61, 0x6e, 0x64, 0x6f, 0x6d, 0x6e, 0x65, 0x73, 0x73, 0x50, 0x61, 0x69, 0x72,
	0x4c, 0x69, 0x73, 0x74, 0x12, 0x26, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x43, 0x72, 0x65,
	0x61, 0x74, 0x65, 0x52, 0x61, 0x6e, 0x64, 0x6f, 0x6d, 0x6e, 0x65, 0x73, 0x73, 0x50, 0x61, 0x69,
	0x72, 0x4c, 0x69, 0x73, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x27, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x52, 0x61, 0x6e, 0x64, 0x6f,
	0x6d, 0x6e, 0x65, 0x73, 0x73, 0x50, 0x61, 0x69, 0x72, 0x4c, 0x69, 0x73, 0x74, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x3e, 0x0a, 0x09, 0x4b, 0x65, 0x79, 0x52, 0x65, 0x63, 0x6f,
	0x72, 0x64, 0x12, 0x17, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x4b, 0x65, 0x79, 0x52, 0x65,
	0x63, 0x6f, 0x72, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x2e, 0x4b, 0x65, 0x79, 0x52, 0x65, 0x63, 0x6f, 0x72, 0x64, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x3b, 0x0a, 0x08, 0x53, 0x69, 0x67, 0x6e, 0x45, 0x4f, 0x54,
	0x53, 0x12, 0x16, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x53, 0x69, 0x67, 0x6e, 0x45, 0x4f,
	0x54, 0x53, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x17, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x2e, 0x53, 0x69, 0x67, 0x6e, 0x45, 0x4f, 0x54, 0x53, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x12, 0x41, 0x0a, 0x0e, 0x55, 0x6e, 0x73, 0x61, 0x66, 0x65, 0x53, 0x69, 0x67, 0x6e,
	0x45, 0x4f, 0x54, 0x53, 0x12, 0x16, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x53, 0x69, 0x67,
	0x6e, 0x45, 0x4f, 0x54, 0x53, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x17, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x53, 0x69, 0x67, 0x6e, 0x45, 0x4f, 0x54, 0x53, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x4d, 0x0a, 0x0e, 0x53, 0x69, 0x67, 0x6e, 0x53, 0x63, 0x68,
	0x6e, 0x6f, 0x72, 0x72, 0x53, 0x69, 0x67, 0x12, 0x1c, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e,
	0x53, 0x69, 0x67, 0x6e, 0x53, 0x63, 0x68, 0x6e, 0x6f, 0x72, 0x72, 0x53, 0x69, 0x67, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1d, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x53, 0x69,
	0x67, 0x6e, 0x53, 0x63, 0x68, 0x6e, 0x6f, 0x72, 0x72, 0x53, 0x69, 0x67, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x42, 0x3f, 0x5a, 0x3d, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63,
	0x6f, 0x6d, 0x2f, 0x62, 0x61, 0x62, 0x79, 0x6c, 0x6f, 0x6e, 0x6c, 0x61, 0x62, 0x73, 0x2d, 0x69,
	0x6f, 0x2f, 0x66, 0x69, 0x6e, 0x61, 0x6c, 0x69, 0x74, 0x79, 0x2d, 0x70, 0x72, 0x6f, 0x76, 0x69,
	0x64, 0x65, 0x72, 0x2f, 0x65, 0x6f, 0x74, 0x73, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_eotsmanager_proto_rawDescOnce sync.Once
	file_eotsmanager_proto_rawDescData = file_eotsmanager_proto_rawDesc
)

func file_eotsmanager_proto_rawDescGZIP() []byte {
	file_eotsmanager_proto_rawDescOnce.Do(func() {
		file_eotsmanager_proto_rawDescData = protoimpl.X.CompressGZIP(file_eotsmanager_proto_rawDescData)
	})
	return file_eotsmanager_proto_rawDescData
}

var file_eotsmanager_proto_msgTypes = make([]protoimpl.MessageInfo, 12)
var file_eotsmanager_proto_goTypes = []interface{}{
	(*PingRequest)(nil),                      // 0: proto.PingRequest
	(*PingResponse)(nil),                     // 1: proto.PingResponse
	(*CreateKeyRequest)(nil),                 // 2: proto.CreateKeyRequest
	(*CreateKeyResponse)(nil),                // 3: proto.CreateKeyResponse
	(*CreateRandomnessPairListRequest)(nil),  // 4: proto.CreateRandomnessPairListRequest
	(*CreateRandomnessPairListResponse)(nil), // 5: proto.CreateRandomnessPairListResponse
	(*KeyRecordRequest)(nil),                 // 6: proto.KeyRecordRequest
	(*KeyRecordResponse)(nil),                // 7: proto.KeyRecordResponse
	(*SignEOTSRequest)(nil),                  // 8: proto.SignEOTSRequest
	(*SignEOTSResponse)(nil),                 // 9: proto.SignEOTSResponse
	(*SignSchnorrSigRequest)(nil),            // 10: proto.SignSchnorrSigRequest
	(*SignSchnorrSigResponse)(nil),           // 11: proto.SignSchnorrSigResponse
}
var file_eotsmanager_proto_depIdxs = []int32{
	0,  // 0: proto.EOTSManager.Ping:input_type -> proto.PingRequest
	2,  // 1: proto.EOTSManager.CreateKey:input_type -> proto.CreateKeyRequest
	4,  // 2: proto.EOTSManager.CreateRandomnessPairList:input_type -> proto.CreateRandomnessPairListRequest
	6,  // 3: proto.EOTSManager.KeyRecord:input_type -> proto.KeyRecordRequest
	8,  // 4: proto.EOTSManager.SignEOTS:input_type -> proto.SignEOTSRequest
	8,  // 5: proto.EOTSManager.UnsafeSignEOTS:input_type -> proto.SignEOTSRequest
	10, // 6: proto.EOTSManager.SignSchnorrSig:input_type -> proto.SignSchnorrSigRequest
	1,  // 7: proto.EOTSManager.Ping:output_type -> proto.PingResponse
	3,  // 8: proto.EOTSManager.CreateKey:output_type -> proto.CreateKeyResponse
	5,  // 9: proto.EOTSManager.CreateRandomnessPairList:output_type -> proto.CreateRandomnessPairListResponse
	7,  // 10: proto.EOTSManager.KeyRecord:output_type -> proto.KeyRecordResponse
	9,  // 11: proto.EOTSManager.SignEOTS:output_type -> proto.SignEOTSResponse
	9,  // 12: proto.EOTSManager.UnsafeSignEOTS:output_type -> proto.SignEOTSResponse
	11, // 13: proto.EOTSManager.SignSchnorrSig:output_type -> proto.SignSchnorrSigResponse
	7,  // [7:14] is the sub-list for method output_type
	0,  // [0:7] is the sub-list for method input_type
	0,  // [0:0] is the sub-list for extension type_name
	0,  // [0:0] is the sub-list for extension extendee
	0,  // [0:0] is the sub-list for field type_name
}

func init() { file_eotsmanager_proto_init() }
func file_eotsmanager_proto_init() {
	if File_eotsmanager_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_eotsmanager_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PingRequest); i {
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
		file_eotsmanager_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PingResponse); i {
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
		file_eotsmanager_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CreateKeyRequest); i {
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
		file_eotsmanager_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CreateKeyResponse); i {
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
		file_eotsmanager_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CreateRandomnessPairListRequest); i {
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
		file_eotsmanager_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CreateRandomnessPairListResponse); i {
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
		file_eotsmanager_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*KeyRecordRequest); i {
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
		file_eotsmanager_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*KeyRecordResponse); i {
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
		file_eotsmanager_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SignEOTSRequest); i {
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
		file_eotsmanager_proto_msgTypes[9].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SignEOTSResponse); i {
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
		file_eotsmanager_proto_msgTypes[10].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SignSchnorrSigRequest); i {
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
		file_eotsmanager_proto_msgTypes[11].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SignSchnorrSigResponse); i {
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
			RawDescriptor: file_eotsmanager_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   12,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_eotsmanager_proto_goTypes,
		DependencyIndexes: file_eotsmanager_proto_depIdxs,
		MessageInfos:      file_eotsmanager_proto_msgTypes,
	}.Build()
	File_eotsmanager_proto = out.File
	file_eotsmanager_proto_rawDesc = nil
	file_eotsmanager_proto_goTypes = nil
	file_eotsmanager_proto_depIdxs = nil
}
