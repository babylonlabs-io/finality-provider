package daemon

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/finality-provider/clientcontroller/babylon"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/babylonlabs-io/finality-provider/util"
)

func CommandRecoverProof() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "recover-rand-proof [fp-eots-pk-hex]",
		Aliases: []string{"rrp"},
		Short:   "Recover the public randomness' merkle proof for a finality provider",
		Long:    "Recover the public randomness' merkle proof for a finality provider. Currently only Babylon consumer chain is supported.",
		Example: `fpd recover-rand-proof --home /home/user/.fpd [fp-eots-pk-hex]`,
		Args:    cobra.ExactArgs(1),
		RunE:    fpcmd.RunEWithClientCtx(runCommandRecoverProof),
	}
	cmd.Flags().Uint64("start-height", math.MaxUint64, "The block height from which the proof is recovered from (optional)")

	return cmd
}

func runCommandRecoverProof(ctx client.Context, cmd *cobra.Command, args []string) error {
	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return err
	}
	startHeight, err := cmd.Flags().GetUint64("start-height")
	if err != nil {
		return err
	}

	// Get homePath from context like in start.go
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return err
	}
	homePath = util.CleanAndExpandPath(homePath)

	cfg, err := fpcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger, err := log.NewRootLoggerWithFile(fpcfg.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	db, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	fpStore, err := store.NewFinalityProviderStore(db)
	if err != nil {
		return fmt.Errorf("failed to initiate finality provider store: %w", err)
	}

	storedFp, err := fpStore.GetFinalityProvider(fpPk.MustToBTCPK())
	if err != nil {
		return fmt.Errorf("failed to load fp %s from db: %w", fpPk.MarshalHex(), err)
	}

	pubRandStore, err := store.NewPubRandProofStore(db)
	if err != nil {
		return fmt.Errorf("failed to initiate public randomness store: %w", err)
	}

	em, err := eotsclient.NewEOTSManagerGRpcClient(cfg.EOTSManagerAddress)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager client: %w", err)
	}

	bcc, err := babylon.NewBabylonConsumerController(cfg.BabylonConfig, &cfg.BTCNetParams, logger)
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

		pubRandList, err := em.CreateRandomnessPairList(fpPk.MustMarshal(), []byte(storedFp.ChainID), commit.StartHeight, uint32(commit.NumPubRand))
		if err != nil {
			return fmt.Errorf("failed to get randomness from height %d to height %d: %w", commit.StartHeight, commit.EndHeight(), err)
		}
		// generate commitment and proof for each public randomness
		commitRoot, proofList := types.GetPubRandCommitAndProofs(pubRandList)
		if !bytes.Equal(commitRoot, commit.Commitment) {
			return fmt.Errorf("the commit root on Babylon does not match the local one, expected: %x, got: %x", commit.Commitment, commitRoot)
		}

		// store them to database
		if err := pubRandStore.AddPubRandProofList(fpPk.MustMarshal(), []byte(storedFp.ChainID), commit.StartHeight, commit.NumPubRand, proofList); err != nil {
			return fmt.Errorf("failed to save public randomness to DB: %w", err)
		}
	}

	return nil
}
