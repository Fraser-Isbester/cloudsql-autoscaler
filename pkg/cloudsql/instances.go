package cloudsql

import (
	"context"
	"fmt"
	"time"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

// ScalingDecision represents a scaling recommendation
type ScalingDecision struct {
	ShouldScale      bool
	CurrentType      string
	RecommendedType  string
	Reason           string
	DowntimeExpected bool
	DowntimeReason   string
	EstimatedSavings float64
	Metrics          *config.MetricsSummary
}

// CanScaleWithoutDowntime checks if an instance can be scaled without downtime
func (c *Client) CanScaleWithoutDowntime(ctx context.Context, instance *config.InstanceInfo, targetMachineType string, isUpscale bool) (bool, string) {
	// Enterprise edition always has downtime
	if instance.Edition == config.EditionEnterprise {
		return false, "Enterprise edition requires downtime for all scaling operations"
	}

	// For Enterprise Plus, check time constraints
	constraints := config.GetScalingConstraints(instance.Edition)

	// Get last scaling time
	lastScaled, err := c.GetLastScalingTime(ctx, instance.Name)
	if err != nil {
		// If we can't determine last scaling time, assume it's safe
		return true, ""
	}

	timeSinceLastScale := time.Since(lastScaled)

	if isUpscale {
		minInterval, _ := time.ParseDuration(constraints.MinUpscaleInterval)
		if timeSinceLastScale < minInterval {
			timeToWait := minInterval - timeSinceLastScale
			return false, fmt.Sprintf("Enterprise Plus requires %s between upscale operations. Wait %v more",
				constraints.MinUpscaleInterval, timeToWait.Round(time.Minute))
		}
	} else {
		minInterval, _ := time.ParseDuration(constraints.MinDownscaleInterval)
		if timeSinceLastScale < minInterval {
			timeToWait := minInterval - timeSinceLastScale
			return false, fmt.Sprintf("Enterprise Plus requires %s between downscale operations. Wait %v more",
				constraints.MinDownscaleInterval, timeToWait.Round(time.Minute))
		}
	}

	return true, ""
}

// ValidateScaling validates if a scaling operation is allowed
func ValidateScaling(instance *config.InstanceInfo, targetMachineType string) error {
	// Validate target machine type exists
	targetMT, err := config.GetMachineType(targetMachineType)
	if err != nil {
		return fmt.Errorf("invalid target machine type: %w", err)
	}

	currentMT, err := config.GetMachineType(instance.MachineType)
	if err != nil {
		return fmt.Errorf("invalid current machine type: %w", err)
	}

	// Check if it's actually a change
	if targetMachineType == instance.MachineType {
		return fmt.Errorf("target machine type is the same as current")
	}

	// Validate series compatibility (can't change series during scaling)
	if targetMT.Series != currentMT.Series {
		return fmt.Errorf("cannot change machine series from %s to %s during scaling",
			currentMT.Series, targetMT.Series)
	}

	// Check instance state
	if instance.State != "RUNNABLE" {
		return fmt.Errorf("instance is not in RUNNABLE state (current: %s)", instance.State)
	}

	return nil
}

// EstimateCostSavings estimates monthly cost savings for a scaling operation
func EstimateCostSavings(currentType, recommendedType string, region string) float64 {
	// This is a simplified estimation - in reality, you'd use GCP pricing API
	// or maintain a pricing table

	currentMT, _ := config.GetMachineType(currentType)
	recommendedMT, _ := config.GetMachineType(recommendedType)

	// Rough estimation based on CPU and memory
	// Actual pricing varies by region and commitment type
	cpuHourlyRate := 0.0475    // $/vCPU/hour (example)
	memoryHourlyRate := 0.0080 // $/GB/hour (example)

	currentMonthlyCost := (float64(currentMT.CPU)*cpuHourlyRate +
		currentMT.MemoryGB*memoryHourlyRate) * 24 * 30

	recommendedMonthlyCost := (float64(recommendedMT.CPU)*cpuHourlyRate +
		recommendedMT.MemoryGB*memoryHourlyRate) * 24 * 30

	return currentMonthlyCost - recommendedMonthlyCost
}
