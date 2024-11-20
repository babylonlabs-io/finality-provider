package keyring

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"

	"github.com/babylonlabs-io/finality-provider/codec"
)

func CreateKeyring(keyringDir string, chainID string, backend string, input *strings.Reader) (keyring.Keyring, error) {
	ctx, err := CreateClientCtx(keyringDir, chainID)
	if err != nil {
		return nil, err
	}

	if backend == "" {
		return nil, fmt.Errorf("the keyring backend should not be empty")
	}

	kr, err := keyring.New(
		ctx.ChainID,
		backend,
		ctx.KeyringDir,
		input,
		ctx.Codec,
		ctx.KeyringOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	return kr, nil
}

func CreateClientCtx(keyringDir string, chainID string) (client.Context, error) {
	var err error
	var homeDir string

	if keyringDir == "" {
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return client.Context{}, err
		}
		keyringDir = path.Join(homeDir, ".finality-provider")
	}

	return client.Context{}.
		WithChainID(chainID).
		WithCodec(codec.MakeCodec()).
		WithKeyringDir(keyringDir), nil
}
