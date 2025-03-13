# Slashing Protection on Finality Provider

In the BTC staking protocol, finality providers run a
Finality Provider Daemon (`fpd`) to send finality votes to Babylon.
If a finality provider signs two conflicting blocks (same committed
randomness but different block hash), all its delegations will be slashed.
Apart from malicious behavior, honest finality providers face [slashing risks](https://cubist.dev/blog/slashing-risks-you-need-to-think-about-when-restaking)
due to factors like hardware failures or software bugs.
Therefore, a proper slashing protection mechanism is required.

Recall that in our system, a finality provider needs to run two daemons:
1. The finality provider daemon (fpd), which connects to the Babylon node
   and initiates EOTS signing requests upon a new block to finalize
2. The EOTS manager daemon (eotsd), which manages the EOTS key and responds to
   signing requests from the finality provider daemon

The two daemons have different responsibilities to prevent double-signing.
The protections from the two daemons are complementary to each other.

### Finality provider daemon protection

**Assumption**:
- The Babylon node the daemon connects to is trusted and responsive.
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
   - `lastFinalizedHeight` (defaults to `0` if no blocks are finalized)
   - `finalityActivationHeight`
   - `highestVotedHeight` (defaults to `0` if no votes exist)

2. If local state is empty or broken:
   - Set `lastVotedHeight = lastFinalizedHeight`

3. Compare `lastVotedHeight` and `highestVotedHeight`:
   - If `lastVotedHeight >= highestVotedHeight`
     - `startHeight = lastVotedHeight + 1`
     - Note: this is possible if `highestVotedHeight` has not been updated due to
       execution delay.
   - If `lastVotedHeight < highestVotedHeight`
     - Query consumer chain for whether the fp has voted for any blocks
       in between `[lastVotedHeight + 1, highestVotedHeight)`
     - Send votes if there are any missed blocks
     - `startHeight = highestVotedHeight + 1`
     - Note: this is possible due to bugs or if the local state is tampered with

4. Start from `max(startHeight, finalityActivationHeight)`

Note that the mechanism shown above is not comprehensive in the sense that
it is still possible that the assumptions listed at the beginning
of the section do not hold, and the assurance might be broken.
One common example is that, during software upgrade,
the Babylon node might not be responsive. In this case, if the `fpd` is
restarted, it might send duplicate signing requests as the previous ones were
not processed.

Therefore, we also need the protection from the EOTS manager daemon, described
in the next section.

### EOTS manager daemon protection

**Assumption**:
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
