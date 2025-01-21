package e2e_utils

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"encoding/hex"

	"github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/stretchr/testify/require"
)

var (
	EventuallyWaitTimeOut = 3 * time.Minute
	EventuallyPollTime    = 500 * time.Millisecond
	FpNamePrefix          = "test-fp-"
	MonikerPrefix         = "moniker-"
	ChainID               = "chain-test"
	Passphrase            = "testpass"
	HdPath                = ""
	WasmStake             = "ustake"  // Default staking token
	WasmFee               = "ucosm"   // Default fee token
	WasmMoniker           = "node001" // Default moniker
	BtcNetworkParams      = &chaincfg.SimNetParams
	StakingTime           = uint16(100)
	StakingAmount         = int64(500000)
)

func NewDescription(moniker string) *stakingtypes.Description {
	dec := stakingtypes.NewDescription(moniker, "", "", "", "")
	return &dec
}

func BaseDir(pattern string) (string, error) {
	tempPath := os.TempDir()

	tempName, err := os.MkdirTemp(tempPath, pattern)
	if err != nil {
		return "", err
	}

	if err = os.Chmod(tempName, 0755); err != nil {
		return "", err
	}

	return tempName, nil
}

func RunCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error running command: %w", err)
	}
	return output, nil
}

func GenerateCovenantCommittee(
	numCovenants int,
	t *testing.T,
) ([]*btcec.PrivateKey, []*bbntypes.BIP340PubKey) {
	var (
		covenantPrivKeys []*btcec.PrivateKey
		covenantPubKeys  []*bbntypes.BIP340PubKey
	)

	for i := 0; i < numCovenants; i++ {
		privKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		covenantPrivKeys = append(covenantPrivKeys, privKey)
		pubKey := bbntypes.NewBIP340PubKeyFromBTCPK(privKey.PubKey())
		covenantPubKeys = append(covenantPubKeys, pubKey)
	}

	return covenantPrivKeys, covenantPubKeys
}

func WaitForFpPubRandCommittedReachTargetHeight(t *testing.T, fpIns *service.FinalityProviderInstance, targetHeight uint64) {
	var lastCommittedHeight uint64
	var err error
	require.Eventually(t, func() bool {
		lastCommittedHeight, err = fpIns.GetLastCommittedHeight()
		if err != nil {
			return false
		}
		return lastCommittedHeight >= targetHeight
	}, EventuallyWaitTimeOut, EventuallyPollTime)
	t.Logf("Public randomness for fp %s is successfully committed at height %d", fpIns.GetBtcPkHex(), lastCommittedHeight)
}

func WaitForFpPubRandCommitted(t *testing.T, fpIns *service.FinalityProviderInstance) {
	WaitForFpPubRandCommittedReachTargetHeight(t, fpIns, 1)
}

func DefaultFpConfig(keyringDir, homeDir string) *config.Config {
	cfg := config.DefaultConfigWithHome(homeDir)

	cfg.NumPubRand = 1000
	cfg.NumPubRandMax = 1000
	cfg.TimestampingDelayBlocks = 0

	cfg.BitcoinNetwork = "simnet"
	cfg.BTCNetParams = chaincfg.SimNetParams

	cfg.PollerConfig.PollInterval = 1 * time.Millisecond
	cfg.PollerConfig.AutoChainScanningMode = false

	// babylon configs for sending transactions
	cfg.BabylonConfig.KeyDirectory = keyringDir
	// need to use this one to send otherwise we will have account sequence mismatch
	// errors
	cfg.BabylonConfig.Key = "test-spending-key"
	// Big adjustment to make sure we have enough gas in our transactions
	cfg.BabylonConfig.GasAdjustment = 20

	return &cfg
}

func DefaultFpConfigWithPorts(keyringDir, homeDir string, fpRpcPort, fpMetricsPort, eotsRpcPort int) *config.Config {
	cfg := DefaultFpConfig(keyringDir, homeDir)
	cfg.RPCListener = fmt.Sprintf("127.0.0.1:%d", fpRpcPort)
	cfg.EOTSManagerAddress = fmt.Sprintf("127.0.0.1:%d", eotsRpcPort)
	cfg.Metrics.Port = fpMetricsPort
	return cfg
}

// ParseRespBTCDelToBTCDel parses an BTC delegation response to BTC Delegation
// adapted from
// https://github.com/babylonlabs-io/babylon/blob/1a3c50da64885452c8d669fcea2a2fad78c8a028/test/e2e/btc_staking_e2e_test.go#L548
func ParseRespBTCDelToBTCDel(resp *bstypes.BTCDelegationResponse) (btcDel *bstypes.BTCDelegation, err error) {
	stakingTx, err := hex.DecodeString(resp.StakingTxHex)
	if err != nil {
		return nil, err
	}

	delSig, err := bbntypes.NewBIP340SignatureFromHex(resp.DelegatorSlashSigHex)
	if err != nil {
		return nil, err
	}

	slashingTx, err := bstypes.NewBTCSlashingTxFromHex(resp.SlashingTxHex)
	if err != nil {
		return nil, err
	}

	btcDel = &bstypes.BTCDelegation{
		// missing BabylonPk, Pop
		// these fields are not sent out to the client on BTCDelegationResponse
		BtcPk:            resp.BtcPk,
		FpBtcPkList:      resp.FpBtcPkList,
		StartHeight:      resp.StartHeight,
		EndHeight:        resp.EndHeight,
		TotalSat:         resp.TotalSat,
		StakingTx:        stakingTx,
		DelegatorSig:     delSig,
		StakingOutputIdx: resp.StakingOutputIdx,
		CovenantSigs:     resp.CovenantSigs,
		UnbondingTime:    resp.UnbondingTime,
		SlashingTx:       slashingTx,
	}

	if resp.UndelegationResponse != nil {
		ud := resp.UndelegationResponse
		unbondTx, err := hex.DecodeString(ud.UnbondingTxHex)
		if err != nil {
			return nil, err
		}

		slashTx, err := bstypes.NewBTCSlashingTxFromHex(ud.SlashingTxHex)
		if err != nil {
			return nil, err
		}

		delSlashingSig, err := bbntypes.NewBIP340SignatureFromHex(ud.DelegatorSlashingSigHex)
		if err != nil {
			return nil, err
		}

		btcDel.BtcUndelegation = &bstypes.BTCUndelegation{
			UnbondingTx:              unbondTx,
			CovenantUnbondingSigList: ud.CovenantUnbondingSigList,
			CovenantSlashingSigs:     ud.CovenantSlashingSigs,
			SlashingTx:               slashTx,
			DelegatorSlashingSig:     delSlashingSig,
		}
	}

	return btcDel, nil
}
