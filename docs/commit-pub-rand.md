# Public Randomness Commit Specification

## Overview

The finality provider periodically commits public randomness to the consumer
chain to be used for future block finalization. This document specifies the
process of committing public randomness.

## Commit Process

A public randomness commit is composed by a merkle root of a list of public,
along with the start height and the number of randomness contained in the merkle
tree.
Public randomness is an essential component of finality. The finality provider
must commit randomness before it can send finality votes, ensuring the randomness
is available when needed.

To achieve this, randomness must be committed well in advance.
The finality provider runs a loop to check whether it needs to make a new commit
periodically. In particualar, the following statement is checked:

```go
if lastCommittedHeight < currentHeight + uint64(MinRandHeightGap)
```

If the statement is true, a new commit should be made to ensure sufficient
randomness is available for future blocks.

## Determining MinRandHeightGap

The value of `MinRandHeightGap` must account for the BTC-timestamping protocol,
which activates randomness for a specific height after the committed epoch is
BTC-timestamped. Here's an example:

- Consumer chain receives a commit with:
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

## Determining commit start height

The general rule of determing the start height of a commit is to the heights
of the randomness are consecutive. In particular,

1. For first-time commit:
   - `startHeight = baseHeight + 1`
   - where `baseHeight` is a future height which is estimated based on the
     BTC-timestamping time.
2. For subsequent commit:
   - `startHeight = lastCommittedHeight + 1`
   - Note that the finality provider might have very long down time. In this
     case, we can consider it as the same case as the first-time commit

Note that `startHeight` should not be higher than `finalityActivationHeight`,
a parameter defined in Babylon. Therefore,

```go
startHeight = max(startHeight, finalityActivationHeight)
```

Also note that consecutiveness is not enforced by the consumer chain but it
is required that different commits should not have overlaps.

## Determining the number of randomness

The number of randomness is specified in the config `NumPubRand`. A general
strategy is that the value should be as large as possible. This is because each
commit to the consumer chain costs gas.
  
However, in real life, this stategy might not always gain due to the following
reasons:

- a finality provider might not have voting power for every block. Randomness
  for those heights is a waste.
- generating more randomness leads to a larger merkle proof size which will be
  used for sending finality votes.
- generating randomness and saving the merkle proofs require time.

Additionally, given that the end height of a commit equals to
`startHeight + NumPubRand - 1`, we should ensure that the condition
`lastCommittedHeight > currentHeight + uint64(MinRandHeightGap)` can hold for
a long period of time to avoid frequent commit of randomness.
In real life, the value of `NumPubRand` should be much larger than
`MinRandHeightGap`, e.g., `NumPubRand = 2 * MinRandHeightGap`.
