# Public Randomness Commit Specification

## Overview

The finality provider periodically commits public randomness to the consumer
chain to be used for future block finalization. This document specifies the
process for committing public randomness.

## Commit Process

### Generating a Commit

A randomness pair is essentially a pair of `32-byte` points over `secp256k1`.
A public randomness commit is a commitment to an association
between public randomnesses and heights for them to be used.
In particular, a commit consists of:

- a merkle root containing a list of public randomness values,
- a start height, indicating from which height the randomness starts, and
- the number of randomness contained in the merkle tree.

To generate a new commit, following steps are needed:

1. Generate a list of randomness. This requires an RPC call to the EOTS manager
   daemon (`eotsd`) to generate a list of public randomness,
   each corresponding to a specific height according to the start height and
   the number of randomness in the request.
   Randomness generation is required to be deterministic.
2. Construct the merkle tree based on the list of randomness using the CometBFT's
   [merkle](https://github.com/cometbft/cometbft/tree/main/crypto/merkle)
   library. The merkle root will be used in the commit, while each randomness
   number and their merkle proofs will be used for finality vote submission
   in the future.
3. Send a
   [Schnorr](https://github.com/btcsuite/btcd/blob/684d64ad74fed203fb846c032f2b55b3e3c36734/btcec/schnorr/signature.go#L391)
   signature request to the EOTS manager over the hash of the commit
   (concatenated by the start height, number of randomness, and the merkle root).
4. Build the commit message
   ([MsgCommitPubRandList](https://github.com/babylonlabs-io/babylon/blob/aa99e2eb093e06cb9a28a58f373e8fa5f2494383/proto/babylon/finality/v1/tx.proto#L29))
   and send a transaction to Babylon.

### Timing to Commit

Public randomness is an essential component of finality voting. It should be
committed before finality votes can be sent.
If the finality provider does not have public randomness committed for a
specific height, they can't vote for it and therefore lose the voting power
they would have otherwise had for it.

To this end, when a finality provider is started, it runs a loop to periodically
check whether it needs to make a new commit and calculate the height
its next public randomness commit should start with. In particular:

```go
    // Estimate the consumer chain block height
    // a new commitment of public randomness would be Bitcoin timestamped
    // and ready for use, if the commitment was made in the current
    // consumer chain block.
	tipHeightWithDelay := tipHeight + uint64(fp.cfg.TimestampingDelayBlocks)
	var startHeight uint64
	switch {
	case lastCommittedHeight < tipHeightWithDelay:
        // If the last height the finality provider has committed
        // randomness, is less than the estimation, then
        // the finality provider should commit randomness from that height.
		startHeight = tipHeightWithDelay
	case lastCommittedHeight < tipHeightWithDelay+uint64(fp.cfg.NumPubRand):
        // otherwise, the finality provider should commit randomness
        // from the last height it has committed to
		startHeight = lastCommittedHeight + 1
	default:
        // randomness is sufficient, do not need to make a commit
```

where:

- `lastCommittedHeight` is the end height (`startHeight + numRand - 1`)
  of the latest public randomness commit recorded on the consumer chain
- `tipHeight` is the current height of the consumer chain
- `TimestampingDelayBlocks` is a configuration value, which measures when to make a
  new commit
- `NumPubRand` is the number of randomness in a commit defined in the config.

### Determining `TimestampingDelayBlocks`

The configuration value `TimestampingDelayBlocks` defines an estimation
estimation of the number of consumer chain blocks that will be generated
until a public randomness commit is Bitcoin timestamped.

Here's an example:
- The consumer chain receives a commit with:
  - Start height: 100
  - Number of randomness values: 1000
  - Current epoch: 10
- This means randomness for heights [100, 1099] becomes available after epoch 10
  is finalized

The BTC-timestamping protocol requires:

- 300 BTC blocks for epoch finalization
- ≈ 3000 minutes (50 hours) at 10-minute average block time
- With consumer chain blocks every 10 seconds, this equals approximately 18,000
  blocks

Therefore,

- `TimestampingDelayBlocks` should be around 18,000
- Recommended production value: > 20,000 to provide additional safety margin

### Determining the Start Height for a Commit

To determine the start height for a commit:

1. If this is a first time commit or the finality provider has not committed
   for a long time:
   - `startHeight = currentConsumerChainHeight + TimestampingDelayBlocks + 1`
2. For subsequent commits:
   - `startHeight = lastCommittedHeight + 1`,
   - where `lastCommittedHeight` is obtained from the consumer chain.

The `baseHeight` can be specified via configuration or CLI options.

> **Important Notes:**
> - Consecutiveness across commits is not enforced by the system but
>   different commits must not overlap.
> - `startHeight` should not be higher than `finalityActivationHeight`,
>    a parameter defined in Babylon Genesis specifying when the finality
>    protocol is activated.
>    Therefore, `startHeight = max(startHeight, finalityActivationHeight)`.

### Determining the Number of Randomness

The number of randomness contained in a commit is specified in the `NumPubRand`
configuration. A general strategy is that the value should be as large
as possible. This is because each commit of the consumer chain costs gas.

However, in real life, this stategy might not always gain due to the following
reasons:

- A finality provider might not have voting power for every block. Randomness
  for those heights is a waste.
- Generating more randomness leads to a larger merkle proof size which will be
  used for sending finality votes.
- Generating randomness and saving the merkle proofs require time.

Additionally, given that the end height of a commit equals to
`startHeight + NumPubRand - 1`, we should ensure that the condition
`lastCommittedHeight > tipHeight + uint64(TimestampingDelayBlocks)` can hold for
a long period of time to avoid frequent commit of randomness.

> **Important Note**:
> In real life, the value of `NumPubRand` should be much larger than
> `TimestampingDelayBlocks`, e.g., `NumPubRand = 2 * TimestampingDelayBlocks`.

> **Important Note**:
> The finality provider daemon enforces a minimum
> of `16384` of `NumPubRand` and a maximum of `50000`.
