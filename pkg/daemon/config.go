package daemon

import (
	"time"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

// daemonConfig implements the Config interface
// Provides immutable access to configuration following Go best practices
type daemonConfig struct {
	interval       time.Duration
	httpPort       int
	metricsEnabled bool
	projectID      string
	dryRun         bool
}

// NewDaemonConfig creates a new daemon configuration
func NewDaemonConfig(cfg *config.Config, interval time.Duration, httpPort int, metricsEnabled bool) Config {
	return &daemonConfig{
		interval:       interval,
		httpPort:       httpPort,
		metricsEnabled: metricsEnabled,
		projectID:      cfg.ProjectID,
		dryRun:         cfg.DryRun,
	}
}

// GetInterval returns the autoscaling check interval
func (c *daemonConfig) GetInterval() time.Duration {
	return c.interval
}

// GetHTTPPort returns the HTTP server port
func (c *daemonConfig) GetHTTPPort() int {
	return c.httpPort
}

// IsMetricsEnabled returns whether metrics are enabled
func (c *daemonConfig) IsMetricsEnabled() bool {
	return c.metricsEnabled
}

// IsDryRun returns whether the daemon is in dry-run mode
func (c *daemonConfig) IsDryRun() bool {
	return c.dryRun
}

// GetProjectID returns the GCP project ID
func (c *daemonConfig) GetProjectID() string {
	return c.projectID
}

// validateConfig validates daemon configuration
// Following explicit error handling patterns
func validateConfig(cfg *config.Config, interval time.Duration, httpPort int) error {
	if cfg == nil {
		return NewDaemonError("validate", "config", ErrInvalidConfig)
	}

	if cfg.ProjectID == "" {
		return NewDaemonError("validate", "config", ErrInvalidConfig)
	}

	if interval <= 0 {
		return NewDaemonError("validate", "config", ErrInvalidConfig)
	}

	if httpPort < 1 || httpPort > 65535 {
		return NewDaemonError("validate", "config", ErrInvalidConfig)
	}

	return nil
}
