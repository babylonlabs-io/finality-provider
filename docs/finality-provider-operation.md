# Finality Provider Operation

This is an operational guide intended for technical finality provider administrators.
This guide covers the complete
lifecycle of running a finality provider, including:

* Installing and configuring the finality provider toolset
  (EOTS Manager and Finality Provider daemon).
* Managing keys (EOTS key for EOTS signatures and Babylon Genesis key for rewards).
* Registering a finality provider on Babylon Genesis.
* Operating a finality provider.
* Withdrawing finality provider commission rewards.

Please review the [high-level explainer](../README.md) before proceeding to
gain an overall understanding of the finality provider.

## Table of Contents

1. [A note about Phase-1 Finality Providers](#1-a-note-about-phase-1-finality-providers)
2. [System Requirements](#2-system-requirements)
3. [Keys Involved in Finality Provider Operation](#3-keys-involved-in-finality-provider-operation)
4. [Install Finality Provider Toolset](#4-install-finality-provider-toolset)
5. [Setting up the EOTS Daemon](#5-setting-up-the-eots-daemon)
   1. [Initialize the EOTS Daemon](#51-initialize-the-eots-daemon)
   2. [Add an EOTS Key](#52-add-an-eots-key)
      1. [Create an EOTS key](#521-create-an-eots-key)
      2. [Import an existing EOTS key](#522-import-an-existing-eots-key)
   3. [Starting the EOTS Daemon](#53-starting-the-eots-daemon)
6. [Setting up the Finality Provider](#6-setting-up-the-finality-provider)
   1. [Initialize the Finality Provider Daemon](#61-initialize-the-finality-provider-daemon)
   2. [Add key for the Babylon Genesis account](#62-add-key-for-the-babylon-genesis-account)
   3. [Configure Your Finality Provider](#63-configure-your-finality-provider)
   4. [Starting the Finality Provider Daemon](#64-starting-the-finality-provider-daemon)
7. [Finality Provider Operation](#7-finality-provider-operations)
   1. [Create Finality Provider](#71-create-finality-provider)
   2. [Rewards](#72-rewards)
   3. [Set Up Operation Key](#73-set-up-operation-key)
   4. [Start Finality Provider](#74-start-finality-provider)
   5. [Status of Finality Provider](#75-status-of-finality-provider)
   6. [Edit finality provider](#76-edit-finality-provider)
   7. [Jailing and Unjailing](#77-jailing-and-unjailing)
   8. [Slashing](#78-slashing-and-anti-slashing)
   9. [Prometheus Metrics](#79-prometheus-metrics)
8. [Recovery and backup](#8-recovery-and-backup)
   1. [Critical assets](#81-critical-assets)
   2. [Backup recommendations](#82-backup-recommendations)
   3. [Recover finality-provider db](#83-recover-finality-provider-db)
      1. [Recover local status of a finality provider](#831-recover-local-status-of-a-finality-provider)
      2. [Recover public randomness proof](#832-recover-public-randomness-proof)

## 1. A note about Phase-1 Finality Providers

This note is for you if you have participated in Phase-1 of Babylon
Genesis to help you transition to Phase-2.

Finality providers that received delegations in Phase-1
are required to register their finality providers on Babylon Genesis
using the same EOTS key that they used and registered with during Phase-1.
The usage of a different key corresponds to setting up an entirely
different finality provider which will not inherit the Phase-1 delegations.
If you don't register your Phase-1 finality provider on Babylon Genesis, all the
received Phase-1 delegations will not be able to register on Babylon Genesis.

If you already have set up a key during Phase-1, you can proceed to
[Import an existing EOTS key](#522-import-an-existing-eots-key) to import
your Phase-1 key to the EOTS manager.

## 2. System Requirements

Recommended specifications for running a Finality Provider:

* CPU: 2 vCPUs
* RAM: 4GB
* Storage: 50GB SSD/NVMe
* Network: Stable internet connection
* Security:
  * Encrypted storage for keys and sensitive data
  * Regular system backups

These are the minimum specifications for running a finality provider.
Requirements may vary based on network activity and your operational needs.
For production environments, you may want to consider using more robust hardware.

For security tips of running a finality provider, please refer to
[Slashing Protection](./slashing-protection.md),
[HMAC Security](./hmac-security.md),
and [8. Recovery and Backup](#8-recovery-and-backup).

## 3. Keys Involved in Finality Provider Operation

Operating a finality provider involves managing multiple keys, each serving distinct purposes. Understanding these keys, their relationships, and security implications is crucial for secure operation.

| Aspect | EOTS Key                                                                                                                                                            | Babylon Genesis Key                                                                                                                                             | Operation Key                                                                                      |
|--------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| **Functions** | • Unique identifier of a finality provider for BTC staking<br>• Initial registration<br>• Signs finality votes and Schnorr signatures<br>• Generates randomness<br> | • Unique identifier of a finality provider for Babylon Genesis<br>• Initial registration<br>• Withdrawing accumulated rewards<br>• Setting withdrawal addresses | • Daily operational transactions<br>• Can be the same as the Babylon Genesis key (not recommended) |
| **Managed By** | `eotsd`                                                                                                                                                             | • `fpd`<br>• Can kept isolated after Operation Key is set                                                                                                       | `fpd`                                                                                              |
| **Mutability** | Immutable after registration                                                                                                                                        | Immutable after registration                                                                                                                                    | Rotatable (if separate from the Babylon Genesis key)                                               |
| **Key Relationships** | Permanently paired with Babylon Genesis Key during registration                                                                                                     | • Permanently paired with EOTS Key during registration<br>• Can delegate operations to Operation Key                                                            | • Not associated with the other keys<br>• Should be set after the finality provider is registered   |
| **Recomended Practices** | • Store backups in multiple secure locations<br>• Use dedicated machine for EOTS Manager                                                                            | • Store backups in multiple secure locations<br>• Setup the Operation Key right after registration<br>• Only use for reward operations                          | • Maintain minimal balance<br>• Monitor for balance and fund it when needed                        |
| **Security Implications** | • Loss is irrecoverable<br>• Cannot participate finality voting                                                                                                     | • Loss is irrecoverable<br>• Cannot withdraw rewards                                                                                                            | • Temporary service disruption<br>• Can be replaced with a new key<br>• Small loss of funds        |

Instructions of setting up the three keys can be found in the following places:

- [5.2. Add an EOTS Key](#52-add-an-eots-key).
- [6.2. Add key for the Babylon Genesis account](#62-add-key-for-the-babylon-genesis-account).
- [7.3. Set Up Operation Key](#73-set-up-operation-key).

## 4. Install Finality Provider Toolset

The finality provider toolset requires [Golang 1.23](https://go.dev/dl)
to be installed.
Please follow the installation instructions [here](https://go.dev/dl).
You can verify the installation with the following command:

```shell
go version
```

### 4.1. Clone the Finality Provider Repository

Subsequently, clone the finality provider
[repository](https://github.com/babylonlabs-io/finality-provider) and checkout
to the version you want to install.

```shell
git clone https://github.com/babylonlabs-io/finality-provider.git
cd finality-provider
git checkout <version>
```

### 4.2. Install Finality Provider Toolset Binaries

Run the following command to build the binaries and
install them to your `$GOPATH/bin` directory:

```shell
make install
```

This command will:

* Build and compile all Go packages.
* Install binaries to `$GOPATH/bin`:
  * `eotsd`: EOTS manager daemon
  * `fpd`: Finality provider daemon.
* Make commands globally accessible from your terminal.

### 4.3. Verify Installation

Run the following command to verify the installation:

```shell
fpd version
Version:       <version>
Git Commit:    <commit>
Git Timestamp: <timestamp>
```

If your shell cannot find the installed binaries, make sure `$GOPATH/bin` is in
the `$PATH` of your shell. Use the following command to add it to your profile
depending on your shell.

```shell
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
source ~/.profile
```

## 5. Setting up the EOTS Daemon

The EOTS manager daemon is a core component of the finality provider
stack responsible for managing your EOTS keys and producing EOTS signatures
to be used for votes. In this section, we are going to go through
its setup and key generation process.

### 5.1. Initialize the EOTS Daemon

If you haven't already, initialize a home directory for the EOTS Manager
with the following command:

```shell
eotsd init --home <path>
```

If the home directly already exists, `init` will not succeed.
> ⚡ Specifying `--force` to `init` will overwrite `eotsd.conf` with default
> config values if the home directory already exists.
> Please backup `eotsd.conf` before you run `init` with `--force`.

Parameters:

* `--home`: Directory for EOTS Manager configuration and data
  * Default: `DefaultEOTSDir` the default EOTS home directory:
    * `C:\Users\<username>\AppData\Local\ on Windows`
    * `~/.eotsd` on Linux
    * `~/Library/Application Support/Eotsd` on MacOS
  * Example: `--home ./eotsHome`

**Home directory structure:**

```shell
~/.eotsd/
├── config/
│   └── eotsd.conf      # Configuration file for the EOTS manager
├── data/
│   └── eotsd.db        # Database containing EOTS keys and mappings
├── keyring-*/          # Directory containing EOTS keyring data
└── logs/
    └── eotsd.log       # Log file for the EOTS manager daemon
```

* **eotsd.conf**
  This configuration file controls the core settings of the EOTS daemon.
  It uses the Cosmos SDK keyring system to securely store and manage EOTS keys.
  The file configures how the daemon interacts with its database, manages keys
  through the keyring backend, and exposes its RPC interface. Essential
  settings include database paths, keyring backend selection (test/file/os),
  RPC listener address, logging levels, and metrics configuration.

* **eotsd.db**:
  * EOTS key to key name mappings
  * EOTS signing history for [Slashing Protection](./slashing-protection.md).

* **keyring-directory***:
  * EOTS private keys are securely stored using Cosmos SDK's keyring system
  * `test` keyring-backend is mandatory for daemon access for automated signing.
  * Keys are used for EOTS signatures

* **eotsd.log**:
  * Key creation and import events
  * Signature generation requests
  * Error messages and debugging information
  * Service status updates

### 5.2. Add an EOTS Key

This section explains the process of creating EOTS keys using the EOTS manager.

The EOTS manager uses [Cosmos SDK](https://docs.cosmos.network/v0.50/user/run-node/keyring)
backends for key storage.
Since this key is accessed by an automated daemon process, it must be stored
unencrypted on disk and associated with the `test` keyring backend.
This ensures that we can access the eots keys when requested to promptly submit
transactions, such as block votes and public randomness submissions that are
essential for its liveness and earning of rewards.

If you already have an existing key from Phase-1, proceed to
[Import an existing EOTS key](#522-import-an-existing-eots-key)

#### 5.2.1. Create an EOTS key

If you have not created an EOTS key,
use the following command to create a new one:

``` shell
eotsd keys add <key-name> --home <path> --keyring-backend test
```

Parameters:

* `<key-name>`: Name for your EOTS key (e.g., "eots-key-1"). We do not allow
the same `keyname` for an existing keyname.
* `--home`: Path to your EOTS daemon home directory (e.g., "~/.eotsHome")
* `--keyring-backend`: Type of keyring storage (`test`)

The command will return a JSON response containing your EOTS key details:

```shell
{
    "name": "eots",
    "pub_key_hex":
    "e1e72d270b90b24f395e76b417218430a75683bd07cf98b91cf9219d1c777c19",
    "mnemonic": "parade hybrid century project toss gun undo ocean exercise
      figure decorate basket peace raw spot gap dose daring patch ski purchase
      prefer can pair"
}
```

> **🔒 Security Tip**: The mnemonic phrase must be stored securely and kept private.
> It is the only way to recover your EOTS key if you lose access to it and
> if lost it can be used by third parties to get access to your key.

#### 5.2.2. Import an existing EOTS key

> ⚡ This section is for Finality Providers who already possess an EOTS key.
> If you don't have keys or want to create new ones, you can skip this section.

There are 3 supported methods of loading your existing EOTS keys:

1. using a mnemonic phrase
2. importing the `.asc` file

We have outlined each of these three paths for you below.

#### Option 1: Using your mnemonic phrase

If you are using your mnemonic seed phrase, use the following command to import
your key:

```shell
eotsd keys add <key-name> --home <path> --recover --keyring-backend test
```

You'll be prompted to enter:

1. Your BIP39 mnemonic phrase (24 words)
2. HD path (optional - press Enter to use the default)

> ⚡ The HD path is optional. If you used the default path when creating your key,
you can skip this by pressing `Enter` , which by default uses your original private
key.

#### Option 2: Using your `.asc` file

If you exported your key to a `.asc` file. The `.asc` file should be in the
following format:

```shell
-----BEGIN TENDERMINT PRIVATE KEY-----
salt: 35ED0BBC00376EC7FC696838F34A7C36
type: secp256k1
kdf: argon2

8VOGhpuaZhTPZrKHysx24OhaxuBhVnKqb3WcTwJY+jvfNv/EJRoqmrHZfCnNgd13
VP88GFE=
=D87O
-----END TENDERMINT PRIVATE KEY-----
```

To load the key, use the following command:

```shell
eotsd keys import <name> <path-to-key> --home <path> --keyring-backend test
```

#### Verify the Key Import

After importing, you can verify that your EOTS key was successfully loaded:

```shell
eotsd keys list --home <path> --keyring-backend test
```

Parameters:

* `--keyring-backend`: Type of keyring backend to use (`test`)
* `--home`: Directory containing EOTS Manager configuration and data

You should see your EOTS key listed with the correct details if the import is
successful.

> ⚠️ **Important**:
> If you are a finality provider transitioning your stack from Phase-1,
> make sure that you are using the same EOTS key that you
> registered in Phase-1.

### 5.3. Starting the EOTS Daemon

To start the EOTS daemon, use the following command:

```shell
eotsd start --home <path>
```

This command starts the EOTS RPC server at the address specified in `eotsd.conf`
under the `RPCListener` field (default: `127.0.0.1:12582`). You can override
this value by specifying a custom address with the `--rpc-listener` flag.

```shell
2024-10-30T12:42:29.393259Z     info  Metrics server is starting
{"addr": "127.0.0.1:2113"}
2024-10-30T12:42:29.393278Z     info  RPC server listening{"address": "127.0.0.1:12582"}
2024-10-30T12:42:29.393363Z     info  EOTS Manager Daemon is fully active!
EOTS Manager Daemon is fully active!
```

>**🔒 Security Tip**:
>
> * `eotsd` holds your private keys which are used for signing
> * operate the daemon in a separate machine or network segment
>   with enhanced security
> * only allow access to the RPC server specified by the `RPCListener`
>   port to trusted sources. You can edit the `EOTSManagerAddress` in
>   the configuration file of the finality provider to
>   reference the address of the machine where `eotsd` is running
> * setup HMAC to secure the communication between `eotsd` and `fpd`. See
>   [HMAC Security](./hmac-security.md).

## 6. Setting up the Finality Provider

### 6.1. Initialize the Finality Provider Daemon

To initialize the finality provider daemon home directory,
use the following command:

```shell
fpd init --home <path>
```

If the home directory already exists, `init` will not succeed.

> ⚡ Running this command with `--force` will overwrite the existing config file.
> Please ensure you have a backup of the existing config file before running
> this command.

**Home directory structure:**

```shell
~/.fpd/
├── config/
│   └── fpd.conf       # Configuration file for the finality provider
├── data/
│   └── finality-provider.db         # Database containing finality provider data
├── keyring-*/         # Directory containing Babylon Genesis keys
└── logs/
    └── fpd.log        # Log file for the finality provider daemon
```

* **fpd.conf**: The main configuration file that defines:
  * Network settings (chain-id, node endpoints)
  * EOTS manager connection settings
  * Database configuration
  * Logging settings
  * RPC listener settings
  * Metrics configuration

* **finality-provider.db**: A bbolt database that stores:
  * Finality provider registration data
  * Public randomness proofs
  * Last voted block height

* **keyring-*** directory: Contains your Babylon Genesis account keys used for:
  * Submitting finality signatures to Babylon
  * Withdrawing rewards
  * Managing your finality provider
  * Loss means inability to operate until restored

* **fpd.log**: Contains detailed logs including:
  * Block monitoring events
  * Signature submissions
  * Error messages and debugging information
  * Service status updates

### 6.2. Add key for the Babylon Genesis account

Each finality provider maintains a Babylon Genesis keyring containing
an account used to receive BTC Staking reward commissions and pay fees for
transactions necessary for the finality provider's operation.

Since this key is accessed by an automated daemon process, it must be stored
unencrypted on disk and associated with the `test` keyring backend.
This ensures that the finality provider daemon can promptly submit
transactions, such as block votes and public randomness submissions that are
essential for its liveness and earning of rewards.

It is also important to note that the finality provider daemon will refund
fees for the submission of valid votes as those are essential for the protocol.
All other transactions, will require gas, but will be happening infrequently
or only once. As this keyring is used for both earning and
operational purposes, we strongly recommend maintaining only the necessary
funds for operations in the keyring, and extracting the rest into
more secure locations.

> ⚠️ **Important**:
> To operate your Finality Provider, ensure your Babylon Genesis account is funded.
> Block vote transactions have their gas fees refunded, but public randomness
> submissions require gas payments. For testnet, you can obtain funds from our
> testnet faucet.

Use the following command to add the Babylon Genesis key for your finality provider:

```shell
fpd keys add <key-name> --keyring-backend test --home <path>
```

The above `keys add` command will create a new key pair and store it in your keyring.
The output should look similar to the one below:

``` json
{
  "address": "bbn19gulf0a4yz87twpjl8cxnerc2wr2xqm9fsygn9",
  "name": "finality-provider",
  "pubkey": {
    "@type": "/cosmos.crypto.secp256k1.PubKey",
    "key": "AhZAL00gKplLQKpLMiXPBqaKCoiessoewOaEATKd4Rcy"
  },
  "type": "local"
}
```

### 6.3. Configure Your Finality Provider

Edit the `fpd.conf` file in your finality provider home directory with the
following parameters:

```shell
[Application Options]
EOTSManagerAddress = 127.0.0.1:12582
RPCListener = 127.0.0.1:12581

[babylon]
Key = <finality-provider-key-name-signer> # the key you used above
ChainID = bbn-test-5 # chain ID of the Babylon Genesis
RPCAddr = http://127.0.0.1:26657 # Your Babylon Genesis node's RPC endpoint
KeyDirectory = <path> # The `--home` path to the directory where the keyring is stored
```

> ⚠️ **Important**: Operating a finality provider requires a connection to a
> Babylon Genesis node. It is **highly recommended** to operate your own
> Babylon Genesis full node instead of relying on third parties. You can find
> instructions on setting up a Babylon Genesis node
> [here](https://github.com/babylonlabs-io/networks/tree/main/bbn-test-5/babylon-node/README.md).
>
> ⚠️ **Critical RPC Configuration**:
> When configuring your finality provider to a Babylon Gensis RPC node, you should
> connect to a **single** node directly. Additionally you **must**
> ensure that this node has transaction indexing enabled (`indexer = "kv"`).
> Using multiple RPC nodes or load balancers can lead to sync issues.

Configuration parameters explained:

* `EOTSManagerAddress`: Address where your EOTS daemon is running
* `RPCListener`: Address for the finality provider RPC server
* `Key`: Your Babylon Genesis key name from Step 2
* `ChainID`: The Babylon Genesis chain ID
* `RPCAddr`: Your Babylon Genesis node's RPC endpoint
* `KeyDirectory`: Path to your keyring directory (same as `--home` path)

Please verify the `chain-id` and other network parameters from the official
[Babylon Genesis Networks
repository](https://github.com/babylonlabs-io/networks).

Another notable configurable parameter is `NumPubRand`, which is the number of
public randomness that will be generated and submitted in one commit to Babylon
Genesis. This value is set to `50,000` by default, which is sufficient for
roughly 5 days of usage with block production time at `10s`.
Larger values can be set to tolerate longer downtime with larger size of
merkle proofs for each randomness, resulting in higher gas fees when submitting
future finality signatures and larger storage requirements.

### 6.4. Starting the Finality Provider Daemon

The finality provider daemon (FPD) needs to be running before proceeding with
registration or voting participation.

Start the daemon with:

``` shell
fpd start --home <path>
```

An example of the `--home` flag is `--home ./fpHome`.

The command flags:

* `start`: Runs the FPD daemon
* `--home`: Specifies the directory for daemon data and configuration
* `--eots-pk`: The finality provider instance that will be started identified
  by the EOTS public key.

It will start the finality provider daemon listening for registration and other
operations. If there is already a finality provider created (described in a
later [section](#71-create-finality-provider)), `fpd start` will also start
the finality provider. If there are multiple finality providers created,
`--eots-pk` is required.

The daemon will establish a connection with the Babylon Genesis node and
boot up its RPC server for executing CLI requests.

You should see logs indicating successful startup:

```shell
[INFO] Starting finality provider daemon...
[INFO] RPC server listening on...
```

> ⚠️ **Important**: The daemon needs to run continuously. It's recommended to set
> up a system service (like `systemd` on Linux or `launchd` on macOS) to manage
> the daemon process, handle automatic restarts, and collect logs.

The above will start the Finality provider RPC server at the address specified
in `fpd.conf` under the `RPCListener` field, which has a default value
of `127.0.0.1:12581`. You can change this value in the configuration file or
override this value and specify a custom address using
the `--rpc-listener` flag.

All the available CLI options can be viewed using the `--help` flag. These
options can also be set in the configuration file.

### 5.5. Interaction with the EOTS Manager

There are two pieces to a finality provider entity: the EOTS manager and the
finality provider instance. These components work together and are managed by
separate daemons (`eotsd` and `fpd`).

The EOTS manager is responsible for managing the keys for finality providers and
handles operations such as key management, signature generation, and randomness
commitments. Whereas the finality provider is responsible for creating and
registering finality providers and handling the monitoring of the Babylon Genesis.
The finality provider daemon is also responsible for coordinating various
operations.

The interactions between the EOTS Manager and the finality provider happen
through RPC calls. These calls handle key operations, signature generation,
and randomness commitments. An easy way to think about it is the EOTS Manager
maintains the keys while the FP daemon coordinates any interactions with the
Babylon Genesis.

The EOTS Manager is designed to handle multiple finality provider keys, operating
as a centralized key management system. When starting a finality provider instance,
you specify which EOTS key to use through the `--eots-pk` flag. This allows you
to run different finality provider instances using different keys from the same
EOTS Manager. Note that someone having access to your EOTS Manager
RPC will have access to all the EOTS keys held within it.

For example, after registering a finality provider, you can start its daemon by
providing the EOTS public key `fpd start --eots-pk <hex-string-of-eots-public-key>`.
Note that a single finality provider daemon can only run with a single
finality provider instance at a time.

## 7. Finality Provider Operations

### 7.1. Create Finality Provider

The `create-finality-provider` command initializes a new finality provider,
submits `MsgCreateFinalityProvider` to register it on Babylon Genesis, and
saves the finality provider information in the database.

``` shell
fpd create-finality-provider \
  --chain-id <chain-id> \
  --eots-pk <eots-pk-hex> \
  --commission-rate 0.05 \
  --key-name finality-provider \
  --moniker "MyFinalityProvider" \
  --website "https://myfinalityprovider.com" \
  --security-contact "security@myfinalityprovider.com" \
  --details "finality provider for the Babylon Genesis network" \
  --home ./fpHome
```

Required parameters:

* `--chain-id`: The Babylon Genesis chain ID (e.g., `bbn-1` and `bbn-test-5`
  for mainnet and testnet, respectively).
* `--eots-pk`: The EOTS public key maintained by the connected EOTS manager
  instance that the finality provider should use. If one is not provided the
  finality provider will request the creation of a new one from the connected
  EOTS manager instance.
* `--commission`: The commission rate (between 0 and 1) that you'll receive from
  delegators
* `--key-name`: The key name in your Babylon Genesis keyring that your finality
  provider will be associated with
* `--moniker`: A human-readable name for your finality provider
* `--home`: Path to your finality provider daemon home directory

> ⚠️ **Important**: The EOTS key and the Babylon Genesis key used in registration is
> unique to the finality provider after registration. Either key cannot be
> rotated. The EOTS key is for sending finality signatures while the latter is
> for withdrawing rewards. You **MUST** keep both keys safe.

Optional parameters:

* `--website`: Your finality provider's website
* `--security-contact`: Contact email for security issues
* `--details`: Additional description of your finality provider
* `--daemon-address`: RPC address of the finality provider daemon
  (default: `127.0.0.1:12581`)

Alternatively, you can create a finality provider by providing a JSON file
with the finality provider details, similar to the following:

```json
{
  "keyName": "The unique key name of the finality provider's Babylon Genesis account",
  "chainID": "The identifier of the consumer chain",
  "passphrase": "The pass phrase used to encrypt the keys",
  "commissionRate": "The commission rate for the finality provider, e.g., 0.05",
  "moniker": "A human-readable name for the finality provider",
  "identity": "A optional identity signature",
  "website": "Validator's (optional) website",
  "securityContract": "Validator's (optional) security contact email",
  "details": "Validator's (optional) details",
  "eotsPK": "The hex string of the finality provider's EOTS public key"
}
```

To create a finality provider using the JSON file, you can use the following command:

```shell
fpd create-finality-provider --from-file <path-to-json-file>
```

Upon successful creation, the command will return a JSON response containing
your finality provider's details:

``` json
{
    "finality_provider":
    {
      "fp_addr": "bbn1ht2nxa6hlyl89m8xpdde9xsj40n0sxd2f9shsq",
      "eots_pk_hex":
      "cf0f03b9ee2d4a0f27240e2d8b8c8ef609e24358b2eb3cfd89ae4e4f472e1a41",
      "description":
      {
        "moniker": "MyFinalityProvider",
        "website": "https://myfinalityprovider.com",
        "security_contact": "security@myfinalityprovider.com",
        "details": "finality provider for the Babylon Genesis network"
      },
      "commission": "0.050000000000000000",
      "status": "REGISTERED"
    }
    "tx_hash": "C08377CF289DF0DC5FA462E6409ADCB65A3492C22A112C58EA449F4DC544A3B1"
}
Your finality provider is successfully created. Please restart your fpd.
```

The response includes:

* `fp_addr`: Your Babylon Genesis account address of the finality provider
* `eots_pk_hex`: Your unique EOTS public key identifier
* `description`: Your finality provider's metadata
* `commission`: Your set commission rate
* `status`: Current status of the finality provider.
* `tx_hash`: Babylon Genesis transaction hash of the finality provider creation
  transaction, which you can use to verify the success of the transaction
  on Babylon Genesis.

### 7.2. Rewards

Rewards are accumulated in a reward gauge, and a finality provider becomes
eligible for rewards if it has participated sending finality votes.
The distribution of rewards is based on the provider's voting power portion
relative to other voters.

#### 7.2.1. Querying Rewards

To query rewards of a given stakeholder address, use the following command.

```shell
fpd reward-gauges <address> --node <babylon-genesis-rpc-address>
```

Parameters:

* `<address>`: The Babylon Genesis address of the stakeholder in bech32 string.
* `--node <babylon-genesis-rpc-address>`: <host>:<port> to Babylon Genesis
RPC interface for this chain (default `tcp://localhost:26657`)

#### 7.2.2. Withdraw Rewards

The `fpd withdraw-reward` command will withdraw all accumulated rewards of the
given finality provider. The finality provider must first be active and have
sent finality votes to be eligible to receive rewards.

```shell
fpd withdraw-reward <type> --from <registered-bbn-address>
--keyring-backend test --home <home-dir> --fees <fees>
--node <babylon-genesis-rpc-address>
```

> ⚠️ **Important**: The `fpd` should be **stopped** before performing this action.
> otherwise, account sequence mismatch error might be encountered because the key
> used for sending the withdrawal transaction is under use by the finality provider
> sending operational transaction. This issue will be resolved after following the
> setup instructions in [7.3. Set Up Operation Key](#73-set-up-operation-key).

The rewards will go to `<registered-bbn-address>` by default. If you want to
set a different address to receive rewards, please refer to
[7.2.3. Set Withdraw Address](#723-set-withdraw-address). But still, the
registration key is always needed when withdrawing the rewards. So the
registration key **MUST** be kept safe.

Parameters:

* `<type>`: The type of reward to withdraw (one of `finality_provider`,
  `btc_delegation`)
* `--from <registered-bbn-address>`: The finality provider's BABY address used
  in registration.
* `--keyring-backend`: The keyring backend to use.
* `--home`: The home directory for the finality provider.
* `--fees`: The fees to pay for the transaction, should be over `400ubbn`.
  These fees are paid from the account specified in `--from`.
* `--node <babylon-genesis-rpc-address>`: <host>:<port> to Babylon Genesis
    RPC interface for this chain (default `tcp://localhost:26657`).

Again, this command should ask to
`confirm transaction before signing and broadcasting [y/N]:` and output the
transaction hash.

This will withdraw **ALL** accumulated rewards to the address you set in the
`set-withdraw-addr` command if you set one. If no withdrawal address was set,
the rewards will be withdrawn to the finality provider's `BABY` address used
in registration.

#### 7.2.3. Set Withdraw Address

The default beneficiary is the address that corresponds to the registration key.
To change the beneficiary address, use the following command:

```shell
fpd set-withdraw-addr <beneficiary-address> --from <registered-bbn-address>
--keyring-backend test --home <home-dir> --fees <fees>
--node <babylon-genesis-rpc-address>
```

Note that change of the beneficiary address is unlimited but for every change,
the registration key is always needed.

Parameters:

* `<beneficiary-address>`: Corresponds to the beneficiary key and is where
  withdraw rewards are sent to.
* `<registered-bbn-address>`: Corresponds to the key used in registration and is where
  withdraw rewards are sent to by default if no other address is set via `set-withdraw-addr`
* `--from`: The finality provider's registered Babylon Genesis address.
* `--keyring-backend`: The keyring backend to use.
* `--home`: The home directory for the finality provider.
* `--fees`: The fees to pay for the transaction, should be over `400ubbn`.
  These fees are paid from the account specified in `--from`.
* `--node <babylon-genesis-rpc-address>`: <host>:<port> to Babylon Genesis
    RPC interface for this chain (default `tcp://localhost:26657`).

This command should ask to
`confirm transaction before signing and broadcasting [y/N]:` and output the
transaction hash.

### 7.3. Set Up Operation Key

Finality providers consume gas for operations with Babylon Genesis.
Given that the cost of sending finality signatures is refunded automatically
after success, the only cost of operating a finality provider is randomness
commit, which is made periodically with low gas cost.

Therefore, it is recommended to use a separate key specifically for operations.
This leaves the key used for registration isolated and only needed for withdrawing
rewards. The operation key is totally replaceable in the sense that it is not
tied to a finality provider, and it can be replaced by any key that is properly
funded.
Therefore, it can hold only a minimum amount of funds to keep the finality
provider running for a long period of time.

You may follow the following procedure to set up the operation key.

1. Create an additional key for operation following steps in
   [6.2 Add key for the Babylon Genesis account](#62-add-key-for-the-babylon-genesis-account).

2. Fund the operation key with minimum BABY tokens for gas cost.

3. Set the operation key name (`Key`) and the keyring directory (`KeyDirectory`)
   under `[babylon]` in `fpd.conf`.

4. Restart the finality provider daemon.

> 💡 **Tip**: Committing randomness has a constant cost of `0.00013 BABY` per
> commit. Therefore, reserving `5-10 BABY` for operations should be enough for a
> long time.

### 7.4. Start Finality Provider

After successful registration and properly set up the operation key,
you may start the finality provider instance by running:

```shell
fpd start --eots-pk <hex-string-of-eots-public-key>
```

If `--eots-pk` is not specified, the command will start the finality provider
if it is the only one stored in the database. If multiple finality providers
are in the database, specifying `--eots-pk` is required.

### 7.5. Status of Finality Provider

Once the finality provider has been created, it will have the `REGISTERED` status.

Below you can see a list of the statuses that a finality provider can transition
to:

* `REGISTERED`: defines a finality provider that has been created and registered
  to the consumer chain but has no delegated stake
* `ACTIVE`: defines a finality provider that is delegated to vote
* `INACTIVE`: defines a finality provider whose delegations are reduced to
  zero but not slashed
* `JAILED`: defines a finality provider that has been jailed
* `SLASHED`: Defines a finality provider that has been permanently removed from
  the network for double signing (signing conflicting blocks at the same height).
  This state is irreversible.

To check the status of a finality provider, you can use the following command:

```shell
fpd finality-provider-info <hex-string-of-eots-public-key>
```

This will return the same response as the `create-finality-provider`
command but you will be able to check in real time the status of the
finality provider.

For more information on status transition, please refer to diagram in the core
documentation[fp-core](fp-core.md).

### 7.6. Edit Finality Provider

If you need to edit your finality provider's information, you can use the
following command:

```shell
fpd edit-finality-provider <hex-string-of-eots-public-key> \
  --commission-rate <commission-rate> \
  --home <path-to-fpd-home-dir>
  # Add any other parameters you would like to modify
```

Parameters:

* `<hex-string-of-eots-public-key>`: The EOTS public key of the finality provider
* `--commission-rate`: A required flag for the commission rate for the finality
  provider
* `--home`: An optional flag for the path to your finality provider daemon home
  directory

Parameters you can edit:

* `--moniker`: A human-readable name for your finality provider
* `--website`: Your finality provider's website
* `--security-contact`: Contact email for security issues
* `--details`: Additional description of your finality provider

You can then use the following command to check if the finality provider has been
edited successfully:

```shell
fpd finality-provider-info <hex-string-of-eots-public-key>
```

### 7.7. Jailing and Unjailing

When jailed, the following happens to a finality provider:

* Their voting power becomes `0`.
* Status is set to `JAILED`.
* Delegations and finality provider rewards stop accumulating.

To unjail a finality provider, you must complete the following steps:

1. Fix the underlying issue that caused jailing (e.g., ensure your node is
   properly synced and voting)
2. Wait for the jailing period to pass (defined by finality module parameters)
3. Send the unjail transaction to Babylon Genesis using the following command:

```shell
fpd unjail-finality-provider <eots-pk> --daemon-address <rpc-address> --home <path>
```

Parameters:

* `<eots-pk>`: Your finality provider's EOTS public key in hex format
* `--daemon-address`: RPC server address of fpd (default: `127.0.0.1:12581`)
* `--home`: Path to your finality provider daemon home directory

> ⚠️ Before unjailing, ensure you've fixed the underlying issue that caused jailing

If unjailing is successful, you may start running the finality provider by
`fpd start --eots-pk <hex-string-of-eots-public-key>`.

### 7.8. Slashing and Anti-slashing

**Slashing occurs** when a finality provider **double signs**, meaning that the
finality provider signs conflicting blocks at the same height. This results in
the extraction of the finality provider's private key and their immediate
removal from the active set. For details about how the slashing works in the
BTC staking protocol, please refer to our [light paper](https://docs.babylonlabs.io/papers/btc_staking_litepaper(EN).pdf).

> ⚠️ **Critical**: Slashing is irreversible and the finality provider can
> no longer gain voting power from the network.

Apart from malicious behavior, honest finality providers face [slashing risks](https://cubist.dev/blog/slashing-risks-you-need-to-think-about-when-restaking)
due to factors like hardware failures or software bugs.
Therefore, a proper slashing protection mechanism is required.
For details about how our built-in anti-slashing works, please refer to
our technical document [Slashing Protection](../docs/slashing-protection.md).

### 7.9. Prometheus Metrics

The finality provider exposes Prometheus metrics for monitoring your
finality provider. The metrics endpoint is configurable in `fpd.conf`:

#### Core Metrics

1. **Status for Finality Providers**
   * `fp_status`: Current status of a finality provider
   * `babylon_tip_height`: The current tip height of the Babylon Genesis network
   * `last_polled_height`: The most recent block height checked by the poller

2. **Key Operations**
   * `fp_seconds_since_last_vote`: Seconds since the last finality sig vote
   * `fp_seconds_since_last_randomness`: Seconds since the last public
      randomness commitment
   * `fp_total_failed_votes`: The total number of failed votes
   * `fp_total_failed_randomness`: The total number of failed
      randomness commitments

Each metric with `fp_` prefix includes the finality provider's BTC public key
hex as a label.

> 💡 **Tip**: Monitor these metrics to detect issues before they lead to jailing:
>
> * Large gaps in `fp_seconds_since_last_vote`
> * Increasing `fp_total_failed_votes`

For a complete list of available metrics, see:

* Finality Provider metrics: [fp_collectors.go](../metrics/fp_collectors.go)
* EOTS metrics: [eots_collectors.go](../metrics/eots_collectors.go)

## 8. Recovery and Backup

### 8.1. Critical Assets

The following assets **must** be backed up frequently to prevent loss of service or funds:

For EOTS Manager:

* **keyring-*** directory: Contains your EOTS private keys used for signing. Loss of these keys means:
  * Unable to sign finality signatures
  * Unable to recover your finality provider identity
  * Permanent loss of your finality provider position
* **eotsd.db**: Contains key mappings and metadata. While less critical, loss means:
  * Need to re-register key mappings
  * Temporary service interruption
  * Loss of anti-slashing protection

For Finality Provider:

* **keyring-*** directory: Contains your Babylon Genesis account keys used for:
  * Submitting finality signatures to Babylon
  * Withdrawing rewards
  * Managing your finality provider
  * Loss means inability to operate until restored
* **finality-provider.db**: Contains operational data including:
  * Public randomness proofs
  * State info of the finality provider
  * Loss of anti-slashing protection

### 8.2. Backup Recommendations

1. Regular Backups:
   * Daily backup of keyring directories
   * Weekly backup of full database files
   * Store backups in encrypted format
   * Keep multiple backup copies in separate locations

2. Critical Times for Backup:
   * After initial setup
   * Before any major updates
   * After key operations
   * After configuration changes

3. Recovery Testing:
   * Regularly test recovery procedures
   * Maintain documented recovery process
   * Practice key restoration in test environment

> 🔒 **Security Note**: While database files can be recreated, loss of private
> keys in the keyring directories is **irrecoverable** and will result in
> permanent loss of your finality provider position and accumulated rewards.

### 8.3. Recover finality-provider db

The `finality-provider.db` file contains both the finality provider's running
status and the public randomness merkle proof. Either information loss
compromised will lead to service halt, but they are recoverable.

#### 8.3.1. Recover local status of a finality provider

The local status of a finality provider is defined as follows:

```go
type StoredFinalityProvider struct {
  FPAddr          string
  BtcPk           *btcec.PublicKey
  Description     *stakingtypes.Description
  Commission      *sdkmath.LegacyDec
  ChainID         string
  LastVotedHeight uint64
  Status          proto.FinalityProviderStatus
}
```

It can be recovered by downloading the finality provider's info from Babylon Genesis. Specifically, this can be achieved by repeating the
[creation process](#71-create-finality-provider). The `create-finality-provider`
cmd will download the info of the finality provider locally if it is already
registered on Babylon.

#### 8.3.2. Recover public randomness proof

Every finality vote must contain the public randomness proof to prove that the
randomness used in the signature is already committed on Babylon. Loss of
public randomness proof leads to direct failure of the vote submission.

To recover the public randomness proof, the following steps should be followed:

1. Ensure the `fpd` is stopped.
2. Unjail your finality provider if needed.
3. Run the recovery command
`fpd recover-rand-proof [eots-pk-hex] --start-height [height-to-recover] --chain-id [chain-id]`
where `start-height` is the height from which you want to recover from. If
the `start-height` is not specified, the command will recover all the proofs
from the first commit on Babylon, which incurs longer time for recovery.
The `chain-id` must be specified exactly the same as the `chain-id` used when
creating the finality provider.
4. Restart the finality provider
