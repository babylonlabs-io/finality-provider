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
	"github.com/babylonlabs-io/finality-provider/eotsmanager/types"
)

var _ eotsmanager.EOTSManager = &EOTSManagerGRpcClient{}

type EOTSManagerGRpcClient struct {
	client proto.EOTSManagerClient
	conn   *grpc.ClientConn
}

func NewEOTSManagerGRpcClient(remoteAddr string) (*EOTSManagerGRpcClient, error) {
	conn, err := grpc.NewClient(remoteAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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

func (c *EOTSManagerGRpcClient) CreateKey(name, passphrase, hdPath string) ([]byte, error) {
	req := &proto.CreateKeyRequest{Name: name, Passphrase: passphrase, HdPath: hdPath}
	res, err := c.client.CreateKey(context.Background(), req)
	if err != nil {
		return nil, err
	}

	return res.Pk, nil
}

func (c *EOTSManagerGRpcClient) CreateRandomnessPairList(uid, chainID []byte, startHeight uint64, num uint32, passphrase string) ([]*btcec.FieldVal, error) {
	req := &proto.CreateRandomnessPairListRequest{
		Uid:         uid,
		ChainId:     chainID,
		StartHeight: startHeight,
		Num:         num,
		Passphrase:  passphrase,
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

func (c *EOTSManagerGRpcClient) KeyRecord(uid []byte, passphrase string) (*types.KeyRecord, error) {
	req := &proto.KeyRecordRequest{Uid: uid, Passphrase: passphrase}

	res, err := c.client.KeyRecord(context.Background(), req)
	if err != nil {
		return nil, err
	}

	privKey, _ := btcec.PrivKeyFromBytes(res.PrivateKey)

	return &types.KeyRecord{
		Name:    res.Name,
		PrivKey: privKey,
	}, nil
}

func (c *EOTSManagerGRpcClient) SignEOTS(uid, chaiID, msg []byte, height uint64, passphrase string) (*btcec.ModNScalar, error) {
	req := &proto.SignEOTSRequest{
		Uid:        uid,
		ChainId:    chaiID,
		Msg:        msg,
		Height:     height,
		Passphrase: passphrase,
	}
	res, err := c.client.SignEOTS(context.Background(), req)
	if err != nil {
		return nil, err
	}

	var s btcec.ModNScalar
	s.SetByteSlice(res.Sig)

	return &s, nil
}

func (c *EOTSManagerGRpcClient) UnsafeSignEOTS(uid, chaiID, msg []byte, height uint64, passphrase string) (*btcec.ModNScalar, error) {
	req := &proto.SignEOTSRequest{
		Uid:        uid,
		ChainId:    chaiID,
		Msg:        msg,
		Height:     height,
		Passphrase: passphrase,
	}
	res, err := c.client.UnsafeSignEOTS(context.Background(), req)
	if err != nil {
		return nil, err
	}

	var s btcec.ModNScalar
	s.SetByteSlice(res.Sig)

	return &s, nil
}

func (c *EOTSManagerGRpcClient) SignSchnorrSig(uid, msg []byte, passphrase string) (*schnorr.Signature, error) {
	req := &proto.SignSchnorrSigRequest{Uid: uid, Msg: msg, Passphrase: passphrase}
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

func (c *EOTSManagerGRpcClient) Close() error {
	return c.conn.Close()
}
