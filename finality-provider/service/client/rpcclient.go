package client

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
)

type FinalityProviderServiceGRpcClient struct {
	client proto.FinalityProvidersClient
}

// NewFinalityProviderServiceGRpcClient creates a new GRPC connection with finality provider daemon.
func NewFinalityProviderServiceGRpcClient(remoteAddr string) (*FinalityProviderServiceGRpcClient, func() error, error) {
	conn, err := grpc.NewClient(remoteAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build gRPC connection to %s: %w", remoteAddr, err)
	}

	cleanUp := func() error {
		if conn == nil {
			return nil
		}

		return conn.Close()
	}

	return &FinalityProviderServiceGRpcClient{
		client: proto.NewFinalityProvidersClient(conn),
	}, cleanUp, nil
}

func (c *FinalityProviderServiceGRpcClient) GetInfo(ctx context.Context) (*proto.GetInfoResponse, error) {
	req := &proto.GetInfoRequest{}
	res, err := c.client.GetInfo(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (c *FinalityProviderServiceGRpcClient) CreateFinalityProvider(
	ctx context.Context,
	keyName, chainID, eotsPkHex, passphrase string,
	description types.Description,
	commission *sdkmath.LegacyDec,
) (*proto.CreateFinalityProviderResponse, error) {
	descBytes, err := description.Marshal()
	if err != nil {
		return nil, err
	}

	req := &proto.CreateFinalityProviderRequest{
		KeyName:     keyName,
		ChainId:     chainID,
		Passphrase:  passphrase,
		Description: descBytes,
		Commission:  commission.String(),
		EotsPkHex:   eotsPkHex,
	}

	res, err := c.client.CreateFinalityProvider(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (c *FinalityProviderServiceGRpcClient) AddFinalitySignature(
	ctx context.Context,
	fpPk string,
	height uint64,
	appHash []byte,
	checkDoubleSign bool,
) (*proto.AddFinalitySignatureResponse, error) {
	req := &proto.AddFinalitySignatureRequest{
		BtcPk:           fpPk,
		Height:          height,
		AppHash:         appHash,
		CheckDoubleSign: checkDoubleSign,
	}

	res, err := c.client.AddFinalitySignature(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (c *FinalityProviderServiceGRpcClient) UnjailFinalityProvider(ctx context.Context, fpPk string) (*proto.UnjailFinalityProviderResponse, error) {
	req := &proto.UnjailFinalityProviderRequest{
		BtcPk: fpPk,
	}

	res, err := c.client.UnjailFinalityProvider(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (c *FinalityProviderServiceGRpcClient) QueryFinalityProviderList(ctx context.Context) (*proto.QueryFinalityProviderListResponse, error) {
	req := &proto.QueryFinalityProviderListRequest{}
	res, err := c.client.QueryFinalityProviderList(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// QueryFinalityProviderInfo - gets the finality provider data from local store
func (c *FinalityProviderServiceGRpcClient) QueryFinalityProviderInfo(ctx context.Context, fpPk *bbntypes.BIP340PubKey) (*proto.QueryFinalityProviderResponse, error) {
	req := &proto.QueryFinalityProviderRequest{BtcPk: fpPk.MarshalHex()}
	res, err := c.client.QueryFinalityProvider(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// EditFinalityProvider - edit the finality provider data.
func (c *FinalityProviderServiceGRpcClient) EditFinalityProvider(
	ctx context.Context, fpPk *bbntypes.BIP340PubKey, desc *proto.Description, rate string) error {
	if rate == "" {
		currentProvider, err := c.QueryFinalityProviderInfo(ctx, fpPk)
		if err != nil {
			return fmt.Errorf("failed to get current provider info: %w", err)
		}
		rate = currentProvider.FinalityProvider.Commission
	}

	req := &proto.EditFinalityProviderRequest{BtcPk: fpPk.MarshalHex(), Description: desc, Commission: rate}

	_, err := c.client.EditFinalityProvider(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to edit finality provider: %w", err)
	}

	return nil
}

// UnsafeRemoveMerkleProof - remove all proofs up to target height
func (c *FinalityProviderServiceGRpcClient) UnsafeRemoveMerkleProof(
	ctx context.Context, fpPk *bbntypes.BIP340PubKey, chainID string, targetHeight uint64) error {
	req := &proto.RemoveMerkleProofRequest{BtcPkHex: fpPk.MarshalHex(), ChainId: chainID, TargetHeight: targetHeight}
	_, err := c.client.UnsafeRemoveMerkleProof(ctx, req)
	if err != nil {
		return err
	}

	return nil
}
