package daemon

import (
	"context"
	"encoding/hex"
	"fmt"

	"cosmossdk.io/math"
	fpcmd "github.com/babylonlabs-io/finality-provider/finality-provider/cmd"
	fpcfg "github.com/babylonlabs-io/finality-provider/finality-provider/config"
	dc "github.com/babylonlabs-io/finality-provider/finality-provider/service/client"
	"github.com/cosmos/cosmos-sdk/client"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/cobra"

	bbn "github.com/babylonlabs-io/babylon/types"
	btcstktypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
)

// FinalityProviderSigned wraps the finality provider by adding the
// signature signed by the finality provider's Babylon key in hex
type FinalityProviderSigned struct {
	btcstktypes.FinalityProvider
	// FpSigHex is the finality provider cosmos sdk chain key
	// can be verified with the pub key in btcstktypes.FinalityProvider.BabylonPk
	FpSigHex string `json:"fp_sig_hex"`
}

// CommandExportFP returns the export-finality-provider command by loading the fp and export the data.
func CommandExportFP() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "export-finality-provider [fp-eots-pk-hex]",
		Aliases: []string{"exfp"},
		Short:   "It exports the finality provider by the given EOTS public key.",
		Example: fmt.Sprintf(`fpd export-finality-provider --daemon-address %s`, defaultFpdDaemonAddress),
		Args:    cobra.NoArgs,
		RunE:    fpcmd.RunEWithClientCtx(runCommandExportFP),
	}

	f := cmd.Flags()
	f.String(fpdDaemonAddressFlag, defaultFpdDaemonAddress, "The RPC server address of fpd")
	f.Bool(signedFlag, false,
		`Specify if the exported finality provider information should be signed,
			if true, it will sign using the flag key-name, if not set it will load from the
			babylon key on config.`,
	)
	f.String(keyNameFlag, "", "The unique name of the finality provider key")
	f.String(sdkflags.FlagHome, fpcfg.DefaultFpdDir, "The application home directory")
	f.String(passphraseFlag, "", "The pass phrase used to encrypt the keys")
	f.String(hdPathFlag, "", "The hd path used to derive the private key")

	return cmd
}

func runCommandExportFP(ctx client.Context, cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	daemonAddress, err := flags.GetString(fpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", fpdDaemonAddressFlag, err)
	}

	client, cleanUp, err := dc.NewFinalityProviderServiceGRpcClient(daemonAddress)
	if err != nil {
		return fmt.Errorf("failled to connect to daemon addr %s: %w", daemonAddress, err)
	}
	defer cleanUp()

	fpBtcPkHex := args[0]
	fpPk, err := bbn.NewBIP340PubKeyFromHex(fpBtcPkHex)
	if err != nil {
		return fmt.Errorf("invalid fp btc pk hex %s: %w", fpBtcPkHex, err)
	}

	fpInfoResp, err := client.QueryFinalityProviderInfo(context.Background(), fpPk)
	if err != nil {
		return fmt.Errorf("failed to query fp info from %s: %w", fpBtcPkHex, err)
	}

	fpInfo := fpInfoResp.FinalityProvider
	comm, err := math.LegacyNewDecFromStr(fpInfo.Commission)
	if err != nil {
		return fmt.Errorf("failed to parse fp commission %s: %w", fpInfo.Commission, err)
	}

	desc := fpInfo.Description
	fp := btcstktypes.FinalityProvider{
		Addr: fpInfo.FpAddr,
		Description: &types.Description{
			Moniker:         desc.Moniker,
			Identity:        desc.Identity,
			Website:         desc.Website,
			SecurityContact: desc.SecurityContact,
			Details:         desc.Details,
		},
		Commission: &comm,
		BtcPk:      fpPk,
		Pop:        nil, // TODO: fill PoP?
	}

	signed, err := flags.GetBool(signedFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", signedFlag, err)
	}
	if !signed {
		printRespJSON(fp)
		return nil
	}

	keyName, err := loadKeyName(ctx.HomeDir, cmd)
	if err != nil {
		return fmt.Errorf("not able to load key name: %w", err)
	}

	// sign the finality provider data.
	fpbz, err := fp.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal finality provider %+v: %w", fp, err)
	}

	passphrase, err := flags.GetString(passphraseFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", passphraseFlag, err)
	}

	hdPath, err := flags.GetString(hdPathFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", hdPathFlag, err)
	}

	resp, err := client.SignMessageFromChainKey(
		context.Background(),
		keyName,
		passphrase,
		hdPath,
		fpbz,
	)
	if err != nil {
		return fmt.Errorf("failed to sign finality provider: %w", err)
	}

	printRespJSON(FinalityProviderSigned{
		FinalityProvider: fp,
		FpSigHex:         hex.EncodeToString(resp.Signature),
	})
	return nil
}
