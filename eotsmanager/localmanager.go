package eotsmanager

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/babylonlabs-io/finality-provider/metrics"

	"github.com/babylonlabs-io/babylon/crypto/eots"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/go-bip39"
	"github.com/lightningnetwork/lnd/kvdb"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/finality-provider/codec"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/randgenerator"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/store"
	eotstypes "github.com/babylonlabs-io/finality-provider/eotsmanager/types"
)

const (
	secp256k1Type       = "secp256k1"
	MnemonicEntropySize = 256
)

var _ EOTSManager = &LocalEOTSManager{}

type LocalEOTSManager struct {
	mu     sync.Mutex
	kr     keyring.Keyring
	es     *store.EOTSStore
	logger *zap.Logger
	// input is to send passphrase to kr
	input   *strings.Reader
	metrics *metrics.EotsMetrics
}

func NewLocalEOTSManager(homeDir, keyringBackend string, dbbackend kvdb.Backend, logger *zap.Logger) (*LocalEOTSManager, error) {
	inputReader := strings.NewReader("")

	es, err := store.NewEOTSStore(dbbackend)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}

	kr, err := InitKeyring(homeDir, keyringBackend, inputReader)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize keyring: %w", err)
	}

	eotsMetrics := metrics.NewEotsMetrics()

	return &LocalEOTSManager{
		kr:      kr,
		es:      es,
		logger:  logger,
		input:   inputReader,
		metrics: eotsMetrics,
	}, nil
}

func InitKeyring(homeDir, keyringBackend string, inputReader *strings.Reader) (keyring.Keyring, error) {
	return keyring.New(
		"eots-manager",
		keyringBackend,
		homeDir,
		inputReader,
		codec.MakeCodec(),
	)
}

func (lm *LocalEOTSManager) CreateKey(name, passphrase, hdPath string) ([]byte, error) {
	mnemonic, err := NewMnemonic()
	if err != nil {
		return nil, err
	}

	eotsPk, err := lm.CreateKeyWithMnemonic(name, passphrase, hdPath, mnemonic)
	if err != nil {
		return nil, err
	}

	return eotsPk.MustMarshal(), nil
}

func NewMnemonic() (string, error) {
	// read entropy seed straight from tmcrypto.Rand and convert to mnemonic
	entropySeed, err := bip39.NewEntropy(MnemonicEntropySize)
	if err != nil {
		return "", err
	}

	mnemonic, err := bip39.NewMnemonic(entropySeed)
	if err != nil {
		return "", err
	}

	return mnemonic, nil
}

func (lm *LocalEOTSManager) CreateKeyWithMnemonic(name, passphrase, hdPath, mnemonic string) (*bbntypes.BIP340PubKey, error) {
	if lm.keyExists(name) {
		return nil, eotstypes.ErrFinalityProviderAlreadyExisted
	}

	keyringAlgos, _ := lm.kr.SupportedAlgorithms()
	algo, err := keyring.NewSigningAlgoFromString(secp256k1Type, keyringAlgos)
	if err != nil {
		return nil, err
	}

	// we need to repeat the passphrase to mock the re-entry
	// as when creating an account, passphrase will be asked twice
	// by the keyring
	lm.input.Reset(passphrase + "\n" + passphrase)
	_, err = lm.kr.NewAccount(name, mnemonic, passphrase, hdPath, algo)
	if err != nil {
		return nil, err
	}

	eotsPk, err := lm.LoadBIP340PubKeyFromKeyName(name)
	if err != nil {
		return nil, err
	}

	if err := lm.SaveEOTSKeyName(eotsPk.MustToBTCPK(), name); err != nil {
		return nil, err
	}

	lm.logger.Info(
		"successfully created an EOTS key",
		zap.String("key name", name),
		zap.String("pk", eotsPk.MarshalHex()),
	)
	lm.metrics.IncrementEotsCreatedKeysCounter()

	return eotsPk, nil
}

func (lm *LocalEOTSManager) SaveEOTSKeyName(pk *btcec.PublicKey, keyName string) error {
	return lm.es.AddEOTSKeyName(pk, keyName)
}

func (lm *LocalEOTSManager) LoadBIP340PubKeyFromKeyName(keyName string) (*bbntypes.BIP340PubKey, error) {
	return LoadBIP340PubKeyFromKeyName(lm.kr, keyName)
}

func LoadBIP340PubKeyFromKeyName(kr keyring.Keyring, keyName string) (*bbntypes.BIP340PubKey, error) {
	info, err := kr.Key(keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to load keyring record for key %s: %w", keyName, err)
	}
	pubKey, err := info.GetPubKey()
	if err != nil {
		return nil, err
	}

	var eotsPk *bbntypes.BIP340PubKey
	switch v := pubKey.(type) {
	case *secp256k1.PubKey:
		pk, err := btcec.ParsePubKey(v.Key)
		if err != nil {
			return nil, err
		}
		eotsPk = bbntypes.NewBIP340PubKeyFromBTCPK(pk)

		return eotsPk, nil
	default:
		return nil, fmt.Errorf("unsupported key type in keyring")
	}
}

func (lm *LocalEOTSManager) CreateRandomnessPairList(fpPk []byte, chainID []byte, startHeight uint64, num uint32, passphrase string) ([]*btcec.FieldVal, error) {
	prList := make([]*btcec.FieldVal, 0, num)

	for i := uint32(0); i < num; i++ {
		height := startHeight + uint64(i)
		_, pubRand, err := lm.getRandomnessPair(fpPk, chainID, height, passphrase)
		if err != nil {
			return nil, err
		}

		prList = append(prList, pubRand)
	}
	lm.metrics.IncrementEotsFpTotalGeneratedRandomnessCounter(hex.EncodeToString(fpPk))
	lm.metrics.SetEotsFpLastGeneratedRandomnessHeight(hex.EncodeToString(fpPk), float64(startHeight))

	return prList, nil
}

func (lm *LocalEOTSManager) SignEOTS(eotsPk []byte, chainID []byte, msg []byte, height uint64, passphrase string) (*btcec.ModNScalar, error) {
	record, found, err := lm.es.GetSignRecord(eotsPk, chainID, height)
	if err != nil {
		return nil, fmt.Errorf("error getting sign record: %w", err)
	}

	if found {
		if bytes.Equal(msg, record.Msg) {
			var s btcec.ModNScalar
			s.SetByteSlice(record.Signature)

			lm.logger.Warn(
				"duplicate sign requested",
				zap.String("eots_pk", hex.EncodeToString(eotsPk)),
				zap.String("hash", hex.EncodeToString(msg)),
				zap.Uint64("height", height),
				zap.String("chainID", string(chainID)),
			)

			return &s, nil
		}

		lm.logger.Error(
			"double sign requested",
			zap.String("eots_pk", hex.EncodeToString(eotsPk)),
			zap.String("hash", hex.EncodeToString(msg)),
			zap.Uint64("height", height),
			zap.String("chainID", string(chainID)),
		)

		return nil, eotstypes.ErrDoubleSign
	}

	privRand, _, err := lm.getRandomnessPair(eotsPk, chainID, height, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to get private randomness: %w", err)
	}

	privKey, err := lm.getEOTSPrivKey(eotsPk, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to get EOTS private key: %w", err)
	}

	// Update metrics
	lm.metrics.IncrementEotsFpTotalEotsSignCounter(hex.EncodeToString(eotsPk))
	lm.metrics.SetEotsFpLastEotsSignHeight(hex.EncodeToString(eotsPk), float64(height))

	signedBytes, err := eots.Sign(privKey, privRand, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign eots")
	}

	b := signedBytes.Bytes()
	if err := lm.es.SaveSignRecord(height, chainID, msg, eotsPk, b[:]); err != nil {
		return nil, fmt.Errorf("failed to save signing record: %w", err)
	}

	return signedBytes, nil
}

// UnsafeSignEOTS should only be used in e2e test to demonstrate double sign
func (lm *LocalEOTSManager) UnsafeSignEOTS(fpPk []byte, chainID []byte, msg []byte, height uint64, passphrase string) (*btcec.ModNScalar, error) {
	privRand, _, err := lm.getRandomnessPair(fpPk, chainID, height, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to get private randomness: %w", err)
	}

	privKey, err := lm.getEOTSPrivKey(fpPk, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to get EOTS private key: %w", err)
	}

	// Update metrics
	lm.metrics.IncrementEotsFpTotalEotsSignCounter(hex.EncodeToString(fpPk))
	lm.metrics.SetEotsFpLastEotsSignHeight(hex.EncodeToString(fpPk), float64(height))

	return eots.Sign(privKey, privRand, msg)
}

func (lm *LocalEOTSManager) SignSchnorrSig(fpPk []byte, msg []byte, passphrase string) (*schnorr.Signature, error) {
	privKey, err := lm.getEOTSPrivKey(fpPk, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to get EOTS private key: %w", err)
	}

	return lm.signSchnorrSigFromPrivKey(privKey, fpPk, msg)
}

// signSchnorrSigFromPrivKey signs a Schnorr signature using the private key and updates metrics by the fpPk
func (lm *LocalEOTSManager) signSchnorrSigFromPrivKey(privKey *btcec.PrivateKey, fpPk []byte, msg []byte) (*schnorr.Signature, error) {
	// Update metrics
	lm.metrics.IncrementEotsFpTotalSchnorrSignCounter(hex.EncodeToString(fpPk))

	return schnorr.Sign(privKey, msg)
}

func (lm *LocalEOTSManager) SignSchnorrSigFromKeyname(keyName, passphrase string, msg []byte) (*schnorr.Signature, *bbntypes.BIP340PubKey, error) {
	lm.input.Reset(passphrase)

	eotsPk, err := lm.LoadBIP340PubKeyFromKeyName(keyName)
	if err != nil {
		return nil, nil, err
	}

	privKey, err := lm.eotsPrivKeyFromKeyName(keyName)
	if err != nil {
		return nil, nil, err
	}

	signature, err := lm.signSchnorrSigFromPrivKey(privKey, *eotsPk, msg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to schnorr sign: %w", err)
	}

	return signature, eotsPk, nil
}

func (lm *LocalEOTSManager) Close() error {
	return lm.es.Close()
}

// getRandomnessPair returns a randomness pair generated based on the given finality provider key, chainID and height
func (lm *LocalEOTSManager) getRandomnessPair(fpPk []byte, chainID []byte, height uint64, passphrase string) (*eots.PrivateRand, *eots.PublicRand, error) {
	record, err := lm.KeyRecord(fpPk, passphrase)
	if err != nil {
		return nil, nil, err
	}
	privRand, pubRand := randgenerator.GenerateRandomness(record.PrivKey.Serialize(), chainID, height)

	return privRand, pubRand, nil
}

func (lm *LocalEOTSManager) KeyRecord(fpPk []byte, passphrase string) (*eotstypes.KeyRecord, error) {
	name, err := lm.es.GetEOTSKeyName(fpPk)
	if err != nil {
		return nil, err
	}
	privKey, err := lm.getEOTSPrivKey(fpPk, passphrase)
	if err != nil {
		return nil, err
	}

	return &eotstypes.KeyRecord{
		Name:    name,
		PrivKey: privKey,
	}, nil
}

func (lm *LocalEOTSManager) getEOTSPrivKey(fpPk []byte, passphrase string) (*btcec.PrivateKey, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	keyName, err := lm.es.GetEOTSKeyName(fpPk)
	if err != nil {
		return nil, err
	}

	lm.input.Reset(passphrase)

	return lm.eotsPrivKeyFromKeyName(keyName)
}

func (lm *LocalEOTSManager) eotsPrivKeyFromKeyName(keyName string) (*btcec.PrivateKey, error) {
	k, err := lm.kr.Key(keyName)
	if err != nil {
		return nil, err
	}
	privKeyCached := k.GetLocal().PrivKey.GetCachedValue()

	var privKey *btcec.PrivateKey
	switch v := privKeyCached.(type) {
	case *secp256k1.PrivKey:
		privKey, _ = btcec.PrivKeyFromBytes(v.Key)

		return privKey, nil
	default:
		return nil, fmt.Errorf("unsupported key type in keyring")
	}
}

func (lm *LocalEOTSManager) keyExists(name string) bool {
	_, err := lm.kr.Key(name)

	return err == nil
}

func (lm *LocalEOTSManager) ListEOTSKeys() (map[string][]byte, error) {
	return lm.es.GetAllEOTSKeyNames()
}
