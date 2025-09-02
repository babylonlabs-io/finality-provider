package clientcontroller

import (
	"encoding/json"
	"testing"

	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	"github.com/stretchr/testify/require"
)

func TestCustomProofIndexField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		proof    cmtcrypto.Proof
		expected string
	}{
		{
			name: "index is 0 - should be included",
			proof: cmtcrypto.Proof{
				Total:    100,
				Index:    0, // This is the critical test case
				LeafHash: []byte("test_leaf"),
				Aunts:    [][]byte{[]byte("aunt1"), []byte("aunt2")},
			},
			expected: `{"total":100,"index":0,"leaf_hash":"dGVzdF9sZWFm","aunts":["YXVudDE=","YXVudDI="]}`,
		},
		{
			name: "index is non-zero - should be included",
			proof: cmtcrypto.Proof{
				Total:    100,
				Index:    42,
				LeafHash: []byte("test_leaf"),
				Aunts:    [][]byte{[]byte("aunt1")},
			},
			expected: `{"total":100,"index":42,"leaf_hash":"dGVzdF9sZWFm","aunts":["YXVudDE="]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test that our CustomProof always includes index
			customProof, err := ConvertProof(tt.proof)
			require.NoError(t, err)
			jsonBytes, err := json.Marshal(customProof)
			require.NoError(t, err)
			require.JSONEq(t, tt.expected, string(jsonBytes))

			// Verify that the JSON contains the index field
			require.Contains(t, string(jsonBytes), `"index":`)
		})
	}
}

func TestOriginalProofOmitsIndexWhenZero(t *testing.T) {
	t.Parallel()

	// This test demonstrates the original problem
	originalProof := cmtcrypto.Proof{
		Total:    100,
		Index:    0, // This will be omitted due to omitempty tag
		LeafHash: []byte("test_leaf"),
		Aunts:    [][]byte{[]byte("aunt1")},
	}

	jsonBytes, err := json.Marshal(originalProof)
	require.NoError(t, err)

	// The original proof should NOT contain "index" when it's 0
	require.NotContains(t, string(jsonBytes), `"index":`)

	// But it should contain other fields
	require.Contains(t, string(jsonBytes), `"total":100`)
	require.Contains(t, string(jsonBytes), `"leaf_hash":`)
}

func TestSubmitFinalitySignatureWithCustomProof(t *testing.T) {
	t.Parallel()

	// Test the complete SubmitFinalitySignature struct
	originalProof := cmtcrypto.Proof{
		Total:    50000,
		Index:    0, // Critical test case - index is 0
		LeafHash: []byte("block_hash"),
		Aunts:    [][]byte{[]byte("merkle_aunt")},
	}

	customProof, err := ConvertProof(originalProof)
	require.NoError(t, err)

	submitMsg := SubmitFinalitySignature{
		FpPubkeyHex: "test_pubkey_hex",
		Height:      340000,
		PubRand:     []byte("public_randomness"),
		Proof:       customProof, // Use our fixed conversion
		BlockHash:   []byte("app_hash"),
		Signature:   []byte("finality_signature"),
	}

	execMsg := ExecMsg{
		SubmitFinalitySignature: &submitMsg,
	}

	// Marshal to JSON (this is what gets sent to the contract)
	jsonBytes, err := json.Marshal(execMsg)
	require.NoError(t, err)

	jsonStr := string(jsonBytes)

	// Verify the JSON contains the index field even though it's 0
	require.Contains(t, jsonStr, `"index":0`)

	// Verify other proof fields are present
	require.Contains(t, jsonStr, `"total":50000`)
	require.Contains(t, jsonStr, `"leaf_hash":`)
	require.Contains(t, jsonStr, `"aunts":`)

	// Verify the structure matches what the contract expects
	require.Contains(t, jsonStr, `"submit_finality_signature"`)
	require.Contains(t, jsonStr, `"proof"`)
}

func TestConvertProofFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    cmtcrypto.Proof
		expected CustomProof
	}{
		{
			name: "all fields populated",
			input: cmtcrypto.Proof{
				Total:    12345,
				Index:    67,
				LeafHash: []byte("leaf_data"),
				Aunts:    [][]byte{[]byte("aunt1"), []byte("aunt2")},
			},
			expected: CustomProof{
				Total:    12345,
				Index:    67,
				LeafHash: []byte("leaf_data"),
				Aunts:    [][]byte{[]byte("aunt1"), []byte("aunt2")},
			},
		},
		{
			name: "index is zero",
			input: cmtcrypto.Proof{
				Total:    100,
				Index:    0,
				LeafHash: []byte("leaf"),
				Aunts:    [][]byte{},
			},
			expected: CustomProof{
				Total:    100,
				Index:    0,
				LeafHash: []byte("leaf"),
				Aunts:    [][]byte{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := ConvertProof(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertProofValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       cmtcrypto.Proof
		shouldError bool
		errorMsg    string
	}{
		{
			name: "negative total should return error",
			input: cmtcrypto.Proof{
				Total:    -1,
				Index:    0,
				LeafHash: []byte("leaf"),
				Aunts:    [][]byte{},
			},
			shouldError: true,
			errorMsg:    "cmtProof.Total cannot be negative: -1",
		},
		{
			name: "negative index should return error",
			input: cmtcrypto.Proof{
				Total:    10,
				Index:    -1,
				LeafHash: []byte("leaf"),
				Aunts:    [][]byte{},
			},
			shouldError: true,
			errorMsg:    "cmtProof.Index cannot be negative: -1",
		},
		{
			name: "valid values should not return error",
			input: cmtcrypto.Proof{
				Total:    10,
				Index:    0,
				LeafHash: []byte("leaf"),
				Aunts:    [][]byte{},
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := ConvertProof(tt.input)

			if tt.shouldError {
				require.Error(t, err)
				require.Equal(t, tt.errorMsg, err.Error())
				require.Equal(t, CustomProof{}, result) // Should return zero value on error
			} else {
				require.NoError(t, err)
				require.NotEqual(t, CustomProof{}, result) // Should return valid result
			}
		})
	}
}
