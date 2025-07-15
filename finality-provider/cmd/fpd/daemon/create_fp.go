package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"cosmossdk.io/math"
	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	dc "github.com/babylonlabs-io/finality-provider/finality-provider/service/client"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	f.String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	f.String(keyNameFlag, "", "The unique key name of the finality provider's Babylon account")
	f.String(sdkflags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")
	f.String(chainIDFlag, "", "The identifier of the consumer chain")
	f.String(commissionRateFlag, "", "The initial commission rate for the finality provider, e.g., 0.05")
	f.String(commissionMaxRateFlag, "", "The maximum commission rate percentage for the finality provider, e.g., 0.20")
	f.String(commissionMaxChangeRateFlag, "", "The maximum commission change rate percentage (per day) for the finality provider, e.g., 0.01")
	f.String(monikerFlag, "", "A human-readable name for the finality provider")
	f.String(identityFlag, "", "An optional identity signature (ex. UPort or Keybase)")
	f.String(websiteFlag, "", "An optional website link")
	f.String(securityContactFlag, "", "An email for security contact")
	f.String(detailsFlag, "", "Other optional details")
	f.String(fpEotsPkFlag, "", "The hex string of the finality provider's EOTS public key")
	f.String(fromFile, "", "Path to a json file containing finality provider data")

	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		fromFilePath, _ := cmd.Flags().GetString(fromFile)
		if fromFilePath == "" {
			// Mark flags as required only if --from-file is not provided
			if err := cmd.MarkFlagRequired(chainIDFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(keyNameFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(monikerFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(commissionRateFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(commissionMaxRateFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(commissionMaxChangeRateFlag); err != nil {
				return err
			}
			if err := cmd.MarkFlagRequired(fpEotsPkFlag); err != nil {
				return err
			}
		}

		return nil
	}

	return cmd
}

func runCommandCreateFP(ctx client.Context, cmd *cobra.Command, _ []string) error {
	flags := cmd.Flags()

	fpJSONPath, err := flags.GetString(fromFile)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", fromFile, err)
	}

	var fp *parsedFinalityProvider
	if fpJSONPath != "" {
		fp, err = parseFinalityProviderJSON(fpJSONPath, ctx.HomeDir)
		if err != nil {
			panic(err)
		}
	} else {
		fp, err = parseFinalityProviderFlags(cmd, ctx.HomeDir)
		if err != nil {
			panic(err)
		}
	}

	daemonAddress, err := flags.GetString(fpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", fpdDaemonAddressFlag, err)
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
		context.Background(),
		fp.keyName,
		fp.chainID,
		fp.eotsPK,
		fp.description,
		fp.commissionRates,
	)
	if err != nil {
		return err
	}

	types.PrintRespJSON(res)

	cmd.Println("Your finality provider is successfully created. Please restart your fpd.")

	return nil
}

func getDescriptionFromFlags(f *pflag.FlagSet) (stakingtypes.Description, error) {
	// get information for description
	var desc stakingtypes.Description
	monikerStr, err := f.GetString(monikerFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", monikerFlag, err)
	}
	identityStr, err := f.GetString(identityFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", identityFlag, err)
	}
	websiteStr, err := f.GetString(websiteFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", websiteFlag, err)
	}
	securityContactStr, err := f.GetString(securityContactFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", securityContactFlag, err)
	}
	detailsStr, err := f.GetString(detailsFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", detailsFlag, err)
	}

	description := stakingtypes.NewDescription(monikerStr, identityStr, websiteStr, securityContactStr, detailsStr)

	return description.EnsureLength()
}

func loadKeyName(homeDir string, cmd *cobra.Command) (string, error) {
	keyName, err := cmd.Flags().GetString(keyNameFlag)
	if err != nil {
		return "", fmt.Errorf("failed to read flag %s: %w", keyNameFlag, err)
	}
	// if key name is not specified, we use the key of the config
	if keyName != "" {
		return keyName, nil
	}

	// we add the following check to ensure that the chain key is created
	// beforehand
	cfg, err := fpcfg.LoadConfig(homeDir)
	if err != nil {
		return "", fmt.Errorf("failed to load config from %s: %w", fpcfg.CfgFile(homeDir), err)
	}

	keyName = cfg.BabylonConfig.Key
	if keyName == "" {
		return "", fmt.Errorf("the key in config is empty")
	}

	return keyName, nil
}

type parsedFinalityProvider struct {
	keyName         string
	chainID         string
	eotsPK          string
	description     stakingtypes.Description
	commissionRates *proto.CommissionRates
}

func parseFinalityProviderJSON(path string, homeDir string) (*parsedFinalityProvider, error) {
	type internalFpJSON struct {
		KeyName                 string `json:"keyName"`
		ChainID                 string `json:"chainID"`
		Passphrase              string `json:"passphrase"`
		CommissionRate          string `json:"commissionRate"`
		CommissionMaxRate       string `json:"commissionMaxRate"`
		CommissionMaxChangeRate string `json:"commissionMaxChangeRate"`
		Moniker                 string `json:"moniker"`
		Identity                string `json:"identity"`
		Website                 string `json:"website"`
		SecurityContract        string `json:"securityContract"`
		Details                 string `json:"details"`
		EotsPK                  string `json:"eotsPK"`
	}

	// #nosec G304 - The log file path is provided by the user and not externally
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var fp internalFpJSON
	if err := json.Unmarshal(contents, &fp); err != nil {
		return nil, err
	}

	if fp.ChainID == "" {
		return nil, fmt.Errorf("chainID is required")
	}

	if fp.KeyName == "" {
		cfg, err := fpcfg.LoadConfig(homeDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", fpcfg.CfgFile(homeDir), err)
		}
		fp.KeyName = cfg.BabylonConfig.Key
		if fp.KeyName == "" {
			return nil, fmt.Errorf("the key is neither in config nor provided in the json file")
		}
	}

	if fp.Moniker == "" {
		return nil, fmt.Errorf("moniker is required")
	}

	if fp.CommissionRate == "" {
		return nil, fmt.Errorf("commissionRate is required")
	}

	if fp.CommissionMaxRate == "" {
		return nil, fmt.Errorf("CommissionMaxRate is required")
	}

	if fp.CommissionMaxChangeRate == "" {
		return nil, fmt.Errorf("CommissionMaxChangeRate is required")
	}

	if fp.EotsPK == "" {
		return nil, fmt.Errorf("eotsPK is required")
	}

	commRates, err := getCommissionRates(fp.CommissionRate, fp.CommissionMaxRate, fp.CommissionMaxChangeRate)
	if err != nil {
		return nil, err
	}

	description, err := stakingtypes.NewDescription(fp.Moniker, fp.Identity, fp.Website, fp.SecurityContract, fp.Details).EnsureLength()
	if err != nil {
		return nil, err
	}

	return &parsedFinalityProvider{
		keyName:         fp.KeyName,
		chainID:         fp.ChainID,
		eotsPK:          fp.EotsPK,
		description:     description,
		commissionRates: commRates,
	}, nil
}

func parseFinalityProviderFlags(cmd *cobra.Command, homeDir string) (*parsedFinalityProvider, error) {
	flags := cmd.Flags()

	commissionRateStr, err := flags.GetString(commissionRateFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", commissionRateFlag, err)
	}

	commissionMaxRateStr, err := flags.GetString(commissionMaxRateFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", commissionMaxRateFlag, err)
	}

	commissionMaxChangeRateStr, err := flags.GetString(commissionMaxChangeRateFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", commissionMaxChangeRateFlag, err)
	}

	commRates, err := getCommissionRates(commissionRateStr, commissionMaxRateStr, commissionMaxChangeRateStr)
	if err != nil {
		return nil, err
	}

	description, err := getDescriptionFromFlags(flags)
	if err != nil {
		return nil, fmt.Errorf("invalid description: %w", err)
	}

	keyName, err := loadKeyName(homeDir, cmd)
	if err != nil {
		return nil, fmt.Errorf("not able to load key name: %w", err)
	}

	if keyName == "" {
		return nil, fmt.Errorf("keyname cannot be empty")
	}

	chainID, err := flags.GetString(chainIDFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", chainIDFlag, err)
	}

	if chainID == "" {
		return nil, fmt.Errorf("chain-id cannot be empty")
	}

	eotsPkHex, err := flags.GetString(fpEotsPkFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", fpEotsPkFlag, err)
	}

	if eotsPkHex == "" {
		return nil, fmt.Errorf("eots-pk cannot be empty")
	}

	return &parsedFinalityProvider{
		keyName:         keyName,
		chainID:         chainID,
		eotsPK:          eotsPkHex,
		description:     description,
		commissionRates: commRates,
	}, nil
}

// getCommissionRates is a helper function to get the commission rates fields
// from string to LegacyDec.
func getCommissionRates(rateStr, maxRateStr, maxChangeRateStr string) (*proto.CommissionRates, error) {
	rate, err := math.LegacyNewDecFromStr(rateStr)
	if err != nil {
		return nil, fmt.Errorf("invalid commission rate: %w", err)
	}
	maxRate, err := math.LegacyNewDecFromStr(maxRateStr)
	if err != nil {
		return nil, fmt.Errorf("invalid commission max rate: %w", err)
	}
	maxRateChange, err := math.LegacyNewDecFromStr(maxChangeRateStr)
	if err != nil {
		return nil, fmt.Errorf("invalid commission max change rate: %w", err)
	}

	return proto.NewCommissionRates(rate, maxRate, maxRateChange), nil
}
