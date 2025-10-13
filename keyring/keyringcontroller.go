package keyring

import (
	"fmt"
	"os"

	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	bstypes "github.com/babylonlabs-io/babylon/v4/x/btcstaking/types"
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
}

func NewChainKeyringController(ctx client.Context, name, keyringBackend string) (*ChainKeyringController, error) {
	if name == "" {
		return nil, fmt.Errorf("the key name should not be empty")
	}

	if keyringBackend == "" {
		return nil, fmt.Errorf("the keyring backend should not be empty")
	}

	kr, err := keyring.New(
		ctx.ChainID,
		keyringBackend,
		ctx.KeyringDir,
		os.Stdin,
		ctx.Codec,
		ctx.KeyringOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	return &ChainKeyringController{
		fpName: name,
		kr:     kr,
	}, nil
}

func NewChainKeyringControllerWithKeyring(kr keyring.Keyring, name string) (*ChainKeyringController, error) {
	if name == "" {
		return nil, fmt.Errorf("the key name should not be empty")
	}

	return &ChainKeyringController{
		kr:     kr,
		fpName: name,
	}, nil
}

func (kc *ChainKeyringController) GetKeyring() keyring.Keyring {
	return kc.kr
}

func (kc *ChainKeyringController) CreateChainKey(passphrase, hdPath, mnemonic string) (*types.ChainKeyInfo, error) {
	keyringAlgos, _ := kc.kr.SupportedAlgorithms()
	algo, err := keyring.NewSigningAlgoFromString(secp256k1Type, keyringAlgos)
	if err != nil {
		return nil, fmt.Errorf("failed to create signing algorithm: %w", err)
	}

	if len(mnemonic) == 0 {
		// read entropy seed straight from tmcrypto.Rand and convert to mnemonic
		entropySeed, err := bip39.NewEntropy(mnemonicEntropySize)
		if err != nil {
			return nil, fmt.Errorf("failed to generate entropy: %w", err)
		}

		mnemonic, err = bip39.NewMnemonic(entropySeed)
		if err != nil {
			return nil, fmt.Errorf("failed to generate mnemonic: %w", err)
		}
	}

	record, err := kc.kr.NewAccount(kc.fpName, mnemonic, passphrase, hdPath, algo)
	if err != nil {
		return nil, fmt.Errorf("failed to create new account: %w", err)
	}

	privKey := record.GetLocal().PrivKey.GetCachedValue()
	accAddress, err := record.GetAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get address from record: %w", err)
	}

	switch v := privKey.(type) {
	case *sdksecp256k1.PrivKey:
		// Validate that the private key bytes fit within the secp256k1 curve order
		// before converting to PrivateKey type. btcd passes this responsibility to callers.
		var keyInt btcec.ModNScalar
		overflow := keyInt.SetByteSlice(v.Key)
		if overflow {
			return nil, fmt.Errorf("private key is greater than or equal to the secp256k1 curve order")
		}
		if keyInt.IsZero() {
			return nil, fmt.Errorf("private key cannot be zero")
		}

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
func (kc *ChainKeyringController) CreatePop(_ string, fpAddr sdk.AccAddress, btcPrivKey *btcec.PrivateKey) (*bstypes.ProofOfPossessionBTC, error) {
	// Use FpPopContextV0 with the provided chain ID
	pop, err := datagen.NewPoPBTC(fpAddr, btcPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create proof of possession: %w", err)
	}

	return pop, nil
}

// Address returns the address from the keyring
func (kc *ChainKeyringController) Address() (sdk.AccAddress, error) {
	k, err := kc.kr.Key(kc.fpName)
	if err != nil {
		return nil, fmt.Errorf("failed to get address: %w", err)
	}

	addr, err := k.GetAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get address from key: %w", err)
	}

	return addr, nil
}
