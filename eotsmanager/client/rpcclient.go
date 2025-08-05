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

var _ eotsmanager.EOTSManager = &EOTSManagerGRpcClient{}

type EOTSManagerGRpcClient struct {
	client proto.EOTSManagerClient
	conn   *grpc.ClientConn
}

// NewEOTSManagerGRpcClient creates a new EOTS manager gRPC client
// The hmacKey parameter is used for authentication with the EOTS manager server
func NewEOTSManagerGRpcClient(remoteAddr string, hmacKey string) (*EOTSManagerGRpcClient, error) {
	processedHmacKey, err := ProcessHMACKey(hmacKey)
	if err != nil {
		// Log warning and continue without HMAC authentication
		fmt.Printf("Warning: Failed to process HMAC key: %v\n", err)
	}

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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

	gClient := &EOTSManagerGRpcClient{
		client: proto.NewEOTSManagerClient(conn),
		conn:   conn,
	}

	if err := gClient.Ping(); err != nil {
		return nil, fmt.Errorf("the EOTS manager server is not responding: %w", err)
	}

	return gClient, nil
}

func (c *EOTSManagerGRpcClient) Ping() error {
	req := &proto.PingRequest{}

	_, err := c.client.Ping(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to ping EOTS manager: %w", err)
	}

	return nil
}

func (c *EOTSManagerGRpcClient) CreateRandomnessPairList(uid, chainID []byte, startHeight uint64, num uint32) ([]*btcec.FieldVal, error) {
	req := &proto.CreateRandomnessPairListRequest{
		Uid:         uid,
		ChainId:     chainID,
		StartHeight: startHeight,
		Num:         num,
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

func (c *EOTSManagerGRpcClient) CreateRandomnessPairListWithInterval(uid, chainID []byte, startHeight uint64, num uint32, interval uint64) ([]*btcec.FieldVal, error) {
	// For now, implement using existing RPC by calling individual heights
	// TODO: Later can add dedicated GRPC method for efficiency
	pubRandList := make([]*btcec.FieldVal, 0, num)

	for i := uint32(0); i < num; i++ {
		height := startHeight + uint64(i)*interval
		singleList, err := c.CreateRandomnessPairList(uid, chainID, height, 1)
		if err != nil {
			return nil, fmt.Errorf("failed to create randomness for height %d: %w", height, err)
		}
		if len(singleList) != 1 {
			return nil, fmt.Errorf("expected 1 randomness value, got %d", len(singleList))
		}
		pubRandList = append(pubRandList, singleList[0])
	}

	return pubRandList, nil
}

func (c *EOTSManagerGRpcClient) SaveEOTSKeyName(pk *btcec.PublicKey, keyName string) error {
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

func (c *EOTSManagerGRpcClient) SignEOTS(uid, chaiID, msg []byte, height uint64) (*btcec.ModNScalar, error) {
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

func (c *EOTSManagerGRpcClient) UnsafeSignEOTS(uid, chaiID, msg []byte, height uint64) (*btcec.ModNScalar, error) {
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

func (c *EOTSManagerGRpcClient) SignSchnorrSig(uid, msg []byte) (*schnorr.Signature, error) {
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

func (c *EOTSManagerGRpcClient) Unlock(uid []byte, passphrase string) error {
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

func (c *EOTSManagerGRpcClient) Backup(dbPath string, backupDir string) (string, error) {
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

func (c *EOTSManagerGRpcClient) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("failed to close EOTS manager client connection: %w", err)
	}

	return nil
}
