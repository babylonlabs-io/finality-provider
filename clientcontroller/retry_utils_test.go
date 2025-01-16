package clientcontroller

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpectedErr(t *testing.T) {
	t.Parallel()
	expectedErr := Expected(fmt.Errorf("some error"))
	require.True(t, IsExpected(expectedErr))
	wrappedErr := fmt.Errorf("expected: %w", expectedErr)
	require.True(t, IsExpected(wrappedErr))
}
