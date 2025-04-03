package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

// These variables are set during build time via -ldflags
var (
	version   = "n/a"
	gitCommit = "n/a"
)

// CreateVersionCommand creates and returns the version command
func CreateVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Version: %s\n", version)
			fmt.Printf("Git commit: %s\n", gitCommit)
		},
	}
}
