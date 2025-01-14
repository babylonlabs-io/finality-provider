package util

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// FileExists reports whether the named file or directory exists.
// This function is taken from https://github.com/btcsuite/btcd
func FileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}

	return true
}

func MakeDirectory(dir string) error {
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		// Show a nicer error message if it's because a symlink
		// is linked to a directory that does not exist
		// (probably because it's not mounted).
		if e := new(os.PathError); errors.As(err, &e) && os.IsExist(err) {
			link, lerr := os.Readlink(e.Path)
			if lerr == nil {
				str := "is symlink %s -> %s mounted?"
				err = fmt.Errorf(str, e.Path, link)
			}
		}

		return fmt.Errorf("failed to create dir %s: %w", dir, err)
	}

	return nil
}

// CleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
// This function is taken from https://github.com/btcsuite/btcd
func CleanAndExpandPath(path string) string {
	if path == "" {
		return ""
	}

	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		var homeDir string
		u, err := user.Current()
		if err == nil {
			homeDir = u.HomeDir
		} else {
			homeDir = os.Getenv("HOME")
		}

		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows-style %VARIABLE%,
	// but the variables can still be expanded via POSIX-style $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
}

// GetSubCommand returns the command if it finds, otherwise it returns nil
func GetSubCommand(cmd *cobra.Command, commandName string) *cobra.Command {
	for _, c := range cmd.Commands() {
		if !strings.EqualFold(c.Name(), commandName) {
			continue
		}

		return c
	}

	return nil
}
