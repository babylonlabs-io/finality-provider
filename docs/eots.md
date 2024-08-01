# EOTS Manager

## 1. Overview

The EOTS daemon is responsible for managing EOTS keys, producing EOTS randomness, and
using them to produce EOTS signatures.

**Note:** EOTS stands for Extractable One Time Signature. You can read more about it
in
the [Babylon BTC Staking Litepaper](https://docs.babylonchain.io/assets/files/btc_staking_litepaper-32bfea0c243773f0bfac63e148387aef.pdf).
In short, the EOTS manager produces EOTS public/private randomness pairs. The
finality provider commits the public part of these pairs to Babylon for every future
block height that they intend to provide a finality signature for. If the finality
provider votes for two different blocks on the same height, they will have to reuse
the same private randomness which will lead to their underlying private key being
exposed, leading to the slashing of them and all their delegators.

The EOTS manager is responsible for the following operations:

1. **EOTS Key Management:**
    - Generates [Schnorr](https://en.wikipedia.org/wiki/Schnorr_signature) key pairs
      for a given finality provider using the
      [BIP-340](https://github.com/bitcoin/bips/blob/master/bip-0340.mediawiki)
      standard.
    - Persists generated key pairs in the internal Cosmos keyring.
2. **Randomness Generation:**
    - Generates lists of EOTS randomness pairs based on the EOTS key, chainID, and
      block height.
    - The randomness is deterministically generated and tied to specific parameters.
3. **Signature Generation:**
    - Signs EOTS using the private key of the finality provider and the corresponding
      secret randomness for a given chain at a specified height.
    - Signs Schnorr signatures using the private key of the finality provider.

The EOTS manager functions as a daemon controlled by the `eotsd` tool.

## 2. Configuration

The `eotsd init` command initializes a home directory for the EOTS manager. This
directory is created in the default home location or in a location specified by
the `--home` flag.

```bash
eotsd init --home /path/to/eotsd/home/
```

After initialization, the home directory will have the following structure

```bash
ls /path/to/eotsd/home/
  ├── eotsd.conf # Eotsd-specific configuration file.
  ├── logs       # Eotsd logs
```

If the `--home` flag is not specified, then the default home location will be used.
For different operating systems, those are:

- **MacOS** `~/Users/<username>/Library/Application Support/Eotsd`
- **Linux** `~/.Eotsd`
- **Windows** `C:\Users\<username>\AppData\Local\Eotsd`

## 3. Keys Management

Handles the keys for EOTS.

### 3.1. Create EOTS Keys

The binary `eotsd` has the option to add a new key to the keyring for
later usage with signing EOTS and Schnorr signatures. Keep in mind
that new keys can be created on demand by the GRPC call from `fpd`.
But, if you would like to add a new EOTS key manually, run `eotsd keys add`.

This command has several flag options:

- `--home` specifies the home directory of the `eotsd` in which
the new key will be stored.
- `--key-name` mandatory flag and identifies the name of the key to be generated.
- `--passphrase` specifies the password used to encrypt the key, if such a
passphrase is required.
- `--hd-path` the hd derivation path of the private key.
- `--keyring-backend` specifies the keyring backend, any of `[file, os, kwallet, test, pass, memory]`
are available, by default `test` is used.
- `--recover` indicates whether the user wants to provide a seed phrase to recover
the existing key instead of randomly creating.

```shell
eotsd keys add --home /path/to/eotsd/home/ --key-name my-key-name --keyring-backend file

Enter keyring passphrase (attempt 1/3):
...

2024-04-25T17:11:09.369163Z     info    successfully created an EOTS key        {"key name": "my-key-name", "pk": "50b106208c921b5e8a1c45494306fe1fc2cf68f33b8996420867dc7667fde383"}
New key for the BTC chain is created (mnemonic should be kept in a safe place for recovery):
{
  "name": "my-key-name",
  "pub_key_hex": "50b106208c921b5e8a1c45494306fe1fc2cf68f33b8996420867dc7667fde383",
  "mnemonic": "bad mnemonic private tilt wish bulb miss plate achieve manage feel word safe dash vanish little miss hockey connect tail certain spread urban series"
}
```

> Store the mnemonic in a safe place. With the mnemonic only it is possible to
recover the generated keys by using the `--recover` flag.

### 3.2. Recover Keys

To recover the keys from a mnemonic, run:

```shell
eotsd keys add --home /path/to/eotsd/home/ --key-name my-key-name --keyring-backend file --recover

> Enter your mnemonic
bad mnemonic private tilt wish bulb miss plate achieve manage feel word safe dash vanish little miss hockey connect tail certain spread urban series
2024-04-25T17:13:29.681324Z     info    successfully created an EOTS key        {"key name": "my-key-name", "pk": "50b106208c921b5e8a1c45494306fe1fc2cf68f33b8996420867dc7667fde383"}
New key for the BTC chain is created (mnemonic should be kept in a safe place for recovery):
{
  "name": "my-key-name",
  "pub_key_hex": "50b106208c921b5e8a1c45494306fe1fc2cf68f33b8996420867dc7667fde383",
  "mnemonic": "noise measure tuition inform battle swallow slender bundle horn pigeon wage mule average bicycle claim solve home swamp banner idle chapter surround edit gossip"
}
```

You will be prompted to provide the mnemonic on key creation.

### 3.3. Sign Schnorr Signatures

You can use your key to create a Schnorr signature over arbitrary data
through the `eotsd sign-schnorr` command.
The command takes as an argument the file path, hashes the file content using
sha256, and signs the hash with the EOTS private key in Schnorr format by the
given `key-name` or `eots-pk`. If both flags `--key-name` and `--eots-pk` are
provided, `eots-pk` takes priority.

```shell
eotsd sign-schnorr /path/to/data/file --home /path/to/eotsd/home/ --key-name my-key-name --keyring-backend file
{
  "key_name": "my-key-name",
  "pub_key_hex": "50b106208c921b5e8a1c45494306fe1fc2cf68f33b8996420867dc7667fde383",
  "signed_data_hash_hex": "b123ef5f69545cd07ad505c6d3b4931aa87b6adb361fb492275bb81374d98953",
  "schnorr_signature_hex": "b91fc06b30b78c0ca66a7e033184d89b61cd6ab572329b20f6052411ab83502effb5c9a1173ed69f20f6502a741eeb5105519bb3f67d37612bc2bcce411f8d72"
}
```

### 3.4. Verify Schnorr Signatures

You can verify the Schnorr signature signed in the previous step through
the `eptsd veify-schnorr-sig` command.
The command takes as an argument the file path, hashes the file content using
sha256 to generate the signed data, and verifies the signature from the `--signature`
flag using the given public key from the `--eots-pk` flag.
If the signature is valid, you will see `Verification is successful!` in the output.
Otherwise, an error message will be printed out.

```shell
eotsd verify-schnorr-sig /path/to/data/file --eots-pk 50b106208c921b5e8a1c45494306fe1fc2cf68f33b8996420867dc7667fde383 \
--signature b91fc06b30b78c0ca66a7e033184d89b61cd6ab572329b20f6052411ab83502effb5c9a1173ed69f20f6502a741eeb5105519bb3f67d37612bc2bcce411f8d72 \
--keyring-backend file
```

## 4. Starting the EOTS Daemon

You can start the EOTS daemon using the following command:

```bash
eotsd start --home /path/to/eotsd/home
```

If the `--home` flag is not specified, then the default home location will be used.

This will start the EOTS rpc server at the address specified in `eotsd.conf` under
the `RpcListener` field, which is by default set to `127.0.0.1:12582`. You can change
this value in the configuration file or override this value and specify a custom
address using the `--rpc-listener` flag.

```bash
eotsd start

2024-02-08T17:59:11.467212Z	info	RPC server listening	{"address": "127.0.0.1:12582"}
2024-02-08T17:59:11.467660Z	info	EOTS Manager Daemon is fully active!
```

All the available cli options can be viewed using the `--help` flag. These options
can also be set in the configuration file.

**Note**: It is recommended to run the `eotsd` daemon on a separate machine or
network segment to enhance security. This helps isolate the key management
functionality and reduces the potential attack surface. You can edit the
`EOTSManagerAddress` in the configuration file of the finality provider to reference
the address of the machine where `eotsd` is running.
