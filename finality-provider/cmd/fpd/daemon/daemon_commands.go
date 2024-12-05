package daemon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/types"
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"

	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	dc "github.com/babylonlabs-io/finality-provider/finality-provider/service/client"
)

var (
	defaultFpdDaemonAddress = "127.0.0.1:" + strconv.Itoa(fpcfg.DefaultRPCPort)
	defaultAppHashStr       = "fd903d9baeb3ab1c734ee003de75f676c5a9a8d0574647e5385834d57d3e79ec"
)

// CommandGetDaemonInfo returns the get-info command by connecting to the fpd daemon.
func CommandGetDaemonInfo() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "get-info",
		Aliases: []string{"gi"},
		Short:   "Get information of the running fpd daemon.",
		Example: fmt.Sprintf(`fpd get-info --daemon-address %s`, defaultFpdDaemonAddress),
		Args:    cobra.NoArgs,
		RunE:    runCommandGetDaemonInfo,
	}
	cmd.Flags().String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	return cmd
}

func runCommandGetDaemonInfo(cmd *cobra.Command, _ []string) error {
	daemonAddress, err := cmd.Flags().GetString(fpdDaemonAddressFlag)
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

	info, err := client.GetInfo(context.Background())
	if err != nil {
		return err
	}

	printRespJSON(info)
	return nil
}

// CommandCreateFP returns the create-finality-provider command by connecting to the fpd daemon.
func CommandCreateFP() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "create-finality-provider",
		Aliases: []string{"cfp"},
		Short:   "Create a finality provider object and save it in database.",
		Long: "Create a new finality provider object and store it in the finality provider database. " +
			"It needs to have an operating EOTS manager available and running.",
		Example: fmt.Sprintf(`fpd create-finality-provider --daemon-address %s ...`, defaultFpdDaemonAddress),
		Args:    cobra.NoArgs,
		RunE:    fpcmd.RunEWithClientCtx(runCommandCreateFP),
	}

	f := cmd.Flags()
	f.String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	f.String(keyNameFlag, "", "The unique key name of the finality provider's Babylon account")
	f.String(sdkflags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")
	f.String(chainIDFlag, "", "The identifier of the consumer chain")
	f.String(passphraseFlag, "", "The pass phrase used to encrypt the keys")
	f.String(commissionRateFlag, "0.05", "The commission rate for the finality provider, e.g., 0.05")
	f.String(monikerFlag, "", "A human-readable name for the finality provider")
	f.String(identityFlag, "", "An optional identity signature (ex. UPort or Keybase)")
	f.String(websiteFlag, "", "An optional website link")
	f.String(securityContactFlag, "", "An email for security contact")
	f.String(detailsFlag, "", "Other optional details")
	f.String(fpEotsPkFlag, "", "The hex string of the finality provider's EOTS public key")

	// make flags required
	if err := cmd.MarkFlagRequired(chainIDFlag); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(keyNameFlag); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(monikerFlag); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(commissionRateFlag); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(fpEotsPkFlag); err != nil {
		panic(err)
	}

	return cmd
}

func runCommandCreateFP(ctx client.Context, cmd *cobra.Command, _ []string) error {
	flags := cmd.Flags()
	daemonAddress, err := flags.GetString(fpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", fpdDaemonAddressFlag, err)
	}

	commissionRateStr, err := flags.GetString(commissionRateFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", commissionRateFlag, err)
	}
	commissionRate, err := math.LegacyNewDecFromStr(commissionRateStr)
	if err != nil {
		return fmt.Errorf("invalid commission rate: %w", err)
	}

	description, err := getDescriptionFromFlags(flags)
	if err != nil {
		return fmt.Errorf("invalid description: %w", err)
	}

	keyName, err := loadKeyName(ctx.HomeDir, cmd)
	if err != nil {
		return fmt.Errorf("not able to load key name: %w", err)
	}

	if keyName == "" {
		return fmt.Errorf("keyname cannot be empty")
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

	chainID, err := flags.GetString(chainIDFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", chainIDFlag, err)
	}

	if chainID == "" {
		return fmt.Errorf("chain-id cannot be empty")
	}

	passphrase, err := flags.GetString(passphraseFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", passphraseFlag, err)
	}

	eotsPkHex, err := flags.GetString(fpEotsPkFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", fpEotsPkFlag, err)
	}

	if eotsPkHex == "" {
		return fmt.Errorf("eots-pk cannot be empty")
	}

	res, err := client.CreateFinalityProvider(
		context.Background(),
		keyName,
		chainID,
		eotsPkHex,
		passphrase,
		description,
		&commissionRate,
	)
	if err != nil {
		return err
	}

	printRespJSON(res)
	return nil
}

// CommandUnjailFP returns the unjail-finality-provider command by connecting to the fpd daemon.
func CommandUnjailFP() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "unjail-finality-provider",
		Aliases: []string{"ufp"},
		Short:   "Unjail the given finality provider.",
		Example: fmt.Sprintf(`fpd unjail-finality-provider [eots-pk] --daemon-address %s ...`, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(1),
		RunE:    fpcmd.RunEWithClientCtx(runCommandUnjailFP),
	}

	f := cmd.Flags()
	f.String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")

	return cmd
}

func runCommandUnjailFP(_ client.Context, cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
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

	_, err = client.UnjailFinalityProvider(context.Background(), args[0])
	if err != nil {
		return err
	}

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

// CommandLsFP returns the list-finality-providers command by connecting to the fpd daemon.
func CommandLsFP() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "list-finality-providers",
		Aliases: []string{"ls"},
		Short:   "List finality providers stored in the database.",
		Example: fmt.Sprintf(`fpd list-finality-providers --daemon-address %s`, defaultFpdDaemonAddress),
		Args:    cobra.NoArgs,
		RunE:    runCommandLsFP,
	}
	cmd.Flags().String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	return cmd
}

func runCommandLsFP(cmd *cobra.Command, _ []string) error {
	daemonAddress, err := cmd.Flags().GetString(fpdDaemonAddressFlag)
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

	resp, err := client.QueryFinalityProviderList(context.Background())
	if err != nil {
		return err
	}
	printRespJSON(resp)

	return nil
}

// CommandInfoFP returns the finality-provider-info command by connecting to the fpd daemon.
func CommandInfoFP() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "finality-provider-info [fp-eots-pk-hex]",
		Aliases: []string{"fpi"},
		Short:   "List finality providers stored in the database.",
		Example: fmt.Sprintf(`fpd finality-provider-info --daemon-address %s`, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(1),
		RunE:    runCommandInfoFP,
	}
	cmd.Flags().String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	return cmd
}

func runCommandInfoFP(cmd *cobra.Command, args []string) error {
	fpPk, err := types.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return err
	}

	daemonAddress, err := cmd.Flags().GetString(fpdDaemonAddressFlag)
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

	resp, err := client.QueryFinalityProviderInfo(context.Background(), fpPk)
	if err != nil {
		return err
	}
	printRespJSON(resp)

	return nil
}

// CommandAddFinalitySig returns the add-finality-sig command by connecting to the fpd daemon.
func CommandAddFinalitySig() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "unsafe-add-finality-sig [fp-eots-pk-hex] [block-height]",
		Aliases: []string{"unsafe-afs"},
		Short:   "[UNSAFE] Send a finality signature to the consumer chain.",
		Long:    "[UNSAFE] Send a finality signature to the consumer chain. This command should only be used for presentation/testing purposes",
		Example: fmt.Sprintf(`fpd unsafe-add-finality-sig [fp-eots-pk-hex] [block-height] --daemon-address %s`, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(2),
		RunE:    runCommandAddFinalitySig,
	}
	cmd.Flags().String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	cmd.Flags().String(appHashFlag, defaultAppHashStr, "The last commit hash of the chain block")
	cmd.Flags().Bool(checkDoubleSignFlag, true, "If 'true', uses anti-slashing protection when doing EOTS sign")

	return cmd
}

func runCommandAddFinalitySig(cmd *cobra.Command, args []string) error {
	fpPk, err := types.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return err
	}
	blkHeight, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return err
	}

	flags := cmd.Flags()
	daemonAddress, err := flags.GetString(fpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", fpdDaemonAddressFlag, err)
	}

	appHashHex, err := flags.GetString(appHashFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", appHashFlag, err)
	}

	checkDoubleSign, err := flags.GetBool(checkDoubleSignFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", checkDoubleSignFlag, err)
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

	appHash, err := hex.DecodeString(appHashHex)
	if err != nil {
		return err
	}

	res, err := client.AddFinalitySignature(context.Background(), fpPk.MarshalHex(), blkHeight, appHash, checkDoubleSign)
	if err != nil {
		return err
	}
	printRespJSON(res)

	return nil
}

// CommandEditFinalityDescription edits description of finality provider
func CommandEditFinalityDescription() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "edit-finality-provider [eots_pk]",
		Aliases: []string{"efp"},
		Short:   "Edit finality provider data without resetting unchanged fields",
		Long: "Edit the details of a finality provider using the specified EOTS public key. " +
			"\nThe provided [eots_pk] must correspond to the Babylon address controlled by the key specified in fpd.conf. " +
			"\nIf one or more optional flags are passed (such as --moniker, --website, etc.), " +
			"the corresponding values are updated, while unchanged fields retain their current values from the Babylon Node.",
		Example: fmt.Sprintf(`fpd edit-finality-provider [eots_pk] --daemon-address %s --moniker "new-moniker"`, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(1),
		RunE:    runCommandEditFinalityDescription,
	}
	cmd.Flags().String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	cmd.Flags().String(monikerFlag, "", "The finality provider's (optional) moniker")
	cmd.Flags().String(websiteFlag, "", "The finality provider's (optional) website")
	cmd.Flags().String(securityContactFlag, "", "The finality provider's (optional) security contact email")
	cmd.Flags().String(detailsFlag, "", "The finality provider's (optional) details")
	cmd.Flags().String(identityFlag, "", "The (optional) identity signature (ex. UPort or Keybase)")
	cmd.Flags().String(commissionRateFlag, "", "The (optional) commission rate percentage (ex. 0.2)")

	return cmd
}

func runCommandEditFinalityDescription(cmd *cobra.Command, args []string) error {
	fpPk, err := types.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return err
	}

	flags := cmd.Flags()
	daemonAddress, err := flags.GetString(fpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", fpdDaemonAddressFlag, err)
	}

	grpcClient, cleanUp, err := dc.NewFinalityProviderServiceGRpcClient(daemonAddress)
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanUp(); err != nil {
			fmt.Printf("Failed to clean up grpc client: %v\n", err)
		}
	}()

	moniker, _ := cmd.Flags().GetString(monikerFlag)
	website, _ := cmd.Flags().GetString(websiteFlag)
	securityContact, _ := cmd.Flags().GetString(securityContactFlag)
	details, _ := cmd.Flags().GetString(detailsFlag)
	identity, _ := cmd.Flags().GetString(identityFlag)
	rate, _ := cmd.Flags().GetString(commissionRateFlag)

	desc := &proto.Description{
		Moniker:         moniker,
		Identity:        identity,
		Website:         website,
		SecurityContact: securityContact,
		Details:         details,
	}

	if err := grpcClient.EditFinalityProvider(cmd.Context(), fpPk, desc, rate); err != nil {
		return fmt.Errorf("failed to edit finality provider %v err %w", fpPk.MarshalHex(), err)
	}

	return nil
}

func printRespJSON(resp interface{}) {
	jsonBytes, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		fmt.Println("unable to decode response: ", err)
		return
	}

	fmt.Printf("%s\n", jsonBytes)
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
