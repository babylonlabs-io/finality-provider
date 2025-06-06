package daemon

import (
	"encoding/hex"
	"fmt"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"os"
)

var UnlockCmdPasswordReader = passwordReader

func passwordReader(cmd *cobra.Command) (string, error) {
	cmd.Print("Enter password to unlock keyring: ")
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))

	return string(passphrase), err
}

func NewUnlockKeyringCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: `Unlocks the "file" based keyring to load the EOTS private key in memory for signing`,
		Long: `Unlocks the "file" based keyring. Keyring password can be provided either through environment variable 
"EOTSD_KEYRING_PASSWORD" or through input when executing the command. If the "EOTSD_KEYRING_PASSWORD" the command doesn't prompt for password.'`,
		RunE: unlockKeyring,
	}

	f := cmd.Flags()

	f.String(eotsPkFlag, "", "EOTS public key of the finality-provider")
	f.String(rpcClientFlag, "", "The RPC address of a running eotsd")
	f.String(flagHMAC, "", "The HMAC key for authentication with EOTSD. When using HMAC either pass here as flag or set env variable HMAC_KEY.")

	if err := cmd.MarkFlagRequired(eotsPkFlag); err != nil {
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
		return fmt.Errorf("failed to get %s flag: %w", eotsPkFlag, err)
	}

	rpcListener, err := cmd.Flags().GetString(rpcClientFlag)
	if err != nil {
		return fmt.Errorf("failed to get %s flag: %w", rpcClientFlag, err)
	}

	hmac, err := cmd.Flags().GetString(flagHMAC)
	if err != nil {
		return fmt.Errorf("failed to get %s flag: %w", flagHMAC, err)
	}

	envHmac, exists := os.LookupEnv("HMAC_KEY")
	if hmac == "" && exists {
		hmac = envHmac
	}

	passphrase, exists := os.LookupEnv("EOTSD_KEYRING_PASSWORD")
	if !exists {
		passphrase, err = UnlockCmdPasswordReader(cmd)
		if err != nil {
			return fmt.Errorf("failed to read passphrase: %w", err)
		}
	}

	eotsdClient, err := eotsclient.NewEOTSManagerGRpcClient(rpcListener, hmac)
	if err != nil {
		return fmt.Errorf("failed to create eotsd client: %w", err)
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
