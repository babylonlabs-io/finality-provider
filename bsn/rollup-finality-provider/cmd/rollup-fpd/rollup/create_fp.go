package rollup

import (
	"fmt"
	"strconv"
	"strings"

	rollupcfg "github.com/babylonlabs-io/finality-provider/bsn/rollup-finality-provider/config"
	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	babyloncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/babylon"
	common "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
)

var (
	defaultFpdDaemonAddress = "127.0.0.1:" + strconv.Itoa(fpcfg.DefaultRPCPort)
)

// CommandCreateFP returns the create-finality-provider command by connecting to the fpd daemon.
func CommandCreateFP() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "create-finality-provider",
		Aliases: []string{"cfp"},
		Short:   "Create a finality provider object and save it in database.",
		Long: "Create a new finality provider object and store it in the finality provider database. " +
			"It needs to have an operating EOTS manager available and running.",
		Example: strings.TrimSpace(
			fmt.Sprintf(`
Either by specifying all flags manually:

$fpd create-finality-provider --daemon-address %s ...

Or providing the path to finality-provider.json:
$fpd create-finality-provider --daemon-address %s --from-file /path/to/finality-provider.json

Where finality-provider.json contains:

{
  "keyName": "The unique key name of the finality provider's Babylon account",
  "chainID": "The identifier of the consumer chain",
  "passphrase": "The pass phrase used to encrypt the keys",
  "commissionRate": "The initial commission rate for the finality provider, e.g., 0.05",
  "commissionMaxRate": "The maximum commission rate percentage for the finality provider, e.g., 0.20",
  "commissionMaxChangeRate": "The maximum commission change rate percentage (per day) for the finality provider, e.g., 0.01",
  "moniker": ""A human-readable name for the finality provider",
  "identity": "A optional identity signature",
  "website": "Validator's (optional) website",
  "securityContract": "Validator's (optional) security contact email",
  "details": "Validator's (optional) details",
  "eotsPK": "The hex string of the finality provider's EOTS public key"
}
`, defaultFpdDaemonAddress, defaultFpdDaemonAddress)),
		Args: cobra.NoArgs,
		RunE: fpcmd.RunEWithClientCtx(runCommandCreateFP),
	}

	f := cmd.Flags()
	f.String(common.FpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	f.String(common.KeyNameFlag, "", "The unique key name of the finality provider's Babylon account")
	f.String(sdkflags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")
	f.String(common.ChainIDFlag, "", "The identifier of the consumer chain")
	f.String(common.CommissionRateFlag, "", "The initial commission rate for the finality provider, e.g., 0.05")
	f.String(common.CommissionMaxRateFlag, "", "The maximum commission rate percentage for the finality provider, e.g., 0.20")
	f.String(common.CommissionMaxChangeRateFlag, "", "The maximum commission change rate percentage (per day) for the finality provider, e.g., 0.01")
	f.String(common.MonikerFlag, "", "A human-readable name for the finality provider")
	f.String(common.IdentityFlag, "", "An optional identity signature (ex. UPort or Keybase)")
	f.String(common.WebsiteFlag, "", "An optional website link")
	f.String(common.SecurityContactFlag, "", "An email for security contact")
	f.String(common.DetailsFlag, "", "Other optional details")
	f.String(common.FpEotsPkFlag, "", "The hex string of the finality provider's EOTS public key")
	f.String(common.FromFileFlag, "", "Path to a json file containing finality provider data")

	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		fromFilePath, _ := cmd.Flags().GetString(common.FromFileFlag)
		if fromFilePath == "" {
			// Mark flags as required only if --from-file is not provided
			if err := cmd.MarkFlagRequired(common.ChainIDFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(common.KeyNameFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(common.MonikerFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(common.CommissionRateFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(common.CommissionMaxRateFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(common.CommissionMaxChangeRateFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(common.FpEotsPkFlag); err != nil {
				return err
			}
		}

		return nil
	}

	return cmd
}

func runCommandCreateFP(ctx client.Context, cmd *cobra.Command, _ []string) error {
	cfg, err := rollupcfg.LoadRollupFPConfig(ctx.HomeDir)
	if err != nil {
		return fmt.Errorf("failed to load config from %s: %w", fpcfg.CfgFile(ctx.HomeDir), err)
	}

	return babyloncmd.RunCommandCreateFPWithCfg(ctx, cmd, cfg.Common)
}
