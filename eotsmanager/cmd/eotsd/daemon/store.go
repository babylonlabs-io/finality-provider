package daemon

import (
	"fmt"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
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
	f.String(keyNameFlag, "", "EOTS key name")
	f.String(eotsPkFlag, "", "EOTS public key of the finality-provider")
	f.String(sdkflags.FlagKeyringBackend, keyring.BackendTest, "EOTS backend of the keyring")

	f.Uint64(flagFromHeight, 0, "height until which to rollback the sign store")

	return cmd
}

func rollbackSignStore(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	eotsHomePath, eotsKeyName, eotsFpPubKeyStr, eotsKeyringBackend, err := eotsFlags(cmd)
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

	eotsManager, err := loadEotsManager(eotsHomePath, eotsFpPubKeyStr, eotsKeyName, eotsKeyringBackend)
	if err != nil {
		return err
	}
	defer cmdCloseEots(cmd, eotsManager)

	err = eotsManager.UnsafeDeleteSignStoreRecords(height)
	if err != nil {
		return fmt.Errorf("failed to delete sign store records: %w", err)
	}

	cmd.Printf("Successfully deleted sign store records until height %d\n", height)

	return nil
}
