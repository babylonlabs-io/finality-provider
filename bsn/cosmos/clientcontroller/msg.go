package clientcontroller

import (
	"fmt"

	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
)

// CustomProof is a custom proof struct that ensures the index field is always included in JSON
// This fixes the issue where cmtcrypto.Proof omits the index field when it's 0 due to the omitempty tag
type CustomProof struct {
	Total    uint64   `json:"total"`
	Index    uint64   `json:"index"` // No omitempty tag to ensure it's always included
	LeafHash []byte   `json:"leaf_hash"`
	Aunts    [][]byte `json:"aunts"`
}

// ConvertProof converts cmtcrypto.Proof to CustomProof to ensure index field is always included
// This function is public so it can be used by tests and other packages
func ConvertProof(cmtProof cmtcrypto.Proof) (CustomProof, error) {
	// Validate that Total and Index are non-negative before converting to uint64
	if cmtProof.Total < 0 {
		return CustomProof{}, fmt.Errorf("cmtProof.Total cannot be negative: %d", cmtProof.Total)
	}
	if cmtProof.Index < 0 {
		return CustomProof{}, fmt.Errorf("cmtProof.Index cannot be negative: %d", cmtProof.Index)
	}

	return CustomProof{
		Total:    uint64(cmtProof.Total),
		Index:    uint64(cmtProof.Index),
		LeafHash: cmtProof.LeafHash,
		Aunts:    cmtProof.Aunts,
	}, nil
}

type ConsumerFpsResponse struct {
	Fps []SingleConsumerFpResponse `json:"fps"`
}

// SingleConsumerFpResponse represents the finality provider data returned by the contract query.
// For more details, refer to the following links:
// https://github.com/babylonchain/babylon-contract/blob/v0.5.3/packages/apis/src/btc_staking_api.rs
// https://github.com/babylonchain/babylon-contract/blob/v0.5.3/contracts/btc-staking/src/msg.rs
// https://github.com/babylonchain/babylon-contract/blob/v0.5.3/contracts/btc-staking/schema/btc-staking.json
type SingleConsumerFpResponse struct {
	BtcPkHex         string `json:"btc_pk_hex"`
	SlashedHeight    uint64 `json:"slashed_height"`
	SlashedBtcHeight uint32 `json:"slashed_btc_height"`
	ConsumerID       string `json:"consumer_id"`
}

type ConsumerDelegationsResponse struct {
	Delegations []SingleConsumerDelegationResponse `json:"delegations"`
}

type SingleConsumerDelegationResponse struct {
	BtcPkHex             string                      `json:"btc_pk_hex"`
	FpBtcPkList          []string                    `json:"fp_btc_pk_list"`
	StartHeight          uint32                      `json:"start_height"`
	EndHeight            uint32                      `json:"end_height"`
	TotalSat             uint64                      `json:"total_sat"`
	StakingTx            []byte                      `json:"staking_tx"`
	SlashingTx           []byte                      `json:"slashing_tx"`
	DelegatorSlashingSig []byte                      `json:"delegator_slashing_sig"`
	CovenantSigs         []CovenantAdaptorSignatures `json:"covenant_sigs"`
	StakingOutputIdx     uint32                      `json:"staking_output_idx"`
	UnbondingTime        uint32                      `json:"unbonding_time"`
	UndelegationInfo     *BtcUndelegationInfo        `json:"undelegation_info"`
	ParamsVersion        uint32                      `json:"params_version"`
}

type ConsumerFpInfoResponse struct {
	BtcPkHex        string `json:"btc_pk_hex"`
	Power           uint64 `json:"power"`
	Slashed         bool   `json:"slashed"`
	TotalActiveSats uint64 `json:"total_active_sats"`
}

type ConsumerFpsByPowerResponse struct {
	Fps []ConsumerFpInfoResponse `json:"fps"`
}

type FinalitySignatureResponse struct {
	Signature []byte `json:"signature"`
}

type BlocksResponse struct {
	Blocks []IndexedBlock `json:"blocks"`
}

type IndexedBlock struct {
	Height    uint64 `json:"height"`
	AppHash   []byte `json:"app_hash"`
	Finalized bool   `json:"finalized"`
}

type NewFinalityProvider struct {
	Description *FinalityProviderDescription `json:"description,omitempty"`
	Commission  string                       `json:"commission"`
	Addr        string                       `json:"addr"`
	BTCPKHex    string                       `json:"btc_pk_hex"`
	Pop         *ProofOfPossessionBtc        `json:"pop,omitempty"`
	ConsumerID  string                       `json:"consumer_id"`
}

type FinalityProviderDescription struct {
	Moniker         string `json:"moniker"`
	Identity        string `json:"identity"`
	Website         string `json:"website"`
	SecurityContact string `json:"security_contact"`
	Details         string `json:"details"`
}

type ProofOfPossessionBtc struct {
	BTCSigType int32  `json:"btc_sig_type"`
	BTCSig     []byte `json:"btc_sig"`
}

type CovenantAdaptorSignatures struct {
	CovPK       []byte   `json:"cov_pk"`
	AdaptorSigs [][]byte `json:"adaptor_sigs"`
}

type SignatureInfo struct {
	PK  []byte `json:"pk"`
	Sig []byte `json:"sig"`
}

type BtcUndelegationInfo struct {
	UnbondingTx           []byte                      `json:"unbonding_tx"`
	DelegatorUnbondingSig []byte                      `json:"delegator_unbonding_sig"`
	CovenantUnbondingSigs []SignatureInfo             `json:"covenant_unbonding_sig_list"`
	SlashingTx            []byte                      `json:"slashing_tx"`
	DelegatorSlashingSig  []byte                      `json:"delegator_slashing_sig"`
	CovenantSlashingSigs  []CovenantAdaptorSignatures `json:"covenant_slashing_sigs"`
}

type ActiveBtcDelegation struct {
	StakerAddr           string                      `json:"staker_addr"`
	BTCPkHex             string                      `json:"btc_pk_hex"`
	FpBtcPkList          []string                    `json:"fp_btc_pk_list"`
	StartHeight          uint32                      `json:"start_height"`
	EndHeight            uint32                      `json:"end_height"`
	TotalSat             uint64                      `json:"total_sat"`
	StakingTx            []byte                      `json:"staking_tx"`
	SlashingTx           []byte                      `json:"slashing_tx"`
	DelegatorSlashingSig []byte                      `json:"delegator_slashing_sig"`
	CovenantSigs         []CovenantAdaptorSignatures `json:"covenant_sigs"`
	StakingOutputIdx     uint32                      `json:"staking_output_idx"`
	UnbondingTime        uint32                      `json:"unbonding_time"`
	UndelegationInfo     BtcUndelegationInfo         `json:"undelegation_info"`
	ParamsVersion        uint32                      `json:"params_version"`
}

type SlashedBtcDelegation struct {
	// Define fields as needed
}

type UnbondedBtcDelegation struct {
	// Define fields as needed
}

type BtcStaking struct {
	NewFP       []NewFinalityProvider   `json:"new_fp"`
	ActiveDel   []ActiveBtcDelegation   `json:"active_del"`
	SlashedDel  []SlashedBtcDelegation  `json:"slashed_del"`
	UnbondedDel []UnbondedBtcDelegation `json:"unbonded_del"`
}

type CommitPublicRandomness struct {
	FPPubKeyHex string `json:"fp_pubkey_hex"`
	StartHeight uint64 `json:"start_height"`
	NumPubRand  uint64 `json:"num_pub_rand"`
	Commitment  []byte `json:"commitment"`
	Signature   []byte `json:"signature"`
}

type SubmitFinalitySignature struct {
	FpPubkeyHex string      `json:"fp_pubkey_hex"`
	Height      uint64      `json:"height"`
	PubRand     []byte      `json:"pub_rand"`
	Proof       CustomProof `json:"proof"` // Use custom proof struct to avoid omitempty issue
	BlockHash   []byte      `json:"block_hash"`
	Signature   []byte      `json:"signature"`
}

type ExecMsg struct {
	SubmitFinalitySignature *SubmitFinalitySignature `json:"submit_finality_signature,omitempty"`
	BtcStaking              *BtcStaking              `json:"btc_staking,omitempty"`
	CommitPublicRandomness  *CommitPublicRandomness  `json:"commit_public_randomness,omitempty"`
	Unjail                  *UnjailMsg               `json:"unjail,omitempty"`
}

type UnjailMsg struct {
	FPPubKeyHex string `json:"fp_pubkey_hex"`
}

type FinalityProviderInfo struct {
	BtcPkHex string `json:"btc_pk_hex"`
	Height   uint64 `json:"height"`
}

type QueryMsgFinalityProviderInfo struct {
	FinalityProviderInfo FinalityProviderInfo `json:"finality_provider_info"`
}

type BlockQuery struct {
	Height uint64 `json:"height"`
}

type QueryMsgBlock struct {
	Block BlockQuery `json:"block"`
}

type QueryMsgBlocks struct {
	Blocks BlocksQuery `json:"blocks"`
}

type BlocksQuery struct {
	StartAfter *uint64 `json:"start_after,omitempty"`
	Limit      *uint64 `json:"limit,omitempty"`
	Finalized  *bool   `json:"finalised,omitempty"` //TODO: finalised or finalized, typo in smart contract
	Reverse    *bool   `json:"reverse,omitempty"`
}

type QueryMsgActivatedHeight struct {
	ActivationHeight struct{} `json:"activation_height"`
}

type QueryMsgFinalitySignature struct {
	FinalitySignature FinalitySignatureQuery `json:"finality_signature"`
}

type FinalitySignatureQuery struct {
	BtcPkHex string `json:"btc_pk_hex"`
	Height   uint64 `json:"height"`
}

type QueryMsgFinalityProviders struct {
	FinalityProviders struct{} `json:"finality_providers"`
}

type QueryMsgFinalityProvider struct {
	FinalityProvider FinalityProvider `json:"finality_provider"`
}

type FinalityProvider struct {
	BtcPkHex string `json:"btc_pk_hex"`
}

type QueryMsgDelegations struct {
	Delegations struct{} `json:"delegations"`
}

type QueryMsgFinalityProvidersByTotalActiveSats struct {
	FinalityProvidersByTotalActiveSats struct{} `json:"finality_providers_by_total_active_sats"`
}

type QueryMsgLastPubRandCommit struct {
	LastPubRandCommit LastPubRandCommitQuery `json:"last_pub_rand_commit"`
}

type LastPubRandCommitQuery struct {
	BtcPkHex string `json:"btc_pk_hex"`
}

type QueryMsgPubRandCommits struct {
	PubRandCommit PubRandCommitQuery `json:"pub_rand_commit"`
}

type PubRandCommitQuery struct {
	BtcPkHex   string `json:"btc_pk_hex"`
	StartAfter uint64 `json:"start_after,omitempty"`
	Limit      uint32 `json:"limit,omitempty"`
	Reverse    bool   `json:"reverse,omitempty"`
}

type QueryMsgLastConsumerHeader struct {
	LastConsumerHeader struct{} `json:"last_consumer_header"`
}

type ConsumerHeaderResponse struct {
	ConsumerID          string `json:"consumer_id"`
	Hash                string `json:"hash"`
	Height              uint64 `json:"height"`
	Time                string `json:"time,omitempty"`
	BabylonHeaderHash   string `json:"babylon_header_hash"`
	BabylonHeaderHeight uint64 `json:"babylon_header_height"`
	BabylonEpoch        uint64 `json:"babylon_epoch"`
	BabylonTxHash       string `json:"babylon_tx_hash"`
}

type BabylonContracts struct {
	BabylonContract        string
	BtcLightClientContract string
	BtcStakingContract     string
	BtcFinalityContract    string
}

type QueryMsgFinalityProviderPower struct {
	FinalityProviderPower FinalityProviderPowerQuery `json:"finality_provider_power"`
}

type FinalityProviderPowerQuery struct {
	BtcPkHex string `json:"btc_pk_hex"`
	Height   uint64 `json:"height"`
}

type ConsumerFpPowerResponse struct {
	Power uint64 `json:"power"`
}

type QueryMsgFinalityConfig struct {
	FinalityConfig struct{} `json:"config"`
}

type FinalityConfigResponse struct {
	Denom                      string `json:"denom"`
	BabylonAddr                string `json:"babylon"`
	StakingAddr                string `json:"staking"`
	MaxActiveFinalityProviders uint32 `json:"max_active_finality_providers"`
	MinPubRand                 uint64 `json:"min_pub_rand"`
	RewardInterval             uint64 `json:"reward_interval"`
	MissedBlocksWindow         uint64 `json:"missed_blocks_window"`
	JailDuration               uint64 `json:"jail_duration"`
	FinalityActivationHeight   uint64 `json:"finality_activation_height"`
}

type QueryMsgSigningInfo struct {
	SigningInfo FinalityProvider `json:"signing_info"`
}

type SigningInfoResponse struct {
	FpBtcPkHex       string  `json:"fp_btc_pk_hex"`
	StartHeight      uint64  `json:"start_height"`
	LastSignedHeight uint64  `json:"last_signed_height"`
	JailedUntil      *uint64 `json:"jailed_until,omitempty"`
}
