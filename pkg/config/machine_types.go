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
			MinUpscaleInterval:   "0s", // No interval restriction
			MinDownscaleInterval: "0s", // No interval restriction
			DowntimeOnScale:      true, // Always causes downtime
		}
	default:
		// Default to Enterprise constraints (more restrictive)
		return ScalingConstraints{
			MinUpscaleInterval:   "0s",
			MinDownscaleInterval: "0s",
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
	mt, exists := MachineTypeRegistry[name]
	if !exists {
		return MachineType{}, fmt.Errorf("machine type %s not found", name)
	}
	return mt, nil
}

// GetNextLargerMachineType returns the next larger machine type in the same series/tier
func GetNextLargerMachineType(currentType string) (string, error) {
	current, err := GetMachineType(currentType)
	if err != nil {
		return "", err
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
