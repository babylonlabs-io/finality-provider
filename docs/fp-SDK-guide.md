# Finality Provider SDK Developer Guide

## Table of Contents

1. [Introduction](#introduction)
2. [Constructor Pattern](#constructor-pattern)
   - [Default Usage](#default-usage)
   - [Custom Usage](#custom-usage)
   - [Interface Responsibilities](#interface-responsibilities)
3. [Interfaces](#interfaces)
   - [ConsumerController Interface](#consumercontroller-interface)
   - [Block Polling Interface](#block-polling-interface)
   - [Randomness Committer Interface](#randomness-committer-interface)
   - [Finality Signature Submitter Interface](#finality-signature-submitter-interface)
   - [Height Determiner Interface](#height-determiner-interface)
4. [Instantiation](#instantiation)
   - [Using Built-in Implementations](#using-built-in-implementations)
   - [Using Custom Implementations](#using-custom-implementations)
5. [Startup](#startup)

## Introduction

The Finality Provider SDK enables Bitcoin Supercharged Networks (BSNs) to 
integrate with Babylon's Bitcoin-secured finality infrastructure as a flexible 
library. It provides finality guarantees for consumer chain blocks through 
EOTS (Extractable One-Time Signatures) and BTC staking.

The SDK offers a modular, interface-based architecture that allows teams to 
customise components for their specific blockchain implementations while 
maintaining compatibility with the core finality provider functionality.

The SDK includes two reference implementations:
- **Cosmos chains** - Uses CosmWasm smart contracts for finality coordination
- **Rollup chains** - Uses Ethereum-compatible smart contracts for finality 
  coordination

Both implementations demonstrate the same extensible interface pattern, 
enabling BSNs to create custom implementations for any consumer chain type 
by implementing well-defined interfaces rather than modifying core logic.

## Constructor Pattern

The BSN SDK uses `service.NewFinalityProviderApp()` as the core constructor
pattern. It supports two usage modes:

### Default Usage
Use built-in implementations with constructors. These create all
required interfaces internally with default implementations:

```go
// Cosmos chains - Uses CosmWasm client, Cosmos SDK queries
fpApp, err := cosmosservice.NewCosmosBSNFinalityProviderAppFromConfig(
    cfg, db, logger)

// Rollup chains - Uses Ethereum RPC client, JSON-RPC queries
fpApp, err := rollupservice.NewRollupBSNFinalityProviderAppFromConfig(
    cfg, db, logger)
```

Both internally call `service.NewFinalityProviderApp()` with pre-configured
implementations.

### Custom Usage
Implement your own interfaces and call the core constructor directly. This
allows custom consumer chain types beyond Cosmos/Rollup:

```go
fpApp, err := service.NewFinalityProviderApp(
    config,              // *fpcfg.Config 
    babylonController,   // ccapi.BabylonController 
    consumerController,  // ccapi.ConsumerController - Can be customised
    eotsManager,         // eotsmanager.EOTSManager 
    blockPoller,         // types.BlockPoller - Can be customised
    randomnessCommitter, // types.RandomnessCommitter - Can be customised
    heightDeterminer,    // types.HeightDeterminer 
    finalitySubmitter,   // types.FinalitySignatureSubmitter - Can be customised
    metrics,             // *metrics.FpMetrics - Metrics collection
    db,                  // kvdb.Backend - Database for state persistence
    logger,              // *zap.Logger - Structured logging
)
```

### Interface Responsibilities

- **`ccapi.ConsumerController`** - Primary interface. Handles block queries,
  finality signature submission, randomness commits to your consumer chain
- **`types.BlockPoller`** - Monitors consumer chain blocks. Emits channel of
  new blocks requiring finality votes based on your chain's finality rules
- **`types.RandomnessCommitter`** - Pre-commits EOTS public randomness.
  Required before finality signatures to enable slashing detection
- **`types.FinalitySignatureSubmitter`** - Submits batched EOTS finality
  signatures. Coordinates with randomness commits for proper EOTS verification
- **`types.HeightDeterminer`** - Calculates finality voting start height.
  Usually built-in implementation sufficient
- **`ccapi.BabylonController`** - Babylon chain operations (FP registration,
  BTC staking queries). Use built-in unless custom Babylon integration
- **`eotsmanager.EOTSManager`** - EOTS key management and signature generation.
  Use built-in unless custom HSM integration

## Interfaces

### ConsumerController Interface

The primary interface for consumer chain integration. Composes three
sub-interfaces for separation of concerns:

```go
type ConsumerController interface {
    RandomnessCommitter    // Public randomness operations
    BlockQuerier[types.BlockDescription]  // Chain state queries
    FinalityOperator      // Finality signature operations
    Close() error         // Cleanup connections
}
```

**Sub-interfaces:**

```go
type RandomnessCommitter interface {
    GetFpRandCommitContext() string
    CommitPubRandList(ctx context.Context, req *CommitPubRandListRequest) (
        *types.TxResponse, error)
    QueryLastPublicRandCommit(ctx context.Context, fpPk *btcec.PublicKey) (
        *types.PubRandCommit, error)
}

type BlockQuerier[T types.BlockDescription] interface {
    QueryLatestFinalizedBlock(ctx context.Context) (T, error)
    QueryBlock(ctx context.Context, height uint64) (T, error)
    QueryIsBlockFinalized(ctx context.Context, height uint64) (bool, error)
    QueryBlocks(ctx context.Context, req *QueryBlocksRequest) ([]T, error)
    QueryLatestBlock(ctx context.Context) (T, error)
    QueryFinalityActivationBlockHeight(ctx context.Context) (uint64, error)
}

type FinalityOperator interface {
    GetFpFinVoteContext() string
    SubmitBatchFinalitySigs(ctx context.Context, 
        req *SubmitBatchFinalitySigsRequest) (*types.TxResponse, error)
    UnjailFinalityProvider(ctx context.Context, fpPk *btcec.PublicKey) (
        *types.TxResponse, error)
    QueryFinalityProviderHasPower(ctx context.Context, 
        req *QueryFinalityProviderHasPowerRequest) (bool, error)
    QueryFinalityProviderStatus(ctx context.Context, fpPk *btcec.PublicKey) (
        *FinalityProviderStatusResponse, error)
    QueryFinalityProviderHighestVotedHeight(ctx context.Context, 
        fpPk *btcec.PublicKey) (uint64, error)
}
```

### Block Polling Interface

Monitors consumer chain for new blocks requiring finality votes. Emits blocks
via channel for processing:

```go
type BlockPoller[T types.BlockDescription] interface {
    Start(ctx context.Context) error    // Begin polling consumer chain
    Stop() error                        // Stop polling gracefully
    IsRunning() bool                    // Check polling status
    GetPolledBlocks() <-chan []T        // Channel of new blocks requiring votes
}
```

Implementation should respect consumer chain finality rules and voting
intervals.

### Randomness Committer Interface

Pre-commits EOTS public randomness for future blocks. Required for EOTS
signature verification and slashing detection:

```go
type RandomnessCommitter interface {
    Start(ctx context.Context) error    // Begin randomness commitment process
    Stop() error                        // Stop committing gracefully
    IsRunning() bool                    // Check commitment status
    CommitPubRand(ctx context.Context, fpPk *btcec.PublicKey) error  
                                        // Commit randomness for FP
}
```

Must commit randomness before corresponding finality signatures to maintain
EOTS security properties.

### Finality Signature Submitter Interface

Submits EOTS finality signatures for consumer chain blocks. Coordinates with
randomness committer for proper EOTS verification:

```go
type FinalitySignatureSubmitter interface {
    Start(ctx context.Context) error       // Begin signature submission process
    Stop() error                           // Stop submitting gracefully  
    IsRunning() bool                       // Check submission status
    SubmitFinalitySignature(ctx context.Context, fpPk *btcec.PublicKey, 
        blocks []types.BlockDescription) error
}
```

Must ensure corresponding public randomness was committed before submitting
signatures for the same blocks.

### Height Determiner Interface

Determines the appropriate starting height when a finality provider begins
processing blocks. Handles bootstrap logic for finality voting:

```go
type HeightDeterminer interface {
    // DetermineStartHeight calculates appropriate height to start processing
    DetermineStartHeight(ctx context.Context, btcPk *types.BIP340PubKey, 
        lastVotedHeight LastVotedHeightProvider) (uint64, error)
}
```

The built-in implementation considers:
- Finality activation height on consumer chain
- Last voted height from finality provider state  
- Latest finalized block height
- Configurable safety margins for bootstrap

Most implementations can use the built-in `NewStartHeightDeterminer()` which
handles standard bootstrap scenarios.

## Instantiation

### Using Built-in Implementations

For standard consumer chain types, use convenience constructors that handle all
interface creation:

**Cosmos chains:**
```go
import (
    cosmosservice "github.com/babylonlabs-io/finality-provider/bsn/cosmos/service"
    cosmosconfig "github.com/babylonlabs-io/finality-provider/bsn/cosmos/config"
)

cfg, err := cosmosconfig.LoadConfig(homePath)  // Load Cosmos BSN config
fpApp, err := cosmosservice.NewCosmosBSNFinalityProviderAppFromConfig(
    cfg, db, logger)
```

**Rollup chains:**
```go
import (
    rollupservice "github.com/babylonlabs-io/finality-provider/bsn/rollup/service"  
    rollupconfig "github.com/babylonlabs-io/finality-provider/bsn/rollup/config"
)

cfg, err := rollupconfig.LoadConfig(homePath)  // Load rollup BSN config
fpApp, err := rollupservice.NewRollupBSNFinalityProviderAppFromConfig(
    cfg, db, logger)
```

Configuration files define consumer chain endpoints, contract addresses, and
finality parameters.

### Using Custom Implementations

For custom consumer chain types, implement required interfaces and call core
constructor:

```go
import (
    "github.com/babylonlabs-io/finality-provider/finality-provider/service"
    "github.com/babylonlabs-io/finality-provider/clientcontroller/api"
)

// Implement consumer chain specific logic
type MyConsumerController struct {
    chainClient YourChainClient  // Your chain's RPC/API client
    contracts   ContractAddresses
}

func (c *MyConsumerController) CommitPubRandList(ctx context.Context, 
    req *api.CommitPubRandListRequest) (*types.TxResponse, error) {
    // Submit randomness commitment to your consumer chain
    return c.chainClient.SubmitTransaction(randomnessCommitTx)
}

func (c *MyConsumerController) SubmitBatchFinalitySigs(ctx context.Context, 
    req *api.SubmitBatchFinalitySigsRequest) (*types.TxResponse, error) {
    // Submit finality signatures to your consumer chain
    return c.chainClient.SubmitTransaction(finalityVoteTx)
}

// ... implement other required methods for your chain type

// Instantiate with custom implementations
consumerController := &MyConsumerController{
    chainClient: yourClient, contracts: addresses}
blockPoller := &MyBlockPoller{chainClient: yourClient}
randomnessCommitter := &MyRandomnessCommitter{controller: consumerController}
finalitySubmitter := &MyFinalitySubmitter{controller: consumerController}

fpApp, err := service.NewFinalityProviderApp(
    config,               // Common finality provider config
    babylonController,    // Built-in Babylon integration
    consumerController,   // Your consumer chain implementation
    eotsManager,         // Built-in EOTS key management
    blockPoller,         // Your block monitoring implementation
    randomnessCommitter, // Your randomness commitment implementation
    heightDeterminer,    // Built-in height calculation (usually sufficient)
    finalitySubmitter,   // Your finality signature implementation
    metrics, db, logger, // Standard components
)
```

## Startup

Start finality provider and manage lifecycle:

```go
// Start all services (connects to chains, starts background processes)
err := fpApp.Start()
if err != nil {
    return fmt.Errorf("failed to start finality provider: %w", err)
}

// Monitor status
if fpApp.IsRunning() {
    logger.Info("Finality provider is running")
}

// Graceful shutdown
err = fpApp.Stop()
```

**Startup sequence:**
1. Connect to Babylon chain - Establishes RPC/gRPC connections for BTC
   staking queries
2. Connect to consumer chain - `ConsumerController` establishes chain
   connections
3. Connect to EOTS manager - gRPC connection for signature generation
4. Start block polling - BlockPoller begins monitoring consumer
   chain
5. Start randomness committer - Begins pre-committing public randomness
6. Start finality submitter - Begins processing polled blocks for finality
   votes
7. Finality voting active - System processes blocks and submits finality
   signatures

Each service starts asynchronously. Monitor logs for connection status and
voting activity.