package proto

import (
	"fmt"

	bbn "github.com/babylonlabs-io/babylon/types"
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
		return nil, err
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
