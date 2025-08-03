package types

import (
	"context"
)

// FinalitySignatureSubmitter defines the interface for processing and submitting finality signatures
// The implementation handles all retry logic internally
type FinalitySignatureSubmitter interface {

	// SubmitBatchFinalitySignatures submits finality signatures for a batch of blocks
	// It handles retries, error handling, and finalization checks internally
	// Returns the transaction response or nil if no submission was needed
	SubmitBatchFinalitySignatures(ctx context.Context, blocks []BlockDescription) (*TxResponse, error)

	// InitState sets the state store of the finality signature submitter
	InitState(state FinalityProviderState) error
}
