package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"cosmossdk.io/math"
	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	commoncmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/common"
	fpdaemon "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/daemon"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	dc "github.com/babylonlabs-io/finality-provider/finality-provider/service/client"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/cosmos/cosmos-sdk/client"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CommandCreateFP returns the create-finality-provider command by connecting to the fpd daemon.
func CommandCreateFP() *cobra.Command {
	cmd := fpdaemon.CommandCreateFPTemplate()
	cmd.RunE = clientctx.RunEWithClientCtx(runCommandCreateFP)

	return cmd
}

func runCommandCreateFP(ctx client.Context, cmd *cobra.Command, _ []string) error {
	flags := cmd.Flags()

	fpJSONPath, err := flags.GetString(commoncmd.FromFileFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", commoncmd.FromFileFlag, err)
	}

	var fp *parsedFinalityProvider
	if fpJSONPath != "" {
		fp, err = parseFinalityProviderJSON(fpJSONPath)
	} else {
		fp, err = parseFinalityProviderFlags(cmd)
	}
	if err != nil {
		panic(err)
	}
	// Handle key name loading if not provided
	if fp.keyName == "" {
		cfg, err := fpcfg.LoadConfig(ctx.HomeDir)
		if err != nil {
			return fmt.Errorf("failed to load config from %s: %w", fpcfg.CfgFile(ctx.HomeDir), err)
		}
		fp.keyName = cfg.BabylonConfig.Key
		if fp.keyName == "" {
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
	monikerStr, err := f.GetString(commoncmd.MonikerFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", commoncmd.MonikerFlag, err)
	}
	identityStr, err := f.GetString(commoncmd.IdentityFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", commoncmd.IdentityFlag, err)
	}
	websiteStr, err := f.GetString(commoncmd.WebsiteFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", commoncmd.WebsiteFlag, err)
	}
	securityContactStr, err := f.GetString(commoncmd.SecurityContactFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", commoncmd.SecurityContactFlag, err)
	}
	detailsStr, err := f.GetString(commoncmd.DetailsFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", commoncmd.DetailsFlag, err)
	}

	description := stakingtypes.NewDescription(monikerStr, identityStr, websiteStr, securityContactStr, detailsStr)

	return description.EnsureLength()
}

type parsedFinalityProvider struct {
	keyName         string
	chainID         string
	eotsPK          string
	description     stakingtypes.Description
	commissionRates *proto.CommissionRates
}

func parseFinalityProviderJSON(path string) (*parsedFinalityProvider, error) {
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

func parseFinalityProviderFlags(cmd *cobra.Command) (*parsedFinalityProvider, error) {
	flags := cmd.Flags()

	commissionRateStr, err := flags.GetString(commoncmd.CommissionRateFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", commoncmd.CommissionRateFlag, err)
	}

	commissionMaxRateStr, err := flags.GetString(commoncmd.CommissionMaxRateFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", commoncmd.CommissionMaxRateFlag, err)
	}

	commissionMaxChangeRateStr, err := flags.GetString(commoncmd.CommissionMaxChangeRateFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", commoncmd.CommissionMaxChangeRateFlag, err)
	}

	commRates, err := getCommissionRates(commissionRateStr, commissionMaxRateStr, commissionMaxChangeRateStr)
	if err != nil {
		return nil, err
	}

	description, err := getDescriptionFromFlags(flags)
	if err != nil {
		return nil, fmt.Errorf("invalid description: %w", err)
	}

	keyName, err := flags.GetString(commoncmd.KeyNameFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", commoncmd.KeyNameFlag, err)
	}

	chainID, err := flags.GetString(commoncmd.ChainIDFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", commoncmd.ChainIDFlag, err)
	}

	if chainID == "" {
		return nil, fmt.Errorf("chain-id cannot be empty")
	}

	eotsPkHex, err := flags.GetString(commoncmd.FpEotsPkFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", commoncmd.FpEotsPkFlag, err)
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
