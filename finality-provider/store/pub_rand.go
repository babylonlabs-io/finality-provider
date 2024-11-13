package store

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cometbft/cometbft/crypto/merkle"
	"github.com/lightningnetwork/lnd/kvdb"
)

var (
	// mapping: pub_rand -> proof
	pubRandProofBucketName = []byte("pub_rand_proof")
)

type PubRandProofStore struct {
	db kvdb.Backend
}

// NewPubRandProofStore returns a new store backed by db
func NewPubRandProofStore(db kvdb.Backend) (*PubRandProofStore, error) {
	store := &PubRandProofStore{db}
	if err := store.initBuckets(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *PubRandProofStore) initBuckets() error {
	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		_, err := tx.CreateTopLevelBucket(pubRandProofBucketName)
		return err
	})
}

func (s *PubRandProofStore) AddPubRandProofList(
	pubRandList []*btcec.FieldVal,
	proofList []*merkle.Proof,
) error {
	if len(pubRandList) != len(proofList) {
		return fmt.Errorf("the number of public randomness is not same as the number of proofs")
	}

	pubRandBytesList := [][]byte{}
	proofBytesList := [][]byte{}
	for i := range pubRandList {
		pubRandBytes := *pubRandList[i].Bytes()
		pubRandBytesList = append(pubRandBytesList, pubRandBytes[:])
		proofBytes, err := proofList[i].ToProto().Marshal()
		if err != nil {
			return fmt.Errorf("invalid proof: %w", err)
		}
		proofBytesList = append(proofBytesList, proofBytes)
	}

	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		bucket := tx.ReadWriteBucket(pubRandProofBucketName)
		if bucket == nil {
			return ErrCorruptedPubRandProofDB
		}

		for i := range pubRandBytesList {
			// skip if already committed
			if bucket.Get(pubRandBytesList[i]) != nil {
				continue
			}
			// set to DB
			if err := bucket.Put(pubRandBytesList[i], proofBytesList[i]); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *PubRandProofStore) GetPubRandProof(pubRand *btcec.FieldVal) ([]byte, error) {
	pubRandBytes := *pubRand.Bytes()
	var proofBytes []byte

	err := s.db.View(func(tx kvdb.RTx) error {
		bucket := tx.ReadBucket(pubRandProofBucketName)
		if bucket == nil {
			return ErrCorruptedPubRandProofDB
		}

		proofBytes = bucket.Get(pubRandBytes[:])
		if proofBytes == nil {
			return ErrPubRandProofNotFound
		}

		return nil
	}, func() {})

	if err != nil {
		return nil, err
	}

	return proofBytes, nil
}

func (s *PubRandProofStore) GetPubRandProofList(pubRandList []*btcec.FieldVal) ([][]byte, error) {
	pubRandBytesList := [][]byte{}
	for i := range pubRandList {
		pubRandBytes := *pubRandList[i].Bytes()
		pubRandBytesList = append(pubRandBytesList, pubRandBytes[:])
	}

	proofBytesList := [][]byte{}

	err := s.db.View(func(tx kvdb.RTx) error {
		bucket := tx.ReadBucket(pubRandProofBucketName)
		if bucket == nil {
			return ErrCorruptedPubRandProofDB
		}

		for i := range pubRandBytesList {
			proofBytes := bucket.Get(pubRandBytesList[i])
			if proofBytes == nil {
				return ErrPubRandProofNotFound
			}
			proofBytesList = append(proofBytesList, proofBytes)
		}

		return nil
	}, func() {})

	if err != nil {
		return nil, err
	}

	return proofBytesList, nil
}

// TODO: delete function?
