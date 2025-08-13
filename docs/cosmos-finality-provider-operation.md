# Cosmos Finality Provider Operation

This is an operational guide intended for technical Cosmos finality provider 
administrators of Cosmos BSNs. This guide covers the complete
lifecycle of running a Cosmos finality provider, including:

* Managing keys (EOTS key for EOTS signatures, Babylon Genesis key for rewards, 
  and Consumer BSN key for submissions).
* Registering a Cosmos finality provider on Babylon Genesis.
* Operating a Cosmos finality provider.
* Withdrawing Cosmos finality provider commission rewards.

Please review the [high-level explainer](../README.md) before proceeding to
gain an overall understanding of the finality provider.

> **⚠️ Important**: Cosmos BSN integration requires the deployment of 
> [CosmWasm smart contracts]() 
> on the consumer Cosmos chain that are responsible for receiving finality 
> signatures and maintaining the finality status of consumer BSN blocks.
> Cosmos Finality providers register with Babylon Genesis for the consumer BSN, 
> then query blocks from the consumer BSN and submit signatures to the CosmWasm 
> contracts on the consumer BSN.
> This is in contrast with Babylon native finality providers which only need
> to interact with the Babylon chain directly.

## 1. Prerequisites

Before proceeding with this guide, you must complete the EOTS daemon setup 
process described in [EOTS Daemon Setup](./eots-daemon.md). This includes:

* Installing the finality provider toolset (`cosmos-fpd` and `eotsd` binaries)
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

| Aspect | EOTS Key                                                                                                                                                            | Babylon Genesis Key                                                                                                                                             | Consumer BSN Key                                                                                 |
|--------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| **Functions** | • Unique identifier of a finality provider for BTC staking<br>• Initial registration<br>• Signs finality votes and Schnorr signatures<br>• Generates randomness<br> | • Unique identifier of a finality provider for Babylon Genesis<br>• Initial registration<br>• Withdrawing accumulated rewards<br>• Setting withdrawal addresses | • Submits finality signatures to consumer BSN contracts<br>• Submits public randomness commitments<br>• Pays transaction fees on consumer BSN |
| **Managed By** | `eotsd`                                                                                                                                                             | `cosmos-fpd`                                                                                                       | `cosmos-fpd`                                                                                              |
| **Mutability** | Immutable after registration                                                                                                                                        | Immutable after registration                                                                                                                                    | Rotatable                                                                                               |
| **Key Relationships** | Permanently paired with Babylon Genesis Key during registration                                                                                                     | Permanently paired with EOTS Key during registration                                                            | • Not associated with the other keys<br>• Must exist after registration before submissions start   |
| **Recommended Practices** | • Store backups in multiple secure locations<br>• Use dedicated machine for EOTS Manager                                                                            | • Store backups in multiple secure locations<br>• Only use for Babylon chain operations and reward withdrawals                          | • Maintain sufficient balance for transaction fees<br>• Monitor consumer chain and key balance, fund it when needed       |
| **Security Implications** | • Loss is irrecoverable<br>• Cannot participate finality voting                                                                                                     | • Loss is irrecoverable<br>• Cannot withdraw rewards                                                                                                            | • Temporary service disruption<br>• Can be replaced with a new key<br>• Loss of remaining balance        |

Instructions for setting up the three keys can be found in the following places:

- [EOTS Daemon Setup - Add an EOTS Key](./eots-daemon.md#22-add-an-eots-key)
- [4.2. Add key for the Babylon Genesis account](#42-add-key-for-the-babylon-genesis-account)
- [4.3. Add key for the Consumer BSN account](#43-add-key-for-the-consumer-chain-account)

## 4. Setting up the Finality Provider

> ⚠️ **Critical**: One finality provider can serve only one BSN.  
> Each finality provider must use a unique EOTS key

### 4.1. Initialize the Finality Provider Daemon

To initialize the finality provider daemon home directory,
use the following command:

```shell
consumer-fpd init --home <path>
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
  * Cosmos BSN Network details (keys,node endpoints, cosmos bsn contracts address)
  * EOTS manager connection settings
  * Database configuration
  * Logging settings
  * RPC listener settings
  * Metrics configuration

* **finality-provider.db**: A bbolt database that stores:
  * Finality provider registration data
  * Public randomness proofs
  * Last voted block height

* **keyring-directory**: Contains account keys for both Babylon Genesis and 
  Consumer BSN chains used for:
  * Babylon Genesis key: Registration, withdrawing rewards, 
    managing finality provider
  * Consumer BSN key: Submitting finality signatures and randomness to contracts

* **fpd.log**: Contains detailed logs including:
  * Block monitoring events
  * Signature submissions
  * Error messages and debugging information
  * Service status updates

### 4.2. Add keys for Babylon Genesis and Consumer BSN accounts

Each finality provider maintains all accounts in the keyring:

1. **Babylon Genesis account**: Used to register with Babylon and receive BTC 
   Staking reward commissions
2. **Consumer BSN account**: Used for submitting finality signatures and public 
   randomness submissions to consumer BSN contracts

Since these keys are accessed by an automated daemon process, they must be stored
unencrypted on disk and associated with the `test` keyring backend.
This ensures that the finality provider daemon can promptly submit transactions 
on both chains.

> **Note:** All finality provider transactions including **registration** and 
> **signature submissions** require gas. 
> Keep only the minimum needed balance in operational accounts and move the rest 
> to more secure storage.

> ⚠️ **Important**: Both accounts need to be funded:
> - **Babylon Genesis account**: Fund with Babylon tokens for **registration gas fees**
> - **Consumer BSN account**: Fund with consumer chain tokens 
    for **finality signature and public randomness submission gas fees**

> ⚠️ **Notice:** Do not reuse the same keys across multiple finality providers.  
> Doing so can cause **sequence number mismatches** and lead to **failed transactions** or 
> **unexpected outages**. Use **unique keys per finality provider**.

> 💡 **Recommendation**: Use different key names for each chain to avoid confusion and 
> ensure proper key management.

Use the following command to add key to your finality provider:
```shell
cosmos-fpd keys add <key-name> --keyring-backend test --home <path>
```
> **Note**: The same command is used to generate both the Babylon Genesis and 
  Consumer BSN accounts.
> Use different key names to distinguish them in your keyring.

The above `keys add` commands will create new key pairs and store them in your 
keyring.
The output should look similar to the one below:

``` json
{
  "address": "bbn19gulf0a4yz87twpjl8cxnerc2wr2xqm9fsygn9",
  "name": "babylon-fp-key",
  "pubkey": {
    "@type": "/cosmos.crypto.secp256k1.PubKey",
    "key": "AhZAL00gKplLQKpLMiXPBqaKCoiessoewOaEATKd4Rcy"
  },
  "type": "local"
}