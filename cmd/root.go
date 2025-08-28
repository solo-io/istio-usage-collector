package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/solo-io/istio-usage-collector/cmd/version"
	"github.com/solo-io/istio-usage-collector/internal/gatherer"
	"github.com/solo-io/istio-usage-collector/internal/logging"
	"github.com/solo-io/istio-usage-collector/internal/utils"
	"github.com/spf13/cobra"
)

type CommandFlags struct {
	HideNames          bool
	ContinueProcessing bool
	KubeContext        string
	OutputDir          string
	OutputFormat       string
	OutputFilePrefix   string
	EnableDebug        bool
	NoProgress         bool
	MaxProcessors      int
}

// DefaultFlags returns a CommandFlags struct initialized with default values
func DefaultFlags() *CommandFlags {
	return &CommandFlags{
		HideNames:          true,
		ContinueProcessing: false,
		KubeContext:        "",
		OutputDir:          ".",
		OutputFormat:       "json",
		OutputFilePrefix:   "",
		EnableDebug:        false,
		NoProgress:         false,
		MaxProcessors:      0,
	}
}

// internalFlags is used for standalone CLI usage
var internalFlags = DefaultFlags()

// GetCommand returns the root command for the istio-usage-collector
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
		Use:          "istio-usage-collector",
		Short:        "Gather Kubernetes cluster information for ambient migration cost estimation.",
		Long:         "The istio-usage-collector tool collects information from your Kubernetes cluster to help estimate the cost and resource requirements for migrating from a sidecar mesh to an ambient mesh.",
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

			// defining the log level
			if flags.EnableDebug {
				logging.EnableDebugMessages()
			}

			// If context is not specified, use current context
			if flags.KubeContext == "" {
				var err error
				flags.KubeContext, err = utils.GetCurrentContext()
				if err != nil {
					logging.Error("No current Kubernetes context found: %v", err)
					return err
				}
				logging.Info("Using current Kubernetes context: %s", flags.KubeContext)
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
				NoProgress:         flags.NoProgress,
				MaxProcessors:      flags.MaxProcessors,
			}

			// Gather cluster information
			if err := gatherer.GatherClusterInfo(ctx, cfg); err != nil {
				logging.Error("Error gathering cluster information: %v", err)
				return err
			}

			logging.Success("Cluster information gathered successfully")
			return nil
		},
	}

	// Define persistent flags for the command
	cmd.PersistentFlags().BoolVarP(&flags.HideNames, "hide-names", "n", true, "Hide the names of the cluster and namespaces by using a hash. If not set, defaults to true.")
	cmd.PersistentFlags().BoolVarP(&flags.ContinueProcessing, "continue", "c", false, "If the script was interrupted, continue processing from the last saved state.")
	cmd.PersistentFlags().StringVarP(&flags.KubeContext, "context", "k", "", "Kubernetes context to use. If not set, uses the current context.")
	cmd.PersistentFlags().StringVarP(&flags.OutputDir, "output-dir", "d", ".", "Directory to store the output file in.")
	cmd.PersistentFlags().StringVarP(&flags.OutputFormat, "format", "f", "json", "Format the output file in json or yaml/yml.")
	cmd.PersistentFlags().StringVarP(&flags.OutputFilePrefix, "output-prefix", "p", "", "Custom prefix for the output file. If not set, uses the cluster name.")
	cmd.PersistentFlags().BoolVar(&flags.EnableDebug, "debug", false, "Enable debug mode.")
	cmd.PersistentFlags().BoolVar(&flags.NoProgress, "no-progress", false, "Disable the progress bar while processing resources.")
	cmd.PersistentFlags().IntVar(&flags.MaxProcessors, "max-processors", 0, "Maximum number of processors to use. If not set, or <= 0, it will use all available processors.")

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main() when the CLI is used standalone.
func Execute() {
	cmd := GetCommand()

	cmd.Version = "n/a" // This needs to be set so that the --version flag works when setting the version template
	cmd.SetVersionTemplate(version.VersionTemplate())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
