package config

import (
	"fmt"
	"strings"
)

// Edition represents the Cloud SQL edition type
type Edition string

const (
	EditionEnterprise     Edition = "ENTERPRISE"
	EditionEnterprisePlus Edition = "ENTERPRISE_PLUS"
)

// MachineType represents a Cloud SQL machine type configuration
type MachineType struct {
	Name     string
	CPU      int     // Number of vCPUs
	MemoryGB float64 // Memory in GB
	Series   string  // Machine series (e.g., "n1", "n2", "e2")
	Tier     string  // Size tier (e.g., "micro", "small", "standard", "highmem")
}

// ScalingConstraints defines the constraints for scaling operations
type ScalingConstraints struct {
	MinUpscaleInterval   string // Minimum interval between upscale operations
	MinDownscaleInterval string // Minimum interval between downscale operations
	DowntimeOnScale      bool   // Whether scaling causes downtime
}

// GetScalingConstraints returns scaling constraints based on edition
func GetScalingConstraints(edition Edition) ScalingConstraints {
	switch edition {
	case EditionEnterprisePlus:
		return ScalingConstraints{
			MinUpscaleInterval:   "30m",
			MinDownscaleInterval: "3h",
			DowntimeOnScale:      false, // Near-zero downtime within intervals
		}
	case EditionEnterprise:
		return ScalingConstraints{
			MinUpscaleInterval:   "6h", // No interval restriction
			MinDownscaleInterval: "6h", // No interval restriction
			DowntimeOnScale:      true, // Always causes downtime
		}
	default:
		// Default to Enterprise constraints (more restrictive)
		return ScalingConstraints{
			MinUpscaleInterval:   "24h",
			MinDownscaleInterval: "24h",
			DowntimeOnScale:      true,
		}
	}
}

// MachineTypeRegistry holds all available Cloud SQL machine types
var MachineTypeRegistry = map[string]MachineType{
	// Shared-core machine types
	"db-f1-micro": {Name: "db-f1-micro", CPU: 1, MemoryGB: 0.6, Series: "f1", Tier: "micro"},
	"db-g1-small": {Name: "db-g1-small", CPU: 1, MemoryGB: 1.7, Series: "g1", Tier: "small"},

	// N1 Series - Standard
	"db-n1-standard-1":  {Name: "db-n1-standard-1", CPU: 1, MemoryGB: 3.75, Series: "n1", Tier: "standard"},
	"db-n1-standard-2":  {Name: "db-n1-standard-2", CPU: 2, MemoryGB: 7.5, Series: "n1", Tier: "standard"},
	"db-n1-standard-4":  {Name: "db-n1-standard-4", CPU: 4, MemoryGB: 15, Series: "n1", Tier: "standard"},
	"db-n1-standard-8":  {Name: "db-n1-standard-8", CPU: 8, MemoryGB: 30, Series: "n1", Tier: "standard"},
	"db-n1-standard-16": {Name: "db-n1-standard-16", CPU: 16, MemoryGB: 60, Series: "n1", Tier: "standard"},
	"db-n1-standard-32": {Name: "db-n1-standard-32", CPU: 32, MemoryGB: 120, Series: "n1", Tier: "standard"},
	"db-n1-standard-64": {Name: "db-n1-standard-64", CPU: 64, MemoryGB: 240, Series: "n1", Tier: "standard"},
	"db-n1-standard-96": {Name: "db-n1-standard-96", CPU: 96, MemoryGB: 360, Series: "n1", Tier: "standard"},

	// N1 Series - High Memory
	"db-n1-highmem-2":  {Name: "db-n1-highmem-2", CPU: 2, MemoryGB: 13, Series: "n1", Tier: "highmem"},
	"db-n1-highmem-4":  {Name: "db-n1-highmem-4", CPU: 4, MemoryGB: 26, Series: "n1", Tier: "highmem"},
	"db-n1-highmem-8":  {Name: "db-n1-highmem-8", CPU: 8, MemoryGB: 52, Series: "n1", Tier: "highmem"},
	"db-n1-highmem-16": {Name: "db-n1-highmem-16", CPU: 16, MemoryGB: 104, Series: "n1", Tier: "highmem"},
	"db-n1-highmem-32": {Name: "db-n1-highmem-32", CPU: 32, MemoryGB: 208, Series: "n1", Tier: "highmem"},
	"db-n1-highmem-64": {Name: "db-n1-highmem-64", CPU: 64, MemoryGB: 416, Series: "n1", Tier: "highmem"},
	"db-n1-highmem-96": {Name: "db-n1-highmem-96", CPU: 96, MemoryGB: 624, Series: "n1", Tier: "highmem"},

	// N2 Series - Standard
	"db-n2-standard-2":   {Name: "db-n2-standard-2", CPU: 2, MemoryGB: 8, Series: "n2", Tier: "standard"},
	"db-n2-standard-4":   {Name: "db-n2-standard-4", CPU: 4, MemoryGB: 16, Series: "n2", Tier: "standard"},
	"db-n2-standard-8":   {Name: "db-n2-standard-8", CPU: 8, MemoryGB: 32, Series: "n2", Tier: "standard"},
	"db-n2-standard-16":  {Name: "db-n2-standard-16", CPU: 16, MemoryGB: 64, Series: "n2", Tier: "standard"},
	"db-n2-standard-32":  {Name: "db-n2-standard-32", CPU: 32, MemoryGB: 128, Series: "n2", Tier: "standard"},
	"db-n2-standard-48":  {Name: "db-n2-standard-48", CPU: 48, MemoryGB: 192, Series: "n2", Tier: "standard"},
	"db-n2-standard-64":  {Name: "db-n2-standard-64", CPU: 64, MemoryGB: 256, Series: "n2", Tier: "standard"},
	"db-n2-standard-80":  {Name: "db-n2-standard-80", CPU: 80, MemoryGB: 320, Series: "n2", Tier: "standard"},
	"db-n2-standard-96":  {Name: "db-n2-standard-96", CPU: 96, MemoryGB: 384, Series: "n2", Tier: "standard"},
	"db-n2-standard-128": {Name: "db-n2-standard-128", CPU: 128, MemoryGB: 512, Series: "n2", Tier: "standard"},

	// N2 Series - High Memory
	"db-n2-highmem-2":   {Name: "db-n2-highmem-2", CPU: 2, MemoryGB: 16, Series: "n2", Tier: "highmem"},
	"db-n2-highmem-4":   {Name: "db-n2-highmem-4", CPU: 4, MemoryGB: 32, Series: "n2", Tier: "highmem"},
	"db-n2-highmem-8":   {Name: "db-n2-highmem-8", CPU: 8, MemoryGB: 64, Series: "n2", Tier: "highmem"},
	"db-n2-highmem-16":  {Name: "db-n2-highmem-16", CPU: 16, MemoryGB: 128, Series: "n2", Tier: "highmem"},
	"db-n2-highmem-32":  {Name: "db-n2-highmem-32", CPU: 32, MemoryGB: 256, Series: "n2", Tier: "highmem"},
	"db-n2-highmem-48":  {Name: "db-n2-highmem-48", CPU: 48, MemoryGB: 384, Series: "n2", Tier: "highmem"},
	"db-n2-highmem-64":  {Name: "db-n2-highmem-64", CPU: 64, MemoryGB: 512, Series: "n2", Tier: "highmem"},
	"db-n2-highmem-80":  {Name: "db-n2-highmem-80", CPU: 80, MemoryGB: 640, Series: "n2", Tier: "highmem"},
	"db-n2-highmem-96":  {Name: "db-n2-highmem-96", CPU: 96, MemoryGB: 768, Series: "n2", Tier: "highmem"},
	"db-n2-highmem-128": {Name: "db-n2-highmem-128", CPU: 128, MemoryGB: 864, Series: "n2", Tier: "highmem"},

	// E2 Series - Standard (Cost-optimized)
	"db-e2-standard-2":  {Name: "db-e2-standard-2", CPU: 2, MemoryGB: 8, Series: "e2", Tier: "standard"},
	"db-e2-standard-4":  {Name: "db-e2-standard-4", CPU: 4, MemoryGB: 16, Series: "e2", Tier: "standard"},
	"db-e2-standard-8":  {Name: "db-e2-standard-8", CPU: 8, MemoryGB: 32, Series: "e2", Tier: "standard"},
	"db-e2-standard-16": {Name: "db-e2-standard-16", CPU: 16, MemoryGB: 64, Series: "e2", Tier: "standard"},
	"db-e2-standard-32": {Name: "db-e2-standard-32", CPU: 32, MemoryGB: 128, Series: "e2", Tier: "standard"},

	// E2 Series - High Memory
	"db-e2-highmem-2":  {Name: "db-e2-highmem-2", CPU: 2, MemoryGB: 16, Series: "e2", Tier: "highmem"},
	"db-e2-highmem-4":  {Name: "db-e2-highmem-4", CPU: 4, MemoryGB: 32, Series: "e2", Tier: "highmem"},
	"db-e2-highmem-8":  {Name: "db-e2-highmem-8", CPU: 8, MemoryGB: 64, Series: "e2", Tier: "highmem"},
	"db-e2-highmem-16": {Name: "db-e2-highmem-16", CPU: 16, MemoryGB: 128, Series: "e2", Tier: "highmem"},
}

// GetMachineType returns a machine type by name
func GetMachineType(name string) (MachineType, error) {
	// Check registry first
	mt, exists := MachineTypeRegistry[name]
	if exists {
		return mt, nil
	}

	// Try to parse custom machine type
	if customMT, err := parseCustomMachineType(name); err == nil {
		return customMT, nil
	}

	// Try to parse performance-optimized machine type
	if perfMT, err := parsePerformanceOptimizedMachineType(name); err == nil {
		return perfMT, nil
	}

	return MachineType{}, fmt.Errorf("machine type %s not found", name)
}

// GetNextLargerMachineType returns the next larger machine type in the same series/tier
func GetNextLargerMachineType(currentType string) (string, error) {
	current, err := GetMachineType(currentType)
	if err != nil {
		return "", err
	}

	// Handle custom machine types
	if current.Series == "custom" {
		return getNextCustomMachineType(current, true)
	}

	// Handle performance-optimized types
	if current.Series == "perf-optimized" {
		return getNextPerformanceOptimizedType(current, true)
	}

	var candidates []MachineType
	for _, mt := range MachineTypeRegistry {
		// Same series and tier, but more resources
		if mt.Series == current.Series && mt.Tier == current.Tier {
			if mt.CPU > current.CPU || mt.MemoryGB > current.MemoryGB {
				candidates = append(candidates, mt)
			}
		}
	}

	// Find the smallest upgrade
	var next *MachineType
	for i := range candidates {
		if next == nil || (candidates[i].CPU < next.CPU && candidates[i].MemoryGB >= current.MemoryGB) {
			next = &candidates[i]
		}
	}

	if next == nil {
		return "", fmt.Errorf("no larger machine type available for %s", currentType)
	}

	return next.Name, nil
}

// GetNextSmallerMachineType returns the next smaller machine type in the same series/tier
func GetNextSmallerMachineType(currentType string) (string, error) {
	current, err := GetMachineType(currentType)
	if err != nil {
		return "", err
	}

	// Handle custom machine types
	if current.Series == "custom" {
		return getNextCustomMachineType(current, false)
	}

	// Handle performance-optimized types
	if current.Series == "perf-optimized" {
		return getNextPerformanceOptimizedType(current, false)
	}

	var candidates []MachineType
	for _, mt := range MachineTypeRegistry {
		// Same series and tier, but fewer resources
		if mt.Series == current.Series && mt.Tier == current.Tier {
			if mt.CPU < current.CPU && mt.MemoryGB < current.MemoryGB {
				candidates = append(candidates, mt)
			}
		}
	}

	// Find the largest downgrade
	var next *MachineType
	for i := range candidates {
		if next == nil || (candidates[i].CPU > next.CPU) {
			next = &candidates[i]
		}
	}

	if next == nil {
		return "", fmt.Errorf("no smaller machine type available for %s", currentType)
	}

	return next.Name, nil
}

// ParseEdition converts a string to Edition type
func ParseEdition(s string) Edition {
	switch strings.ToUpper(s) {
	case "ENTERPRISE_PLUS":
		return EditionEnterprisePlus
	case "ENTERPRISE":
		return EditionEnterprise
	default:
		return EditionEnterprise // Default to Enterprise for safety
	}
}

// parseCustomMachineType parses custom machine types like "db-custom-4-16384"
func parseCustomMachineType(name string) (MachineType, error) {
	if !strings.HasPrefix(name, "db-custom-") {
		return MachineType{}, fmt.Errorf("not a custom machine type")
	}

	parts := strings.Split(strings.TrimPrefix(name, "db-custom-"), "-")
	if len(parts) != 2 {
		return MachineType{}, fmt.Errorf("invalid custom machine type format")
	}

	var cpu int
	var memoryMB int
	if _, err := fmt.Sscanf(parts[0], "%d", &cpu); err != nil {
		return MachineType{}, fmt.Errorf("invalid CPU count: %v", err)
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &memoryMB); err != nil {
		return MachineType{}, fmt.Errorf("invalid memory size: %v", err)
	}

	// Validate custom machine type constraints
	if cpu < 1 || cpu > 96 {
		return MachineType{}, fmt.Errorf("custom machine type CPU must be between 1 and 96")
	}

	memoryGB := float64(memoryMB) / 1024.0
	minMemoryGB := float64(cpu) * 0.9 // Minimum 0.9 GB per vCPU
	maxMemoryGB := float64(cpu) * 6.5 // Maximum 6.5 GB per vCPU

	if memoryGB < minMemoryGB || memoryGB > maxMemoryGB {
		return MachineType{}, fmt.Errorf("custom machine type memory must be between %.1f GB and %.1f GB for %d vCPUs",
			minMemoryGB, maxMemoryGB, cpu)
	}

	// Determine tier based on memory per CPU ratio
	memoryPerCPU := memoryGB / float64(cpu)
	tier := "standard"
	if memoryPerCPU > 4.0 {
		tier = "highmem"
	}

	return MachineType{
		Name:     name,
		CPU:      cpu,
		MemoryGB: memoryGB,
		Series:   "custom",
		Tier:     tier,
	}, nil
}

// parsePerformanceOptimizedMachineType parses performance-optimized types like "db-perf-optimized-N-2"
func parsePerformanceOptimizedMachineType(name string) (MachineType, error) {
	if !strings.HasPrefix(name, "db-perf-optimized-") {
		return MachineType{}, fmt.Errorf("not a performance-optimized machine type")
	}

	// Extract the size suffix (e.g., "N-2" from "db-perf-optimized-N-2")
	suffix := strings.TrimPrefix(name, "db-perf-optimized-")

	// Performance-optimized instances have specific configurations
	// Based on GCP documentation, these are high-performance instances
	switch suffix {
	case "N-2":
		return MachineType{
			Name:     name,
			CPU:      2,
			MemoryGB: 16, // High memory ratio for performance
			Series:   "perf-optimized",
			Tier:     "performance",
		}, nil
	case "N-4":
		return MachineType{
			Name:     name,
			CPU:      4,
			MemoryGB: 32,
			Series:   "perf-optimized",
			Tier:     "performance",
		}, nil
	case "N-8":
		return MachineType{
			Name:     name,
			CPU:      8,
			MemoryGB: 64,
			Series:   "perf-optimized",
			Tier:     "performance",
		}, nil
	case "N-16":
		return MachineType{
			Name:     name,
			CPU:      16,
			MemoryGB: 128,
			Series:   "perf-optimized",
			Tier:     "performance",
		}, nil
	default:
		return MachineType{}, fmt.Errorf("unknown performance-optimized type: %s", suffix)
	}
}

// getNextCustomMachineType calculates the next custom machine type
func getNextCustomMachineType(current MachineType, scaleUp bool) (string, error) {
	currentCPU := current.CPU
	currentMemoryMB := int(current.MemoryGB * 1024)

	var nextCPU int
	var nextMemoryMB int

	if scaleUp {
		// For scaling up, increase resources by ~50%
		nextCPU = currentCPU
		nextMemoryMB = currentMemoryMB

		// Try to increase CPU first if we're CPU constrained
		cpuUtilRatio := float64(currentMemoryMB) / float64(currentCPU) / 1024.0
		if cpuUtilRatio > 4.0 {
			// Memory heavy, increase CPU
			if currentCPU < 96 {
				nextCPU = min(currentCPU+max(1, currentCPU/2), 96)
			}
		} else {
			// Balanced or CPU heavy, increase memory
			nextMemoryMB = currentMemoryMB + max(1024, currentMemoryMB/2)
		}

		// If we can't increase one dimension, try the other
		if nextCPU == currentCPU && nextMemoryMB == currentMemoryMB {
			if currentCPU < 96 {
				nextCPU = currentCPU + 1
			}
			nextMemoryMB = currentMemoryMB + 1024
		}
	} else {
		// For scaling down, decrease resources by ~33%
		nextCPU = max(1, currentCPU-max(1, currentCPU/3))
		nextMemoryMB = max(1024, currentMemoryMB-max(1024, currentMemoryMB/3))
	}

	// Validate the new configuration
	memoryGB := float64(nextMemoryMB) / 1024.0
	minMemoryGB := float64(nextCPU) * 0.9
	maxMemoryGB := float64(nextCPU) * 6.5

	// Adjust memory to fit constraints
	if memoryGB < minMemoryGB {
		nextMemoryMB = int(minMemoryGB * 1024)
	} else if memoryGB > maxMemoryGB {
		nextMemoryMB = int(maxMemoryGB * 1024)
	}

	// Round memory to nearest 256MB for cleaner values
	nextMemoryMB = (nextMemoryMB + 128) / 256 * 256

	if nextCPU == currentCPU && nextMemoryMB == currentMemoryMB {
		if scaleUp {
			return "", fmt.Errorf("already at maximum custom configuration")
		}
		return "", fmt.Errorf("already at minimum custom configuration")
	}

	return fmt.Sprintf("db-custom-%d-%d", nextCPU, nextMemoryMB), nil
}

// getNextPerformanceOptimizedType returns next performance-optimized type
func getNextPerformanceOptimizedType(current MachineType, scaleUp bool) (string, error) {
	// Define the sequence of performance-optimized types
	sequence := []string{"N-2", "N-4", "N-8", "N-16"}
	cpuMap := map[string]int{"N-2": 2, "N-4": 4, "N-8": 8, "N-16": 16}

	// Find current position
	currentSuffix := ""
	for suffix, cpu := range cpuMap {
		if cpu == current.CPU {
			currentSuffix = suffix
			break
		}
	}

	if currentSuffix == "" {
		return "", fmt.Errorf("unknown performance-optimized configuration")
	}

	// Find current index
	currentIdx := -1
	for i, suffix := range sequence {
		if suffix == currentSuffix {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		return "", fmt.Errorf("invalid performance-optimized type")
	}

	// Get next type
	var nextIdx int
	if scaleUp {
		nextIdx = currentIdx + 1
		if nextIdx >= len(sequence) {
			return "", fmt.Errorf("already at maximum performance-optimized size")
		}
	} else {
		nextIdx = currentIdx - 1
		if nextIdx < 0 {
			return "", fmt.Errorf("already at minimum performance-optimized size")
		}
	}

	return fmt.Sprintf("db-perf-optimized-%s", sequence[nextIdx]), nil
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
