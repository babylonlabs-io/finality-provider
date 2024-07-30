package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"cosmossdk.io/math"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"google.golang.org/grpc"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/babylonlabs-io/finality-provider/version"
)

// rpcServer is the main RPC server for the Finality Provider daemon that handles
// gRPC incoming requests.
type rpcServer struct {
	started  int32
	shutdown int32

	proto.UnimplementedFinalityProvidersServer

	app *FinalityProviderApp

	quit chan struct{}
	wg   sync.WaitGroup
}

// newRPCServer creates a new RPC sever from the set of input dependencies.
func newRPCServer(
	fpa *FinalityProviderApp,
) *rpcServer {

	return &rpcServer{
		quit: make(chan struct{}),
		app:  fpa,
	}
}

// Start signals that the RPC server starts accepting requests.
func (r *rpcServer) Start() error {
	if atomic.AddInt32(&r.started, 1) != 1 {
		return nil
	}

	return nil
}

// Stop signals that the RPC server should attempt a graceful shutdown and
// cancel any outstanding requests.
func (r *rpcServer) Stop() error {
	if atomic.AddInt32(&r.shutdown, 1) != 1 {
		return nil
	}

	close(r.quit)

	r.wg.Wait()

	return nil
}

// RegisterWithGrpcServer registers the rpcServer with the passed root gRPC
// server.
func (r *rpcServer) RegisterWithGrpcServer(grpcServer *grpc.Server) error {
	// Register the main RPC server.
	proto.RegisterFinalityProvidersServer(grpcServer, r)
	return nil
}

// GetInfo returns general information relating to the active daemon
func (r *rpcServer) GetInfo(context.Context, *proto.GetInfoRequest) (*proto.GetInfoResponse, error) {

	return &proto.GetInfoResponse{
		Version: version.Version(),
	}, nil
}

// CreateFinalityProvider generates a finality-provider object and saves it in the database
func (r *rpcServer) CreateFinalityProvider(ctx context.Context, req *proto.CreateFinalityProviderRequest) (
	*proto.CreateFinalityProviderResponse, error) {

	commissionRate, err := math.LegacyNewDecFromStr(req.Commission)
	if err != nil {
		return nil, err
	}

	var description stakingtypes.Description
	if err := description.Unmarshal(req.Description); err != nil {
		return nil, err
	}

	result, err := r.app.CreateFinalityProvider(
		req.KeyName,
		req.ChainId,
		req.Passphrase,
		req.HdPath,
		&description,
		&commissionRate,
	)

	if err != nil {
		return nil, err
	}

	return &proto.CreateFinalityProviderResponse{
		FinalityProvider: result.FpInfo,
	}, nil
}

// RegisterFinalityProvider sends a transactions to Babylon to register a BTC finality-provider
func (r *rpcServer) RegisterFinalityProvider(ctx context.Context, req *proto.RegisterFinalityProviderRequest) (
	*proto.RegisterFinalityProviderResponse, error) {

	txRes, err := r.app.RegisterFinalityProvider(req.BtcPk)
	if err != nil {
		return nil, fmt.Errorf("failed to register the finality-provider to Babylon: %w", err)
	}

	// the finality-provider instance should be started right after registration
	if err := r.app.StartHandlingFinalityProvider(txRes.btcPubKey, req.Passphrase); err != nil {
		return nil, fmt.Errorf("failed to start the registered finality-provider %s: %w", txRes.bbnAddress.String(), err)
	}

	return &proto.RegisterFinalityProviderResponse{TxHash: txRes.TxHash}, nil
}

// AddFinalitySignature adds a manually constructed finality signature to Babylon
// NOTE: this is only used for presentation/testing purposes
func (r *rpcServer) AddFinalitySignature(ctx context.Context, req *proto.AddFinalitySignatureRequest) (
	*proto.AddFinalitySignatureResponse, error) {

	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(req.BtcPk)
	if err != nil {
		return nil, err
	}

	fpi, err := r.app.GetFinalityProviderInstance(fpPk)
	if err != nil {
		return nil, err
	}

	b := &types.BlockInfo{
		Height: req.Height,
		Hash:   req.AppHash,
	}

	txRes, privKey, err := fpi.TestSubmitFinalitySignatureAndExtractPrivKey(b)
	if err != nil {
		return nil, err
	}

	res := &proto.AddFinalitySignatureResponse{TxHash: txRes.TxHash}

	// if privKey is not empty, then this BTC finality-provider
	// has voted for a fork and will be slashed
	if privKey != nil {
		localPrivKey, err := r.app.getFpPrivKey(fpPk.MustMarshal())
		res.ExtractedSkHex = privKey.Key.String()
		if err != nil {
			return nil, err
		}
		localSkHex := localPrivKey.Key.String()
		localSkNegateHex := localPrivKey.Key.Negate().String()
		if res.ExtractedSkHex == localSkHex {
			res.LocalSkHex = localSkHex
		} else if res.ExtractedSkHex == localSkNegateHex {
			res.LocalSkHex = localSkNegateHex
		} else {
			return nil, fmt.Errorf("the finality-provider's BTC private key is extracted but does not match the local key,"+
				"extrated: %s, local: %s, local-negated: %s",
				res.ExtractedSkHex, localSkHex, localSkNegateHex)
		}
	}

	return res, nil
}

// QueryFinalityProvider queries the information of the finality-provider
func (r *rpcServer) QueryFinalityProvider(ctx context.Context, req *proto.QueryFinalityProviderRequest) (
	*proto.QueryFinalityProviderResponse, error) {

	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(req.BtcPk)
	if err != nil {
		return nil, err
	}
	fp, err := r.app.GetFinalityProviderInfo(fpPk)
	if err != nil {
		return nil, err
	}

	return &proto.QueryFinalityProviderResponse{FinalityProvider: fp}, nil
}

// QueryFinalityProviderList queries the information of a list of finality providers
func (r *rpcServer) QueryFinalityProviderList(ctx context.Context, req *proto.QueryFinalityProviderListRequest) (
	*proto.QueryFinalityProviderListResponse, error) {

	fps, err := r.app.ListAllFinalityProvidersInfo()
	if err != nil {
		return nil, err
	}

	return &proto.QueryFinalityProviderListResponse{FinalityProviders: fps}, nil
}

// SignMessageFromChainKey signs a message from the chain keyring.
func (r *rpcServer) SignMessageFromChainKey(ctx context.Context, req *proto.SignMessageFromChainKeyRequest) (
	*proto.SignMessageFromChainKeyResponse, error) {
	signature, err := r.app.SignRawMsg(req.KeyName, req.Passphrase, req.HdPath, req.MsgToSign)
	if err != nil {
		return nil, err
	}

	return &proto.SignMessageFromChainKeyResponse{Signature: signature}, nil
}
