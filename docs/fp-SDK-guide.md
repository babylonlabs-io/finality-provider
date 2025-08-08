# Finality Provider SDK Developer Guide

## Table of Contents

1. [Introduction](#1-introduction)
2. [Core constructor](#2-core-constructor)
   1. [Using Built-in Implementations](#21-using-built-in-implementations)
   2. [Custom Usage](#22-custom-usage)
3. [Interfaces](#3-interfaces)
   1. [Communication Level Interfaces](#31-communication-level-interfaces)
      1. [ConsumerController Interface](#311-consumercontroller-interface)
   2. [Service Level Interfaces](#32-service-level-interfaces)
      1. [Block Polling Interface](#321-blockpolling-interface)
      2. [Randomness Committer Interface](#322-randomnesscommitter-interface)
      3. [Finality Signature Submitter Interface](#323-finalitysignaturesubmitter-interface)
      4. [Height Determiner Interface](#324-heightdeterminer-interface)
4. [Instantiation](#4-instantiation)
   1. [Using Built-in Implementations](#41-using-built-in-implementations)
   2. [Using Custom Implementations](#42-using-custom-implementations)
5. [Startup](#5-startup)

## 1. Introduction

The Finality Provider SDK enables Bitcoin Supercharged Networks (BSNs) by 
providing interfaces to implement that create a compatible Finality Provider 
for their chain. It provides finality guarantees for consumer chain blocks through EOTS 
(Extractable One-Time Signatures) and BTC staking.

The SDK offers a modular, interface-based architecture that allows teams to 
customise components for their specific blockchain implementations while 
maintaining compatibility with the core finality provider functionality. 
Whilst also handling block querying, randomness generation, finality 
signature generation, and data submission.

The SDK includes two reference implementations:
- **Cosmos chains** - Uses CosmWasm contracts deployed on consumer Cosmos chain for finality 
  coordination ([implementation](../bsn/cosmos/), [config](../bsn/cosmos/config/))
- **Rollup chains** - Uses CosmWasm contracts deployed on Babylon chain for finality 
  coordination ([implementation](../bsn/rollup/), [config](../bsn/rollup/config/))

Both implementations demonstrate the same extensible interface pattern, 
enabling BSNs to create custom implementations for any consumer chain type 
by implementing [interfaces](../clientcontroller/api/interface.go) rather 
than modifying core logic.

> **⚡ Important:** If you need a custom finality provider implementation for 
> your chain, simply implement the required interfaces. There's no need to
> modify the core logic—this extensible pattern allows full flexibility 
> across different consumer chain types.

## 2. Core constructor

The BSN SDK uses `service.NewFinalityProviderApp()` as the main entry point. It 
supports two usage modes:

### 2.1. Using Built-In Implementations
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

Both internally call [`service.NewFinalityProviderApp()`](../finality-provider/service/app.go)
with pre-configured implementations.

### 2.2. Custom Usage
Implement your own interfaces and call the core constructor directly. This
allows custom consumer chain types beyond Cosmos/Rollup:

```go
fpApp, err := service.NewFinalityProviderApp(
    config,              // *fpcfg.Config 
    babylonController,   // ccapi.BabylonController - Can be customised
    consumerController,  // ccapi.ConsumerController - Can be customised
    eotsManager,         // eotsmanager.EOTSManager - Can be customised
    blockPoller,         // types.BlockPoller - Can be customised
    randomnessCommitter, // types.RandomnessCommitter - Can be customised
    heightDeterminer,    // types.HeightDeterminer - Can be customised
    finalitySubmitter,   // types.FinalitySignatureSubmitter - Can be customised
    metrics,             // *metrics.FpMetrics - Metrics collection
    db,                  // kvdb.Backend - Database for state persistence
    logger,              // *zap.Logger - Structured logging
)
```
> **💡Example:** The rollup BSN implementation shows exactly how to create 
> and wire up all the required interfaces:
> [`NewRollupBSNFinalityProviderAppFromConfig`](../bsn/rollup/service/app.go). 
> Similarly, the Cosmos implementation demonstrates the pattern: 
> [`NewCosmosBSNFinalityProviderAppFromConfig`](../bsn/cosmos/service/app.go).

## 3. Interfaces

### 3.1. Communication Level Interfaces
#### 3.1.1. ConsumerController Interface

The communication layer interface for consumer chain integration. It composes three
sub-interfaces into a single contract for separation of concerns. This is the 
main interface BSNs implement to connect their consumer chain to Babylon's 
Bitcoin-secured finality system.

The one interface allows for a single integration rather than managing 3 
separate dependencies and handles all finality aspects (randomness, querying, 
voting).

```go
type ConsumerController interface {
    RandomnessCommitter    // Public randomness operations
    BlockQuerier[types.BlockDescription]  // Chain state queries
    FinalityOperator      // Finality signature operations
    Close() error         // Cleanup connections
}
```

The [`ConsumerController`](../clientcontroller/api/interface.go) interface 
definition can be found in the codebase.

**Sub-interfaces:**

```go
type RandomnessCommitter interface {
    GetFpRandCommitContext() string
    CommitPubRandList(ctx context.Context, req *CommitPubRandListRequest) (
        *types.TxResponse, error)
    QueryLastPubRandCommit(ctx context.Context, fpPk *btcec.PublicKey) (
        *types.PubRandCommit, error)
    // QueryPubRandCommitList returns a list of public randomness commitments 
    QueryPubRandCommitList(ctx context.Context, fpPk *btcec.PublicKey, startHeight uint64) (
        []types.PubRandCommit, error)
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

All sub-interfaces are also defined in [`clientcontroller/api/interface.go`](../clientcontroller/api/interface.go).

### 3.2. Service Level Interfaces
#### 3.2.1. BlockPolling Interface

The `BlockPoller` sits between the consumer chain and the finality providers,
monitors the consumer chain for new blocks, then provides those blocks to 
finality providers for voting. The generic type parameter `T` must implement 
the `BlockDescription` interface, enabling BSNs to attach chain-specific data 
while ensuring every block provides the required core methods. Additionally, 
every block must provide `MsgToSign()` which generates the exact message that 
will be signed by EOTS.

```go
type BlockDescription interface {
GetHeight() uint64
GetHash() []byte
IsFinalized() bool
MsgToSign(signCtx string) []byte // this is the message that will be signed by the eots signer
}

type BlockPoller[T BlockDescription] interface {
    // NextBlock returns the next block requiring finality vote
    // Blocks until a block is available or context is cancelled
    NextBlock(ctx context.Context) (T, error)
    
    // TryNextBlock non-blocking version that returns immediately
    // Returns (block, true) if available, (nil, false) if not
    TryNextBlock() (T, bool)
    
    // SetStartHeight configures the starting point for block polling
    // Allows resuming from a specific height after restarts
    SetStartHeight(ctx context.Context, height uint64) error
    
    // NextHeight returns the next height to poll for
    // Useful for tracking polling progress and debugging
    NextHeight() uint64
    
    // Stop gracefully stops the poller
    Stop() error
}
```

The [`BlockPoller`](../types/expected_block.go) interface definition can be found in the codebase.

#### 3.2.2. RandomnessCommitter Interface

The `RandomnessCommitter` interface separates the information on committing
randomness, abstracting both when and how to commit randomness, and allows
BSNs to implement custom commitment strategies e.g. commit every N blocks. 
This enables BSNs to customise commitment behavior based on their specific 
requirements.

For example, chains with higher gas costs might implement larger batch 
sizes to minimise transaction costs, while other chains might want to commit 
more frequently with smaller batches to reduce finality latency.

```go
type RandomnessCommitter interface {
    // ShouldCommit determines whether randomness should be committed
    // Returns commitment decision, start height, and any error
    // Enables custom strategies for commitment timing and batching
    ShouldCommit(ctx context.Context) (should bool, startHeight uint64, err error)
    
    // Commit performs the actual randomness commitment to the consumer chain
    // BSNs implement chain-specific transaction logic here
    Commit(ctx context.Context, startHeight uint64) (*TxResponse, error)
    
    // GetLastCommittedHeight retrieves the last successfully committed height
    // Used for recovery and ensuring commitment continuity after restarts
    GetLastCommittedHeight(ctx context.Context) (uint64, error)
    
    // GetPubRandProofList retrieves cryptographic proofs for committed randomness
    // Required for EOTS signature generation and slashing detection
    GetPubRandProofList(height uint64, numPubRand uint64) ([][]byte, error)
    
    // Init initializes the committer with finality provider identity
    // Associates the committer with a specific BTC key and chain
    Init(btcPk *types.BIP340PubKey, chainID []byte) error
}
```

The [`RandomnessCommitter`](../types/expected_rand_committer.go) interface
definition can be found in the codebase.

#### 3.2.3. FinalitySignatureSubmitter Interface

The `FinalitySignatureSubmitter` interface abstracts away the processes of 
batching and retry handling. This means that BSNs only 
need to implement transaction submission logic; complex retry handling, 
batching optimization, and finalisation checking are handled by the SDK. 
This ensures signatures are only submitted after corresponding randomness is
committed, maintaining slashing security. The 
function `InitState()` provides access to voting history and
EOTS key state, enabling proper signature generation and preventing
double-signing violations.

```go
type FinalitySignatureSubmitter interface {
    // SubmitBatchFinalitySignatures submits finality signatures for a batch of blocks
    // Handles retries, error recovery, and finalization checks internally
    // BSNs implement the chain-specific transaction submission logic
    SubmitBatchFinalitySignatures(ctx context.Context, blocks []BlockDescription) (*TxResponse, error)
    
    // InitState provides access to finality provider state for signature generation
    // Enables the submitter to track voting history and manage EOTS key usage
    InitState(state FinalityProviderState) error
}
```

The [`FinalitySignatureSubmitter`](../types/expected_finality_submitter.go) 
interface definition can be found in the codebase.

#### 3.2.4. HeightDeterminer Interface

The `HeightDeterminer` interface abstracts the bootstrap logic required 
when a finality provider starts or restarts, enabling BSNs to implement custom 
restart strategies while ensuring continuation of voting and preventing gaps in 
finality coverage. Different chains may have varying requirements for safe 
restart heights as well as deployment strategies which are affected by the start 
height. This separation allows different deployment scenarios to be handled 
appropriately.

```go
type HeightDeterminer interface {
    // DetermineStartHeight calculates the appropriate height to begin processing
    // Considers finality provider state, chain state, and safety requirements
    // Enables custom bootstrap strategies for different deployment scenarios
    DetermineStartHeight(ctx context.Context, btcPk *types.BIP340PubKey, 
        lastVotedHeight LastVotedHeightProvider) (uint64, error)
}
```

The [`HeightDeterminer`](../types/expected_bootstraper.go) interface definition can be found in the codebase.

Most BSNs can use `NewStartHeightDeterminer()` unless they have specific 
requirements for custom bootstrap logic.

## 4. Instantiation

### 4.1. Using Built-in Implementations

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

1. [Cosmos service implementation](../bsn/cosmos/service/)
2. [Cosmos config package](../bsn/cosmos/config/)

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

1. [Rollup service implementation](../bsn/rollup/service/)
2. [Rollup config package](../bsn/rollup/config/)

Configuration files define consumer chain endpoints, contract addresses, and
finality parameters.

### 4.2. Using Custom Implementations

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

## 5. Startup

Once you have a finality provider app instance, start it:

```go
// Start the finality provider app
err := fpApp.Start()
if err != nil {
    return fmt.Errorf("failed to start finality provider: %w", err)
}

// Optionally start a specific finality provider instance
if err := fpApp.StartFinalityProvider(ctx, fpPk); err != nil {
    return fmt.Errorf("failed to start finality provider: %w", err)
}
```

For handling registration and daemon commands (over gRPC), wrap the app with a gRPC server:

```go
fpServer := service.NewFinalityProviderServer(cfg, logger, fpApp, dbBackend)
if err := fpServer.RunUntilShutdown(ctx); err != nil {
    return fmt.Errorf("failed to run finality provider server: %w", err)
}
```

**Startup sequence:**
1. Connections to Babylon chain, consumer chain, and EOTS manager are established 
2. 4 background processes start: metrics updates, critical error monitoring, 
   registration handling, and unjailing requests
3. If finality provider specified, determines start height and launches two 
   main processes:
   - Randomness commitment loop - Pre-commits public randomness
   - Finality signature submission loop - Processes blocks and submits votes
4. gRPC server starts for external API access

Monitor logs for connection status and voting activity.