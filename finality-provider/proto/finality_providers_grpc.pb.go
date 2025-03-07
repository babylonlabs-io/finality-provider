// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             (unknown)
// source: finality_providers.proto

package proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	FinalityProviders_GetInfo_FullMethodName                   = "/proto.FinalityProviders/GetInfo"
	FinalityProviders_CreateFinalityProvider_FullMethodName    = "/proto.FinalityProviders/CreateFinalityProvider"
	FinalityProviders_AddFinalitySignature_FullMethodName      = "/proto.FinalityProviders/AddFinalitySignature"
	FinalityProviders_UnjailFinalityProvider_FullMethodName    = "/proto.FinalityProviders/UnjailFinalityProvider"
	FinalityProviders_QueryFinalityProvider_FullMethodName     = "/proto.FinalityProviders/QueryFinalityProvider"
	FinalityProviders_QueryFinalityProviderList_FullMethodName = "/proto.FinalityProviders/QueryFinalityProviderList"
	FinalityProviders_EditFinalityProvider_FullMethodName      = "/proto.FinalityProviders/EditFinalityProvider"
	FinalityProviders_UnsafeRemoveMerkleProof_FullMethodName   = "/proto.FinalityProviders/UnsafeRemoveMerkleProof"
)

// FinalityProvidersClient is the client API for FinalityProviders service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type FinalityProvidersClient interface {
	// GetInfo returns the information of the daemon
	GetInfo(ctx context.Context, in *GetInfoRequest, opts ...grpc.CallOption) (*GetInfoResponse, error)
	// CreateFinalityProvider generates and saves a finality provider object
	CreateFinalityProvider(ctx context.Context, in *CreateFinalityProviderRequest, opts ...grpc.CallOption) (*CreateFinalityProviderResponse, error)
	// AddFinalitySignature sends a transactions to the consumer chain to add a
	// Finality signature for a block
	AddFinalitySignature(ctx context.Context, in *AddFinalitySignatureRequest, opts ...grpc.CallOption) (*AddFinalitySignatureResponse, error)
	// UnjailFinalityProvider sends a transactions to the consumer chain to
	// unjail a given finality provider
	UnjailFinalityProvider(ctx context.Context, in *UnjailFinalityProviderRequest, opts ...grpc.CallOption) (*UnjailFinalityProviderResponse, error)
	// QueryFinalityProvider queries the finality provider
	QueryFinalityProvider(ctx context.Context, in *QueryFinalityProviderRequest, opts ...grpc.CallOption) (*QueryFinalityProviderResponse, error)
	// QueryFinalityProviderList queries a list of finality providers
	QueryFinalityProviderList(ctx context.Context, in *QueryFinalityProviderListRequest, opts ...grpc.CallOption) (*QueryFinalityProviderListResponse, error)
	// EditFinalityProvider edits finality provider
	EditFinalityProvider(ctx context.Context, in *EditFinalityProviderRequest, opts ...grpc.CallOption) (*EmptyResponse, error)
	// UnsafeRemoveMerkleProof removes merkle proofs up to target height
	UnsafeRemoveMerkleProof(ctx context.Context, in *RemoveMerkleProofRequest, opts ...grpc.CallOption) (*EmptyResponse, error)
}

type finalityProvidersClient struct {
	cc grpc.ClientConnInterface
}

func NewFinalityProvidersClient(cc grpc.ClientConnInterface) FinalityProvidersClient {
	return &finalityProvidersClient{cc}
}

func (c *finalityProvidersClient) GetInfo(ctx context.Context, in *GetInfoRequest, opts ...grpc.CallOption) (*GetInfoResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetInfoResponse)
	err := c.cc.Invoke(ctx, FinalityProviders_GetInfo_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *finalityProvidersClient) CreateFinalityProvider(ctx context.Context, in *CreateFinalityProviderRequest, opts ...grpc.CallOption) (*CreateFinalityProviderResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CreateFinalityProviderResponse)
	err := c.cc.Invoke(ctx, FinalityProviders_CreateFinalityProvider_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *finalityProvidersClient) AddFinalitySignature(ctx context.Context, in *AddFinalitySignatureRequest, opts ...grpc.CallOption) (*AddFinalitySignatureResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(AddFinalitySignatureResponse)
	err := c.cc.Invoke(ctx, FinalityProviders_AddFinalitySignature_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *finalityProvidersClient) UnjailFinalityProvider(ctx context.Context, in *UnjailFinalityProviderRequest, opts ...grpc.CallOption) (*UnjailFinalityProviderResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(UnjailFinalityProviderResponse)
	err := c.cc.Invoke(ctx, FinalityProviders_UnjailFinalityProvider_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *finalityProvidersClient) QueryFinalityProvider(ctx context.Context, in *QueryFinalityProviderRequest, opts ...grpc.CallOption) (*QueryFinalityProviderResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(QueryFinalityProviderResponse)
	err := c.cc.Invoke(ctx, FinalityProviders_QueryFinalityProvider_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *finalityProvidersClient) QueryFinalityProviderList(ctx context.Context, in *QueryFinalityProviderListRequest, opts ...grpc.CallOption) (*QueryFinalityProviderListResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(QueryFinalityProviderListResponse)
	err := c.cc.Invoke(ctx, FinalityProviders_QueryFinalityProviderList_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *finalityProvidersClient) EditFinalityProvider(ctx context.Context, in *EditFinalityProviderRequest, opts ...grpc.CallOption) (*EmptyResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(EmptyResponse)
	err := c.cc.Invoke(ctx, FinalityProviders_EditFinalityProvider_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *finalityProvidersClient) UnsafeRemoveMerkleProof(ctx context.Context, in *RemoveMerkleProofRequest, opts ...grpc.CallOption) (*EmptyResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(EmptyResponse)
	err := c.cc.Invoke(ctx, FinalityProviders_UnsafeRemoveMerkleProof_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FinalityProvidersServer is the server API for FinalityProviders service.
// All implementations must embed UnimplementedFinalityProvidersServer
// for forward compatibility.
type FinalityProvidersServer interface {
	// GetInfo returns the information of the daemon
	GetInfo(context.Context, *GetInfoRequest) (*GetInfoResponse, error)
	// CreateFinalityProvider generates and saves a finality provider object
	CreateFinalityProvider(context.Context, *CreateFinalityProviderRequest) (*CreateFinalityProviderResponse, error)
	// AddFinalitySignature sends a transactions to the consumer chain to add a
	// Finality signature for a block
	AddFinalitySignature(context.Context, *AddFinalitySignatureRequest) (*AddFinalitySignatureResponse, error)
	// UnjailFinalityProvider sends a transactions to the consumer chain to
	// unjail a given finality provider
	UnjailFinalityProvider(context.Context, *UnjailFinalityProviderRequest) (*UnjailFinalityProviderResponse, error)
	// QueryFinalityProvider queries the finality provider
	QueryFinalityProvider(context.Context, *QueryFinalityProviderRequest) (*QueryFinalityProviderResponse, error)
	// QueryFinalityProviderList queries a list of finality providers
	QueryFinalityProviderList(context.Context, *QueryFinalityProviderListRequest) (*QueryFinalityProviderListResponse, error)
	// EditFinalityProvider edits finality provider
	EditFinalityProvider(context.Context, *EditFinalityProviderRequest) (*EmptyResponse, error)
	// UnsafeRemoveMerkleProof removes merkle proofs up to target height
	UnsafeRemoveMerkleProof(context.Context, *RemoveMerkleProofRequest) (*EmptyResponse, error)
	mustEmbedUnimplementedFinalityProvidersServer()
}

// UnimplementedFinalityProvidersServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedFinalityProvidersServer struct{}

func (UnimplementedFinalityProvidersServer) GetInfo(context.Context, *GetInfoRequest) (*GetInfoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetInfo not implemented")
}
func (UnimplementedFinalityProvidersServer) CreateFinalityProvider(context.Context, *CreateFinalityProviderRequest) (*CreateFinalityProviderResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateFinalityProvider not implemented")
}
func (UnimplementedFinalityProvidersServer) AddFinalitySignature(context.Context, *AddFinalitySignatureRequest) (*AddFinalitySignatureResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AddFinalitySignature not implemented")
}
func (UnimplementedFinalityProvidersServer) UnjailFinalityProvider(context.Context, *UnjailFinalityProviderRequest) (*UnjailFinalityProviderResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UnjailFinalityProvider not implemented")
}
func (UnimplementedFinalityProvidersServer) QueryFinalityProvider(context.Context, *QueryFinalityProviderRequest) (*QueryFinalityProviderResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryFinalityProvider not implemented")
}
func (UnimplementedFinalityProvidersServer) QueryFinalityProviderList(context.Context, *QueryFinalityProviderListRequest) (*QueryFinalityProviderListResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryFinalityProviderList not implemented")
}
func (UnimplementedFinalityProvidersServer) EditFinalityProvider(context.Context, *EditFinalityProviderRequest) (*EmptyResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method EditFinalityProvider not implemented")
}
func (UnimplementedFinalityProvidersServer) UnsafeRemoveMerkleProof(context.Context, *RemoveMerkleProofRequest) (*EmptyResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UnsafeRemoveMerkleProof not implemented")
}
func (UnimplementedFinalityProvidersServer) mustEmbedUnimplementedFinalityProvidersServer() {}
func (UnimplementedFinalityProvidersServer) testEmbeddedByValue()                           {}

// UnsafeFinalityProvidersServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to FinalityProvidersServer will
// result in compilation errors.
type UnsafeFinalityProvidersServer interface {
	mustEmbedUnimplementedFinalityProvidersServer()
}

func RegisterFinalityProvidersServer(s grpc.ServiceRegistrar, srv FinalityProvidersServer) {
	// If the following call pancis, it indicates UnimplementedFinalityProvidersServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&FinalityProviders_ServiceDesc, srv)
}

func _FinalityProviders_GetInfo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetInfoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FinalityProvidersServer).GetInfo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FinalityProviders_GetInfo_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FinalityProvidersServer).GetInfo(ctx, req.(*GetInfoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FinalityProviders_CreateFinalityProvider_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateFinalityProviderRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FinalityProvidersServer).CreateFinalityProvider(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FinalityProviders_CreateFinalityProvider_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FinalityProvidersServer).CreateFinalityProvider(ctx, req.(*CreateFinalityProviderRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FinalityProviders_AddFinalitySignature_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AddFinalitySignatureRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FinalityProvidersServer).AddFinalitySignature(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FinalityProviders_AddFinalitySignature_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FinalityProvidersServer).AddFinalitySignature(ctx, req.(*AddFinalitySignatureRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FinalityProviders_UnjailFinalityProvider_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UnjailFinalityProviderRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FinalityProvidersServer).UnjailFinalityProvider(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FinalityProviders_UnjailFinalityProvider_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FinalityProvidersServer).UnjailFinalityProvider(ctx, req.(*UnjailFinalityProviderRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FinalityProviders_QueryFinalityProvider_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryFinalityProviderRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FinalityProvidersServer).QueryFinalityProvider(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FinalityProviders_QueryFinalityProvider_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FinalityProvidersServer).QueryFinalityProvider(ctx, req.(*QueryFinalityProviderRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FinalityProviders_QueryFinalityProviderList_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryFinalityProviderListRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FinalityProvidersServer).QueryFinalityProviderList(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FinalityProviders_QueryFinalityProviderList_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FinalityProvidersServer).QueryFinalityProviderList(ctx, req.(*QueryFinalityProviderListRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FinalityProviders_EditFinalityProvider_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(EditFinalityProviderRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FinalityProvidersServer).EditFinalityProvider(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FinalityProviders_EditFinalityProvider_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FinalityProvidersServer).EditFinalityProvider(ctx, req.(*EditFinalityProviderRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FinalityProviders_UnsafeRemoveMerkleProof_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RemoveMerkleProofRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FinalityProvidersServer).UnsafeRemoveMerkleProof(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: FinalityProviders_UnsafeRemoveMerkleProof_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FinalityProvidersServer).UnsafeRemoveMerkleProof(ctx, req.(*RemoveMerkleProofRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// FinalityProviders_ServiceDesc is the grpc.ServiceDesc for FinalityProviders service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var FinalityProviders_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.FinalityProviders",
	HandlerType: (*FinalityProvidersServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetInfo",
			Handler:    _FinalityProviders_GetInfo_Handler,
		},
		{
			MethodName: "CreateFinalityProvider",
			Handler:    _FinalityProviders_CreateFinalityProvider_Handler,
		},
		{
			MethodName: "AddFinalitySignature",
			Handler:    _FinalityProviders_AddFinalitySignature_Handler,
		},
		{
			MethodName: "UnjailFinalityProvider",
			Handler:    _FinalityProviders_UnjailFinalityProvider_Handler,
		},
		{
			MethodName: "QueryFinalityProvider",
			Handler:    _FinalityProviders_QueryFinalityProvider_Handler,
		},
		{
			MethodName: "QueryFinalityProviderList",
			Handler:    _FinalityProviders_QueryFinalityProviderList_Handler,
		},
		{
			MethodName: "EditFinalityProvider",
			Handler:    _FinalityProviders_EditFinalityProvider_Handler,
		},
		{
			MethodName: "UnsafeRemoveMerkleProof",
			Handler:    _FinalityProviders_UnsafeRemoveMerkleProof_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "finality_providers.proto",
}
