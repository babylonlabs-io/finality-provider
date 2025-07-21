package types

import (
	"context"
)

// FinalitySignatureSubmitter defines the interface for processing and submitting finality signatures
// The implementation handles all retry logic internally
type FinalitySignatureSubmitter interface {
	// FilterBlocksForVoting processes a batch of blocks and returns ones that need voting
	// It checks voting power, updates finality provider status, and filters out blocks
	// that are below the last voted height or don't have voting power
	FilterBlocksForVoting(ctx context.Context, blocks []BlockDescription) ([]BlockDescription, error)

	// SubmitBatchFinalitySignatures submits finality signatures for a batch of blocks
	// It handles retries, error handling, and finalization checks internally
	// Returns the transaction response or nil if no submission was needed
	SubmitBatchFinalitySignatures(ctx context.Context, blocks []BlockDescription) (*TxResponse, error)

	// SetState sets the state store of the finality signature submitter
	SetState(state FinalityProviderState)
}
