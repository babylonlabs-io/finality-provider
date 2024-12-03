package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/proto"
	pm "google.golang.org/protobuf/proto"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb"
)

var (
	eotsBucketName       = []byte("fpKeyNames")
	signRecordBucketName = []byte("signRecord")
)

type EOTSStore struct {
	db kvdb.Backend
}

func NewEOTSStore(db kvdb.Backend) (*EOTSStore, error) {
	s := &EOTSStore{db}
	if err := s.initBuckets(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *EOTSStore) initBuckets() error {
	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		_, err := tx.CreateTopLevelBucket(eotsBucketName)
		if err != nil {
			return err
		}

		_, err = tx.CreateTopLevelBucket(signRecordBucketName)
		if err != nil {
			return err
		}

		return nil
	})
}

func (s *EOTSStore) AddEOTSKeyName(
	btcPk *btcec.PublicKey,
	keyName string,
) error {
	pkBytes := schnorr.SerializePubKey(btcPk)

	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		eotsBucket := tx.ReadWriteBucket(eotsBucketName)
		if eotsBucket == nil {
			return ErrCorruptedEOTSDb
		}

		// check btc pk first to avoid duplicates
		if eotsBucket.Get(pkBytes) != nil {
			return ErrDuplicateEOTSKeyName
		}

		return saveEOTSKeyName(eotsBucket, pkBytes, keyName)
	})
}

func saveEOTSKeyName(
	eotsBucket walletdb.ReadWriteBucket,
	btcPk []byte,
	keyName string,
) error {
	if keyName == "" {
		return fmt.Errorf("cannot save empty key name")
	}

	return eotsBucket.Put(btcPk, []byte(keyName))
}

func (s *EOTSStore) GetEOTSKeyName(pk []byte) (string, error) {
	var keyName string
	err := s.db.View(func(tx kvdb.RTx) error {
		eotsBucket := tx.ReadBucket(eotsBucketName)
		if eotsBucket == nil {
			return ErrCorruptedEOTSDb
		}

		keyNameBytes := eotsBucket.Get(pk)
		if keyNameBytes == nil {
			return ErrEOTSKeyNameNotFound
		}

		keyName = string(keyNameBytes)
		return nil
	}, func() {})

	if err != nil {
		return "", err
	}

	return keyName, nil
}

func (s *EOTSStore) SaveSignRecord(
	height uint64,
	blockHash []byte,
	publicKey []byte,
	signature []byte,
) error {
	key := uint64ToBytes(height)

	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		bucket := tx.ReadWriteBucket(signRecordBucketName)
		if bucket == nil {
			return ErrCorruptedEOTSDb
		}

		if bucket.Get(key) != nil {
			return ErrDuplicateSignRecord
		}

		signRecord := &proto.SigningRecord{
			BlockHash: blockHash,
			PublicKey: publicKey,
			Signature: signature,
			Timestamp: time.Now().UnixMilli(),
		}

		marshalled, err := pm.Marshal(signRecord)
		if err != nil {
			return err
		}

		return bucket.Put(key, marshalled)
	})
}

func (s *EOTSStore) GetSignRecord(height uint64) (*proto.SigningRecord, bool, error) {
	key := uint64ToBytes(height)
	res := &proto.SigningRecord{}

	err := s.db.View(func(tx kvdb.RTx) error {
		bucket := tx.ReadBucket(signRecordBucketName)
		if bucket == nil {
			return ErrCorruptedEOTSDb
		}

		signRecordBytes := bucket.Get(key)
		if signRecordBytes == nil {
			return ErrSignRecordNotFound
		}

		return pm.Unmarshal(signRecordBytes, res)
	}, func() {})

	if err != nil {
		if errors.Is(err, ErrSignRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}

	return res, true, nil
}

// Converts an uint64 value to a byte slice.
func uint64ToBytes(v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	return buf[:]
}
