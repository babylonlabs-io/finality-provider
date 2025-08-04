package clientcontroller

type CommitPublicRandomnessMsg struct {
	CommitPublicRandomness CommitPublicRandomnessMsgParams `json:"commit_public_randomness"`
}

type CommitPublicRandomnessMsgParams struct {
	FpPubkeyHex string `json:"fp_pubkey_hex"`
	StartHeight uint64 `json:"start_height"`
	NumPubRand  uint64 `json:"num_pub_rand"`
	Commitment  []byte `json:"commitment"`
	Signature   []byte `json:"signature"`
}

// TODO: need to update based on contract implementation
type CommitPublicRandomnessResponse struct {
	Result bool `json:"result"`
}

type SubmitFinalitySignatureMsg struct {
	SubmitFinalitySignature SubmitFinalitySignatureMsgParams `json:"submit_finality_signature"`
}

type SubmitFinalitySignatureMsgParams struct {
	FpPubkeyHex string `json:"fp_pubkey_hex"`
	Height      uint64 `json:"height"`
	PubRand     []byte `json:"pub_rand"`
	Proof       Proof  `json:"proof"`
	BlockHash   []byte `json:"block_hash"`
	Signature   []byte `json:"signature"`
}

// TODO: need to update based on contract implementation
type SubmitFinalitySignatureResponse struct {
	Result bool `json:"result"`
}

type QueryMsg struct {
	Config                 *ContractConfig              `json:"config,omitempty"`
	FirstPubRandCommit     *PubRandCommit               `json:"first_pub_rand_commit,omitempty"`
	LastPubRandCommit      *PubRandCommit               `json:"last_pub_rand_commit,omitempty"`
	PubRandCommitForHeight *PubRandCommitForHeightQuery `json:"pub_rand_commit_for_height,omitempty"`
	HighestVotedHeight     *HighestVotedHeightQuery     `json:"highest_voted_height,omitempty"`
}

// ContractConfig represents the full configuration from the finality contract
type ContractConfig struct {
	BsnID                     string             `json:"bsn_id"`
	MinPubRand                uint64             `json:"min_pub_rand"`
	RateLimiting              RateLimitingConfig `json:"rate_limiting"`
	BsnActivationHeight       uint64             `json:"bsn_activation_height"`
	FinalitySignatureInterval uint64             `json:"finality_signature_interval"`
}

type RateLimitingConfig struct {
	MaxMsgsPerInterval uint32 `json:"max_msgs_per_interval"`
	BlockInterval      uint64 `json:"block_interval"`
}

type PubRandCommit struct {
	BtcPkHex string `json:"btc_pk_hex"`
}

type PubRandCommitForHeightQuery struct {
	BtcPkHex string `json:"btc_pk_hex"`
	Height   uint64 `json:"height"`
}

type HighestVotedHeightQuery struct {
	BtcPkHex string `json:"btc_pk_hex"`
}

type PubRandCommitResponse struct {
	StartHeight  uint64 `json:"start_height"`
	NumPubRand   uint64 `json:"num_pub_rand"`
	Commitment   []byte `json:"commitment"`
	BabylonEpoch uint64 `json:"babylon_epoch"`
}

// FIXME: Remove this ancillary struct.
// Only required because the e2e tests are using a zero index, which is removed by the `json:"omitempty"` annotation in
// the original cmtcrypto Proof
type Proof struct {
	Total    uint64   `json:"total"`
	Index    uint64   `json:"index"`
	LeafHash []byte   `json:"leaf_hash"`
	Aunts    [][]byte `json:"aunts"`
}
