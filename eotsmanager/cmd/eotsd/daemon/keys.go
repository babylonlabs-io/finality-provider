package daemon

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/client/input"
	"github.com/cosmos/go-bip39"
	"github.com/urfave/cli"

	"github.com/babylonchain/finality-provider/eotsmanager"
	"github.com/babylonchain/finality-provider/eotsmanager/config"
	"github.com/babylonchain/finality-provider/log"
	bbntypes "github.com/babylonlabs-io/babylon/types"
)

type KeyOutput struct {
	Name      string `json:"name" yaml:"name"`
	PubKeyHex string `json:"pub_key_hex" yaml:"pub_key_hex"`
	Mnemonic  string `json:"mnemonic,omitempty" yaml:"mnemonic"`
}

var KeysCommands = []cli.Command{
	{
		Name:     "keys",
		Usage:    "Command sets of managing keys for interacting with BTC eots keys.",
		Category: "Key management",
		Subcommands: []cli.Command{
			AddKeyCmd,
		},
	},
}

var AddKeyCmd = cli.Command{
	Name:  "add",
	Usage: "Add a key to the EOTS manager keyring.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "Path to the keyring directory",
			Value: config.DefaultEOTSDir,
		},
		cli.StringFlag{
			Name:     keyNameFlag,
			Usage:    "The name of the key to be created",
			Required: true,
		},
		cli.StringFlag{
			Name:  passphraseFlag,
			Usage: "The pass phrase used to encrypt the keys",
			Value: defaultPassphrase,
		},
		cli.StringFlag{
			Name:  hdPathFlag,
			Usage: "The hd path used to derive the private key",
			Value: defaultHdPath,
		},
		cli.StringFlag{
			Name:  keyringBackendFlag,
			Usage: "The backend of the keyring",
			Value: defaultKeyringBackend,
		},
		cli.BoolFlag{
			Name: recoverFlag,
			Usage: `Will need to provide a seed phrase to recover
	the existing key instead of creating`,
		},
	},
	Action: addKey,
}

func addKey(ctx *cli.Context) error {
	keyName := ctx.String(keyNameFlag)
	keyringBackend := ctx.String(keyringBackendFlag)

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

	eotsPk, mnemonic, err := createKey(ctx, eotsManager, keyName)
	if err != nil {
		return fmt.Errorf("failed to create key: %w", err)
	}

	printRespJSONKeys(
		KeyOutput{
			Name:      keyName,
			PubKeyHex: eotsPk.MarshalHex(),
			Mnemonic:  mnemonic,
		},
	)
	return nil
}

// createKey checks if recover flag is set to create a key from mnemonic or if not set, randomly creates it.
func createKey(
	ctx *cli.Context,
	eotsManager *eotsmanager.LocalEOTSManager,
	keyName string,
) (eotsPk *bbntypes.BIP340PubKey, mnemonic string, err error) {
	passphrase := ctx.String(passphraseFlag)
	hdPath := ctx.String(hdPathFlag)

	mnemonic, err = getMnemonic(ctx)
	if err != nil {
		return nil, "", err
	}

	eotsPk, err = eotsManager.CreateKeyWithMnemonic(keyName, passphrase, hdPath, mnemonic)
	if err != nil {
		return nil, "", err
	}
	return eotsPk, mnemonic, nil
}

func getMnemonic(ctx *cli.Context) (string, error) {
	if ctx.Bool(recoverFlag) {
		reader := bufio.NewReader(os.Stdin)
		mnemonic, err := input.GetString("Enter your mnemonic", reader)
		if err != nil {
			return "", fmt.Errorf("failed to read mnemonic from stdin: %w", err)
		}
		if !bip39.IsMnemonicValid(mnemonic) {
			return "", errors.New("invalid mnemonic")
		}

		return mnemonic, nil
	}

	return eotsmanager.NewMnemonic()
}

func printRespJSONKeys(resp interface{}) {
	jsonBytes, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		fmt.Println("unable to decode response: ", err)
		return
	}

	fmt.Printf("New key for the BTC chain is created "+
		"(mnemonic should be kept in a safe place for recovery):\n%s\n", jsonBytes)
}
