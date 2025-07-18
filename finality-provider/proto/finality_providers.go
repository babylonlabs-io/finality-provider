package proto

import (
	"errors"
	"fmt"
	"time"

	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	"cosmossdk.io/math"
	bbn "github.com/babylonlabs-io/babylon/v3/types"
	btcstktypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
)

func (sfp *FinalityProvider) MustGetBTCPK() *btcec.PublicKey {
	btcPubKey, err := schnorr.ParsePubKey(sfp.BtcPk)
	if err != nil {
		panic(fmt.Errorf("failed to parse BTC PK: %w", err))
	}

	return btcPubKey
}

func (sfp *FinalityProvider) MustGetBIP340BTCPK() *bbn.BIP340PubKey {
	btcPK := sfp.MustGetBTCPK()

	return bbn.NewBIP340PubKeyFromBTCPK(btcPK)
}

func NewFinalityProviderInfo(sfp *FinalityProvider) (*FinalityProviderInfo, error) {
	var des types.Description
	if err := des.Unmarshal(sfp.Description); err != nil {
		return nil, fmt.Errorf("failed to unmarshal description: %w", err)
	}

	return &FinalityProviderInfo{
		FpAddr:   sfp.FpAddr,
		BtcPkHex: sfp.MustGetBIP340BTCPK().MarshalHex(),
		Description: &Description{
			Moniker:         des.Moniker,
			Identity:        des.Identity,
			Website:         des.Website,
			SecurityContact: des.SecurityContact,
			Details:         des.Details,
		},
		LastVotedHeight: sfp.LastVotedHeight,
		Status:          sfp.Status.String(),
	}, nil
}

func NewCommissionInfoWithTime(maxRate, maxChangeRate math.LegacyDec, updatedAt time.Time) *CommissionInfo {
	return &CommissionInfo{
		MaxRate:       maxRate.String(),
		MaxChangeRate: maxChangeRate.String(),
		UpdateTime:    timestamppb.New(updatedAt),
	}
}

func NewCommissionRates(rate, maxRate, maxChangeRate math.LegacyDec) *CommissionRates {
	return &CommissionRates{
		Rate:          rate.String(),
		MaxRate:       maxRate.String(),
		MaxChangeRate: maxChangeRate.String(),
	}
}

func (req *CreateFinalityProviderRequest) GetCommissionRates() (btcstktypes.CommissionRates, error) {
	rates := btcstktypes.CommissionRates{}
	if req.Commission == nil {
		return rates, errors.New("nil Commission in request. Cannot get CommissionRates")
	}

	rate, err := math.LegacyNewDecFromStr(req.Commission.Rate)
	if err != nil {
		return rates, fmt.Errorf("invalid commission rate: %w", err)
	}
	maxRate, err := math.LegacyNewDecFromStr(req.Commission.MaxRate)
	if err != nil {
		return rates, fmt.Errorf("invalid commission max rate: %w", err)
	}
	maxChangeRate, err := math.LegacyNewDecFromStr(req.Commission.MaxChangeRate)
	if err != nil {
		return rates, fmt.Errorf("invalid commission max change rate: %w", err)
	}

	return btcstktypes.NewCommissionRates(rate, maxRate, maxChangeRate), nil
}
