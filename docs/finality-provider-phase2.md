# Finality Provider Phase 2 Migration Guide

This guide covers the Phase 2 launch of the Babylon network, 
a critical transition that introduces active participation 
in finality voting, rewards distribution, and the Proof of 
Stake (PoS) system. This migration represents a shift from 
the limited role of finality providers in Phase 1 to a fully 
active role in network security and governance in Phase 2.
This guide provides a step-by-step process for operators to 
onboard, whether setting up anew or using a pre-existing setup, 
with a focus on Phase 2-specific requirements.

The following explains the migration from Phase 1 to 2.

## Phase 1 (Previous phase)

- Only involves Bitcoin holders submitting staking transactions
- No active Proof of Stake (PoS) chain operation
- Finality providers only need EOTS keys registered
- No need to run finality service

## Phase 2 (New phase)

- Active participation in finality voting
- Running the complete finality provider toolset:
  - Babylon full node
  - EOTS manager daemon
  - Finality provider daemon
- Earning commissions from delegations
- Proof of Stake (PoS) and network security
- Rewards distribution
- Participating in governance

There are 2 different types of paths for Finality providers

1. **New Setup** 
 - For operators starting fresh
 - Need to generate both EOTS and FP keys
 - Complete full configuration process

If you are a new operator, start with the 
[Install Finality Provider Binary](#install-finality-provider-binary) section.

 2. **Has existing EOTS Key**
   - Already have EOTS key from Phase 1
   - Reference existing EOTS key in setup

If you have an existing EOTS key from Phase 1 please skip to 
[Loading Existing Keys](#loading-existing-keys-only-for-phase-1-finality-providers) 
steps

For an introduction on the finality providers see (here)
<!-- #[this-should-link-to-a-highlevel-document-of-what-a-finality-provider-is] -->

## Install Finality Provider Binary 
<!-- TODO: check add in the correct tag forthe testnet --> 
Download [Golang 1.21](https://go.dev/dl) 

Download and install Golang 1.21. Verify the installation with the following command:

```shell 
go version 
```

### Step 1: Clone the Finality Provider Repository

Subsequently Clone the finality provider 
[repository](https://github.com/babylonlabs-io/finality-provider) and navigate to the 
`bbn-testnet-5` tag.

```shell 
git clone https://github.com/babylonchain/finality-provider.git
cd finality-provider
git checkout <tag>
```

### Step 2: Build and Install Finality Provider Binaries

Run:
```shell 
make install 
```

This command will:
- Build and compile all Go packages
- Install binaries to `$GOPATH/bin`:
  - `eotsd`: EOTS manager daemon
  - `fpd`: Finality provider daemon
  - `fpcli`: Finality provider CLI tool
- Make commands globally accessible from your terminal

### Step 3: Verify Installation

Run `eotsd` to check the available actions:

```shell 
eotsd 
```

Sample output:

```shell 
NAME:
   eotsd - Extractable One Time Signature Daemon (eotsd).

USAGE:
   eotsd [global options] command [command options] [arguments...]

COMMANDS:
   start            Start the Extractable One Time Signature Daemon.  
   init             Initialize the eotsd home directory.  
   sign-schnorr     Signs a Schnorr signature over arbitrary data with
...
```

If your shell cannot find the installed binaries, make sure `$GOPATH/bin` is in
the `$PATH` of your shell. Usually these commands will do the job

```shell 
export PATH=$HOME/go/bin:$PATHecho 'export PATH=$HOME/go/bin:$PATH' >>
~/.profile 
```

## Install Babylon Binary

Clone the Babylon [repository](https://github.com/babylonlabs-io/babylon) and 
checkout the `bbn-testnet-5` tag:

```shell 
git clone git@github.com:babylonlabs-io/babylon.git
cd babylon
git checkout <tag>
```

### Step 1: Build and Install the Babylon Binary

Run:
```shell 
make install 
```

This command will:
- Build and compile all Go packages
- Install binary to `$GOPATH/bin`:
  - `babylond`: Babylon network daemon
- Make command globally accessible from your terminal

### Step 2: Verify Installation

Run `babylond` to see the available commands:

```shell 
babylond
```

Sample output:  

```shell 
Available Commands:
  add-genesis-account Add a genesis account to genesis.json 
  collect-gentxs Collect genesis txs and output a genesis.json file 
  comet CometBFT subcommands
  config Utilities for managing application configuration ...
```

If your shell cannot find the installed binaries, make sure `$GOPATH/bin` 
is in the `$PATH` of your shell. The following command should help this issue. 

```shell
export PATH=$HOME/go/bin:$PATH
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
```

## Connecting to a Babylon Node

To operate as a finality provider, the Finality Provider program must be connected
to a Babylon node. It is highly recommended for each finality provider to set up 
their own node to ensure network reliability and direct access to network updates.

For detailed instructions on setting up a Babylon node, please refer to the 
[Setting up a Node](https://github.com/babylonlabs-io/networks/syncing-a-node) 
section of the Babylon Networks repository. 

## Setting up the EOTS Manager

>Note: If you have already set up an EOTS Manager, you can skip this section. 
The following steps are only for users who have not yet set up an EOTS Manager. 
Phase 1 users who already have an EOTS Manager set up can skip to the 
[Loading Existing Keys](#loading-existing-keys-only-for-phase-1-finality-providers) 
section at the end of this guide for specific instructions 
on re-using their Phase 1 EOTS keys.

After a node and a keyring have been set up, the operator can set up and run the
Extractable One Time Signature (EOTS) manager daemon.

The EOTS daemon is responsible for managing EOTS keys, producing EOTS
randomness, and using them to produce EOTS signatures. To read more on the EOTS
Manager see [here](#)

### Step 1: Initialize the EOTS Manager

Use the `eotsd init` command to initialize a home directory for the EOTS Manager.
You can set or change your home directory using the `--home` tag. For example, use 
`--home ./eotsKey` to specify a custom directory.

```shell 
eotsd init --home <path> 
```

### Step 2: Create an EOTS Key

Once the EOTS Manager is initialized, you need to create an EOTS key:

``` shell
eotsd keys add --key-name <key-name> --home <path> 
```

You will be prompted to enter and confirm a passphrase. 
Ensure this is completed before starting the daemon.

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

## Loading Existing Keys (Only for Phase 1 Finality Providers)

Note: This section is only for users who participated in Phase 1 and already 
have an EOTS key. If you are a new user, you can skip this section.

If you participated in Phase 1, follow these steps to load your existing EOTS 
key and configure it for Phase 2.

### Step 1: Verify Your EOTS Key Backup
Before proceeding, ensure you have a backup of your Phase 1 EOTS key. 
You will need this backup file to import the key into the Phase 2 environment.

### Step 2: Import Your EOTS Key into the Keyring
To load your existing EOTS key, use the following command to import it into the 
keyring:

```shell 
eotsd keys import <key-name> <path-to-backup>
```

- `<key-name>`: The name you want to assign to this key in Phase 2.
- `<path-to-backup>`: The path to your EOTS key backup file from Phase 1.

## Starting the EOTS Daemon

You will need to navigate to the You can start the EOTS daemon using the following command:

```shell 
eotsd start --home <path> 
```

This command starts the EOTS RPC server at the address specified in eotsd.conf 
under the RpcListener field (default: 127.0.0.1:12582). You can override this value 
by specifying a custom address with the --rpc-listener flag.

```shell
2024-10-30T12:42:29.393259Z     info    Metrics server is starting
{"addr": "127.0.0.1:2113"} 
2024-10-30T12:42:29.393278Z     info    RPC server listening    {"address": "127.0.0.1:12582"} 
2024-10-30T12:42:29.393363Z     info    EOTS Manager Daemon is fully active!  
EOTS Manager Daemon is fully active!  
```

>**Note**: It is recommended to run the `eotsd` daemon on a separate machine or
network segment to enhance security. This helps isolate the key management
functionality and reduces the potential attack surface. You can edit
the`EOTSManagerAddress` in the configuration file of the finality provider to
reference the address of the machine where `eotsd` is running.

## Setting up the Finality Provider

The Finality Provider Daemon (FPD) is responsible for monitoring new Babylon blocks, 
committing public randomness for the blocks it intends to provide finality signatures for, 
and submitting finality signatures. For more information on Finality Providers, please see 
[here](#).

### Step 1: Initialize the Finality Provider Daemon

Use the fpd init command to initialize a home directory for the Finality Provider. 
You can set or change the home directory using the `--home` tag. For example, use 
`--home ./fpKeys` to specify a custom directory.

```shell 
fpd init --home <path> 
```

>Note: Running this command may return the message 
`service injective.evm.v1beta1.Msg does not have cosmos.msg.v1.service proto annotation`, 
which is expected and can be ignored.

### Step 2: Add a Key for the Finality Provider on the Babylon Chain

The Finality Provider Daemon uses a keyring to store keys locally, enabling it to sign 
transactions on the Babylon chain.

Add a key for your finality provider:

```shell 
Copy code
fpd keys add --keyname <key-name> --keyring-backend test --home <path>
--keyring-backend 
```

Options:
- `test`: Stores keys unencrypted on disk. This is suitable for testing but not recommended 
for production.
- `file`: Stores encrypted keys on disk, offering more security than the test option.
- `os`: Uses the operating system's native keyring for the highest level of security, 
relying on OS-managed encryption and access controls.

The command will create a new key pair and store it in your keyring. Sample output:

```shell 
- address: bbn19gulf0a4yz87twpjl8cxnerc2wr2xqm9fsygn9
  name: finality-provider
  pubkey: '{"@type":"/cosmos.crypto.secp256k1.PubKey", 
  "key":"AhZAL00gKplLQKpLMiXPBqaKCoiessoewOaEATKd4Rcy"}'
  type: local
```

>Note: Verify the chain-id by checking the Babylon RPC node status at 
[https://rpc.testnet5.babylonlabs.io/status](https://rpc.testnet5.babylonlabs.io/status).

### Step 3: Configure fpd.config

After initializing the Finality Provider Daemon, it will generate an fpd.config file. 
Open config.toml to set the necessary parameters as shown below:

```shell 
[Application Options]
EOTSManagerAddress = 127.0.0.1:12582
RpcListener = 127.0.0.1:12581

[babylon]
Key = <finality-provider-key-name-signer>  # the key you used above
ChainID = bbn-test-5
RPCAddr = http://127.0.0.1:26657
GRPCAddr = https://127.0.0.1:9090
KeyDirectory = ./fpKey
```

The Key field stores the name of the key used for signing transactions on the 
Babylon chain and should match the key name specified during key creation. 
KeyDirectory points to the location where the keyring is stored.

The Finality Provider Daemon is responsible for monitoring for new Babylon
blocks, committing public randomness for the blocks it intends to provide
finality signatures for, and submitting finality signatures. To read more on
Finality Providers please see [here](#) 
<!-- add link to finality providers high level docs-->

The `fpd init` command initializes a home directory for the EOTS manager. You
can wish to set/change your home directory with the `--home` tag.  For the home
`<path>` we have used `./fpKey`

``` shell
fpd init  --home <path> 
```

Note: will return 
`service injective.evm.v1beta1.Msg does not have cosmos.msg.v1.service proto annotation`
which is expected and can be ignored.

### Step 4: Add key for the finality provider on the Babylon chain

The keyring is maintained by the finality provider daemon, this is local storage
of the keys that the daemon uses. The account associated with this key exists on
the babylon chain.

Use the following command to add a key for your finality provider:

```shell 
fpd keys add --keyname <key-name> --keyring-backend test --home <path>
```

We use `--keyring-backend test`, which specifies which backend to use for the
keyring, `test` stores keys unencrypted on disk. For production environments,
use `file` or `os` backend.


 There are three options for the keyring backend:

 `test`: Stores keys unencrypted on disk. It’s meant for testing purposes and
 should never be used in production.  `file`: Stores encrypted keys on disk,
 which is a more secure option than test but less secure than using the OS
 keyring.  `os`: Uses the operating system's native keyring, providing the
 highest level of security by relying on OS-managed encryption and access
 controls.

This command will create a new key pair and store it in your keyring. The output
should look similar to the below.

``` shell
- address: bbn19gulf0a4yz87twpjl8cxnerc2wr2xqm9fsygn9
  name: finality-provider pubkey:
  '{"@type":"/cosmos.crypto.secp256k1.PubKey",
  "key":"AhZAL00gKplLQKpLMiXPBqaKCoiessoewOaEATKd4Rcy"}'
  type: local
```

>Note: Please verify the `chain-id` from the Babylon RPC
node [https://rpc.testnet5.babylonlabs.io/status]
(https://rpc.testnet5.babylonlabs.io/status)

 >The configuration below requires to point to the path where this keyring is
 stored `KeyDirectory`. This `Key` field stores the key name used for
 interacting with the babylon chain and will be specified along with
 the `KeyringBackend`field in the next step. So we can ignore the setting of the
 two fields in this step.

Once the node is initialized with the above command. It should generate a
`fpd.config` Edit the `config.toml` to set the necessary parameters with the
below

```shell 
[Application Options] EOTSManagerAddress = 127.0.0.1:12582 RpcListener
= 127.0.0.1:12581

[babylon] Key = <finality-provider-key-name-signer> // the key you used above
ChainID = bbn-test-5 RPCAddr = http://127.0.0.1:26657 GRPCAddr =
https://127.0.0.1:9090 KeyDirectory = ./fpKey 
``` 

### Step 3: Verify the Key Import
After importing, verify that your EOTS key was successfully loaded:

```shell 
eotsd keys list
```

You should see your EOTS key listed with the correct details, confirming that 
it has been imported correctly.

>Note: Make sure you're using the same key name and EOTS public key that were 
registered in Phase 1.


## Starting the Finality provider Daemon

The finality provider daemon (FPD) needs to be running before proceeding with 
registration or voting participation.

Start the daemon with:

``` shell
fpd start  --home ./fp 
```

The command flags:
- `start`: Initiates the FPD daemon
- `--home`: Specifies the directory for daemon data and configuration

The daemon will start the RPC server for CLI communication then begin listening 
for incoming requests and finally initialize finality provider services

You should see logs indicating successful startup:

```
[INFO] Starting finality provider daemon...
[INFO] RPC server listening on...
```

>Note: Keep this terminal window open as the daemon needs to run continuously.

The above will start the Finality provider RPC server at the address specified
in `fpd.conf` under the `RpcListener` field, which has a default value
of `127.0.0.1:12581`. You can change this value in the configuration file or
override this value and specify a custom address using
the `--rpc-listener` flag.

To start the daemon with a specific finality provider instance, use
the `--btc-pk` flag followed by the hex string of the BTC public key of the
finality provider (`btc_pk_hex`) in the next step

All the available CLI options can be viewed using the `--help` flag. These
options can also be set in the configuration file.

## Create Finality Provider

The `create-finality-provider` command initializes a new finality provider
instance locally. This command:

- Generates a BTC public key that uniquely identifies your finality provider 
- Creates a Babylon account to receive staking rewards

``` shell
fpd create-finality-provider \ --daemon-address 127.0.0.1:12581 \ --chain-id
bbn-test-5 \ --commission 0.05 \ --key-name finality-provider \ --moniker
"MyFinalityProvider" \ --website "https://myfinalityprovider.com" \
--security-contact "security@myfinalityprovider.com" \ --details "finality
provider for the Babylon network" \ --home ./fp \ --passphrase "passphrase" 
```

Required parameters: - `--chain-id`: The Babylon chain ID (`bbn-test-5`) -
`--commission`: The commission rate (between 0 and 1) that you'll receive from
delegators - `--key-name`: Name of the key in your keyring for signing
transactions - `--moniker`: A human-readable name for your finality provider

Optional parameters: - `--website`: Your finality provider's website -
`--security-contact`: Contact email for security issues - `--details`:
Additional description of your finality provider - `--daemon-address`: RPC
address of the finality provider daemon (default: 127.0.0.1:12581)

Upon successful creation, the command will return a JSON response containing
your finality provider's details:

``` json
{
    "fp_addr": "bbn1ht2nxa6hlyl89m8xpdde9xsj40n0sxd2f9shsq", 
    "btc_pk_hex":
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
- `btc_pk_hex`: Your unique BTC public key identifier (needed for
registration) 
- `description`: Your finality provider's metadata 
- `commission`:
Your set commission rate 
- `status`: Current status of the finality provider

## Register Finality Provider

The `register-finality-provider` command registers your finality provider on the
Babylon chain. This command requires:

1. The BTC public key (obtained from the `create-finality-provider` command) 
2. A funded Babylon account (needs BBN tokens for transaction fees) 
3. A running FPD daemon

``` shell
fpd register-finality-provider \
cf0f03b9ee2d4a0f27240e2d8b8c8ef609e24358b2eb3cfd89ae4e4f472e1a41 \
--daemon-address 127.0.0.1:12581 \ --passphrase "Zubu99012" \ --home ./fp \ 
```

> Note: The BTC public key (`cf0f03...1a41`) is obtained from the previous
`create-finality-provider` command.

If successful, the command will return a transaction hash:

``` shell
{ "tx_hash":
"C08377CF289DF0DC5FA462E6409ADCB65A3492C22A112C58EA449F4DC544A3B1" } 
```

 You can query this hash to confirm the transaction was successful by navigating
 to the babylon chain and making a query, such as below:

```shell 
babylond query tx <transaction-hash> --chain-id bbn-test-5 
```

>Note: This query must be executed using the Babylon daemon (`babylond`), not
the finality provider daemon (`fpd`), as the registration transaction is
recorded on the Babylon blockchain.

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

When a finality provider is created, it's associated with two key elements:

**a) BTC Public Key:** - This serves as the unique identifier for the finality
provider.  - It's derived from a Bitcoin private key, likely using the secp256k1
elliptic curve.  - This key is used in the Bitcoin-based security model of
Babylon.

**b) Babylon Account:** - This is an account on the Babylon blockchain.  - It's
where staking rewards for the finality provider are sent.  - This account is
controlled by the key you use to create and manage the finality provider (the
one you added with fpd keys add).

This dual association allows the finality provider to interact with both the
Bitcoin network (for security) and the Babylon network (for rewards and
governance).

## Slashing Conditions

Slashing occurs when a finality provider **double signs**. This occurs when a
finality provider signs conflicting blocks at the same height. This results in
the extraction of the provider's private key and automatically triggers shutdown
of the finality provider.

### Withdrawing Rewards

When withdrawing rewards, you need to use the Babylon chain's CLI since rewards
are managed by the main chain.

To withdraw your finality provider rewards:

``` babylond tx incentive finality_provider ...  ```

<!-- //this code needs to be updated before further instructions can be provided
-->
