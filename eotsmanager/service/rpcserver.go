package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/gogo/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/proto"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/types"
)

// rpcServer is the main RPC server for the EOTS daemon that handles
// gRPC incoming requests.
type rpcServer struct {
	proto.UnimplementedEOTSManagerServer

	em  *eotsmanager.LocalEOTSManager
	cfg *config.Config
}

// newRPCServer creates a new RPC sever from the set of input dependencies.
func newRPCServer(
	em *eotsmanager.LocalEOTSManager,
	cfg *config.Config,
) *rpcServer {
	return &rpcServer{
		em:  em,
		cfg: cfg,
	}
}

// RegisterWithGrpcServer registers the rpcServer with the passed root gRPC
// server.
func (r *rpcServer) RegisterWithGrpcServer(grpcServer *grpc.Server) error {
	// Register the main RPC server.
	proto.RegisterEOTSManagerServer(grpcServer, r)

	return nil
}

func (r *rpcServer) Ping(_ context.Context, _ *proto.PingRequest) (*proto.PingResponse, error) {
	return &proto.PingResponse{}, nil
}

// CreateRandomnessPairList returns a list of Schnorr randomness pairs
func (r *rpcServer) CreateRandomnessPairList(_ context.Context, req *proto.CreateRandomnessPairListRequest) (
	*proto.CreateRandomnessPairListResponse, error) {
	var options []eotsmanager.RandomnessOption
	if req.Interval != nil && *req.Interval > 0 {
		options = append(options, eotsmanager.WithInterval(*req.Interval))
	}

	pubRandList, err := r.em.CreateRandomnessPairList(req.Uid, req.ChainId, req.StartHeight, req.Num, options...)

	if err != nil {
		return nil, fmt.Errorf("failed to create randomness pair list: %w", err)
	}

	pubRandBytesList := make([][]byte, 0, len(pubRandList))
	for _, p := range pubRandList {
		pubRandBytesList = append(pubRandBytesList, p.Bytes()[:])
	}

	return &proto.CreateRandomnessPairListResponse{
		PubRandList: pubRandBytesList,
	}, nil
}

// SignEOTS signs an EOTS with the EOTS private key and the relevant randomness
func (r *rpcServer) SignEOTS(_ context.Context, req *proto.SignEOTSRequest) (
	*proto.SignEOTSResponse, error) {
	sig, err := r.em.SignEOTS(req.Uid, req.ChainId, req.Msg, req.Height)
	if err != nil {
		if errors.Is(err, types.ErrDoubleSign) {
			return nil, status.Error(codes.FailedPrecondition, err.Error()) //nolint:wrapcheck
		}

		return nil, fmt.Errorf("failed to sign EOTS: %w", err)
	}

	sigBytes := sig.Bytes()

	return &proto.SignEOTSResponse{Sig: sigBytes[:]}, nil
}

// UnsafeSignEOTS only used for testing purposes. Doesn't offer slashing protection!
func (r *rpcServer) UnsafeSignEOTS(_ context.Context, req *proto.SignEOTSRequest) (
	*proto.SignEOTSResponse, error) {
	if r.cfg.DisableUnsafeEndpoints {
		return nil, status.Error(codes.PermissionDenied,
			"UnsafeSignEOTS endpoint is disabled in configuration for security reasons")
	}

	sig, err := r.em.UnsafeSignEOTS(req.Uid, req.ChainId, req.Msg, req.Height)
	if err != nil {
		return nil, fmt.Errorf("failed to sign EOTS: %w", err)
	}

	sigBytes := sig.Bytes()

	return &proto.SignEOTSResponse{Sig: sigBytes[:]}, nil
}

// SignSchnorrSig signs a Schnorr sig with the EOTS private key
func (r *rpcServer) SignSchnorrSig(_ context.Context, req *proto.SignSchnorrSigRequest) (
	*proto.SignSchnorrSigResponse, error) {
	sig, err := r.em.SignSchnorrSig(req.Uid, req.Msg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign EOTS: %w", err)
	}

	return &proto.SignSchnorrSigResponse{Sig: sig.Serialize()}, nil
}

// SignBatchEOTS signs multiple EOTS in batch
func (r *rpcServer) SignBatchEOTS(_ context.Context, req *proto.SignBatchEOTSRequest) (
	*proto.SignBatchEOTSResponse, error) {
	signRequests := make([]*eotsmanager.SignDataRequest, len(req.SignRequests))
	for i, signReq := range req.SignRequests {
		signRequests[i] = &eotsmanager.SignDataRequest{
			Msg:    signReq.Msg,
			Height: signReq.Height,
		}
	}

	batchReq := &eotsmanager.SignBatchEOTSRequest{
		UID:         req.Uid,
		ChainID:     req.ChainId,
		SignRequest: signRequests,
	}

	responses, err := r.em.SignBatchEOTS(batchReq)
	if err != nil {
		if errors.Is(err, types.ErrDoubleSign) {
			return nil, status.Error(codes.FailedPrecondition, err.Error()) //nolint:wrapcheck
		}

		return nil, fmt.Errorf("failed to sign batch EOTS: %w", err)
	}

	protoResponses := make([]*proto.SignDataResponse, len(responses))
	for i, resp := range responses {
		sigBytes := resp.Signature.Bytes()
		protoResponses[i] = &proto.SignDataResponse{
			Sig:    sigBytes[:],
			Height: resp.Height,
		}
	}

	return &proto.SignBatchEOTSResponse{Responses: protoResponses}, nil
}

// SaveEOTSKeyName signs a Schnorr sig with the EOTS private key
func (r *rpcServer) SaveEOTSKeyName(
	_ context.Context,
	req *proto.SaveEOTSKeyNameRequest,
) (*proto.SaveEOTSKeyNameResponse, error) {
	eotsPk, err := btcec.ParsePubKey(req.EotsPk)
	if err != nil {
		return nil, fmt.Errorf("failed to parse EOTS public key: %w", err)
	}
	if err := r.em.SaveEOTSKeyName(eotsPk, req.KeyName); err != nil {
		return nil, fmt.Errorf("failed to save EOTS key name: %w", err)
	}

	return &proto.SaveEOTSKeyNameResponse{}, nil
}

func (r *rpcServer) Backup(_ context.Context, req *proto.BackupRequest) (*proto.BackupResponse, error) {
	backupName, err := r.em.Backup(req.DbPath, req.BackupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to backup: %w", err)
	}

	return &proto.BackupResponse{
		BackupName: backupName,
	}, nil
}

func (r *rpcServer) UnlockKey(_ context.Context, req *proto.UnlockKeyRequest) (*proto.UnlockKeyResponse, error) {
	err := r.em.Unlock(req.Uid, req.Passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to unlock key: %w", err)
	}

	return &proto.UnlockKeyResponse{}, nil
}
