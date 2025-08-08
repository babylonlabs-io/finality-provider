package daemon

import (
	"fmt"
	rollupfpcc "github.com/babylonlabs-io/finality-provider/bsn/rollup/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/cometbft/cometbft/crypto/merkle"
	"path/filepath"

	rollupfpcfg "github.com/babylonlabs-io/finality-provider/bsn/rollup/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	fpdaemon "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
)

func CommandRecoverProof(binaryName string) *cobra.Command {
	cmd := fpdaemon.CommandRecoverProofTemplate(binaryName)
	cmd.RunE = clientctx.RunEWithClientCtx(runCommandRecoverProof)

	return cmd
}

func runCommandRecoverProof(ctx client.Context, cmd *cobra.Command, args []string) error {
	// Get homePath from context like in start.go
	homePath, err := filepath.Abs(ctx.HomeDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	homePath = util.CleanAndExpandPath(homePath)

	cfg, err := rollupfpcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger, err := log.NewRootLoggerWithFile(rollupfpcfg.LogFile(homePath), cfg.Common.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	rollupCtrl, err := rollupfpcc.NewRollupBSNController(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create rpc client for the consumer chain rollup: %w", err)
	}

	db, err := cfg.Common.DatabaseConfig.GetDBBackend()
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

	return fpdaemon.RunCommandRecoverProofWithConfig(ctx, cmd, cfg.Common, rollupCtrl, func(chainID []byte, pk []byte, commit types.PubRandCommit, proofList []*merkle.Proof) error {
		concreteCommit, ok := commit.(*rollupfpcc.RollupPubRandCommit)
		if !ok {
			return fmt.Errorf("expected RollupPubRandCommit, got %T", commit)
		}

		if err := pubRandStore.AddPubRandProofListWithInterval(chainID, pk, commit.GetStartHeight(), commit.GetNumPubRand(), proofList, concreteCommit.Interval); err != nil {
			return fmt.Errorf("failed to save public randomness to DB: %w", err)
		}

		return nil
	}, args)
}
