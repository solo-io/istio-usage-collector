package main

import (
	"github.com/solo-io/istio-usage-collector/cmd/root"
)

// These variables are set during build time via -ldflags
var (
	version   string = "n/a"
	gitCommit string = "n/a"
)

func main() {
	// Set the version variables for the commands
	root.SetVersionInfo(version, gitCommit)
	root.Execute()
}
