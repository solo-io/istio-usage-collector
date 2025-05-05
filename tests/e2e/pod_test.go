//go:build test || e2e

package e2e

import (
	"os"
	"testing"
	"time"

	"github.com/solo-io/istio-usage-collector/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// PodTestSuite defines the main structure for our E2E tests
type PodTestSuite struct {
	BaseTestSuite
	clusterName   string
	metricsServer bool
}

// SetupSuite runs once before all tests in the suite.
func (s *PodTestSuite) SetupSuite() {
	s.clusterName = "e2e-pod-test-cluster"
	s.metricsServer = false
	s.SetupBase(s.T(), s.clusterName, s.metricsServer)
}

// TearDownSuite runs once after all tests in the suite have finished.
func (s *PodTestSuite) TearDownSuite() {
	s.TearDownBase(s.T(), s.clusterName)
}

// TestE2ERunner is the entry point for running the suite.
func TestPodTestSuiteRunner(t *testing.T) {
	suite.Run(t, new(PodTestSuite))
}

// Example test case structure
func (s *PodTestSuite) TestPodJSONOutput() {
	require := s.Require() // Use require for assertions within the test

	// --- Test Setup ---
	inputManifest := "testdata/input/pod-manifest.yaml"
	expectedOutputFile := "./testdata/output/pod-expected.json"

	// create temporary directory for output
	testOutputDir, err := os.MkdirTemp("", "istio-usage-collector-e2e-test")
	require.NoError(err, "Failed to create temporary directory for output")
	defer os.RemoveAll(testOutputDir)

	// --- Apply Input Manifests ---
	s.T().Logf("Applying input manifest: %s", inputManifest)
	applyKubectl(s.T(), s.kubeconfigPath, inputManifest)

	// --- Run the Main Binary ---
	s.T().Log("Running main binary...")
	config := utils.Config{
		ObfuscateNames: false,
		NoProgress:     true, // Disabled for cleaner test logs
		OutputDir:      testOutputDir,
		// Not necessary, but for the ease of testing (finding exact output file), setting these values
		OutputFormat:     "json",
		OutputFilePrefix: "output",
	}

	assert.Eventually(s.T(), func() bool {
		actualOutputFile := runMainBinary(s.T(), config)
		if err := compareFiles(actualOutputFile, expectedOutputFile); err != nil {
			s.T().Logf("Output comparison failed: %v", err)
			return false
		}

		return true
	}, 30*time.Second, 100*time.Millisecond, "Output comparison failed")
}
