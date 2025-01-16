# Finality Votes Submission Specification

## Overview

Finality providers submit votes to finalize blocks on the consumer chain.
This document specifies the process of submitting finality votes.

## Submission Process

### Internal State

The finality provider maintains a persistent state variable `lastVotedHeight`
to track the most recent height where a finality vote was successfully submitted
This is to prevent voting for a previously voted height.

### Bootstrapping

To determine the initial processing height `startHeight`:

1. Query consumer chain for:
   - `lastFinalizedHeight` (defaults to `0` if no blocks are finalized)
   - `finalityActivationHeight`
   - `lastVotedHeightRemote` (defaults to `0` if no votes exist)

2. Synchronize local state:
   - Verify local `lastVotedHeight` >= `lastVotedHeightRemote`
   - If verification fails, update local state to match remote

3. Calculate starting height based on reward distribution policy:
   - If rewards are available for already finalized blocks:

     ```go
     startHeight = max(lastVotedHeight + 1, finalityActivationHeight)
     ```

   - If rewards are only for unfinalized blocks:

     ```go
     startHeight = max(lastFinalizedHeight + 1, finalityActivationHeight)
     ```

The choice between using `lastVotedHeight` or `lastFinalizedHeight` depends on
the consumer chain's reward distribution mechanism.
Use `lastVotedHeight` if the chain allows collecting rewards for already
finalized blocks. Otherwise, use `lastFinalizedHeight` to only process
unfinalized blocks.

### Normal Submission Loop

After the finality provider is bootstrap, it continuously monitors for
new blocks from a trusted full node of the consumer chain.
For each new block, it performs these validation checks:

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

To submit a finality vote, the finality provider needs to fill the
[MsgAddFinalitySig](https://github.com/babylonlabs-io/babylon/blob/e7ac8fdf888406b16727b9ffca1f2e48364e9f53/x/finality/types/tx.pb.go#L154):

1. Finality provider public key: the BTC PK of the finality provider that casts
   the vote in [BIP340 format](https://github.com/babylonlabs-io/babylon/blob/79615c6b057de041a9f4c1c4466ef212a0c678d6/types/btc_schnorr_pk.go#L14).
2. Block height: the height of the block that the vote is signed for.
3. Public randomness: the public randomness that is retrieved from the local,
   which is a [32-byte point](https://github.com/babylonlabs-io/babylon/blob/5f8af8ced17d24f3f0c6172293cd37fb3d055807/types/btc_schnorr_pub_rand.go#L12) over `secp256k1`.
4. Merkle proof: the merkle proof of the public randomness, which is generated
   when constructing the public randomness commit using the CometBFT's [merkle](https://github.com/cometbft/cometbft/tree/main/crypto/merkle)
   library.
5. Block hash: the hash bytes of the block that the vote is signed.
6. Finality signature: the [EOTS signature](https://github.com/babylonlabs-io/babylon/blob/067082b9d3dd8dbe775d5ada70cd60151fe0f577/types/btc_schnorr_eots.go#L11)
   that is [signed](https://github.com/babylonlabs-io/babylon/blob/f19de7d0fcc4ea786a070a700a03d2cde3f57b7f/crypto/eots/eots.go#L54)
   by the finality provider's private key and the corresponding private randomness.

The consumer chain verifies:

- The finality provider has voting power for the given height
- Randomness was pre-committed and BTC-timestamped
- EOTS signature validity
