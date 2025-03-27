package daemon

import (
	"fmt"
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
	f.Uint64(flagFromHeight, 0, "height until which to rollback the sign store")

	return cmd
}

func rollbackSignStore(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()

	eotsHomePath, err := getHomePath(cmd)
	if err != nil {
		return err
	}

	height, err := f.GetUint64(flagFromHeight)
	if err != nil {
		return err
	}

	if height == 0 {
		return fmt.Errorf("rollback-until-height flag is required")
	}

	cfg, err := config.LoadConfig(eotsHomePath)
	if err != nil {
		return fmt.Errorf("failed to load config at %s: %w", eotsHomePath, err)
	}

	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}

	es, err := store.NewEOTSStore(dbBackend)
	if err != nil {
		return err
	}

	defer es.Close()

	if err = es.DeleteSignRecordsFromHeight(height); err != nil {
		return fmt.Errorf("failed to delete sign store records: %w", err)
	}

	cmd.Printf("Successfully deleted sign store records until height %d\n", height)

	return nil
}
