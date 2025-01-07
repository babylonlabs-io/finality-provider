package daemon

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cometbft/cometbft/crypto/tmhash"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"

	bbnparams "github.com/babylonlabs-io/babylon/app/params"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/babylonlabs-io/finality-provider/codec"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/log"
	"github.com/urfave/cli"
)

const (
	flagHomeBaby           = "baby-home"
	flagKeyNameBaby        = "baby-key-name"
	flagKeyringBackendBaby = "baby-keyring-backend"
)

func init() {
	bbnparams.SetAddressPrefixes()
}

// PoPExport the data needed to prove ownership of the eots and baby key pairs.
type PoPExport struct {
	// Btc public key is the EOTS PK *bbntypes.BIP340PubKey marshal hex
	EotsPublicKey string `json:"eotsPublicKey"`
	// Baby public key is the *secp256k1.PubKey marshal hex
	BabyPublicKey string `json:"babyPublicKey"`

	// Babylon key pair signs EOTS public key as hex
	BabySignEotsPk string `json:"babySignEotsPk"`
	// Schnorr signature of EOTS private key over the SHA256(Baby address)
	EotsSignBaby string `json:"eotsSignBaby"`

	// Babylon address ex.: bbn1f04czxeqprn0s9fe7kdzqyde2e6nqj63dllwsm
	BabyAddress string `json:"babyAddress"`
}

var ExportPoPCommand = cli.Command{
	Name:  "export-pop",
	Usage: "Exports the Proof of Possession by (1) signing over the BABY address with the EOTS private key and (2) signing over the EOTS public key with the BABY private key.",
	Description: `Parse the address from the BABY keyring, load the address, hash it with
		sha256 and sign based on the EOTS key associated with the key-name or eots-pk flag.
		If the both flags are supplied, eots-pk takes priority. Use the generated signature
		to build a Proof of Possession. For the creation of the BABY signature over the eots pk,
		it loads the BABY key pair and signs the eots-pk hex and exports it.`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "EOTS home directory",
			Value: config.DefaultEOTSDir,
		},
		cli.StringFlag{
			Name:  keyNameFlag,
			Usage: "EOTS key name",
		},
		cli.StringFlag{
			Name:  eotsPkFlag,
			Usage: "EOTS public key of the finality-provider",
		},
		cli.StringFlag{
			Name:  passphraseFlag,
			Usage: "EOTS passphrase used to decrypt the keyring",
			Value: defaultPassphrase,
		},
		cli.StringFlag{
			Name:  keyringBackendFlag,
			Usage: "EOTS backend of the keyring",
			Value: defaultKeyringBackend,
		},
		cli.StringFlag{
			Name:  flagHomeBaby,
			Usage: "BABY home directory",
		},
		cli.StringFlag{
			Name:  flagKeyNameBaby,
			Usage: "BABY key name",
		},
		cli.StringFlag{
			Name:  flagKeyringBackendBaby,
			Usage: "BABY backend of the keyring",
			Value: defaultKeyringBackend,
		},
	},
	Action: exportPop,
}

func exportPop(ctx *cli.Context) error {
	eotsKeyName := ctx.String(keyNameFlag)
	eotsFpPubKeyStr := ctx.String(eotsPkFlag)
	eotsPassphrase := ctx.String(passphraseFlag)
	eotsKeyringBackend := ctx.String(keyringBackendFlag)

	if len(eotsFpPubKeyStr) == 0 && len(eotsKeyName) == 0 {
		return fmt.Errorf("at least one of the flags: %s, %s needs to be informed", keyNameFlag, eotsPkFlag)
	}

	eotsHomePath, err := getHomeFlag(ctx)
	if err != nil {
		return fmt.Errorf("failed to load home flag: %w", err)
	}

	babyHomePath, err := getCleanPath(ctx, flagHomeBaby)
	if err != nil {
		return err
	}

	babyKeyName := ctx.String(flagKeyNameBaby)
	babyKeyringBackend := ctx.String(flagKeyringBackendBaby)

	cdc := codec.MakeCodec()
	babyKeyring, err := keyring.New("baby", babyKeyringBackend, babyHomePath, os.Stdin, cdc)
	if err != nil {
		return fmt.Errorf("failed to create keyring: %w", err)
	}
	babyKeyRecord, err := babyKeyring.Key(babyKeyName)
	if err != nil {
		return err
	}
	bbnAddr, err := babyKeyRecord.GetAddress()
	if err != nil {
		return err
	}
	if len(eotsFpPubKeyStr) == 0 && len(eotsKeyName) == 0 {
		return fmt.Errorf("at least one of the flags: %s, %s needs to be informed", keyNameFlag, eotsPkFlag)
	}

	cfg, err := config.LoadConfig(eotsHomePath)
	if err != nil {
		return fmt.Errorf("failed to load config at %s: %w", eotsHomePath, err)
	}

	logger, err := log.NewRootLoggerWithFile(config.LogFile(eotsHomePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to load the logger")
	}

	dbBackend, err := cfg.DatabaseConfig.GetDbBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}
	defer dbBackend.Close()

	eotsManager, err := eotsmanager.NewLocalEOTSManager(eotsHomePath, eotsKeyringBackend, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	hashOfMsgToSign := tmhash.Sum([]byte(bbnAddr.String()))
	schnorrSigOverBabyAddr, btcPubKey, err := eotsSignMsg(eotsManager, eotsKeyName, eotsFpPubKeyStr, eotsPassphrase, hashOfMsgToSign)
	if err != nil {
		return fmt.Errorf("failed to sign address %s: %w", bbnAddr.String(), err)
	}

	babyPubKey, err := babyPk(babyKeyRecord)
	if err != nil {
		return err
	}
	eotsPkHex := btcPubKey.MarshalHex()
	babySignBtcDoc := NewCosmosSignDoc(
		bbnAddr.String(),
		eotsPkHex,
	)
	babySignBtcMarshaled, err := json.Marshal(babySignBtcDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal sign doc: %w", err)
	}
	babySignBtcBz := sdk.MustSortJSON(babySignBtcMarshaled)
	babySignBtc, _, err := babyKeyring.Sign(babyKeyName, babySignBtcBz, signing.SignMode_SIGN_MODE_DIRECT)
	if err != nil {
		return err
	}
	out := PoPExport{
		EotsPublicKey:  eotsPkHex,
		BabyPublicKey:  base64.StdEncoding.EncodeToString(babyPubKey.Bytes()),
		BabyAddress:    bbnAddr.String(),
		EotsSignBaby:   base64.StdEncoding.EncodeToString(schnorrSigOverBabyAddr.Serialize()),
		BabySignEotsPk: base64.StdEncoding.EncodeToString(babySignBtc),
	}

	printRespJSON(out)
	return nil
}

func VerifyPopExport(pop PoPExport) (bool, error) {
	valid, err := ValidEotsSignBaby(pop.EotsPublicKey, pop.BabyAddress, pop.EotsSignBaby)
	if err != nil || !valid {
		return false, err
	}
	return ValidBabySignEots(
		pop.BabyPublicKey,
		pop.BabyAddress,
		pop.EotsPublicKey,
		pop.BabySignEotsPk,
	)
}

func ValidEotsSignBaby(eotsPk, babyAddr, eotsSigOverBabyAddr string) (bool, error) {
	eotsPubKey, err := bbntypes.NewBIP340PubKeyFromHex(eotsPk)
	if err != nil {
		return false, err
	}
	schnorrSigBase64, err := base64.StdEncoding.DecodeString(eotsSigOverBabyAddr)
	if err != nil {
		return false, err
	}
	schnorrSig, err := schnorr.ParseSignature(schnorrSigBase64)
	if err != nil {
		return false, err
	}
	sha256Addr := tmhash.Sum([]byte(babyAddr))
	return schnorrSig.Verify(sha256Addr, eotsPubKey.MustToBTCPK()), nil
}

func ValidBabySignEots(babyPk, babyAddr, eotsPk, babySigOverEotsPk string) (bool, error) {
	babyPubKeyBz, err := base64.StdEncoding.DecodeString(babyPk)
	if err != nil {
		return false, err
	}
	babyPubKey := &secp256k1.PubKey{
		Key: babyPubKeyBz,
	}
	babySignBtcDoc := NewCosmosSignDoc(babyAddr, eotsPk)
	babySignBtcMarshaled, err := json.Marshal(babySignBtcDoc)
	if err != nil {
		return false, err
	}
	babySignEotsBz := sdk.MustSortJSON(babySignBtcMarshaled)
	secp256SigBase64, err := base64.StdEncoding.DecodeString(babySigOverEotsPk)
	if err != nil {
		return false, err
	}
	return babyPubKey.VerifySignature(babySignEotsBz, secp256SigBase64), nil
}

func babyPk(babyRecord *keyring.Record) (*secp256k1.PubKey, error) {
	pubKey, err := babyRecord.GetPubKey()
	if err != nil {
		return nil, err
	}
	switch v := pubKey.(type) {
	case *secp256k1.PubKey:
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported key type in keyring")
	}
}

func eotsSignMsg(
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

type Msg struct {
	Type  string   `json:"type"`
	Value MsgValue `json:"value"`
}
type SignDoc struct {
	ChainID       string `json:"chain_id"`
	AccountNumber string `json:"account_number"`
	Sequence      string `json:"sequence"`
	Fee           Fee    `json:"fee"`
	Msgs          []Msg  `json:"msgs"`
	Memo          string `json:"memo"`
}
type Fee struct {
	Gas    string   `json:"gas"`
	Amount []string `json:"amount"`
}
type MsgValue struct {
	Signer string `json:"signer"`
	Data   string `json:"data"`
}

func NewCosmosSignDoc(
	signer string,
	data string,
) *SignDoc {
	return &SignDoc{
		ChainID:       "",
		AccountNumber: "0",
		Sequence:      "0",
		Fee: Fee{
			Gas:    "0",
			Amount: []string{},
		},
		Msgs: []Msg{
			{
				Type: "sign/MsgSignData",
				Value: MsgValue{
					Signer: signer,
					Data:   data,
				},
			},
		},
		Memo: "",
	}
}
