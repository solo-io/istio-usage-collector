//go:build test || unit

package gatherer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/solo-io/istio-usage-collector/pkg/models"
	"sigs.k8s.io/yaml"

	testutils "github.com/solo-io/istio-usage-collector/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	v1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestProcessNamespace(t *testing.T) {
	ctx := context.Background()

	// these are the default webhooks that are created through `istioctl install`, without any additional configuration
	var istioRevisionTagDefaultWebhook admissionregistrationv1.MutatingWebhookConfiguration
	data, err := os.ReadFile("../../tests/data/default-istio-revision-tag-mwh.yaml")
	require.NoError(t, err)
	err = yaml.Unmarshal(data, &istioRevisionTagDefaultWebhook)
	require.NoError(t, err)

	var istioSidecarInjectorWebhook admissionregistrationv1.MutatingWebhookConfiguration
	data, err = os.ReadFile("../../tests/data/default-istio-sidecar-injector-mwh.yaml")
	require.NoError(t, err)
	err = yaml.Unmarshal(data, &istioSidecarInjectorWebhook)
	require.NoError(t, err)

	defaultMutatingWebhookList := admissionregistrationv1.MutatingWebhookConfigurationList{Items: []admissionregistrationv1.MutatingWebhookConfiguration{istioRevisionTagDefaultWebhook, istioSidecarInjectorWebhook}}

	tests := []struct {
		name           string
		namespace      string
		kubeObjects    []runtime.Object // Namespaces, Pods
		metricsObjects []runtime.Object // PodMetrics
		hasMetricsAPI  bool
		expectedNsInfo *models.NamespaceInfo
		expectError    bool
	}{
		{
			name:      "Namespace without istio injection, no pods, metrics enabled",
			namespace: "default",
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			},
			metricsObjects: []runtime.Object{},
			hasMetricsAPI:  true,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            0,
				IsIstioInjected: false,
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{
						Containers: 0,
						Request:    models.Resources{CPU: 0, MemoryGB: 0},
						Actual:     &models.Resources{CPU: 0, MemoryGB: 0}, // Actual is present but zero when metrics enabled
					},
					Istio: nil, // No istio injection
				},
			},
			expectError: false,
		},
		{
			name:      "Namespace with istio injection label, pods, metrics enabled",
			namespace: "test-istio",
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-istio", Labels: map[string]string{"istio-injection": "enabled"}}},
				testutils.NewPod("test-istio", "pod-1", "node-a", "200m", "256Mi", true, "100m", "128Mi", map[string]string{}),
				testutils.NewPod("test-istio", "pod-2", "node-a", "100m", "64Mi", true, "100m", "128Mi", map[string]string{}),
			},
			metricsObjects: []runtime.Object{
				testutils.NewPodMetrics("test-istio", "pod-1", "150m", "180Mi", true, "50m", "64Mi"),
				testutils.NewPodMetrics("test-istio", "pod-2", "50m", "40Mi", true, "50m", "64Mi"),
			},
			hasMetricsAPI: true,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            2,
				IsIstioInjected: true,
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{
						Containers: 2,                                                              // pod-1 app + pod-2 app
						Request:    models.Resources{CPU: 0.3, MemoryGB: (256.0 + 64.0) / 1024.0},  // 200m + 100m, 256Mi + 64Mi
						Actual:     &models.Resources{CPU: 0.2, MemoryGB: (180.0 + 40.0) / 1024.0}, // 150m + 50m, 180Mi + 40Mi
					},
					Istio: &models.ContainerResources{
						Containers: 2,                                                              // pod-1 istio-proxy + pod-2 istio-proxy
						Request:    models.Resources{CPU: 0.2, MemoryGB: (128.0 + 128.0) / 1024.0}, // 100m + 100m, 128Mi + 128Mi
						Actual:     &models.Resources{CPU: 0.1, MemoryGB: (64.0 + 64.0) / 1024.0},  // 50m + 50m, 64Mi + 64Mi
					},
				},
			},
			expectError: false,
		},
		{
			name:      "Namespace with istio rev label, pods, no metrics api",
			namespace: "test-rev",
			kubeObjects: []runtime.Object{
				// istio.io/rev is set to "default" so that it matches the automatic istio injection (defined in the default-istio-sidecar-injector-mwh.yaml)
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-rev", Labels: map[string]string{"istio.io/rev": "default"}}},
				testutils.NewPod("test-rev", "pod-rev", "node-b", "100m", "256Mi", true, "50m", "50Mi", map[string]string{}),
			},
			metricsObjects: []runtime.Object{}, // No metrics objects available
			hasMetricsAPI:  false,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            1,
				IsIstioInjected: true,
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{
						Containers: 1,
						Request:    models.Resources{CPU: 0.1, MemoryGB: 256.0 / 1024.0},
						Actual:     nil, // No metrics API
					},
					Istio: &models.ContainerResources{
						Containers: 1,
						Request:    models.Resources{CPU: 0.05, MemoryGB: 50.0 / 1024.0}, // From helper func defaults
						Actual:     nil,                                                  // No metrics API
					},
				},
			},
			expectError: false,
		},
		{
			name:           "Namespace lookup fails",
			namespace:      "non-existent",
			kubeObjects:    []runtime.Object{&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "existent"}}},
			metricsObjects: []runtime.Object{},
			hasMetricsAPI:  true,
			expectedNsInfo: nil,
			expectError:    true, // Error getting namespace details
		},
		{
			name:      "Namespace with no istio injection label, no pods, metrics enabled",
			namespace: "default",
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			},
			metricsObjects: []runtime.Object{},
			hasMetricsAPI:  true,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            0,
				IsIstioInjected: false,
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{Containers: 0, Request: models.Resources{CPU: 0, MemoryGB: 0}, Actual: &models.Resources{CPU: 0, MemoryGB: 0}},
					Istio:   nil,
				},
			},
			expectError: false,
		},
		{
			name:      "Namespace with istio injection label, no pods, metrics enabled",
			namespace: "test-istio",
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-istio", Labels: map[string]string{"istio-injection": "enabled"}}},
			},
			metricsObjects: []runtime.Object{},
			hasMetricsAPI:  true,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            0,
				IsIstioInjected: false, // this is false because no pods within the namespace have istio sidecar injected
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{Containers: 0, Request: models.Resources{CPU: 0, MemoryGB: 0}, Actual: &models.Resources{CPU: 0, MemoryGB: 0}},
					Istio:   nil,
				},
			},
			expectError: false,
		},
		{
			name:      "Namespace without istio injection label, pods without istio injection, metrics enabled",
			namespace: "test-istio",
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-istio"}},
				testutils.NewPod("test-istio", "pod-1", "node-a", "200m", "256Mi", false, "", "", map[string]string{}),
			},
			metricsObjects: []runtime.Object{
				testutils.NewPodMetrics("test-istio", "pod-1", "150m", "180Mi", true, "50m", "64Mi"),
			},
			hasMetricsAPI: true,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            1,
				IsIstioInjected: false,
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{
						Containers: 1,                                                      // pod-1 app
						Request:    models.Resources{CPU: 0.2, MemoryGB: 256.0 / 1024.0},   // 200m, 256Mi
						Actual:     &models.Resources{CPU: 0.15, MemoryGB: 180.0 / 1024.0}, // 150m, 180Mi
					},
				},
			},
			expectError: false,
		},
		{
			name:      "Namespace with istio injection disabled, pods without istio injection, metrics enabled",
			namespace: "test-istio",
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-istio", Labels: map[string]string{"istio-injection": "disabled"}}},
				testutils.NewPod("test-istio", "pod-1", "node-a", "200m", "256Mi", false, "", "", map[string]string{}),
			},
			metricsObjects: []runtime.Object{
				testutils.NewPodMetrics("test-istio", "pod-1", "150m", "180Mi", false, "", ""),
			},
			hasMetricsAPI: true,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            1,
				IsIstioInjected: false,
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{
						Containers: 1,                                                      // pod-1 app
						Request:    models.Resources{CPU: 0.2, MemoryGB: 256.0 / 1024.0},   // 200m, 256Mi
						Actual:     &models.Resources{CPU: 0.15, MemoryGB: 180.0 / 1024.0}, // 150m, 180Mi
					},
				},
			},
			expectError: false,
		},
		{
			name:      "Namespace without istio injection label, pod with istio injection enabled (sidecar.istio.io/inject=true), no metrics",
			namespace: "test-istio",
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-istio"}},
				testutils.NewPod("test-istio", "pod-1", "node-a", "200m", "256Mi", true, "100m", "128Mi", map[string]string{"sidecar.istio.io/inject": "true"}),
			},
			metricsObjects: []runtime.Object{},
			hasMetricsAPI:  false,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            1,
				IsIstioInjected: true, // should be true because while the namespace does not have istio injection enabled, a pod within the namespace has istio injection enabled
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{
						Containers: 1,
						Request:    models.Resources{CPU: 0.2, MemoryGB: 256.0 / 1024.0},
						Actual:     nil,
					},
					Istio: &models.ContainerResources{
						Containers: 1,
						Request:    models.Resources{CPU: 0.1, MemoryGB: 128.0 / 1024.0},
						Actual:     nil,
					},
				},
			},
			expectError: false,
		},
		{
			name:      "Namespace without istio injection label, pod with istio rev label, no metrics",
			namespace: "test-istio",
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-istio"}},
				testutils.NewPod("test-istio", "pod-1", "node-a", "200m", "256Mi", true, "100m", "128Mi", map[string]string{"istio.io/rev": "default"}),
			},
			metricsObjects: []runtime.Object{},
			hasMetricsAPI:  false,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            1,
				IsIstioInjected: true, // should be true because while the namespace does not have istio injection enabled, a pod within the namespace has istio injection enabled
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{
						Containers: 1,
						Request:    models.Resources{CPU: 0.2, MemoryGB: 256.0 / 1024.0},
						Actual:     nil,
					},
					Istio: &models.ContainerResources{
						Containers: 1,
						Request:    models.Resources{CPU: 0.1, MemoryGB: 128.0 / 1024.0},
						Actual:     nil,
					},
				},
			},
			expectError: false,
		},
		{
			name:      "Namespace with istio injection disabled label, pod with istio injection enabled (sidecar.istio.io/inject=true), no metrics",
			namespace: "test-istio",
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-istio", Labels: map[string]string{"istio-injection": "disabled"}}},
				// while the pod has istio injection enabled, the namespace has istio injection disabled, so the istio-proxy container will not be injected
				testutils.NewPod("test-istio", "pod-1", "node-a", "200m", "256Mi", false, "", "", map[string]string{"sidecar.istio.io/inject": "true"}),
			},
			metricsObjects: []runtime.Object{},
			hasMetricsAPI:  false,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            1,
				IsIstioInjected: false, // should be false because the namespace has istio injection explicitly disabled
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{
						Containers: 1,
						Request:    models.Resources{CPU: 0.2, MemoryGB: 256.0 / 1024.0},
						Actual:     nil,
					},
					Istio: nil,
				},
			},
			expectError: false,
		},
		{
			// in this test we have a pod with an 'istio-proxy' container within it. because the namespace has istio injection disabled, the 'istio-proxy' container is treated as a regular container
			name:      "Namespace with istio injection disabled label, one pod, unrelated 'istio-proxy' container, no metrics",
			namespace: "test-istio",
			kubeObjects: []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-istio", Labels: map[string]string{"istio-injection": "disabled"}}},
				testutils.NewPod("test-istio", "pod-1", "node-a", "200m", "256Mi", true, "100m", "128Mi", map[string]string{}),
			},
			metricsObjects: []runtime.Object{},
			hasMetricsAPI:  false,
			expectedNsInfo: &models.NamespaceInfo{
				Pods:            1,
				IsIstioInjected: false,
				Resources: models.ResourceInfo{
					Regular: models.ContainerResources{
						Containers: 2, // the pod has 2 'regular' containers, the app and the istio-proxy
						Request:    models.Resources{CPU: 0.3, MemoryGB: (256.0 + 128.0) / 1024.0},
						Actual:     nil,
					},
					Istio: nil,
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewSimpleClientset(tt.kubeObjects...)

			// The metrics fake client has some issues (or lack of documentation) with List(), so we are manually adding the metrics objects
			podMetricsList := &v1beta1.PodMetricsList{}
			// go through all tt.metricsObjects and convert to v1beta1.PodMetrics (if possible), else node metrics
			for _, obj := range tt.metricsObjects {
				if podMetrics, ok := obj.(*v1beta1.PodMetrics); ok {
					podMetricsList.Items = append(podMetricsList.Items, *podMetrics)
				}
			}
			fakeMetricsClient := metricsfake.NewSimpleClientset()
			fakeMetricsClient.PrependReactor("list", "pods", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				if podMetricsList == nil {
					return true, &v1beta1.PodMetricsList{}, nil
				}
				return true, podMetricsList, nil
			})

			nsInfo, err := processNamespace(ctx, fakeClient, fakeMetricsClient, tt.namespace, tt.hasMetricsAPI, &defaultMutatingWebhookList)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, nsInfo)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, nsInfo)
				require.NotNil(t, tt.expectedNsInfo)

				assert.Equal(t, tt.expectedNsInfo.Pods, nsInfo.Pods)
				assert.Equal(t, tt.expectedNsInfo.IsIstioInjected, nsInfo.IsIstioInjected)

				// Assert Regular resources
				assert.Equal(t, tt.expectedNsInfo.Resources.Regular.Containers, nsInfo.Resources.Regular.Containers)
				assert.InDelta(t, tt.expectedNsInfo.Resources.Regular.Request.CPU, nsInfo.Resources.Regular.Request.CPU, 0.001)
				assert.InDelta(t, tt.expectedNsInfo.Resources.Regular.Request.MemoryGB, nsInfo.Resources.Regular.Request.MemoryGB, 0.001)
				if tt.expectedNsInfo.Resources.Regular.Actual != nil {
					require.NotNil(t, nsInfo.Resources.Regular.Actual)
					assert.InDelta(t, tt.expectedNsInfo.Resources.Regular.Actual.CPU, nsInfo.Resources.Regular.Actual.CPU, 0.001)
					assert.InDelta(t, tt.expectedNsInfo.Resources.Regular.Actual.MemoryGB, nsInfo.Resources.Regular.Actual.MemoryGB, 0.001)
				} else {
					assert.Nil(t, nsInfo.Resources.Regular.Actual)
				}

				// Assert Istio resources
				if tt.expectedNsInfo.Resources.Istio != nil {
					require.NotNil(t, nsInfo.Resources.Istio)
					assert.Equal(t, tt.expectedNsInfo.Resources.Istio.Containers, nsInfo.Resources.Istio.Containers)
					assert.InDelta(t, tt.expectedNsInfo.Resources.Istio.Request.CPU, nsInfo.Resources.Istio.Request.CPU, 0.001)
					assert.InDelta(t, tt.expectedNsInfo.Resources.Istio.Request.MemoryGB, nsInfo.Resources.Istio.Request.MemoryGB, 0.001)
					if tt.expectedNsInfo.Resources.Istio.Actual != nil {
						require.NotNil(t, nsInfo.Resources.Istio.Actual)
						assert.InDelta(t, tt.expectedNsInfo.Resources.Istio.Actual.CPU, nsInfo.Resources.Istio.Actual.CPU, 0.001)
						assert.InDelta(t, tt.expectedNsInfo.Resources.Istio.Actual.MemoryGB, nsInfo.Resources.Istio.Actual.MemoryGB, 0.001)
					} else {
						assert.Nil(t, nsInfo.Resources.Istio.Actual)
					}
				} else {
					assert.Nil(t, nsInfo.Resources.Istio)
				}
			}
		})
	}
}

// TODO(infocus7): Create tests like above, but where the MWH had the setup where a user enabled namespace injection by default

func TestProcessNode(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		node             corev1.Node
		metricsObjects   []runtime.Object // NodeMetrics
		hasMetricsAPI    bool
		expectedNodeInfo models.NodeInfo
		expectError      bool
	}{
		{
			name: "Node with standard labels and metrics",
			node: *testutils.NewNode("node-1", "4", "16Gi", map[string]string{
				"node.kubernetes.io/instance-type": "m5.large",
				"topology.kubernetes.io/region":    "us-east-1",
				"topology.kubernetes.io/zone":      "us-east-1a",
			}),
			metricsObjects: []runtime.Object{
				testutils.NewNodeMetrics("node-1", "1500m", "8Gi"),
			},
			hasMetricsAPI: true,
			expectedNodeInfo: models.NodeInfo{
				InstanceType: "m5.large",
				Region:       "us-east-1",
				Zone:         "us-east-1a",
				Resources: models.NodeResources{
					Capacity: models.NodeResourceSpec{CPU: 4.0, MemoryGB: 16.0},
					Actual:   &models.NodeResourceSpec{CPU: 1.5, MemoryGB: 8.0},
				},
			},
			expectError: false,
		},
		{
			name: "Node with deprecated labels and metrics",
			node: *testutils.NewNode("node-1", "4", "16Gi", map[string]string{
				"beta.kubernetes.io/instance-type":         "t3.medium",
				"failure-domain.beta.kubernetes.io/region": "eu-west-1",
				"failure-domain.beta.kubernetes.io/zone":   "eu-west-1b",
			}),
			metricsObjects: []runtime.Object{
				testutils.NewNodeMetrics("node-1", "1500m", "8Gi"),
			},
			hasMetricsAPI: true,
			expectedNodeInfo: models.NodeInfo{
				InstanceType: "t3.medium",
				Region:       "eu-west-1",
				Zone:         "eu-west-1b",
				Resources: models.NodeResources{
					Capacity: models.NodeResourceSpec{CPU: 4.0, MemoryGB: 16.0},
					Actual:   &models.NodeResourceSpec{CPU: 1.5, MemoryGB: 8.0},
				},
			},
			expectError: false,
		},
		{
			name: "Node with labels but no metrics",
			node: *testutils.NewNode("node-1", "4", "16Gi", map[string]string{
				"node.kubernetes.io/instance-type": "m5.large",
				"topology.kubernetes.io/region":    "us-east-1",
				"topology.kubernetes.io/zone":      "us-east-1a",
			}),
			metricsObjects: []runtime.Object{},
			hasMetricsAPI:  false,
			expectedNodeInfo: models.NodeInfo{
				InstanceType: "m5.large",
				Region:       "us-east-1",
				Zone:         "us-east-1a",
				Resources: models.NodeResources{
					Capacity: models.NodeResourceSpec{CPU: 4.0, MemoryGB: 16.0},
					Actual:   nil,
				},
			},
			expectError: false,
		},
		{
			name:           "Node with missing labels (expect unknown) and metrics available but no metrics for node",
			node:           *testutils.NewNode("node-nolabels", "1000m", "2Gi", map[string]string{}),
			metricsObjects: []runtime.Object{}, // No metrics available for the fake client to return
			hasMetricsAPI:  true,               // API is present, but Get() will fail for "node-nolabels"
			expectedNodeInfo: models.NodeInfo{
				InstanceType: "unknown",
				Region:       "unknown",
				Zone:         "unknown",
				Resources: models.NodeResources{
					Capacity: models.NodeResourceSpec{CPU: 1.0, MemoryGB: 2.0},
					Actual:   nil,
				},
			},
			expectError: false, // processNode only logs warning for metrics error to allow for regular resources to be gathered
		},
		// TODO: Add tests for context cancellation, specific metrics errors (permanent vs transient - needs mock client interaction)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeMetricsList := &v1beta1.NodeMetricsList{}
			for _, obj := range tt.metricsObjects {
				if nodeMetrics, ok := obj.(*v1beta1.NodeMetrics); ok {
					nodeMetricsList.Items = append(nodeMetricsList.Items, *nodeMetrics)
				}
			}

			fakeMetricsClient := metricsfake.NewSimpleClientset()
			fakeMetricsClient.PrependReactor("get", "nodes", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				// return the gotten nodeMetrics based on the action
				name := action.(clienttesting.GetAction).GetName()
				for _, nodeMetrics := range nodeMetricsList.Items {
					if nodeMetrics.Name == name {
						return true, &nodeMetrics, nil
					}
				}
				return false, nil, fmt.Errorf("nodeMetrics not found")
			})

			nodeInfo, err := processNode(ctx, fakeMetricsClient, tt.node, tt.hasMetricsAPI)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedNodeInfo.InstanceType, nodeInfo.InstanceType)
				assert.Equal(t, tt.expectedNodeInfo.Region, nodeInfo.Region)
				assert.Equal(t, tt.expectedNodeInfo.Zone, nodeInfo.Zone)
				assert.InDelta(t, tt.expectedNodeInfo.Resources.Capacity.CPU, nodeInfo.Resources.Capacity.CPU, 0.001)
				assert.InDelta(t, tt.expectedNodeInfo.Resources.Capacity.MemoryGB, nodeInfo.Resources.Capacity.MemoryGB, 0.001)
				if tt.expectedNodeInfo.Resources.Actual != nil {
					require.NotNil(t, nodeInfo.Resources.Actual)
					assert.InDelta(t, tt.expectedNodeInfo.Resources.Actual.CPU, nodeInfo.Resources.Actual.CPU, 0.001)
					assert.InDelta(t, tt.expectedNodeInfo.Resources.Actual.MemoryGB, nodeInfo.Resources.Actual.MemoryGB, 0.001)
				} else {
					assert.Nil(t, nodeInfo.Resources.Actual)
				}
			}
		})
	}
}

func TestLoadExistingData(t *testing.T) {
	tempDir := t.TempDir()

	jsonData := `{
  "name": "test-cluster",
  "has_metrics": true,
  "namespaces": {
    "default": {
      "pods": 1,
      "is_istio_injected": false,
      "resources": {
        "regular": {
          "containers": 1,
          "request": {
            "cpu": 0.1,
            "memory_gb": 0.125
          },
          "actual": {
            "cpu": 0.05,
            "memory_gb": 0.06
          }
        }
      }
    }
  },
  "nodes": {
    "test-node": {
      "instance_type": "m5.large",
      "region": "us-east-1",
      "zone": "us-east-1a",
      "resources": {
        "capacity": {
          "cpu": 10,
          "memory_gb": 46.96
        },
        "actual": {
          "cpu": 0.115,
          "memory_gb": 1.36
        }
      }
    },
    "test-node-2": {
      "instance_type": "m5.xlarge",
      "region": "us-east-1",
      "zone": "us-east-1a",
      "resources": {
        "capacity": {
          "cpu": 20,
          "memory_gb": 80
        },
        "actual": {
          "cpu": 1.25,
          "memory_gb": 10
        }
      }
    }
  }
}`
	yamlData := `name: yaml-cluster
has_metrics: false
namespaces:
  kube-system:
    pods: 5
    is_istio_injected: false
    resources:
      regular:
        containers: 5
        request:
          cpu: 0.5
          memory_gb: 1.5
nodes:
  test-node:
    instance_type: m5.large
    region: us-east-1
    zone: us-east-1a
    resources:
      capacity:
        cpu: 10
        memory_gb: 46.96
`

	jsonFile := filepath.Join(tempDir, "data.json")
	require.NoError(t, os.WriteFile(jsonFile, []byte(jsonData), 0644))
	yamlFile := filepath.Join(tempDir, "data.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(yamlData), 0644))
	invalidFile := filepath.Join(tempDir, "invalid.txt")
	require.NoError(t, os.WriteFile(invalidFile, []byte("hello"), 0644))
	malformedFile := filepath.Join(tempDir, "malformed.json")
	require.NoError(t, os.WriteFile(malformedFile, []byte(`{"name":"bad"`), 0644))

	tests := []struct {
		name        string
		filePath    string
		expected    *models.ClusterInfo
		expectError bool
	}{
		{
			name:     "Load valid JSON",
			filePath: jsonFile,
			expected: &models.ClusterInfo{
				Name:       "test-cluster",
				HasMetrics: true,
				Namespaces: map[string]*models.NamespaceInfo{
					"default": {
						Pods:            1,
						IsIstioInjected: false,
						Resources: models.ResourceInfo{
							Regular: models.ContainerResources{
								Containers: 1,
								Request:    models.Resources{CPU: 0.1, MemoryGB: 0.125},
								Actual:     &models.Resources{CPU: 0.05, MemoryGB: 0.06},
							},
						},
					},
				},
				Nodes: map[string]models.NodeInfo{
					"test-node": {
						InstanceType: "m5.large",
						Region:       "us-east-1",
						Zone:         "us-east-1a",
						Resources: models.NodeResources{
							Capacity: models.NodeResourceSpec{CPU: 10, MemoryGB: 46.96},
							Actual:   &models.NodeResourceSpec{CPU: 0.115, MemoryGB: 1.36},
						},
					},
					"test-node-2": {
						InstanceType: "m5.xlarge",
						Region:       "us-east-1",
						Zone:         "us-east-1a",
						Resources: models.NodeResources{
							Capacity: models.NodeResourceSpec{CPU: 20, MemoryGB: 80},
							Actual:   &models.NodeResourceSpec{CPU: 1.25, MemoryGB: 10},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:     "Load valid YAML",
			filePath: yamlFile,
			expected: &models.ClusterInfo{
				Name:       "yaml-cluster",
				HasMetrics: false,
				Namespaces: map[string]*models.NamespaceInfo{
					"kube-system": {
						Pods:            5,
						IsIstioInjected: false,
						Resources: models.ResourceInfo{
							Regular: models.ContainerResources{
								Containers: 5,
								Request:    models.Resources{CPU: 0.5, MemoryGB: 1.5},
								// Actual is nil implicitly
							},
						},
					},
				},
				Nodes: map[string]models.NodeInfo{
					"test-node": {
						InstanceType: "m5.large",
						Region:       "us-east-1",
						Zone:         "us-east-1a",
						Resources: models.NodeResources{
							Capacity: models.NodeResourceSpec{CPU: 10, MemoryGB: 46.96},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:        "File not found",
			filePath:    filepath.Join(tempDir, "nonexistent.json"),
			expected:    nil,
			expectError: true,
		},
		{
			name:        "Unsupported extension",
			filePath:    invalidFile,
			expected:    nil,
			expectError: true,
		},
		{
			name:        "Malformed JSON",
			filePath:    malformedFile,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterInfo, err := loadExistingData(tt.filePath)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, clusterInfo)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, clusterInfo)

				// Deep comparison is tricky, compare key fields
				assert.Equal(t, tt.expected.Name, clusterInfo.Name)
				assert.Equal(t, tt.expected.HasMetrics, clusterInfo.HasMetrics)
				assert.Equal(t, len(tt.expected.Namespaces), len(clusterInfo.Namespaces))
				// Add more detailed comparison of nested structs if needed
				for name, expectedNs := range tt.expected.Namespaces {
					actualNs, ok := clusterInfo.Namespaces[name]
					assert.True(t, ok, "Namespace %s missing", name)
					if ok {
						// Compare nsInfo fields (using InDelta for floats)
						assert.Equal(t, expectedNs.Pods, actualNs.Pods)
						assert.Equal(t, expectedNs.IsIstioInjected, actualNs.IsIstioInjected)
						// ... compare resources ...
					}
				}
				assert.Equal(t, len(tt.expected.Nodes), len(clusterInfo.Nodes))

			}
		})
	}
}

// TestGetClusterName tests the getClusterName function
// Deeper obfuscation tests are in obfuscation_test.go
func TestGetClusterName(t *testing.T) {
	tests := []struct {
		name      string
		obfuscate bool
		expected  string
	}{
		{
			name:      "test-cluster",
			obfuscate: false,
			expected:  "test-cluster",
		},
		{
			name:      "test-cluster",
			obfuscate: true,
			expected:  "f069097ced1b3fc738e80c852c5a26267ef4504b7658044bb9fd1636ff545dcd", // Note: this is the full hex encoded hash of "test-cluster"
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := getClusterName(context.Background(), tt.name, tt.obfuscate)
			assert.NoError(t, err)

			expected := tt.expected
			if tt.obfuscate {
				expected = expected[:32] // the sha256 is truncated to 16 bytes, and hex encoded to 32. hard-coding in case we make this configurable.
			}
			assert.Equal(t, expected, actual)
		})
	}
}
