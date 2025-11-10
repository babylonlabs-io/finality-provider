//nolint:revive
package util_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/finality-provider/util"
)

func TestHasDuplicateHeights(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		heights   []uint64
		expectDup bool
		dupHeight uint64
	}{
		{
			name:      "no duplicates",
			heights:   []uint64{1, 2, 3, 4, 5},
			expectDup: false,
			dupHeight: 0,
		},
		{
			name:      "duplicate at start",
			heights:   []uint64{1, 1, 3},
			expectDup: true,
			dupHeight: 1,
		},
		{
			name:      "duplicate in middle",
			heights:   []uint64{1, 2, 2, 3},
			expectDup: true,
			dupHeight: 2,
		},
		{
			name:      "duplicate at end",
			heights:   []uint64{1, 2, 3, 3},
			expectDup: true,
			dupHeight: 3,
		},
		{
			name:      "empty slice",
			heights:   []uint64{},
			expectDup: false,
			dupHeight: 0,
		},
		{
			name:      "single element",
			heights:   []uint64{42},
			expectDup: false,
			dupHeight: 0,
		},
		{
			name:      "two identical elements",
			heights:   []uint64{100, 100},
			expectDup: true,
			dupHeight: 100,
		},
		{
			name:      "non-consecutive unique heights",
			heights:   []uint64{10, 20, 30, 15, 25},
			expectDup: false,
			dupHeight: 0,
		},
		{
			name:      "large numbers no duplicates",
			heights:   []uint64{1000000, 2000000, 3000000},
			expectDup: false,
			dupHeight: 0,
		},
		{
			name:      "large numbers with duplicate",
			heights:   []uint64{1000000, 2000000, 1000000},
			expectDup: true,
			dupHeight: 1000000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hasDup, dupHeight := util.HasDuplicateHeights(tt.heights)
			require.Equal(t, tt.expectDup, hasDup)
			if tt.expectDup {
				require.Equal(t, tt.dupHeight, dupHeight)
			} else {
				require.Equal(t, uint64(0), dupHeight)
			}
		})
	}
}

func TestValidateNoDuplicateHeights(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		heights   []uint64
		expectErr bool
	}{
		{
			name:      "valid unique heights",
			heights:   []uint64{1, 2, 3, 4, 5},
			expectErr: false,
		},
		{
			name:      "invalid with duplicates",
			heights:   []uint64{1, 2, 2, 3},
			expectErr: true,
		},
		{
			name:      "empty is valid",
			heights:   []uint64{},
			expectErr: false,
		},
		{
			name:      "single element is valid",
			heights:   []uint64{1},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := util.ValidateNoDuplicateHeights(tt.heights)
			if tt.expectErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "duplicate height detected")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
