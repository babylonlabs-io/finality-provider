package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

const modName = "github.com/babylonlabs-io/babylon/v4"

// GetBabylonVersion returns babylond version from go.mod
func GetBabylonVersion() (string, error) {
	// Get the current working directory
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
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
		return "", fmt.Errorf("failed to parse go.mod file: %w", err)
	}

	// Parse the go.mod file
	modFile, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return "", fmt.Errorf("failed to read go.mod file: %w", err)
	}

	for _, require := range modFile.Require {
		if require.Mod.Path == modName {
			return require.Mod.Version, nil
		}
	}

	return "", fmt.Errorf("module %s not found", modName)
}

// Helper function to get the entire Babylon commit hash
// corresponding to the current version used.
func GetBabylonCommitHash() (string, error) {
	version, err := GetBabylonVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get babylon version: %w", err)
	}

	return getFullCommit(modName, version)
}

// Struct to parse the .info JSON file
type ModuleInfo struct {
	Origin struct {
		Hash string `json:"Hash"`
	} `json:"Origin"`
}

// getFullCommit is a helper function to get the full commit hash
// of the specified module version
func getFullCommit(modName, version string) (string, error) {
	// Get GOMODCACHE location
	modCache := os.Getenv("GOMODCACHE")
	if modCache == "" {
		//nolint:noctx
		cmd := exec.Command("go", "env", "GOMODCACHE")
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to get GOMODCACHE: %w", err)
		}
		modCache = strings.TrimSpace(string(out))
	}

	// Construct path to the .info file
	infoFile := filepath.Join(modCache, "cache", "download", modName, "@v", version+".info")

	// Read the .info file
	data, err := os.ReadFile(infoFile)
	if err != nil {
		return "", fmt.Errorf("failed to read .info file: %w", err)
	}

	// Parse JSON to extract commit hash
	var modInfo ModuleInfo
	if err := json.Unmarshal(data, &modInfo); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	if modInfo.Origin.Hash == "" {
		return "", fmt.Errorf("commit hash not found")
	}

	return modInfo.Origin.Hash, nil
}
