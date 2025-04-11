//go:build test || e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/google/go-cmp/cmp"
	"github.com/solo-io/istio-usage-collector/internal/models"
	"github.com/solo-io/istio-usage-collector/internal/utils"
)

// runCommand executes a shell command and returns its output or an error.
func runCommand(t *testing.T, name string, args ...string) string {
	cmd := exec.Command(name, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command '%s %s' failed: %v\nOutput: %s", name, strings.Join(args, " "), err, string(output))
	}

	t.Logf("Command '%s %s' output: %s", name, strings.Join(args, " "), string(output))
	return string(output)
}

// createKindCluster creates a Kind cluster with the given name.
func createKindCluster(t *testing.T, clusterName string, kindConfigPath string) (kubeconfigPath string) {
	args := []string{"create", "cluster", "--name", clusterName}
	if kindConfigPath != "" {
		if _, statErr := os.Stat(kindConfigPath); os.IsNotExist(statErr) {
			t.Fatalf("kind configuration file not found at '%s'", kindConfigPath)
		}

		args = append(args, "--config", kindConfigPath)
	}
	args = append(args, "--wait", "5m")

	// Create the cluster
	_ = runCommand(t, "kind", args...)

	// Get the kubeconfig content
	kubeconfigContent := runCommand(t, "kind", "get", "kubeconfig", "--name", clusterName)

	// Create a temporary file for the kubeconfig
	tempFile, err := os.CreateTemp("", "kubeconfig-"+clusterName+"-*.yaml")
	if err != nil {
		deleteKindCluster(t, clusterName, "") // Ignore cleanup error
		t.Fatalf("failed to create temp kubeconfig file: %v", err)
	}
	defer tempFile.Close()

	kubeconfigPath = tempFile.Name()

	_, err = tempFile.WriteString(kubeconfigContent)
	if err != nil {
		deleteKindCluster(t, clusterName, kubeconfigPath)
		t.Fatalf("failed to write kubeconfig to temp file '%s': %v", kubeconfigPath, err)
	}

	return kubeconfigPath
}

// deleteKindCluster deletes the Kind cluster with the given name and removes the associated temp kubeconfig file.
func deleteKindCluster(t *testing.T, clusterName string, kubeconfigPath string) {
	_ = runCommand(t, "kind", "delete", "cluster", "--name", clusterName)

	if kubeconfigPath != "" {
		removeErr := os.Remove(kubeconfigPath)
		if removeErr != nil && !os.IsNotExist(removeErr) {
			t.Fatalf("failed to remove kubeconfig file '%s': %v", kubeconfigPath, removeErr)
		}
	}
}

func installIstio(t *testing.T, kubeconfigPath string, valuesPath string) {
	istioNamespace := "istio-system"

	if _, err := os.Stat(valuesPath); os.IsNotExist(err) {
		t.Fatalf("helm values file not found at '%s'", valuesPath)
	}

	_ = runCommand(t, "kubectl", "create", "namespace", istioNamespace, "--kubeconfig", kubeconfigPath)
	_ = runCommand(t, "helm", "install", "istio-base", "istio/base", "-n", istioNamespace, "--kubeconfig", kubeconfigPath, "--set", "defaultRevision=default", "--wait")
	_ = runCommand(t, "helm", "install", "istiod", "istio/istiod", "-n", istioNamespace, "-f", valuesPath, "--kubeconfig", kubeconfigPath, "--wait")

	_ = runCommand(t, "kubectl", "wait", "--for=condition=available", "deployment/istiod", "-n", istioNamespace, "--timeout=5m", "--kubeconfig", kubeconfigPath)
}

// installMetricsServer installs the Kubernetes Metrics Server.
func installMetricsServer(t *testing.T, kubeconfigPath string) {
	// Install the metrics-server chart
	_ = runCommand(t, "helm", "install", "metrics-server", "metrics-server/metrics-server", "-n", "kube-system", "--kubeconfig", kubeconfigPath, "--set", "args[0]=--kubelet-insecure-tls", "--wait")

	// Wait for metrics-server deployment to be ready
	_ = runCommand(t, "kubectl", "wait", "--for=condition=available", "deployment/metrics-server", "-n", "kube-system", "--timeout=2m", "--kubeconfig", kubeconfigPath)
}

// applyKubectl applies a Kubernetes manifest file.
func applyKubectl(t *testing.T, kubeconfigPath string, path string) {
	_ = runCommand(t, "kubectl", "apply", "-f", path, "--kubeconfig", kubeconfigPath)
}

// runMainBinary runs the main application binary with the specified configuration.
func runMainBinary(t *testing.T, config utils.Config) string {
	err := os.MkdirAll(config.OutputDir, 0755)
	if err != nil {
		t.Fatalf("failed to create output directory '%s': %v", config.OutputDir, err)
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

	// Execute the command from the repository root
	cmd := exec.Command("go", args...)
	cmd.Dir = "../../" // tests/e2e is two levels down from root -- TODO: use a better way to run the main binary

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run main binary: %v\nOutput: %s", err, string(output))
	}

	// expected output file path based on conventions
	outputFileName := fmt.Sprintf("%s.%s", config.OutputFilePrefix, config.OutputFormat)
	outputFilePath := fmt.Sprintf("%s/%s", config.OutputDir, outputFileName)

	// Check if the output file was actually created
	if _, statErr := os.Stat(outputFilePath); os.IsNotExist(statErr) {
		t.Fatalf("main binary ran but output file '%s' was not found", outputFilePath)
	}

	return outputFilePath
}

// compareFiles compares the content of two JSON files using go-cmp.
func compareFiles(file1, file2 string) error {
	content1, err := os.ReadFile(file1)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %v", file1, err)
	}
	content2, err := os.ReadFile(file2)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %v", file2, err)
	}

	var data1, data2 models.ClusterInfo

	err = json.Unmarshal(content1, &data1)
	if err != nil {
		return fmt.Errorf("failed to unmarshal %s as JSON: %v", file1, err)
	}

	err = json.Unmarshal(content2, &data2)
	if err != nil {
		return fmt.Errorf("failed to unmarshal %s as JSON: %v", file2, err)
	}

	irrelevantNamespaces := map[string]struct{}{
		"istio-system":       {},
		"kube-node-lease":    {},
		"kube-public":        {},
		"kube-system":        {},
		"local-path-storage": {},
	}

	opts := cmp.Options{
		cmpopts.IgnoreMapEntries(func(key string, value *models.NamespaceInfo) bool {
			_, irrelevant := irrelevantNamespaces[key]
			return irrelevant
		}),
		cmp.Transformer("ActualPresence", func(in *models.Resources) bool {
			return in != nil
		}),
		cmp.Transformer("NodeActualPresence", func(in *models.NodeResourceSpec) bool {
			return in != nil
		}),
	}

	// Compare the unmarshalled data
	if diff := cmp.Diff(data1, data2, opts); diff != "" {
		return fmt.Errorf("JSON content mismatch between '%s' and '%s':\n--- Diff ---\n%s\n------------", file1, file2, diff)
	}

	return nil
}

// checkForPrerequisites checks if required CLIs are installed.
func checkForPrerequisites(t *testing.T) {
	requiredCmds := []string{"kind", "kubectl", "helm"}
	for _, cmd := range requiredCmds {
		_, err := exec.LookPath(cmd)
		if err != nil {
			t.Fatalf("Required command '%s' not found in PATH. Please install it.", cmd)
		}
	}

	// Check that required helm charts are available
	checkHelmChartAvailable(t, "istio", "https://istio-release.storage.googleapis.com/charts")
	checkHelmChartAvailable(t, "metrics-server", "https://kubernetes-sigs.github.io/metrics-server/")
}

// checkHelmChartAvailable checks if the desired helm chart is available
func checkHelmChartAvailable(t *testing.T, chartName string, expectedURL string) {
	listOutput := runCommand(t, "helm", "repo", "list", "--output", "json")
	jsonOutput := []byte(listOutput)

	// parse the json
	var jsonOutputList []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	err := json.Unmarshal(jsonOutput, &jsonOutputList)
	if err != nil {
		t.Fatalf("Failed to unmarshal helm search repo istio output to json: %v", err)
	}

	istioRepoFound, istioRepoURL := false, ""
	for _, repo := range jsonOutputList {
		if repo.Name == chartName {
			istioRepoFound = true
			istioRepoURL = repo.URL
		}
	}
	if !istioRepoFound {
		t.Fatalf("Helm chart %s not found", chartName)
	}
	if istioRepoURL != expectedURL {
		t.Fatalf("Helm chart %s URL mismatch: expected %s, got %s", chartName, expectedURL, istioRepoURL)
	}
}

type testContext struct {
	kubeResourceManifest string
	expectedOutputFile   string
}
