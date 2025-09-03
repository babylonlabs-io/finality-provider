//nolint:revive
package common

import (
	"fmt"

	dc "github.com/babylonlabs-io/finality-provider/finality-provider/service/client"
	"github.com/spf13/cobra"
)

func NewBackupCmd(binaryName string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "backup",
		Short:   "Runs a safe hot backup of fpd.db while the fpd is running.",
		Aliases: []string{"bkp"},
		Long:    "Runs a safe hot backup of fpd.db. The fpd can continue to run while the backup is being created.",
		Example: fmt.Sprintf(`%s backup --db-path /path/fp.db --backup-dir /path/backupdir --daemon-address %s ...`, binaryName, defaultFpdDaemonAddress),
		RunE:    backup,
	}

	f := cmd.Flags()

	f.String(flagDBPath, "", "Full path to fpd.db")
	f.String(flagBackupDir, "", "Full path to backup directory")
	f.String(FpdDaemonAddressFlag, "", "The RPC address of a running fpd")

	if err := cmd.MarkFlagRequired(flagDBPath); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(flagBackupDir); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(FpdDaemonAddressFlag); err != nil {
		panic(err)
	}

	return cmd
}

func backup(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()

	dbPath, err := f.GetString(flagDBPath)
	if err != nil {
		return fmt.Errorf("failed to get db-path %s flag: %w", flagDBPath, err)
	}

	backupDir, err := f.GetString(flagBackupDir)
	if err != nil {
		return fmt.Errorf("failed to get %s flag: %w", flagBackupDir, err)
	}

	rpcListener, err := cmd.Flags().GetString(FpdDaemonAddressFlag)
	if err != nil {
		return fmt.Errorf("failed to get %s flag: %w", FpdDaemonAddressFlag, err)
	}

	grpcClient, cleanUp, err := dc.NewFinalityProviderServiceGRpcClient(rpcListener)
	if err != nil {
		return fmt.Errorf("failed to create grpc client: %w", err)
	}

	defer func() {
		if err := cleanUp(); err != nil {
			cmd.Printf("Failed to clean up grpc client: %v\n", err)
		}
	}()

	backupName, err := grpcClient.Backup(cmd.Context(), dbPath, backupDir)
	if err != nil {
		return fmt.Errorf("failed to backup database: %w", err)
	}

	cmd.Printf("Successfully created backup at: %s/%s", backupDir, backupName)

	return nil
}
