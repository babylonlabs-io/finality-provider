package eotsmanager

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/babylonlabs-io/finality-provider/metrics"

	"github.com/babylonlabs-io/babylon/v3/crypto/eots"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
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
	mu          sync.Mutex
	kr          keyring.Keyring
	es          *store.EOTSStore
	logger      *zap.Logger
	input       *strings.Reader // to send passphrase to the keyring
	privateKeys map[string]*btcec.PrivateKey
	metrics     *metrics.EotsMetrics
}

func NewLocalEOTSManager(homeDir, keyringBackend string, dbbackend kvdb.Backend, logger *zap.Logger) (*LocalEOTSManager, error) {
	es, err := store.NewEOTSStore(dbbackend)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}

	inputReader := strings.NewReader("")

	kr, err := InitKeyring(homeDir, keyringBackend, inputReader)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize keyring: %w", err)
	}

	eotsMetrics := metrics.NewEotsMetrics()

	return &LocalEOTSManager{
		kr:          kr,
		es:          es,
		logger:      logger,
		metrics:     eotsMetrics,
		input:       inputReader,
		privateKeys: make(map[string]*btcec.PrivateKey), // key name -> private key
	}, nil
}

func InitKeyring(homeDir, keyringBackend string, input *strings.Reader) (keyring.Keyring, error) {
	kr, err := keyring.New(
		"eots-manager",
		keyringBackend,
		homeDir,
		input,
		codec.MakeCodec(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	return kr, nil
}

// CreateKey creates a new EOTS key with a random mnemonic and returns the public key in bytes.
// passphrase is used to unlock the keyring if it is file based.
func (lm *LocalEOTSManager) CreateKey(name, passphrase string) ([]byte, error) {
	mnemonic, err := NewMnemonic()
	if err != nil {
		return nil, err
	}

	eotsPk, err := lm.CreateKeyWithMnemonic(name, mnemonic, passphrase)
	if err != nil {
		return nil, err
	}

	return eotsPk.MustMarshal(), nil
}

func NewMnemonic() (string, error) {
	// read entropy seed straight from tmcrypto.Rand and convert to mnemonic
	entropySeed, err := bip39.NewEntropy(MnemonicEntropySize)
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropySeed)
	if err != nil {
		return "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	return mnemonic, nil
}

func (lm *LocalEOTSManager) CreateKeyWithMnemonic(name, mnemonic, passphrase string) (*bbntypes.BIP340PubKey, error) {
	if lm.kr.Backend() == keyring.BackendFile && len(passphrase) < 8 {
		return nil, fmt.Errorf("passphrase should be at least 8 characters")
	}

	if lm.keyExists(name) {
		return nil, eotstypes.ErrFinalityProviderAlreadyExisted
	}

	keyringAlgos, _ := lm.kr.SupportedAlgorithms()
	algo, err := keyring.NewSigningAlgoFromString(secp256k1Type, keyringAlgos)
	if err != nil {
		return nil, fmt.Errorf("failed to create signing algorithm: %w", err)
	}

	// when the first key is created for the `file` keyring backend, it will prompt for a passphrase twice
	// https://github.com/cosmos/cosmos-sdk/blob/v0.50.12/crypto/keyring/keyring.go#L735
	lm.input.Reset(passphrase + "\n" + passphrase)
	_, err = lm.kr.NewAccount(name, mnemonic, "", "", algo)
	if err != nil {
		return nil, fmt.Errorf("failed to create new account: %w", err)
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
	if err := lm.es.AddEOTSKeyName(pk, keyName); err != nil {
		return fmt.Errorf("failed to save EOTS key name: %w", err)
	}

	return nil
}

func (lm *LocalEOTSManager) LoadBIP340PubKeyFromKeyName(keyName string) (*bbntypes.BIP340PubKey, error) {
	pk, err := LoadBIP340PubKeyFromKeyName(lm.kr, keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to load BIP340 public key from key name %s: %w", keyName, err)
	}

	return pk, nil
}

func LoadBIP340PubKeyFromKeyName(kr keyring.Keyring, keyName string) (*bbntypes.BIP340PubKey, error) {
	info, err := kr.Key(keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to load keyring record for key %s: %w", keyName, err)
	}
	pubKey, err := info.GetPubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key from keyring info: %w", err)
	}

	var eotsPk *bbntypes.BIP340PubKey
	switch v := pubKey.(type) {
	case *secp256k1.PubKey:
		pk, err := btcec.ParsePubKey(v.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key: %w", err)
		}
		eotsPk = bbntypes.NewBIP340PubKeyFromBTCPK(pk)

		return eotsPk, nil
	default:
		return nil, fmt.Errorf("unsupported key type in keyring")
	}
}

func (lm *LocalEOTSManager) CreateRandomnessPairList(fpPk []byte, chainID []byte, startHeight uint64, num uint32) ([]*btcec.FieldVal, error) {
	prList := make([]*btcec.FieldVal, 0, num)

	for i := uint32(0); i < num; i++ {
		height := startHeight + uint64(i)
		_, pubRand, err := lm.getRandomnessPair(fpPk, chainID, height)
		if err != nil {
			return nil, err
		}

		prList = append(prList, pubRand)
	}
	lm.metrics.IncrementEotsFpTotalGeneratedRandomnessCounter(hex.EncodeToString(fpPk))
	lm.metrics.SetEotsFpLastGeneratedRandomnessHeight(hex.EncodeToString(fpPk), float64(startHeight))

	return prList, nil
}

func (lm *LocalEOTSManager) SignEOTS(eotsPk []byte, chainID []byte, msg []byte, height uint64) (*btcec.ModNScalar, error) {
	record, found, err := lm.es.GetSignRecord(eotsPk, chainID, height)
	if err != nil {
		return nil, fmt.Errorf("error getting sign record: %w", err)
	}

	if found {
		if bytes.Equal(msg, record.Msg) {
			var s btcec.ModNScalar
			s.SetByteSlice(record.Signature)

			lm.logger.Info(
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

	privRand, _, err := lm.getRandomnessPair(eotsPk, chainID, height)
	if err != nil {
		return nil, fmt.Errorf("failed to get private randomness: %w", err)
	}

	privKey, err := lm.getEOTSPrivKey(eotsPk)
	if err != nil {
		return nil, fmt.Errorf("failed to get EOTS private key: %w", err)
	}

	// Update metrics
	lm.metrics.IncrementEotsFpTotalEotsSignCounter(hex.EncodeToString(eotsPk))
	lm.metrics.SetEotsFpLastEotsSignHeight(hex.EncodeToString(eotsPk), float64(height))

	signedBytes, err := eots.Sign(privKey, privRand, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign eots: %w", err)
	}

	b := signedBytes.Bytes()
	if err := lm.es.SaveSignRecord(height, chainID, msg, eotsPk, b[:]); err != nil {
		return nil, fmt.Errorf("failed to save signing record: %w", err)
	}

	return signedBytes, nil
}

// UnsafeSignEOTS should only be used in e2e test to demonstrate double sign
func (lm *LocalEOTSManager) UnsafeSignEOTS(fpPk []byte, chainID []byte, msg []byte, height uint64) (*btcec.ModNScalar, error) {
	privRand, _, err := lm.getRandomnessPair(fpPk, chainID, height)
	if err != nil {
		return nil, fmt.Errorf("failed to get private randomness: %w", err)
	}

	privKey, err := lm.getEOTSPrivKey(fpPk)
	if err != nil {
		return nil, fmt.Errorf("failed to get EOTS private key: %w", err)
	}

	// Update metrics
	lm.metrics.IncrementEotsFpTotalEotsSignCounter(hex.EncodeToString(fpPk))
	lm.metrics.SetEotsFpLastEotsSignHeight(hex.EncodeToString(fpPk), float64(height))

	signedBytes, err := eots.Sign(privKey, privRand, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign eots: %w", err)
	}

	return signedBytes, nil
}

func (lm *LocalEOTSManager) SignSchnorrSig(fpPk []byte, msg []byte) (*schnorr.Signature, error) {
	privKey, err := lm.getEOTSPrivKey(fpPk)
	if err != nil {
		return nil, fmt.Errorf("failed to get EOTS private key: %w", err)
	}

	return lm.signSchnorrSigFromPrivKey(privKey, fpPk, msg)
}

// signSchnorrSigFromPrivKey signs a Schnorr signature using the private key and updates metrics by the fpPk
func (lm *LocalEOTSManager) signSchnorrSigFromPrivKey(privKey *btcec.PrivateKey, fpPk []byte, msg []byte) (*schnorr.Signature, error) {
	// Update metrics
	lm.metrics.IncrementEotsFpTotalSchnorrSignCounter(hex.EncodeToString(fpPk))

	sig, err := schnorr.Sign(privKey, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign schnorr signature: %w", err)
	}

	return sig, nil
}

func (lm *LocalEOTSManager) SignSchnorrSigFromKeyname(keyName string, msg []byte) (*schnorr.Signature, *bbntypes.BIP340PubKey, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

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
	if err := lm.es.Close(); err != nil {
		return fmt.Errorf("failed to close EOTS store: %w", err)
	}

	return nil
}

// getRandomnessPair returns a randomness pair generated based on the given finality provider key, chainID and height
func (lm *LocalEOTSManager) getRandomnessPair(fpPk []byte, chainID []byte, height uint64) (*eots.PrivateRand, *eots.PublicRand, error) {
	record, err := lm.KeyRecord(fpPk)
	if err != nil {
		return nil, nil, err
	}
	privRand, pubRand := randgenerator.GenerateRandomness(record.PrivKey.Serialize(), chainID, height)

	return privRand, pubRand, nil
}

func (lm *LocalEOTSManager) KeyRecord(fpPk []byte) (*eotstypes.KeyRecord, error) {
	name, err := lm.es.GetEOTSKeyName(fpPk)
	if err != nil {
		return nil, fmt.Errorf("failed to get EOTS key name: %w", err)
	}
	privKey, err := lm.getEOTSPrivKey(fpPk)
	if err != nil {
		return nil, err
	}

	return &eotstypes.KeyRecord{
		Name:    name,
		PrivKey: privKey,
	}, nil
}

func (lm *LocalEOTSManager) getEOTSPrivKey(fpPk []byte) (*btcec.PrivateKey, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	keyName, err := lm.es.GetEOTSKeyName(fpPk)
	if err != nil {
		return nil, fmt.Errorf("failed to get EOTS key name: %w", err)
	}

	return lm.eotsPrivKeyFromKeyName(keyName)
}

func (lm *LocalEOTSManager) eotsPrivKeyFromKeyName(keyName string) (*btcec.PrivateKey, error) {
	var (
		privKey *btcec.PrivateKey
		err     error
	)

	switch lm.kr.Backend() {
	case keyring.BackendTest:
		privKey, err = lm.getKeyFromKeyring(keyName, "")
		if err != nil {
			return nil, err
		}
	case keyring.BackendFile:
		privKey, err = lm.getKeyFromMap(keyName)
		if err != nil {
			return nil, err
		}
	}

	return privKey, nil
}

func (lm *LocalEOTSManager) Unlock(fpPk []byte, passphrase string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	keyName, err := lm.es.GetEOTSKeyName(fpPk)
	if err != nil {
		return fmt.Errorf("failed to get EOTS key name: %w", err)
	}

	privKey, err := lm.getKeyFromKeyring(keyName, passphrase)
	if err != nil {
		return fmt.Errorf("failed to unlock the key ring: %w", err)
	}

	if _, ok := lm.privateKeys[keyName]; ok {
		return fmt.Errorf("private key already unlocked for key name: %s, fpPk: %s", keyName, hex.EncodeToString(fpPk))
	}

	lm.privateKeys[keyName] = privKey

	return nil
}

func (lm *LocalEOTSManager) getKeyFromMap(keyName string) (*btcec.PrivateKey, error) {
	// we don't call the lock here because we are already in the lock in caller function
	privKey, ok := lm.privateKeys[keyName]
	if !ok {
		return nil, fmt.Errorf("private key not found in map for key name: %s", keyName)
	}

	if privKey == nil {
		return nil, fmt.Errorf("private key is nil for key name: %s", keyName)
	}

	return privKey, nil
}

func (lm *LocalEOTSManager) getKeyFromKeyring(keyName, passphrase string) (*btcec.PrivateKey, error) {
	lm.input.Reset(passphrase)

	k, err := lm.kr.Key(keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to get key from keyring: %w", err)
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
	keys, err := lm.es.GetAllEOTSKeyNames()
	if err != nil {
		return nil, fmt.Errorf("failed to get all EOTS key names: %w", err)
	}

	return keys, nil
}

// UnsafeDeleteSignStoreRecords removes all sign store records from the given height
func (lm *LocalEOTSManager) UnsafeDeleteSignStoreRecords(eotsPK []byte, chainID []byte, fromHeight uint64) error {
	if err := lm.es.DeleteSignRecordsFromHeight(eotsPK, chainID, fromHeight); err != nil {
		return fmt.Errorf("failed to delete sign records from height %d: %w", fromHeight, err)
	}

	return nil
}

func (lm *LocalEOTSManager) IsRecordInDB(eotsPk []byte, chainID []byte, height uint64) (bool, error) {
	_, found, err := lm.es.GetSignRecord(eotsPk, chainID, height)
	if err != nil {
		return false, fmt.Errorf("error getting sign record: %w", err)
	}

	return found, nil
}

func (lm *LocalEOTSManager) Backup(dbPath string, backupDir string) (string, error) {
	backupPath, err := lm.es.BackupDB(dbPath, backupDir)
	if err != nil {
		return "", fmt.Errorf("failed to backup database: %w", err)
	}

	return backupPath, nil
}
