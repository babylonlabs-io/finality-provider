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

> ⚠️ **Critical**: One finality provider can serve only one Cosmos BSN.  
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

**keyring-directory**: Contains account keys for both Babylon Genesis and 
Consumer BSN chains used for:
  * Babylon Genesis key: Registration, withdrawing rewards, 
    managing finality provider
  * Consumer BSN key: Submitting finality signatures and randomness to contracts

* **fpd.log**: Contains detailed logs including:
  * Block monitoring events
  * Signature submissions
  * Error messages and debugging information
  * Service status updates