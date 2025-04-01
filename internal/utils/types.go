package utils

// Config represents the configuration for the cluster information gatherer
type Config struct {
	// KubeContext is the name of the Kubernetes context to use
	KubeContext string

	// ObfuscateNames indicates whether to obfuscate names of clusters and namespaces
	ObfuscateNames bool

	// ContinueProcessing indicates whether to continue processing from the last saved state
	ContinueProcessing bool

	// OutputDir is the directory where the output file should be written
	OutputDir string

	// OutputFormat is the format of the output file (json, yaml, csv)
	OutputFormat string

	// OutputFilePrefix is the prefix for the output file name
	OutputFilePrefix string
}
