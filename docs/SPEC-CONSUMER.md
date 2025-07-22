# Finality provider specification for Consumer chains and rollups

- [Changelog](#changelog)
- [Abstract](#abstract)
- [Background](#background)
  - [BTC staking integration](#btc-staking-integration)
  - [The role of the finality provider](#the-role-of-the-finality-provider)
- [Keywords](#keywords)
- [Types of Integration](#types-of-integration)
- [Specification](#specification)
  - [Consumer Controller Interface](#consumer-controller-interface)
  - [Randomness Committer Interface](#randomness-committer-interface)
  - [Block Querier Interface](#block-querier-interface)
  - [Finality Operator Interface](#finality-operator-interface)
  - [Request and Response Types](#request-and-response-types)
  - [Expected behavior of the Finality Provider Adapter](#expected-behavior-of-the-finality-provider-adapter)
  - [**Start Height Determination Logic:**](#start-height-determination-logic)
- [Implementation status](#implementation-status)

## Changelog

- 05-06-2025: Initial draft.

## Abstract

This document specifies the design and requirements of the interface for the
finality provider program, to interact and integrate with different blockchains,
such as Babylon itself, and Cosmos and Ethereum rollups, to provide finality
signatures based on Bitcoin staking.

The main purpose of this specification is to provide a standard interface for
integrators of Babylon's Bitcoin staking protocol. So that they can implement
only the part of the finality program that is specific to their blockchain or
rollup architecture, while reusing the common components that are shared across
all blockchain types.

## Background

### BTC staking integration

Babylon's phase-3 network introduces Bitcoin staking integration to provide
Bitcoin security to other decentralized systems, known as Bitcoin Supercharged
Networks (BSNs), such as L1 blockchains and rollups. This integration enables
BTC stakers to delegate their native BTC to finality providers on BSNs, and each
BSN will leverage this BTC stake for economic security. For more details, see
the [Cosmos integration
1-pager](https://www.notion.so/BTC-staking-integration-for-Cosmos-chains-1-pager-f0574cd4e624475eb00d64912698a38c?pvs=4)
and [OP Stack integration
1-pager](https://www.notion.so/BTC-staking-integration-for-OP-stack-chains-1-pager-16f28a013c55805fbebdec6102b43c92?pvs=4).

### The role of the finality provider

The finality provider is a fundamental component in the architecture of
Babylon's Bitcoin staking protocol. It is responsible for providing finality
signatures for blocks in the Babylon chain and other BSNs, based on the BTC
stake delegated to it by BTC stakers. The finality provider's role includes:

1. Generating public randomness commitments for the BTC stake it holds.
2. Submitting finality signatures for blocks in the Babylon chain and other
   BSNs.

## Keywords

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD",
"SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and "OPTIONAL" in this
document are to be interpreted as described in [RFC
2119](https://www.ietf.org/rfc/rfc2119.html) and [RFC
8174](https://www.ietf.org/rfc/rfc8174.html).

## Types of Integration

We can distinguish between three main types of integration with the finality
provider program:

1. **Babylon chain**: The Babylon chain itself implements the finality provider
   program, which is responsible for generating public randomness commitments
   and submitting finality signatures for blocks in the Babylon chain.
2. **Cosmos-based chains**: Cosmos-based chains, such as Osmosis, Neutron, and
   others, can implement the finality provider program to generate public
   randomness commitments and submit finality signatures for blocks in their
   respective chains. In this case, the finality provider program will interact
   with the Staking integration module. That is, by integrating the babylon
   module (i.e. the `babylon-sdk`) into their stack.
3. **Ethereum rollups**: Ethereum rollups, such as OP Stack and Arbitrum Orbit,
   can implement the finality provider program to generate public randomness
   commitments and submit finality signatures for blocks in their respective
   rollups. These commitments and signatures will be stored in the Babylon chain
   through rollup finality contracts (implemented using variants of
   [`rollup-bsn-contracts`](https://github.com/babylonlabs-io/rollup-bsn-contracts).
   See `SPEC.md` in that repository for details), and used to provide finality
   for the rollup blocks.

## Specification

What we define here is a standard interface that the finality provider program
must implement, which defines the interaction with Babylon-integrated chains and
rollups.

In this way, the finality provider program can be reused across different
blockchains, while allowing each chain or rollup to implement only the specific
components that are relevant to its architecture.

The specification distinguishes between required ("MUST") and recommended
("SHOULD") components.

**Block Abstraction:** The interfaces use `types.BlockDescription` as an
abstraction for blocks, allowing different consumer chains to implement their
own block representations while maintaining a consistent interface. This
abstraction supports:

- Different block formats across chain types (Cosmos, Ethereum rollups, etc.)
- Consistent block processing logic in the finality provider core
- Extensibility for future blockchain architectures

```go
/*
    The following functions are used for submitting messages to the Consumer chain
*/
type ConsumerController interface {
    // MUST: Core messages
    // CommitPubRandList commits a list of EOTS public randomness to the consumer chain.
    // It returns tx hash and error
    CommitPubRandList(ctx context.Context, req *CommitPubRandListRequest) (*types.TxResponse, error)

    // SubmitBatchFinalitySigs submit a batch of finality signatures to the consumer chain
    SubmitBatchFinalitySigs(ctx context.Context, req *SubmitBatchFinalitySigsRequest) (*types.TxResponse, error)

    // Close closes the connection to the consumer chain
    Close() error

    // SHOULD: Core messages
    // UnjailFinalityProvider sends an unjail transaction to the consumer chain
    UnjailFinalityProvider(ctx context.Context, fpPk *btcec.PublicKey) (*types.TxResponse, error)
}

/*
    The following methods are queries to the consumer chain
*/
type ConsumerQueries interface {
    // MUST: Core finality queries
    // QueryFinalityProviderHasPower queries whether the finality provider has
    // voting power at a given height
    QueryFinalityProviderHasPower(ctx context.Context, req *QueryFinalityProviderHasPowerRequest) (bool, error)

    // QueryLatestFinalizedBlock returns the latest finalized block.
    // Note: nil will be returned if the finalized block does not exist
    QueryLatestFinalizedBlock(ctx context.Context) (types.BlockDescription, error)

    // QueryFinalityProviderHighestVotedHeight queries the highest voted height 
    // of the given finality provider
    QueryFinalityProviderHighestVotedHeight(ctx context.Context, fpPk *btcec.PublicKey) (uint64, error)

    // QueryLastPublicRandCommit returns the last public randomness commitment
    QueryLastPublicRandCommit(ctx context.Context, fpPk *btcec.PublicKey) (*types.PubRandCommit, error)

    // QueryBlock queries the block at the given height
    QueryBlock(ctx context.Context, height uint64) (types.BlockDescription, error)

    // QueryLatestBlockHeight queries the tip block height of the consumer chain
    QueryLatestBlockHeight(ctx context.Context) (uint64, error)

    // QueryFinalityActivationBlockHeight returns the finality activation height of the consumer chain.
    // This is the minimum height from which the finality provider can start voting.
    // Error will be returned if the consumer chain has not been activated.
    // If the consumer chain wants to accept finality voting at any block
    // height, zero should be returned.
    QueryFinalityActivationBlockHeight(ctx context.Context) (uint64, error)

    // SHOULD: Convenience finality queries
    // QueryFinalityProviderStatus queries the status of the finality provider
    QueryFinalityProviderStatus(ctx context.Context, fpPk *btcec.PublicKey) (*FinalityProviderStatusResponse, error)

    // QueryIsBlockFinalized queries if the block at the given height is finalized
    QueryIsBlockFinalized(ctx context.Context, height uint64) (bool, error)

    // QueryBlocks returns a list of blocks from startHeight to endHeight
    QueryBlocks(ctx context.Context, req *QueryBlocksRequest) ([]types.BlockDescription, error)
}
```

### Consumer Controller Interface

The main interface that consumer chains must implement. It combines three sub-interfaces:

```go
type ConsumerController interface {
    RandomnessCommitter
    BlockQuerier[types.BlockDescription]
    FinalityOperator

    Close() error
}
```

### Randomness Committer Interface

Handles public randomness commitment operations:

```go
type RandomnessCommitter interface {
    // MUST: Core randomness commitment
    // GetFpRandCommitContext returns the signing context for public randomness commitment
    GetFpRandCommitContext() string

    // MUST: Core randomness commitment
    // CommitPubRandList commits a list of EOTS public randomness to the consumer chain
    CommitPubRandList(ctx context.Context, req *CommitPubRandListRequest) (*types.TxResponse, error)

    // MUST: Core randomness commitment
    // QueryLastPublicRandCommit returns the last public randomness commitment
    QueryLastPublicRandCommit(ctx context.Context, fpPk *btcec.PublicKey) (*types.PubRandCommit, error)
}
```

### Block Querier Interface

Handles block-related queries (generic over BlockDescription):

```go
type BlockQuerier[T types.BlockDescription] interface {
    // MUST: Core block queries
    // QueryLatestFinalizedBlock returns the latest finalized block
    QueryLatestFinalizedBlock(ctx context.Context) (T, error)

    // MUST: Core block queries
    // QueryBlock queries the block at the given height
    QueryBlock(ctx context.Context, height uint64) (T, error)

    // MUST: Core block queries
    // QueryLatestBlock queries the tip block of the consumer chain
    QueryLatestBlock(ctx context.Context) (T, error)

    // MUST: Core block queries
    // QueryActivatedHeight returns the activated height of the consumer chain
    QueryActivatedHeight(ctx context.Context) (uint64, error)

    // MUST: Core block queries
    // QueryFinalityActivationBlockHeight returns the block height when finality voting starts
    QueryFinalityActivationBlockHeight(ctx context.Context) (uint64, error)

    // SHOULD: Convenience block queries
    // QueryIsBlockFinalized queries if the block at the given height is finalized
    QueryIsBlockFinalized(ctx context.Context, height uint64) (bool, error)

    // SHOULD: Convenience block queries
    // QueryBlocks returns a list of blocks from startHeight to endHeight
    QueryBlocks(ctx context.Context, req *QueryBlocksRequest) ([]T, error)
}
```

### Finality Operator Interface

Handles finality signature submission operations:

```go
type FinalityOperator interface {
    // MUST: Core finality operations
    // GetFpFinVoteContext returns the signing context for finality vote
    GetFpFinVoteContext() string

    // MUST: Core finality operations
    // SubmitBatchFinalitySigs submits a batch of finality signatures to the consumer chain
    SubmitBatchFinalitySigs(ctx context.Context, req *SubmitBatchFinalitySigsRequest) (*types.TxResponse, error)

    // MUST: Core finality operations
    // QueryFinalityProviderHasPower queries whether the finality provider has voting power at a given height
    QueryFinalityProviderHasPower(ctx context.Context, req *QueryFinalityProviderHasPowerRequest) (bool, error)

    // MUST: Core finality operations
    // QueryFinalityProviderHighestVotedHeight queries the highest voted height of the given finality provider
    QueryFinalityProviderHighestVotedHeight(ctx context.Context, fpPk *btcec.PublicKey) (uint64, error)

    // SHOULD: Convenience finality operations
    // UnjailFinalityProvider sends an unjail transaction to the consumer chain
    UnjailFinalityProvider(ctx context.Context, fpPk *btcec.PublicKey) (*types.TxResponse, error)

    // SHOULD: Convenience finality operations
    // QueryFinalityProviderStatus queries the finality provider status
    QueryFinalityProviderStatus(ctx context.Context, fpPk *btcec.PublicKey) (*FinalityProviderStatusResponse, error)
}
```

### Request and Response Types

```go
type CommitPubRandListRequest struct {
    FpPk        *btcec.PublicKey
    StartHeight uint64
    NumPubRand  uint64
    Commitment  []byte
    Sig         *schnorr.Signature
}

type SubmitBatchFinalitySigsRequest struct {
    FpPk        *btcec.PublicKey
    Blocks      []*types.BlockInfo
    PubRandList []*btcec.FieldVal
    ProofList   [][]byte
    Sigs        []*btcec.ModNScalar
}

type QueryBlocksRequest struct {
    StartHeight uint64
    EndHeight   uint64
    Limit       uint32
}

type QueryFinalityProviderHasPowerRequest struct {
    FpPk        *btcec.PublicKey
    BlockHeight uint64
}

type FinalityProviderStatusResponse struct {
    Slashed bool
    Jailed  bool
}
```

### Expected behavior of the Finality Provider Adapter

The finality provider adapter is expected to implement the `ConsumerController`
interface and its sub-interfaces, which define the interaction with the Consumer
chain. The adapter should handle the following behaviors:

1. **Commit public randomness**: The adapter should be able to commit a list of
   public randomness values to the Consumer chain, which are used for validating
   finality signatures.
2. **Submit finality signatures**: The adapter should be able to submit finality
   signatures for blocks in the Consumer chain, which are used to finalize
   blocks and provide economic security.
3. **Unjail finality provider**: The adapter should be able to send an unjail
   transaction to the Consumer chain, allowing the finality provider to resume
   its operations after being jailed.
4. **Query finality provider status**: The adapter should be able to query the
   status of the finality provider, including whether it has voting power, if it
   is slashed or jailed, and its highest voted height.
5. **Query last finalized block**: The adapter should be able to query the last
   finalized block in the Consumer chain.
6. **Query last public randomness commitment**: The adapter should be able to
   query the last public randomness commitment made by the finality provider.
7. **Query blocks and block finality**: The adapter should be able to query
   blocks in the Consumer chain, including their finality status, and the latest
   block height.
8. **Query finality activation height**: The adapter should be able to query the
   finality activation height of the Consumer chain, which is used to determine
   when the finality provider can start voting on blocks.

### **Start Height Determination Logic:**

The finality provider must implement a deterministic algorithm to calculate the
appropriate starting height for block processing. This logic ensures the
finality provider resumes from the correct position after restarts and respects
chain activation constraints.

**Algorithm:**
1. Query the following heights from the consumer chain:
   - `finalityActivationHeight`: The minimum height from which finality voting
     is allowed
   - `highestVotedHeight`: The highest block height this finality provider has
     voted on (from chain state)
   - `lastFinalizedHeight`: The height of the most recently finalized block
2. Retrieve the local `lastVotedHeight` from the finality provider's internal
   state
3. Calculate the start height as: `max(max(lastVotedHeight, highestVotedHeight,
   lastFinalizedHeight) + 1, finalityActivationHeight)`

**Rationale:**
- The finality provider must not vote below the finality activation height
- The finality provider must not re-vote on blocks it has already processed
- Starting from `lastFinalizedHeight + 1` ensures no finalized blocks are
  re-processed
- The `+ 1` ensures the next block to be processed (not the last processed
  block)

**Implementation Notes:**
- This logic must be implemented consistently across all consumer chain adapters
- The algorithm handles edge cases where heights may be uninitialized (typically
  0)
- Proper error handling is required when queries fail or return invalid heights

## Implementation status

As of this writing, there are three finality provider adapter implementations:

1. **Babylon Finality Provider Adapter** - Under
    [clientcontroller/babylon](https://github.com/babylonlabs-io/finality-provider/tree/main/clientcontroller/babylon)
    This is the main implementation that integrates with the Babylon chain, and
    provides the finality provider functionality for Babylon itself.
2. **CosmWasm Finality Provider Adapter** - Under
    [clientcontroller/cosmwasm](https://github.com/babylonlabs-io/finality-provider/tree/main/clientcontroller/cosmwasm)
    This implementation is designed for Cosmos-based chains, allowing them to
    leverage the finality provider functionality by integrating with the Cosmos
    chain through CosmWasm smart contracts (the
    [`cosmos-bsn-contracts`](https://github.com/babylonlabs-io/cosmos-bsn-contracts)
    repository) and a thin integration layer (the
    [`babylon-sdk`](https://github.com/babylonlabs-io/babylon-sdk) repository).
3. **Rollup BSN Finality Provider Adapter** - Under
    [bsn/rollup-finality-provider/clientcontroller](https://github.com/babylonlabs-io/finality-provider/tree/main/bsn/rollup-finality-provider/clientcontroller)
    This implementation is tailored for OP Stack rollups, specifically designed
    to work with rollup finality contracts, which are finality provider
    contracts that run on Babylon (based on the
    [`rollup-bsn-contracts`](https://github.com/babylonlabs-io/rollup-bsn-contracts)
    repository), and complements the OP Stack architecture.

<!-- TODO: add other potential or existing finality provider adapters -->