# Proof of Possession (PoP) Specification

## Overview

The Proof of Possession (PoP) mechanism verifies that the owner of an EOTS key
pair also controls a BABY key pair. This validation requires five essential
attributes, which are exposed by the `PoPExport` structure.

## Attributes

The `PoPExport` structure is defined
[here](https://github.com/babylonlabs-io/finality-provider/blob/cc07bcd4dc434f7095668724aad6865bffe425e0/eotsmanager/cmd/eotsd/daemon/pop.go#L36).
It contains the following fields:

- `EotsPublicKey` – The EOTS public key of the finality provider, marshaled as
a hexadecimal string. This value represents the public component of the EOTS
key pair used in the signing process.
- `BabyPublicKey` – The secp256k1 public key, marshaled as a base64 string.
This key is extracted from the BABY keyring and uniquely identifies the BABY
key pair.
- `BabyAddress` – The BABY address, prefixed with `bbn`. It is derived from the
`BabyPublicKey` and used as the primary identifier on the Babylon network.
- `EotsSignBaby` – A Schnorr signature, created by signing the
`sha256(BabyAddress)` with the private key corresponding to the EOTS public
key. Encoded in base64, this ensures that the EOTS key can verify ownership
of the BABY address.
- `BabySignEotsPk` – A signature of the `EotsPublicKey`, created by the BABY
private key. This signature follows the Cosmos
[ADR-036](https://github.com/cosmos/cosmos-sdk/blob/main/docs/architecture/adr-036-arbitrary-signature.md)
specification and is encoded in base64. It demonstrates that the BABY key pair
acknowledges and signs the EOTS public key.

## Example

Below is an example JSON representation of the PoPExport structure:

```json
{
  "eotsPublicKey": "3d0bebcbe800236ce8603c5bb1ab6c2af0932e947db4956a338f119797c37f1e",
  "babyPublicKey": "A0V6yw74EdvoAWVauFqkH/GVM9YIpZitZf6bVEzG69tT",
  "babySignEotsPk": "AOoIG2cwC2IMiJL3OL0zLEIUY201X1qKumDr/1qDJ4oQvAp78W1nb5EnVasRPQ/XrKXqudUDnZFprLd0jaRJtQ==",
  "eotsSignBaby": "pR6vxgU0gXq+VqO+y7dHpZgHTz3zr5hdqXXh0WcWNkqUnRjHrizhYAHDMV8gh4vks4PqzKAIgZ779Wqwf5UrXQ==",
  "babyAddress": "bbn1f04czxeqprn0s9fe7kdzqyde2e6nqj63dllwsm"
}
```

## Validation

The function responsible for validating the `PoPExport` is `VerifyPopExport`,
which can be found [here](https://github.com/babylonlabs-io/finality-provider/blob/cc07bcd4dc434f7095668724aad6865bffe425e0/eotsmanager/cmd/eotsd/daemon/pop.go#L211).

`VerifyPopExport` ensures the authenticity and integrity of the `PoPExport`
by cross-verifying the provided signatures and public keys. This process
consists of two core validation steps:

- `VerifyEotsSignBaby` – This function checks the validity of the Schnorr
signature `(EotsSignBaby)` by verifying that the EOTS private key has correctly
signed the SHA256 hash of the BABY address.
- `VerifyBabySignEots` – This function confirms that the BABY private key has
signed the EOTS public key `(EotsPublicKey)`, ensuring mutual validation
between the key pairs.

If both signatures pass verification, the export is deemed valid, confirming
that the finality provider holds both key pairs. This function plays a critical
role in maintaining trust and security in the finality provider's key
management process.
