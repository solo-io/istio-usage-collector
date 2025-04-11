package e2e

import (
	"os"
	"testing"
	"time"

	"github.com/solo-io/istio-usage-collector/internal/utils"
	"github.com/stretchr/testify/suite"
)

// SimpleTestSuite defines the main structure for our E2E tests
type SimpleTestSuite struct {
	suite.Suite
	clusterName          string
	kubeconfigPath       string
	istioValuesPath      string
	installMetricsServer bool
}

// SetupSuite runs once before all tests in the suite.
func (s *SimpleTestSuite) SetupSuite() {
	// Check for necessary prerequisites (kind, kubectl, helm)
	checkForPrerequisites(s.T())

	s.T().Log("Setting up E2E test suite...")
	s.clusterName = "e2e-simple-test-cluster"
	s.installMetricsServer = false

	// Create kind cluster (using default config for now)
	kubeconfigPath := createKindCluster(s.T(), s.clusterName, "") // Pass empty string for default config
	s.kubeconfigPath = kubeconfigPath
	s.T().Logf("Using kubeconfig: %s", s.kubeconfigPath)

	// Install Istio using Helm and a values file
	s.istioValuesPath = "testdata/input/istio-values.yaml" // Path to Helm values
	installIstio(s.T(), s.kubeconfigPath, s.istioValuesPath)

	// Optionally install Metrics Server
	if s.installMetricsServer {
		s.T().Log("Installing Kubernetes Metrics Server...")
		installMetricsServer(s.T(), s.kubeconfigPath)
		s.T().Log("Metrics Server installed successfully.")
	} else {
		s.T().Log("Skipping Metrics Server installation.")
	}

	s.T().Log("Suite setup complete.")
}

// TearDownSuite runs once after all tests in the suite have finished.
func (s *SimpleTestSuite) TearDownSuite() {
	s.T().Log("Tearing down E2E test suite...")

	// Delete kind cluster
	deleteKindCluster(s.T(), s.clusterName, s.kubeconfigPath)
	// Log error but don't fail the suite teardown if cleanup fails
	s.T().Log("Kind cluster deleted successfully.")

	s.T().Log("Suite teardown complete.")
}

// TestE2ERunner is the entry point for running the suite.
func TestSimpleTestSuiteRunner(t *testing.T) {
	suite.Run(t, new(SimpleTestSuite))
}

// Example test case structure
func (s *SimpleTestSuite) TestSimpleJSONOutput() {
	s.T().Log("Running test: TestSimpleJSONOutput")
	require := s.Require() // Use require for assertions within the test

	// --- Test Setup ---
	inputManifest := "testdata/input/simple-manifest.yaml"

	// create temporary directory for output
	testOutputDir, err := os.MkdirTemp("", "istio-usage-collector-e2e-test")
	require.NoError(err, "Failed to create temporary directory for output")
	defer os.RemoveAll(testOutputDir)

	outputFilePrefix := "output"
	outputFormat := "json"
	expectedOutputFile := "./testdata/output/simple-expected.json"

	// Clean previous output dir if it exists
	err = os.RemoveAll(testOutputDir)
	require.NoError(err, "Failed to clean test output directory: %s", testOutputDir)

	// --- Apply Input Manifests ---
	s.T().Logf("Applying input manifest: %s", inputManifest)
	applyKubectl(s.T(), s.kubeconfigPath, inputManifest)
	// TODO: Add waits or checks here if needed to ensure resources are ready

	// --- Run the Main Binary ---
	s.T().Log("Running main binary...")
	config := utils.Config{
		ObfuscateNames:   false,
		OutputDir:        testOutputDir,
		OutputFormat:     outputFormat,
		OutputFilePrefix: outputFilePrefix,
		NoProgress:       true, // Disabled for cleaner test logs
	}

	retry := 10
	waitTime := 100 * time.Millisecond
	var lastError error
	for i := 0; i < retry; i++ {
		actualOutputFile := runMainBinary(s.T(), config, s.kubeconfigPath)
		s.T().Logf("Actual output file generated: %s", actualOutputFile)

		// --- Compare Output ---
		s.T().Logf("Comparing actual output (%s) with expected output (%s)", actualOutputFile, expectedOutputFile)
		err := compareFiles(actualOutputFile, expectedOutputFile)
		if err == nil {
			s.T().Log("Output comparison successful.")
			break
		} else {
			lastError = err
			s.T().Logf("Output comparison failed: %v", err)
			time.Sleep(waitTime)
		}
	}

	if lastError != nil {
		s.T().Fatalf("Output comparison failed after %d retries: %v", retry, lastError)
	} else {
		s.T().Log("Output comparison successful.")
	}

	s.T().Log("TestSimpleJSONOutput completed successfully.")
}
