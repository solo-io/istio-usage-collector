package gatherer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/solo-io/istio-usage-collector/internal/logging"
	"github.com/solo-io/istio-usage-collector/internal/models"
	"github.com/solo-io/istio-usage-collector/internal/utils"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	v1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// GatherClusterInfo gathers information about the Kubernetes cluster
func GatherClusterInfo(ctx context.Context, cfg *utils.Config) error {
	// Initialize the cluster info
	clusterInfo := models.NewClusterInfo()

	// Create the output file path
	outputFile := filepath.Join(cfg.OutputDir, fmt.Sprintf("%s.%s", cfg.OutputFilePrefix, cfg.OutputFormat))

	// Check if we should load existing data
	if cfg.ContinueProcessing {
		logging.Info("Continuing from existing data file %s", outputFile)
		existingData, err := loadExistingData(outputFile)
		if err != nil {
			logging.Warn("Failed to load existing data: %v. Starting fresh.", err)
		} else {
			// verify that it is the same cluster
			name := cfg.KubeContext
			if cfg.ObfuscateNames {
				name = ObfuscateName(name)
			}

			if name != existingData.Name {
				logging.Error("Existing data is from a different cluster or name obfuscation is changed. Please delete the existing file and try again.")
				os.Exit(1)
			} else {
				clusterInfo = existingData
				logging.Info("Loaded existing data with %d namespaces", len(clusterInfo.Namespaces))
			}
		}
	}

	// Create Kubernetes clients
	clientset, metricsClient, hasMetrics, err := createKubernetesClients(ctx, cfg.KubeContext)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	// Get cluster name
	if clusterInfo.Name == "" {
		cluster, err := getClusterName(ctx, cfg.KubeContext, cfg.ObfuscateNames)
		if err != nil {
			return fmt.Errorf("failed to get cluster name: %w", err)
		}
		clusterInfo.Name = cluster
	}

	// Create a context with a timeout to ensure we don't get stuck forever
	// if the context is not properly cancelled elsewhere
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// Process namespaces concurrently
	logging.Info("Gathering namespace information")
	err = processNamespaces(ctxWithTimeout, clientset, metricsClient, clusterInfo, cfg, hasMetrics)
	if err != nil {
		if ctxWithTimeout.Err() != nil {
			return fmt.Errorf("namespace processing cancelled: %w", ctxWithTimeout.Err())
		}
		return fmt.Errorf("failed to process namespaces: %w", err)
	}

	// Process nodes concurrently
	logging.Info("Gathering node information")
	err = processNodes(ctxWithTimeout, clientset, metricsClient, clusterInfo, cfg, hasMetrics)
	if err != nil {
		if ctxWithTimeout.Err() != nil {
			return fmt.Errorf("node processing cancelled: %w", ctxWithTimeout.Err())
		}
		return fmt.Errorf("failed to process nodes: %w", err)
	}

	// Set metrics availability flag
	clusterInfo.HasMetrics = hasMetrics

	// Output to file
	err = saveClusterInfo(clusterInfo, outputFile)
	if err != nil {
		return fmt.Errorf("failed to save cluster info: %w", err)
	}

	return nil
}

// loadExistingData loads cluster info from an existing file - used for --continue flag
func loadExistingData(fileName string) (*models.ClusterInfo, error) {
	// Check if file exists
	_, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", fileName)
	}

	// Read file
	data, err := os.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	fileExt := filepath.Ext(fileName)

	// Initialize clusterInfo before unmarshaling
	clusterInfo := models.NewClusterInfo()

	switch fileExt {
	case ".json":
		err = json.Unmarshal(data, clusterInfo)
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, clusterInfo)
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", fileExt)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	return clusterInfo, nil
}

// processNodes processes all nodes in the cluster
func processNodes(ctx context.Context, clientset *kubernetes.Clientset, metricsClient *metricsv.Clientset, clusterInfo *models.ClusterInfo, cfg *utils.Config, hasMetrics bool) error {
	// Check if the context is cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Get all nodes
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	totalNodes := len(nodes.Items)
	if totalNodes == 0 {
		logging.Warn("No nodes found in cluster %s", cfg.KubeContext)
		return nil
	}

	// Set up progress tracking
	progress := logging.NewProgress("Processing nodes", totalNodes)
	logging.Info("Found %d nodes to process", totalNodes)

	var processedCount int32

	// Use a mutex for safe map updates
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create a context for cancellation
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Use a channel to collect errors from goroutines
	errorCh := make(chan error, totalNodes)

	// Use a semaphore to control concurrency
	concurrentLimit := runtime.NumCPU() * 2
	semaphore := make(chan struct{}, concurrentLimit)

	logging.Info("Processing nodes with up to %d concurrent requests", concurrentLimit)

	// Process each node
	for _, node := range nodes.Items {
		// Check if context is cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}

		outNodeName := node.Name
		if cfg.ObfuscateNames {
			outNodeName = ObfuscateName(node.Name)
		}

		// Check if we should skip this node if continuing
		if cfg.ContinueProcessing {
			if _, ok := clusterInfo.Nodes[outNodeName]; ok {
				// Increment counter but don't process this as it has already been processed
				atomic.AddInt32(&processedCount, 1)
				progress.Update(int(atomic.LoadInt32(&processedCount)))
				continue
			}
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(node corev1.Node, outName string) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			// Check if context is cancelled
			if workerCtx.Err() != nil {
				errorCh <- workerCtx.Err()
				return
			}

			// Process node
			nodeInfo, err := processNode(workerCtx, metricsClient, node, hasMetrics)

			// Update progress
			count := atomic.AddInt32(&processedCount, 1)
			progress.Update(int(count))

			if err != nil {
				logging.Warn("Failed to process node %s: %v", node.Name, err)
				errorCh <- fmt.Errorf("node %s: %w", node.Name, err)
				return
			}

			// Add node to cluster info with lock protection
			mu.Lock()
			clusterInfo.Nodes[outName] = nodeInfo
			mu.Unlock()
		}(node, outNodeName)
	}

	// Set up a goroutine to close the error channel when all workers finish
	go func() {
		wg.Wait()
		close(errorCh)
	}()

	// Collect errors from workers
	var errors []error
	for err := range errorCh {
		if err != context.Canceled {
			errors = append(errors, err)
		}
	}

	// Complete the progress bar
	progress.Complete()

	if len(errors) > 0 {
		return fmt.Errorf("encountered %d errors processing nodes", len(errors))
	}

	return nil
}

// processNamespaces processes all namespaces in the cluster in parallel
func processNamespaces(ctx context.Context, clientset *kubernetes.Clientset, metricsClient *metricsv.Clientset, clusterInfo *models.ClusterInfo, cfg *utils.Config, hasMetrics bool) error {
	// Add context checking for cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Get all namespaces
	namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	if len(namespaces.Items) == 0 {
		logging.Warn("No namespaces found in cluster %s", cfg.KubeContext)
		return nil
	}

	// Set up progress tracking
	totalNamespaces := len(namespaces.Items)
	progress := logging.NewProgress("Processing namespaces", totalNamespaces)
	logging.Info("Found %d namespaces to process", totalNamespaces)

	// Use an atomic counter for progress
	var processedCount int32

	// Use a mutex for safe map updates
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create a context that's cancellable for spawned goroutines
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Use a channel to collect errors from goroutines
	errorCh := make(chan error, totalNamespaces)

	// Use a semaphore to control concurrency based on system resources
	concurrentLimit := runtime.NumCPU() * 4
	semaphore := make(chan struct{}, concurrentLimit)

	logging.Info("Processing namespaces with up to %d concurrent requests", concurrentLimit)
	for _, ns := range namespaces.Items {
		// Check parent context for cancellation before spawning more goroutines
		if ctx.Err() != nil {
			return ctx.Err()
		}

		outNsName := ns.Name
		if cfg.ObfuscateNames {
			outNsName = ObfuscateName(ns.Name)
		}

		// Check if we should skip this namespace if continuing
		if cfg.ContinueProcessing {
			if _, ok := clusterInfo.Namespaces[outNsName]; ok {
				// Increment counter but don't process
				atomic.AddInt32(&processedCount, 1)
				progress.Update(int(atomic.LoadInt32(&processedCount)))
				continue
			}
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(namespace corev1.Namespace, outName string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// Check if context is cancelled
			if workerCtx.Err() != nil {
				errorCh <- workerCtx.Err()
				return
			}

			nsInfo, err := processNamespace(workerCtx, clientset, metricsClient, namespace.Name, hasMetrics)

			count := atomic.AddInt32(&processedCount, 1)
			progress.Update(int(count))

			if err != nil {
				logging.Warn("Failed to process namespace %s: %v", namespace.Name, err)
				errorCh <- fmt.Errorf("namespace %s: %w", namespace.Name, err)
				return
			}

			mu.Lock()
			clusterInfo.Namespaces[outName] = nsInfo
			mu.Unlock()
		}(ns, outNsName)
	}

	// Set up a goroutine to close the error channel when all workers finish
	go func() {
		wg.Wait()
		close(errorCh)
	}()

	var errors []error
	for err := range errorCh {
		if err != context.Canceled {
			errors = append(errors, err)
		}
	}

	progress.Complete()

	if len(errors) > 0 {
		return fmt.Errorf("encountered %d errors processing namespaces", len(errors))
	}

	return nil
}

// processNamespace processes an individual namespace
func processNamespace(ctx context.Context, clientset *kubernetes.Clientset, metricsClient *metricsv.Clientset, namespace string, hasMetrics bool) (*models.NamespaceInfo, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Check if namespace has Istio injection
	ns, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace details: %w", err)
	}

	isIstioInjected := false
	if value, ok := ns.Labels["istio-injection"]; ok && value == "enabled" {
		isIstioInjected = true
	} else if _, ok := ns.Labels["istio.io/rev"]; ok {
		isIstioInjected = true
	}

	// Get pods in the namespace
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Check context cancellation after pods API call
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var metricsData *v1beta1.PodMetricsList
	if hasMetrics && metricsClient != nil {
		// Get metrics in a safe way with retry logic
		metricsData, err = getMetricsWithRetries(ctx, metricsClient, namespace)
		if err != nil {
			// Just log a warning but continue - metrics are optional
			logging.Warn("Failed to get metrics for namespace %s: %v", namespace, err)
		}
	}

	// Check context cancellation after metrics API call
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Count regular and istio containers
	regularContainers := 0
	istioContainers := 0

	// Resource values for both types
	regularCpuRequest := 0.0
	regularMemRequest := 0.0
	istioCpuRequest := 0.0
	istioMemRequest := 0.0

	// Actual usage (from metrics API)
	regularCpuActual := 0.0
	regularMemActual := 0.0
	istioCpuActual := 0.0
	istioMemActual := 0.0

	// Process all pods
	for _, pod := range pods.Items {
		// Check each container
		for _, container := range pod.Spec.Containers {
			isIstioProxy := container.Name == "istio-proxy"

			// Count container types
			if isIstioProxy {
				istioContainers++
			} else {
				regularContainers++
			}

			// Get CPU request
			if container.Resources.Requests != nil {
				if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
					if isIstioProxy {
						istioCpuRequest += cpu.AsApproximateFloat64()
					} else {
						regularCpuRequest += cpu.AsApproximateFloat64()
					}
				}

				// Get memory request
				if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
					memInGiB := float64(mem.Value()) / (1024 * 1024 * 1024)
					if isIstioProxy {
						istioMemRequest += memInGiB
					} else {
						regularMemRequest += memInGiB
					}
				}
			}
		}
	}

	// Process metrics data if available
	if metricsData != nil {
		for _, podMetric := range metricsData.Items {
			for _, containerMetric := range podMetric.Containers {
				isIstioProxy := containerMetric.Name == "istio-proxy"

				// CPU usage
				cpuUsage := containerMetric.Usage.Cpu().AsApproximateFloat64()
				if isIstioProxy {
					istioCpuActual += cpuUsage
				} else {
					regularCpuActual += cpuUsage
				}

				// Memory usage in GB
				memUsage := float64(containerMetric.Usage.Memory().Value()) / (1024 * 1024 * 1024)
				if isIstioProxy {
					istioMemActual += memUsage
				} else {
					regularMemActual += memUsage
				}
			}
		}
	}

	// Create namespace info
	nsInfo := &models.NamespaceInfo{
		Pods:            len(pods.Items),
		IsIstioInjected: isIstioInjected,
		Resources: models.ResourceInfo{
			Regular: models.ContainerResources{
				Containers: regularContainers,
				Request: models.Resources{
					CPU:      regularCpuRequest,
					MemoryGB: regularMemRequest,
				},
			},
		},
	}

	if metricsData != nil {
		nsInfo.Resources.Regular.Actual = &models.Resources{
			CPU:      regularCpuActual,
			MemoryGB: regularMemActual,
		}
	}

	// Add Istio resources if the namespace has Istio injection
	if isIstioInjected {
		nsInfo.Resources.Istio = &models.ContainerResources{
			Containers: istioContainers,
			Request: models.Resources{
				CPU:      istioCpuRequest,
				MemoryGB: istioMemRequest,
			},
		}

		if metricsData != nil {
			nsInfo.Resources.Istio.Actual = &models.Resources{
				CPU:      istioCpuActual,
				MemoryGB: istioMemActual,
			}
		}
	}

	return nsInfo, nil
}

// getMetricsWithRetries gets metrics for all pods in a namespace with retry logic
func getMetricsWithRetries(ctx context.Context, metricsClient *metricsv.Clientset, namespace string) (*v1beta1.PodMetricsList, error) {
	var result *v1beta1.PodMetricsList
	var lastErr error

	// Define retry parameters
	maxRetries := 3
	retryDelay := 500 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Try to get metrics
		result, lastErr = metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
		if lastErr == nil {
			return result, nil
		}

		// Check if error is likely to be permanent (not found, forbidden, etc.)
		if errors.IsNotFound(lastErr) || errors.IsForbidden(lastErr) || errors.IsUnauthorized(lastErr) {
			logging.Warn("Permanent error getting metrics for namespace %s: %v", namespace, lastErr)
			return nil, lastErr
		}

		// Log the retry attempt
		logging.Warn("Failed to get metrics for namespace %s (attempt %d/%d): %v",
			namespace, attempt+1, maxRetries, lastErr)

		// Last attempt - don't sleep
		if attempt == maxRetries-1 {
			break
		}

		// Exponential backoff
		sleepTime := retryDelay * time.Duration(1<<uint(attempt))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sleepTime):
			// Continue with next attempt
		}
	}

	return nil, fmt.Errorf("failed to get metrics after %d attempts: %w", maxRetries, lastErr)
}

// createKubernetesClients creates Kubernetes clients for the specified context
func createKubernetesClients(ctx context.Context, kubeContext string) (*kubernetes.Clientset, *metricsv.Clientset, bool, error) {
	// Get kubeconfig path
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return nil, nil, false, fmt.Errorf("HOME environment variable not set")
		}
		kubeconfigPath = fmt.Sprintf("%s/.kube/config", home)
	}

	// Build config
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext}).ClientConfig()
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	// Increase QPS and burst to avoid client-side throttling
	config.QPS = 100
	config.Burst = 100

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	// Verify the connection
	_, err = clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to connect to Kubernetes API server: %w", err)
	}

	// Create metrics client
	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		// If the metrics API is not available for whatever reason, we return the regular clientset for usage
		logging.Warn("Failed to create metrics client: %v", err)
		return clientset, nil, false, nil
	}

	// Check if metrics API is available by calling the metrics API
	hasMetrics := false
	if metricsClient != nil {
		_, err := metricsClient.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{Limit: 1})
		hasMetrics = err == nil
		if hasMetrics {
			logging.Info("Metrics API available")
		} else {
			logging.Warn("Metrics API not available: %v", err)
		}
	}

	return clientset, metricsClient, hasMetrics, nil
}

// getClusterName gets the name of the current cluster
func getClusterName(ctx context.Context, kubeContext string, obfuscate bool) (string, error) {
	// Check if the context is cancelled
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// Get cluster name from context
	clusterName := kubeContext
	if obfuscate {
		clusterName = ObfuscateName(clusterName)
	}

	return clusterName, nil
}

// saveClusterInfo saves the cluster info to the specified format and location
func saveClusterInfo(clusterInfo *models.ClusterInfo, outputFile string) error {
	// Extract file extension if it exists
	fileExt := "json"
	if idx := strings.LastIndex(outputFile, "."); idx >= 0 {
		fileExt = strings.ToLower(outputFile[idx+1:])
	}

	// For JSON and YAML formats
	var data []byte
	var err error

	switch fileExt {
	case "json":
		// Create JSON data
		data, err = json.MarshalIndent(clusterInfo, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal cluster info to JSON: %w", err)
		}
	case "yaml", "yml":
		// Create YAML data
		data, err = yaml.Marshal(clusterInfo)
		if err != nil {
			return fmt.Errorf("failed to marshal cluster info to YAML: %w", err)
		}
	default:
		return fmt.Errorf("unsupported output format: %s", fileExt)
	}

	// Ensure parent directories exist
	dir := filepath.Dir(outputFile)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory %s: %w", dir, err)
		}
	}

	// Write to file
	err = os.WriteFile(outputFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	logging.Info("Saved cluster info to file: %s", outputFile)
	return nil
}

// processNode processes an individual node
func processNode(ctx context.Context, metricsClient *metricsv.Clientset, node corev1.Node, hasMetrics bool) (models.NodeInfo, error) {
	// Check if the context is cancelled
	if ctx.Err() != nil {
		return models.NodeInfo{}, ctx.Err()
	}

	labels := node.Labels

	// Extract instance type, region, and zone
	instanceType := labels["node.kubernetes.io/instance-type"]
	if instanceType == "" {
		instanceType = labels["beta.kubernetes.io/instance-type"]
	}
	if instanceType == "" {
		instanceType = "unknown"
	}

	region := labels["topology.kubernetes.io/region"]
	if region == "" {
		region = labels["failure-domain.beta.kubernetes.io/region"]
	}
	if region == "" {
		region = "unknown"
	}

	zone := labels["topology.kubernetes.io/zone"]
	if zone == "" {
		zone = labels["failure-domain.beta.kubernetes.io/zone"]
	}
	if zone == "" {
		zone = "unknown"
	}

	// Get CPU and memory capacity
	cpuCapacity := float64(node.Status.Capacity.Cpu().Value())
	memoryBytes := float64(node.Status.Capacity.Memory().Value())
	memoryGB := memoryBytes / (1024 * 1024 * 1024)

	// Create node info
	nodeInfo := models.NewNodeInfo(instanceType, region, zone, cpuCapacity, memoryGB)

	// Add metrics if available
	if hasMetrics && metricsClient != nil {
		// Get node metrics with retries
		nodeMetrics, err := getNodeMetricsWithRetries(ctx, metricsClient, node.Name)
		if err != nil {
			logging.Warn("Failed to get metrics for node %s: %v", node.Name, err)
		} else if nodeMetrics != nil {
			cpuUsage := nodeMetrics.Usage.Cpu().AsApproximateFloat64()
			memoryUsage := float64(nodeMetrics.Usage.Memory().Value()) / (1024 * 1024 * 1024)
			nodeInfo.SetActualNodeResources(cpuUsage, memoryUsage)
		}
	}

	return nodeInfo, nil
}

// getNodeMetricsWithRetries gets metrics for a node with retry logic
func getNodeMetricsWithRetries(ctx context.Context, metricsClient *metricsv.Clientset, nodeName string) (*v1beta1.NodeMetrics, error) {
	var result *v1beta1.NodeMetrics
	var lastErr error

	// Define retry parameters
	maxRetries := 3
	retryDelay := 500 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		result, lastErr = metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, nodeName, metav1.GetOptions{})
		if lastErr == nil {
			return result, nil
		}

		// Check if error is likely to be permanent
		if errors.IsNotFound(lastErr) || errors.IsForbidden(lastErr) || errors.IsUnauthorized(lastErr) {
			logging.Warn("Permanent error getting metrics for node %s: %v", nodeName, lastErr)
			return nil, lastErr
		}

		// Log the retry attempt
		logging.Warn("Failed to get metrics for node %s (attempt %d/%d): %v",
			nodeName, attempt+1, maxRetries, lastErr)

		// Last attempt - don't sleep
		if attempt == maxRetries-1 {
			break
		}

		// Exponential backoff
		sleepTime := retryDelay * time.Duration(1<<uint(attempt))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sleepTime):
			// Continue with next attempt
		}
	}

	return nil, fmt.Errorf("failed to get node metrics after %d attempts: %w", maxRetries, lastErr)
}
