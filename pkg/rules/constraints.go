package rules

import (
	"fmt"
	"time"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

// ScalingWindow represents a time window for analyzing scaling patterns
type ScalingWindow struct {
	Start    time.Time
	End      time.Time
	Duration time.Duration
}

// CheckScalingConstraints validates all constraints for a scaling operation
func CheckScalingConstraints(instance *config.InstanceInfo, metrics *config.MetricsSummary, cfg *config.Config) []string {
	var warnings []string

	// Check data completeness
	expectedDataPoints := int(cfg.MetricsPeriod / cfg.MetricsInterval)
	dataCompleteness := float64(metrics.DataPoints) / float64(expectedDataPoints) * 100

	if dataCompleteness < 80 {
		warnings = append(warnings,
			fmt.Sprintf("Limited metrics data available (%.0f%% complete). Recommendations may be less accurate.",
				dataCompleteness))
	}

	// Check for recent scaling operations
	if !instance.LastScaledTime.IsZero() {
		timeSinceScale := time.Since(instance.LastScaledTime)
		if timeSinceScale < cfg.CoolDownPeriod {
			warnings = append(warnings,
				fmt.Sprintf("Instance was scaled recently (%.0f minutes ago). Consider waiting for cooldown period.",
					timeSinceScale.Minutes()))
		}
	}

	// Check for high availability configuration
	if instance.HighAvailability {
		warnings = append(warnings,
			"Instance has high availability enabled. Scaling will affect both primary and standby instances.")
	}

	// Check backup windows
	if instance.BackupEnabled {
		warnings = append(warnings,
			"Instance has backups enabled. Avoid scaling during backup windows.")
	}

	return warnings
}

// GetOptimalScalingWindow suggests the best time window for scaling
func GetOptimalScalingWindow(metrics *config.MetricsData, constraints config.ScalingConstraints) *ScalingWindow {
	// For Enterprise Plus with no downtime (within intervals), any time is fine
	if !constraints.DowntimeOnScale {
		return &ScalingWindow{
			Start:    time.Now(),
			End:      time.Now().Add(24 * time.Hour),
			Duration: 24 * time.Hour,
		}
	}

	// For operations with downtime, find low-usage periods
	// This is a simplified version - in practice, you'd analyze usage patterns
	lowestUsageHour := findLowestUsageHour(metrics)

	// Suggest maintenance window during low usage
	windowStart := time.Now().Truncate(24 * time.Hour).Add(time.Duration(lowestUsageHour) * time.Hour)
	if windowStart.Before(time.Now()) {
		windowStart = windowStart.Add(24 * time.Hour)
	}

	return &ScalingWindow{
		Start:    windowStart,
		End:      windowStart.Add(2 * time.Hour),
		Duration: 2 * time.Hour,
	}
}

// findLowestUsageHour analyzes metrics to find the hour with lowest usage
func findLowestUsageHour(metrics *config.MetricsData) int {
	if len(metrics.Timestamps) == 0 {
		return 2 // Default to 2 AM
	}

	// Group by hour of day
	hourlyUsage := make(map[int][]float64)

	for i, ts := range metrics.Timestamps {
		hour := ts.Hour()
		usage := metrics.CPUUtilization[i]
		hourlyUsage[hour] = append(hourlyUsage[hour], usage)
	}

	// Find hour with lowest average usage
	lowestHour := 2
	lowestAvg := 100.0

	for hour, usages := range hourlyUsage {
		if len(usages) == 0 {
			continue
		}

		sum := 0.0
		for _, u := range usages {
			sum += u
		}
		avg := sum / float64(len(usages))

		if avg < lowestAvg {
			lowestAvg = avg
			lowestHour = hour
		}
	}

	return lowestHour
}

// EstimateDowntime estimates the downtime duration for a scaling operation
func EstimateDowntime(instance *config.InstanceInfo, currentType, targetType string) time.Duration {
	constraints := config.GetScalingConstraints(instance.Edition)

	if !constraints.DowntimeOnScale {
		// Enterprise Plus within timing windows
		return 0
	}

	// Estimate based on instance size
	// Larger instances typically take longer to scale
	currentMT, _ := config.GetMachineType(currentType)
	targetMT, _ := config.GetMachineType(targetType)

	maxCPU := currentMT.CPU
	if targetMT.CPU > maxCPU {
		maxCPU = targetMT.CPU
	}

	// Base estimate: 5 minutes + 30 seconds per vCPU
	baseDowntime := 5 * time.Minute
	cpuDowntime := time.Duration(maxCPU) * 30 * time.Second

	return baseDowntime + cpuDowntime
}
