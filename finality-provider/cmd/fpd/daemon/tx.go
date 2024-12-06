package daemon

import (
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	btcstakingcli "github.com/babylonlabs-io/babylon/x/btcstaking/client/cli"
	incentivecli "github.com/babylonlabs-io/babylon/x/incentive/client/cli"

	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authclient "github.com/cosmos/cosmos-sdk/x/auth/client"
	authcli "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
)

// CommandTxs returns the transaction commands for finality provider related msgs.
func CommandTxs() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "tx",
		Short:                      "transactions subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		authcli.GetSignCommand(),
		btcstakingcli.NewCreateFinalityProviderCmd(),
		NewValidateSignedFinalityProviderCmd(),
		incentivecli.NewWithdrawRewardCmd(),
		incentivecli.NewSetWithdrawAddressCmd(),
	)

	return cmd
}

// NewValidateSignedFinalityProviderCmd returns the command line for
// tx validate-signed-finality-provider
func NewValidateSignedFinalityProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate-signed-finality-provider [file_path_signed_msg]",
		Args:  cobra.ExactArgs(1),
		Short: "Validates the signed MsgCreateFinalityProvider",
		Long: strings.TrimSpace(`
			Loads the signed MsgCreateFinalityProvider and checks if the basic
			information is satisfied and the Proof of Possession is valid against the
			signer of the msg and the finality provider's BTC public key
		`),
		Example: strings.TrimSpace(
			`fdp tx validate-signed-finality-provider ./path/to/signed-msg.json`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			stdTx, err := authclient.ReadTxFromFile(ctx, args[0])
			if err != nil {
				return err
			}

			msgsV2, err := stdTx.GetMsgsV2()
			if err != nil {
				return err
			}

			msgs := stdTx.GetMsgs()
			if len(msgs) == 0 {
				return fmt.Errorf("invalid msg, there is no msg inside %s file", args[0])
			}

			for i, sdkMsg := range msgs {
				msgV2 := msgsV2[i]
				msg, ok := sdkMsg.(*btcstakingtypes.MsgCreateFinalityProvider)
				if !ok {
					return fmt.Errorf("unable to parse %+v to MsgCreateFinalityProvider", msg)
				}

				if err := msg.ValidateBasic(); err != nil {
					return fmt.Errorf("error validating basic msg: %w", err)
				}

				signers, err := ctx.Codec.GetMsgV2Signers(msgV2)
				if err != nil {
					return fmt.Errorf("failed to get signers from msg %+v: %w", msg, err)
				}

				if len(signers) == 0 {
					return fmt.Errorf("no signer at msg %+v", msgV2)
				}

				signerAddrStr, err := ctx.Codec.InterfaceRegistry().SigningContext().AddressCodec().BytesToString(signers[0])
				if err != nil {
					return err
				}

				signerBbnAddr, err := sdk.AccAddressFromBech32(signerAddrStr)
				if err != nil {
					return fmt.Errorf("invalid signer address %s, please sign with a valid bbn address, err: %w", signerAddrStr, err)
				}

				if !strings.EqualFold(msg.Addr, signerAddrStr) {
					return fmt.Errorf("signer address: %s is different from finality provider address: %s", signerAddrStr, msg.Addr)
				}

				if err := msg.Pop.VerifyBIP340(signerBbnAddr, msg.BtcPk); err != nil {
					return fmt.Errorf("invalid Proof of Possession with signer %s: %w", signerBbnAddr.String(), err)
				}
			}

			_, err = cmd.OutOrStdout().Write([]byte("The signed MsgCreateFinalityProvider is valid"))
			return err
		},
	}

	return cmd
}
