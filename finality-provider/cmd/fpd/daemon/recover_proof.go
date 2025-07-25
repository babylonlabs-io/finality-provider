package daemon

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"

	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/babylonlabs-io/finality-provider/util"
)

func CommandRecoverProof(binaryName string) *cobra.Command {
	cmd := CommandRecoverProofTemplate(binaryName)
	cmd.RunE = clientctx.RunEWithClientCtx(runCommandRecoverProof)

	return cmd
}

func CommandRecoverProofTemplate(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "recover-rand-proof [fp-eots-pk-hex]",
		Aliases: []string{"rrp"},
		Short:   "Recover the public randomness' merkle proof for a finality provider",
		Long:    "Recover the public randomness' merkle proof for a finality provider. Currently only Babylon consumer chain is supported.",
		Example: fmt.Sprintf(`%s recover-rand-proof --home /home/user/.fpd [fp-eots-pk-hex]`, binaryName),
		Args:    cobra.ExactArgs(1),
	}
	cmd.Flags().Uint64("start-height", 1, "The block height from which the proof is recovered from (optional)")
	cmd.Flags().String(flags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")
	cmd.Flags().String(flags.FlagChainID, "", "The consumer identifier")

	return cmd
}

func runCommandRecoverProof(ctx client.Context, cmd *cobra.Command, args []string) error {
	// Get homePath from context like in start.go
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return fmt.Errorf("failed to get home path: %w", err)
	}
	homePath = util.CleanAndExpandPath(homePath)

	cfg, err := fpcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	return RunCommandRecoverProofWithConfig(ctx, cmd, homePath, cfg, args)
}

func RunCommandRecoverProofWithConfig(_ client.Context, cmd *cobra.Command, homePath string, cfg *fpcfg.Config, args []string) error {
	chainID, err := cmd.Flags().GetString(flags.FlagChainID)
	if err != nil {
		return fmt.Errorf("failed to read chain id flag: %w", err)
	}

	if chainID == "" {
		return fmt.Errorf("please specify chain-id")
	}

	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return fmt.Errorf("failed to parse EOTS public key: %w", err)
	}
	startHeight, err := cmd.Flags().GetUint64("start-height")
	if err != nil {
		return fmt.Errorf("failed to get start height flag: %w", err)
	}

	logger, err := log.NewRootLoggerWithFile(fpcfg.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	db, err := cfg.DatabaseConfig.GetDBBackend()
	defer func() {
		err := db.Close()
		if err != nil {
			panic(fmt.Errorf("failed to close db: %w", err))
		}
	}()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	pubRandStore, err := store.NewPubRandProofStore(db)
	if err != nil {
		return fmt.Errorf("failed to initiate public randomness store: %w", err)
	}

	em, err := eotsclient.NewEOTSManagerGRpcClient(cfg.EOTSManagerAddress, cfg.HMACKey)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager client: %w", err)
	}

	bcc, err := babylon.NewBabylonConsumerController(cfg.BabylonConfig, logger)
	if err != nil {
		return fmt.Errorf("failed to create Babylon rpc client: %w", err)
	}

	commitList, err := bcc.QueryPublicRandCommitList(fpPk.MustToBTCPK(), startHeight)
	if err != nil {
		return fmt.Errorf("failed to query randomness commit list for fp %s: %w", fpPk.MarshalHex(), err)
	}

	for _, commit := range commitList {
		// to bypass gosec check of overflow risk
		if commit.NumPubRand > uint64(math.MaxUint32) {
			return fmt.Errorf("NumPubRand %d exceeds maximum uint32 value", commit.NumPubRand)
		}

		pubRandList, err := em.CreateRandomnessPairList(fpPk.MustMarshal(), []byte(chainID), commit.StartHeight, uint32(commit.NumPubRand))
		if err != nil {
			return fmt.Errorf("failed to get randomness from height %d to height %d: %w", commit.StartHeight, commit.EndHeight(), err)
		}
		// generate commitment and proof for each public randomness
		commitRoot, proofList := types.GetPubRandCommitAndProofs(pubRandList)
		if !bytes.Equal(commitRoot, commit.Commitment) {
			return fmt.Errorf("the commit root on Babylon does not match the local one, expected: %x, got: %x", commit.Commitment, commitRoot)
		}

		// store them to database
		if err := pubRandStore.AddPubRandProofList([]byte(chainID), fpPk.MustMarshal(), commit.StartHeight, commit.NumPubRand, proofList); err != nil {
			return fmt.Errorf("failed to save public randomness to DB: %w", err)
		}
	}

	return nil
}
