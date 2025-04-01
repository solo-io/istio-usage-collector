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

type CommandFlags struct {
	HideNames          bool
	ContinueProcessing bool
	KubeContext        string
	OutputDir          string
	OutputFormat       string
	OutputFilePrefix   string
}

// DefaultFlags returns a CommandFlags struct initialized with default values
func DefaultFlags() *CommandFlags {
	return &CommandFlags{
		HideNames:          false,
		ContinueProcessing: false,
		KubeContext:        "",
		OutputDir:          ".",
		OutputFormat:       "json",
		OutputFilePrefix:   "",
	}
}

// internalFlags is used for standalone CLI usage
var internalFlags = DefaultFlags()

// SetVersionInfo sets the version information for the application
func SetVersionInfo(binary, ver, commit, goVer, built string) {
	binaryName = binary
	version = ver
	gitCommit = commit
	goVersion = goVer
	buildTime = built
}

// GetCommand returns the root command for the ambient-migration-estimator
// This allows it to be used as a standalone command or as a subcommand in another CLI
// If customFlags is provided, those flags will be used instead of the default ones
func GetCommand(customFlags ...*CommandFlags) *cobra.Command {
	// Determine which flags to use
	var flags *CommandFlags
	if len(customFlags) > 0 && customFlags[0] != nil {
		flags = customFlags[0]
	} else {
		flags = internalFlags
	}

	cmd := &cobra.Command{
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
			if flags.KubeContext == "" {
				var err error
				flags.KubeContext, err = utils.GetCurrentContext()
				if err != nil {
					logging.Error("No current kubectl context found: %v", err)
					return err
				}
				logging.Info("Using current context: %s", flags.KubeContext)
			} else {
				logging.Info("Using Kubernetes context from flags: %s", flags.KubeContext)
			}

			if flags.OutputDir == "" {
				flags.OutputDir = "."
			}

			if flags.OutputFormat != "" && flags.OutputFormat != "json" && flags.OutputFormat != "yaml" && flags.OutputFormat != "yml" {
				return fmt.Errorf("unsupported output format: %s", flags.OutputFormat)
			}
			if flags.OutputFormat == "" {
				flags.OutputFormat = "json"
			}

			prefix := flags.OutputFilePrefix
			if prefix == "" {
				// Use the context name as the default prefix
				prefix = flags.KubeContext
				if flags.HideNames {
					prefix = gatherer.ObfuscateName(prefix)
				}
			}

			// Create config
			cfg := &utils.Config{
				KubeContext:        flags.KubeContext,
				ObfuscateNames:     flags.HideNames,
				ContinueProcessing: flags.ContinueProcessing,
				OutputDir:          flags.OutputDir,
				OutputFormat:       flags.OutputFormat,
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

	// Define persistent flags for the command
	cmd.PersistentFlags().BoolVarP(&flags.HideNames, "hide-names", "n", false, "Hide the names of the cluster and namespaces using a hash")
	cmd.PersistentFlags().BoolVarP(&flags.ContinueProcessing, "continue", "c", false, "Continue processing from the last saved state if the script was interrupted")
	cmd.PersistentFlags().StringVarP(&flags.KubeContext, "context", "k", "", "Kubernetes context to use (if not set, uses current context)")
	cmd.PersistentFlags().StringVarP(&flags.OutputDir, "output-dir", "d", ".", "Directory to store output file")
	cmd.PersistentFlags().StringVarP(&flags.OutputFormat, "format", "f", "json", "Output format (json, yaml/yml)")
	cmd.PersistentFlags().StringVarP(&flags.OutputFilePrefix, "output-prefix", "p", "", "Custom prefix for output file (default: cluster name)")

	// Add the version command to every instance
	cmd.AddCommand(CreateVersionCommand())

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main() when the CLI is used standalone.
func Execute() {
	err := GetCommand().Execute()
	if err != nil {
		os.Exit(1)
	}
}
