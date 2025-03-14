# Slashing Protection on Finality Provider

In the BTC staking protocol, finality providers operate the
Finality Provider Daemon (`fpd`) to send finality votes to Babylon Genesis.
If a finality provider re-uses the same committed randomness
to sign two conflicting blocks on the same height,
their EOTS private key is exposed, leading to the slashing
of their delegations.
Apart from malicious behavior, honest finality providers face
[slashing risks](https://cubist.dev/blog/slashing-risks-you-need-to-think-about-when-restaking)
due to factors like hardware failures or software bugs.
To combat these risks, the finality provider program stack employs
an anti-slashing protection mechanism.

Recall that in our system, the finality provider operation stack involves
two daemons:
1. The EOTS manager daemon (`eotsd`) manages the EOTS key and responds to
   signing requests from the finality provider daemon.
2. The finality provider daemon (`fpd`) connects to the Babylon Genesis node
   and initiates EOTS signing requests upon a new block to finalize.

The two daemons have different responsibilities to prevent double-signing.
The anti-slashing protections of the two daemons are complementary to each other.
Even if the db file of one daemon is compromised, the protection is still
in effective, and the state will recover after restarting the service.

### Finality provider daemon protection

**Requirement**:
- The Babylon Genesis node the daemon connects to is trusted and responsive.
- The `finality-provider.db` file is not compromised.

The finality provider daemon ensures that it will never initiate
a signing request for the same height twice if the previous request succeeds.

To achieve this, the daemon does the following:

- Maintains a local state `lastVotedHeight`, which is updated once
  a vote submission succeeds, and never votes for a height that is not higher
  than `lastVotedHeight`.
- Polls blocks one-by-one in a monotonically increasing order.

Once a finality provider is restarted, it needs to determine which height to
start from, or bootstrapping. The bootstrapping needs to ensure that no blocks
will be missed and voted blocks will not be polled again. The bootstrapping
process is as follows:

1. Query consumer chain for:
   - `lastFinalizedHeight` (defaults to `0` if no blocks are finalized): the
   latest finalized height,
   - `finalityActivationHeight`: the height after which the finality is
   activated, defined as the finality parameters,
   - `highestVotedHeight` (defaults to `0` if no votes exist): the highest
   height for which the given finality provider has ever voted.

2. If local state is empty or broken:
   - Set `lastVotedHeight = lastFinalizedHeight`

3. Compare `lastVotedHeight` and `highestVotedHeight`:
   - If `lastVotedHeight >= highestVotedHeight`
     - `startHeight = lastVotedHeight + 1`
     - Note: this is possible if `highestVotedHeight` has not been updated due to
       execution delay.
   - If `lastVotedHeight < highestVotedHeight`
     - `startHeight = highestVotedHeight + 1`
     - Note: this is possible due to bugs or if the local state is tampered with

4. Start from `max(startHeight, finalityActivationHeight)`

Note that the mechanism shown above is not comprehensive in the sense that
it is still possible that the assumptions listed at the beginning
of the section do not hold, and the assurance might be broken.
One common example is that, during software upgrade,
the Babylon Genesis node might not be responsive. In this case, if the `fpd` is
restarted, it might send duplicate signing requests as the previous ones were
not processed.

Therefore, we also need the protection from the EOTS manager daemon, described
in the next section.

### EOTS manager daemon protection

**Requirement**:
- The `eots.db` file is not compromised.

The EOTS manager daemon ensures that EOTS signatures will not be signed
twice for the same height. To achieve this, the daemon keeps track of all the
signing histories in the EOTS manager. The signing record is defined below:

```go
type SigningRecord struct {
    Height      uint64
    BlockHash   []byte
    PublicKey   []byte
    Signature   []byte
    Timestamp   time.Time
}
```

For each EOTS signing request, the following checks are performed:

- Check if the height is already signed in the local storage.
  - If yes, check if the signing message in the request matches the previously
    signed message.
    - If yes, return the previous vote.
    - If no, return error with double-signing warning.
  - If no, sign the EOTS signature, save it in the local storage by height,
    and return the vote.

The local storage of the EOTS manager should be backed up periodically, and
corruption checks should be performed before the signing service starts.
Pruning of old records can be done with configurable retention policies.

### Operation Recommendations

Detailed specifications on the secure operation of the finality provider
program stack can be found in
the [Finality Provider Operation](./finality-provider-operation.md) document.
Here, we list security tips specifically for preventing double-sign:
- Operate your own Babylon Genesis RPC node and securely connect with it
  to ensure a trustless setup
- The keyring files or the mnemonic phrases should be backed up and kept safe
- Operate `fpd` and `eotsd` in separate machines connected in a secure
  network (config `EOTSManagerAddress` in `fpd.conf`)
- Set up HMAC for authentication between the two daemons.
  Details in [HMAC Security](./hmac-security.md)
- Backup the db files for both daemons periodically
  (one-hour interval is recommended)
