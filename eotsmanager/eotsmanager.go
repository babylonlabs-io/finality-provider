package eotsmanager

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"

	"github.com/babylonlabs-io/finality-provider/eotsmanager/types"
)

type EOTSManager interface {
	// CreateKey generates a key pair at the given name and persists it in storage.
	// The key pair is formatted by BIP-340 (Schnorr Signatures)
	// It fails if there is an existing key Info with the same name or public key.
	CreateKey(name, passphrase, hdPath string) ([]byte, error)

	// CreateRandomnessPairList generates a list of Schnorr randomness pairs from
	// startHeight to startHeight+(num-1) where num means the number of public randomness
	// It fails if the finality provider does not exist or a randomness pair has been created before
	// or passPhrase is incorrect
	// NOTE: the randomness is deterministically generated based on the EOTS key, chainID and
	// block height
	CreateRandomnessPairList(uid []byte, chainID []byte, startHeight uint64, num uint32, passphrase string) ([]*btcec.FieldVal, error)

	// KeyRecord returns the finality provider record
	// It fails if the finality provider does not exist or passPhrase is incorrect
	KeyRecord(uid []byte, passphrase string) (*types.KeyRecord, error)

	// SignEOTS signs an EOTS using the private key of the finality provider and the corresponding
	// secret randomness of the given chain at the given height
	// It fails if the finality provider does not exist or there's no randomness committed to the given height
	// or passPhrase is incorrect. Has built-in anti-slashing mechanism to ensure signature
	// for the same height will not be signed twice.
	SignEOTS(uid []byte, chainID []byte, msg []byte, height uint64, passphrase string) (*btcec.ModNScalar, error)

	// UnsafeSignEOTS should only be used in e2e tests for demonstration purposes.
	// Does not offer double sign protection.
	// Use SignEOTS for real operations.
	UnsafeSignEOTS(uid []byte, chainID []byte, msg []byte, height uint64, passphrase string) (*btcec.ModNScalar, error)

	// SignSchnorrSig signs a Schnorr signature using the private key of the finality provider
	// It fails if the finality provider does not exist or the message size is not 32 bytes
	// or passPhrase is incorrect
	SignSchnorrSig(uid []byte, msg []byte, passphrase string) (*schnorr.Signature, error)

	// SaveEOTSKeyName saves a new key under the EOTS key name mapping
	SaveEOTSKeyName(pk *btcec.PublicKey, keyName string) error

	Close() error
}
