package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/urfave/cli"
)

type DataSigned struct {
	KeyName             string `json:"key_name"`
	PubKeyHex           string `json:"pub_key_hex"`
	SignedDataHashHex   string `json:"signed_data_hash_hex"`
	SchnorrSignatureHex string `json:"schnorr_signature_hex"`
}

var SignSchnorrSig = cli.Command{
	Name:      "sign-schnorr",
	Usage:     "Signs a Schnorr signature over arbitrary data with the EOTS private key.",
	UsageText: "sign-schnorr [file-path]",
	Description: fmt.Sprintf(`Read the file received as argument, hash it with
	sha256 and sign based on the Schnorr key associated with the %s or %s flag.
	If the both flags are supplied, %s takes priority`, keyNameFlag, eotsPkFlag, eotsPkFlag),
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "Path to the keyring directory",
			Value: config.DefaultEOTSDir,
		},
		cli.StringFlag{
			Name:  keyNameFlag,
			Usage: "The name of the key to load private key for signing",
		},
		cli.StringFlag{
			Name:  eotsPkFlag,
			Usage: "The public key of the finality-provider to load private key for signing",
		},
		cli.StringFlag{
			Name:  passphraseFlag,
			Usage: "The passphrase used to decrypt the keyring",
			Value: defaultPassphrase,
		},
		cli.StringFlag{
			Name:  keyringBackendFlag,
			Usage: "The backend of the keyring",
			Value: defaultKeyringBackend,
		},
	},
	Action: SignSchnorr,
}

var VerifySchnorrSig = cli.Command{
	Name:      "verify-schnorr-sig",
	Usage:     "Verify a Schnorr signature over arbitrary data with the given public key.",
	UsageText: "verify-schnorr-sig [file-path]",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:     eotsPkFlag,
			Usage:    "The EOTS public key that will be used to verify the signature",
			Required: true,
		},
		cli.StringFlag{
			Name:     signatureFlag,
			Usage:    "The hex signature to verify",
			Required: true,
		},
	},
	Action: SignSchnorrVerify,
}

func SignSchnorrVerify(ctx *cli.Context) error {
	fpPkStr := ctx.String(eotsPkFlag)
	signatureHex := ctx.String(signatureFlag)

	args := ctx.Args()
	inputFilePath := args.First()
	if len(inputFilePath) == 0 {
		return errors.New("invalid argument, please provide a valid file path as input argument")
	}

	hashOfMsgToSign, err := hashFromFile(inputFilePath)
	if err != nil {
		return fmt.Errorf("failed to generate hash from file %s: %w", inputFilePath, err)
	}

	fpPk, err := bbntypes.NewBIP340PubKeyFromHex(fpPkStr)
	if err != nil {
		return fmt.Errorf("invalid finality-provider public key %s: %w", fpPkStr, err)
	}

	pubKey, err := schnorr.ParsePubKey(*fpPk)
	if err != nil {
		return fmt.Errorf("unable to parse public key %s: %w", fpPkStr, err)
	}

	signatureBz, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("unable to decode signature %s: %w", signatureHex, err)
	}

	signature, err := schnorr.ParseSignature(signatureBz)
	if err != nil {
		return fmt.Errorf("unable to parse schnorr signature %s: %w", signatureBz, err)
	}

	if !signature.Verify(hashOfMsgToSign, pubKey) {
		return errors.New("invalid signature")
	}

	fmt.Print("Verification is successful!")
	return nil
}

func SignSchnorr(ctx *cli.Context) error {
	keyName := ctx.String(keyNameFlag)
	fpPkStr := ctx.String(eotsPkFlag)
	passphrase := ctx.String(passphraseFlag)
	keyringBackend := ctx.String(keyringBackendFlag)

	args := ctx.Args()
	inputFilePath := args.First()
	if len(inputFilePath) == 0 {
		return errors.New("invalid argument, please provide a valid file path as input argument")
	}

	if len(fpPkStr) == 0 && len(keyName) == 0 {
		return fmt.Errorf("at least one of the flags: %s, %s needs to be informed", keyNameFlag, eotsPkFlag)
	}

	homePath, err := getHomeFlag(ctx)
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

	return printRespJSON(ctx, DataSigned{
		KeyName:             keyName,
		PubKeyHex:           pubKey.MarshalHex(),
		SignedDataHashHex:   hex.EncodeToString(hashOfMsgToSign),
		SchnorrSignatureHex: hex.EncodeToString(signature.Serialize()),
	})
}

func hashFromFile(inputFilePath string) ([]byte, error) {
	h := sha256.New()

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

func printRespJSON(ctx *cli.Context, resp interface{}) error {
	jsonBz, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		fmt.Println("unable to decode response: ", err)
		return nil
	}

	outputFilePath := ctx.String(flagOutputFile)
	if len(outputFilePath) > 0 {
		if err := os.WriteFile(outputFilePath, jsonBz, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
	}

	fmt.Printf("%s\n", jsonBz)

	return nil
}
