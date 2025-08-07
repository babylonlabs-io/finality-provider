package daemon

import (
	"fmt"
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/clientcontroller"
	"github.com/babylonlabs-io/finality-provider/bsn/cosmos/config"
	cosmwasmcfg "github.com/babylonlabs-io/finality-provider/bsn/cosmos/cosmwasmclient/config"
	"github.com/babylonlabs-io/finality-provider/log"
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
	wasmEncodingCfg := cosmwasmcfg.GetWasmdEncodingConfigWithPrefix(cfg.Cosmwasm.AccountPrefix)
	cosmWasmCtrl, err := clientcontroller.NewCosmwasmConsumerController(cfg.Cosmwasm, wasmEncodingCfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create rpc client for the consumer chain cosmos: %w", err)
	}

	if err := fpdaemon.RunCommandRecoverProofWithConfig(ctx, cmd, cfg.Common, cosmWasmCtrl, args); err != nil {
		return fmt.Errorf("failed to run recover proof command: %w", err)
	}

	return nil
}
