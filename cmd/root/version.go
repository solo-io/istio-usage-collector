package root

import (
	"fmt"

	"github.com/spf13/cobra"
)

// CreateVersionCommand creates and returns the version command
func CreateVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(binaryName)
			fmt.Printf("Version: %s\n", version)
			fmt.Printf("Git commit: %s\n", gitCommit)
			fmt.Printf("Go version: %s\n", goVersion)
			fmt.Printf("Build time: %s\n", buildTime)
		},
	}
}
