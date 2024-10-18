package main

import (
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/finality-provider/util"
)

func getHomePath(cmd *cobra.Command) (string, error) {
	rawHomePath, err := cmd.Flags().GetString(homeFlag)
	if err != nil {
		return "", err
	}

	homePath, err := filepath.Abs(rawHomePath)
	if err != nil {
		return "", err
	}
	// Create home directory
	homePath = util.CleanAndExpandPath(homePath)

	return homePath, nil
}

func searchInTxt(text, search string) string {
	idxOfRecovery := strings.Index(text, search)
	jsonKeyOutputOut := text[idxOfRecovery+len(search):]
	return strings.ReplaceAll(jsonKeyOutputOut, "\n", "")
}
