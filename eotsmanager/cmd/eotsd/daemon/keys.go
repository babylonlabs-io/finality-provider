package daemon

import (
	"bytes"
	"fmt"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/babylonlabs-io/finality-provider/util"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

type KeyOutput struct {
	Name      string `json:"name" yaml:"name"`
	PubKeyHex string `json:"pub_key_hex" yaml:"pub_key_hex"`
}

func NewKeysCmd() *cobra.Command {
	keysCmd := keys.Commands()

	// Find the "add" subcommand
	addCmd := util.GetSubCommand(keysCmd, "add")
	if addCmd == nil {
		panic("failed to find keys add command")
	}

	// Override the original RunE function to run almost the same as
	// the sdk, but it allows empty hd path and allow to save the key
	// in the name mapping
	addCmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Create a buffer to intercept the key items
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		// Run the original command
		err := runAddCmdPrepare(cmd, args)
		if err != nil {
			return err
		}

		return saveKeyNameMapping(cmd, args)
	}

	return keysCmd
}

func saveKeyNameMapping(cmd *cobra.Command, args []string) error {
	clientCtx, err := client.GetClientQueryContext(cmd)
	if err != nil {
		return err
	}
	keyName := args[0]

	// Load configuration
	cfg, err := config.LoadConfig(clientCtx.HomeDir)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Setup logger
	logger, err := log.NewRootLoggerWithFile(config.LogFile(clientCtx.HomeDir), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to load the logger: %w", err)
	}

	// Get database backend
	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}
	defer dbBackend.Close()

	// Create EOTS manager
	eotsManager, err := eotsmanager.NewLocalEOTSManager(clientCtx.HomeDir, clientCtx.Keyring.Backend(), dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	// Get the public key for the newly added key
	eotsPk, err := eotsManager.LoadBIP340PubKeyFromKeyName(keyName)
	if err != nil {
		return fmt.Errorf("failed to get public key for key %s: %w", keyName, err)
	}

	// Save the public key to key name mapping
	if err := eotsManager.SaveEOTSKeyName(eotsPk.MustToBTCPK(), keyName); err != nil {
		return fmt.Errorf("failed to save key name mapping: %w", err)
	}

	return printKey(
		&KeyOutput{
			Name:      keyName,
			PubKeyHex: eotsPk.MarshalHex(),
		},
	)
}

func printKey(keyRecord *KeyOutput) error {
	out, err := yaml.Marshal(keyRecord)
	if err != nil {
		return err
	}

	fmt.Printf("\n%s\n", out)

	return nil
}
