# Recovery and Backup

This document provides comprehensive guidance on backing up and recovering critical 
assets for finality providers and EOTS managers.

## Table of Contents

1. [Critical Assets](#1-critical-assets)
2. [Backup Recommendations](#2-backup-recommendations)
3. [Recover finality-provider db](#3-recover-finality-provider-db)
   1. [Recover local status of a finality provider](#31-recover-local-status-of-a-finality-provider)
   2. [Recover public randomness proof](#32-recover-public-randomness-proof)

## 1. Critical Assets

The following assets **must** be backed up frequently to prevent loss of service 
or funds:

For EOTS Manager:

* **keyring-*** directory: Contains your EOTS private keys used for signing. 
Loss of these keys means:
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

## 2. Backup Recommendations

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

## 3. Recover finality-provider db

The `finality-provider.db` file contains both the finality provider's running
status and the public randomness merkle proof. Either information loss
compromised will lead to service halt, but they are recoverable.

### 3.1. Recover local status of a finality provider

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

It can be recovered by downloading the finality provider's info from Babylon 
Genesis. Specifically, this can be achieved by repeating the 
[creation process](./finality-provider-operation.md#51-create-finality-provider). 
The `create-finality-provider` cmd will download the info of the finality provider 
locally if it is already registered on Babylon.

### 3.2. Recover public randomness proof

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