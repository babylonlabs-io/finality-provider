package client

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	bbntypes "github.com/babylonchain/babylon/types"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
)

type FinalityProviderServiceGRpcClient struct {
	client proto.FinalityProvidersClient
}

// NewFinalityProviderServiceGRpcClient creates a new GRPC connection with finality provider daemon.
func NewFinalityProviderServiceGRpcClient(remoteAddr string) (client *FinalityProviderServiceGRpcClient, cleanUp func(), err error) {
	conn, err := grpc.Dial(remoteAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build gRPC connection to %s: %w", remoteAddr, err)
	}

	cleanUp = func() {
		conn.Close()
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

func (c *FinalityProviderServiceGRpcClient) RegisterFinalityProvider(
	ctx context.Context,
	fpPk *bbntypes.BIP340PubKey,
	passphrase string,
) (*proto.RegisterFinalityProviderResponse, error) {

	req := &proto.RegisterFinalityProviderRequest{BtcPk: fpPk.MarshalHex(), Passphrase: passphrase}
	res, err := c.client.RegisterFinalityProvider(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (c *FinalityProviderServiceGRpcClient) CreateFinalityProvider(
	ctx context.Context,
	keyName, chainID, passphrase, hdPath string,
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
		HdPath:      hdPath,
		Description: descBytes,
		Commission:  commission.String(),
	}

	res, err := c.client.CreateFinalityProvider(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (c *FinalityProviderServiceGRpcClient) AddFinalitySignature(ctx context.Context, fpPk string, height uint64, appHash []byte) (*proto.AddFinalitySignatureResponse, error) {
	req := &proto.AddFinalitySignatureRequest{
		BtcPk:   fpPk,
		Height:  height,
		AppHash: appHash,
	}

	res, err := c.client.AddFinalitySignature(ctx, req)
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

func (c *FinalityProviderServiceGRpcClient) QueryFinalityProviderInfo(ctx context.Context, fpPk *bbntypes.BIP340PubKey) (*proto.QueryFinalityProviderResponse, error) {
	req := &proto.QueryFinalityProviderRequest{BtcPk: fpPk.MarshalHex()}
	res, err := c.client.QueryFinalityProvider(ctx, req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (c *FinalityProviderServiceGRpcClient) SignMessageFromChainKey(
	ctx context.Context,
	keyName, passphrase, hdPath string,
	rawMsgToSign []byte,
) (*proto.SignMessageFromChainKeyResponse, error) {
	req := &proto.SignMessageFromChainKeyRequest{
		MsgToSign:  rawMsgToSign,
		KeyName:    keyName,
		Passphrase: passphrase,
		HdPath:     hdPath,
	}
	return c.client.SignMessageFromChainKey(ctx, req)
}
