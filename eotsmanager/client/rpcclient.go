package client

import (
	"context"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/proto"
)

var _ eotsmanager.EOTSManager = &EOTSManagerGRPCClient{}

type EOTSManagerGRPCClient struct {
	client proto.EOTSManagerClient
	conn   *grpc.ClientConn
}

// NewEOTSManagerGRPCClient creates a new EOTS manager gRPC client
// The hmacKey parameter is used for authentication with the EOTS manager server
func NewEOTSManagerGRPCClient(remoteAddr string, hmacKey string, grpcOpts ...grpc.DialOption) (*EOTSManagerGRPCClient, error) {
	processedHmacKey, err := ProcessHMACKey(hmacKey)
	if err != nil {
		// Log warning and continue without HMAC authentication
		fmt.Printf("Warning: Failed to process HMAC key: %v\n", err)
	}

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	if len(grpcOpts) > 0 {
		dialOpts = append(dialOpts, grpcOpts...)
	}

	// Add HMAC interceptor if key is available
	if processedHmacKey != "" {
		dialOpts = append(dialOpts, grpc.WithUnaryInterceptor(HMACUnaryClientInterceptor(processedHmacKey)))
	} else {
		fmt.Printf("Warning: HMAC key not configured. Authentication will not be enabled.\n")
	}

	conn, err := grpc.NewClient(remoteAddr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build gRPC connection to %s: %w", remoteAddr, err)
	}

	gClient := &EOTSManagerGRPCClient{
		client: proto.NewEOTSManagerClient(conn),
		conn:   conn,
	}

	if err := gClient.Ping(); err != nil {
		return nil, fmt.Errorf("the EOTS manager server is not responding: %w", err)
	}

	return gClient, nil
}

func (c *EOTSManagerGRPCClient) Ping() error {
	req := &proto.PingRequest{}

	_, err := c.client.Ping(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to ping EOTS manager: %w", err)
	}

	return nil
}

func (c *EOTSManagerGRPCClient) CreateRandomnessPairList(uid, chainID []byte, startHeight uint64, num uint32, options ...eotsmanager.RandomnessOption) ([]*btcec.FieldVal, error) {
	cfg := &eotsmanager.RandomnessConfig{}
	for _, opt := range options {
		opt(cfg)
	}

	req := &proto.CreateRandomnessPairListRequest{
		Uid:         uid,
		ChainId:     chainID,
		StartHeight: startHeight,
		Num:         num,
	}

	// Set interval if specified
	if cfg.Interval != nil {
		req.Interval = cfg.Interval
	}

	res, err := c.client.CreateRandomnessPairList(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("failed to create randomness pair list: %w", err)
	}

	pubRandFieldValList := make([]*btcec.FieldVal, 0, len(res.PubRandList))
	for _, r := range res.PubRandList {
		var fieldVal btcec.FieldVal
		fieldVal.SetByteSlice(r)
		pubRandFieldValList = append(pubRandFieldValList, &fieldVal)
	}

	return pubRandFieldValList, nil
}

func (c *EOTSManagerGRPCClient) SaveEOTSKeyName(pk *btcec.PublicKey, keyName string) error {
	req := &proto.SaveEOTSKeyNameRequest{
		KeyName: keyName,
		EotsPk:  pk.SerializeUncompressed(),
	}
	_, err := c.client.SaveEOTSKeyName(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to save EOTS key name: %w", err)
	}

	return nil
}

func (c *EOTSManagerGRPCClient) SignEOTS(uid, chaiID, msg []byte, height uint64) (*btcec.ModNScalar, error) {
	req := &proto.SignEOTSRequest{
		Uid:     uid,
		ChainId: chaiID,
		Msg:     msg,
		Height:  height,
	}
	res, err := c.client.SignEOTS(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("failed to sign EOTS: %w", err)
	}

	var s btcec.ModNScalar
	s.SetByteSlice(res.Sig)

	return &s, nil
}

func (c *EOTSManagerGRPCClient) UnsafeSignEOTS(uid, chaiID, msg []byte, height uint64) (*btcec.ModNScalar, error) {
	req := &proto.SignEOTSRequest{
		Uid:     uid,
		ChainId: chaiID,
		Msg:     msg,
		Height:  height,
	}
	res, err := c.client.UnsafeSignEOTS(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("failed to unsafe sign EOTS: %w", err)
	}

	var s btcec.ModNScalar
	s.SetByteSlice(res.Sig)

	return &s, nil
}

func (c *EOTSManagerGRPCClient) SignBatchEOTS(req *eotsmanager.SignBatchEOTSRequest) ([]eotsmanager.SignDataResponse, error) {
	signRequests := make([]*proto.SignDataRequest, len(req.SignRequest))
	for i, signReq := range req.SignRequest {
		signRequests[i] = &proto.SignDataRequest{
			Msg:    signReq.Msg,
			Height: signReq.Height,
		}
	}

	protoReq := &proto.SignBatchEOTSRequest{
		Uid:          req.UID,
		ChainId:      req.ChainID,
		SignRequests: signRequests,
	}

	res, err := c.client.SignBatchEOTS(context.Background(), protoReq)
	if err != nil {
		return nil, fmt.Errorf("failed to sign batch EOTS: %w", err)
	}

	responses := make([]eotsmanager.SignDataResponse, len(res.Responses))
	for i, resp := range res.Responses {
		var s btcec.ModNScalar
		s.SetByteSlice(resp.Sig)

		responses[i] = eotsmanager.SignDataResponse{
			Signature: &s,
			Height:    resp.Height,
		}
	}

	return responses, nil
}

func (c *EOTSManagerGRPCClient) SignSchnorrSig(uid, msg []byte) (*schnorr.Signature, error) {
	req := &proto.SignSchnorrSigRequest{Uid: uid, Msg: msg}
	res, err := c.client.SignSchnorrSig(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("failed to sign Schnorr signature: %w", err)
	}

	sig, err := schnorr.ParseSignature(res.Sig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Schnorr signature: %w", err)
	}

	return sig, nil
}

func (c *EOTSManagerGRPCClient) Unlock(uid []byte, passphrase string) error {
	req := &proto.UnlockKeyRequest{
		Uid:        uid,
		Passphrase: passphrase,
	}

	_, err := c.client.UnlockKey(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to unlock key: %w", err)
	}

	return nil
}

func (c *EOTSManagerGRPCClient) Backup(dbPath string, backupDir string) (string, error) {
	req := &proto.BackupRequest{
		DbPath:    dbPath,
		BackupDir: backupDir,
	}

	res, err := c.client.Backup(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("failed to backup: %w", err)
	}

	return res.BackupName, nil
}

func (c *EOTSManagerGRPCClient) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("failed to close EOTS manager client connection: %w", err)
	}

	return nil
}
