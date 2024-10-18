package main

import (
	"fmt"

	"github.com/cometbft/cometbft/crypto/tmhash"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	bbnparams "github.com/babylonlabs-io/babylon/app/params"
	bbn "github.com/babylonlabs-io/babylon/types"
	btcstktypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/log"
)

func init() {
	bbnparams.SetAddressPrefixes()
}

// PoPExport the data for exporting the PoP.
// The PubKeyHex is the public key of the finality provider EOTS key to load
// the private key and sign the AddressSiged.
type PoPExport struct {
	PubKeyHex      string `json:"pub_key_hex"`
	PoPHex         string `json:"pop_hex"`
	BabylonAddress string `json:"babylon_address"`
}

func NewExportPoPCmd() *cobra.Command {
	exportPoPCmd := &cobra.Command{
		Use:   "pop-export <babylon-address>",
		Short: "Export the Proof of Possession for a finality provider",
		Long: `Export the Proof of Possession (PoP) by signing over the finality provider's Babylon address with the EOTS private key.

This command performs the following steps:
1. Parse the provided Babylon address
2. Hash the address using SHA256
3. Sign the hash using the EOTS key associated with either the key-name or eots-pk flag
4. Generate a Proof of Possession using the signature
5. Export the PoP along with relevant information

If both key-name and eots-pk flags are provided, eots-pk takes precedence.`,
		Args: cobra.ExactArgs(1),
		RunE: exportPoP,
	}

	exportPoPCmd.Flags().String(keyNameFlag, "", "Name of the key to load private key for signing (mutually exclusive with eots-pk)")
	exportPoPCmd.Flags().String(eotsPkFlag, "", "Public key of the finality provider to load private key for signing (mutually exclusive with key-name)")
	exportPoPCmd.Flags().String(passphraseFlag, "", "Passphrase used to decrypt the keyring")
	exportPoPCmd.Flags().String(keyringBackendFlag, defaultKeyringBackend, "Backend of the keyring (default: test)")
	exportPoPCmd.MarkFlagsMutuallyExclusive(keyNameFlag, eotsPkFlag)

	return exportPoPCmd
}

func exportPoP(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("please provide a valid Babylon address as argument")
	}

	// Extract flags
	flags, err := extractFlags(cmd)
	if err != nil {
		return err
	}

	// Parse Babylon address
	bbnAddr, err := sdk.AccAddressFromBech32(args[0])
	if err != nil {
		return fmt.Errorf("invalid Babylon address: %w", err)
	}

	// Setup environment
	homePath, err := getHomePath(cmd)
	if err != nil {
		return fmt.Errorf("failed to load home flag: %w", err)
	}

	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load config at %s: %w", homePath, err)
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to load the logger")
	}

	dbBackend, err := cfg.DatabaseConfig.GetDbBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}
	defer dbBackend.Close()

	eotsManager, err := eotsmanager.NewLocalEOTSManager(homePath, flags.keyringBackend, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	// Sign message and generate Pop
	hashOfMsgToSign := tmhash.Sum(bbnAddr.Bytes())
	btcSig, pubKey, err := singMsg(eotsManager, flags.keyName, flags.eotsPkStr, flags.passphrase, hashOfMsgToSign)
	if err != nil {
		return fmt.Errorf("failed to sign address %s: %w", bbnAddr.String(), err)
	}

	bip340Sig := bbn.NewBIP340SignatureFromBTCSig(btcSig)
	btcSigBz, err := bip340Sig.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal BTC Sig: %w", err)
	}

	pop := btcstktypes.ProofOfPossessionBTC{
		BtcSigType: btcstktypes.BTCSigType_BIP340,
		BtcSig:     btcSigBz,
	}

	// Export results
	popHex, err := pop.ToHexStr()
	if err != nil {
		return fmt.Errorf("failed to marshal pop to hex: %w", err)
	}

	printRespJSON(PoPExport{
		PubKeyHex:      pubKey.MarshalHex(),
		PoPHex:         popHex,
		BabylonAddress: bbnAddr.String(),
	})

	return nil
}

func extractFlags(cmd *cobra.Command) (flags struct {
	keyName        string
	eotsPkStr      string
	passphrase     string
	keyringBackend string
}, err error) {
	flags.keyName, err = cmd.Flags().GetString(keyNameFlag)
	if err != nil {
		return flags, fmt.Errorf("failed to get key name flag: %w", err)
	}

	flags.eotsPkStr, err = cmd.Flags().GetString(eotsPkFlag)
	if err != nil {
		return flags, fmt.Errorf("failed to get the eots public key flag: %w", err)
	}

	flags.passphrase, err = cmd.Flags().GetString(passphraseFlag)
	if err != nil {
		return flags, fmt.Errorf("failed to get passphrase flag: %w", err)
	}

	flags.keyringBackend, err = cmd.Flags().GetString(keyringBackendFlag)
	if err != nil {
		return flags, fmt.Errorf("failed to get keyring backend flag: %w", err)
	}

	if flags.eotsPkStr == "" && flags.keyName == "" {
		return flags, fmt.Errorf("at least one of the flags: %s, %s needs to be provided", keyNameFlag, eotsPkFlag)
	}

	return flags, nil
}
