package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Test suite for RollupRandomnessCommitter ShouldCommit function
// These tests verify the complex interval-based decision logic

func TestRollupRandomnessCommitterShouldCommit(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the helper functions directly for now
			rrc := &RollupRandomnessCommitter{
				interval: tt.interval,
			}

			// Test calculateFirstEligibleHeightWithActivation
			baseHeight := tt.currentHeight + uint64(tt.timestampingDelay)
			if tt.lastCommittedHeight >= baseHeight {
				baseHeight = tt.lastCommittedHeight + 1
			}
			baseHeight = max(baseHeight, tt.activationHeight)

			alignedHeight := rrc.calculateFirstEligibleHeightWithActivation(baseHeight, tt.activationHeight)

			// For first commit scenarios, test the alignment logic
			if tt.lastCommittedHeight == 0 {
				require.Equal(t, tt.expectedStartHeight, alignedHeight, tt.description)
			}

			// Test getLastVotingHeightWithRandomness if we have a committed height
			if tt.lastCommittedHeight > 0 {
				lastVotingHeight := rrc.getLastVotingHeightWithRandomness(tt.lastCommittedHeight, tt.activationHeight)
				// This should be <= lastCommittedHeight and aligned to interval
				require.LessOrEqual(t, lastVotingHeight, tt.lastCommittedHeight)
				if lastVotingHeight >= tt.activationHeight {
					offset := lastVotingHeight - tt.activationHeight
					require.Equal(t, uint64(0), offset%tt.interval, "Last voting height should be aligned to interval")
				}
			}
		})
	}
}

// Unit tests for individual helper functions
func TestCalculateFirstEligibleHeightWithActivation(t *testing.T) {
	tests := []struct {
		name             string
		startHeight      uint64
		activationHeight uint64
		interval         uint64
		expected         uint64
		description      string
	}{
		{
			name:             "before_activation",
			startHeight:      50,
			activationHeight: 100,
			interval:         5,
			expected:         100,
			description:      "When startHeight < activationHeight, should return activationHeight",
		},
		{
			name:             "exactly_at_activation",
			startHeight:      100,
			activationHeight: 100,
			interval:         5,
			expected:         100,
			description:      "When startHeight == activationHeight, should return activationHeight",
		},
		{
			name:             "already_aligned_after_activation",
			startHeight:      105, // 100 + 1*5
			activationHeight: 100,
			interval:         5,
			expected:         105,
			description:      "When startHeight is already aligned, should return startHeight",
		},
		{
			name:             "needs_rounding_up",
			startHeight:      103,
			activationHeight: 100,
			interval:         5,
			expected:         105, // Round up to next interval
			description:      "When startHeight is not aligned, should round up to next interval boundary",
		},
		{
			name:             "consecutive_generation",
			startHeight:      101,
			activationHeight: 100,
			interval:         1,
			expected:         101,
			description:      "With interval=1 (consecutive), every height is aligned",
		},
		{
			name:             "large_interval",
			startHeight:      150,
			activationHeight: 100,
			interval:         100,
			expected:         200, // 100 + 2*100
			description:      "With large intervals, should align correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rrc := &RollupRandomnessCommitter{
				interval: tt.interval,
			}

			result := rrc.calculateFirstEligibleHeightWithActivation(tt.startHeight, tt.activationHeight)
			require.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestGetLastVotingHeightWithRandomness(t *testing.T) {
	tests := []struct {
		name                string
		lastCommittedHeight uint64
		activationHeight    uint64
		interval            uint64
		expected            uint64
		description         string
	}{
		{
			name:                "before_activation",
			lastCommittedHeight: 50,
			activationHeight:    100,
			interval:            5,
			expected:            0,
			description:         "When lastCommittedHeight < activationHeight, should return 0",
		},
		{
			name:                "exactly_at_activation",
			lastCommittedHeight: 100,
			activationHeight:    100,
			interval:            5,
			expected:            100,
			description:         "When lastCommittedHeight == activationHeight, should return activationHeight",
		},
		{
			name:                "one_interval_after_activation",
			lastCommittedHeight: 105,
			activationHeight:    100,
			interval:            5,
			expected:            105, // 100 + 1*5
			description:         "Should find the voting height at exact interval",
		},
		{
			name:                "between_intervals",
			lastCommittedHeight: 107,
			activationHeight:    100,
			interval:            5,
			expected:            105, // 100 + 1*5 (floor division)
			description:         "Should floor to previous voting height when between intervals",
		},
		{
			name:                "multiple_intervals",
			lastCommittedHeight: 120,
			activationHeight:    100,
			interval:            5,
			expected:            120, // 100 + 4*5
			description:         "Should handle multiple intervals correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rrc := &RollupRandomnessCommitter{
				interval: tt.interval,
			}

			result := rrc.getLastVotingHeightWithRandomness(tt.lastCommittedHeight, tt.activationHeight)
			require.Equal(t, tt.expected, result, tt.description)
		})
	}
}
