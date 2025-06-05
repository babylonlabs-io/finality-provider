package daemon

import (
	"encoding/hex"
	"fmt"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	"github.com/spf13/cobra"
)

func NewUnlockKeyringCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "Unlocks the file based keyring to load the EOTS private key in memory for signing",
		RunE:  unlockKeyring,
	}

	f := cmd.Flags()

	f.String(eotsPkFlag, "", "EOTS public key of the finality-provider")
	f.String(passphraseFlag, "", "The keyring passphrase")
	f.String(rpcClientFlag, "", "The RPC address of a running eotsd")

	if err := cmd.MarkFlagRequired(eotsPkFlag); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(passphraseFlag); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(rpcClientFlag); err != nil {
		panic(err)
	}

	return cmd
}

func unlockKeyring(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()

	eotsFpPubKeyStr, err := f.GetString(eotsPkFlag)
	if err != nil {
		return fmt.Errorf("failed to get eots pk flag: %w", err)
	}

	passphrase, err := f.GetString(passphraseFlag)
	if err != nil {
		return fmt.Errorf("failed to get chain-id flag: %w", err)
	}

	rpcListener, err := cmd.Flags().GetString(rpcClientFlag)
	if err != nil {
		return err
	}

	eotsdClient, err := eotsclient.NewEOTSManagerGRpcClient(rpcListener, "")
	if err != nil {
		return err
	}

	eotsFpPub, err := hex.DecodeString(eotsFpPubKeyStr)
	if err != nil {
		return fmt.Errorf("failed to decode eots public key: %w", err)
	}

	if err := eotsdClient.Unlock(eotsFpPub, passphrase); err != nil {
		return fmt.Errorf("failed to unlock keyring: %w", err)
	}

	cmd.Printf("Successfully unlocked keystore to load the EOTS private key for %s in memory of eotsd\n", eotsFpPubKeyStr)

	return nil
}
