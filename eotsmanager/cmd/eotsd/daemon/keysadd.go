package daemon

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/input"
	"github.com/cosmos/cosmos-sdk/client/keys"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cryptokeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/multisig"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/go-bip39"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

const (
	mnemonicEntropySize = 256
)

func runAddCmdPrepare(cmd *cobra.Command, args []string) error {
	clientCtx, err := client.GetClientQueryContext(cmd)
	if err != nil {
		return err
	}

	buf := bufio.NewReader(clientCtx.Input)
	return runAddCmd(clientCtx, cmd, args, buf)
}

/*
Code adapted from cosmos-sdk https://github.com/cosmos/cosmos-sdk/blob/c64d1010800d60677cc25e2fca5b3d8c37b683cc/client/keys/add.go#L127-L128
where the only diff is to allow empty HD path
input
  - bip39 mnemonic
  - bip39 passphrase
  - bip44 path
  - local encryption password

output
  - armor encrypted private key (saved to file)
    mostly coppied from the cosmos-sdk@v0.50.9 and
    replaced to allow use of empty hd path
*/
//nolint:gocyclo,maintidx
func runAddCmd(ctx client.Context, cmd *cobra.Command, args []string, inBuf *bufio.Reader) error {
	var err error

	name := args[0]
	interactive, _ := cmd.Flags().GetBool(flagInteractive)
	noBackup, _ := cmd.Flags().GetBool(flagNoBackup)
	showMnemonic := !noBackup
	kb := ctx.Keyring
	outputFormat := ctx.OutputFormat

	keyringAlgos, _ := kb.SupportedAlgorithms()
	algoStr, _ := cmd.Flags().GetString(flags.FlagKeyType)
	algo, err := cryptokeyring.NewSigningAlgoFromString(algoStr, keyringAlgos)
	if err != nil {
		return err
	}

	if dryRun, _ := cmd.Flags().GetBool(flags.FlagDryRun); dryRun {
		// use in memory keybase
		kb = cryptokeyring.NewInMemory(ctx.Codec)
	} else {
		_, err = kb.Key(name)
		if err == nil {
			// account exists, ask for user confirmation
			response, err2 := input.GetConfirmation(fmt.Sprintf("override the existing name %s", name), inBuf, cmd.ErrOrStderr())
			if err2 != nil {
				return err2
			}

			if !response {
				return errors.New("aborted")
			}

			err2 = kb.Delete(name)
			if err2 != nil {
				return err2
			}
		}

		multisigKeys, _ := cmd.Flags().GetStringSlice(flagMultisig)
		if len(multisigKeys) != 0 {
			pks := make([]cryptotypes.PubKey, len(multisigKeys))
			multisigThreshold, _ := cmd.Flags().GetInt(flagMultiSigThreshold)
			if err := validateMultisigThreshold(multisigThreshold, len(multisigKeys)); err != nil {
				return err
			}

			for i, keyname := range multisigKeys {
				k, err := kb.Key(keyname)
				if err != nil {
					return err
				}

				key, err := k.GetPubKey()
				if err != nil {
					return err
				}
				pks[i] = key
			}

			if noSort, _ := cmd.Flags().GetBool(flagNoSort); !noSort {
				sort.Slice(pks, func(i, j int) bool {
					return bytes.Compare(pks[i].Address(), pks[j].Address()) < 0
				})
			}

			pk := multisig.NewLegacyAminoPubKey(multisigThreshold, pks)
			k, err := kb.SaveMultisig(name, pk)
			if err != nil {
				return err
			}

			return printCreate(cmd, k, false, "", outputFormat)
		}
	}

	pubKey, _ := cmd.Flags().GetString(keys.FlagPublicKey)
	pubKeyBase64, _ := cmd.Flags().GetString(flagPubKeyBase64)
	if pubKey != "" && pubKeyBase64 != "" {
		return fmt.Errorf(`flags %s and %s cannot be used simultaneously`, keys.FlagPublicKey, flagPubKeyBase64)
	}
	if pubKey != "" {
		var pk cryptotypes.PubKey
		if err = ctx.Codec.UnmarshalInterfaceJSON([]byte(pubKey), &pk); err != nil {
			return err
		}

		k, err := kb.SaveOfflineKey(name, pk)
		if err != nil {
			return err
		}

		return printCreate(cmd, k, false, "", outputFormat)
	}
	if pubKeyBase64 != "" {
		b64, err := base64.StdEncoding.DecodeString(pubKeyBase64)
		if err != nil {
			return err
		}

		var pk cryptotypes.PubKey
		// create an empty pubkey in order to get the algo TypeUrl.
		tempAny, err := codectypes.NewAnyWithValue(algo.Generate()([]byte{}).PubKey())
		if err != nil {
			return err
		}

		jsonPub, err := json.Marshal(struct {
			Type string `json:"@type,omitempty"`
			Key  string `json:"key,omitempty"`
		}{tempAny.TypeUrl, string(b64)})
		if err != nil {
			return fmt.Errorf("failed to JSON marshal typeURL and base64 key: %w", err)
		}

		if err = ctx.Codec.UnmarshalInterfaceJSON(jsonPub, &pk); err != nil {
			return err
		}

		k, err := kb.SaveOfflineKey(name, pk)
		if err != nil {
			return fmt.Errorf("failed to save offline key: %w", err)
		}

		return printCreate(cmd, k, false, "", outputFormat)
	}

	coinType, _ := cmd.Flags().GetUint32(flagCoinType)
	account, _ := cmd.Flags().GetUint32(flagAccount)
	index, _ := cmd.Flags().GetUint32(flagIndex)
	hdPath, _ := cmd.Flags().GetString(flagHDPath)
	useLedger, _ := cmd.Flags().GetBool(flags.FlagUseLedger)

	// This is the diff point, the sdk verifies if the hd path in flag was empty,
	// and if it was it would assign something, we do allow empty hd path to be used.
	// https://github.com/cosmos/cosmos-sdk/blob/c64d1010800d60677cc25e2fca5b3d8c37b683cc/client/keys/add.go#L261
	if useLedger {
		return errors.New("cannot set custom bip32 path with ledger")
	}

	// If we're using ledger, only thing we need is the path and the bech32 prefix.
	if useLedger {
		bech32PrefixAccAddr := sdk.GetConfig().GetBech32AccountAddrPrefix()
		k, err := kb.SaveLedgerKey(name, hd.Secp256k1, bech32PrefixAccAddr, coinType, account, index)
		if err != nil {
			return err
		}

		return printCreate(cmd, k, false, "", outputFormat)
	}

	// Get bip39 mnemonic
	var mnemonic, bip39Passphrase string

	recoverFlag, _ := cmd.Flags().GetBool(flagRecover)
	mnemonicSrc, _ := cmd.Flags().GetString(flagMnemonicSrc)
	if recoverFlag {
		if mnemonicSrc != "" {
			mnemonic, err = readMnemonicFromFile(mnemonicSrc)
			if err != nil {
				return err
			}
		} else {
			mnemonic, err = input.GetString("Enter your bip39 mnemonic", inBuf)
			if err != nil {
				return err
			}
		}

		if !bip39.IsMnemonicValid(mnemonic) {
			return errors.New("invalid mnemonic")
		}
	} else if interactive {
		if mnemonicSrc != "" {
			mnemonic, err = readMnemonicFromFile(mnemonicSrc)
			if err != nil {
				return err
			}
		} else {
			mnemonic, err = input.GetString("Enter your bip39 mnemonic, or hit enter to generate one.", inBuf)
			if err != nil {
				return err
			}
		}

		if !bip39.IsMnemonicValid(mnemonic) && mnemonic != "" {
			return errors.New("invalid mnemonic")
		}
	}

	if len(mnemonic) == 0 {
		// read entropy seed straight from cmtcrypto.Rand and convert to mnemonic
		entropySeed, err := bip39.NewEntropy(mnemonicEntropySize)
		if err != nil {
			return err
		}

		mnemonic, err = bip39.NewMnemonic(entropySeed)
		if err != nil {
			return err
		}
	}

	// override bip39 passphrase
	if interactive {
		bip39Passphrase, err = input.GetString(
			"Enter your bip39 passphrase. This is combined with the mnemonic to derive the seed. "+
				"Most users should just hit enter to use the default, \"\"", inBuf)
		if err != nil {
			return err
		}

		// if they use one, make them re-enter it
		if len(bip39Passphrase) != 0 {
			p2, err := input.GetString("Repeat the passphrase:", inBuf)
			if err != nil {
				return err
			}

			if bip39Passphrase != p2 {
				return errors.New("passphrases don't match")
			}
		}
	}

	k, err := kb.NewAccount(name, mnemonic, bip39Passphrase, hdPath, algo)
	if err != nil {
		return err
	}

	// Recover key from seed passphrase
	if recoverFlag {
		// Hide mnemonic from output
		showMnemonic = false
		mnemonic = ""
	}

	return printCreate(cmd, k, showMnemonic, mnemonic, outputFormat)
}

func validateMultisigThreshold(k, nKeys int) error {
	if k <= 0 {
		return fmt.Errorf("threshold must be a positive integer")
	}
	if nKeys < k {
		return fmt.Errorf(
			"threshold k of n multisignature: %d < %d", nKeys, k)
	}
	return nil
}

func printCreate(cmd *cobra.Command, k *cryptokeyring.Record, showMnemonic bool, mnemonic, outputFormat string) error {
	switch outputFormat {
	case flags.OutputFormatText:
		cmd.PrintErrln()
		if err := printKeyringRecord(cmd.OutOrStdout(), k, keys.MkAccKeyOutput, outputFormat); err != nil {
			return err
		}

		// print mnemonic unless requested not to.
		if showMnemonic {
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "\n**Important** write this mnemonic phrase in a safe place.\nIt is the only way to recover your account if you ever forget your password.\n\n%s\n", mnemonic); err != nil {
				return fmt.Errorf("failed to print mnemonic: %s", err.Error())
			}
		}
	case flags.OutputFormatJSON:
		out, err := keys.MkAccKeyOutput(k)
		if err != nil {
			return err
		}

		if showMnemonic {
			out.Mnemonic = mnemonic
		}

		jsonString, err := json.Marshal(out)
		if err != nil {
			return err
		}

		cmd.Println(string(jsonString))

	default:
		return fmt.Errorf("invalid output format %s", outputFormat)
	}

	return nil
}

type bechKeyOutFn func(k *cryptokeyring.Record) (keys.KeyOutput, error)

func printKeyringRecord(w io.Writer, k *cryptokeyring.Record, bechKeyOut bechKeyOutFn, output string) error {
	ko, err := bechKeyOut(k)
	if err != nil {
		return err
	}

	switch output {
	case flags.OutputFormatText:
		if err := printTextRecords(w, []keys.KeyOutput{ko}); err != nil {
			return err
		}

	case flags.OutputFormatJSON:
		out, err := json.Marshal(ko)
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintln(w, string(out)); err != nil {
			return err
		}
	}

	return nil
}

func readMnemonicFromFile(filePath string) (string, error) {
	file, err := os.Open(filepath.Clean(filePath))
	if err != nil {
		return "", err
	}
	defer file.Close()

	bz, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return string(bz), nil
}

func printTextRecords(w io.Writer, kos []keys.KeyOutput) error {
	out, err := yaml.Marshal(&kos)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, string(out)); err != nil {
		return err
	}

	return nil
}
