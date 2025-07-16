package daemon

import (
	"fmt"

	rollupfpcfg "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	commoncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpdaemon "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	dc "github.com/babylonlabs-io/finality-provider/finality-provider/service/client"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
)

// CommandCreateFP returns the create-finality-provider command by connecting to the fpd daemon.
func CommandCreateFP(binaryName string) *cobra.Command {
	cmd := fpdaemon.CommandCreateFPTemplate(binaryName)
	cmd.RunE = clientctx.RunEWithClientCtx(runCommandCreateFP)

	return cmd
}

func runCommandCreateFP(ctx client.Context, cmd *cobra.Command, _ []string) error {
	flags := cmd.Flags()

	fpJSONPath, err := flags.GetString(commoncmd.FromFileFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", commoncmd.FromFileFlag, err)
	}

	var fp *fpdaemon.ParsedFinalityProvider
	if fpJSONPath != "" {
		fp, err = fpdaemon.ParseFinalityProviderJSON(fpJSONPath)
	} else {
		fp, err = fpdaemon.ParseFinalityProviderFlags(cmd)
	}
	if err != nil {
		panic(err)
	}

	// Handle key name loading if not provided
	if fp.KeyName == "" {
		cfg, err := rollupfpcfg.LoadConfig(ctx.HomeDir)
		if err != nil {
			return fmt.Errorf("failed to load config from %s: %w", rollupfpcfg.CfgFile(ctx.HomeDir), err)
		}
		fp.KeyName = cfg.Common.BabylonConfig.Key
		if fp.KeyName == "" {
			return fmt.Errorf("the key is neither in config nor provided")
		}
	}

	daemonAddress, err := flags.GetString(commoncmd.FpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", commoncmd.FpdDaemonAddressFlag, err)
	}

	client, cleanUp, err := dc.NewFinalityProviderServiceGRpcClient(daemonAddress)
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanUp(); err != nil {
			fmt.Printf("Failed to clean up grpc client: %v\n", err)
		}
	}()

	res, err := client.CreateFinalityProvider(
		cmd.Context(),
		fp.KeyName,
		fp.ChainID,
		fp.EotsPK,
		fp.Description,
		fp.CommissionRates,
	)
	if err != nil {
		return err
	}

	types.PrintRespJSON(cmd, res)

	cmd.Println("Finality provider created successfully. Please restart the fpd.")

	return nil
}
