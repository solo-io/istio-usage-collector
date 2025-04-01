package gatherer

import (
	"math"

	"k8s.io/apimachinery/pkg/api/resource"
)

// CPUToFloat64 converts Kubernetes CPU resource.Quantity to float64
func CPUToFloat64(cpu string) float64 {
	if cpu == "" {
		return 0
	}

	quantity, err := resource.ParseQuantity(cpu)
	if err != nil {
		return 0
	}

	return float64(quantity.MilliValue()) / 1000.0
}

// MemoryToGB converts Kubernetes memory resource.Quantity to gigabytes as float64
func MemoryToGB(memory string) float64 {
	if memory == "" {
		return 0
	}

	quantity, err := resource.ParseQuantity(memory)
	if err != nil {
		return 0
	}

	// Convert to GB
	return float64(quantity.Value()) / (1024 * 1024 * 1024)
}

// FormatFloat formats a float to two decimal places
func FormatFloat(num float64) float64 {
	return math.Round(num*100) / 100
}
