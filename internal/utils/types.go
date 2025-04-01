package utils

// Config represents the configuration for the cluster information gatherer
type Config struct {
	// KubeContext is the name of the Kubernetes context to use
	KubeContext string

	// ObfuscateNames indicates whether to obfuscate names of clusters and namespaces
	ObfuscateNames bool

	// ContinueProcessing indicates whether to continue processing from the last saved state
	ContinueProcessing bool

	// OutputFile is the name of the output JSON file
	OutputFile string
}
