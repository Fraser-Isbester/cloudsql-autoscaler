package daemon

import (
	"context"
	"time"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/analyzer"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/cloudsql"
)

// Analyzer defines the interface for instance analysis
// Following Russ Cox principle: "Accept interfaces, return concrete types"
type Analyzer interface {
	AnalyzeAllInstances(ctx context.Context) (*analyzer.ProjectAnalysisResult, error)
	ApplyScaling(ctx context.Context, instanceName string, decision *cloudsql.ScalingDecision) error
	Close() error
}

// HTTPServerInterface defines the interface for HTTP health/metrics server
type HTTPServerInterface interface {
	Start() error
	Shutdown(ctx context.Context) error
}

// MetricsReporter defines the interface for metrics reporting
// Small interface following the principle of minimal API surface
type MetricsReporter interface {
	RecordCycleDuration(duration time.Duration)
	RecordCycleCompletion()
	RecordError(errorType string)
	RecordInstanceCounts(total, analyzed, scalable int)
}

// SignalHandler defines the interface for handling OS signals
type SignalHandler interface {
	WaitForShutdown() <-chan struct{}
}

// CycleRunner defines the interface for running autoscaling cycles
// Clear single responsibility: run autoscaling logic
type CycleRunner interface {
	RunCycle(ctx context.Context) error
}

// Config provides read-only access to daemon configuration
// Following principle of clear data flow and immutability where possible
type Config interface {
	GetInterval() time.Duration
	GetHTTPPort() int
	IsMetricsEnabled() bool
	IsDryRun() bool
	GetProjectID() string
}
