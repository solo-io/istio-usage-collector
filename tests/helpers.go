package tests

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// Helper function to create a simple pod
func NewPod(namespace, name, nodeName string, cpuRequest, memRequest string, hasIstioProxy bool, istioProxyCpu, istioProxyMem string, labels map[string]string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{},
					},
				},
			},
		},
	}
	if cpuRequest != "" {
		pod.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cpuRequest)
	}
	if memRequest != "" {
		pod.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory] = resource.MustParse(memRequest)
	}

	if hasIstioProxy {
		istioContainer := corev1.Container{
			Name: "istio-proxy",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(istioProxyCpu),
					corev1.ResourceMemory: resource.MustParse(istioProxyMem),
				},
			},
		}
		pod.Spec.Containers = append(pod.Spec.Containers, istioContainer)
	}
	return pod
}

// Helper function to create pod metrics
func NewPodMetrics(namespace, name string, cpuUsage, memUsage string, hasIstioProxy bool, istioCpuUsage, istioMemUsage string) *v1beta1.PodMetrics {
	metrics := &v1beta1.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Containers: []v1beta1.ContainerMetrics{
			{
				Name: "app",
				Usage: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse(cpuUsage),
					corev1.ResourceMemory: resource.MustParse(memUsage),
				},
			},
		},
	}
	if hasIstioProxy {
		istioMetrics := v1beta1.ContainerMetrics{
			Name: "istio-proxy",
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(istioCpuUsage),
				corev1.ResourceMemory: resource.MustParse(istioMemUsage),
			},
		}
		metrics.Containers = append(metrics.Containers, istioMetrics)
	}
	return metrics
}

// Helper function to create a simple node
func NewNode(name, cpuCapacity, memCapacity string, labels map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpuCapacity),
				corev1.ResourceMemory: resource.MustParse(memCapacity),
			},
		},
	}
}

// Helper function to create node metrics
func NewNodeMetrics(name, cpuUsage, memUsage string) *v1beta1.NodeMetrics {
	return &v1beta1.NodeMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Usage: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpuUsage),
			corev1.ResourceMemory: resource.MustParse(memUsage),
		},
	}
}
