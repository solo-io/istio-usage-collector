package utils

import (
	"k8s.io/client-go/tools/clientcmd"
)

// GetCurrentContext returns the current Kubernetes context from the kubeconfig
func GetCurrentContext() (string, error) {
	// Use the default loading rules (which respect the KUBECONFIG env variable)
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})

	// Get the raw kubeconfig
	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return "", err
	}

	if rawConfig.CurrentContext == "" {
		return "", ErrNoCurrentContext
	}

	return rawConfig.CurrentContext, nil
}
