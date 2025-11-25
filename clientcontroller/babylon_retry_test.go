package clientcontroller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sdkErr "cosmossdk.io/errors"
	"github.com/babylonlabs-io/babylon/client/babylonclient"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

// mockMsg implements sdk.Msg for testing
type mockMsg struct {
	id string
}

func (m *mockMsg) Reset()                       {}
func (m *mockMsg) String() string               { return m.id }
func (m *mockMsg) ProtoMessage()                {}
func (m *mockMsg) ValidateBasic() error         { return nil }
func (m *mockMsg) GetSigners() []sdk.AccAddress { return nil }

// mockBabylonClient is a mock implementation for testing
type mockBabylonClient struct {
	callCount int
	// errors to return for each call (indexed by callCount)
	errors []error
	// track which messages were sent on each call
	sentMsgs [][]sdk.Msg
}

func (m *mockBabylonClient) ReliablySendMsgs(
	_ context.Context,
	msgs []sdk.Msg,
	_ []*sdkErr.Error,
	_ []*sdkErr.Error,
) (*babylonclient.RelayerTxResponse, error) {
	// Record the messages sent
	msgsCopy := make([]sdk.Msg, len(msgs))
	copy(msgsCopy, msgs)
	m.sentMsgs = append(m.sentMsgs, msgsCopy)

	if m.callCount < len(m.errors) {
		err := m.errors[m.callCount]
		m.callCount++
		if err != nil {
			return nil, err
		}

		return &babylonclient.RelayerTxResponse{
			TxHash: "mock-tx-hash",
		}, nil
	}

	m.callCount++

	return &babylonclient.RelayerTxResponse{
		TxHash: "mock-tx-hash",
	}, nil
}

// Helper function to create test messages
func createTestMsgs(ids ...string) []sdk.Msg {
	msgs := make([]sdk.Msg, len(ids))
	for i, id := range ids {
		msgs[i] = &mockMsg{id: id}
	}

	return msgs
}

// Helper to create error with message index
func createMsgIndexError(index int, errMsg string) error {
	return fmt.Errorf("failed to execute message; message index: %d: %s", index, errMsg)
}

func TestReliablySendMsgsResendingOnMsgErr_Success(t *testing.T) {
	t.Parallel()

	mockClient := &mockBabylonClient{
		errors: []error{nil}, // Success on first try
	}

	bc := &BabylonController{
		testClient: mockClient,
	}

	msgs := createTestMsgs("msg1", "msg2", "msg3")
	expectedErrs := []*sdkErr.Error{}
	unrecoverableErrs := []*sdkErr.Error{}

	resp, err := bc.reliablySendMsgsResendingOnMsgErr(
		context.Background(),
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "mock-tx-hash", resp.TxHash)
	require.Equal(t, 1, mockClient.callCount)
}

func TestReliablySendMsgsResendingOnMsgErr_SingleExpectedError(t *testing.T) {
	t.Parallel()

	duplicateVoteErr := sdkErr.Register("finality", 1, "duplicated finality signature")

	mockClient := &mockBabylonClient{
		errors: []error{
			// First call fails with duplicated vote for message index 1
			createMsgIndexError(1, "duplicated finality signature"),
			// Second call succeeds
			nil,
		},
	}

	bc := &BabylonController{
		testClient: mockClient,
	}

	msgs := createTestMsgs("msg0", "msg1", "msg2")
	expectedErrs := []*sdkErr.Error{duplicateVoteErr}
	unrecoverableErrs := []*sdkErr.Error{}

	resp, err := bc.reliablySendMsgsResendingOnMsgErr(
		context.Background(),
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 2, mockClient.callCount, "should have made 2 calls")

	// Verify first call had 3 messages
	require.Len(t, mockClient.sentMsgs[0], 3)
	// Verify second call had 2 messages (message at index 1 removed)
	require.Len(t, mockClient.sentMsgs[1], 2)
	msg0, ok := mockClient.sentMsgs[1][0].(*mockMsg)
	require.True(t, ok)
	require.Equal(t, "msg0", msg0.id)
	msg2, ok := mockClient.sentMsgs[1][1].(*mockMsg)
	require.True(t, ok)
	require.Equal(t, "msg2", msg2.id)
}

func TestReliablySendMsgsResendingOnMsgErr_MultipleExpectedErrors(t *testing.T) {
	t.Parallel()

	duplicateVoteErr := sdkErr.Register("finality", 2, "duplicated finality signature")

	mockClient := &mockBabylonClient{
		errors: []error{
			// First call: message 2 fails
			createMsgIndexError(2, "duplicated finality signature"),
			// Second call: message 0 fails (was originally message 0, message 2 removed)
			createMsgIndexError(0, "duplicated finality signature"),
			// Third call succeeds
			nil,
		},
	}

	bc := &BabylonController{
		testClient: mockClient,
	}

	msgs := createTestMsgs("msg0", "msg1", "msg2", "msg3")
	expectedErrs := []*sdkErr.Error{duplicateVoteErr}
	unrecoverableErrs := []*sdkErr.Error{}

	resp, err := bc.reliablySendMsgsResendingOnMsgErr(
		context.Background(),
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 3, mockClient.callCount)

	// Verify message removal sequence
	require.Len(t, mockClient.sentMsgs[0], 4) // Original 4 messages
	require.Len(t, mockClient.sentMsgs[1], 3) // msg2 removed
	require.Len(t, mockClient.sentMsgs[2], 2) // msg0 removed
}

func TestReliablySendMsgsResendingOnMsgErr_UnexpectedError(t *testing.T) {
	t.Parallel()

	duplicateVoteErr := sdkErr.Register("finality", 3, "duplicated finality signature")

	mockClient := &mockBabylonClient{
		errors: []error{
			// Unexpected error (not in expectedErrs)
			createMsgIndexError(1, "insufficient fee"),
		},
	}

	bc := &BabylonController{
		testClient: mockClient,
	}

	msgs := createTestMsgs("msg0", "msg1", "msg2")
	expectedErrs := []*sdkErr.Error{duplicateVoteErr}
	unrecoverableErrs := []*sdkErr.Error{}

	resp, err := bc.reliablySendMsgsResendingOnMsgErr(
		context.Background(),
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)

	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "insufficient fee")
	require.Equal(t, 1, mockClient.callCount, "should stop on first unexpected error")
}

func TestReliablySendMsgsResendingOnMsgErr_ExpectedThenUnexpectedError(t *testing.T) {
	t.Parallel()

	duplicateVoteErr := sdkErr.Register("finality", 4, "duplicated finality signature")

	mockClient := &mockBabylonClient{
		errors: []error{
			// First call: expected error
			createMsgIndexError(1, "duplicated finality signature"),
			// Second call: unexpected error
			createMsgIndexError(0, "insufficient fee"),
		},
	}

	bc := &BabylonController{
		testClient: mockClient,
	}

	msgs := createTestMsgs("msg0", "msg1", "msg2")
	expectedErrs := []*sdkErr.Error{duplicateVoteErr}
	unrecoverableErrs := []*sdkErr.Error{}

	resp, err := bc.reliablySendMsgsResendingOnMsgErr(
		context.Background(),
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)

	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "insufficient fee")
	require.Equal(t, 2, mockClient.callCount)

	// This test validates that we check errSendMsg (not accumulated err)
	// If we were checking accumulated err, it would incorrectly pass because
	// the accumulated error contains "duplicated finality signature" from iteration 1
}

func TestReliablySendMsgsResendingOnMsgErr_AllMessagesRemoved(t *testing.T) {
	t.Parallel()

	duplicateVoteErr := sdkErr.Register("finality", 5, "duplicated finality signature")

	mockClient := &mockBabylonClient{
		errors: []error{
			createMsgIndexError(0, "duplicated finality signature"),
			createMsgIndexError(0, "duplicated finality signature"),
			createMsgIndexError(0, "duplicated finality signature"),
		},
	}

	bc := &BabylonController{
		testClient: mockClient,
	}

	msgs := createTestMsgs("msg0", "msg1", "msg2")
	expectedErrs := []*sdkErr.Error{duplicateVoteErr}
	unrecoverableErrs := []*sdkErr.Error{}

	resp, err := bc.reliablySendMsgsResendingOnMsgErr(
		context.Background(),
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)

	// When all messages are removed due to expected errors, should return success
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, 3, mockClient.callCount)
}

func TestReliablySendMsgsResendingOnMsgErr_ErrorWithoutMessageIndex(t *testing.T) {
	t.Parallel()

	duplicateVoteErr := sdkErr.Register("finality", 6, "duplicated finality signature")

	mockClient := &mockBabylonClient{
		errors: []error{
			// Error without message index
			errors.New("some generic error without message index"),
		},
	}

	bc := &BabylonController{
		testClient: mockClient,
	}

	msgs := createTestMsgs("msg0", "msg1", "msg2")
	expectedErrs := []*sdkErr.Error{duplicateVoteErr}
	unrecoverableErrs := []*sdkErr.Error{}

	resp, err := bc.reliablySendMsgsResendingOnMsgErr(
		context.Background(),
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)

	// Should return error immediately as it doesn't contain "message index:"
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, 1, mockClient.callCount)
}

func TestReliablySendMsgsResendingOnMsgErr_MaxRetriesReached(t *testing.T) {
	t.Parallel()

	duplicateVoteErr := sdkErr.Register("finality", 7, "duplicated finality signature")

	// Create more errors than max retries (BatchRetries returns len(msgs))
	mockClient := &mockBabylonClient{
		errors: []error{
			createMsgIndexError(0, "duplicated finality signature"),
			createMsgIndexError(0, "duplicated finality signature"),
			createMsgIndexError(0, "duplicated finality signature"),
			createMsgIndexError(0, "duplicated finality signature"), // Extra, shouldn't reach
		},
	}

	bc := &BabylonController{
		testClient: mockClient,
	}

	msgs := createTestMsgs("msg0", "msg1", "msg2")
	expectedErrs := []*sdkErr.Error{duplicateVoteErr}
	unrecoverableErrs := []*sdkErr.Error{}

	resp, err := bc.reliablySendMsgsResendingOnMsgErr(
		context.Background(),
		msgs,
		expectedErrs,
		unrecoverableErrs,
	)

	// All errors were expected, so should return success
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Should have stopped at maxRetries = min(len(msgs), 10) = 3
	require.Equal(t, 3, mockClient.callCount)
}
