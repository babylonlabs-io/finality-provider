# Finality Provider Registration

Phase-2 launch of the Babylon network is a critical transition
that introduces active participation in finality voting, 
rewards distribution, and the Proof of Stake (PoS) system. 
This guide provides a step-by-step process for operators to 
onboard, whether setting up a new or pre-existing setup, 
with a focus on Phase-2 specific requirements.

The following explains the migration from Phase-1 to Phase-2.

**Phase-1 (Previous phase)**

- Only involves Bitcoin holders locking their assets on the Bitcoin chain
- No active Proof of Stake (PoS) chain operation
- Finality providers only need EOTS keys generated
- No need to run finality service

**Phase-2 (New phase)**

- Active participation in finality voting
- Running the complete finality provider toolset:
  - Babylon full node
  - EOTS manager daemon
  - Finality provider daemon
- Earning rewards from Bitcoin delegations

There are 2 paths for setting up a finality provider:

1. **Has existing EOTS Key**
   - Already generated a EOTS key pair from [Phase-1](https://github.com/babylonlabs-io/networks/tree/main/bbn-test-5/finality-providers) 
   <!-- TODO: waiting on merge, ensure this link is up to date -->
   - Reference the existing EOTS key in setup

If you have an existing EOTS key from Phase-1, please skip to 
[Loading Existing Keys](#loading-existing-keys) 
steps

>Note: Any finality providers from Phase-1 that do not transition their existing key, cannot claim any rewards if they dont have their Finality Provider setup on Phase-2

2. **New Setup** 
 - For operators starting fresh
 - Need to generate the EOTS key first
 - Complete full configuration process

If you are a new operator, start with the 
[Install Finality Provider Binary](#install-finality-provider-binary) section.

## Overview of Keys for Finality Provider and EOTS Manager

There are two distinct keys you'll be working with:

- **EOTS Key**: 
  - Used for generating EOTS signatures, Schnorr signatures, and randomness pairs
  - This serves as the unique identifier for the finality provider
  - It's derived from a Bitcoin private key, likely using the secp256k1
  elliptic curve.  
  - Stored in the EOTS manager daemon's keyring
  - This key is used in the Bitcoin-based security model of Babylon.

- **Babylon Key**: 
  - Used for signing transactions on Babylon
  - It's where staking rewards for the finality provider are sent. 
  - Associated with a Babylon account that receives rewards
  - Stored in the finality provider daemon's keyring
  - This account is controlled by the key you use to create and manage the 
finality provider (the one you added with `fpd keys add`).

This dual association allows the finality provider to interact with both the
Bitcoin network (for security) and the Babylon network (for rewards and
governance).

Once a finality provider is created, neither key can be rotated or changed - 
they are permanently associated with that specific finality provider instance.

## Install Finality Provider Binary 
<!-- TODO: check add in the correct tag for the testnet --> 

Download and install [Golang 1.23](https://go.dev/dl). 
Verify the installation with the following command:

```shell 
go version 
```

### Step 1: Clone the Finality Provider Repository

Subsequently clone the finality provider 
[repository](https://github.com/babylonlabs-io/finality-provider) and navigate to the 
`bbn-test-5` tag. 
<!-- TODO: change to a specific version -->

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
- Make commands globally accessible from your terminal

### Step 3: Verify Installation

Run `eotsd --help` to check the available actions:

```shell 
eotsd --help
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
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
```

## Run Babylon Full Node

Before proceeding with the finality provider setup, you'll need to have a Babylon 
full node running. Please follow the instructions at:

[Babylon Node Setup Guide](https://github.com/babylonlabs-io/networks/blob/main/bbn-test-5/babylon-node/README.md) <!-- TODO: Update link when mainnet is live -->

Once you have completed the node setup and it is fully synced, return here 
to continue with the finality provider configuration.

## Setting up the EOTS Manager

>Note: If you have already set up an EOTS Manager, you can skip this section. 
The following steps are only for users who have not yet set up an EOTS Manager. 
Phase-1 users who already had an EOTS Manager set up can skip to the 
[Loading Existing Keys](#loading-existing-keys) 
section at the end of this guide for specific instructions 
on re-using their Phase-1 EOTS keys.

After the full node has been setup, the operator can set up and run the
Extractable One Time Signature (EOTS) manager daemon.

The EOTS daemon is responsible for managing EOTS keys, producing EOTS
randomness, and using them to produce EOTS signatures. To read more on the EOTS
Manager see [here](#)

### Step 1: Initialize the EOTS Manager

Use the `eotsd init` command to initialize a home directory for the EOTS Manager.
You can set or change your home directory using the `--home` tag. For example, use 
`--home ./eotsHome` to specify a custom directory.

```shell 
eotsd init --home <path> 
```

### Step 2: Create an EOTS Key

Once the EOTS Manager is initialized, you need to create an EOTS key:

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

>**Important**: The mnemonic phrase must be stored securely and kept private, as it is the 
only way to recover your EOTS key if access is lost and is critical for maintaining control 
of your finality provider operations.

## Loading Existing Keys 

>Note: If you participated in Phase-1, follow these steps to load your existing EOTS 
key and configure it for Phase-2. 

>This section is only for Finality Providers who participated in Phase-1 and already 
had an EOTS key. If you are a new user, you can skip this section unless you would like to 
familiarlize yourself with the backup and recovery process. 

### Step 1: Verify Your EOTS Key Backup
>Note: Before proceeding, ensure you have access to your original EOTS key from Phase-1. 
This is the same key that was registered in the [Phase-1 registration](https://github.com/babylonlabs-io/networks/tree/main/bbn-test-5/finality-providers). 
<!-- TODO: update link when merged -->

### Step 2: Import Your EOTS Key into the Keyring

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

## Starting the EOTS Daemon

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

>**Note**: It is recommended to run the `eotsd` daemon on a separate machine or
network segment to enhance security. This helps isolate the key management
functionality and reduces the potential attack surface. You can edit
the `EOTSManagerAddress` in the configuration file of the finality provider to
reference the address of the machine where `eotsd` is running.

## Setting up the Finality Provider

### Step 1: Initialize the Finality Provider Daemon

Use the `fpd init` command to initialize a home directory for the Finality Provider. 
You can set or change the home directory using the `--home` tag. For example, use 
`--home ./fpHome` to specify a custom directory.

```shell 
fpd init --home <path> 
```

>Note: Running this command may return the message 
`service injective.evm.v1beta1.Msg does not have cosmos.msg.v1.service proto annotation`, 
which is expected and can be ignored.

### Step 2: Add key for the Babylon account

The keyring is kept in the local storage of the finality provider daemon. 
The key associates a Babylon account with the finality provider to receive BTC 
delegation rewards.

Use the following command to add the Babylonkey for your finality provider:

```shell 
fpd keys add --keyname <key-name> --keyring-backend test --home <path>
```

We use `--keyring-backend test`, which specifies which backend to use for the
keyring, `test` stores keys unencrypted on disk. For production environments,
use `file` or `os` backend.

 There are three options for the keyring backend:

 `test`: Stores keys unencrypted on disk. It’s meant for testing purposes and
 should never be used in production.  
 `file`: Stores encrypted keys on disk,
 which is a more secure option than test but less secure than using the OS
 keyring.  
 `os`: Uses the operating system's native keyring, providing the
 highest level of security by relying on OS-managed encryption and access
 controls.

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

>Note: This command will automatically update the `Key` field in 
the config file to use this key name. This key will be used for all interactions 
with the Babylon chain, including finality provider registration and transaction 
signing.

>Note: Please verify the `chain-id` and other network parameters from the official 
Babylon Networks repository at [https://github.com/babylonlabs-io/networks/tree/main/bbn-test-5/finality-providers](https://github.com/babylonlabs-io/networks/tree/main/bbn-test-5/finality-providers)

>The configuration below requires to point to the path where this keyring is
 stored `KeyDirectory`. This `Key` field stores the key name used for
 interacting with the babylon chain and will be specified along with
 the `KeyringBackend`field in the next step. So we can ignore the setting of the
 two fields in this step.

Once the node is initialized with the above command. It should generate a
`fpd.config` Edit the `config.toml` to set the necessary parameters with the
below

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

### Step 3: Verify the Key Import
After importing, verify that your EOTS key was successfully loaded:

```shell 
eotsd keys list <key-name> --keyring-backend test --home <path>
```

You should see your EOTS key listed with the correct details, confirming that 
it has been imported correctly.

>Note: Make sure you're using the same key name and EOTS public key that were 
registered in Phase-1.

## Starting the Finality Provider Daemon

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
in `fpd.conf` under the `RpcListener` field, which has a default value
of `127.0.0.1:12581`. You can change this value in the configuration file or
override this value and specify a custom address using
the `--rpc-listener` flag.

To start the daemon with a specific finality provider instance after 
registration, use the `--eots_pk_hex` flag followed by the hex string of the EOTS 
public key of the finality provider.

All the available CLI options can be viewed using the `--help` flag. These
options can also be set in the configuration file.

## Create Finality Provider

The `create-finality-provider` command initializes a new finality provider
instance locally. 

This command generates a BTC public key that uniquely identifies your finality provider. 

``` shell
fpd create-finality-provider \ 
--daemon-address 127.0.0.1:12581 \ 
--chain-id bbn-test-5 \ 
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

- `<btc-public-key>`: Your BTC public key from create-finality-provider (e.g., "cf0f03b9ee2d4a0f27240e2d8b8c8ef609e24358b2eb3cfd89ae4e4f472e1a41")
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

## Slashing Conditions

Slashing occurs when a finality provider **double signs**. This occurs when a
finality provider signs conflicting blocks at the same height. This results in
the extraction of the provider's private key and automatically triggers shutdown
of the finality provider.

## Jailing and Unjailing

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
- Then send the unjail transaction to the Babylon chain.

So while slashing is permanent, jailing is a temporary state that can be recovered 
from through the unjailing process, as long as the finality provider wasn't slashed.

## Public Randomness Submission
<!-- Waiting for clarification on this -->

## Reading the logs

The logs are stored based on your daemon home directories:

- EOTS Daemon logs
<eots-home>/logs/eotsd.log

- Finality Provider Daemon logs
<fpd-home>/logs/fpd.log

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

## Withdrawing Rewards

When withdrawing rewards, you need to use the Babylon chain's CLI since rewards
are managed by the main chain.

To withdraw your finality provider rewards:

``` babylond tx incentive finality_provider ...  ```

<!-- //this code needs to be updated before further instructions can be provided
-->
