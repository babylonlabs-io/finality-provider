package eotsmanager

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

type EOTSManager interface {
	// CreateRandomnessPairList generates a list of Schnorr randomness pairs from
	// startHeight to startHeight+(num-1) where num means the number of public randomness
	// It fails if the finality provider does not exist or a randomness pair has been created before
	// or passPhrase is incorrect
	// NOTE: the randomness is deterministically generated based on the EOTS key, chainID and
	// block height
	CreateRandomnessPairList(uid []byte, chainID []byte, startHeight uint64, num uint32) ([]*btcec.FieldVal, error)

	// SignEOTS signs an EOTS using the private key of the finality provider and the corresponding
	// secret randomness of the given chain at the given height
	// It fails if the finality provider does not exist or there's no randomness committed to the given height
	// or passPhrase is incorrect. Has built-in anti-slashing mechanism to ensure signature
	// for the same height will not be signed twice.
	SignEOTS(uid []byte, chainID []byte, msg []byte, height uint64) (*btcec.ModNScalar, error)

	// UnsafeSignEOTS should only be used in e2e tests for demonstration purposes.
	// Does not offer double sign protection.
	// Use SignEOTS for real operations.
	// uid represents EOTS key
	UnsafeSignEOTS(uid []byte, chainID []byte, msg []byte, height uint64) (*btcec.ModNScalar, error)

	// SignSchnorrSig signs a Schnorr signature using the private key of the finality provider
	// It fails if the finality provider does not exist or the message size is not 32 bytes
	// or passPhrase is incorrect.
	// uid represents EOTS key
	SignSchnorrSig(uid []byte, msg []byte) (*schnorr.Signature, error)

	// Unlock makes the private key for the given EOTS key (uid) accessible in memory using the provided passphrase.
	// After a successful call to Unlock, signing operations using this key will succeed.
	// This should be called during startup for `file`-based keyring, which requires explicit unlocking.
	Unlock(uid []byte, passphrase string) error

	Close() error
}
