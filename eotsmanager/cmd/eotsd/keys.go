package main

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/log"
)

func NewKeysCmd() *cobra.Command {
	keysCmd := keys.Commands()

	// Find the "add" subcommand
	addCmd := findSubCommand(keysCmd, "add")
	if addCmd == nil {
		panic("failed to find keys add command")
	}

	// Wrap the original RunE function
	originalRunE := addCmd.RunE
	addCmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Run the original command
		if err := originalRunE(cmd, args); err != nil {
			return err
		}

		// Add save the key name and public key mapping
		return saveKeyNameMapping(cmd, args)
	}

	return keysCmd
}

func findSubCommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, subCmd := range cmd.Commands() {
		if subCmd.Name() == name {
			return subCmd
		}
	}
	return nil
}

func saveKeyNameMapping(cmd *cobra.Command, args []string) error {
	clientCtx := client.GetClientContextFromCmd(cmd)
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
	dbBackend, err := cfg.DatabaseConfig.GetDbBackend()
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
		return fmt.Errorf("failed to get public key for key: %s", keyName)
	}

	// Save the public key to key name mapping
	if err := eotsManager.SaveEOTSKeyName(eotsPk.MustToBTCPK(), keyName); err != nil {
		return fmt.Errorf("failed to save key name mapping: %w", err)
	}

	fmt.Printf("Successfully saved mapping for key: %s\n", keyName)
	return nil
}
