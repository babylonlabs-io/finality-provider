package daemon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/types"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

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

func runCommandGetDaemonInfo(cmd *cobra.Command, args []string) error {
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
		Long: fmt.Sprintf(`
		Create a new finality provider object and store it in the finality provider database.
		It needs to have an operating EOTS manager available and running.

		If the flag %s is set, it will ask for the key record from the EOTS manager for the
		corresponding EOTS public key. If it is not set, it will create a new EOTS key`, fpEotsPkFlag),
		Example: fmt.Sprintf(`fpd create-finality-provider --daemon-address %s ...`, defaultFpdDaemonAddress),
		Args:    cobra.NoArgs,
		RunE:    fpcmd.RunEWithClientCtx(runCommandCreateFP),
	}

	f := cmd.Flags()
	f.String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	f.String(keyNameFlag, "", "The unique name of the finality provider key")
	f.String(sdkflags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")
	f.String(chainIdFlag, "", "The identifier of the consumer chain")
	f.String(passphraseFlag, "", "The pass phrase used to encrypt the keys")
	f.String(hdPathFlag, "", "The hd path used to derive the private key")
	f.String(commissionRateFlag, "0.05", "The commission rate for the finality provider, e.g., 0.05")
	f.String(monikerFlag, "", "A human-readable name for the finality provider")
	f.String(identityFlag, "", "An optional identity signature (ex. UPort or Keybase)")
	f.String(websiteFlag, "", "An optional website link")
	f.String(securityContactFlag, "", "An email for security contact")
	f.String(detailsFlag, "", "Other optional details")
	f.String(fpEotsPkFlag, "", "Optional hex EOTS public key, if not provided a new one will be created")

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

	client, cleanUp, err := dc.NewFinalityProviderServiceGRpcClient(daemonAddress)
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanUp(); err != nil {
			fmt.Printf("Failed to clean up grpc client: %v\n", err)
		}
	}()

	chainId, err := flags.GetString(chainIdFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", chainIdFlag, err)
	}

	passphrase, err := flags.GetString(passphraseFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", passphraseFlag, err)
	}

	hdPath, err := flags.GetString(hdPathFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", hdPathFlag, err)
	}

	eotsPkHex, err := flags.GetString(fpEotsPkFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", fpEotsPkFlag, err)
	}

	if len(eotsPkHex) > 0 {
		// if is set, validate before the creation request
		_, err := types.NewBIP340PubKeyFromHex(eotsPkHex)
		if err != nil {
			return fmt.Errorf("invalid eots public key %s: %w", eotsPkHex, err)
		}
	}

	info, err := client.CreateFinalityProvider(
		context.Background(),
		keyName,
		chainId,
		eotsPkHex,
		passphrase,
		hdPath,
		description,
		&commissionRate,
	)
	if err != nil {
		return err
	}

	printRespJSON(info.FinalityProvider)
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

func getDescriptionFromFlags(f *pflag.FlagSet) (desc stakingtypes.Description, err error) {
	// get information for description
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

func runCommandLsFP(cmd *cobra.Command, args []string) error {
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
	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(args[0])
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

// CommandRegisterFP returns the register-finality-provider command by connecting to the fpd daemon.
func CommandRegisterFP() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "register-finality-provider [fp-eots-pk-hex]",
		Aliases: []string{"rfp"},
		Short:   "Register a created finality provider to Babylon.",
		Example: fmt.Sprintf(`fpd register-finality-provider --daemon-address %s`, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(1),
		RunE:    runCommandRegisterFP,
	}
	f := cmd.Flags()
	f.String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	f.String(passphraseFlag, "", "The pass phrase used to encrypt the keys")
	return cmd
}

func runCommandRegisterFP(cmd *cobra.Command, args []string) error {
	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return err
	}

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

	passphrase, err := flags.GetString(passphraseFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", passphraseFlag, err)
	}

	res, err := client.RegisterFinalityProvider(context.Background(), fpPk, passphrase)
	if err != nil {
		return err
	}
	printRespJSON(res)

	return nil
}

// CommandAddFinalitySig returns the add-finality-sig command by connecting to the fpd daemon.
func CommandAddFinalitySig() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "add-finality-sig [fp-eots-pk-hex] [block-height]",
		Aliases: []string{"afs"},
		Short:   "Send a finality signature to the consumer chain. This command should only be used for presentation/testing purposes",
		Example: fmt.Sprintf(`fpd add-finality-sig --daemon-address %s`, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(2),
		RunE:    runCommandAddFinalitySig,
	}
	cmd.Flags().String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	cmd.Flags().String(appHashFlag, defaultAppHashStr, "The last commit hash of the chain block")
	return cmd
}

func runCommandAddFinalitySig(cmd *cobra.Command, args []string) error {
	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(args[0])
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

	res, err := client.AddFinalitySignature(context.Background(), fpPk.MarshalHex(), blkHeight, appHash)
	if err != nil {
		return err
	}
	printRespJSON(res)

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
		return "", fmt.Errorf("failed to load config from %s: %w", fpcfg.ConfigFile(homeDir), err)
	}

	keyName = cfg.BabylonConfig.Key
	if keyName == "" {
		return "", fmt.Errorf("the key in config is empty")
	}
	return keyName, nil
}
