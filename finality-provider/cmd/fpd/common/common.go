package common

import (
	"encoding/json"
	"fmt"
	"os"

	"cosmossdk.io/math"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func LoadKeyName(cfg *fpcfg.Config, cmd *cobra.Command) (string, error) {
	keyName, err := cmd.Flags().GetString(KeyNameFlag)
	if err != nil {
		return "", fmt.Errorf("failed to read flag %s: %w", KeyNameFlag, err)
	}
	// if key name is not specified, we use the key of the config
	if keyName != "" {
		return keyName, nil
	}

	keyName = cfg.BabylonConfig.Key
	if keyName == "" {
		return "", fmt.Errorf("the key in config is empty")
	}

	return keyName, nil
}

type ParsedFinalityProvider struct {
	KeyName         string
	ChainID         string
	EotsPK          string
	Description     stakingtypes.Description
	CommissionRates *proto.CommissionRates
}

func ParseFinalityProviderJSON(path string, cfg *fpcfg.Config) (*ParsedFinalityProvider, error) {
	type InternalFpJSON struct {
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

	var fp InternalFpJSON
	if err := json.Unmarshal(contents, &fp); err != nil {
		return nil, err
	}

	if fp.ChainID == "" {
		return nil, fmt.Errorf("chainID is required")
	}

	if fp.KeyName == "" {
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

	commRates, err := GetCommissionRates(fp.CommissionRate, fp.CommissionMaxRate, fp.CommissionMaxChangeRate)
	if err != nil {
		return nil, err
	}

	description, err := stakingtypes.NewDescription(fp.Moniker, fp.Identity, fp.Website, fp.SecurityContract, fp.Details).EnsureLength()
	if err != nil {
		return nil, err
	}

	return &ParsedFinalityProvider{
		KeyName:         fp.KeyName,
		ChainID:         fp.ChainID,
		EotsPK:          fp.EotsPK,
		Description:     description,
		CommissionRates: commRates,
	}, nil
}

func ParseFinalityProviderFlags(cmd *cobra.Command, cfg *fpcfg.Config) (*ParsedFinalityProvider, error) {
	flags := cmd.Flags()

	commissionRateStr, err := flags.GetString(CommissionRateFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", CommissionRateFlag, err)
	}

	commissionMaxRateStr, err := flags.GetString(CommissionMaxRateFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", CommissionMaxRateFlag, err)
	}

	commissionMaxChangeRateStr, err := flags.GetString(CommissionMaxChangeRateFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", CommissionMaxChangeRateFlag, err)
	}

	commRates, err := GetCommissionRates(commissionRateStr, commissionMaxRateStr, commissionMaxChangeRateStr)
	if err != nil {
		return nil, err
	}

	description, err := GetDescriptionFromFlags(flags)
	if err != nil {
		return nil, fmt.Errorf("invalid description: %w", err)
	}

	keyName, err := LoadKeyName(cfg, cmd)
	if err != nil {
		return nil, fmt.Errorf("not able to load key name: %w", err)
	}

	if keyName == "" {
		return nil, fmt.Errorf("keyname cannot be empty")
	}

	chainID, err := flags.GetString(ChainIDFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", ChainIDFlag, err)
	}

	if chainID == "" {
		return nil, fmt.Errorf("chain-id cannot be empty")
	}

	eotsPkHex, err := flags.GetString(FpEotsPkFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to read flag %s: %w", FpEotsPkFlag, err)
	}

	if eotsPkHex == "" {
		return nil, fmt.Errorf("eots-pk cannot be empty")
	}

	return &ParsedFinalityProvider{
		KeyName:         keyName,
		ChainID:         chainID,
		EotsPK:          eotsPkHex,
		Description:     description,
		CommissionRates: commRates,
	}, nil
}

// GetCommissionRates is a helper function to get the commission rates fields
// from string to LegacyDec.
func GetCommissionRates(rateStr, maxRateStr, maxChangeRateStr string) (*proto.CommissionRates, error) {
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

func GetDescriptionFromFlags(f *pflag.FlagSet) (stakingtypes.Description, error) {
	// get information for description
	var desc stakingtypes.Description
	monikerStr, err := f.GetString(MonikerFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", MonikerFlag, err)
	}
	identityStr, err := f.GetString(IdentityFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", IdentityFlag, err)
	}
	websiteStr, err := f.GetString(WebsiteFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", WebsiteFlag, err)
	}
	securityContactStr, err := f.GetString(SecurityContactFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", SecurityContactFlag, err)
	}
	detailsStr, err := f.GetString(DetailsFlag)
	if err != nil {
		return desc, fmt.Errorf("failed to read flag %s: %w", DetailsFlag, err)
	}

	description := stakingtypes.NewDescription(monikerStr, identityStr, websiteStr, securityContactStr, detailsStr)

	return description.EnsureLength()
}
