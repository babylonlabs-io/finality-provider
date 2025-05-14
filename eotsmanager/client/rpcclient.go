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
		return err
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
		return nil, err
	}

	pubRandFieldValList := make([]*btcec.FieldVal, 0, len(res.PubRandList))
	for _, r := range res.PubRandList {
		var fieldVal btcec.FieldVal
		fieldVal.SetByteSlice(r)
		pubRandFieldValList = append(pubRandFieldValList, &fieldVal)
	}

	return pubRandFieldValList, nil
}

func (c *EOTSManagerGRpcClient) SaveEOTSKeyName(pk *btcec.PublicKey, keyName string) error {
	req := &proto.SaveEOTSKeyNameRequest{
		KeyName: keyName,
		EotsPk:  pk.SerializeUncompressed(),
	}
	_, err := c.client.SaveEOTSKeyName(context.Background(), req)

	return err
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
		return nil, err
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
		return nil, err
	}

	var s btcec.ModNScalar
	s.SetByteSlice(res.Sig)

	return &s, nil
}

func (c *EOTSManagerGRpcClient) SignSchnorrSig(uid, msg []byte) (*schnorr.Signature, error) {
	req := &proto.SignSchnorrSigRequest{Uid: uid, Msg: msg}
	res, err := c.client.SignSchnorrSig(context.Background(), req)
	if err != nil {
		return nil, err
	}

	sig, err := schnorr.ParseSignature(res.Sig)
	if err != nil {
		return nil, err
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
		return err
	}

	return nil
}

func (c *EOTSManagerGRpcClient) Backup(dbPath string, backupDir string) error {
	req := &proto.BackupRequest{
		DbPath:    dbPath,
		BackupDir: backupDir,
	}

	if _, err := c.client.Backup(context.Background(), req); err != nil {
		return err
	}

	return nil
}

func (c *EOTSManagerGRpcClient) Close() error {
	return c.conn.Close()
}
