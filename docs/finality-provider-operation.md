# Finality Provider Operation

This document guides operators through the complete
lifecycle of running a finality provider, including:

* Installing and configuring the finality provider toolset (EOTS Manager and Finality Provider daemon)
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
   3. [Public Randomness Submission](#53-public-randomness-submission)
   4. [Submission of votes](#54-submission-of-votes)
   5. [Keyring maintenance and gas requirements](#55-keyring-maintenance-and-gas-requirements)
   6. [Reading the logs](#56-reading-the-logs)
   7. [Withdrawing Rewards](#57-withdrawing-rewards)
   8. [Jailing and Unjailing](#58-jailing-and-unjailing)
   9. [Monitoring](#59-monitoring)
6. [Security and Slashing](#6-security-and-slashing)

## 1. A note about Phase-1 Finality Providers

<!-- TODO: Vitalis edit

Thank you for being a participant in the first phase of the Babylon launch.
This guide involves instructions for setting up the full finality provider
toolset that you will be required to operate for your participation in the
second phase of the Babylon launch.

* use the same keys -- ur delegations associated with only the key
* migrate mainnet to mainnet and testnet to testnet
* how to proceed with the guide.

> ⚠️ **Critical**: Ensure that you use  -->

## 2. Install Finality Provider Toolset 
<!-- TODO: check add in the correct tag for the testnet --> 

Download and install [Golang 1.23](https://go.dev/dl). 
Verify the installation with the following command:

```shell 
go version 
```

### 2.1. Clone the Finality Provider Repository

Subsequently clone the finality provider 
[repository](https://github.com/babylonlabs-io/finality-provider) and navigate to the 
`bbn-test-5` tag. 
<!-- TODO: change to a specific version -->

```shell 
git clone https://github.com/babylonchain/finality-provider.git
cd finality-provider
git checkout <tag>
```

### 2.2. Build and Install Finality Provider Toolset Binaries

Run the following command to build the binaries and install them to your `$GOPATH/bin` directory:

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
<!-- fix this section -->
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

> You wont be able to run `eots version` as its not supported yet but you can 
> run `eotsd` to verify the installation.

If your shell cannot find the installed binaries, make sure `$GOPATH/bin` is in
the `$PATH` of your shell. Use the following command to add it to your profile.

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

#### 3.2.1. Import an existing EOTS key

> This section is for Finality Providers who participated in Phase-1 and already posess an EOTS key. 
>If you are a new user, you can skip this section unless you would like to familiarize yourself 
>with the backup and recovery process.

There are 3 supported methods of loading your existing EOTS keys: using a mnemonic phrase,
exporting the `.asc` file and backing up your entire home directory. We have outlined
each of these three paths for you below.

####  Using your Mnemonic Phrase

If you are using your mnemonic seed phrase, use the following command to import your key:

```shell
eotsd keys add <key-name> --home <path> --recover
```

You'll be prompted to enter:
1. Your bip39 mnemonic phrase (24 words)
2. A passphrase to encrypt your key
3. HD path (optional - press Enter to use default)

> The HD path is optional. If you used the default path when creating your key, 
you can skip this by pressing Enter.

#### Using your `.asc` file

If you exported your key to a `.asc` file. The `.asc` file should be in the following format:

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

To load your existing EOTS key, use the following command:

```shell
eotsd keys import <name> <path-to-key> --home <path> --keyring-backend test
```

#### Using Home Directory Backup

If you backed up your entire EOTS home directory or are using your `--home` directory.

1. Check to see if the EOTS daemon is running. If it is, stop it.
2. Copy your backup directory or your old home directory to the new location.

```shell
# Stop the EOTS daemon if running
# Copy your Phase-1 directory to Phase-2 location
cp -r ~/.eotsd ~/.eotsd-phase2

# Initialize it for Phase-2
eotsd init --home ~/.eotsd-phase2
```
> Please note that the above directory is just an example. 
Your directory will be located at the path you specified.

##### Verify the Key Import
After importing, you can verify that your EOTS key was successfully loaded:

```shell 
eotsd keys list <key-name> --keyring-backend test --home <path>
```

Parameters:
* `<key-name>`: Name of the EOTS key to verify
* `--keyring-backend`: Type of keyring backend to use (default: test)
* `--home`: Directory containing EOTS Manager configuration and data

You should see your EOTS key listed with the correct details, confirming that
it has been imported correctly.

>⚠️ **Important**: 
> Make sure you're using the same key name and EOTS public key that were
> registered in Phase-1.

#### 3.2.2. Create an EOTS key

If you have not created an EOTS key yet, use the following command to create a new one:

``` shell
eotsd keys add --key-name <key-name> --home <path>  --keyring-backend test 
```

Parameters:
- `<key-name>`: Name for your EOTS key (e.g., "eots-key-1"). We do not allow the same
`keyname` for an existing keyname.
- `--home`: Path to your EOTS daemon home directory (e.g., "~/.eotsHome")
- `--keyring-backend`: Type of keyring storage:
  - `test`: Stores keys unencrypted, no passphrase prompts
  - `os`: Uses system's secure keyring, requires passphrase at startup
  - `file`: Encrypted file storage, requires passphrase at startup


The command will return a JSON response containing your EOTS key details:

```json 
{
    "name": "eots", 
    "pub_key_hex":
    "e1e72d270b90b24f395e76b417218430a75683bd07cf98b91cf9219d1c777c19",
    "mnemonic": "parade hybrid century project toss gun undo ocean exercise
    figure decorate basket peace raw spot gap dose daring patch ski purchase
    prefer can pair"
} 
``` 

> **Security Tip 🔒**: The mnemonic phrase must be stored securely and kept private, as it is the 
only way to recover your EOTS key if access is lost and is critical for maintaining control 
of your finality provider operations.

### 3.3. Starting the EOTS Daemon

To start the EOTS daemon using the following command:

```shell 
eotsd start --home <path> 
```

This command starts the EOTS RPC server at the address specified in `eotsd.conf`
under the `RPCListener` field (default: `127.0.0.1:12582`). You can override this value 
by specifying a custom address with the `--rpc-listener` flag.

```shell
2024-10-30T12:42:29.393259Z     info    Metrics server is starting
{"addr": "127.0.0.1:2113"} 
2024-10-30T12:42:29.393278Z     info    RPC server listening    {"address": "127.0.0.1:12582"} 
2024-10-30T12:42:29.393363Z     info    EOTS Manager Daemon is fully active!  
EOTS Manager Daemon is fully active!  
```

>**Security Tip 🔒**:
> * `eotsd` holds your private keys which are used for signing
> * keep it in a separate machine or network segment with enhanced security
> * only allow access to the RPC server specified by the `RPCListener` port to trusted sources. You can edit
the `EOTSManagerAddress` in the configuration file of the finality provider to
reference the address of the machine where `eotsd` is running

## 4. Setting up the Finality Provider

### 4.1. Initialize the Finality Provider Daemon

To initialize the finality provider daemon, use the following command:

```shell 
fpd init --home <path> 
```

> Running this command may return the message 
`service injective.evm.v1beta1.Msg does not have cosmos.msg.v1.service proto annotation`, 
which is expected and can be ignored.

### 4.2. Add key for the Babylon account

For the Finality Provider Daemon, the keyring is kept in the local storage of the finality provider daemon. 
The key associates a Babylon account with the finality provider to receive BTC 
delegation rewards.

Use the following command to add the Babylon key for your finality provider:

```shell 
fpd keys add --keyname <key-name> --keyring-backend test --home <path>
```

>⚠️ **Important**:
>To operate your Finality Provider, ensure your Babylon account is funded. Block vote transactions 
have their gas fees refunded, but public randomness submissions require gas payments. For testnet, 
you can obtain funds from our [faucet](#add-faucet).

<!-- add faucet link -->
>We use the `--keyring-backend test`, which stores keys unencrypted on disk. This backend is suitable 
for testing but not recommended for large fund storage. Rewards from Finality Provider commissions 
are stored in this keyring. Other keyring backends are not supported yet, and prolonged inactivity 
(missing transactions) can lead to your Finality Provider being jailed.

>Keep only enough funds in the keyring for operations, which you can view [here](#keyring-maintenance-and-gas-requirements). 
We are also exploring options to support different withdrawal addresses.

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

Edit the `config.toml` file in your finality provider home directory with the following parameters:

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

> ⚠️ **Important**: Operating a finality provider requires a connection to a Babylon blockchain node. 
> It is **highly recommended** to operate your own Babylon full node instead of relying on third parties. 
> You can find instructions on setting up a Babylon node [here](https://github.com/babylonlabs-io/networks/tree/main/bbn-1/node-setup).

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
fpd start  --home <path> 
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

>⚠️ **Important**: The daemon needs to run continuously. It's recommended to set up a system 
>service (like `systemd` on Linux or `launchd` on macOS) to manage the daemon 
>process, handle automatic restarts, and collect logs. For testing purposes, 
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
--eots-pk <eots-pk-hex> \ //this is the EOTS public key of the finality provider which was generated in 
`eotsd keys add`
--commission 0.05 \ 
--key-name finality-provider \ 
--moniker "MyFinalityProvider" \ 
--website "https://myfinalityprovider.com" \
--security-contact "security@myfinalityprovider.com" \ 
--details "finality provider for the Babylon network" \ 
--home ./fp \ --passphrase "passphrase" 
```

Required parameters: 
- `--chain-id`: The Babylon chain ID (`bbn-test-5`) 
- `--commission`: The commission rate (between 0 and 1) that you'll receive from
delegators 
- `--key-name`: Name of the key in your keyring for signing
transactions - 
`--moniker`: A human-readable name for your finality provider

Optional parameters: 
- `--eots-pk`: The EOTS public key of the finality provider which was generated in `eotsd keys add`
- `--website`: Your finality provider's website 
- `--security-contact`: Contact email for security issues 
- `--details`:
Additional description of your finality provider 
- `--daemon-address`: RPC address of the finality provider daemon (default: `127.0.0.1:12581`)

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
- `commission`:
Your set commission rate 
- `status`: Current status of the finality provider

### 5.2. Register Finality Provider

The `register-finality-provider` command registers your finality provider on the
Babylon chain. This command requires:

1. The EOTS public key 
2. A funded Babylon account (needs BBN tokens for transaction fees). 
This account will be bonded to your finality provider and used to claim rewards.
3. A running FPD daemon

``` shell
fpd register-finality-provider \
 <fp-eots-pk-hex> \
--daemon-address <rpc-address> \ 
--passphrase <passphrase> \ 
--home <path> \ 
```

Parameters:
- `<fp-eots-pk-hex>`: Your EOTS public key (obtained from running `eotsd keys show <key-name>`)
(e.g., `cf0f03b9ee2d4a0f27240e2d8b8c8ef609e24358b2eb3cfd89ae4e4f472e1a41`)
- `--daemon-address`: RPC address of your finality provider daemon (default: `127.0.0.1:12581`)
- `--passphrase`: Passphrase for your key
- `--home`: Path to your finality provider daemon home directory (e.g., `~/.fpdHome`)
- `--keyring-backend`: Type of keyring storage (use `test` for testing, `file` for production)

> The BTC public key (`cf0f03...1a41`) is obtained from the previous
`eotsd keys add` command.

If successful, the command will return a transaction hash:

``` shell
{ "tx_hash":
"C08377CF289DF0DC5FA462E6409ADCB65A3492C22A112C58EA449F4DC544A3B1" } 
```

You can verify the transaction was successful by looking up this transaction 
hash on the Babylon chain.

The hash returned should look similar to below:

```shell
  type: message
- attributes:
  - index: true
    key: fp value:
  '{
    "addr":"bbn1ht2nxa6hlyl89m8xpdde9xsj40n0sxd2f9shsq",
    "description":
    {
      "moniker":"MyFinalityProvider",
      "identity":"
      ","website":"https://myfinalityprovider.com",
      "security_contact":"security@myfinalityprovider.com",
      "details":"Reliablefinality provider for the Babylon
        network"
    },
      "commission":"0.050000000000000000",
      "btc_pk":"cf0f03b9ee2d4a0f27240e2d8b8c8ef609e24358b2eb3cfd89ae4e4f472e1a41",
      "pop":{"btc_sig_type":"BIP340",
      "btc_sig":"YJgc6NU7Z011imqSfPc9w/Namr1hFj48oTlEjGqbAVvHJv+9h3p/1shTohEb1g0fDWij7Ti9yKZzjAgNVepObA=="},
      "slashed_babylon_height":"0",
      "slashed_btc_height":"0",
      "jailed":false,
      "consumer_id":"euphrates-0.5.0"
    }'
  - index: true
    key: msg_index value: "0"
  type: babylon.btcstaking.v1.EventNewFinalityProvider
gas_used: "82063" gas_wanted: "94429" height: "66693" info: "" logs: [] raw_log:""
```

### 5.3. Public Randomness Submission

For detailed information about public randomness commits, see:
* [Technical Overview](./docs/commit-pub-rand.md)
* [Core Implementation Details](./fp-core.md#committing-public-randomness)

### 5.4. Submission of votes

There is a responsibility for the finality providers to submit votes to finalize blocks on the consumer chain. The finality provider daemon automatically handles vote submission for finality providers.

To see the logs of the finality provider daemon, you can use the following command:

``` shell
tail -f <fpd-home>/logs/fpd.log | grep "successfully submitted a finality signature to the consumer chain"
```

To see the technical documentation for [sending finality votes](./send-finality-vote.md)

### 5.5. Keyring maintenance and gas requirements

The keyring stores funds for gas fees and collects rewards. We recommend not holding a large number of funds here—just enough for operations. We are also exploring ways to support different withdrawal addresses. 

We encourage the following for keyring maintenance:
- Backup Keys: Use mnemonic phrases, export key files, or back up the home directory.
- Monitor and Secure: Regularly check for unauthorized activity.
- Production Transition: Replace the test keyring with a secure backend when available.

For gas requirements, the finality provider daemon will automatically handle gas fees but we recommend monitoring the gas usage to ensure the finality provider is functioning properly.

The transaction types that consume gas are:
- `MsgCreateFinalityProvider`: Initial creation (requires gas)
- `MsgRegisterFinalityProvider`: Registration (requires gas)
- `MsgSubmitFinalityVote`: Block vote transactions (gas is refunded)
- `MsgCommitPubRandList`: Public randomness submissions (requires gas)

As a guide we recommend using the following formula to estimate gas requirements:

```
Estimated Gas = (Transaction Size) x (Gas Price)
```

We recommend holding 2 x Estimated Gas in your keyring to ensure that the finality provider has enough gas to submit votes and other transactions.

To get your finality provider address, you can use the following command:

```shell
fpd keys show <key-name> --keyring-backend test --home <path>
```

To check your current balance, you must navigate to the Babylon chain and use the following command:

```shell
babylond query bank balances <finality-provider-address>
```

### 5.6. Reading the logs

The logs are stored based on your daemon home directories:

- EOTS Daemon logs
`<eots-home>/logs/eotsd.log`

- Finality Provider Daemon logs
`<fpd-home>/logs/fpd.log`

You also can access the logs via flags when starting the daemon:

```shell
fpd start --home <path> --log_level debug
```
 Important Log Events to Monitor

**Status Changes:**
```shell
DEBUG "the finality-provider status is changed to ACTIVE"
DEBUG "the finality-provider is slashed"
DEBUG "the finality-provider status is changed to INACTIVE"
```

**Block Finalization Logs:**
```shell
INFO "successfully committed public randomness to the consumer chain"
DEBUG "failed to commit public randomness to the consumer chain"
DEBUG "checking randomness"
```

**Finality Votes:**
```shell
DEBUG "the block is already finalized, skip submission"
DEBUG "the finality-provider instance is closing"
```

You can also review the logs in real-time using standard Unix tools:

```shell
# Follow EOTS daemon logs
tail -f <eots-home>/logs/eotsd.log

# Follow Finality Provider daemon logs
tail -f <fpd-home>/logs/fpd.log

# Filter for specific events (example: status changes)
tail -f <fpd-home>/logs/fpd.log | grep "status is changed"
```

### 5.7. Withdrawing Rewards

When withdrawing rewards, you need to use the Babylon chain's CLI since rewards
are managed by the main chain.

To withdraw your finality provider rewards:

``` babylond tx incentive finality_provider ...  ```

<!-- //this code needs to be updated before further instructions can be provided
-->

### 5.8. Jailing and Unjailing

A finality provider can be jailed for various reasons, 
including not signing for a certain number of blocks, not committing public 
randomness for a certain number of blocks, or not being responsive to the 
finality provider daemon.

When jailed, the following happens to a finality provider:
- Their voting power becomes 0
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

>  Before unjailing, ensure you've fixed the underlying issue that caused jailing

So while slashing is permanent, jailing is a temporary state that can be recovered 
from through the unjailing process, as long as the finality provider wasn't slashed.

### 5.9. Monitoring

We want to monitor the following metrics to ensure the health and performance of your Finality Provider:

- Public randomness commitment success/failure
- Double-signing attempts
- Status changes (`Active`/`Inactive`/`Jailed`)
- Block vote submission success/failure
- Missing block signatures
- Transaction failures
- RPC endpoint availability
- Gas usage and balance

There are several ways to monitor these metrics but we suggest using Prometheus.

First, expose the metrics through the following Prometheus endpoints:

- `Port 12581`: Finality Provider Daemon metrics
- `Port 12582`: EOTS Manager metrics

Next we will enable metric collection in `app.toml` for your node and configure Prometheus to scrape the metrics.

``` toml
[telemetry]
enabled = true
prometheus-retention-time = 600 # 10 minutes
[api]
enable = true
address = "127.0.0.1:12581" # Secure RPC listener for Finality Provider Daemon
```
Restrict access to the EOTS Manager RPC by configuring it to listen only on `127.0.0.1:12582`.
Restart your Finality Provider node to apply these changes.

After enabling metrics in the app.toml, configure Prometheus to scrape these endpoints (prometheus.yml):

``` yaml
scrape_configs:
  - job_name: 'finality_provider'
    static_configs:
      - targets: ['localhost:12581', 'localhost:12582']
    metrics_path: '/metrics'
    scrape_interval: 10s
```

Next we want you to download and install [Prometheus](https://prometheus.io/download/).
Then create a `prometheus.yml` file and add the following scrape configuration:

``` yaml
scrape_configs:
  - job_name: 'finality_provider'
    static_configs:
      - targets: ['localhost:12581', 'localhost:12582']
    metrics_path: '/metrics'
    scrape_interval: 10s
```

Then start Prometheus using the following command:

``` shell
./prometheus --config.file=prometheus.yml
```

Once Prometheus is running:

- Open the Prometheus web interface in your browser (default: `http://localhost:9090`).
- Navigate to Status > Targets.
- Confirm that the endpoints `localhost:12581` and `localhost:12582` are listed and their status is UP.

## 6. Security and Slashing

**Slashing occurs** when a finality provider **double signs**. This occurs when a
finality provider signs conflicting blocks at the same height. This results in
the extraction of the provider's private key and automatically triggers shutdown
of the finality provider, removal from the active set, jailing and compromised rewards.

> **Critical**: Slashing is irreversible and results in permanent removal from the network.

**Keyring Security 🔒**:
The finality provider daemon uses the `--keyring-backend test` which stores keys unencrypted on disk.
While this is generally not secure, it's necessary for the finality provider service because:

* The daemon needs to automatically sign and send transactions frequently
* If transactions stop for too long, the provider gets jailed
* Using encrypted keystores would require manual password entry after every restart
* Service availability is critical to avoid jailing

We are actively working on implementing more secure keyring solutions that maintain both security
and high availability.

**Security Best Practices 🔒**:
Here are some best practices to secure your finality provider:
* Run EOTS Manager and Finality Provider on separate machine/network segment
* Regular system security audits
* Monitor for unauthorized activities
* Allow only necessary ports (`12581`, `12582`)
* Active monitoring of the logs


