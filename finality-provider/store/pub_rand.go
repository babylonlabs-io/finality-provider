package store

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcwallet/walletdb"
	sdk "github.com/cosmos/cosmos-sdk/types"

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

// getKey key is (chainID || pk || height)
func getKey(chainID, pk []byte, height uint64) []byte {
	// Convert height to bytes
	heightBytes := sdk.Uint64ToBigEndian(height)

	// Concatenate all components to create the key
	key := make([]byte, 0, len(pk)+len(chainID)+len(heightBytes))
	key = append(key, chainID...)
	key = append(key, pk...)
	key = append(key, heightBytes...)

	return key
}

func getPrefixKey(chainID, pk []byte) []byte {
	// Concatenate chainID and pk to form the prefix
	prefix := make([]byte, 0, len(chainID)+len(pk))
	prefix = append(prefix, chainID...)
	prefix = append(prefix, pk...)

	return prefix
}

func buildKeys(chainID, pk []byte, height uint64, num uint64) [][]byte {
	keys := make([][]byte, 0, num)

	for i := uint64(0); i < num; i++ {
		key := getKey(chainID, pk, height+i)
		keys = append(keys, key)
	}

	return keys
}

func (s *PubRandProofStore) AddPubRandProofList(
	chainID []byte,
	pk []byte,
	height uint64,
	numPubRand uint64,
	proofList []*merkle.Proof,
) error {
	keys := buildKeys(chainID, pk, height, numPubRand)

	if len(keys) != len(proofList) {
		return fmt.Errorf("the number of public randomness is not same as the number of proofs")
	}

	var proofBytesList [][]byte
	for _, proof := range proofList {
		proofBytes, err := proof.ToProto().Marshal()
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

		for i, key := range keys {
			// skip if already committed
			if bucket.Get(key) != nil {
				continue
			}
			// set to DB
			if err := bucket.Put(key, proofBytesList[i]); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *PubRandProofStore) GetPubRandProof(chainID []byte, pk []byte, height uint64) ([]byte, error) {
	key := getKey(chainID, pk, height)
	var proofBytes []byte

	err := s.db.View(func(tx kvdb.RTx) error {
		bucket := tx.ReadBucket(pubRandProofBucketName)
		if bucket == nil {
			return ErrCorruptedPubRandProofDB
		}

		proofBytes = bucket.Get(key)
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

func (s *PubRandProofStore) GetPubRandProofList(chainID []byte,
	pk []byte,
	height uint64,
	numPubRand uint64,
) ([][]byte, error) {
	keys := buildKeys(chainID, pk, height, numPubRand)

	var proofBytesList [][]byte

	err := s.db.View(func(tx kvdb.RTx) error {
		bucket := tx.ReadBucket(pubRandProofBucketName)
		if bucket == nil {
			return ErrCorruptedPubRandProofDB
		}

		for _, key := range keys {
			proofBytes := bucket.Get(key)
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

// RemovePubRandProofList removes all proofs up to the target height
func (s *PubRandProofStore) RemovePubRandProofList(chainID []byte, pk []byte, targetHeight uint64) error {
	prefix := getPrefixKey(chainID, pk)

	err := s.db.Update(func(tx walletdb.ReadWriteTx) error {
		bucket := tx.ReadWriteBucket(pubRandProofBucketName)
		if bucket == nil {
			return walletdb.ErrBucketNotFound
		}

		cursor := bucket.ReadWriteCursor()

		for k, _ := cursor.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = cursor.Next() {
			heightBytes := k[len(k)-8:]
			height := sdk.BigEndianToUint64(heightBytes)

			// no need to keep iterating, keys are sorted in lexicographical order upon insert
			if height > targetHeight {
				break
			}

			if err := cursor.Delete(); err != nil {
				return err
			}
		}

		return nil
	}, func() {})

	if err != nil {
		return err
	}

	return nil
}
