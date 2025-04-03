package version

import (
	"fmt"
)

// These variables are set during build time via -ldflags
var (
	version   = "n/a"
	gitCommit = "n/a"
)

// VersionTemplate returns the version template for the command, used to print the version information via the --version flag
func VersionTemplate() string {
	return fmt.Sprintf("Version: %s\nGit commit: %s\n", version, gitCommit)
}
