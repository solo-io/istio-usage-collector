package root

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/solo-io/ambient-migration-estimator-snapshot/internal/gatherer"
	"github.com/solo-io/ambient-migration-estimator-snapshot/internal/logging"
	"github.com/solo-io/ambient-migration-estimator-snapshot/internal/utils"
	"github.com/spf13/cobra"
)

// These variables are set during build time via -ldflags
var (
	binaryName = "n/a"
	version    = "n/a"
	buildTime  = "n/a"
	gitCommit  = "n/a"
	goVersion  = "n/a"
)

// CLI flags
var (
	hideNames          bool
	continueProcessing bool
	kubeContext        string
	outputDir          string
	outputFormat       string
	outputFilePrefix   string
)

// SetVersionInfo sets the version information for the application
func SetVersionInfo(binary, ver, commit, goVer, built string) {
	binaryName = binary
	version = ver
	gitCommit = commit
	goVersion = goVer
	buildTime = built
}

var RootCmd = &cobra.Command{
	Use:   "ambient-migration-estimator",
	Short: "Gather Kubernetes cluster information for ambient migration estimation",
	Long: `The ambient-migration-estimator tool collects information from your Kubernetes cluster
to help estimate the cost and resource requirements for migrating to Ambient Mesh.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		// If context is not specified, use current context
		if kubeContext == "" {
			var err error
			kubeContext, err = utils.GetCurrentContext()
			if err != nil {
				logging.Error("No current kubectl context found: %v", err)
				return err
			}
			logging.Info("Using current context: %s", kubeContext)
		} else {
			logging.Info("Using Kubernetes context from flags: %s", kubeContext)
		}

		if outputDir == "" {
			outputDir = "."
		}

		if outputFormat != "" && outputFormat != "json" && outputFormat != "yaml" && outputFormat != "yml" && outputFormat != "csv" {
			return fmt.Errorf("unsupported output format: %s", outputFormat)
		}
		if outputFormat == "" {
			outputFormat = "json"
		}

		prefix := outputFilePrefix
		if prefix == "" {
			// Use the context name as the default prefix
			prefix = kubeContext
			if hideNames {
				prefix = gatherer.ObfuscateName(prefix)
			}
		}

		// Create config
		cfg := &utils.Config{
			KubeContext:        kubeContext,
			ObfuscateNames:     hideNames,
			ContinueProcessing: continueProcessing,
			OutputDir:          outputDir,
			OutputFormat:       outputFormat,
			OutputFilePrefix:   prefix,
		}

		// Gather cluster information
		if err := gatherer.GatherClusterInfo(ctx, cfg); err != nil {
			logging.Error("Error gathering cluster information: %v", err)
			return err
		}

		logging.Info("Cluster information gathered successfully")
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Define persistent flags for the root command
	RootCmd.PersistentFlags().BoolVarP(&hideNames, "hide-names", "n", false, "Hide the names of the cluster and namespaces using a hash")
	RootCmd.PersistentFlags().BoolVarP(&continueProcessing, "continue", "c", false, "Continue processing from the last saved state if the script was interrupted")
	RootCmd.PersistentFlags().StringVarP(&kubeContext, "context", "k", "", "Kubernetes context to use (if not set, uses current context)")
	RootCmd.PersistentFlags().StringVarP(&outputDir, "output-dir", "d", ".", "Directory to store output file")
	RootCmd.PersistentFlags().StringVarP(&outputFormat, "format", "f", "json", "Output format (json, yaml/yml)")
	RootCmd.PersistentFlags().StringVarP(&outputFilePrefix, "output-prefix", "p", "", "Custom prefix for output file (default: cluster name)")
}
