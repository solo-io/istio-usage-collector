package models

// ClusterInfo represents the top level structure for a Kubernetes cluster
type ClusterInfo struct {
	Name       string                    `json:"name"`
	Namespaces map[string]*NamespaceInfo `json:"namespaces"`
	Nodes      map[string]NodeInfo       `json:"nodes"`
	HasMetrics bool                      `json:"has_metrics"`
}

// NamespaceInfo represents information about a Kubernetes namespace
type NamespaceInfo struct {
	Pods            int          `json:"pods"`
	IsIstioInjected bool         `json:"is_istio_injected"`
	Resources       ResourceInfo `json:"resources"`
}

// ResourceInfo represents resource information for a namespace
type ResourceInfo struct {
	Regular ContainerResources  `json:"regular"`
	Istio   *ContainerResources `json:"istio,omitempty"`
}

// ContainerResources represents a group of container resources
type ContainerResources struct {
	Containers int        `json:"containers"`
	Request    Resources  `json:"request"`
	Actual     *Resources `json:"actual,omitempty"`
}

// Resources represents resource specifications
type Resources struct {
	CPU      float64 `json:"cpu"`
	MemoryGB float64 `json:"memory_gb"`
}

// NodeInfo represents information about a Kubernetes node
type NodeInfo struct {
	InstanceType string        `json:"instance_type"`
	Region       string        `json:"region"`
	Zone         string        `json:"zone"`
	Resources    NodeResources `json:"resources"`
}

// NodeResources represents resource information for a node
type NodeResources struct {
	Capacity NodeResourceSpec  `json:"capacity"`
	Actual   *NodeResourceSpec `json:"actual,omitempty"`
}

// NodeResourceSpec represents resource specifications for a node
type NodeResourceSpec struct {
	CPU      float64 `json:"cpu"`
	MemoryGB float64 `json:"memory_gb"`
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

// SetActualNodeResources sets the actual resource usage for a NodeInfo
func (ni *NodeInfo) SetActualNodeResources(cpuUsage, memoryUsage float64) {
	ni.Resources.Actual = &NodeResourceSpec{
		CPU:      cpuUsage,
		MemoryGB: memoryUsage,
	}
}
