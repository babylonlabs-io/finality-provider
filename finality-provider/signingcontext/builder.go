package signingcontext

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	stakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	finalitytypes "github.com/babylonlabs-io/babylon/x/finality/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

const (
	protocolName = "btcstaking"
	versionV0    = "0"
	fpPop        = "fp_pop"
	fpRandCommit = "fp_rand_commit"
	fpFinVote    = "fp_fin_vote"
)

var (
	AccFinality   = authtypes.NewModuleAddress(finalitytypes.ModuleName)
	AccBTCStaking = authtypes.NewModuleAddress(stakingtypes.ModuleName)
)

func btcStakingV0Context(operationTag string, chainID string, address string) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", protocolName, versionV0, operationTag, chainID, address)
}

// HashedHexContext returns the hex encoded sha256 hash of the context string i.e
// hex(sha256(contextString))
func HashedHexContext(contextString string) string {
	bytes := sha256.Sum256([]byte(contextString))

	return hex.EncodeToString(bytes[:])
}

// FpPopContextV0 returns context string in format:
// btcstaking/0/fp_pop/{chainID}/{address}
func FpPopContextV0(chainID string, address string) string {
	return HashedHexContext(btcStakingV0Context(fpPop, chainID, address))
}

// FpRandCommitContextV0 returns context string in format:
// btcstaking/0/fp_rand_commit/{chainID}/{address}
func FpRandCommitContextV0(chainID string, address string) string {
	return HashedHexContext(btcStakingV0Context(fpRandCommit, chainID, address))
}

// FpFinVoteContextV0 returns context string in format:
// btcstaking/0/fp_fin_vote/{chainID}/{address}
func FpFinVoteContextV0(chainID string, address string) string {
	return HashedHexContext(btcStakingV0Context(fpFinVote, chainID, address))
}
