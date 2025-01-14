package daemon

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cometbft/cometbft/crypto/tmhash"
	sdkflags "github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/spf13/cobra"

	bbnparams "github.com/babylonlabs-io/babylon/app/params"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	"github.com/babylonlabs-io/finality-provider/codec"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/log"
)

const (
	flagHomeBaby           = "baby-home"
	flagKeyNameBaby        = "baby-key-name"
	flagKeyringBackendBaby = "baby-keyring-backend"
	flagMessage            = "message"
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

// PoPExportDelete the data needed to delete an ownership previously created.
type PoPExportDelete struct {
	// Btc public key is the EOTS PK *bbntypes.BIP340PubKey marshal hex
	EotsPublicKey string `json:"eotsPublicKey"`
	// Baby public key is the *secp256k1.PubKey marshal hex
	BabyPublicKey string `json:"babyPublicKey"`

	// Babylon key pair signs message
	BabySignature string `json:"babySignature"`

	// Babylon address ex.: bbn1f04czxeqprn0s9fe7kdzqyde2e6nqj63dllwsm
	BabyAddress string `json:"babyAddress"`
}

func NewPopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pop",
		Short: "Proof of Possession commands",
	}

	cmd.AddCommand(
		NewPopExportCmd(),
		NewPopDeleteCmd(),
	)

	return cmd
}

func NewPopExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Exports the Proof of Possession by (1) signing over the BABY address with the EOTS private key and (2) signing over the EOTS public key with the BABY private key.",
		Long: `Parse the address from the BABY keyring, load the address, hash it with
		sha256 and sign based on the EOTS key associated with the key-name or eots-pk flag.
		If the both flags are supplied, eots-pk takes priority. Use the generated signature
		to build a Proof of Possession. For the creation of the BABY signature over the eots pk,
		it loads the BABY key pair and signs the eots-pk hex and exports it.`,
		RunE: exportPop,
	}

	f := cmd.Flags()

	f.String(sdkflags.FlagHome, config.DefaultEOTSDir, "EOTS home directory")
	f.String(keyNameFlag, "", "EOTS key name")
	f.String(eotsPkFlag, "", "EOTS public key of the finality-provider")
	f.String(passphraseFlag, "", "EOTS passphrase used to decrypt the keyring")
	f.String(sdkflags.FlagKeyringBackend, keyring.BackendTest, "EOTS backend of the keyring")

	f.String(flagHomeBaby, "", "BABY home directory")
	f.String(flagKeyNameBaby, "", "BABY key name")
	f.String(flagKeyringBackendBaby, keyring.BackendTest, "BABY backend of the keyring")

	return cmd
}

func NewPopDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Generate the delete data for removing a proof of possession previously created.",
		Long: `Parse the message from the flag and sign with the BABY keyring, it also loads
		the EOTS public key based on the EOTS key associated with the key-name or eots-pk flag.
		If the both flags are supplied, eots-pk takes priority.`,
		RunE: deletePop,
	}

	f := cmd.Flags()

	f.String(sdkflags.FlagHome, config.DefaultEOTSDir, "EOTS home directory")
	f.String(keyNameFlag, "", "EOTS key name")
	f.String(eotsPkFlag, "", "EOTS public key of the finality-provider")
	f.String(sdkflags.FlagKeyringBackend, keyring.BackendTest, "EOTS backend of the keyring")

	f.String(flagHomeBaby, "", "BABY home directory")
	f.String(flagKeyNameBaby, "", "BABY key name")
	f.String(flagKeyringBackendBaby, keyring.BackendTest, "BABY backend of the keyring")

	f.String(flagMessage, "", "Message to be signed")

	return cmd
}

func exportPop(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()

	eotsKeyName, err := f.GetString(keyNameFlag)
	if err != nil {
		return err
	}

	eotsFpPubKeyStr, err := f.GetString(eotsPkFlag)
	if err != nil {
		return err
	}

	eotsPassphrase, err := f.GetString(passphraseFlag)
	if err != nil {
		return err
	}

	eotsKeyringBackend, err := f.GetString(sdkflags.FlagKeyringBackend)
	if err != nil {
		return err
	}

	eotsHomePath, err := getHomePath(cmd)
	if err != nil {
		return fmt.Errorf("failed to load home flag: %w", err)
	}

	babyHomePath, err := getCleanPath(cmd, flagHomeBaby)
	if err != nil {
		return fmt.Errorf("failed to load baby home flag: %w", err)
	}

	babyKeyName, err := f.GetString(flagKeyNameBaby)
	if err != nil {
		return err
	}

	babyKeyringBackend, err := f.GetString(flagKeyringBackendBaby)
	if err != nil {
		return err
	}

	cdc := codec.MakeCodec()
	babyKeyring, err := keyring.New("baby", babyKeyringBackend, babyHomePath, cmd.InOrStdin(), cdc)
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

	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}
	defer dbBackend.Close()

	eotsManager, err := eotsmanager.NewLocalEOTSManager(eotsHomePath, eotsKeyringBackend, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	bbnAddrStr := bbnAddr.String()
	hashOfMsgToSign := tmhash.Sum([]byte(bbnAddrStr))
	schnorrSigOverBabyAddr, eotsPk, err := eotsSignMsg(eotsManager, eotsKeyName, eotsFpPubKeyStr, eotsPassphrase, hashOfMsgToSign)
	if err != nil {
		return fmt.Errorf("failed to sign address %s: %w", bbnAddrStr, err)
	}

	babyPubKey, err := babyPk(babyKeyRecord)
	if err != nil {
		return err
	}

	babySignEots := []byte(eotsPk.MarshalHex())
	babySignature, err := SignCosmosAdr36(babyKeyring, babyKeyName, bbnAddrStr, babySignEots)
	if err != nil {
		return err
	}

	eotsPkHex := eotsPk.MarshalHex()
	out := PoPExport{
		EotsPublicKey: eotsPkHex,
		BabyPublicKey: base64.StdEncoding.EncodeToString(babyPubKey.Bytes()),

		BabyAddress: bbnAddrStr,

		EotsSignBaby:   base64.StdEncoding.EncodeToString(schnorrSigOverBabyAddr.Serialize()),
		BabySignEotsPk: base64.StdEncoding.EncodeToString(babySignature),
	}

	jsonString, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}

	cmd.Println(string(jsonString))

	return nil
}

func deletePop(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()

	eotsKeyName, err := f.GetString(keyNameFlag)
	if err != nil {
		return err
	}

	eotsFpPubKeyStr, err := f.GetString(eotsPkFlag)
	if err != nil {
		return err
	}

	eotsKeyringBackend, err := f.GetString(sdkflags.FlagKeyringBackend)
	if err != nil {
		return err
	}

	eotsHomePath, err := getHomePath(cmd)
	if err != nil {
		return fmt.Errorf("failed to load home flag: %w", err)
	}

	babyHomePath, err := getCleanPath(cmd, flagHomeBaby)
	if err != nil {
		return fmt.Errorf("failed to load baby home flag: %w", err)
	}

	babyKeyName, err := f.GetString(flagKeyNameBaby)
	if err != nil {
		return err
	}

	babyKeyringBackend, err := f.GetString(flagKeyringBackendBaby)
	if err != nil {
		return err
	}

	cdc := codec.MakeCodec()
	babyKeyring, err := keyring.New("baby", babyKeyringBackend, babyHomePath, cmd.InOrStdin(), cdc)
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

	dbBackend, err := cfg.DatabaseConfig.GetDBBackend()
	if err != nil {
		return fmt.Errorf("failed to create db backend: %w", err)
	}
	defer dbBackend.Close()

	eotsManager, err := eotsmanager.NewLocalEOTSManager(eotsHomePath, eotsKeyringBackend, dbBackend, logger)
	if err != nil {
		return fmt.Errorf("failed to create EOTS manager: %w", err)
	}

	btcPubKey, err := eotsPubKey(eotsManager, eotsKeyName, eotsFpPubKeyStr)
	if err != nil {
		return fmt.Errorf("failed to sign address %s: %w", bbnAddr.String(), err)
	}

	babyPubKey, err := babyPk(babyKeyRecord)
	if err != nil {
		return err
	}

	eotsPkHex := btcPubKey.MarshalHex()

	msg, err := f.GetString(flagMessage)
	if err != nil {
		return err
	}
	if len(msg) == 0 {
		return fmt.Errorf("flage --%s is empty", flagMessage)
	}

	// We are assuming we are receiving string literal with escape characters
	interpretedMsg, err := strconv.Unquote(`"` + msg + `"`)
	if err != nil {
		return err
	}

	bbnAddrStr := bbnAddr.String()
	babySignature, err := SignCosmosAdr36(babyKeyring, babyKeyName, bbnAddrStr, []byte(interpretedMsg))
	if err != nil {
		return err
	}

	out := PoPExportDelete{
		EotsPublicKey: eotsPkHex,
		BabyPublicKey: base64.StdEncoding.EncodeToString(babyPubKey.Bytes()),

		BabyAddress: bbnAddr.String(),

		BabySignature: base64.StdEncoding.EncodeToString(babySignature),
	}

	jsonString, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}

	cmd.Println(string(jsonString))

	return nil
}

func SignCosmosAdr36(
	kr keyring.Keyring,
	keyName string,
	cosmosBech32Address string,
	bytesToSign []byte,
) ([]byte, error) {
	base64Bytes := base64.StdEncoding.EncodeToString(bytesToSign)

	signDoc := NewCosmosSignDoc(
		cosmosBech32Address,
		base64Bytes,
	)

	marshaled, err := json.Marshal(signDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sign doc: %w", err)
	}

	bz := sdk.MustSortJSON(marshaled)

	babySignBytes, _, err := kr.Sign(
		keyName,
		bz,
		signing.SignMode_SIGN_MODE_DIRECT,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to sign btc address bytes: %w", err)
	}

	return babySignBytes, nil
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

func ValidBabySignEots(babyPk, babyAddr, eotsPkHex, babySigOverEotsPk string) (bool, error) {
	babyPubKeyBz, err := base64.StdEncoding.DecodeString(babyPk)
	if err != nil {
		return false, err
	}

	babyPubKey := &secp256k1.PubKey{
		Key: babyPubKeyBz,
	}

	eotsPk, err := bbntypes.NewBIP340PubKeyFromHex(eotsPkHex)
	if err != nil {
		return false, err
	}

	babySignEots := []byte(eotsPk.MarshalHex())
	base64Bytes := base64.StdEncoding.EncodeToString(babySignEots)
	babySignBtcDoc := NewCosmosSignDoc(babyAddr, base64Bytes)
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

func eotsPubKey(
	eotsManager *eotsmanager.LocalEOTSManager,
	keyName, fpPkStr string,
) (*bbntypes.BIP340PubKey, error) {
	if len(fpPkStr) > 0 {
		fpPk, err := bbntypes.NewBIP340PubKeyFromHex(fpPkStr)
		if err != nil {
			return nil, fmt.Errorf("invalid finality-provider public key %s: %w", fpPkStr, err)
		}

		return fpPk, nil
	}

	return eotsManager.LoadBIP340PubKeyFromKeyName(keyName)
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
