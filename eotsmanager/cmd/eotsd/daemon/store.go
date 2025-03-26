package daemon

import (
	"fmt"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/store"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"

	"github.com/spf13/cobra"
)

const (
	flagFromHeight = "rollback-until-height"
)

func NewSignStoreRollbackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unsafe-rollback",
		Short: "Unsafe! Rollback the sign store until a certain height",
		Long: `Rollback the sign store until a certain height. Records from sign store will be deleted, offering
				no protection on slashing for deleted records!`,
		RunE: rollbackSignStore,
	}

	f := cmd.Flags()

	f.String(sdkflags.FlagHome, config.DefaultEOTSDir, "EOTS home directory")
	f.String(eotsPkFlag, "", "EOTS public key of the finality-provider")
	f.String(flagChainID, "", "chain ID to remove the signature for that chain")
	f.Uint64(flagFromHeight, 0, "height until which to rollback the sign store")

	return cmd
}

func rollbackSignStore(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()

	eotsHomePath, err := getHomePath(cmd)
	if err != nil {
		return fmt.Errorf("failed to load home flag: %w", err)
	}

	eotsFpPubKeyStr, err := f.GetString(eotsPkFlag)
	if err != nil {
		return err
	}

	fromHeight, err := f.GetUint64(flagFromHeight)
	if err != nil {
		return err
	}
	if fromHeight == 0 {
		return fmt.Errorf("rollback-until-height flag is required")
	}
	cfg, err := config.LoadConfig(eotsHomePath)
	if err != nil {
		return fmt.Errorf("failed to load config at %s: %w", eotsHomePath, err)
	}

	chainID, err := f.GetString(flagChainID)
	if err != nil {
		return err
	}
	if len(chainID) == 0 {
		return fmt.Errorf("flag %s is required", flagChainID)
	}

	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	es, err := store.NewEOTSStore(dbBackend)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	defer es.Close()

	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(eotsFpPubKeyStr)
	if err != nil {
		return fmt.Errorf("invalid finality-provider public key %s: %w", eotsFpPubKeyStr, err)
	}

	bzChainID := []byte(chainID)
	bzEotsPk := fpPk.MustMarshal()

	err = es.DeleteSignRecordsFromHeight(bzEotsPk, bzChainID, fromHeight)
	if err != nil {
		return fmt.Errorf("failed to delete sign store records: %w", err)
	}

	cmd.Printf("Successfully deleted sign store records until height %d\n", fromHeight)

	return nil
}
