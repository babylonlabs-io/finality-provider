package daemon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/babylonlabs-io/babylon/v3/types"
	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	dc "github.com/babylonlabs-io/finality-provider/finality-provider/service/client"
	"github.com/cometbft/cometbft/crypto/tmhash"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
)

var (
	defaultFpdDaemonAddress = "127.0.0.1:" + strconv.Itoa(fpcfg.DefaultRPCPort)
)

func AddDaemonCommands(rootCmd *cobra.Command) {
	rootCmd.AddCommand(
		CommandKeys(),
		CommandGetDaemonInfo(),
		CommandLsFP(),
		CommandInfoFP(),
		CommandAddFinalitySig(),
		CommandUnjailFP(),
		CommandEditFinalityDescription(),
		CommandUnsafePruneMerkleProof(),
	)
}

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
	cmd.Flags().String(appHashFlag, "", "The last commit hash of the chain block")
	cmd.Flags().Bool(checkDoubleSignFlag, true, "If 'true', uses anti-slashing protection when doing EOTS sign")

	if err := cmd.MarkFlagRequired(fpdDaemonAddressFlag); err != nil {
		panic(err)
	}

	if err := cmd.MarkFlagRequired(appHashFlag); err != nil {
		panic(err)
	}

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

	if len(appHashHex) == 0 {
		return fmt.Errorf("app hash is required")
	}

	appHash, err := hex.DecodeString(appHashHex)
	if err != nil {
		return fmt.Errorf("failed to decode app hash: %w", err)
	}

	if len(appHash) != tmhash.Size {
		return fmt.Errorf("invalid app hash length: got %d bytes, expected 32 bytes", len(appHash))
	}

	checkDoubleSign, err := flags.GetBool(checkDoubleSignFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", checkDoubleSignFlag, err)
	}

	client, cleanUp, err := dc.NewFinalityProviderServiceGRpcClient(daemonAddress)
	if err != nil {
		return fmt.Errorf("failed to create grpc client: %w", err)
	}
	defer func() {
		if err := cleanUp(); err != nil {
			fmt.Printf("Failed to clean up grpc client: %v\n", err)
		}
	}()

	res, err := client.AddFinalitySignature(cmd.Context(), fpPk.MarshalHex(), blkHeight, appHash, checkDoubleSign)
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

// CommandUnsafePruneMerkleProof prunes merkle proof
func CommandUnsafePruneMerkleProof() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "unsafe-prune-merkle-proof [eots_pk]",
		Aliases: []string{"rmp"},
		Short:   "Prunes merkle proofs up to the specified target height",
		Long: strings.TrimSpace(`This command will prune all merkle proof up to the target height. The 
operator of this command should ensure that finality provider has voted, or doesn't have voting power up to the target height.'
`),
		Example: fmt.Sprintf(`fpd unsafe-prune-merkle-proof [eots_pk] --daemon-address %s`, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(1),
		RunE:    runCommandUnsafePruneMerkleProof,
	}
	cmd.Flags().String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	cmd.Flags().String(chainIDFlag, "", "The identifier of the consumer chain")
	cmd.Flags().Uint64(upToHeight, 0, "Target height to prune merkle proofs")

	if err := cmd.MarkFlagRequired(chainIDFlag); err != nil {
		panic(err)
	}

	if err := cmd.MarkFlagRequired(upToHeight); err != nil {
		panic(err)
	}

	return cmd
}

func runCommandUnsafePruneMerkleProof(cmd *cobra.Command, args []string) error {
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

	chainID, _ := cmd.Flags().GetString(chainIDFlag)
	targetHeight, _ := cmd.Flags().GetUint64(upToHeight)

	if err := grpcClient.UnsafeRemoveMerkleProof(cmd.Context(), fpPk, chainID, targetHeight); err != nil {
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

func loadKeyName(cfg *fpcfg.Config, cmd *cobra.Command) (string, error) {
	keyName, err := cmd.Flags().GetString(keyNameFlag)
	if err != nil {
		return "", fmt.Errorf("failed to read flag %s: %w", keyNameFlag, err)
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
