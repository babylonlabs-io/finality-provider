package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"

	"github.com/babylonlabs-io/babylon/types"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	cryptokeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

type KeyOutputWithPubKeyHex struct {
	keys.KeyOutput
	PubKeyHex string `json:"pubkey_hex" yaml:"pubkey_hex"`
}

func NewKeysCmd() *cobra.Command {
	keysCmd := keys.Commands()

	// Find the "add" subcommand
	addCmd := util.GetSubCommand(keysCmd, "add")
	if addCmd == nil {
		panic("failed to find keys add command")
	}

	addCmd.Flags().String(rpcClientFlag, "", "The RPC address of a running eotsd to connect and save new key")

	// Override the original RunE function to run almost the same as
	// the sdk, but it allows empty hd path and allow to save the key
	// in the name mapping
	addCmd.RunE = func(cmd *cobra.Command, args []string) error {
		oldOut := cmd.OutOrStdout()

		// Create a buffer to intercept the key items
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		// Run the original command
		err := runAddCmdPrepare(cmd, args)
		if err != nil {
			return err
		}

		cmd.SetOut(oldOut)
		keyName := args[0]
		eotsPk, err := saveKeyNameMapping(cmd, keyName)
		if err != nil {
			return err
		}

		return printFromKey(cmd, keyName, eotsPk)
	}

	saveKeyOnPostRun(keysCmd, "import")
	saveKeyOnPostRun(keysCmd, "import-hex")

	return keysCmd
}

func saveKeyOnPostRun(cmd *cobra.Command, commandName string) {
	subCmd := util.GetSubCommand(cmd, commandName)
	if subCmd == nil {
		panic(fmt.Sprintf("failed to find keys %s command", commandName))
	}

	subCmd.Flags().String(rpcClientFlag, "", "The RPC address of a running eotsd to connect and save new key")

	subCmd.PostRunE = func(cmd *cobra.Command, args []string) error {
		keyName := args[0]
		_, err := saveKeyNameMapping(cmd, keyName)

		return err
	}
}

func saveKeyNameMapping(cmd *cobra.Command, keyName string) (*types.BIP340PubKey, error) {
	clientCtx, err := client.GetClientQueryContext(cmd)
	if err != nil {
		return nil, err
	}

	// Load configuration
	cfg, err := config.LoadConfig(clientCtx.HomeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	rpcListener, err := cmd.Flags().GetString(rpcClientFlag)
	if err != nil {
		return nil, err
	}

	if len(rpcListener) > 0 {
		client, err := eotsclient.NewEOTSManagerGRpcClient(rpcListener)
		if err != nil {
			return nil, err
		}

		kr, err := eotsmanager.InitKeyring(clientCtx.HomeDir, clientCtx.Keyring.Backend(), strings.NewReader(""))
		if err != nil {
			return nil, fmt.Errorf("failed to init keyring: %w", err)
		}

		eotsPk, err := eotsmanager.LoadBIP340PubKeyFromKeyName(kr, keyName)
		if err != nil {
			return nil, fmt.Errorf("failed to get public key for key %s: %w", keyName, err)
		}

		if err := client.SaveEOTSKeyName(eotsPk.MustToBTCPK(), keyName); err != nil {
			return nil, fmt.Errorf("failed to save key name mapping: %w", err)
		}

		return eotsPk, nil
	}

	// Setup logger
	logger, err := log.NewRootLoggerWithFile(config.LogFile(clientCtx.HomeDir), cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to load the logger: %w", err)
	}

	// Get database backend
	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to create db backend: %w", err)
	}
	defer dbBackend.Close()

	// Create EOTS manager
	eotsManager, err := eotsmanager.NewLocalEOTSManager(clientCtx.HomeDir, clientCtx.Keyring.Backend(), dbBackend, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	// Get the public key for the newly added key
	eotsPk, err := eotsManager.LoadBIP340PubKeyFromKeyName(keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to get public key for key %s: %w", keyName, err)
	}

	// Save the public key to key name mapping
	if err := eotsManager.SaveEOTSKeyName(eotsPk.MustToBTCPK(), keyName); err != nil {
		return nil, fmt.Errorf("failed to save key name mapping: %w", err)
	}

	return eotsPk, nil
}

// CommandPrintAllKeys prints all EOTS keys
func CommandPrintAllKeys() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "Print all EOTS key names and public keys mapping from database.",
		Example: `eotsd list --home=/path/to/cfg`,
		Args:    cobra.NoArgs,
		RunE:    runCommandPrintAllKeys,
	}

	cmd.Flags().String(flags.FlagHome, config.DefaultEOTSDir, "The path to the eotsd home directory")

	return cmd
}

func runCommandPrintAllKeys(cmd *cobra.Command, _ []string) error {
	eotsKeys, err := getAllEOTSKeys(cmd)
	if err != nil {
		return err
	}

	for keyName, key := range eotsKeys {
		pk, err := schnorr.ParsePubKey(key)
		if err != nil {
			return err
		}
		eotsPk := types.NewBIP340PubKeyFromBTCPK(pk)
		cmd.Printf("Key Name: %s, EOTS PK: %s\n", keyName, eotsPk.MarshalHex())
	}

	return nil
}

func getAllEOTSKeys(cmd *cobra.Command) (map[string][]byte, error) {
	homePath, err := getHomePath(cmd)
	if err != nil {
		return nil, err
	}

	// Load configuration
	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Setup logger
	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to load the logger: %w", err)
	}

	// Get database backend
	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to create db backend: %w", err)
	}
	defer dbBackend.Close()

	// Create EOTS manager
	eotsManager, err := eotsmanager.NewLocalEOTSManager(homePath, cfg.KeyringBackend, dbBackend, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	res, err := eotsManager.ListEOTSKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to get keys from db: %w", err)
	}

	return res, nil
}

func printFromKey(cmd *cobra.Command, keyName string, eotsPk *types.BIP340PubKey) error {
	clientCtx, err := client.GetClientQueryContext(cmd)
	if err != nil {
		return err
	}

	k, err := clientCtx.Keyring.Key(keyName)
	if err != nil {
		return fmt.Errorf("failed to get public get key %s: %w", keyName, err)
	}

	ctx := cmd.Context()
	mnemonic := ctx.Value(mnemonicCtxKey).(string) //nolint: forcetypeassert
	showMnemonic := ctx.Value(mnemonicShowCtxKey).(bool)

	return printCreatePubKeyHex(cmd, k, eotsPk, showMnemonic, mnemonic, clientCtx.OutputFormat)
}

func printCreatePubKeyHex(cmd *cobra.Command, k *cryptokeyring.Record, eotsPk *types.BIP340PubKey, showMnemonic bool, mnemonic, outputFormat string) error {
	out, err := keys.MkAccKeyOutput(k)
	if err != nil {
		return err
	}
	keyOutput := newKeyOutputWithPubKeyHex(out, eotsPk)

	switch outputFormat {
	case flags.OutputFormatText:
		cmd.PrintErrln()
		if err := printKeyringRecord(cmd.OutOrStdout(), keyOutput, outputFormat); err != nil {
			return err
		}

		// print mnemonic unless requested not to.
		if showMnemonic {
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "\n**Important** write this mnemonic phrase in a safe place.\nIt is the only way to recover your account if you ever forget your password.\n\n%s\n", mnemonic); err != nil {
				return fmt.Errorf("failed to print mnemonic: %s", err.Error())
			}
		}
	case flags.OutputFormatJSON:
		if showMnemonic {
			keyOutput.Mnemonic = mnemonic
		}

		jsonString, err := json.MarshalIndent(keyOutput, "", "  ")
		if err != nil {
			return err
		}

		cmd.Println(string(jsonString))

	default:
		return fmt.Errorf("invalid output format %s", outputFormat)
	}

	return nil
}

func newKeyOutputWithPubKeyHex(k keys.KeyOutput, eotsPk *types.BIP340PubKey) KeyOutputWithPubKeyHex {
	return KeyOutputWithPubKeyHex{
		KeyOutput: k,
		PubKeyHex: eotsPk.MarshalHex(),
	}
}

func printKeyringRecord(w io.Writer, ko KeyOutputWithPubKeyHex, output string) error {
	switch output {
	case flags.OutputFormatText:
		if err := printTextRecords(w, []KeyOutputWithPubKeyHex{ko}); err != nil {
			return err
		}

	case flags.OutputFormatJSON:
		out, err := json.Marshal(ko)
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintln(w, string(out)); err != nil {
			return err
		}
	}

	return nil
}

func printTextRecords(w io.Writer, kos []KeyOutputWithPubKeyHex) error {
	out, err := yaml.Marshal(&kos)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, string(out)); err != nil {
		return err
	}

	return nil
}
