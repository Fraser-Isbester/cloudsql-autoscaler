package rules

import (
	"fmt"
	"time"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/cloudsql"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

// Engine is the scaling rules engine
type Engine struct {
	config *config.Config
}

// NewEngine creates a new scaling rules engine
func NewEngine(cfg *config.Config) *Engine {
	return &Engine{
		config: cfg,
	}
}

// AnalyzeInstance analyzes an instance and provides scaling recommendations
func (e *Engine) AnalyzeInstance(instance *config.InstanceInfo, metrics *config.MetricsSummary) (*cloudsql.ScalingDecision, error) {
	decision := &cloudsql.ScalingDecision{
		CurrentType: instance.MachineType,
		Metrics:     metrics,
	}

	// Check if we have enough data
	if metrics.DataPoints < 10 {
		decision.ShouldScale = false
		decision.Reason = "Insufficient metrics data for analysis"
		return decision, nil
	}

	// Determine if scaling is needed based on utilization
	scaleUp := e.shouldScaleUp(metrics)
	scaleDown := e.shouldScaleDown(metrics)

	if !scaleUp && !scaleDown {
		decision.ShouldScale = false
		decision.Reason = fmt.Sprintf("Current utilization is within target range (CPU: %.1f%%, Memory: %.1f%%)",
			metrics.CPUP95, metrics.MemoryP95Pct)
		return decision, nil
	}

	// Determine target machine type
	var targetType string
	var err error

	if scaleUp {
		targetType, err = config.GetNextLargerMachineType(instance.MachineType)
		if err != nil {
			decision.ShouldScale = false
			decision.Reason = fmt.Sprintf("Cannot scale up: %v", err)
			return decision, nil
		}
		decision.Reason = fmt.Sprintf("High resource utilization detected (CPU P95: %.1f%%, Memory P95: %.1f%%)",
			metrics.CPUP95, metrics.MemoryP95Pct)
	} else {
		targetType, err = config.GetNextSmallerMachineType(instance.MachineType)
		if err != nil {
			decision.ShouldScale = false
			decision.Reason = fmt.Sprintf("Cannot scale down: %v", err)
			return decision, nil
		}
		decision.Reason = fmt.Sprintf("Low resource utilization detected (CPU P95: %.1f%%, Memory P95: %.1f%%)",
			metrics.CPUP95, metrics.MemoryP95Pct)
	}

	decision.ShouldScale = true
	decision.RecommendedType = targetType

	// Check for downtime implications
	constraints := config.GetScalingConstraints(instance.Edition)
	if constraints.DowntimeOnScale {
		decision.DowntimeExpected = true
		decision.DowntimeReason = "Enterprise edition requires downtime for all scaling operations"
	} else {
		// Check Enterprise Plus timing constraints
		decision.DowntimeExpected, decision.DowntimeReason = e.checkDowntimeForEnterprisePlus(
			instance, scaleUp)
	}

	// Estimate cost savings
	decision.EstimatedSavings = cloudsql.EstimateCostSavings(
		instance.MachineType, targetType, instance.Region)

	return decision, nil
}

// shouldScaleUp determines if instance should be scaled up
func (e *Engine) shouldScaleUp(metrics *config.MetricsSummary) bool {
	// Scale up if P95 utilization exceeds threshold
	cpuExceeds := metrics.CPUP95 > (e.config.ScaleUpThreshold * 100)
	memoryExceeds := metrics.MemoryP95Pct > (e.config.ScaleUpThreshold * 100)

	return cpuExceeds || memoryExceeds
}

// shouldScaleDown determines if instance should be scaled down
func (e *Engine) shouldScaleDown(metrics *config.MetricsSummary) bool {
	// Scale down if P95 utilization is below threshold
	// Both CPU and memory should be low to scale down
	cpuLow := metrics.CPUP95 < (e.config.ScaleDownThreshold * 100)
	memoryLow := metrics.MemoryP95Pct < (e.config.ScaleDownThreshold * 100)

	return cpuLow && memoryLow
}

// checkDowntimeForEnterprisePlus checks if Enterprise Plus scaling would cause downtime
func (e *Engine) checkDowntimeForEnterprisePlus(instance *config.InstanceInfo, isUpscale bool) (bool, string) {
	if instance.LastScaledTime.IsZero() {
		// No previous scaling information
		return false, ""
	}

	timeSinceLastScale := time.Since(instance.LastScaledTime)
	constraints := config.GetScalingConstraints(config.EditionEnterprisePlus)

	if isUpscale {
		minInterval, _ := time.ParseDuration(constraints.MinUpscaleInterval)
		if timeSinceLastScale < minInterval {
			timeToWait := minInterval - timeSinceLastScale
			return true, fmt.Sprintf("Scaling within %s of last operation would cause downtime. Wait %v more",
				constraints.MinUpscaleInterval, timeToWait.Round(time.Minute))
		}
	} else {
		minInterval, _ := time.ParseDuration(constraints.MinDownscaleInterval)
		if timeSinceLastScale < minInterval {
			timeToWait := minInterval - timeSinceLastScale
			return true, fmt.Sprintf("Downscaling within %s of last operation would cause downtime. Wait %v more",
				constraints.MinDownscaleInterval, timeToWait.Round(time.Minute))
		}
	}

	return false, ""
}

// ValidateScalingDecision performs final validation of a scaling decision
func (e *Engine) ValidateScalingDecision(decision *cloudsql.ScalingDecision, force bool) error {
	if !decision.ShouldScale {
		return nil
	}

	// Check if downtime is expected and not forced
	if decision.DowntimeExpected && !force {
		return fmt.Errorf("scaling operation would cause downtime: %s. Use --force to proceed",
			decision.DowntimeReason)
	}

	// Validate machine type transition
	if decision.CurrentType == decision.RecommendedType {
		return fmt.Errorf("recommended type is the same as current type")
	}

	return nil
}
