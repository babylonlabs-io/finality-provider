# Finality Provider Operation

This document guides operators through the complete
lifecycle of running a finality provider, including:

* Installing and configuring the finality provider toolset
  (EOTS Manager and Finality Provider daemon)
* Managing keys (EOTS key for signatures and Babylon key for rewards)
* Registering your finality provider on the Babylon network
* Operating and maintaining your finality provider
* Collecting rewards

This is an operational guide intended for technical finality provider administrators.
For conceptual understanding, see our [Technical Documentation](./docs/fp-core.md). 
Please review the [high-level explainer](./README.md) before proceeding to 
gain an overall understanding of the finality provider. 

## Table of Contents

1. [A note about Phase-1 Finality Providers](#1-a-note-about-phase-1-finality-providers)
2. [Install Finality Provider Toolset](#2-install-finality-provider-toolset)
3. [Setting up the EOTS Daemon](#3-setting-up-the-eots-daemon)
   1. [Initialize the EOTS Daemon](#31-initialize-the-eots-daemon)
   2. [Create/Add an EOTS Key](#32-createadd-an-eots-key)
      1. [Import an existing EOTS key](#321-import-an-existing-eots-key)
      2. [Create an EOTS key](#322-create-an-eots-key)
   3. [Starting the EOTS Daemon](#33-starting-the-eots-daemon)
4. [Setting up the Finality Provider](#4-setting-up-the-finality-provider)
   1. [Initialize the Finality Provider Daemon](#41-initialize-the-finality-provider-daemon)
   2. [Add key for the Babylon account](#42-add-key-for-the-babylon-account)
   3. [Configure Your Finality Provider](#43-configure-your-finality-provider)
   4. [Starting the Finality Provider Daemon](#44-starting-the-finality-provider-daemon)
5. [Finality Provider Operation](#5-finality-provider-operations)
   1. [Create Finality Provider](#51-create-finality-provider)
   2. [Register Finality Provider](#52-register-finality-provider)
   3. [Withdrawing Rewards](#53-withdrawing-rewards)
   4. [Jailing and Unjailing](#54-jailing-and-unjailing)
   5. [Slashing](#55-slashing)
6. [Prometheus Metrics](#6-prometheus-metrics)

## 1. A note about Phase-1 Finality Providers

Thank you for participating in the first phase of the Babylon launch. This guide 
provides instructions for setting up the full finality provider toolset required 
for your participation in the second phase of the Babylon launch.

To ensure a safe and seamless transition without any loss of delegations, we 
recommend completing the following setup tasks. First and foremost, it is 
essential that you use the same key from Phase-1, as your delegations are 
exclusively tied to this canonical key.

- If you already have delegations and have set up a key during Phase-1, 
  please proceed to [Adding Keys](#32-createadd-an-eots-key) to import your 
  Phase-1 key.
- If you are just starting out, you can begin with the setup steps outlined below.

## 2. Install Finality Provider Toolset 
<!-- TODO: check add in the correct tag for the testnet --> 

The finality provider toolset requires Golang 1.23](https://go.dev/dl)
to be installed.
Please follow the installation instructions [here](https://go.dev/dl).
You can verify the installation with the following command:

```shell 
go version 
```

### 2.1. Clone the Finality Provider Repository

Subsequently, clone the finality provider 
[repository](https://github.com/babylonlabs-io/finality-provider) and navigate 
to the `bbn-test-5` tag. 
<!-- TODO: change to a specific version -->

```shell 
git clone https://github.com/babylonchain/finality-provider.git
cd finality-provider
git checkout <tag>
```

### 2.2. Build and Install Finality Provider Toolset Binaries

Run the following command to build the binaries and
install them to your `$GOPATH/bin` directory:

```shell 
make install 
```

This command will:
- Build and compile all Go packages
- Install binaries to `$GOPATH/bin`:
  - `eotsd`: EOTS manager daemon
  - `fpd`: Finality provider daemon
- Make commands globally accessible from your terminal

### 2.3. Verify Installation 
Run the following command to verify the installation:

```shell 
fpd version
``` 

The expected output should be:

```shell
# example output
version: v0.11.0 
commit: 7d37c88e9a33c0b6a86614f743e9426ce6e31d4a
```

If your shell cannot find the installed binaries, make sure `$GOPATH/bin` is in
the `$PATH` of your shell. Use the following command to add it to your profile 
depending on your shell.

```shell 
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
```

## 3. Setting up the EOTS Daemon

The EOTS manager daemon is a core component of the finality provider
stack responsible for managing your EOTS keys and producing EOTS signatures
to be used for votes. In this section, we are going to go through
its setup and key generation process.

### 3.1. Initialize the EOTS Daemon

If you haven't already, initialize a home directory for the EOTS Manager 
with the following command:

```shell
eotsd init --home <path>
```

Parameters:
- `--home`: Directory for EOTS Manager configuration and data
  - Default: `/Users/<username>/Library/Application Support/Eotsd`
  - Example: `--home ./eotsHome`

### 3.2. Create/Add an EOTS Key

This section explains the process of setting up the private keys for the 
EOTS manager. Operators *must* create an EOTS key before starting the 
EOTS daemon. We will be using the [cosmos-sdk](https://docs.cosmos.network/v0.52/user/run-node/keyring) backends for key storage. Below is an overview of the supported keyrings and their
 usecases.

For storing the key, the following keyrings are supported: `test`, `os`, `file`. 
- `test` is a password-less keyring and is unencrypted on disk.
- `os` uses the system's secure keyring and will prompt for a passphrase at startup.
- `file` encrypts the keyring with a passphrase and stores it on disk.

Operators can choose whether to keep the key encrypted or unencrypted on disk 
based on their requirements. However, for production workloads, we recommend 
using the os or file keyrings for enhanced security.

For consistency purposes, we will be using the `test` keyring throughout the 
rest of the document.

- If you already have an existing key from Phase-1, proceed to 
  [Import an existing EOTS key](#321-import-an-existing-eots-key)
- If you need to create a new key, proceed to [Create an EOTS key](#32-createadd-an-eots-key).

#### 3.2.1. Import an existing EOTS key

> ⚡ This section is for Finality Providers who participated in Phase-1 and
> already possess an EOTS key. If you are a new user, you can skip this section.

There are 3 supported methods of loading your existing EOTS keys: (1) using a 
mnemonic phrase, (2) exporting the `.asc` file, or (3) backing up your entire 
home directory. We have outlined each of these three paths for you below.

#### Option 1: Using your Mnemonic Phrase

If you are using your mnemonic seed phrase, use the following command to import 
your key:

```shell
eotsd keys add <key-name> --home <path> --recover --keyring-backend test
```

You'll be prompted to enter:
1. Your BIP39 mnemonic phrase (24 words)
2. A passphrase to encrypt your key
3. HD path (optional - press Enter to use the default)

> ⚡ The HD path is optional. If you used the default path when creating your key, 
you can skip this by pressing Enter.

#### Option 2: Using your `.asc` file

If you exported your key to a `.asc` file. The `.asc` file should be in the 
following format:

```
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

#### Option 3: Using Home Directory Backup

If you backed up your entire EOTS home directory,
you can load it manually to the machine you intend to operate
the EOTS daemon on and specify its location as the `--home` argument.

#### Verify the Key Import

After importing, you can verify that your EOTS key was successfully loaded:

```shell 
eotsd keys list <key-name> --home <path> --keyring-backend test 
```

Parameters:
* `<key-name>`: Name of the EOTS key to verify
* `--keyring-backend`: Type of keyring backend to use (default: `test`)
* `--home`: Directory containing EOTS Manager configuration and data

You should see your EOTS key listed with the correct details, confirming that
it has been imported correctly.

>⚠️ **Important**: 
> If you are a finality provider transitioning your stack from phase-1,
> make sure that you are using the same EOTS key that you
> registered in Phase-1. 

#### 3.2.2. Create an EOTS key

If you have not created an EOTS key,
use the following command to create a new one:

``` shell
eotsd keys add --key-name <key-name> --home <path> --keyring-backend test 
```

Parameters:
- `<key-name>`: Name for your EOTS key (e.g., "eots-key-1"). We do not allow 
the same `keyname` for an existing keyname.
- `--home`: Path to your EOTS daemon home directory (e.g., "~/.eotsHome")
- `--keyring-backend`: Type of keyring storage:
  - `test`: Stores keys unencrypted, no passphrase prompts
  - `os`: Uses system's secure keyring, requires passphrase at startup
  - `file`: Encrypted file storage, requires passphrase at startup


The command will return a JSON response containing your EOTS key details:

``` 
{
    "name": "eots", 
    "pub_key_hex":
    "e1e72d270b90b24f395e76b417218430a75683bd07cf98b91cf9219d1c777c19",
    "mnemonic": "parade hybrid century project toss gun undo ocean exercise
      figure decorate basket peace raw spot gap dose daring patch ski purchase
      prefer can pair"
} 
``` 

> **Security Tip 🔒**: The mnemonic phrase must be stored securely and kept private.
> It is the only way to recover your EOTS key if you lose access to it and
> if lost it can be used by third parties to get access to your key.

### 3.3. Starting the EOTS Daemon

To start the EOTS daemon use the following command:

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

>**Security Tip 🔒**:
> * `eotsd` holds your private keys which are used for signing
> * operate the daemon in a separate machine or network segment
>   with enhanced security
> * only allow access to the RPC server specified by the `RPCListener`
>   port to trusted sources. You can edit the `EOTSManagerAddress` in
>   the configuration file of the finality provider to
>   reference the address of the machine where `eotsd` is running

## 4. Setting up the Finality Provider

### 4.1. Initialize the Finality Provider Daemon

To initialize the finality provider daemon, use the following command:

```shell
fpd init --home <path> 
```

<!--- TODO: this should be removed prior to the launch -->
> ⚡ Running this command may return the message 
> `service injective.evm.v1beta1.Msg does not have cosmos.msg.v1.service proto annotation`, 
> which is expected and can be ignored.

### 4.2. Add key for the Babylon account

Each finality provider maintains a Babylon keyring containing
an account used to receive BTC Staking reward commissions and pay fees for 
transactions necessary for the finality provider's operation.

Since this key is accessed by an automated daemon process, it must be stored
unencrypted on disk and associated with the `test` keyring backend.
This ensures that the finality provider daemon can promptly submit 
transactions, such as block votes and public randomness submissions that are 
essential for its liveness and earning rewards. 

For the `fpd` keyring, the `test` backend will be exclusively used, and it is 
mandatory that you  follow this practice until automated key management becomes 
available. Additionally, we are also exploring options to support different 
withdrawal addresses.

It is also important to note that the finality provider daemon will refund 
certain fees, such as those for transactions that are required for the 
finality provider's operation. As this keyring is used for both earning and 
operational purposes, we strongly recommend maintaining only the necessary 
funds for operations in the keyring.

>⚠️ **Important**:
>To operate your Finality Provider, ensure your Babylon account is funded. 
>Block vote transactions have their gas fees refunded, but public randomness 
>submissions require gas payments. For testnet, you can obtain funds from our 
>[faucet](#add-faucet).

<!-- add faucet link -->
We recommend **not** holding a large number of funds here—just enough for 
operations. We are also exploring ways to support different withdrawal addresses.

We encourage the following for keyring maintenance:
- **Backup Keys**: Use mnemonic phrases, export key files, or back up the home 
directory.
- **Monitor Regularly**: Check for unauthorized activity, monitor status changes,
  balance and gas usage.
- **Keyring Transition**: Replace the test keyring with a secure backend when 
available.

For gas requirements, the finality provider daemon will
automatically handle gas fees, but we recommend monitoring
the gas usage to ensure the finality provider is functioning properly.

The transaction types that consume gas are:
- `MsgRegisterFinalityProvider`: Registration (requires gas)
- `MsgSubmitFinalityVote`: Block vote transactions (gas is refunded)
- `MsgCommitPubRandList`: Public randomness submissions (requires gas)

**Keyring Security 🔒**:
The finality provider daemon uses the `--keyring-backend test` which stores keys 
unencrypted on disk. While this is generally not secure, it's necessary for the 
finality provider service because:

- The daemon needs to automatically sign and send transactions frequently
- If transactions stop for too long, the provider gets jailed
- Using encrypted keystores would require manual password entry after every restart
- Service availability is critical to avoid jailing

We are actively working on implementing more secure keyring solutions that maintain 
both security and high availability.

Use the following command to add the Babylon key for your finality provider:

```shell 
fpd keys add <key-name> --keyring-backend test --home <path>
```

The above `keys add` command will create a new key pair and store it in your keyring. 
The output should look similar to the below:

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

This command will automatically update the `Key` field in 
the config file to use this key name. This key will be used for all interactions 
with the Babylon chain, including finality provider registration and transaction 
signing.
 
### 4.3. Configure Your Finality Provider

Edit the `fpd.conf` file in your finality provider home directory with the 
following parameters:

```shell 
[Application Options] 
EOTSManagerAddress = 127.0.0.1:12582 
RpcListener = 127.0.0.1:12581

[babylon] 
Key = <finality-provider-key-name-signer> # the key you used above
ChainID = bbn-test-5 
RPCAddr = http://127.0.0.1:26657 
GRPCAddr = https://127.0.0.1:9090 
KeyDirectory = <path> # The `--home`path to the directory where the keyring is stored
``` 

> ⚠️ **Important**: Operating a finality provider requires a connection to a 
> Babylon blockchain node. It is **highly recommended** to operate your own 
> Babylon full node instead of relying on third parties. You can find 
> instructions on setting up a Babylon node 
> [here](https://github.com/babylonlabs-io/networks/tree/main/bbn-1/node-setup).

Configuration parameters explained:
* `EOTSManagerAddress`: Address where your EOTS daemon is running
* `RpcListener`: Address for the finality provider RPC server
* `Key`: Your Babylon key name from Step 2
* `ChainID`: The Babylon network chain ID
* `RPCAddr`: Your Babylon node's RPC endpoint
* `GRPCAddr`: Your Babylon node's GRPC endpoint
* `KeyDirectory`: Path to your keyring directory (same as --home path)

Please verify the `chain-id` and other network parameters from the official 
Babylon Networks repository at [https://github.com/babylonlabs-io/networks/tree/main/bbn-test-5/finality-providers](https://github.com/babylonlabs-io/networks/tree/main/bbn-test-5/finality-providers)

### 4.4. Starting the Finality Provider Daemon

The finality provider daemon (FPD) needs to be running before proceeding with 
registration or voting participation.

Start the daemon with:

``` shell
fpd start --home <path> 
```
An example of the `--home` flag is `--home ./fpHome`.

The command flags:
- `start`: Runs the FPD daemon
- `--home`: Specifies the directory for daemon data and configuration

The daemon will start the RPC server for CLI communication then begin listening 
for incoming requests and finally initialize finality provider services

You should see logs indicating successful startup:

```
[INFO] Starting finality provider daemon...
[INFO] RPC server listening on...
```

>⚠️ **Important**: The daemon needs to run continuously. It's recommended to set 
>up a system service (like `systemd` on Linux or `launchd` on macOS) to manage 
>the daemon process, handle automatic restarts, and collect logs. For testing 
you can run the daemon directly in a terminal, but remember it must stay 
running to function properly.

The above will start the Finality provider RPC server at the address specified
in `fpd.conf` under the `RpcListener` field, which has a default value
of `127.0.0.1:12581`. You can change this value in the configuration file or
override this value and specify a custom address using
the `--rpc-listener` flag.

To start the daemon with a specific finality provider instance after 
registration, use the `--eots_pk_hex` flag followed by the hex string of the EOTS 
public key of the finality provider.

All the available CLI options can be viewed using the `--help` flag. These
options can also be set in the configuration file.

## 5. Finality Provider Operations

### 5.1 Create Finality Provider

The `create-finality-provider` command initializes a new finality provider
instance locally. 

``` shell
fpd create-finality-provider \ 
  --daemon-address 127.0.0.1:12581 \ 
  --chain-id bbn-test-5 \ 
  --eots-pk <eots-pk-hex> \ # this was output in fpd keys add
  --commission 0.05 \ 
  --key-name finality-provider \ 
  --moniker "MyFinalityProvider" \ 
  --website "https://myfinalityprovider.com" \
  --security-contact "security@myfinalityprovider.com" \ 
  --details "finality provider for the Babylon network" \ 
  --home ./fpHome \
  --passphrase "passphrase" 
```

Required parameters: 
- `--chain-id`: The Babylon chain ID (`bbn-test-5`) 
- `--commission`: The commission rate (between 0 and 1) that you'll receive from
  delegators 
- `--key-name`: The key name in your Babylon keyring that your finality
  provider will be associated with
- `--moniker`: A human-readable name for your finality provider

Optional parameters: 
- `--eots-pk`: The EOTS public key of the finality provider. If one is not 
provided a new one will be created but please use the same EOTS key that you
generated in `fpd keys add`
- `--website`: Your finality provider's website 
- `--security-contact`: Contact email for security issues 
- `--details`: Additional description of your finality provider 
- `--daemon-address`: RPC address of the finality provider daemon
  (default: `127.0.0.1:12581`)

Upon successful creation, the command will return a JSON response containing
your finality provider's details:

``` json
{
    "fp_addr": "bbn1ht2nxa6hlyl89m8xpdde9xsj40n0sxd2f9shsq", 
    "eots_pk_hex":
    "cf0f03b9ee2d4a0f27240e2d8b8c8ef609e24358b2eb3cfd89ae4e4f472e1a41",
      "description": 
      {
        "moniker": "MyFinalityProvider", 
        "website": "https://myfinalityprovider.com", 
        "security_contact": "security@myfinalityprovider.com", 
        "details": "finality provider for the Babylon network"
      }, 
      "commission": "0.050000000000000000", 
      "status": "CREATED"
} 
```

The response includes: 
- `fp_addr`: Your Babylon account address for receiving
  rewards 
- `eots_pk_hex`: Your unique BTC public key identifier (needed for
  registration) 
- `description`: Your finality provider's metadata 
- `commission`: Your set commission rate 
- `status`: Current status of the finality provider.


Below you can see a list of the statuses that a finality provider can transition
to:

```
	- `CREATED`: defines a finality provider that is awaiting registration
	- `REGISTERED`: defines a finality provider that has been registered
	  to the consumer chain but has no delegated stake
	- `ACTIVE`: defines a finality provider that is delegated to vote
	- `INACTIVE`: defines a finality provider whose delegations are reduced to 
    zero but not slashed
	- `JAILED`: defines a finality provider that has been jailed
```

### 5.2. Register Finality Provider

The `register-finality-provider` command registers your finality provider on the
Babylon chain. This command requires:

1. The EOTS public key 
2. The Babylon account associated with your finality provider
   (the one specified in the creation) having sufficient funds 
   to pay for the transaction fee.
3. A running fpd daemon

``` shell
fpd register-finality-provider \
  <fp-eots-pk-hex> \
  --daemon-address <rpc-address> \ 
  --home <path>
```

Parameters:
- `<fp-eots-pk-hex>`: The EOTS public key of the finality provider you want to 
register (e.g., `cf0f03b9ee2d4a0f27240e2d8b8c8ef609e24358b2eb3cfd89ae4e4f472e1a41`)
- `--daemon-address`: RPC address of the finality provider daemon
  (default: `127.0.0.1:12581`)
- `--home`: Path to your finality provider daemon home directory (e.g., `~/.fpdHome`)

If successful, the command will return a transaction hash:

``` shell
{ "tx_hash":
"C08377CF289DF0DC5FA462E6409ADCB65A3492C22A112C58EA449F4DC544A3B1" } 
```

You can verify the transaction was successful by looking up this transaction 
hash on the Babylon chain.




<!-- vitsalis: TODO: How about listing the finality providers using the CLI to
demonstrate that the finality provider has the status `REGISTERED`?
That would be a native way to verify the installation, without having to
touch Babylon. Natural way to introduce the `CREATED` status.

Maybe we can find a way to naturally introduce the `ACTIVE` or `INACTIVE` statuses.
-->

### 5.3. Withdrawing Rewards

As a participant in the Finality Provider Program, you will earn rewards that 
can be withdrawn. The functionality for withdrawing rewards is currently under 
development and will be available soon. Further updates will be provided once 
this feature is implemented.

### 5.4. Jailing and Unjailing

A finality provider can be jailed for the following reasons:
1. Missing Votes
   - Not submitting finality votes for a certain number of blocks
   - Missing votes when the FP has positive voting power

2. Missing Public Randomness
   - Not committing public randomness for blocks
   - Required before voting can occur

The specific parameters specifying the exact metrics that are taken
into account for jailing and the period of unjailing
is controlled by the Babylon chain governance.

When jailed, the following happens to a finality provider:
- Their voting power becomes `0`
- Status is set to `JAILED`
- Delegator rewards stop

To unjail a finality provider, you must complete the following steps:
1. Fix the underlying issue that caused jailing
2. Wait for the jailing period to pass (if it was due to downtime)
3. Then send the unjail transaction to the Babylon chain (not the finality provider daemon).

```shell
fpd unjail-finality-provider <eots-pk> --daemon-address <rpc-address> --home <path>
```

Parameters:
- `<eots-pk>`: Your finality provider's EOTS public key in hex format
- `--daemon-address`: RPC server address of fpd (default: `127.0.0.1:12581`)
- `--home`: Path to your finality provider daemon home directory

> ⚠️ Before unjailing, ensure you've fixed the underlying issue that caused jailing

### 5.5. Slashing

**Slashing occurs** when a finality provider **double signs**, meaning that the
finality provider signs conflicting blocks at the same height. This results in
the extraction of the finality provider's private key and their immediate
removal from the active set.

> ⚠️ **Critical**: Slashing is irreversible and results in
> permanent removal from the network.

## 6. Prometheus Metrics

<!--- vitsalis: TODO:
* We are exposing Prometheus metrics by default.
* We don't need to instruct people on how to install Prometheus.
* We just need to tell them what metrics the service exposes and
  on which port/how to configure the port.

Ask devops
-->

<!-- The daemon services of the finality provider stack
export Prometheus metrics to support their monitoring.
In this section, we highlight some key Prometheus metrics
to monitor for.

To configure the metrics:
....

The finality provider stack exports Prometheus metrics
to support the monitoring of 

We want to monitor the following metrics to
ensure the health and performance of the Finality Provider:

- Public randomness commitment success/failure
- Double-signing attempts
- Status changes (`Active`/`Inactive`/`Jailed`)
- Block vote submission success/failure
- Missing block signatures
- Transaction failures
- RPC endpoint availability
- Gas usage and balance

There are several ways to monitor these metrics,
but we suggest using Prometheus.

First, expose the metrics through the following Prometheus endpoints:

- `Port 12581`: Finality Provider Daemon metrics
- `Port 12582`: EOTS Manager metrics

<!--- TODO: config.toml?? -->
<!-- Next we will enable metric collection in `app.toml`
in both the finality provider daemon and the eots manager and
configure Prometheus to scrape the metrics.

```toml
[telemetry]
enabled = true
prometheus-retention-time = 600 # 10 minutes

[api]
enable = true
address = "127.0.0.1:12581" # change this to 127.0.0.1:12582 for the eots manager
``` --> -->