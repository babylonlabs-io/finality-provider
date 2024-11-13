package store

import "errors"

var (
	// ErrCorruptedFinalityProviderDB For some reason, db on disk representation have changed
	ErrCorruptedFinalityProviderDB = errors.New("finality provider db is corrupted")

	// ErrFinalityProviderNotFound The finality provider we try update is not found in db
	ErrFinalityProviderNotFound = errors.New("finality provider not found")

	// ErrDuplicateFinalityProvider, The finality provider we try to add already exists in db
	ErrDuplicateFinalityProvider = errors.New("finality provider already exists")

	// ErrCorruptedPubRandProofDB For some reason, db on disk representation have changed
	ErrCorruptedPubRandProofDB = errors.New("public randomness proof db is corrupted")

	// ErrPubRandProofNotFound The finality provider we try update is not found in db
	ErrPubRandProofNotFound = errors.New("public randomness proof not found")
)
