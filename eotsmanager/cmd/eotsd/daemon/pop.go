package daemon

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

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
	btcstktypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/babylonlabs-io/finality-provider/codec"
	"github.com/babylonlabs-io/finality-provider/eotsmanager"
	"github.com/babylonlabs-io/finality-provider/eotsmanager/config"
	"github.com/babylonlabs-io/finality-provider/log"
)

const (
	flagHomeBaby           = "baby-home"
	flagKeyNameBaby        = "baby-key-name"
	flagKeyringBackendBaby = "baby-keyring-backend"
)

func init() {
	bbnparams.SetAddressPrefixes()
}

// PoPExport the data for exporting the PoP
type PoPExport struct {
	// Btc public key is the eots *bbntypes.BIP340PubKey marshal hex
	BtcPublicKey string `json:"btcPublicKey"`
	// Baby public key is the *secp256k1.PubKey marshal hex
	BabyPublicKey string `json:"babyPublicKey"`

	// Babylon key pair signs adr36
	BabySignBtc string `json:"babySignBtc"`
	// BIP340 ProofOfPossessionBTC as hex
	BtcSignBaby string `json:"btcSignBaby"`

	// Btc address is the same as the btc pub key
	BtcAddress string `json:"btcAddress"`
	// Babylon address ex.: bbn1f04czxeqprn0s9fe7kdzqyde2e6nqj63dllwsm
	BabyAddress string `json:"babyAddress"`
}

func NewExportPopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pop-export [bbn-address]",
		Short: "Exports the Proof of Possession by signing over the finality provider's Babylon address with the EOTS private key.",
		Long: `Parse the address received as argument, hash it with
		sha256 and sign based on the EOTS key associated with the key-name or eots-pk flag.
		If the both flags are supplied, eots-pk takes priority. Use the generated signature
		to build a Proof of Possession and export it.`,
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

	babyAccAddr, err := babyKeyRecord.GetAddress()
	if err != nil {
		return err
	}

	bbnAddrBz, err := sdk.GetFromBech32(babyAccAddr.String(), "bbn")
	if err != nil {
		return err
	}
	bbnAddr := sdk.AccAddress(bbnAddrBz)

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

	hashOfMsgToSign := tmhash.Sum(bbnAddr.Bytes())
	btcSig, btcPubKey, err := eotsSignMsg(eotsManager, eotsKeyName, eotsFpPubKeyStr, eotsPassphrase, hashOfMsgToSign)
	if err != nil {
		return fmt.Errorf("failed to sign address %s: %w", bbnAddr.String(), err)
	}

	bip340Sig := bbntypes.NewBIP340SignatureFromBTCSig(btcSig)
	btcSigBz, err := bip340Sig.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal BTC Sig: %w", err)
	}

	popBtcSignBaby := btcstktypes.ProofOfPossessionBTC{
		BtcSigType: btcstktypes.BTCSigType_BIP340,
		BtcSig:     btcSigBz,
	}

	popHexBtcSignBaby, err := popBtcSignBaby.ToHexStr()
	if err != nil {
		return fmt.Errorf("failed to marshal pop to hex: %w", err)
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
		BtcPublicKey:  eotsPkHex,
		BabyPublicKey: base64.StdEncoding.EncodeToString(babyPubKey.Bytes()),

		BtcAddress:  eotsPkHex,
		BabyAddress: bbnAddr.String(),

		BtcSignBaby: popHexBtcSignBaby,
		BabySignBtc: base64.StdEncoding.EncodeToString(babySignBtc),
	}

	jsonString, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}

	cmd.Println(string(jsonString))

	return nil
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
