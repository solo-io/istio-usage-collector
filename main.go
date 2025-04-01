package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/solo-io/ambient-migration-estimator-snapshot/internal/gatherer"
	"github.com/solo-io/ambient-migration-estimator-snapshot/internal/logging"
	"github.com/solo-io/ambient-migration-estimator-snapshot/internal/utils"
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Run the signal handler in a goroutine
	go func() {
		sig := <-sigCh
		logging.Info("Received signal: %v, initiating shutdown...", sig)

		// Create a timeout context for shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		go func() {
			<-shutdownCtx.Done()
			if shutdownCtx.Err() == context.DeadlineExceeded {
				logging.Error("Shutdown timed out, forcing exit")
				os.Exit(1)
			}
		}()

		cancel()
	}()

	// flags
	hideNames := flag.Bool("hide-names", false, "Hide the names of the cluster and namespaces using a hash")
	help := flag.Bool("help", false, "Show help message")
	continueProcessing := flag.Bool("continue", false, "Continue processing from the last saved state if the script was interrupted")
	showVersion := flag.Bool("version", false, "Show version information")
	context := flag.String("context", "", "Kubernetes context to use (if not set, uses current context)")

	// short flag aliases
	flag.BoolVar(hideNames, "hn", false, "Hide the names of the cluster and namespaces using a hash")
	flag.BoolVar(help, "h", false, "Show help message")
	flag.BoolVar(continueProcessing, "c", false, "Continue processing from the last saved state if the script was interrupted")
	flag.BoolVar(showVersion, "v", false, "Show version information")
	flag.StringVar(context, "ctx", "", "Kubernetes context to use (if not set, uses current context)")

	flag.Parse()

	if *help {
		displayHelpMessage()
		return
	}
	if *showVersion {
		displayVersionInfo()
		return
	}

	// Get Kubernetes context from environment variable or use default
	kubeContext := *context
	if kubeContext == "" {
		var err error
		kubeContext, err = utils.GetCurrentContext()
		if err != nil {
			logging.Error("No current kubectl context found and CONTEXT environment variable not set: %v", err)
			os.Exit(1)
		}
		logging.Info("Using current context: %s", kubeContext)
	} else {
		logging.Info("Using Kubernetes context from environment: %s", kubeContext)
	}

	// Create config
	cfg := &utils.Config{
		KubeContext:        kubeContext,
		ObfuscateNames:     *hideNames,
		ContinueProcessing: *continueProcessing,
		OutputFile:         "cluster_info.json",
	}

	// Gather cluster information
	if err := gatherer.GatherClusterInfo(ctx, cfg); err != nil {
		logging.Error("Error gathering cluster information: %v", err)
		os.Exit(1)
	}

	logging.Info("Cluster information gathered successfully")
}

func displayVersionInfo() {
	fmt.Println(binaryName)
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Git commit: %s\n", gitCommit)
	fmt.Printf("Go version: %s\n", goVersion)
	fmt.Printf("Build time: %s\n", buildTime)
}

func displayHelpMessage() {
	fmt.Println("Usage: ambient-migration-estimator [options]")
	fmt.Println("Options:")
	fmt.Println("  --hide-names|-hn     Hide the names of the cluster and namespaces using a hash")
	fmt.Println("  --help|-h            Show this help message")
	fmt.Println("  --continue|-c        Continue processing from the last saved state if the script was interrupted")
	fmt.Println("  --version|-v         Show version information")
	fmt.Println("  --context|-ctx       Kubernetes context to use (if not set, uses current context)")
	fmt.Println("")
	fmt.Println("Environment variables:")
	fmt.Println("  CONTEXT              Kubernetes context to use (if not set, uses current context)")
}
