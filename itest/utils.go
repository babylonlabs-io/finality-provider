package e2etest

import (
	"os"
	"testing"
)

func tempDir(t *testing.T, pattern string) (string, error) {
	tempName, err := os.MkdirTemp(os.TempDir(), pattern)
	if err != nil {
		return "", err
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(tempName)
	})

	if err = os.Chmod(tempName, 0755); err != nil {
		return "", err
	}

	return tempName, nil
}
