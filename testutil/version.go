package testutil

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

const modName = "github.com/babylonlabs-io/babylon"

// GetBabylonVersion returns babylond version from go.mod
func GetBabylonVersion() (string, error) {
	goModPath := filepath.Join("..", "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}

	// Parse the go.mod file
	modFile, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return "", err
	}

	for _, require := range modFile.Require {
		if require.Mod.Path == modName {
			return require.Mod.Version, nil
		}
	}

	return "", fmt.Errorf("module %s not found", modName)
}
