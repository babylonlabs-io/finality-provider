package version

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// AddVersionCommands adds all the version-related commands to the provided command.
// The version commands are generic to {Babylon, Cosmos BSN, rollup BSN} finality providers
func AddVersionCommands(cmd *cobra.Command, binaryName string) {
	cmd.AddCommand(CommandVersion(binaryName))
}

// CommandVersion prints cmd version
func CommandVersion(binaryName string) *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "version",
		Short:   "Prints version of this binary.",
		Aliases: []string{"v"},
		Example: fmt.Sprintf("%s version", binaryName),
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			v := Version()
			commit, ts := CommitInfo()

			if v == "" {
				v = "main"
			}

			var sb strings.Builder
			_, _ = sb.WriteString("Version:       " + v)
			_, _ = sb.WriteString("\n")
			_, _ = sb.WriteString("Git Commit:    " + commit)
			_, _ = sb.WriteString("\n")
			_, _ = sb.WriteString("Git Timestamp: " + ts)
			_, _ = sb.WriteString("\n")

			cmd.Printf(sb.String()) //nolint:govet // it's not an issue
		},
	}

	return cmd
}
