package store

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcwallet/walletdb"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/lightningnetwork/lnd/kvdb"
	pm "google.golang.org/protobuf/proto"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
)

var (
	// mapping pk -> proto.FinalityProvider
	finalityProviderBucketName = []byte("finalityProviders")
)

type FinalityProviderStore struct {
	db kvdb.Backend
}

// NewFinalityProviderStore returns a new store backed by db
func NewFinalityProviderStore(db kvdb.Backend) (*FinalityProviderStore, error) {
	store := &FinalityProviderStore{db}
	if err := store.initBuckets(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *FinalityProviderStore) initBuckets() error {
	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		_, err := tx.CreateTopLevelBucket(finalityProviderBucketName)

		return err
	})
}

func (s *FinalityProviderStore) CreateFinalityProvider(
	fpAddr sdk.AccAddress,
	btcPk *btcec.PublicKey,
	description *stakingtypes.Description,
	commission *sdkmath.LegacyDec,
	chainID string,
) error {
	desBytes, err := description.Marshal()
	if err != nil {
		return fmt.Errorf("invalid description: %w", err)
	}
	fp := &proto.FinalityProvider{
		FpAddr:      fpAddr.String(),
		BtcPk:       schnorr.SerializePubKey(btcPk),
		Description: desBytes,
		Commission:  commission.String(),
		ChainId:     chainID,
		Status:      proto.FinalityProviderStatus_REGISTERED,
	}

	return s.createFinalityProviderInternal(fp)
}

func (s *FinalityProviderStore) createFinalityProviderInternal(
	fp *proto.FinalityProvider,
) error {
	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		fpBucket := tx.ReadWriteBucket(finalityProviderBucketName)
		if fpBucket == nil {
			return ErrCorruptedFinalityProviderDB
		}

		// check btc pk first to avoid duplicates
		if fpBucket.Get(fp.BtcPk) != nil {
			return ErrDuplicateFinalityProvider
		}

		return saveFinalityProvider(fpBucket, fp)
	})
}

func saveFinalityProvider(
	fpBucket walletdb.ReadWriteBucket,
	fp *proto.FinalityProvider,
) error {
	if fp == nil {
		return fmt.Errorf("cannot save nil finality provider")
	}

	marshalled, err := pm.Marshal(fp)
	if err != nil {
		return err
	}

	return fpBucket.Put(fp.BtcPk, marshalled)
}

func (s *FinalityProviderStore) SetFpStatus(btcPk *btcec.PublicKey, status proto.FinalityProviderStatus) error {
	setFpStatus := func(fp *proto.FinalityProvider) error {
		fp.Status = status

		return nil
	}

	return s.setFinalityProviderState(btcPk, setFpStatus)
}

func (s *FinalityProviderStore) MustSetFpStatus(btcPk *btcec.PublicKey, status proto.FinalityProviderStatus) {
	if err := s.SetFpStatus(btcPk, status); err != nil {
		panic(err)
	}
}

// UpdateFpStatusFromVotingPower based on the current voting power of the finality provider
// updates the status, if it has some voting power, sets to active
func (s *FinalityProviderStore) UpdateFpStatusFromVotingPower(
	vp uint64,
	fp *StoredFinalityProvider,
) (proto.FinalityProviderStatus, error) {
	if fp.Status == proto.FinalityProviderStatus_SLASHED {
		// Slashed FP should not update status
		return proto.FinalityProviderStatus_SLASHED, nil
	}

	if vp > 0 {
		// voting power > 0 then set the status to ACTIVE
		return proto.FinalityProviderStatus_ACTIVE, s.SetFpStatus(fp.BtcPk, proto.FinalityProviderStatus_ACTIVE)
	}

	if fp.Status == proto.FinalityProviderStatus_ACTIVE {
		// previous status is ACTIVE then set to INACTIVE
		return proto.FinalityProviderStatus_INACTIVE, s.SetFpStatus(fp.BtcPk, proto.FinalityProviderStatus_INACTIVE)
	}

	return fp.Status, nil
}

// SetFpLastVotedHeight sets the last voted height to the stored last voted height and last processed height
// only if it is larger than the stored one. This is to ensure the stored state to increase monotonically
func (s *FinalityProviderStore) SetFpLastVotedHeight(btcPk *btcec.PublicKey, lastVotedHeight uint64) error {
	setFpLastVotedHeight := func(fp *proto.FinalityProvider) error {
		if fp.LastVotedHeight < lastVotedHeight {
			fp.LastVotedHeight = lastVotedHeight
		}

		return nil
	}

	return s.setFinalityProviderState(btcPk, setFpLastVotedHeight)
}

func (s *FinalityProviderStore) setFinalityProviderState(
	btcPk *btcec.PublicKey,
	stateTransitionFn func(provider *proto.FinalityProvider) error,
) error {
	pkBytes := schnorr.SerializePubKey(btcPk)

	return kvdb.Batch(s.db, func(tx kvdb.RwTx) error {
		fpBucket := tx.ReadWriteBucket(finalityProviderBucketName)
		if fpBucket == nil {
			return ErrCorruptedFinalityProviderDB
		}

		fpFromDB := fpBucket.Get(pkBytes)
		if fpFromDB == nil {
			return ErrFinalityProviderNotFound
		}

		var storedFp proto.FinalityProvider
		if err := pm.Unmarshal(fpFromDB, &storedFp); err != nil {
			return ErrCorruptedFinalityProviderDB
		}

		if err := stateTransitionFn(&storedFp); err != nil {
			return err
		}

		return saveFinalityProvider(fpBucket, &storedFp)
	})
}

func (s *FinalityProviderStore) GetFinalityProvider(btcPk *btcec.PublicKey) (*StoredFinalityProvider, error) {
	var storedFp *StoredFinalityProvider
	pkBytes := schnorr.SerializePubKey(btcPk)

	err := s.db.View(func(tx kvdb.RTx) error {
		fpBucket := tx.ReadBucket(finalityProviderBucketName)
		if fpBucket == nil {
			return ErrCorruptedFinalityProviderDB
		}

		fpBytes := fpBucket.Get(pkBytes)
		if fpBytes == nil {
			return ErrFinalityProviderNotFound
		}

		var fpProto proto.FinalityProvider
		if err := pm.Unmarshal(fpBytes, &fpProto); err != nil {
			return ErrCorruptedFinalityProviderDB
		}

		fpFromDB, err := protoFpToStoredFinalityProvider(&fpProto)
		if err != nil {
			return err
		}

		storedFp = fpFromDB

		return nil
	}, func() {})

	if err != nil {
		return nil, err
	}

	return storedFp, nil
}

// GetAllStoredFinalityProviders fetches all the stored finality providers from db
// pagination is probably not needed as the expected number of finality providers
// in the store is small
func (s *FinalityProviderStore) GetAllStoredFinalityProviders() ([]*StoredFinalityProvider, error) {
	var storedFps []*StoredFinalityProvider

	err := s.db.View(func(tx kvdb.RTx) error {
		fpBucket := tx.ReadBucket(finalityProviderBucketName)
		if fpBucket == nil {
			return ErrCorruptedFinalityProviderDB
		}

		return fpBucket.ForEach(func(_, v []byte) error {
			var fpProto proto.FinalityProvider
			if err := pm.Unmarshal(v, &fpProto); err != nil {
				return ErrCorruptedFinalityProviderDB
			}

			fpFromDB, err := protoFpToStoredFinalityProvider(&fpProto)
			if err != nil {
				return err
			}
			storedFps = append(storedFps, fpFromDB)

			return nil
		})
	}, func() {})

	if err != nil {
		return nil, err
	}

	return storedFps, nil
}

// SetFpDescription updates description of finality provider
func (s *FinalityProviderStore) SetFpDescription(btcPk *btcec.PublicKey, desc *stakingtypes.Description, rate *sdkmath.LegacyDec) error {
	setDescription := func(fp *proto.FinalityProvider) error {
		descBytes, err := desc.Marshal()
		if err != nil {
			return err
		}

		fp.Description = descBytes
		fp.Commission = rate.String()

		return nil
	}

	return s.setFinalityProviderState(btcPk, setDescription)
}
