package config

import "time"

// Config holds the configuration for the autoscaler
type Config struct {
	ProjectID string
	Instance  string

	// Telemetry settings
	MetricsPeriod   time.Duration
	MetricsInterval time.Duration // Granularity of metrics

	// Scaling thresholds
	CPUTargetUtilization    float64
	MemoryTargetUtilization float64
	ScaleUpThreshold        float64 // e.g., 0.8 = 80%
	ScaleDownThreshold      float64 // e.g., 0.5 = 50%

	// Scaling behavior
	MinStableDuration time.Duration // Minimum time at threshold before scaling
	CoolDownPeriod    time.Duration // Time to wait after scaling

	// Operation settings
	DryRun bool
	Force  bool // Force scaling even if it causes downtime
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		MetricsPeriod:           7 * 24 * time.Hour, // 7 days
		MetricsInterval:         5 * time.Minute,    // 5 minute granularity
		CPUTargetUtilization:    0.7,                // 70%
		MemoryTargetUtilization: 0.8,                // 80%
		ScaleUpThreshold:        0.8,                // Scale up at 80% utilization
		ScaleDownThreshold:      0.5,                // Scale down at 50% utilization
		MinStableDuration:       1 * time.Hour,      // Sustained for 1 hour
		CoolDownPeriod:          30 * time.Minute,   // Wait 30 minutes after scaling
		DryRun:                  false,
		Force:                   false,
	}
}

// InstanceInfo holds information about a Cloud SQL instance
type InstanceInfo struct {
	Name             string
	Project          string
	DatabaseVersion  string
	MachineType      string
	Edition          Edition
	State            string
	LastScaledTime   time.Time
	CurrentCPU       int
	CurrentMemoryGB  float64
	MaxConnections   int
	BackupEnabled    bool
	HighAvailability bool
	Region           string
	Zone             string
}

// MetricsData holds time series metrics data
type MetricsData struct {
	Timestamps     []time.Time
	CPUUtilization []float64 // Percentage (0-100)
	MemoryUsageGB  []float64 // Actual memory used in GB
	MemoryPercent  []float64 // Memory utilization percentage
	Connections    []int
	DiskUsageGB    []float64
	DiskIOPS       []float64
}

// MetricsSummary holds statistical summary of metrics
type MetricsSummary struct {
	CPUAvg         float64
	CPUP95         float64
	CPUP99         float64
	CPUMax         float64
	MemoryAvgGB    float64
	MemoryP95GB    float64
	MemoryP99GB    float64
	MemoryMaxGB    float64
	MemoryAvgPct   float64
	MemoryP95Pct   float64
	MemoryP99Pct   float64
	ConnectionsAvg float64
	ConnectionsMax int
	Period         time.Duration
	DataPoints     int
}
