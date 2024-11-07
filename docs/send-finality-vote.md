# Finality Votes Submission Specification

## Overview

Finality providers submit votes to finalize blocks on the consumer chain.
This document specifies the process of submitting finality votes, including
bootstrapping, submission loops, and fast sync mechanisms.

## Finality Votes

A finality vote consists of:

- A signature over the block to be finalized
- The public randomness associated with that block height
- A merkle inclusion proof for the public randomness

The signature is generated using a one-time signing key derived from the finality provider's master key. The public randomness must have been previously committed to the consumer chain.

## Submission Process

### Bootstrapping

When a finality provider starts, it performs the following bootstrap sequence:

1. Query the latest block from the consumer chain
2. Check if fast sync is needed by comparing the last processed height:
   ```go
   if currentHeight >= lastProcessedHeight + FastSyncGap
   ```
3. If fast sync is needed, perform fast sync to catch up
4. Determine the starting height for the poller:
   - If in auto mode:
     - `startHeight = max(lastProcessedHeight + 1, lastFinalizedHeight + 1, finalityActivationHeight)`
   - If in static mode:
     - Use configured static starting height

### Submission Loop

The finality provider runs a continuous loop that:

1. Waits for new blocks from the chain poller
2. For each new block:
   - Checks if block has already been processed
   - Verifies block is after finality activation height
   - Confirms finality provider has voting power
   - Generates and submits finality signature
   - Retries submission until either:
     - Signature is successfully submitted
     - Block becomes finalized
     - Maximum retries reached
     - Provider is shut down

**Important Notes:**
- Votes cannot be submitted for already finalized blocks
- Each block requires its own unique public randomness
- Submission failures are retried with configurable intervals
- Critical errors (e.g., slashing) terminate the loop

## Fast Sync

Fast sync is triggered when the provider falls behind by more than `FastSyncGap` blocks. The process:

1. Determines sync start height:
   ```go
   startHeight = max(lastProcessedHeight + 1, lastFinalizedHeight + 1, finalityActivationHeight)
   ```

2. Batches multiple signatures into single transactions to catch up efficiently

3. Continues until either:
   - Current height is reached
   - Provider is slashed/jailed
   - Critical error occurs

Fast sync can be:
- Disabled by setting `FastSyncInterval = 0`
- Configured via `FastSyncGap` to determine lag threshold
- Automatically triggered by periodic checks
