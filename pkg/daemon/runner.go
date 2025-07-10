package daemon

import (
	"context"
	"log"
	"time"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/analyzer"
)

// autoscalingRunner implements CycleRunner interface
// Following single responsibility principle
type autoscalingRunner struct {
	analyzer Analyzer
	config   Config
	metrics  MetricsReporter
}

// NewAutoscalingRunner creates a new cycle runner
func NewAutoscalingRunner(analyzer Analyzer, config Config, metrics MetricsReporter) CycleRunner {
	return &autoscalingRunner{
		analyzer: analyzer,
		config:   config,
		metrics:  metrics,
	}
}

// RunCycle executes a single autoscaling cycle
// Clear function with single responsibility and explicit error handling
func (r *autoscalingRunner) RunCycle(ctx context.Context) error {
	start := time.Now()

	// Defer metrics recording - ensures we always record, even on panic
	defer func() {
		duration := time.Since(start)
		r.metrics.RecordCycleDuration(duration)
		r.metrics.RecordCycleCompletion()

		if rec := recover(); rec != nil {
			r.metrics.RecordError("panic")
			log.Printf("Recovered from panic in autoscaling cycle: %v", rec)
		}
	}()

	log.Printf("Starting autoscaling cycle for project: %s", r.config.GetProjectID())

	// Analyze all instances
	results, err := r.analyzer.AnalyzeAllInstances(ctx)
	if err != nil {
		r.metrics.RecordError("analysis_error")
		return WrapError("analyze_instances", err)
	}

	scalableInstances := results.GetScalableInstances()

	// Record metrics
	r.metrics.RecordInstanceCounts(
		results.TotalInstances,
		results.AnalyzedInstances,
		len(scalableInstances),
	)

	log.Printf("Found %d instances needing scaling out of %d total instances",
		len(scalableInstances), results.TotalInstances)

	if r.config.IsDryRun() {
		log.Printf("Dry-run mode: would scale %d instances", len(scalableInstances))
		return nil
	}

	// Apply scaling decisions
	return r.applyScalingDecisions(ctx, scalableInstances)
}

// applyScalingDecisions applies scaling to instances that need it
func (r *autoscalingRunner) applyScalingDecisions(ctx context.Context, instances []*analyzer.AnalysisResult) error {
	successCount := 0
	var lastErr error

	for _, result := range instances {
		err := r.analyzer.ApplyScaling(ctx, result.Instance.Name, result.Decision)
		if err != nil {
			log.Printf("Failed to scale instance %s: %v", result.Instance.Name, err)
			r.metrics.RecordError("scaling_failed")
			lastErr = err
		} else {
			log.Printf("Successfully scaled instance %s from %s to %s",
				result.Instance.Name, result.Decision.CurrentType, result.Decision.RecommendedType)
			successCount++
		}
	}

	log.Printf("Applied scaling to %d/%d instances", successCount, len(instances))

	// Return the last error if any scaling failed
	// This follows Go's pattern of returning the most recent error
	if lastErr != nil {
		return WrapError("apply_scaling", lastErr)
	}

	return nil
}

// simpleMetricsReporter provides a no-op implementation when metrics are disabled
type simpleMetricsReporter struct{}

func (r *simpleMetricsReporter) RecordCycleDuration(duration time.Duration)         {}
func (r *simpleMetricsReporter) RecordCycleCompletion()                             {}
func (r *simpleMetricsReporter) RecordError(errorType string)                       {}
func (r *simpleMetricsReporter) RecordInstanceCounts(total, analyzed, scalable int) {}

// NewSimpleMetricsReporter creates a no-op metrics reporter
func NewSimpleMetricsReporter() MetricsReporter {
	return &simpleMetricsReporter{}
}

// prometheusMetricsReporter implements MetricsReporter using Prometheus metrics
type prometheusMetricsReporter struct{}

func (r *prometheusMetricsReporter) RecordCycleDuration(duration time.Duration) {
	if metricsEnabled {
		autoscalingCycleDuration.Set(duration.Seconds())
	}
}

func (r *prometheusMetricsReporter) RecordCycleCompletion() {
	if metricsEnabled {
		autoscalingCyclesTotal.Inc()
	}
}

func (r *prometheusMetricsReporter) RecordError(errorType string) {
	if metricsEnabled {
		autoscalingErrors.WithLabelValues(errorType).Inc()
	}
}

func (r *prometheusMetricsReporter) RecordInstanceCounts(total, analyzed, scalable int) {
	if metricsEnabled {
		instancesTotal.Set(float64(total))
		instancesAnalyzed.Set(float64(analyzed))
		instancesScalable.Set(float64(scalable))
	}
}

// NewPrometheusMetricsReporter creates a Prometheus-backed metrics reporter
func NewPrometheusMetricsReporter() MetricsReporter {
	return &prometheusMetricsReporter{}
}
