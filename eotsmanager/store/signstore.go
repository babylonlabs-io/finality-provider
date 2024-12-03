package store

import (
	"encoding/binary"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/proto"
	"github.com/lightningnetwork/lnd/kvdb"
	pm "google.golang.org/protobuf/proto"
	"time"
)

var (
	signRecordBucketName = []byte("signRecord")
)

type SignStore struct {
	db kvdb.Backend
}

func NewSignStore(db kvdb.Backend) (*SignStore, error) {
	s := &SignStore{db}
	if err := s.initBuckets(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *SignStore) initBuckets() error {
	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		_, err := tx.CreateTopLevelBucket(signRecordBucketName)
		if err != nil {
			return err
		}

		return nil
	})
}

func (s *SignStore) SaveSignRecord(
	height uint64,
	BlockHash []byte,
	PublicKey []byte,
	Signature []byte,
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
			BlockHash: BlockHash,
			PublicKey: PublicKey,
			Signature: Signature,
			Timestamp: time.Now().UnixMilli(),
		}

		marshalled, err := pm.Marshal(signRecord)
		if err != nil {
			return err
		}

		return bucket.Put(key, marshalled)
	})
}

func (s *SignStore) GetSignRecord(height uint64) (*proto.SigningRecord, error) {
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
		return nil, err
	}

	return res, nil
}

// Converts an uint64 value to a byte slice.
func uint64ToBytes(v uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	return buf[:]
}
