package daemon

import (
	"encoding/hex"
	"fmt"
	eotsclient "github.com/babylonlabs-io/finality-provider/eotsmanager/client"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"io"
	"os"
)

var UnlockCmdPasswordReader = passwordReader

func passwordReader(cmd *cobra.Command) (string, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat stdin: %w", err)
	}

	if (fi.Mode() & os.ModeCharDevice) == 0 {
		// Not a terminal, read full input from stdin
		passphrase, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read passphrase from stdin: %w", err)
		}

		return string(passphrase), nil
	}

	// TTY: interactive prompt
	cmd.Print("Enter password to unlock keyring: ")
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	cmd.Println()
	if err != nil {
		return "", fmt.Errorf("failed to read passphrase from terminal: %w", err)
	}

	return string(passphrase), nil
}

func NewUnlockKeyringCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: `Unlocks the "file" based keyring to load the EOTS private key in memory for signing`,
		Long: `Unlocks the "file"-based keyring to load the EOTS private key into memory for signing operations.

The keyring password can be provided in two ways:
  - By setting the "EOTSD_KEYRING_PASSWORD" environment variable.
  - By entering it interactively when prompted (if the environment variable is not set).

The HMAC key can also be provided in two ways:
  - By specifying the "home" flag, which points to the eotsd home directory containing the config.
  - By setting the "HMAC_KEY" environment variable, which takes precedence over the "home" flag.`,

		RunE: unlockKeyring,
	}

	f := cmd.Flags()

	f.String(eotsPkFlag, "", "EOTS public key of the finality-provider")
	f.String(rpcClientFlag, "", "The RPC address of a running eotsd")
	f.String(sdkflags.FlagHome, "", "The path to the eotsd home directory")

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

	hmac, exists := os.LookupEnv("HMAC_KEY")
	if !exists {
		var err error
		hmac, err = getHMACFromConfig(cmd)
		if err != nil {
			return err
		}
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

func getHMACFromConfig(cmd *cobra.Command) (string, error) {
	flagHome, err := cmd.Flags().GetString(sdkflags.FlagHome)
	if err != nil {
		return "", fmt.Errorf("failed to get %s flag: %w", sdkflags.FlagHome, err)
	}

	if flagHome == "" {
		return "", nil
	}

	homePath, err := getHomePath(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to load home flag: %w", err)
	}

	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return "", fmt.Errorf("failed to load config at %s: %w", homePath, err)
	}

	return cfg.HMACKey, nil
}
