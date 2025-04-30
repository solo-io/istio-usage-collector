package models

// ClusterInfo represents the top level structure for a Kubernetes cluster
type ClusterInfo struct {
	Name       string                    `json:"name" yaml:"name"`
	Namespaces map[string]*NamespaceInfo `json:"namespaces" yaml:"namespaces"`
	Nodes      map[string]NodeInfo       `json:"nodes" yaml:"nodes"`
	HasMetrics bool                      `json:"has_metrics" yaml:"has_metrics"`
}

// NamespaceInfo represents information about a Kubernetes namespace
type NamespaceInfo struct {
	Pods int `json:"pods" yaml:"pods"`
	// IsIstioInjected is true if the namespace has istio injection enabled or if a pod within the namespace has istio injection enabled
	IsIstioInjected bool         `json:"is_istio_injected" yaml:"is_istio_injected"`
	Resources       ResourceInfo `json:"resources" yaml:"resources"`
}

// ResourceInfo represents resource information for a namespace
type ResourceInfo struct {
	Regular ContainerResources  `json:"regular" yaml:"regular"`
	Istio   *ContainerResources `json:"istio,omitempty" yaml:"istio,omitempty"`
}

// ContainerResources represents a group of container resources
type ContainerResources struct {
	Containers int        `json:"containers" yaml:"containers"`
	Request    Resources  `json:"request" yaml:"request"`
	Actual     *Resources `json:"actual,omitempty" yaml:"actual,omitempty"`
}

// Resources represents resource specifications
type Resources struct {
	CPU      float64 `json:"cpu" yaml:"cpu"`
	MemoryGB float64 `json:"memory_gb" yaml:"memory_gb"`
}

// NodeInfo represents information about a Kubernetes node
type NodeInfo struct {
	InstanceType string        `json:"instance_type" yaml:"instance_type"`
	Region       string        `json:"region" yaml:"region"`
	Zone         string        `json:"zone" yaml:"zone"`
	Resources    NodeResources `json:"resources" yaml:"resources"`
}

// NodeResources represents resource information for a node
type NodeResources struct {
	Capacity NodeResourceSpec  `json:"capacity" yaml:"capacity"`
	Actual   *NodeResourceSpec `json:"actual,omitempty" yaml:"actual,omitempty"`
}

// NodeResourceSpec represents resource specifications for a node
type NodeResourceSpec struct {
	CPU      float64 `json:"cpu" yaml:"cpu"`
	MemoryGB float64 `json:"memory_gb" yaml:"memory_gb"`
}

// NewClusterInfo creates a new ClusterInfo with initialized maps
func NewClusterInfo() *ClusterInfo {
	return &ClusterInfo{
		Namespaces: make(map[string]*NamespaceInfo),
		Nodes:      make(map[string]NodeInfo),
	}
}

// NewNodeInfo creates a new NodeInfo
func NewNodeInfo(instanceType, region, zone string, cpuCapacity, memoryCapacity float64) NodeInfo {
	return NodeInfo{
		InstanceType: instanceType,
		Region:       region,
		Zone:         zone,
		Resources: NodeResources{
			Capacity: NodeResourceSpec{
				CPU:      cpuCapacity,
				MemoryGB: memoryCapacity,
			},
		},
	}
}
