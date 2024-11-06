# Public Randomness Commit Specification

## Overview

The finality provider periodically commits public randomness to the consumer
chain to be used for future block finalization. This document specifies the
process of committing public randomness.

## Public Randomness Commit

A public randomness commit is essentially a list of public
randomness, each committed to a specific height. In particular, it consists of:

- a merkle root containing a list of public randomness values,
- a start height, indicating from which height the randomness starts, and
- the number of randomness contained in the merkle tree.

## Commit Process

Public randomness is an essential component of finality. It should be
committed before finality votes can be sent. Otherwise, the finality provider
looses voting power for this height.

To this end, when a finality provider is started, it runs a loop to periodically
check whether it needs to make a new commit. In particualar,
the following statement is checked:

```go
if lastCommittedHeight < currentHeight + uint64(MinRandHeightGap)
```

where:

- `lastCommittedHeight` is the end height (`startHeight + numRand - 1`)
from the latest public randomness commit recorded on the consumer chain
- `currentHeight` is the current height of the consumer chain
- `MinRandHeightGap` is a configuration value, which measures when to make a
  new commit

If the statement is true, a new commit should be made to ensure sufficient
randomness is available for future blocks.

### Determining MinRandHeightGap

The value of `MinRandHeightGap` must account for BTC-timestamping
delays, which is needed to activate the randomness for a specific height
after the committed epoch is BTC-timestamped. Here's an example:

- The consumer chain receives a commit with:
  - Start height: 100
  - Number of randomness values: 1000
  - Current epoch: 10
- This means randomness for heights [100, 1099] becomes available after epoch 10
  is finalized

The BTC-timestamping protocol requires:

- 100 BTC blocks for epoch finalization
- ≈ 1000 minutes (17 hours) at 10-minute average block time
- With consumer chain blocks every 10 seconds, this equals approximately 6,000
  blocks

Therefore,

- `MinRandHeightGap` should be > 6,000 to ensure randomness is always available
- Recommended production value: > 10,000 to provide additional safety margin

### Determining Start Height

To determine the start height of a commit:

1. For first-time commit:
   - `startHeight = baseHeight + 1`,
   - where `baseHeight` is a future height which is estimated based on the
     BTC-timestamping delays.
2. For subsequent commit:
   - `startHeight = lastCommittedHeight + 1`,
   - where `lastCommittedHeight` is obtained from the consumer chain.

The `baseHeight` can be specified via configuration or CLI options.

**Important Notes:**

- After long downtime, treat as first-time commit by specifying `baseHeight`.
- Consecutiveness across commits is not enforced by the system but
  different commits must not overlap.
- `startHeight` should not be higher than `finalityActivationHeight`,
a parameter defined in Babylon. Therefore,
`startHeight = max(startHeight, finalityActivationHeight)`.

### Determining the Number of Randomness

The number of randomness contained in a commit is specified in the config
`NumPubRand`. A general strategy is that the value should be as large
as possible. This is because each commit to the consumer chain costs gas.
  
However, in real life, this stategy might not always gain due to the following
reasons:

- A finality provider might not have voting power for every block. Randomness
  for those heights is a waste.
- Generating more randomness leads to a larger merkle proof size which will be
  used for sending finality votes.
- Generating randomness and saving the merkle proofs require time.

Additionally, given that the end height of a commit equals to
`startHeight + NumPubRand - 1`, we should ensure that the condition
`lastCommittedHeight > currentHeight + uint64(MinRandHeightGap)` can hold for
a long period of time to avoid frequent commit of randomness.
In real life, the value of `NumPubRand` should be much larger than
`MinRandHeightGap`, e.g., `NumPubRand = 2 * MinRandHeightGap`.
