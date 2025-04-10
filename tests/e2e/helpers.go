//go:build test || e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/solo-io/istio-usage-collector/internal/utils"
)

// runCommand executes a shell command and returns its output or an error.
func runCommand(t *testing.T, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command failed: %s\nOutput:\n%s", err, string(output))
		return string(output), err
	}

	return string(output), nil
}

// createKindCluster creates a Kind cluster with the given name.
func createKindCluster(t *testing.T, clusterName string, kindConfigPath string) (kubeconfigPath string, err error) {
	args := []string{"create", "cluster", "--name", clusterName}
	if kindConfigPath != "" {
		if _, statErr := os.Stat(kindConfigPath); os.IsNotExist(statErr) {
			return "", fmt.Errorf("kind configuration file not found at '%s'", kindConfigPath)
		}

		args = append(args, "--config", kindConfigPath)
	}
	args = append(args, "--wait", "5m")

	// Create the cluster
	_, err = runCommand(t, "kind", args...)
	if err != nil {
		return "", fmt.Errorf("failed to create kind cluster '%s': %w", clusterName, err)
	}

	// Get the kubeconfig content
	kubeconfigContent, err := runCommand(t, "kind", "get", "kubeconfig", "--name", clusterName)
	if err != nil {
		_ = deleteKindCluster(t, clusterName, "") // Ignore cleanup error
		return "", fmt.Errorf("failed to get kubeconfig for cluster '%s': %w", clusterName, err)
	}

	// Create a temporary file for the kubeconfig
	tempFile, err := os.CreateTemp("", "kubeconfig-"+clusterName+"-*.yaml")
	if err != nil {
		_ = deleteKindCluster(t, clusterName, "") // Ignore cleanup error
		return "", fmt.Errorf("failed to create temp kubeconfig file: %w", err)
	}
	defer tempFile.Close()

	kubeconfigPath = tempFile.Name()

	_, err = tempFile.WriteString(kubeconfigContent)
	if err != nil {
		_ = deleteKindCluster(t, clusterName, kubeconfigPath)
		return "", fmt.Errorf("failed to write kubeconfig to temp file '%s': %w", kubeconfigPath, err)
	}

	return kubeconfigPath, nil
}

// deleteKindCluster deletes the Kind cluster with the given name and removes the associated temp kubeconfig file.
func deleteKindCluster(t *testing.T, clusterName string, kubeconfigPath string) error {
	_, err := runCommand(t, "kind", "delete", "cluster", "--name", clusterName)
	if err != nil {
		t.Logf("Failed to delete kind cluster '%s': %v", clusterName, err)
		// Don't return error immediately, to continue trying to remove kubeconfig
	}

	if kubeconfigPath != "" {
		removeErr := os.Remove(kubeconfigPath)
		if removeErr != nil && !os.IsNotExist(removeErr) {
			if err == nil {
				err = fmt.Errorf("failed to remove kubeconfig file: %w", removeErr)
			}
		}
	}
	return err
}

// installIstioWithHelm installs Istio using Helm charts - this assumes the 'istio' Helm repo is added and available.
// TODO: Use a specific version of istio?
func installIstio(t *testing.T, kubeconfigPath string, valuesPath string) error {
	istioNamespace := "istio-system"
	t.Logf("Installing Istio using Helm with values file: %s", valuesPath)

	// Check if values file exists
	if _, err := os.Stat(valuesPath); os.IsNotExist(err) {
		return fmt.Errorf("helm values file not found at '%s'", valuesPath)
	}

	// 1. Create istio-system namespace
	_, err := runCommand(t, "kubectl", "create", "namespace", istioNamespace, "--kubeconfig", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create istio-system namespace: %w", err)
	}

	// 2. Install istio-base chart
	_, err = runCommand(t, "helm", "install", "istio-base", "istio/base", "-n", istioNamespace, "--kubeconfig", kubeconfigPath, "--set", "defaultRevision=default", "--wait")
	if err != nil {
		return fmt.Errorf("failed to install istio-base chart: %w", err)
	}

	// 3. Install istiod chart with custom values
	_, err = runCommand(t, "helm", "install", "istiod", "istio/istiod", "-n", istioNamespace, "-f", valuesPath, "--kubeconfig", kubeconfigPath, "--wait")
	if err != nil {
		return fmt.Errorf("failed to install istiod chart: %w", err)
	}

	// 4. Wait for istiod to be fully available -- TODO: Is this enough for the sidecars to be set up?
	_, waitErr := runCommand(t, "kubectl", "wait", "--for=condition=available", "deployment/istiod", "-n", istioNamespace, "--timeout=5m", "--kubeconfig", kubeconfigPath)
	if waitErr != nil {
		return fmt.Errorf("failed waiting for istiod deployment to become available after helm install: %w", waitErr)
	}

	return nil
}

// installMetricsServer installs the Kubernetes Metrics Server.
func installMetricsServer(t *testing.T, kubeconfigPath string) error {
	// TODO: Use a specific version of metrics-server?
	metricsServerURL := "https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml"
	_, err := runCommand(t, "kubectl", "apply", "-f", metricsServerURL, "--kubeconfig", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to apply metrics-server components: %w", err)
	}

	// Patch the deployment for Kind (add --kubelet-insecure-tls)
	patch := `{"spec":{"template":{"spec":{"containers":[{"name":"metrics-server","args":["--cert-dir=/tmp","--secure-port=4443","--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname","--kubelet-use-node-status-port","--metric-resolution=15s","--kubelet-insecure-tls"]}]}}}}`
	_, err = runCommand(t, "kubectl", "patch", "deployment", "metrics-server", "-n", "kube-system", "--type=strategic", "--patch", patch, "--kubeconfig", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to patch metrics-server deployment: %w", err)
	}

	// Optional: Wait for metrics-server deployment to be ready -- TODO: Is this enough to get metrics?
	_, waitErr := runCommand(t, "kubectl", "wait", "--for=condition=available", "deployment/metrics-server", "-n", "kube-system", "--timeout=2m", "--kubeconfig", kubeconfigPath)
	if waitErr != nil {
		return fmt.Errorf("failed to wait for metrics-server deployment to be ready: %w", waitErr)
	}

	return nil
}

// applyKubectl applies a Kubernetes YAML manifest file.
func applyKubectl(t *testing.T, kubeconfigPath string, yamlPath string) error {
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		return fmt.Errorf("kubectl apply failed: manifest file not found at %s", yamlPath)
	}

	_, err := runCommand(t, "kubectl", "apply", "-f", yamlPath, "--kubeconfig", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("kubectl apply failed for manifest '%s': %w", yamlPath, err)
	}

	return nil
}

// runMainBinary runs the main application binary with the specified configuration.
func runMainBinary(t *testing.T, config utils.Config, kubeconfigPath string) (outputFilePath string, err error) {
	err = os.MkdirAll(config.OutputDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create output directory '%s': %w", config.OutputDir, err)
	}

	// Construct command line arguments
	args := []string{"run", "main.go"}
	if config.ObfuscateNames {
		args = append(args, "--hide-names")
	}
	if config.ContinueProcessing {
		args = append(args, "--continue")
	}
	if config.OutputDir != "" {
		args = append(args, "--output-dir", config.OutputDir)
	}
	if config.OutputFormat != "" {
		args = append(args, "--format", config.OutputFormat)
	}
	if config.OutputFilePrefix != "" {
		args = append(args, "--output-prefix", config.OutputFilePrefix)
	}
	if config.NoProgress {
		args = append(args, "--no-progress")
	}

	t.Logf("Running main binary with args: %v", args)

	// Execute the command from the repository root
	cmd := exec.Command("go", args...)
	cmd.Dir = "../../" // tests/e2e is two levels down from root -- TODO: use a better way to run the main binary

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run main binary: %w\nOutput: %s", err, string(output))
	}

	// Determine the expected output file path based on conventions
	outputFileName := fmt.Sprintf("%s.%s", config.OutputFilePrefix, config.OutputFormat)
	outputFilePath = fmt.Sprintf("%s/%s", config.OutputDir, outputFileName)

	// Check if the output file was actually created
	if _, statErr := os.Stat(outputFilePath); os.IsNotExist(statErr) {
		return "", fmt.Errorf("main binary ran but output file '%s' was not found", outputFilePath)
	}

	return outputFilePath, nil
}

// compareFiles compares the content of two JSON files using go-cmp.
func compareFiles(t *testing.T, file1, file2 string) error {
	content1, err := os.ReadFile(file1)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %w", file1, err)
	}
	content2, err := os.ReadFile(file2)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %w", file2, err)
	}

	var data1, data2 interface{}

	err = json.Unmarshal(content1, &data1)
	if err != nil {
		return fmt.Errorf("failed to unmarshal %s as JSON: %w", file1, err)
	}

	err = json.Unmarshal(content2, &data2)
	if err != nil {
		return fmt.Errorf("failed to unmarshal %s as JSON: %w", file2, err)
	}

	// Compare the unmarshalled data
	if diff := cmp.Diff(data1, data2); diff != "" {
		return fmt.Errorf("JSON content mismatch between '%s' and '%s':\n--- Diff ---\n%s\n------------", file1, file2, diff)
	}

	return nil
}
