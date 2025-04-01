package main

import (
	"github.com/solo-io/ambient-migration-estimator-snapshot/cmd/root"
)

// These variables are set during build time via -ldflags
var (
	binaryName string = "n/a"
	version    string = "n/a"
	buildTime  string = "n/a"
	gitCommit  string = "n/a"
	goVersion  string = "n/a"
)

func main() {
	// Set the version variables for the commands
	root.SetVersionInfo(binaryName, version, gitCommit, goVersion, buildTime)

	root.Execute()
}
