package service

import (
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/v3/types"
	rollupclient "github.com/babylonlabs-io/finality-provider/bsn/rollup/clientcontroller"
	service "github.com/babylonlabs-io/finality-provider/finality-provider/service"
	"github.com/babylonlabs-io/finality-provider/metrics"
	"github.com/babylonlabs-io/finality-provider/testutil/mocks"
	"github.com/babylonlabs-io/finality-provider/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zaptest"
)

// Test suite for RollupRandomnessCommitter ShouldCommit function
// These tests verify the complete decision logic by calling the actual function

//nolint:maintidx // This test function has high cyclomatic complexity due to comprehensive test cases
func TestRollupRandomnessCommitterShouldCommit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// Mock setup
		activationHeight    uint64
		currentHeight       uint64
		lastCommittedHeight uint64
		// Committer config
		interval          uint64
		numPubRand        uint32
		timestampingDelay int64
		// Expected results
		expectedShouldCommit bool
		expectedStartHeight  uint64
		description          string
	}{
		// === FIRST COMMIT SCENARIOS ===
		{
			name:                 "first_commit_aligned_tip",
			activationHeight:     100,
			currentHeight:        105, // 100 + 1*5, perfectly aligned
			lastCommittedHeight:  0,   // No previous commits
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  105,
			description:          "First commit with tip already aligned to interval",
		},
		{
			name:                 "first_commit_tip_needs_rounding",
			activationHeight:     100,
			currentHeight:        107, // Between 105 and 110
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  110, // Round up to next boundary
			description:          "First commit with tip needing alignment",
		},
		{
			name:                 "first_commit_before_activation",
			activationHeight:     100,
			currentHeight:        80, // Before activation
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  100, // Must start at activation
			description:          "First commit when tip is before activation",
		},
		{
			name:                 "first_commit_with_delay_aligned",
			activationHeight:     100,
			currentHeight:        105,
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    10,
			expectedShouldCommit: true,
			expectedStartHeight:  115, // 105 + 10 = 115, already aligned (100 + 3*5)
			description:          "First commit with timestamping delay, result aligned",
		},
		{
			name:                 "first_commit_with_delay_needs_rounding",
			activationHeight:     100,
			currentHeight:        103,
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    10,
			expectedShouldCommit: true,
			expectedStartHeight:  115, // 103 + 10 = 113, rounded up to 115 (100 + 3*5)
			description:          "First commit with timestamping delay, needs rounding",
		},

		// === SUFFICIENT COVERAGE SCENARIOS ===
		{
			name:                 "sufficient_coverage_no_commit",
			activationHeight:     100,
			currentHeight:        110,
			lastCommittedHeight:  130, // EndHeight = 130, covers voting up to 130
			interval:             5,
			numPubRand:           3, // Required: 110 + 3*5 = 125
			timestampingDelay:    0,
			expectedShouldCommit: false, // 130 >= 125
			expectedStartHeight:  0,     // Not applicable
			description:          "Don't commit when sufficient randomness exists",
		},
		{
			name:                 "exact_coverage_no_commit",
			activationHeight:     100,
			currentHeight:        110,
			lastCommittedHeight:  125, // Exactly covers required
			interval:             5,
			numPubRand:           3, // Required: 110 + 3*5 = 125
			timestampingDelay:    0,
			expectedShouldCommit: false, // 125 >= 125
			expectedStartHeight:  0,
			description:          "Don't commit when exact coverage exists",
		},

		// === GAP/CONTINUATION SCENARIOS ===
		{
			name:                 "small_gap_continue_from_tip",
			activationHeight:     100,
			currentHeight:        115,
			lastCommittedHeight:  110, // Small gap
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  115, // Continue from tip (already aligned)
			description:          "Continue when tip is ahead of last commit",
		},
		{
			name:                 "large_gap_continue_from_tip",
			activationHeight:     100,
			currentHeight:        200,
			lastCommittedHeight:  120, // Large gap
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  200, // Jump to current tip
			description:          "Handle large gaps by jumping to current tip",
		},
		{
			name:                 "tip_behind_but_need_more_randomness",
			activationHeight:     100,
			currentHeight:        110,
			lastCommittedHeight:  120, // Tip behind last commit
			interval:             5,
			numPubRand:           4, // Required: 110 + 4*5 = 130
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  125, // Continue from 120+1, aligned to 125
			description:          "Commit when tip behind but need more coverage",
		},

		// === EDGE CASES ===
		{
			name:                 "consecutive_generation",
			activationHeight:     100,
			currentHeight:        105,
			lastCommittedHeight:  0,
			interval:             1, // Every height
			numPubRand:           5,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  105,
			description:          "Handle consecutive generation (interval=1)",
		},
		{
			name:                 "large_interval",
			activationHeight:     1000,
			currentHeight:        1050,
			lastCommittedHeight:  0,
			interval:             100,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  1100, // 1000 + 1*100, rounded up from 1050
			description:          "Handle large intervals correctly",
		},
		{
			name:                 "minimum_randomness",
			activationHeight:     100,
			currentHeight:        105,
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           1, // Minimum
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  105,
			description:          "Handle minimum randomness requirement",
		},

		// === BOUNDARY EDGE CASES ===
		{
			name:                 "activation_height_exact",
			activationHeight:     100,
			currentHeight:        100, // Exactly at activation
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  100, // Should start exactly at activation
			description:          "Current height exactly at activation height",
		},
		{
			name:                 "activation_height_plus_one",
			activationHeight:     100,
			currentHeight:        101, // Just after activation
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  105, // Should align to next interval
			description:          "Current height just after activation",
		},
		{
			name:                 "large_timestamping_delay",
			activationHeight:     100,
			currentHeight:        110,
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    50, // Large delay
			expectedShouldCommit: true,
			expectedStartHeight:  160, // 110 + 50 = 160, aligned to 160 (100 + 12*5)
			description:          "Handle large timestamping delay",
		},
		{
			name:                 "zero_timestamping_delay",
			activationHeight:     100,
			currentHeight:        110,
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0, // Zero delay
			expectedShouldCommit: true,
			expectedStartHeight:  110, // 110 + 0 = 110, aligned to 110
			description:          "Handle zero timestamping delay",
		},

		// === COVERAGE BOUNDARY CASES ===
		{
			name:                 "coverage_just_below_required",
			activationHeight:     100,
			currentHeight:        110,
			lastCommittedHeight:  120, // Just below required (110 + 3*5 = 125), aligned to interval
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  125, // Continue from 120+1, aligned to 125
			description:          "Coverage just below required threshold",
		},
		{
			name:                 "coverage_exactly_required",
			activationHeight:     100,
			currentHeight:        110,
			lastCommittedHeight:  125, // Exactly covers required (110 + 3*5 = 125), aligned to interval
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: false, // Sufficient coverage
			expectedStartHeight:  0,
			description:          "Coverage exactly matches required threshold",
		},
		{
			name:                 "coverage_just_above_required",
			activationHeight:     100,
			currentHeight:        110,
			lastCommittedHeight:  130, // Just above required (110 + 3*5 = 125), aligned to interval
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: false, // Sufficient coverage
			expectedStartHeight:  0,
			description:          "Coverage just above required threshold",
		},

		// === INTERVAL ALIGNMENT EDGE CASES ===
		{
			name:                 "interval_alignment_edge_case",
			activationHeight:     100,
			currentHeight:        104, // Just before interval boundary
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  105, // Should round up to next boundary
			description:          "Current height just before interval boundary",
		},
		{
			name:                 "interval_alignment_exact",
			activationHeight:     100,
			currentHeight:        105, // Exactly on interval boundary
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  105, // Should use exact boundary
			description:          "Current height exactly on interval boundary",
		},
		{
			name:                 "interval_alignment_just_after",
			activationHeight:     100,
			currentHeight:        106, // Just after interval boundary
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  110, // Should round up to next boundary
			description:          "Current height just after interval boundary",
		},

		// === CONTINUATION SCENARIOS ===
		{
			name:                 "continue_from_last_commit_plus_one",
			activationHeight:     100,
			currentHeight:        120,
			lastCommittedHeight:  115, // Last commit at 115
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  120, // Continue from 115+1, aligned to 120
			description:          "Continue from last commit + 1, aligned to current tip",
		},
		{
			name:                 "continue_from_last_commit_plus_interval",
			activationHeight:     100,
			currentHeight:        125,
			lastCommittedHeight:  115, // Last commit at 115
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  125, // Continue from 115+1, aligned to 125 (not 120)
			description:          "Continue from last commit + interval",
		},

		// === LARGE NUMBERS EDGE CASES ===
		{
			name:                 "very_large_interval",
			activationHeight:     1000,
			currentHeight:        1050,
			lastCommittedHeight:  0,
			interval:             1000, // Very large interval
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  2000, // 1000 + 1*1000, aligned to 2000
			description:          "Handle very large intervals",
		},
		{
			name:                 "very_large_num_pub_rand",
			activationHeight:     100,
			currentHeight:        105,
			lastCommittedHeight:  0,
			interval:             5,
			numPubRand:           100, // Very large number
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  105,
			description:          "Handle very large numPubRand",
		},

		// === COMPLEX CONTINUATION SCENARIOS ===
		{
			name:                 "complex_continuation_with_gap",
			activationHeight:     100,
			currentHeight:        200,
			lastCommittedHeight:  150, // Gap between last commit and current tip
			interval:             5,
			numPubRand:           3,
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  200, // Jump to current tip due to large gap
			description:          "Complex continuation with large gap",
		},
		{
			name:                 "continuation_with_insufficient_coverage",
			activationHeight:     100,
			currentHeight:        110,
			lastCommittedHeight:  120, // Tip behind last commit
			interval:             5,
			numPubRand:           4, // Need more coverage
			timestampingDelay:    0,
			expectedShouldCommit: true,
			expectedStartHeight:  125, // Continue from 120+1, aligned to 125
			description:          "Continue when tip behind but need more coverage",
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			// Create mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockConsumerController := mocks.NewMockConsumerController(ctrl)

			// Setup mock expectations
			mockConsumerController.EXPECT().
				QueryLatestBlock(gomock.Any()).
				Return(types.NewBlockInfo(tt.currentHeight, []byte("mock-hash"), false), nil).
				AnyTimes()

			mockConsumerController.EXPECT().
				QueryFinalityActivationBlockHeight(gomock.Any()).
				Return(tt.activationHeight, nil).
				AnyTimes()

			// Mock the last committed public randomness query
			if tt.lastCommittedHeight > 0 {
				// Create a mock PubRandCommit with the expected end height
				mockPubRandCommit := &rollupclient.RollupPubRandCommit{
					StartHeight:  tt.lastCommittedHeight - uint64(tt.numPubRand-1)*tt.interval,
					NumPubRand:   uint64(tt.numPubRand),
					Interval:     tt.interval,
					BabylonEpoch: 1,
					Commitment:   []byte("mock-commitment"),
				}
				mockConsumerController.EXPECT().
					QueryLastPublicRandCommit(gomock.Any(), gomock.Any()).
					Return(mockPubRandCommit, nil).
					AnyTimes()
			} else {
				// No previous commits
				mockConsumerController.EXPECT().
					QueryLastPublicRandCommit(gomock.Any(), gomock.Any()).
					Return(nil, nil).
					AnyTimes()
			}

			// Create RollupRandomnessCommitter with proper setup
			cfg := &service.RandomnessCommitterConfig{
				NumPubRand:              tt.numPubRand,
				TimestampingDelayBlocks: tt.timestampingDelay,
				ChainID:                 []byte("test-chain"),
			}
			logger := zaptest.NewLogger(t)

			// Create a mock BTC public key for testing
			r := rand.New(rand.NewSource(42))
			_, btcPK, err := datagen.GenRandomBTCKeyPair(r)
			require.NoError(t, err)
			btcPk := bbntypes.NewBIP340PubKeyFromBTCPK(btcPK)

			// Create mock PubRandState
			mockPubRandState := &service.PubRandState{}

			// Create mock metrics
			mockMetrics := &metrics.FpMetrics{}

			rrc := &RollupRandomnessCommitter{
				DefaultRandomnessCommitter: &service.DefaultRandomnessCommitter{
					BtcPk:        btcPk,
					Cfg:          cfg,
					PubRandState: mockPubRandState,
					ConsumerCon:  mockConsumerController,
					Logger:       logger,
					Metrics:      mockMetrics,
				},
				interval: tt.interval,
			}

			// Call the ACTUAL ShouldCommit function
			shouldCommit, startHeight, err := rrc.ShouldCommit(ctx)

			// Verify the results
			require.NoError(t, err, tt.description)
			require.Equal(t, tt.expectedShouldCommit, shouldCommit, tt.description)
			if tt.expectedShouldCommit {
				require.Equal(t, tt.expectedStartHeight, startHeight, tt.description)
			}
		})
	}
}
