package daemon

import (
	"fmt"

	"github.com/cometbft/cometbft/crypto/tmhash"
	sdk "github.com/cosmos/cosmos-sdk/types"

	bbnparams "github.com/babylonlabs-io/babylon/app/params"
	bbn "github.com/babylonlabs-io/babylon/types"
	btcstktypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/urfave/cli"
)

func init() {
	bbnparams.SetAddressPrefixes()
}

// PoPExport the data for exporting the PoP.
// The PubKeyHex is the public key of the finality provider BTC key to load
// the private key and sign the AddressSiged.
type PoPExport struct {
	PubKeyHex      string `json:"pub_key_hex"`
	PoPHex         string `json:"pop_hex"`
	BabylonAddress string `json:"babylon_address"`
}

var ExportPoPCommand = cli.Command{
	Name:      "pop-export",
	Usage:     "Exports the Proof of Possession by signing over the finality provider's Babylon address with the EOTS private key.",
	UsageText: "pop-export [bbn-address]",
	Description: `Parse the address received as argument, hash it with
	sha256 and sign based on the EOTS key associated with the key-name or btc-pk flag.
	If the both flags are supplied, btc-pk takes priority. Use the generated signature
	to build a Proof of Possession and export it.`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "Path to the keyring directory",
			Value: config.DefaultEOTSDir,
		},
		cli.StringFlag{
			Name:  keyNameFlag,
			Usage: "The name of the key to load private key for signing",
		},
		cli.StringFlag{
			Name:  fpPkFlag,
			Usage: "The public key of the finality-provider to load private key for signing",
		},
		cli.StringFlag{
			Name:  passphraseFlag,
			Usage: "The passphrase used to decrypt the keyring",
			Value: defaultPassphrase,
		},
		cli.StringFlag{
			Name:  keyringBackendFlag,
			Usage: "The backend of the keyring",
			Value: defaultKeyringBackend,
		},
	},
	Action: ExportPoP,
}

func ExportPoP(ctx *cli.Context) error {
	keyName := ctx.String(keyNameFlag)
	fpPkStr := ctx.String(fpPkFlag)
	passphrase := ctx.String(passphraseFlag)
	keyringBackend := ctx.String(keyringBackendFlag)

	args := ctx.Args()
	bbnAddressStr := args.First()
	bbnAddr, err := sdk.AccAddressFromBech32(bbnAddressStr)
	if err != nil {
		return fmt.Errorf("invalid argument %s, please provide a valid bbn address as argument, err: %w", bbnAddressStr, err)
	}

	if len(fpPkStr) == 0 && len(keyName) == 0 {
		return fmt.Errorf("at least one of the flags: %s, %s needs to be informed", keyNameFlag, fpPkFlag)
	}

	homePath, err := getHomeFlag(ctx)
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

	eotsManager, err := eotsmanager.NewLocalEOTSManager(homePath, keyringBackend, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	hashOfMsgToSign := tmhash.Sum(bbnAddr.Bytes())
	btcSig, pubKey, err := singMsg(eotsManager, keyName, fpPkStr, passphrase, hashOfMsgToSign)
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
