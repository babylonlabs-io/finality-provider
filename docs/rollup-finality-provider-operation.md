# Rollup Finality Provider Operation

This is an operational guide intended for technical Rollup finality provider 
administrators of rollup BSNs. This guide covers the complete
lifecycle of running a Rollup finality provider, including:

* Managing keys (EOTS key for EOTS signatures and Babylon Genesis key for rewards).
* Registering a Rollup finality provider on Babylon Genesis.
* Operating a Rollup finality provider.
* Withdrawing Rollup finality provider commission rewards.

Please review the [high-level explainer](../README.md) before proceeding to
gain an overall understanding of the finality provider.

> **⚠️ Important**: Rollup BSN integration requires the deployment of a
> [CosmoWasm smart contract](https://github.com/babylonlabs-io/rollup-bsn-contracts)
> on Babylon Genesis that is responsible for receiving finality signatures and
> maintaining the finality status of rollup blocks.
> Finality providers connect with both this CosmWasm contract and the Rollup itself.
> This is in contrast with Babylon Genesis finality providers which only need
> to interact with the Babylon Genesis chain.

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [System Requirements](#2-system-requirements)
3. [Keys Involved in Finality Provider Operation](#3-keys-involved-in-finality-provider-operation)
4. [Setting up the Finality Provider](#4-setting-up-the-finality-provider)
   1. [Initialize the Finality Provider Daemon](#41-initialize-the-finality-provider-daemon)
   2. [Add key for the Babylon Genesis account](#42-add-key-for-the-babylon-genesis-account)
   3. [Configure Your Finality Provider](#43-configure-your-finality-provider)
   4. [Starting the Finality Provider Daemon](#44-starting-the-finality-provider-daemon)
5. [Finality Provider Operations](#5-finality-provider-operations)
   1. [Create Finality Provider](#51-create-finality-provider)
   2. [Rewards](#52-rewards)
   3. [Set Up Operation Key](#53-set-up-operation-key)
   4. [Start Finality Provider](#54-start-finality-provider)
   5. [Status of Finality Provider](#55-status-of-finality-provider)
   6. [Edit finality provider](#56-edit-finality-provider)
   7. [Slashing](#57-slashing-and-anti-slashing)
   8. [Prometheus Metrics](#58-prometheus-metrics)
6. [Recovery and Backup](#6-recovery-and-backup)
   1. [Critical Assets](#61-critical-assets)
   2. [Backup Recommendations](#62-backup-recommendations)
   3. [Recover finality-provider db](#63-recover-finality-provider-db)
      1. [Recover local status of a finality provider](#631-recover-local-status-of-a-finality-provider)
      2. [Recover public randomness proof](#632-recover-public-randomness-proof)

## 1. Prerequisites

Before proceeding with this guide, you must complete the EOTS daemon setup 
process described in [EOTS Daemon Setup](./eots-daemon.md). This includes:

* Installing the finality provider toolset (`rollup-fpd` and `eotsd` binaries)
* Initializing and configuring the EOTS daemon
* Adding your EOTS key to the daemon
* Starting the EOTS daemon service

> ⚠️ **Critical**: The EOTS daemon must be running and accessible before you can 
> operate a finality provider.

> ⚠️ **Important**: Each Finality Provider must generate a new EOTS key.
> EOTS keys cannot be reused across different finality providers.

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
and [Recovery and Backup](#6-recovery-and-backup).

## 3. Keys Involved in Finality Provider Operation

Operating a finality provider involves managing multiple keys, each serving distinct 
purposes. Understanding these keys, their relationships, and security implications is 
crucial for secure operation.

| Aspect | EOTS Key                                                                                                                                                            | Babylon Genesis Key                                                                                                                                             | Operation Key                                                                                      |
|--------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| **Functions** | • Unique identifier of a finality provider for BTC staking<br>• Initial registration<br>• Signs finality votes and Schnorr signatures<br>• Generates randomness<br> | • Unique identifier of a finality provider for Babylon Genesis<br>• Initial registration<br>• Withdrawing accumulated rewards<br>• Setting withdrawal addresses | • Daily operational transactions<br>• Can be the same as the Babylon Genesis key (not recommended) |
| **Managed By** | `eotsd`                                                                                                                                                             | • `rollup-fpd`<br>• Can kept isolated after Operation Key is set                                                                                                       | `rollup-fpd`                                                                                              |
| **Mutability** | Immutable after registration                                                                                                                                        | Immutable after registration                                                                                                                                    | Rotatable (if separate from the Babylon Genesis key)                                               |
| **Key Relationships** | Permanently paired with Babylon Genesis Key during registration                                                                                                     | • Permanently paired with EOTS Key during registration<br>• Can delegate operations to Operation Key                                                            | • Not associated with the other keys<br>• Should be set after the finality provider is registered   |
| **Recommended Practices** | • Store backups in multiple secure locations<br>• Use dedicated machine for EOTS Manager                                                                            | • Store backups in multiple secure locations<br>• Setup the Operation Key right after registration<br>• Only use for reward operations                          | • Maintain minimal balance<br>• Monitor for balance and fund it when needed                        |
| **Security Implications** | • Loss is irrecoverable<br>• Cannot participate finality voting                                                                                                     | • Loss is irrecoverable<br>• Cannot withdraw rewards                                                                                                            | • Temporary service disruption<br>• Can be replaced with a new key<br>• Small loss of funds        |

Instructions of setting up the three keys can be found in the following places:

- [EOTS Daemon Setup - Add an EOTS Key](./eots-daemon.md#22-add-an-eots-key).
- [4.2. Add key for the Babylon Genesis account](#42-add-key-for-the-babylon-genesis-account).
- [5.3. Set Up Operation Key](#53-set-up-operation-key).

## 4. Setting up the Finality Provider

> ⚠️ **Critical**: One finality provider can serve only one Rollup BSN.  
> Each finality provider must use a unique EOTS key


### 4.1. Initialize the Finality Provider Daemon

To initialize the finality provider daemon home directory,
use the following command:

```shell
rollup-fpd init --home <path>
```

If the home directory already exists, `init` will not succeed.

> ⚡ Running this command with `--force` will overwrite the existing config file.
> Please ensure you have a backup of the existing config file before running
> this command.

**Home directory structure:**

```shell
~/.fpd/
│   fpd.conf       # Configuration file for the finality provider
├── data/
│   └── finality-provider.db         # Database containing finality provider data
├── keyring-*/         # Directory containing Babylon Genesis keys
└── logs/
    └── fpd.log        # Log file for the finality provider daemon
```

* **fpd.conf**: The main configuration file that defines:
  * Babylon Genesis Network settings (chain-id, node endpoints)
  * Rollup BSN Network details (node endpoints, rollup bsn contract address)
  * EOTS manager connection settings
  * Database configuration
  * Logging settings
  * RPC listener settings
  * Metrics configuration

* **finality-provider.db**: A bbolt database that stores:
  * Finality provider registration data
  * Public randomness proofs
  * Last voted block height

**keyring-directory**: Contains your Babylon Genesis account keys used for:
  * Submitting finality signatures to Babylon
  * Withdrawing rewards
  * Managing your finality provider
  * Loss means inability to operate until restored

* **fpd.log**: Contains detailed logs including:
  * Block monitoring events
  * Signature submissions
  * Error messages and debugging information
  * Service status updates

### 4.2. Add key for the Babylon Genesis account

Each finality provider maintains a Babylon Genesis keyring containing
an account used to receive BTC Staking reward commissions and pay fees for
transactions necessary for the finality provider's operation.

Since this key is accessed by an automated daemon process, it must be stored
unencrypted on disk and associated with the `test` keyring backend.
This ensures that the finality provider daemon can promptly submit
transactions, such as block votes and public randomness submissions that are
essential for its liveness and earning of rewards.

> **Note:** All finality provider transactions including vote submissions and 
> public randomness submissions require gas. 
> Because this keyring is used for both reward related and operational actions, 
> keep only the minimum needed balance here and move the rest to more secure storage.

> ⚠️ **Important**: To operate your Finality Provider, ensure your Babylon Genesis account 
> is funded.
> **Block vote transactions and public randomness submissions require gas payments.**

> ⚠️ **Notice:** Do not reuse the same Operation Key across multiple finality providers.  
> Doing so can cause sequence number mismatches and lead to failed transactions or 
> unexpected outages. Use a unique, rotatable Operation Key per FP.

Use the following command to add the Babylon Genesis key for your finality provider:

```shell
rollup-fpd keys add <key-name> --keyring-backend test --home <path>
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

### 4.3. Configure Your Finality Provider

Edit the `fpd.conf` file in your finality provider home directory with the
following parameters:

```shell
[Application Options]
RollupNodeRPCAddress = <rollup-bsn-rpc-endpoint>
FinalityContractAddress = <rollup-bsn-contract-address>
EOTSManagerAddress = 127.0.0.1:12582
RPCListener = 127.0.0.1:12581

[babylon]
Key = <finality-provider-key-name-signer> # the key you used above
ChainID = <babylon-genesis-chain-id> # chain ID of the Babylon Genesis
RPCAddr = http://127.0.0.1:26657 # Your Babylon Genesis node's RPC endpoint
KeyDirectory = <path> # The `--home` path to the directory where the keyring is stored
```

> ⚠️ **Important**: Operating a finality provider requires direct 
> connections to both a Babylon Genesis full node and a Rollup BSN node. 
> It is **highly recommended** to operate your own instances of
> full nodes instead of relying on third parties.
>
> ⚠️ **Critical RPC Configuration**:
> When configuring your finality provider to connect to
> a Babylon Genesis RPC node,
> you should connect directly to a single node.
> It is essential that this node has transaction indexing enabled
> (`indexer = "kv"`).
> Avoid using multiple RPC nodes or load balancers, as this can lead to synchronization issues.

> ⚠️ **Contract Requirement**: To start a finality provider you must 
> supply a valid Rollup BSN contract address associated with a registered BSN. 
> Additionally, the `btc_pk` obtained 
> from `eotsd` must be included in the contract’s allow-list. If the contract 
> address is missing/invalid or the `btc_pk` isn’t in the allow-list, the finality provider 
> will fail to start. 
> To learn more about the allow-list, see the 
> [parameter selection guidelines under `allowed_finality_providers`](https://github.com/babylonlabs-io/rollup-bsn-contracts/blob/main/docs/contract-managment.md#42-parameter-selection-guidelines)

Configuration parameters explained:

* `RollupNodeRPCAddress`: Your Rollup BSN node's RPC endpoint
* `FinalityContractAddress`: Address of the rollup finality contract
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
roughly 13h of usage with rollup block production time at `1s`.
Larger values can be set to tolerate longer downtime with larger size of
merkle proofs for each randomness, resulting in higher gas fees when submitting
future finality signatures and larger storage requirements.
<!-- TODO: Update this paragraph once we add filtering based on the public randomness submission interval. -->

### 4.4. Starting the Finality Provider Daemon

> ⚠️ **Note**: Before starting, ensure this finality provider’s EOTS public key 
> is allow-listed in the Rollup BSN contract; otherwise it will fail to start.

The finality provider daemon (`rollup-fpd`) needs to be running before proceeding with
registration or voting participation.

Start the daemon with:

``` shell
rollup-fpd start --home <path>
```

An example of the `--home` flag is `--home ./fpHome`.

The command flags:

* `start`: Runs the `rollup-fpd` daemon
* `--home`: Specifies the directory for daemon data and configuration
* `--eots-pk`: The finality provider instance that will be started identified
  by the EOTS public key.

It will start the finality provider daemon listening for registration and other
operations. If there is already a finality provider created (described in a
later [section](#51-create-finality-provider)), `rollup-fpd start` will also start
the finality provider. If there are multiple finality providers created,
`--eots-pk` is required.

The daemon will establish a connection with the Rollup BSN node, 
Babylon Genesis node and the rollup BSN finality contract, and
boot up its RPC server for executing CLI requests.

You should see logs indicating successful startup:

```shell
[INFO] Starting FinalityProviderApp
[INFO] RPC server listening...
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

### 4.5. Interaction with the EOTS Manager

There are two pieces to a finality provider entity: the EOTS manager and the
finality provider instance. These components work together and are managed by
separate daemons (`eotsd` and `rollup-fpd`).

The EOTS manager is responsible for managing the keys for finality providers and
handles operations such as key management, signature generation, and randomness
commitments. Whereas the finality provider is responsible for creating and
registering finality providers, monitoring the Rollup BSN, and
submitting finality votes on the finality contract deployed on Babylon Genesis.

The interactions between the EOTS Manager and the finality provider happen
through RPC calls. These calls handle key operations, signature generation,
and randomness commitments. An easy way to think about it is the EOTS Manager
maintains the keys while the FP daemon coordinates any interactions with the
Rollup BSN and the finality contract deployed on Babylon Genesis.

The EOTS Manager is designed to handle multiple finality provider keys, operating
as a centralized key management system. When starting a finality provider instance,
you specify which EOTS key to use through the `--eots-pk` flag. This allows you
to run different finality provider instances using different keys from the same
EOTS Manager. Note that someone having access to your EOTS Manager
RPC will have access to all the EOTS keys held within it.

For example, after registering a finality provider, you can start its daemon by
providing the EOTS public key `rollup-fpd start --eots-pk <hex-string-of-eots-public-key>`.

> ⚠️ **Note**: A single finality provider daemon can only run with a single
> finality provider instance at a time.

## 5. Finality Provider Operations

### 5.1. Create Finality Provider

The `create-finality-provider` command initializes a new finality provider,
submits `MsgCreateFinalityProvider` to register it on Babylon Genesis, and
saves the finality provider information in the database.

``` shell
rollup-fpd create-finality-provider \
  --chain-id <chain-id> \
  --eots-pk <eots-pk-hex> \
  --commission-rate 0.1 \
  --commission-max-rate 0.2 \
  --commission-max-change-rate 0.01 \
  --key-name finality-provider \
  --moniker "MyFinalityProvider" \
  --website "https://myfinalityprovider.com" \
  --security-contact "security@myfinalityprovider.com" \
  --details "finality provider for the Rollup BSN network" \
  --home ./fpHome
```

Required parameters:

* `--chain-id`: The Rollup BSN chain ID. Needs to be the same as the once used in
  the Rollup BSN finality contract
* `--eots-pk`: The EOTS public key maintained by the connected EOTS manager
  instance that the finality provider should use. If one is not provided the
  finality provider will request the creation of a new one from the connected
  EOTS manager instance.
* `--commission-rate`: The initial commission rate percentage (between 0 and 1)
  that you'll receive from delegators
* `--commission-max-rate`: The maximum commission rate percentage (between 0 and 1) that
  you can modify your commission to
* `--commission-max-change-rate`: The maximum commission change rate percentage
  (per day)
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
rollup-fpd create-finality-provider --from-file <path-to-json-file>
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
        "details": "finality provider for the Rollup BSN network"
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

### 5.2. Rewards

Rewards are calculated by the Rollup BSN based on finality providers’ participation 
in sending valid finality votes, with voting power typically the primary weight. 
They accumulate in a reward gauge and are bridged into Babylon Genesis as 
native `x/bank` assets. To learn more, see the 
[rewards distribution documentation](https://github.com/babylonlabs-io/babylon/blob/release/v3.x/x/btcstaking/docs/rewards-distribution.md)

#### 5.2.1. Querying Rewards

To query rewards of a given stakeholder address, use the following command.

```shell
rollup-fpd reward-gauges <address> --node <babylon-genesis-rpc-address>
```

Parameters:

* `<address>`: The Babylon Genesis address of the finality provider in bech32 format.
* `--node <babylon-genesis-rpc-address>`: <host>:<port> to Babylon Genesis
RPC interface for this chain (default `tcp://localhost:26657`)

#### 5.2.2. Withdraw Rewards

The `rollup-fpd withdraw-reward` command will withdraw all accumulated rewards of the
given finality provider. The finality provider must first be active and have
sent finality votes to be eligible to receive rewards.

```shell
rollup-fpd withdraw-reward <type> --from <registered-bbn-address>
--keyring-backend test --home <home-dir> --fees <fees>
--node <babylon-genesis-rpc-address>
```

> ⚠️ **Important**: The `rollup-fpd` should be **stopped** before performing this action.
> otherwise, account sequence mismatch error might be encountered because the key
> used for sending the withdrawal transaction is under use by the finality provider
> sending operational transaction. This issue will be resolved after following the
> setup instructions in [5.3. Set Up Operation Key](#53-set-up-operation-key).

The rewards will go to `<registered-bbn-address>` by default. If you want to
set a different address to receive rewards, please refer to
[5.2.3. Set Withdraw Address](#523-set-withdraw-address). But still, the
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
the rewards will be withdrawn to the finality provider's Babylon Genesis address used
in registration.

#### 5.2.3. Set Withdraw Address

The default beneficiary is the address that corresponds to the registration key.
To change the beneficiary address, use the following command:

```shell
rollup-fpd set-withdraw-addr <beneficiary-address> --from <registered-bbn-address>
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

### 5.3. Set Up Operation Key

Finality providers consume gas for operations with Babylon Genesis.
Therefore, it is recommended to use a separate key specifically for operations.
This leaves the key used for registration isolated and only needed for withdrawing
rewards. The operation key is totally replaceable in the sense that it is not
tied to a finality provider, and it can be replaced by any key that is properly
funded.
Therefore, it can hold only a minimum amount of funds to keep the finality
provider running for a long period of time.

You may follow the following procedure to set up the operation key.

1. Create an additional key for operation following steps in
   [4.2 Add key for the Babylon Genesis account](#42-add-key-for-the-babylon-genesis-account).

2. Fund the operation key with BABY tokens for gas cost.

3. Set the operation key name (`Key`) and the keyring directory (`KeyDirectory`)
   under `[babylon]` in `fpd.conf`.

4. Restart the finality provider daemon.

### 5.4. Start Finality Provider

After a successful registration and proper set up of the operation key,
you may start the finality provider instance by running:

```shell
rollup-fpd start --home <path> --eots-pk <hex-string-of-eots-public-key>
```

If `--eots-pk` is not specified, the command will start the finality provider
if it is the only one stored in the database. If multiple finality providers
are in the database, specifying `--eots-pk` is required.

> ⚠️ **Important**: The BTC public key (`btc_pk`) from `eotsd` must be in the 
> Rollup BSN contract's allow-list or the finality provider will fail to start.

### 5.5. Status of Finality Provider

Once the finality provider has been created, it will have the `REGISTERED` status.

Below you can see a list of the statuses that a finality provider can transition
to:

* `REGISTERED`: defines a finality provider that has been created and registered
  to the consumer chain but has no delegated stake
* `ACTIVE`: defines a finality provider that is delegated to vote
* `INACTIVE`: defines a finality provider whose delegations are reduced to
  zero but not slashed
* `SLASHED`: Defines a finality provider that has been permanently removed from
  the network for double signing (signing conflicting blocks at the same height).
  This state is irreversible.

To check the status of a finality provider, you can use the following command:

```shell
rollup-fpd finality-provider-info <hex-string-of-eots-public-key>
```

This will return the same response as the `create-finality-provider`
command but you will be able to check in real time the status of the
finality provider.

### 5.6. Edit Finality Provider

If you need to edit your finality provider's information, you can use the
following command:

```shell
rollup-fpd edit-finality-provider <hex-string-of-eots-public-key> \
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
rollup-fpd finality-provider-info <hex-string-of-eots-public-key>
```

### 5.7. Slashing and Anti-slashing

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

### 5.8. Prometheus Metrics

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

---

## 6. Recovery and Backup

### 6.1. Critical Assets

The following assets **must** be backed up frequently to prevent loss of service or funds:

* **keyring-*** directory: Contains your Babylon Genesis account keys used for:
  * Submitting finality signatures to Babylon
  * Withdrawing rewards
  * Managing your finality provider
  * Loss means inability to operate until restored
* **finality-provider.db**: Contains operational data including:
  * Public randomness proofs
  * State info of the finality provider
  * Loss of anti-slashing protection

### 6.2. Backup Recommendations

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

### 6.3. Recover finality-provider db

The `finality-provider.db` file contains both the finality provider's running
status and the public randomness merkle proof. Either information loss
compromised will lead to service halt, but they are recoverable.

#### 6.3.1. Recover local status of a finality provider

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

It can be recovered by downloading the finality provider's info from Babylon Genesis. 
Specifically, this can be achieved by repeating the 
[creation process](#51-create-finality-provider). The `create-finality-provider`
cmd will download the info of the finality provider locally if it is already
registered on Babylon.

#### 6.3.2. Recover public randomness proof

Every finality vote must contain the public randomness proof to prove that the
randomness used in the signature is already committed on the finality contract. Loss of
public randomness proof leads to direct failure of the vote submission.

To recover the public randomness proof, the following steps should be followed:

1. Ensure the `rollup-fpd` is stopped.
2. Run the recovery command
`rollup-fpd recover-rand-proof [eots-pk-hex] --start-height [height-to-recover] --chain-id [chain-id]`
where `start-height` is the height from which you want to recover from. If
the `start-height` is not specified, the command will recover all the proofs
from the first commit on Babylon, which incurs longer time for recovery.
The `chain-id` must be specified exactly the same as the `chain-id` used when
creating the finality provider.
4. Restart the finality provider