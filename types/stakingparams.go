package types

import (
	sdkmath "cosmossdk.io/math"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
)

type StakingParams struct {
	// K-deep
	ComfirmationTimeBlocks uint32
	// W-deep
	FinalizationTimeoutBlocks uint32

	// Minimum amount of tx fee (quantified in Satoshi) needed for the pre-signed slashing tx
	MinSlashingTxFeeSat btcutil.Amount

	// Bitcoin public keys of the covenant committee
	CovenantPks []*btcec.PublicKey

	// The pk_script expected in slashing output i.e., the first
	// output of slashing transaction
	SlashingPkScript []byte

	// Minimum number of signatures needed for the covenant multisignature
	CovenantQuorum uint32

	// The staked amount to be slashed, expressed as a decimal (e.g., 0.5 for 50%).
	SlashingRate sdkmath.LegacyDec

	// The exact block time for unbonding transaction timelock in BTC blocks
	UnbondingTime uint32
}
