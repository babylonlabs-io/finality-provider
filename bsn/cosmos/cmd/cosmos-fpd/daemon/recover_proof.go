package daemon

import (
	"fmt"
	appparams "github.com/babylonlabs-io/babylon/v3/app/params"
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/config"
	cosmwasmcfg "github.com/babylonlabs-io/finality-provider/bsn/cosmos/cosmwasmclient/config"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	"github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/finality-provider/store"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cometbft/cometbft/crypto/merkle"
	"path/filepath"

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

	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.Common.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize the logger: %w", err)
	}

	// Create encoding config with the correct account prefix
	service.LockAddressPrefix()
	appparams.SetAddressPrefixes()
	service.UnlockAddressPrefix()
	wasmEncodingCfg := cosmwasmcfg.GetWasmdEncodingConfig()
	cosmWasmCtrl, err := clientcontroller.NewCosmwasmConsumerController(cfg.Cosmwasm, wasmEncodingCfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create rpc client for the consumer chain cosmos: %w", err)
	}

	db, err := cfg.Common.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	defer func() {
		err := db.Close()
		if err != nil {
			panic(fmt.Errorf("failed to close db: %w", err))
		}
	}()

	pubRandStore, err := store.NewPubRandProofStore(db)
	if err != nil {
		return fmt.Errorf("failed to initiate public randomness store: %w", err)
	}

	err = fpdaemon.RunCommandRecoverProofWithConfig(ctx, cmd, cfg.Common, cosmWasmCtrl, args,
		func(chainID []byte, pk []byte, commit types.PubRandCommit, proofList []*merkle.Proof) error {
			if err := pubRandStore.AddPubRandProofList(chainID, pk, commit.GetStartHeight(), commit.GetNumPubRand(), proofList); err != nil {
				return fmt.Errorf("failed to save public randomness to DB: %w", err)
			}

			return nil
		}, func(em *eotsclient.EOTSManagerGRpcClient, fpPk []byte, chainID []byte, commit types.PubRandCommit) ([]*btcec.FieldVal, error) {
			return em.CreateRandomnessPairList(fpPk, chainID, commit.GetStartHeight(), uint32(commit.GetNumPubRand())) // #nosec G115 - already checked by caller
		})

	if err != nil {
		return fmt.Errorf("failed to run recover proof command: %w", err)
	}

	return nil
}
