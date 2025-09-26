package daemon

import (
	"fmt"

	bbntypes "github.com/babylonlabs-io/babylon/v4/types"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/store"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
)

const (
	flagFromHeight = "rollback-until-height"
	flagChainID    = "chain-id"
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
	f.String(flagChainID, "", "The identifier of the consumer chain")
	f.Uint64(flagFromHeight, 0, "height until which to rollback the sign store")

	if err := cmd.MarkFlagRequired(sdkflags.FlagHome); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(eotsPkFlag); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(flagChainID); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(flagFromHeight); err != nil {
		panic(err)
	}

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
		return fmt.Errorf("failed to get rollback-until-height flag: %w", err)
	}

	if height == 0 {
		return fmt.Errorf("rollback-until-height flag is required")
	}

	eotsFpPubKeyStr, err := f.GetString(eotsPkFlag)
	if err != nil {
		return fmt.Errorf("failed to get eots pk flag: %w", err)
	}

	chainID, err := f.GetString(flagChainID)
	if err != nil {
		return fmt.Errorf("failed to get chain-id flag: %w", err)
	}
	if len(chainID) == 0 {
		return fmt.Errorf("flag %s is required", flagChainID)
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
		return fmt.Errorf("failed to create eots store: %w", err)
	}

	defer func() {
		if err := es.Close(); err != nil {
			fmt.Printf("Error closing EOTS store: %v\n", err)
		}
	}()

	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(eotsFpPubKeyStr)
	if err != nil {
		return fmt.Errorf("invalid finality-provider public key %s: %w", eotsFpPubKeyStr, err)
	}

	if err = es.DeleteSignRecordsFromHeight(fpPk.MustMarshal(), []byte(chainID), height); err != nil {
		return fmt.Errorf("failed to delete sign store records: %w", err)
	}

	cmd.Printf("Successfully deleted sign store records from height %d\n", height)

	return nil
}
