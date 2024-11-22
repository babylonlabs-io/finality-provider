# Finality Provider Operation

This document guides operators through the complete
lifecycle of running a finality provider, including:

* Installing and configuring the finality provider 
  toolset (EOTS Manager and FP daemon)
* Managing keys (EOTS key for signatures and Babylon key for rewards)
* Registering your finality provider on the Babylon network
* Operating and maintaining your finality provider
* Collecting rewards

This is an operational guide intended for technical finality provider administrators.
For conceptual understanding, see our [Technical Documentation](./docs/fp-core.md). 
Please review the [high-level explainer](./README.md) before proceeding to 
gain an overall understanding of the finality provider. 

## Table of Contents

1. [A note about Phase-1 Finality Providers](#phase-1-finality-providers)
2. [Install Finality Provider Toolset](#install-finality-provider-binary)
3. [Setting up the EOTS Manager](#setting-up-the-eots-manager)
   1. [Loading Existing Keys](#loading-existing-keys)
   2. [Starting the EOTS Daemon](#starting-the-eots-daemon)
4. [Setting up the Finality Provider](#setting-up-the-finality-provider)
   1. [Starting the Finality Provider Daemon](#starting-the-finality-provider-daemon)
5. [Finality Provider Operation]()
   1. [Create Finality Provider](#create-finality-provider)
   2. [Register Finality Provider](#register-finality-provider)
   3. [Public Randomness Submission](#public-randomness-submission)
   4. [Submission of votes]()
   5. [Keyring maintenance and gas requirements]()
   6. [Reading the logs](#reading-the-logs)
   7. [Withdrawing Rewards](#withdrawing-rewards)
   8. [Jailing and Unjailing](#jailing-and-unjailing)
   9. [Monitoring]() <!-- ask filippos -->
6. [Security and Slashing]()

<!--- This document is an operational document.
It is not targeted to people that need to understand on a deep
level what the finality provider is or does, it is targeted
to devops people that are responsible for operating a finality provider.
These people need to know the following:
* How do I install the finality provider toolset and set it up.
* What is included in the finality provider toolset (fpd, eotsd),
  and what do these tools correspond to.
* How do I set up the toolset and daemons and how do I connect them
  with each other. Security tips?
* What keys do I need for the operation of this stack,
  and what is the security level I should apply to them.
    * should I hold funds into them?
* What is the lifecycle of my finality provider:
   * registration, creation, submitting votes and public randomness
   * being active/inactive
   * being jailed and unjailing
   * being slahsed and how it can happen and how I can be protected against it
* How do I understand my rewards and retrieve them?
-->

## 1. A note about Phase-1 Finality Providers

<!-- TODO: Vitalis edit -->

Thank you for being a participant in the first phase of the Babylon launch.
This guide involves instructions for setting up the full finality provider
toolset that you will be required to operate for your participation in the
second phase of the Babylon launch.

* use the same keys -- ur delegations associated with only the key
* migrate mainnet to mainnet and testnet to testnet
* how to proceed with the guide.

> ⚠️ **Critical**: Ensure that you use 

Finality providers that participated in the first phase of the Babylon mainnet,
should use the same EOTS keys for the second phase instead of creating new ones,
as all their BTC stake delegations are associated with the keys they used on phase-1.

* `Phase-1` operators should use their existing EOTS keys for `Phase-2` participation.
* All your delegations are associated with your existing EOTS key - you must import 
your existing key instead of creating a new one, as 
**creating new keys will result in loss of delegations**.
* For those who participated in both testnet and mainnet, ensure you 
**use the correct key for each network to avoid delegation loss and potential slashing**.
* Locate your `Phase-1` EOTS key backup and proceed to the [Loading Existing Keys](#loading-existing-keys) 
section for import instructions.

<!-- * If you were an fp on phase-1, you don't need to create new keys.
* All your delegations are associated with your existing EOTS key,
  so you need to import that instead of creating a new one. (highlight this as its dangerous)
* If you participated in both the testnet and the mainnet,
  make sure that you transition the finality provider key you used on the respective network. (highlight this as its dangerous) -->


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

Run to build the binaries and install them to your `$GOPATH/bin` directory:
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
Run `fpd version` to verify the installation:

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
the `$PATH` of your shell. Use the following command to add it to your profile.

```shell 
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
```

## 3. Setting up the EOTS Manager Daemon

The EOTS manager daemon is a core component of the finality provider
stack responsible for managing your EOTS keys and producing EOTS signatures
to be used for votes. In this section, we are going to go through
its setup and key generation process.

> Note: If you were a participant of a phase-1 Babylon launch network
> intending to transition your finality provider to a later phase,
> you do not need to create a new EOTS key. Please follow this section
> as it will guide you in transitioning your existing keys to the
> phase-2 and beyond finality provider stack.

### 3.1. Initialize the EOTS Manager

If you haven't already,
initialize a home directory for the EOTS Manager with the following command:

```shell
eotsd init --home <path>
```

Parameters:
* `--home`: Directory for EOTS Manager configuration and data
  - Default: `/Users/<username>/Library/Application Support/Eotsd`
  - Example: `--home ./eotsHome`

### 3.2. Create/Add an EOTS Key

#### 3.2.1. Import an existing EOTS key

<!-- TODO: Different cases people might have backed up their existing key:
1. Mnemonic -> need to instruct them how to import via mnemonic.
2. Just export (`.asc` file) -> need to instruct them how to import like this.
3. Backed up entire home directory -> need to instruct them on what to move -->

>If you have already set up an EOTS Manager, you can skip this section.
The following steps are only for users who have not yet set up an EOTS Manager.
Phase-1 users who already had an EOTS Manager set up can skip to the
[Loading Existing Keys](#loading-existing-keys)
section at the end of this guide for specific instructions
on re-using their Phase-1 EOTS keys unless you would like to familiarize yourself
with the backup and recovery process.

If you participated in Phase-1, follow these steps to load your existing EOTS
key and configure it for Phase-2.

This section is only for Finality Providers who participated in Phase-1 and already
had an EOTS key. If you are a new user, you can skip this section unless you would like to
familiarlize yourself with the backup and recovery process.

##### Verify Your EOTS Key Backup
>⚠️ **Important**: Before proceeding, ensure you have access to your original EOTS key from Phase-1.
This is the same key that was registered in the [Phase-1 registration](https://github.com/babylonlabs-io/networks/tree/main/bbn-test-5/finality-providers).
<!-- TODO: update link when merged -->

##### Import Your EOTS Key into the Keyring

Before importing the key, it should be in a file (the file will be named `key.asc`)
in the following format.

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

To load your existing EOTS key, use the following command to import it into the
keyring:

```shell 
eotsd keys import <name> <path-to-key> --home <path> --keyring-backend test
```

- `<name>`: New name for your key in Phase-2. This should be unique from the keyname
  used in Phase-1.
- `<path-to-key>`: Path to the exported key file
- `--home`: EOTS daemon home directory for Phase-2
- `--keyring-backend`: Keyring backend type (use `test` for testing)

##### Verify the Key Import
After importing, verify that your EOTS key was successfully loaded:

```shell 
eotsd keys list <key-name> --keyring-backend test --home <path>
```

* `<key-name>`: Name of the EOTS key to verify
* `--keyring-backend`: Type of keyring backend to use (default: test)
* `--home`: Directory containing EOTS Manager configuration and data

You should see your EOTS key listed with the correct details, confirming that
it has been imported correctly.

>⚠️ **Important**: Make sure you're using the same key name and EOTS public key that were
registered in Phase-1.

#### 3.2.2. Create an EOTS key

``` shell
eotsd keys add --key-name <key-name> --home <path>  --keyring-backend test 
```

- `<key-name>`: Name for your EOTS key (e.g., "eots-key-1"). We do not allow the same
`keyname` for an existing keyname.
- `--home`: Path to your EOTS daemon home directory (e.g., "~/.eotsd")
- `--keyring-backend`: Type of keyring storage (use "test" for testing)

Sample output:

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

>⚠️ **Important**: The mnemonic phrase must be stored securely and kept private, as it is the 
only way to recover your EOTS key if access is lost and is critical for maintaining control 
of your finality provider operations.

### 3.3. Starting the EOTS Daemon

You can start the EOTS daemon using the following command:
```shell 
eotsd start --home <path> 
```

This command starts the EOTS RPC server at the address specified in eotsd.conf 
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
> * only allow access to the RPC server specified by the `RPCListener` port to trusted sources

<!--- It is recommended to run the `eotsd` daemon on a separate machine or
network segment to enhance security with restricted access to the RPC server specified `RPCListener`.
This helps isolate the key management functionality and reduces the potential attack surface. You can edit
the `EOTSManagerAddress` in the configuration file of the finality provider to
reference the address of the machine where `eotsd` is running. -->

## 4. Setting up the Finality Provider

### 4.1. Initialize the Finality Provider Daemon

Use the `fpd init` command to initialize a home directory for the Finality Provider. 
You can set or change the home directory using the `--home` tag. For example, use 
`--home ./fpHome` to specify a custom directory. The application default home directory is 
`/Users/<username>/Library/Application Support/Fpd`.

```shell 
fpd init --home <path> 
```

>Note: Running this command may return the message 
`service injective.evm.v1beta1.Msg does not have cosmos.msg.v1.service proto annotation`, 
which is expected and can be ignored.

### 4.2. Add key for the Babylon account

The keyring is kept in the local storage of the finality provider daemon. 
The key associates a Babylon account with the finality provider to receive BTC 
delegation rewards.

Use the following command to add the Babylon key for your finality provider:

```shell 
fpd keys add --keyname <key-name> --keyring-backend test --home <path>
```

We use `--keyring-backend test`, which specifies which backend to use for the
keyring, `test` stores keys unencrypted on disk. For production environments,
use `file` or `os` backend.

<!-- TODO: we should say that this keyring is where the rewards of
the finality provider commissions go to -->

<!-- TODO: Explanation on why we require a test key ring.
Maybe a tip that they should not hold a lot of funds there,
just enough for operations, with a link to the later section
which specifies the operational costs for a finality provider.
Also, we can tell them that we are examining ways to support
different withdrawal addresses. -->


This command will create a new key pair and store it in your keyring. 
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

Please verify the `chain-id` and other network parameters from the official 
Babylon Networks repository at [https://github.com/babylonlabs-io/networks/tree/main/bbn-test-5/finality-providers](https://github.com/babylonlabs-io/networks/tree/main/bbn-test-5/finality-providers)

> ⚠️ **Important**: Funding Your Babylon Account
> * You need to have funds in your Babylon account to submit transactions
> * Block vote transactions get their gas refunded
> * Public randomness submissions require you to pay gas fees
> * For testnet operations, you can get funds from our [faucet]()
> * For mainnet operations, see the required funds documentation [here]()
 

### 4.3. Configure Your Finality Provider

Edit the `config.toml` file in your finality provider home directory with the following parameters:

```shell 
[Application Options] 
EOTSManagerAddress = 127.0.0.1:12582 
RpcListener = 127.0.0.1:12581

[babylon] 
Key = <finality-provider-key-name-signer> // the key you used above
ChainID = bbn-test-5 
RPCAddr = http://127.0.0.1:26657 
GRPCAddr = https://127.0.0.1:9090 
KeyDirectory = <path> # The `--home`path to the directory where the keyring is stored
``` 

> ⚠️ **Important**: Operating a finality provider requires a connection to a Babylon blockchain node. 
It is **highly recommended** to operate your own Babylon full node instead of relying on third parties. 
You can find instructions on setting up a Babylon node [here](https://github.com/babylonlabs-io/networks/tree/main/bbn-1/node-setup).

Configuration parameters explained:
* `EOTSManagerAddress`: Address where your EOTS daemon is running
* `RpcListener`: Address for the finality provider RPC server
* `Key`: Your Babylon key name from Step 2
* `ChainID`: The Babylon network chain ID
* `RPCAddr`: Your Babylon node's RPC endpoint
* `GRPCAddr`: Your Babylon node's GRPC endpoint
* `KeyDirectory`: Path to your keyring directory (same as --home path)

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

>Note: The daemon needs to run continuously. It's recommended to set up a system 
service (like `systemd` on Linux or `launchd` on macOS) to manage the daemon 
process, handle automatic restarts, and collect logs. For testing purposes, 
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
- `--daemon-address`: RPC
address of the finality provider daemon (default: 127.0.0.1:12581)

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
 <btc-public-key> \
--daemon-address <rpc-address> \ 
--passphrase <passphrase> \ 
--home <path> \ 
```

- `<btc-public-key>`: Your BTC public key from create-finality-provider 
(e.g., `cf0f03b9ee2d4a0f27240e2d8b8c8ef609e24358b2eb3cfd89ae4e4f472e1a41`)
- `--daemon-address`: RPC address of your finality provider daemon (default: "127.0.0.1:12581")
- `--passphrase`: Passphrase for your key
- `--home`: Path to your finality provider daemon home directory (e.g., "~/.fpd")
- `--keyring-backend`: Type of keyring storage (use "test" for testing, "file" for production)

Example:
```shell
fpd register-finality-provider \
    cf0f03b9ee2d4a0f27240e2d8b8c8ef609e24358b2eb3cfd89ae4e4f472e1a41 \
    --daemon-address 127.0.0.1:12581 \
    --passphrase "my-secure-passphrase" \
    --home ~/.fpd \
    --keyring-backend test
```

> Note: The BTC public key (`cf0f03...1a41`) is obtained from the previous
`eotsd keys add` command.

If successful, the command will return a transaction hash:

``` shell
{ "tx_hash":
"C08377CF289DF0DC5FA462E6409ADCB65A3492C22A112C58EA449F4DC544A3B1" } 
```

You can verify the transaction was successful by looking up this transaction 
hash on the Babylon chain.

The hash returned should look something similar to below:

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

### 5.3. Jailing and Unjailing

As mentioned above, a finality provider can be jailed for various reasons, 
including not signing for a certain number of blocks, not committing public 
randomness for a certain number of blocks, or not being responsive to the 
finality provider daemon.

When jailed, the following happens to a finality provider:
- Their voting power becomes 0
- Status is set to `JAILED`
- Delegator rewards stop

To unjail a finality provider, you must complete the following steps:
- Fix the underlying issue that caused jailing
- Wait for the jailing period to pass (if it was due to downtime)
- Then send the unjail transaction to the Babylon chain (not the finality provider daemon).

You can use the following command to send the unjail transaction:
```shell
babylond tx slashing unjail \
--from=<your-key-name> \
--chain-id="euphrates-0.5.0" \
--gas="auto" \
--gas-adjustment=1.5 \
--gas-prices=0.025ubbn \
--keyring-backend=test \
--home=./nodeDir
```

Parameters:
* `--from`: Your Babylon key name
* `--chain-id`: Current Babylon network chain ID
* `--gas`: Gas limit for transaction (auto recommended)
* `--gas-adjustment`: Multiplier for auto gas calculation
* `--gas-prices`: Gas price in ubbn
* `--keyring-backend`: Keyring backend type
* `--home`: Babylon node home directory

> ⚠️ **Important**: Before unjailing, ensure you've fixed the underlying issue that caused jailing

So while slashing is permanent, jailing is a temporary state that can be recovered 
from through the unjailing process, as long as the finality provider wasn't slashed.

### 5.3. Public Randomness Submission

For detailed information about public randomness commits, see:
* [Technical Overview](./docs/commit-pub-rand.md)
* [Core Implementation Details](./fp-core.md#committing-public-randomness)

### 5.4. Submission of votes

<!-- TODO -->

### 5.5. Keyring maintenance and gas requirements
<!-- TODO -->

### 5.6. Reading the logs

The logs are stored based on your daemon home directories:

- EOTS Daemon logs
`<eots-home>/logs/eotsd.log`

- Finality Provider Daemon logs
`<fpd-home>/logs/fpd.log`

You also can access the logs via flags when starting the daemon:

```
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

<!--- TODO -->

### 5.9. Monitoring

<!--- TODO -->

## 6. Security and Slashing

Slashing occurs when a finality provider **double signs**. This occurs when a
finality provider signs conflicting blocks at the same height. This results in
the extraction of the provider's private key and automatically triggers shutdown
of the finality provider.

<!--- TODO: incorporate on the rest of the document.
Of course if something doesn't fit anywhere, we can have a separate section,
but only when the user has context --->

> ⚠️ **Critical**: Implement these measures before starting daemons or handling keys

* Finality Provider Daemon: Secure RPC listener (`127.0.0.1:12581`)
* Keys:
    - EOTS: Secure offline backup required
    - Babylon: Use `os` or `file` backend for production
* Monitor:
    - Double-signing attempts
    - Status changes (`Active`/`Inactive`/`Jailed`)
    - Public randomness commits

> ⚠️ **Important Note on Keyring Security**:
The finality provider daemon uses the `--keyring-backend test` which stores keys unencrypted on disk.
While this is generally not secure, it's necessary for the finality provider service because:

* The daemon needs to automatically sign and send transactions frequently
* If transactions stop for too long, the provider gets jailed
* Using encrypted keystores would require manual password entry after every restart
* Service availability is critical to avoid jailing

For other Babylon services that don't require automatic transaction signing, we recommend:
* Use `os` backend (most secure, uses system keyring)
* Or use `file` backend (encrypted storage)
* Never use `test` backend outside of automated services

We are actively working on implementing more secure keyring solutions that maintain both security
and high availability.

> ⚠️ **Warning**: Security breaches can result in slashing and permanent loss of provider status
