//go:build test || e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type BaseTestSuite struct {
	suite.Suite
	kubeconfigPath  string
	istioValuesPath string
}

func (b *BaseTestSuite) SetupBase(t *testing.T, clusterName string, metricsServer bool) {
	// Check prerequisites before setting up the test suite
	checkForPrerequisites(t)

	// Create kind cluster
	b.kubeconfigPath = createKindCluster(t, clusterName, "")
	t.Logf("Using kubeconfig: %s", b.kubeconfigPath)

	// Install Istio
	b.istioValuesPath = "testdata/input/istio-values.yaml"
	installIstio(t, b.kubeconfigPath, b.istioValuesPath)

	// Optionally install Metrics Server
	if metricsServer {
		t.Log("Installing Kubernetes Metrics Server...")
		installMetricsServer(t, b.kubeconfigPath)
	}
}

func (b *BaseTestSuite) TearDownBase(t *testing.T, clusterName string) {
	t.Log("Tearing down base E2E test suite...")
	deleteKindCluster(t, clusterName, b.kubeconfigPath)
	t.Log("Base suite teardown complete.")
}
