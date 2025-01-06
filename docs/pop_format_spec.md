# Proof of Possession

This PoP (Proof of Possession) has a goal proves that one EOTS
key pair owener, also owns an BABY key pair.

This leads us to the need of 5 attributes to validate it all, the structure
[`PoPExport`](https://github.com/babylonlabs-io/finality-provider/blob/56f7a3908402f0ffb895d92fe692219540f80e69/eotsmanager/cmd/eotsd/daemon/pop.go#L36)
exposes:

- `EotsPublicKey` is the finality provider EOTS public key marshal as hex.
- `BabyPublicKey` is the secp256k1 public key marshal as hex.
- `BabyAddress` is the BABY address with `bbn` prefix.
- `EotsSignBaby` is the schnorr signature that the private key of the EOTS
public key signed the `sha256(BabyAddress)` in base 64.
- `BabySignEotsPk` is the cosmos
[ADR-036](https://github.com/cosmos/cosmos-sdk/blob/main/docs/architecture/adr-036-arbitrary-signature.md)
compatible signature of the BABY private key over the `EotsPublicKey` in base 64.

One example of JSON representation structure of the `PoPExport` is the following:

```json
{
  "eotsPublicKey": "3d0bebcbe800236ce8603c5bb1ab6c2af0932e947db4956a338f119797c37f1e",
  "babyPublicKey": "A0V6yw74EdvoAWVauFqkH/GVM9YIpZitZf6bVEzG69tT",
  "babySignBtc": "AOoIG2cwC2IMiJL3OL0zLEIUY201X1qKumDr/1qDJ4oQvAp78W1nb5EnVasRPQ/XrKXqudUDnZFprLd0jaRJtQ==",
  "btcSignBaby": "pR6vxgU0gXq+VqO+y7dHpZgHTz3zr5hdqXXh0WcWNkqUnRjHrizhYAHDMV8gh4vks4PqzKAIgZ779Wqwf5UrXQ==",
  "babyAddress": "bbn1f04czxeqprn0s9fe7kdzqyde2e6nqj63dllwsm"
}
```

Function to validate the `PoPExport` was created as
[`VerifyPopExport`](https://github.com/babylonlabs-io/finality-provider/blob/cc07bcd4dc434f7095668724aad6865bffe425e0/eotsmanager/cmd/eotsd/daemon/pop.go#L211).
