# Finality Votes Submission Specification

## Overview

Finality providers submit votes to finalize blocks on the consumer chain.
This document specifies the process of submitting finality votes.

## Internal State

The finality provider maintains two critical persistent states:

- Last voted height
- Last processed height

### Last voted height

Tracks the most recent height for which a finality vote was successfully
submitted. This prevents duplicate voting on previously voted heights.

### Last processed height

Tracks the most recent height for which a voting decision was made
(whether the decision was to vote or not).
By definition, `LastProcessedHeight >= LastVotedHeight` always holds.

## Submission Process

### Bootstraping

To determine the initial processing height:

1. Query consumer chain for `lastFinalizedHeight`
2. If no finalized blocks exist:
   - Start from `finalityActivationHeight`
3. Query `lastVotedHeightRemote` from consumer chain
4. If no previous votes from this provider:
   - Start from `lastFinalizedHeight + 1`
5. Synchronize local state:
   - Ensure local `lastVotedHeight` and `lastProcessedHeight` ≥ `lastVotedHeightRemote`
6. Begin processing at:
   - `max(lastProcessedHeight + 1, lastFinalizedHeight + 1)`

### Normal Submission Loop

After the finality provider is bootstraped, it continuously monitors for
new blocks. For each new block, it performs these validation checks:

1. Block hasn't been previously processed
2. Block height exceeds finality activation height
3. Finality provider has sufficient voting power

Upon passing all checks, the finality provider:

1. Requests a finality signature from the EOTS manager
2. Submits the vote transaction to the consumer chain
3. Implements retry logic until either:
   - Maximum retry attempts are reached
   - Block becomes finalized

#### Batch submission

A batch submission mechanism is needed to deal with cases where:

- recovery from downtime, and
- the consumer chain has rapid block production.

Batch sumission puts multiple new blocks into a batch and
process them in the same loop, after which all the finality votes will be sent
in the same transaction to the consumer chain.

### Generating Finality Votes

Each finality vote contains:

1. EOTS signature for the target block (signed by EOTS manager)
2. Public randomness for the block height
3. Merkle inclusion proof for the public randomness

The consumer chain verifies:

- EOTS signature validity
- Randomness was pre-committed and BTC-timestamped
