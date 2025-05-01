//go:build performance

package gatherer

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/solo-io/istio-usage-collector/internal/utils"
	"github.com/solo-io/istio-usage-collector/pkg/models"
	"sigs.k8s.io/yaml"

	testutils "github.com/solo-io/istio-usage-collector/tests"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	v1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

// PerformanceTestConfig defines the overall structure for the performance test YAML config.
type PerformanceTestConfig struct {
	Namespaces []NamespaceConfig `yaml:"namespaces"`
}

// NamespaceConfig defines the configuration for a single namespace in the test.
type NamespaceConfig struct {
	NamePrefix      string            `yaml:"namePrefix"`      // Prefix for generated namespace names
	Count           int               `yaml:"count"`           // Number of namespaces to generate with this config
	Labels          map[string]string `yaml:"labels"`          // Labels to apply to the namespace
	PodConfig       PodConfig         `yaml:"podConfig"`       // Configuration for pods within this namespace
	HasMetrics      bool              `yaml:"hasMetrics"`      // Whether to generate metrics for pods in this namespace
	IsIstioInjected bool              `yaml:"isIstioInjected"` // Convenience flag to set injection labels automatically
}

// PodConfig defines the configuration for pods within a namespace.
type PodConfig struct {
	NamePrefix        string            `yaml:"namePrefix"`        // Prefix for generated pod names
	Count             int               `yaml:"count"`             // Number of pods to generate per namespace
	Labels            map[string]string `yaml:"labels"`            // Labels to apply to the pod
	NodeNamePrefix    string            `yaml:"nodeNamePrefix"`    // Prefix for node names pods are scheduled on
	AppContainer      ContainerConfig   `yaml:"appContainer"`      // Configuration for the main application container
	IstioProxy        *ContainerConfig  `yaml:"istioProxy"`        // Configuration for the istio-proxy container (optional)
	HasIstioProxy     bool              `yaml:"hasIstioProxy"`     // Explicitly state if istio-proxy container should exist (used even if NamespaceConfig.IsIstioInjected is false for testing edge cases)
	IstioInjectedBy   string            `yaml:"istioInjectedBy"`   // How istio injection is determined ("namespace", "podLabel", "revisionLabel", "" (none)) - defaults to "namespace" if NamespaceConfig.IsIstioInjected is true
	IstioRevisionName string            `yaml:"istioRevisionName"` // Revision name used if istioInjectedBy is "revisionLabel"
}

// ContainerConfig defines the resource requests and usage for a container.
type ContainerConfig struct {
	CPURequest string `yaml:"cpuRequest"` // e.g., "100m"
	MemRequest string `yaml:"memRequest"` // e.g., "128Mi"
	CPUActual  string `yaml:"cpuActual"`  // e.g., "50m" - Only used if NamespaceConfig.HasMetrics is true
	MemActual  string `yaml:"memActual"`  // e.g., "64Mi" - Only used if NamespaceConfig.HasMetrics is true
}

// loadPerformanceTestConfig loads the test configuration from the specified YAML file.
func loadPerformanceTestConfig(filePath string) (*PerformanceTestConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read performance test config file %s: %w", filePath, err)
	}

	var config PerformanceTestConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal performance test config from %s: %w", filePath, err)
	}

	return &config, nil
}

// generateMockResources creates fake Kubernetes and metrics objects based on the config.
func generateMockResources(config *PerformanceTestConfig, revisionWebhookPath, sidecarWebhookPath string) ([]runtime.Object, []runtime.Object, error) {
	kubeObjects := []runtime.Object{}
	metricsObjects := []runtime.Object{}

	// load default webhooks for injection checks
	var istioRevisionTagDefaultWebhook admissionregistrationv1.MutatingWebhookConfiguration
	data, err := os.ReadFile(revisionWebhookPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read revision webhook file %s: %w", revisionWebhookPath, err)
	}
	err = yaml.Unmarshal(data, &istioRevisionTagDefaultWebhook)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal revision webhook: %w", err)
	}

	var istioSidecarInjectorWebhook admissionregistrationv1.MutatingWebhookConfiguration
	data, err = os.ReadFile(sidecarWebhookPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read sidecar injector webhook file %s: %w", sidecarWebhookPath, err)
	}
	err = yaml.Unmarshal(data, &istioSidecarInjectorWebhook)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal sidecar injector webhook: %w", err)
	}

	// Add webhooks to kubeObjects so they can be retrieved by processNamespace
	kubeObjects = append(kubeObjects, &istioRevisionTagDefaultWebhook, &istioSidecarInjectorWebhook)

	for _, nsConfig := range config.Namespaces {
		for i := 0; i < nsConfig.Count; i++ {
			nsName := fmt.Sprintf("%s%d", nsConfig.NamePrefix, i)
			nsLabels := make(map[string]string)
			for k, v := range nsConfig.Labels {
				nsLabels[k] = v
			}

			// Automatically set istio-injection label if IsIstioInjected is true and not explicitly set
			if nsConfig.IsIstioInjected {
				if _, exists := nsLabels["istio-injection"]; !exists {
					// only add if istio.io/rev is not set
					if _, revExists := nsLabels["istio.io/rev"]; !revExists {
						nsLabels["istio-injection"] = "enabled"
					}
				}
			}

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   nsName,
					Labels: nsLabels,
				},
			}
			kubeObjects = append(kubeObjects, ns)

			// Generate Pods for this namespace
			for j := 0; j < nsConfig.PodConfig.Count; j++ {
				podName := fmt.Sprintf("%s%d", nsConfig.PodConfig.NamePrefix, j)
				nodeName := fmt.Sprintf("%s%d", nsConfig.PodConfig.NodeNamePrefix, j%5) // Distribute pods across a few nodes
				podLabels := make(map[string]string)
				for k, v := range nsConfig.PodConfig.Labels {
					podLabels[k] = v
				}

				// Determine istio injection based on config
				hasIstioSidecar := nsConfig.PodConfig.HasIstioProxy
				isPodIstioInjected := false
				injectionMechanism := nsConfig.PodConfig.IstioInjectedBy
				if nsConfig.IsIstioInjected && injectionMechanism == "" {
					injectionMechanism = "namespace" // Default to namespace if namespace is flagged and mechanism isn't specified
				}

				switch injectionMechanism {
				case "namespace":
					isPodIstioInjected = nsConfig.IsIstioInjected // Inherit from namespace setting
				case "podLabel":
					// Check namespace labels first for disable
					if nsLabels["istio-injection"] != "disabled" {
						podLabels["sidecar.istio.io/inject"] = "true"
						isPodIstioInjected = true
					}
				case "revisionLabel":
					// Check namespace labels first for disable
					if nsLabels["istio-injection"] != "disabled" {
						revName := nsConfig.PodConfig.IstioRevisionName
						if revName == "" {
							revName = "default" // Assume default revision if not specified
						}
						podLabels["istio.io/rev"] = revName
						isPodIstioInjected = true
					}
				default: // No injection explicitly configured for the pod
					isPodIstioInjected = nsConfig.IsIstioInjected && nsLabels["istio-injection"] != "disabled"
				}

				// Override hasIstioSidecar based on final injection decision
				hasIstioSidecar = hasIstioSidecar && isPodIstioInjected

				// Create Pod
				istioCPUReq := ""
				istioMemReq := ""
				if nsConfig.PodConfig.IstioProxy != nil {
					istioCPUReq = nsConfig.PodConfig.IstioProxy.CPURequest
					istioMemReq = nsConfig.PodConfig.IstioProxy.MemRequest
				}

				pod := testutils.NewPod(nsName, podName, nodeName,
					nsConfig.PodConfig.AppContainer.CPURequest,
					nsConfig.PodConfig.AppContainer.MemRequest,
					hasIstioSidecar, // Only add istio-proxy container if explicitly requested *and* injection is active
					istioCPUReq,
					istioMemReq,
					podLabels,
				)
				kubeObjects = append(kubeObjects, pod)

				// Generate Pod Metrics if enabled for the namespace
				if nsConfig.HasMetrics {
					istioCPUAct := ""
					istioMemAct := ""
					if hasIstioSidecar && nsConfig.PodConfig.IstioProxy != nil {
						istioCPUAct = nsConfig.PodConfig.IstioProxy.CPUActual
						istioMemAct = nsConfig.PodConfig.IstioProxy.MemActual
					}
					podMetrics := testutils.NewPodMetrics(nsName, podName,
						nsConfig.PodConfig.AppContainer.CPUActual,
						nsConfig.PodConfig.AppContainer.MemActual,
						hasIstioSidecar,
						istioCPUAct,
						istioMemAct,
					)
					metricsObjects = append(metricsObjects, podMetrics)
				}
			}
		}
	}

	return kubeObjects, metricsObjects, nil
}

type benchmarkCase struct {
	name                string
	configPath          string
	revisionWebhookPath string
	sidecarWebhookPath  string
}

type benchmarkResult struct {
	name                string
	namespacesProcessed int
	podsProcessed       int
	duration            time.Duration
}

func BenchmarkProcessNamespaces(b *testing.B) {
	results := make(map[string]benchmarkResult)
	// Define benchmark cases
	cases := []benchmarkCase{
		{
			name:                "Large Namespaces",
			configPath:          "../../tests/data/performance/large_namespaces.yaml",
			revisionWebhookPath: "../../tests/data/default-istio-revision-tag-mwh.yaml",
			sidecarWebhookPath:  "../../tests/data/default-istio-sidecar-injector-mwh.yaml",
		},
		{
			name:                "Large Pods",
			configPath:          "../../tests/data/performance/large_pods.yaml",
			revisionWebhookPath: "../../tests/data/default-istio-revision-tag-mwh.yaml",
			sidecarWebhookPath:  "../../tests/data/default-istio-sidecar-injector-mwh.yaml",
		},
	}

	for _, bc := range cases {
		// Use b.Run to create a sub-benchmark for each case
		b.Run(bc.name, func(b *testing.B) {
			config, err := loadPerformanceTestConfig(bc.configPath)
			if err != nil {
				// Skip benchmark if config file doesn't exist, log instead of fatal
				if os.IsNotExist(err) {
					b.Skipf("Skipping benchmark %s: Config file not found: %s", bc.name, bc.configPath)
				}
				b.Fatalf("Failed to load performance test config %s: %v", bc.configPath, err)
			}

			// Calculate totals for logging
			totalNamespaces := 0
			totalPods := 0
			for _, nsConfig := range config.Namespaces {
				totalNamespaces += nsConfig.Count
				totalPods += nsConfig.Count * nsConfig.PodConfig.Count
			}

			kubeObjects, metricsObjects, err := generateMockResources(config, bc.revisionWebhookPath, bc.sidecarWebhookPath)
			if err != nil {
				b.Fatalf("[%s] Failed to generate mock resources: %v", bc.name, err)
			}

			// Prepare fake clients
			fakeClient := fake.NewSimpleClientset(kubeObjects...)
			fakeMetricsClient := metricsfake.NewSimpleClientset(metricsObjects...)

			// Set up metrics client reactors (handle potential nil metrics list for specific namespaces)
			fakeMetricsClient.PrependReactor("list", "pods", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				listAction := action.(clienttesting.ListAction)
				ns := listAction.GetNamespace()

				// Filter metricsObjects for the requested namespace
				nsMetrics := &v1beta1.PodMetricsList{Items: []v1beta1.PodMetrics{}}
				// Need to capture metricsObjects for this specific benchmark run
				currentMetricsObjects := metricsObjects
				for _, obj := range currentMetricsObjects {
					if podMetrics, ok := obj.(*v1beta1.PodMetrics); ok {
						if podMetrics.Namespace == ns {
							nsMetrics.Items = append(nsMetrics.Items, *podMetrics)
						}
					}
				}
				return true, nsMetrics, nil
			})

			// Prepare ClusterInfo and Config for processNamespaces

			// Basic config for the benchmark
			processCfg := &utils.Config{
				KubeContext:    fmt.Sprintf("benchmark-context-%s", bc.name),
				ObfuscateNames: false,
				NoProgress:     true, // Disable progress bar during benchmark for cleaner console output
			}

			// Check if any namespace config requires metrics
			hasMetricsInConfig := false
			for _, nsConfig := range config.Namespaces {
				if nsConfig.HasMetrics {
					hasMetricsInConfig = true
					break
				}
			}

			startTime := time.Now()
			b.ResetTimer() // Start timing after setup for this specific case

			for i := 0; i < b.N; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

				clusterInfo := models.NewClusterInfo()

				err := processNamespaces(ctx, fakeClient, fakeMetricsClient, clusterInfo, processCfg, hasMetricsInConfig)

				cancel()

				if err != nil {
					// Stop the benchmark if an error occurs during processing
					b.Fatalf("[%s] Benchmark iteration %d failed: %v", bc.name, i, err)
				}
			}

			b.StopTimer() // Stop timing explicitly for this case
			results[bc.name] = benchmarkResult{
				name:                bc.name,
				namespacesProcessed: totalNamespaces,
				podsProcessed:       totalPods,
				duration:            time.Since(startTime),
			}
		})
	}

	// Print results
	fmt.Println("Benchmark Results:")
	for name, result := range results {
		fmt.Printf("%s:\n", name)
		fmt.Printf("  Namespaces Processed: %d\n", result.namespacesProcessed)
		fmt.Printf("  Pods Processed: %d\n", result.podsProcessed)
		fmt.Printf("  Duration: %s\n", result.duration)
	}
}
