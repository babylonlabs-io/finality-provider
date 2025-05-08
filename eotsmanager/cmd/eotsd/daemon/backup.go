package daemon

import (
	"fmt"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	"github.com/spf13/cobra"
)

func NewBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Runs a hot backup of eots.db",
		RunE:  backup,
	}

	f := cmd.Flags()

	f.String(flagDBPath, "", "Full path to eots.db")
	f.String(flagBackupDir, "", "Full path to backup directory")
	f.String(rpcClientFlag, "", "The RPC address of a running eotsd")

	if err := cmd.MarkFlagRequired(flagDBPath); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(flagBackupDir); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(rpcClientFlag); err != nil {
		panic(err)
	}

	return cmd
}

func backup(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()

	dbPath, err := f.GetString(flagDBPath)
	if err != nil {
		return fmt.Errorf("failed to get db-path flag: %w", err)
	}

	backupDir, err := f.GetString(flagBackupDir)
	if err != nil {
		return fmt.Errorf("failed to get backup-dir flag: %w", err)
	}

	rpcListener, err := cmd.Flags().GetString(rpcClientFlag)
	if err != nil {
		return err
	}

	eotsdClient, err := eotsclient.NewEOTSManagerGRpcClient(rpcListener, "")
	if err != nil {
		return err
	}

	if err := eotsdClient.Backup(dbPath, backupDir); err != nil {
		return fmt.Errorf("failed to unlock keyring: %w", err)
	}

	cmd.Printf("Successfully created backup at %s", backupDir)

	return nil
}
