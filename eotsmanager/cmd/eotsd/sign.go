package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/log"
)

type DataSigned struct {
	KeyName             string `json:"key_name"`
	PubKeyHex           string `json:"pub_key_hex"`
	SignedDataHashHex   string `json:"signed_data_hash_hex"`
	SchnorrSignatureHex string `json:"schnorr_signature_hex"`
}

func NewSignSchnorrCmd() *cobra.Command {
	signSchnorrCmd := &cobra.Command{
		Use:   "sign-schnorr [file-path]",
		Short: "Signs a Schnorr signature over arbitrary data with the EOTS private key.",
		Long: fmt.Sprintf(`Read the file received as argument, hash it with
	sha256 and sign based on the Schnorr key associated with the %s or %s flag.
	If both flags are supplied, %s takes priority`, keyNameFlag, eotsPkFlag, eotsPkFlag),
		Args: cobra.ExactArgs(1),
		RunE: signSchnorr,
	}

	signSchnorrCmd.Flags().String(keyNameFlag, "", "The name of the key to load private key for signing")
	signSchnorrCmd.Flags().String(eotsPkFlag, "", "The public key of the finality-provider to load private key for signing")
	signSchnorrCmd.Flags().String(passphraseFlag, defaultPassphrase, "The passphrase used to decrypt the keyring")
	signSchnorrCmd.Flags().String(keyringBackendFlag, defaultKeyringBackend, "The backend of the keyring")

	return signSchnorrCmd
}

func signSchnorr(cmd *cobra.Command, args []string) error {
	keyName, err := cmd.Flags().GetString(keyNameFlag)
	if err != nil {
		return fmt.Errorf("failed to get key name flag: %w", err)
	}

	fpPkStr, err := cmd.Flags().GetString(eotsPkFlag)
	if err != nil {
		return fmt.Errorf("failed to get eots public key flag: %w", err)
	}

	passphrase, err := cmd.Flags().GetString(passphraseFlag)
	if err != nil {
		return fmt.Errorf("failed to get passphrase flag: %w", err)
	}

	keyringBackend, err := cmd.Flags().GetString(keyringBackendFlag)
	if err != nil {
		return fmt.Errorf("failed to get keyring backend flag: %w", err)
	}

	inputFilePath := args[0]

	if len(fpPkStr) == 0 && len(keyName) == 0 {
		return fmt.Errorf("at least one of the flags: %s, %s needs to be informed", keyNameFlag, eotsPkFlag)
	}

	homePath, err := getHomePath(cmd)
	if err != nil {
		return fmt.Errorf("failed to load home flag: %w", err)
	}

	cfg, err := config.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load config at %s: %w", homePath, err)
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to load the logger")
	}

	dbBackend, err := cfg.DatabaseConfig.GetDbBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}
	defer dbBackend.Close()

	eotsManager, err := eotsmanager.NewLocalEOTSManager(homePath, keyringBackend, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	hashOfMsgToSign, err := hashFromFile(inputFilePath)
	if err != nil {
		return fmt.Errorf("failed to generate hash from file %s: %w", inputFilePath, err)
	}

	signature, pubKey, err := singMsg(eotsManager, keyName, fpPkStr, passphrase, hashOfMsgToSign)
	if err != nil {
		return fmt.Errorf("failed to sign msg: %w", err)
	}

	printRespJSON(DataSigned{
		KeyName:             keyName,
		PubKeyHex:           pubKey.MarshalHex(),
		SignedDataHashHex:   hex.EncodeToString(hashOfMsgToSign),
		SchnorrSignatureHex: hex.EncodeToString(signature.Serialize()),
	})

	return nil
}

func hashFromFile(inputFilePath string) ([]byte, error) {
	h := sha256.New()

	// #nosec G304 - The log file path is provided by the user and not externally
	f, err := os.Open(inputFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open the file %s: %w", inputFilePath, err)
	}
	defer f.Close()

	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}

func singMsg(
	eotsManager *eotsmanager.LocalEOTSManager,
	keyName, fpPkStr, passphrase string,
	hashOfMsgToSign []byte,
) (*schnorr.Signature, *bbntypes.BIP340PubKey, error) {
	if len(fpPkStr) > 0 {
		fpPk, err := bbntypes.NewBIP340PubKeyFromHex(fpPkStr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid finality-provider public key %s: %w", fpPkStr, err)
		}
		signature, err := eotsManager.SignSchnorrSig(*fpPk, hashOfMsgToSign, passphrase)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to sign msg with pk %s: %w", fpPkStr, err)
		}
		return signature, fpPk, nil
	}

	return eotsManager.SignSchnorrSigFromKeyname(keyName, passphrase, hashOfMsgToSign)
}

func printRespJSON(resp interface{}) {
	jsonBytes, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		fmt.Println("unable to decode response: ", err)
		return
	}

	fmt.Printf("%s\n", jsonBytes)
}
