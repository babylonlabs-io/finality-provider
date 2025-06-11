# Finality provider specification for Consumer chains and rollups

- [Changelog](#changelog)
- [Abstract](#abstract)
- [Background](#background)
  - [BTC staking integration](#btc-staking-integration)
  - [The role of the finality provider](#the-role-of-the-finality-provider)
- [Keywords](#keywords)
- [Types of Integration](#types-of-integration)
- [Specification](#specification)
- [Expected behavior of the Finality Provider Adapter](#expected-behavior-of-the-finality-provider-adapter)
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

The finality provider is a fundamental component in the architecture of Babylon's
Bitcoin staking protocol. It is responsible for providing finality signatures
for blocks in the Babylon chain and other BSNs, based on the BTC stake delegated
to it by BTC stakers. The finality provider's role includes:

1. Generating public randomness commitments for the BTC stake it holds.
2. Submitting finality signatures for blocks in the Babylon chain and other BSNs.

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
   respective chains.
   In this case, the finality provider program will interact with the Staking
   integration module. That is, by integrating the babylon module (i.e. the
   `babylon-sdk`) into their stack.
3. **Ethereum rollups**: Ethereum rollups, such as OP Stack and Arbitrum Orbit,
   can implement the finality provider program to generate public randomness
   commitments and submit finality signatures for blocks in their respective
   rollups. These commitments and signatures will be stored in the Babylon chain
   (i.e. using a variant of [`rollup-bsn-contracts`](https://github.com/babylonlabs-io/rollup-bsn-contracts).
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

```go
/*
    The following functions are used for submitting messages to the Consumer chain
*/
type ConsumerController interface {
    // MUST: Core messages
    // CommitPubRandList commits a list of EOTS public randomness to the consumer chain.
    // It returns tx hash and error
    CommitPubRandList(fpPk *btcec.PublicKey, startHeight uint64, numPubRand uint64, commitment []byte, sig *schnorr.Signature) (*types.TxResponse, error)

    // MUST: Core messages
    // SubmitFinalitySig submits the finality signature to the consumer chain
    SubmitFinalitySig(fpPk *btcec.PublicKey, block *types.BlockInfo, pubRand *btcec.FieldVal, proof []byte, sig *btcec.ModNScalar) (*types.TxResponse, error)

    // SHOULD: Convenience messages
    // SubmitBatchFinalitySigs submit a batch of finality signatures to the consumer chain
    SubmitBatchFinalitySigs(fpPk *btcec.PublicKey, blocks []*types.BlockInfo, pubRandList []*btcec.FieldVal, proofList [][]byte, sigs []*btcec.ModNScalar) (*types.TxResponse, error)

    // SHOULD: Core messages
    // UnjailFinalityProvider sends an unjail transaction to the consumer chain
    UnjailFinalityProvider(fpPk *btcec.PublicKey) (*types.TxResponse, error)
}

/*
    The following methods are queries to the consumer chain
*/
type ConsumerQueries interface {
    // MUST: Core finality queries
    // QueryFinalityProviderHasPower queries whether the finality provider has
    // voting power at a given height
    QueryFinalityProviderHasPower(fpPk *btcec.PublicKey, blockHeight uint64) (bool, error)

    // MUST: Core finality queries
    // QueryLatestFinalizedBlock returns the latest finalized block.
    // Note: nil will be returned if the finalized block does not exist
    QueryLatestFinalizedBlock() (*types.BlockInfo, error)

    // MUST: Core finality queries
    // QueryFinalityProviderHighestVotedHeight queries the highest voted height 
    // of the given finality provider
    QueryFinalityProviderHighestVotedHeight(fpPk *btcec.PublicKey) (uint64, error)

    // MUST: Core finality queries
    // QueryLastPublicRandCommit returns the last public randomness commitment
    QueryLastPublicRandCommit(fpPk *btcec.PublicKey) (*types.PubRandCommit, error)

    // MUST: Core finality queries
    // QueryBlock queries the block at the given height
    QueryBlock(height uint64) (*types.BlockInfo, error)

    // MUST: Core finality queries
    // QueryLatestBlockHeight queries the tip block height of the consumer chain
    QueryLatestBlockHeight() (uint64, error)

    // MUST: Core finality queries
    // QueryActivatedHeight returns the activated height of the consumer chain.
    // Error will be returned if the consumer chain has not been activated.
    // If the consumer chain wants to accept finality voting at any block
    // height, zero should be returned.
    QueryActivatedHeight() (uint64, error)

    // SHOULD: Convenience finality queries
    // QueryFinalityProviderSlashedOrJailed queries if the finality provider is
    // slashed or jailed.
    // Note: If the FP wants to get the information from the Consumer chain
    // directly, they should add this interface.
    QueryFinalityProviderSlashedOrJailed(fpPk *btcec.PublicKey) (slashed bool, jailed bool, err error)

    // SHOULD: Convenience finality queries
    // QueryIsBlockFinalized queries if the block at the given height is finalized
    QueryIsBlockFinalized(height uint64) (bool, error)

    // SHOULD: Convenience finality queries
    // QueryBlocks returns a list of blocks from startHeight to endHeight
    QueryBlocks(startHeight, endHeight uint64, limit uint32) ([]*types.BlockInfo, error)
}
```

### **Expected behavior of the Finality Provider Adapter:** 

The finality provider adapter is expected to implement the `ConsumerController`
and `ConsumerQueries` interfaces, which define the interaction with the Consumer
chain. The adapter should handle the following behaviors:

1. **Commit public randomness**: The adapter should be able to commit a list of
   public randomness values to the Consumer chain, which are used for validating
   finality signatures.
2. **Submit finality signatures**: The adapter should be able to submit
   finality signatures for blocks in the Consumer chain, which are used to
   finalize blocks and provide economic security.
3. **Unjail finality provider**: The adapter should be able to send an unjail
   transaction to the Consumer chain, allowing the finality provider to resume
   its operations after being jailed.
4. **Query finality provider status**: The adapter should be able to query the
   status of the finality provider, including whether it has voting power, if it
   is slashed or jailed, and its highest voted height.
5. **Query last finalized block**: The adapter should be able to query the
   last finalized block in the Consumer chain.
6. **Query last public randomness commitment**: The adapter should be able to
   query the last public randomness commitment made by the finality provider.
7. **Query blocks and block finality**: The adapter should be able to query
   blocks in the Consumer chain, including their finality status, and the latest
   block height.
8. **Query activated height**: The adapter should be able to query the
   activated height of the Consumer chain, which is used to determine when the
   finality provider can start voting on blocks.

## Implementation status

As of this writing, there are three finality provider adapter implementations:

1. **Babylon Finality Provider Adapter** - Under [clientcontroller/babylon](https://github.com/babylonlabs-io/finality-provider/tree/main/clientcontroller/babylon)
    This is the main implementation that integrates with the Babylon chain, and
    provides the finality provider functionality for Babylon itself.
2. **CosmWasm Finality Provider Adapter** - Under [clientcontroller/cosmwasm](https://github.com/babylonlabs-io/finality-provider/tree/main/clientcontroller/cosmwasm)
    This implementation is designed for Cosmos-based chains, allowing them to
    leverage the finality provider functionality by integrating with the Cosmos
    chain through CosmWasm smart contracts (the [`cosmos-bsn-contracts`](https://github.com/babylonlabs-io/cosmos-bsn-contracts)
    repository) and a thin integration layer (the [`babylon-sdk`](https://github.com/babylonlabs-io/babylon-sdk)
    repository).
3. **OP Stack L2 Finality Provider Adapter** - Under [clientcontroller/opstackl2](https://github.com/babylonlabs-io/finality-provider/tree/main/clientcontroller/opstackl2)
    This implementation is tailored for OP Stack rollups, specifically designed
    to work with the OP Finality Gadget, which is a finality provider contract
    that runs on Babylon (based on the [`rollup-bsn-contracts`](https://github.com/babylonlabs-io/rollup-bsn-contracts)
    repository), and complements the OP Stack architecture.

**Comparison**: While all of these implementations follow the general principles
outlined in this document, they target different architectures.
The OP Stack L2 is specifically designed for OP Stack chains, and leverages
CosmWasm for deployment in Babylon, whereas the CosmWasm Adapter is more
general-purpose, and can be used by any Cosmos-based chain that supports
CosmWasm smart contracts. In this case, the CosmWasm smart contracts run in
the respective Cosmos Consumer chain, and interact with Babylon through IBC
messages.

<!-- TODO: add other potential or existing finality provider adapters -->