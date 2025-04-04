package utils

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
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

// createKubernetesClients creates Kubernetes clients for the specified context
func CreateKubernetesClients(ctx context.Context, kubeContext string) (*kubernetes.Clientset, *metricsv.Clientset, bool, error) {
	// Get kubeconfig path
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return nil, nil, false, fmt.Errorf("HOME environment variable not set")
		}
		kubeconfigPath = fmt.Sprintf("%s/.kube/config", home)
	}

	// Build config
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext}).ClientConfig()
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	// Increase QPS and burst to avoid client-side throttling
	config.QPS = 100
	config.Burst = 100

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	// Verify the connection
	_, err = clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to connect to Kubernetes API server: %w", err)
	}

	// Create metrics client
	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		return clientset, nil, false, fmt.Errorf("failed to create metrics client: %w", err)
	}

	// Check if metrics API is available by calling the metrics API
	hasMetrics := false
	if metricsClient != nil {
		_, err := metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{Limit: 1})
		hasMetrics = err == nil
	}

	return clientset, metricsClient, hasMetrics, nil
}
