package common

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/babylonlabs-io/babylon/v3/types"
	clientctx "github.com/babylonlabs-io/finality-provider/finality-provider/cmd/fpd/clientctx"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	"github.com/babylonlabs-io/finality-provider/finality-provider/proto"
	dc "github.com/babylonlabs-io/finality-provider/finality-provider/service/client"
	fptypes "github.com/babylonlabs-io/finality-provider/types"
	"github.com/cometbft/cometbft/crypto/tmhash"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
)

var (
	defaultFpdDaemonAddress = "127.0.0.1:" + strconv.Itoa(fpcfg.DefaultRPCPort)
)

// AddCommonCommands adds all the common subcommands to the given command.
// These commands are generic to {Babylon, Cosmos BSN, rollup BSN} finality providers
func AddCommonCommands(cmd *cobra.Command, binaryName string) {
	cmd.AddCommand(
		CommandGetDaemonInfo(binaryName),
		CommandUnjailFP(binaryName),
		CommandLsFP(binaryName),
		CommandInfoFP(binaryName),
		CommandAddFinalitySig(binaryName),
		CommandEditFinalityDescription(binaryName),
		CommandUnsafePruneMerkleProof(binaryName),
	)
}

// CommandGetDaemonInfo returns the get-info command by connecting to the fpd daemon.
func CommandGetDaemonInfo(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "get-info",
		Aliases: []string{"gi"},
		Short:   "Get information of the running fpd daemon.",
		Example: fmt.Sprintf(`%s get-info --daemon-address %s`, binaryName, defaultFpdDaemonAddress),
		Args:    cobra.NoArgs,
		RunE:    runCommandGetDaemonInfo,
	}
	cmd.Flags().String(FpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")

	return cmd
}

func runCommandGetDaemonInfo(cmd *cobra.Command, _ []string) error {
	daemonAddress, err := cmd.Flags().GetString(FpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", FpdDaemonAddressFlag, err)
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

	info, err := client.GetInfo(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get daemon info: %w", err)
	}

	fptypes.PrintRespJSON(cmd, info)

	return nil
}

// CommandUnjailFP returns the unjail-finality-provider command by connecting to the fpd daemon.
func CommandUnjailFP(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "unjail-finality-provider",
		Aliases: []string{"ufp"},
		Short:   "Unjail the given finality provider.",
		Example: fmt.Sprintf(`%s unjail-finality-provider [eots-pk] --daemon-address %s ...`, binaryName, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(1),
		RunE:    clientctx.RunEWithClientCtx(runCommandUnjailFP),
	}

	f := cmd.Flags()
	f.String(FpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")

	return cmd
}

func runCommandUnjailFP(_ client.Context, cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	daemonAddress, err := flags.GetString(FpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", FpdDaemonAddressFlag, err)
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

	_, err = client.UnjailFinalityProvider(cmd.Context(), args[0])
	if err != nil {
		return fmt.Errorf("failed to unjail finality provider: %w", err)
	}

	return nil
}

// CommandLsFP returns the list-finality-providers command by connecting to the fpd daemon.
func CommandLsFP(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "list-finality-providers",
		Aliases: []string{"ls"},
		Short:   "List finality providers stored in the database.",
		Example: fmt.Sprintf(`%s list-finality-providers --daemon-address %s`, binaryName, defaultFpdDaemonAddress),
		Args:    cobra.NoArgs,
		RunE:    runCommandLsFP,
	}
	cmd.Flags().String(FpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")

	return cmd
}

func runCommandLsFP(cmd *cobra.Command, _ []string) error {
	daemonAddress, err := cmd.Flags().GetString(FpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", FpdDaemonAddressFlag, err)
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

	resp, err := client.QueryFinalityProviderList(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to query finality provider list: %w", err)
	}
	fptypes.PrintRespJSON(cmd, resp)

	return nil
}

// CommandInfoFP returns the finality-provider-info command by connecting to the fpd daemon.
func CommandInfoFP(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "finality-provider-info [fp-eots-pk-hex]",
		Aliases: []string{"fpi"},
		Short:   "List finality providers stored in the database.",
		Example: fmt.Sprintf(`%s finality-provider-info --daemon-address %s`, binaryName, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(1),
		RunE:    runCommandInfoFP,
	}
	cmd.Flags().String(FpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")

	return cmd
}

func runCommandInfoFP(cmd *cobra.Command, args []string) error {
	fpPk, err := types.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", FpdDaemonAddressFlag, err)
	}

	daemonAddress, err := cmd.Flags().GetString(FpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", FpdDaemonAddressFlag, err)
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

	resp, err := client.QueryFinalityProviderInfo(cmd.Context(), fpPk)
	if err != nil {
		return fmt.Errorf("failed to query finality provider info: %w", err)
	}
	fptypes.PrintRespJSON(cmd, resp)

	return nil
}

// CommandAddFinalitySig returns the add-finality-sig command by connecting to the fpd daemon.
func CommandAddFinalitySig(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "unsafe-add-finality-sig [fp-eots-pk-hex] [block-height]",
		Aliases: []string{"unsafe-afs"},
		Short:   "[UNSAFE] Send a finality signature to the consumer chain.",
		Long:    "[UNSAFE] Send a finality signature to the consumer chain. This command should only be used for presentation/testing purposes",
		Example: fmt.Sprintf(`%s unsafe-add-finality-sig [fp-eots-pk-hex] [block-height] --daemon-address %s`, binaryName, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(2),
		RunE:    runCommandAddFinalitySig,
	}
	cmd.Flags().String(FpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	cmd.Flags().String(AppHashFlag, "", "The last commit hash of the chain block")
	cmd.Flags().Bool(CheckDoubleSignFlag, true, "If 'true', uses anti-slashing protection when doing EOTS sign")

	if err := cmd.MarkFlagRequired(FpdDaemonAddressFlag); err != nil {
		panic(err)
	}

	if err := cmd.MarkFlagRequired(AppHashFlag); err != nil {
		panic(err)
	}

	return cmd
}

func runCommandAddFinalitySig(cmd *cobra.Command, args []string) error {
	fpPk, err := types.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return fmt.Errorf("failed to parse BIP340 public key from hex: %w", err)
	}
	blkHeight, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse block height: %w", err)
	}

	flags := cmd.Flags()
	daemonAddress, err := flags.GetString(FpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", FpdDaemonAddressFlag, err)
	}

	appHashHex, err := flags.GetString(AppHashFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", AppHashFlag, err)
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

	checkDoubleSign, err := flags.GetBool(CheckDoubleSignFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", CheckDoubleSignFlag, err)
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
		return fmt.Errorf("failed to add finality signature: %w", err)
	}
	fptypes.PrintRespJSON(cmd, res)

	return nil
}

// CommandEditFinalityDescription edits description of finality provider
func CommandEditFinalityDescription(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "edit-finality-provider [eots_pk]",
		Aliases: []string{"efp"},
		Short:   "Edit finality provider data without resetting unchanged fields",
		Long: "Edit the details of a finality provider using the specified EOTS public key. " +
			"\nThe provided [eots_pk] must correspond to the Babylon address controlled by the key specified in fpd.conf. " +
			"\nIf one or more optional flags are passed (such as --moniker, --website, etc.), " +
			"the corresponding values are updated, while unchanged fields retain their current values from the Babylon Node.",
		Example: fmt.Sprintf(`%s edit-finality-provider [eots_pk] --daemon-address %s --moniker "new-moniker"`, binaryName, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(1),
		RunE:    runCommandEditFinalityDescription,
	}
	cmd.Flags().String(FpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	cmd.Flags().String(MonikerFlag, "", "The finality provider's (optional) moniker")
	cmd.Flags().String(WebsiteFlag, "", "The finality provider's (optional) website")
	cmd.Flags().String(SecurityContactFlag, "", "The finality provider's (optional) security contact email")
	cmd.Flags().String(DetailsFlag, "", "The finality provider's (optional) details")
	cmd.Flags().String(IdentityFlag, "", "The (optional) identity signature (ex. UPort or Keybase)")
	cmd.Flags().String(CommissionRateFlag, "", "The (optional) commission rate percentage (ex. 0.2)")

	return cmd
}

func runCommandEditFinalityDescription(cmd *cobra.Command, args []string) error {
	fpPk, err := types.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return fmt.Errorf("failed to parse BIP340 public key from hex: %w", err)
	}

	flags := cmd.Flags()
	daemonAddress, err := flags.GetString(FpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", FpdDaemonAddressFlag, err)
	}

	grpcClient, cleanUp, err := dc.NewFinalityProviderServiceGRpcClient(daemonAddress)
	if err != nil {
		return fmt.Errorf("failed to create grpc client: %w", err)
	}
	defer func() {
		if err := cleanUp(); err != nil {
			fmt.Printf("Failed to clean up grpc client: %v\n", err)
		}
	}()

	moniker, _ := cmd.Flags().GetString(MonikerFlag)
	website, _ := cmd.Flags().GetString(WebsiteFlag)
	securityContact, _ := cmd.Flags().GetString(SecurityContactFlag)
	details, _ := cmd.Flags().GetString(DetailsFlag)
	identity, _ := cmd.Flags().GetString(IdentityFlag)
	rate, _ := cmd.Flags().GetString(CommissionRateFlag)

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
func CommandUnsafePruneMerkleProof(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "unsafe-prune-merkle-proof [eots_pk]",
		Aliases: []string{"rmp"},
		Short:   "Prunes merkle proofs up to the specified target height",
		Long: strings.TrimSpace(`This command will prune all merkle proof up to the target height. The 
operator of this command should ensure that finality provider has voted, or doesn't have voting power up to the target height.'
`),
		Example: fmt.Sprintf(`%s unsafe-prune-merkle-proof [eots_pk] --daemon-address %s`, binaryName, defaultFpdDaemonAddress),
		Args:    cobra.ExactArgs(1),
		RunE:    runCommandUnsafePruneMerkleProof,
	}
	cmd.Flags().String(FpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	cmd.Flags().String(ChainIDFlag, "", "The identifier of the consumer chain")
	cmd.Flags().Uint64(UpToHeightFlag, 0, "Target height to prune merkle proofs")

	if err := cmd.MarkFlagRequired(ChainIDFlag); err != nil {
		panic(err)
	}

	if err := cmd.MarkFlagRequired(UpToHeightFlag); err != nil {
		panic(err)
	}

	return cmd
}

func runCommandUnsafePruneMerkleProof(cmd *cobra.Command, args []string) error {
	fpPk, err := types.NewBIP340PubKeyFromHex(args[0])
	if err != nil {
		return fmt.Errorf("failed to parse BIP340 public key from hex: %w", err)
	}

	flags := cmd.Flags()
	daemonAddress, err := flags.GetString(FpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", FpdDaemonAddressFlag, err)
	}

	grpcClient, cleanUp, err := dc.NewFinalityProviderServiceGRpcClient(daemonAddress)
	if err != nil {
		return fmt.Errorf("failed to create grpc client: %w", err)
	}
	defer func() {
		if err := cleanUp(); err != nil {
			fmt.Printf("Failed to clean up grpc client: %v\n", err)
		}
	}()

	chainID, _ := cmd.Flags().GetString(ChainIDFlag)
	targetHeight, _ := cmd.Flags().GetUint64(UpToHeightFlag)

	if err := grpcClient.UnsafeRemoveMerkleProof(cmd.Context(), fpPk, chainID, targetHeight); err != nil {
		return fmt.Errorf("failed to remove merkle proof %v err %w", fpPk.MarshalHex(), err)
	}

	return nil
}
