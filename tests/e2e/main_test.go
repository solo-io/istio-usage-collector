//go:build test || e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/solo-io/istio-usage-collector/internal/utils"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// E2ETestSuite defines the main structure for our E2E tests
type E2ETestSuite struct {
	suite.Suite
	clusterName          string
	kubeconfigPath       string
	istioValuesPath      string
	installMetricsServer bool
}

// SetupSuite runs once before all tests in the suite.
func (s *E2ETestSuite) SetupSuite() {
	s.T().Log("Setting up E2E test suite...")
	s.clusterName = "e2e-test-cluster" // Or generate dynamically if needed
	s.installMetricsServer = false     // Default: Do not install metrics-server

	// Create kind cluster (using default config for now)
	kubeconfigPath, err := createKindCluster(s.T(), s.clusterName, "") // Pass empty string for default config
	require.NoError(s.T(), err, "Failed to create kind cluster")
	s.kubeconfigPath = kubeconfigPath
	s.T().Logf("Using kubeconfig: %s", s.kubeconfigPath)

	// Install Istio using Helm and a values file
	// Note: Assumes istio repo is already added or available
	s.istioValuesPath = "testdata/input/simple/istio-values.yaml" // Path to Helm values
	err = installIstio(s.T(), s.kubeconfigPath, s.istioValuesPath)
	require.NoError(s.T(), err, "Failed to install Istio using Helm with values %s", s.istioValuesPath)

	// Optionally install Metrics Server
	if s.installMetricsServer {
		s.T().Log("Installing Kubernetes Metrics Server...")
		err = installMetricsServer(s.T(), s.kubeconfigPath)
		require.NoError(s.T(), err, "Failed to install metrics-server")
		s.T().Log("Metrics Server installed successfully.")
	} else {
		s.T().Log("Skipping Metrics Server installation.")
	}

	s.T().Log("Suite setup complete.")
}

// TearDownSuite runs once after all tests in the suite have finished.
func (s *E2ETestSuite) TearDownSuite() {
	s.T().Log("Tearing down E2E test suite...")

	// Delete kind cluster
	err := deleteKindCluster(s.T(), s.clusterName, s.kubeconfigPath)
	// Log error but don't fail the suite teardown if cleanup fails
	if err != nil {
		s.T().Logf("Error deleting kind cluster: %v", err)
	} else {
		s.T().Log("Kind cluster deleted successfully.")
	}

	s.T().Log("Suite teardown complete.")
}

// TestE2ERunner is the entry point for running the suite.
func TestE2ERunner(t *testing.T) {
	// Check for necessary prerequisites (kind, kubectl, helm)
	checkForPrerequisites(t)

	suite.Run(t, new(E2ETestSuite))
}

// checkForPrerequisites checks if required CLIs are installed.
func checkForPrerequisites(t *testing.T) {
	requiredCmds := []string{"kind", "kubectl", "helm"}
	for _, cmd := range requiredCmds {
		_, err := exec.LookPath(cmd)
		require.NoErrorf(t, err, "Required command '%s' not found in PATH. Please install it.", cmd)
	}

	// Check that istio helm chart/repo is available
	checkHelmChartAvailable(t, "istio", "https://istio-release.storage.googleapis.com/charts")
}

// checkHelmChartAvailable checks if the desired helm chart is available
func checkHelmChartAvailable(t *testing.T, chartName string, expectedURL string) {
	// run the helm command to check if the chart is available
	listOutput, err := runCommand(t, "helm", "repo", "list", "--output", "json")
	require.NoErrorf(t, err, "Required command 'helm repo list' failed. Please ensure the helm installation is correct.")

	jsonOutput := []byte(listOutput)

	// parse the json
	var jsonOutputList []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	err = json.Unmarshal(jsonOutput, &jsonOutputList)
	require.NoErrorf(t, err, "Failed to unmarshal helm search repo istio output to json")

	istioRepoFound, istioRepoURL := false, ""
	for _, repo := range jsonOutputList {
		if repo.Name == chartName {
			istioRepoFound = true
			istioRepoURL = repo.URL
		}
	}
	require.True(t, istioRepoFound, "Helm chart %s not found", chartName)
	require.Equal(t, istioRepoURL, expectedURL, "Helm chart %s URL mismatch", chartName)
}

// Example test case structure
func (s *E2ETestSuite) TestExampleScenario() {
	s.T().Log("Running test: TestExampleScenario")
	require := s.Require() // Use require for assertions within the test

	// --- Test Setup ---
	inputManifest := "testdata/input/simple/manifest.yaml"

	// create temporary directory for output
	testOutputDir, err := os.MkdirTemp("", "istio-usage-collector-e2e-test")
	require.NoError(err, "Failed to create temporary directory for output")
	defer os.RemoveAll(testOutputDir)

	outputFilePrefix := "output"
	outputFormat := "json"
	expectedOutputFile := "./testdata/output/simple/expected-output.json"

	// Clean previous output dir if it exists
	err = os.RemoveAll(testOutputDir)
	require.NoError(err, "Failed to clean test output directory: %s", testOutputDir)

	// --- Apply Input Manifests ---
	s.T().Logf("Applying input manifest: %s", inputManifest)
	err = applyKubectl(s.T(), s.kubeconfigPath, inputManifest)
	require.NoError(err, "Failed to apply input manifest %s", inputManifest)
	// Add waits or checks here if needed to ensure resources are ready

	// --- Run the Main Binary ---
	s.T().Log("Running main binary...")
	config := utils.Config{
		ObfuscateNames:   false,
		OutputDir:        testOutputDir,
		OutputFormat:     outputFormat,
		OutputFilePrefix: outputFilePrefix,
		NoProgress:       true, // Disabled for cleaner test logs
	}
	actualOutputFile, err := runMainBinary(s.T(), config, s.kubeconfigPath)
	require.NoError(err, "Failed to run main binary")
	s.T().Logf("Actual output file generated: %s", actualOutputFile)

	// --- Compare Output ---
	s.T().Logf("Comparing actual output (%s) with expected output (%s)", actualOutputFile, expectedOutputFile)
	err = compareFiles(s.T(), actualOutputFile, expectedOutputFile)
	require.NoError(err, "Output file content mismatch")

	s.T().Log("TestExampleScenario completed successfully.")
}
