package testutil

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// GetBabylonVersion returns babylond version from go.mod
func GetBabylonVersion() (string, error) {
	// Get the current working directory
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up directories until we find go.mod
	var goModPath string
	dir := wd
	for {
		candidate := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			goModPath = candidate

			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}

	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}

	// Parse the go.mod file
	modFile, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return "", err
	}

	const modName = "github.com/babylonlabs-io/babylon"
	for _, require := range modFile.Require {
		if require.Mod.Path == modName {
			return require.Mod.Version, nil
		}
	}

	return "", fmt.Errorf("module %s not found", modName)
}
