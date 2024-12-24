package keyring

import (
	"fmt"
	"strings"

	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdksecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/go-bip39"

	"github.com/babylonlabs-io/finality-provider/types"
)

const (
	secp256k1Type       = "secp256k1"
	mnemonicEntropySize = 256
)

type ChainKeyringController struct {
	kr     keyring.Keyring
	fpName string
	// input is to send passphrase to kr
	input *strings.Reader
}

func NewChainKeyringController(ctx client.Context, name, keyringBackend string) (*ChainKeyringController, error) {
	if name == "" {
		return nil, fmt.Errorf("the key name should not be empty")
	}

	if keyringBackend == "" {
		return nil, fmt.Errorf("the keyring backend should not be empty")
	}

	inputReader := strings.NewReader("")
	kr, err := keyring.New(
		ctx.ChainID,
		keyringBackend,
		ctx.KeyringDir,
		inputReader,
		ctx.Codec,
		ctx.KeyringOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	return &ChainKeyringController{
		fpName: name,
		kr:     kr,
		input:  inputReader,
	}, nil
}

func NewChainKeyringControllerWithKeyring(kr keyring.Keyring, name string, input *strings.Reader) (*ChainKeyringController, error) {
	if name == "" {
		return nil, fmt.Errorf("the key name should not be empty")
	}

	return &ChainKeyringController{
		kr:     kr,
		fpName: name,
		input:  input,
	}, nil
}

func (kc *ChainKeyringController) GetKeyring() keyring.Keyring {
	return kc.kr
}

func (kc *ChainKeyringController) CreateChainKey(passphrase, hdPath, mnemonic string) (*types.ChainKeyInfo, error) {
	keyringAlgos, _ := kc.kr.SupportedAlgorithms()
	algo, err := keyring.NewSigningAlgoFromString(secp256k1Type, keyringAlgos)
	if err != nil {
		return nil, err
	}

	if len(mnemonic) == 0 {
		// read entropy seed straight from tmcrypto.Rand and convert to mnemonic
		entropySeed, err := bip39.NewEntropy(mnemonicEntropySize)
		if err != nil {
			return nil, err
		}

		mnemonic, err = bip39.NewMnemonic(entropySeed)
		if err != nil {
			return nil, err
		}
	}

	// we need to repeat the passphrase to mock the reentry
	kc.input.Reset(passphrase + "\n" + passphrase)
	record, err := kc.kr.NewAccount(kc.fpName, mnemonic, passphrase, hdPath, algo)
	if err != nil {
		return nil, err
	}

	privKey := record.GetLocal().PrivKey.GetCachedValue()
	accAddress, err := record.GetAddress()
	if err != nil {
		return nil, err
	}

	switch v := privKey.(type) {
	case *sdksecp256k1.PrivKey:
		sk, pk := btcec.PrivKeyFromBytes(v.Key)

		return &types.ChainKeyInfo{
			Name:       kc.fpName,
			AccAddress: accAddress,
			PublicKey:  pk,
			PrivateKey: sk,
			Mnemonic:   mnemonic,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported key type in keyring")
	}
}

// CreatePop creates proof-of-possession of Babylon and BTC public keys
// the input is the bytes of BTC public key used to sign
// this requires both keys created beforehand
func (kc *ChainKeyringController) CreatePop(fpAddr sdk.AccAddress, btcPrivKey *btcec.PrivateKey) (*bstypes.ProofOfPossessionBTC, error) {
	return bstypes.NewPoPBTC(fpAddr, btcPrivKey)
}

// Address returns the address from the keyring
func (kc *ChainKeyringController) Address(passphrase string) (sdk.AccAddress, error) {
	kc.input.Reset(passphrase)
	k, err := kc.kr.Key(kc.fpName)
	if err != nil {
		return nil, fmt.Errorf("failed to get address: %w", err)
	}

	return k.GetAddress()
}

func (kc *ChainKeyringController) GetChainPrivKey(passphrase string) (*sdksecp256k1.PrivKey, error) {
	kc.input.Reset(passphrase)
	k, err := kc.kr.Key(kc.fpName)
	if err != nil {
		return nil, fmt.Errorf("failed to get private key: %w", err)
	}

	privKeyCached := k.GetLocal().PrivKey.GetCachedValue()

	switch v := privKeyCached.(type) {
	case *sdksecp256k1.PrivKey:
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported key type in keyring")
	}
}
