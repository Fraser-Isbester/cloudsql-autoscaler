package analyzer

import (
	"context"
	"fmt"
	"time"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/cloudsql"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/rules"
)

// Analyzer performs instance analysis and generates recommendations
type Analyzer struct {
	sqlClient     *cloudsql.Client
	metricsClient *cloudsql.MetricsClient
	rulesEngine   *rules.Engine
	config        *config.Config
}

// NewAnalyzer creates a new analyzer
func NewAnalyzer(ctx context.Context, cfg *config.Config) (*Analyzer, error) {
	sqlClient, err := cloudsql.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud SQL client: %w", err)
	}

	metricsClient, err := cloudsql.NewMetricsClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics client: %w", err)
	}

	rulesEngine := rules.NewEngine(cfg)

	return &Analyzer{
		sqlClient:     sqlClient,
		metricsClient: metricsClient,
		rulesEngine:   rulesEngine,
		config:        cfg,
	}, nil
}

// Close closes all clients
func (a *Analyzer) Close() error {
	return a.metricsClient.Close()
}

// GetInstance retrieves instance information
func (a *Analyzer) GetInstance(ctx context.Context, instanceName string) (*config.InstanceInfo, error) {
	return a.sqlClient.GetInstance(ctx, instanceName)
}

// AnalyzeInstance performs a complete analysis of a Cloud SQL instance
func (a *Analyzer) AnalyzeInstance(ctx context.Context, instanceName string) (*AnalysisResult, error) {
	// Get instance information
	fmt.Printf("Fetching instance information for %s...\n", instanceName)
	instance, err := a.sqlClient.GetInstance(ctx, instanceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance info: %w", err)
	}

	// Get last scaling time
	instance.LastScaledTime, _ = a.sqlClient.GetLastScalingTime(ctx, instanceName)

	// Fetch metrics
	fmt.Printf("Collecting metrics for the last %v...\n", a.config.MetricsPeriod)
	metrics, err := a.metricsClient.GetInstanceMetrics(ctx, instanceName, a.config)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	// Calculate metrics summary
	summary := cloudsql.CalculateMetricsSummary(metrics)

	// Analyze scaling requirements
	fmt.Println("Analyzing scaling requirements...")
	decision, err := a.rulesEngine.AnalyzeInstance(instance, summary)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze instance: %w", err)
	}

	// Check constraints
	warnings := rules.CheckScalingConstraints(instance, summary, a.config)

	// Get optimal scaling window if scaling is recommended
	var scalingWindow *rules.ScalingWindow
	if decision.ShouldScale {
		constraints := config.GetScalingConstraints(instance.Edition)
		scalingWindow = rules.GetOptimalScalingWindow(metrics, constraints)
	}

	return &AnalysisResult{
		Instance:      instance,
		Metrics:       metrics,
		Summary:       summary,
		Decision:      decision,
		Warnings:      warnings,
		ScalingWindow: scalingWindow,
		AnalyzedAt:    time.Now(),
	}, nil
}

// AnalysisResult contains the complete analysis results
type AnalysisResult struct {
	Instance      *config.InstanceInfo
	Metrics       *config.MetricsData
	Summary       *config.MetricsSummary
	Decision      *cloudsql.ScalingDecision
	Warnings      []string
	ScalingWindow *rules.ScalingWindow
	AnalyzedAt    time.Time
}

// PrintAnalysisReport prints a formatted analysis report
func (r *AnalysisResult) PrintAnalysisReport() {
	fmt.Printf("\n=== Cloud SQL Instance Analysis Report ===\n")
	fmt.Printf("Instance: %s\n", r.Instance.Name)
	fmt.Printf("Project: %s\n", r.Instance.Project)
	fmt.Printf("Analyzed at: %s\n\n", r.AnalyzedAt.Format(time.RFC3339))

	fmt.Printf("Current Configuration:\n")
	fmt.Printf("  Machine Type: %s\n", r.Instance.MachineType)
	fmt.Printf("  Edition: %s\n", r.Instance.Edition)
	fmt.Printf("  CPU: %d vCPUs\n", r.Instance.CurrentCPU)
	fmt.Printf("  Memory: %.1f GB\n", r.Instance.CurrentMemoryGB)
	fmt.Printf("  Region: %s\n", r.Instance.Region)
	if r.Instance.Zone != "" {
		fmt.Printf("  Zone: %s\n", r.Instance.Zone)
	}
	if !r.Instance.LastScaledTime.IsZero() {
		fmt.Printf("  Last Scaled: %s (%s ago)\n",
			r.Instance.LastScaledTime.Format(time.RFC3339),
			time.Since(r.Instance.LastScaledTime).Round(time.Minute))
	}

	fmt.Printf("\nMetrics Summary (Period: %v):\n", r.Summary.Period.Round(time.Hour))
	fmt.Printf("  Data Points: %d\n", r.Summary.DataPoints)
	fmt.Printf("  CPU Utilization:\n")
	fmt.Printf("    Average: %.1f%%\n", r.Summary.CPUAvg)
	fmt.Printf("    P95: %.1f%%\n", r.Summary.CPUP95)
	fmt.Printf("    P99: %.1f%%\n", r.Summary.CPUP99)
	fmt.Printf("    Max: %.1f%%\n", r.Summary.CPUMax)
	fmt.Printf("  Memory Utilization:\n")
	fmt.Printf("    Average: %.1f%% (%.1f GB)\n", r.Summary.MemoryAvgPct, r.Summary.MemoryAvgGB)
	fmt.Printf("    P95: %.1f%% (%.1f GB)\n", r.Summary.MemoryP95Pct, r.Summary.MemoryP95GB)
	fmt.Printf("    P99: %.1f%% (%.1f GB)\n", r.Summary.MemoryP99Pct, r.Summary.MemoryP99GB)
	fmt.Printf("    Max: %.1f GB\n", r.Summary.MemoryMaxGB)

	fmt.Printf("\nScaling Recommendation:\n")
	if r.Decision.ShouldScale {
		fmt.Printf("  Action: SCALE\n")
		fmt.Printf("  Current Type: %s\n", r.Decision.CurrentType)
		fmt.Printf("  Recommended Type: %s\n", r.Decision.RecommendedType)
		fmt.Printf("  Reason: %s\n", r.Decision.Reason)

		if r.Decision.EstimatedSavings > 0 {
			fmt.Printf("  Estimated Monthly Savings: $%.2f\n", r.Decision.EstimatedSavings)
		} else if r.Decision.EstimatedSavings < 0 {
			fmt.Printf("  Estimated Monthly Cost Increase: $%.2f\n", -r.Decision.EstimatedSavings)
		}

		if r.Decision.DowntimeExpected {
			fmt.Printf("  ⚠️  Downtime Expected: %s\n", r.Decision.DowntimeReason)
			estimatedDowntime := rules.EstimateDowntime(r.Instance, r.Decision.CurrentType, r.Decision.RecommendedType)
			if estimatedDowntime > 0 {
				fmt.Printf("  Estimated Downtime: %v\n", estimatedDowntime)
			}
		} else {
			fmt.Printf("  ✓ No Downtime Expected\n")
		}

		if r.ScalingWindow != nil {
			fmt.Printf("\nRecommended Scaling Window:\n")
			fmt.Printf("  Start: %s\n", r.ScalingWindow.Start.Format(time.RFC3339))
			fmt.Printf("  End: %s\n", r.ScalingWindow.End.Format(time.RFC3339))
		}
	} else {
		fmt.Printf("  Action: NO SCALING NEEDED\n")
		fmt.Printf("  Reason: %s\n", r.Decision.Reason)
	}

	if len(r.Warnings) > 0 {
		fmt.Printf("\nWarnings:\n")
		for _, warning := range r.Warnings {
			fmt.Printf("  ⚠️  %s\n", warning)
		}
	}

	fmt.Printf("\n")
}

// PrintMetricsSummary prints a brief metrics summary
func (r *AnalysisResult) PrintMetricsSummary() {
	fmt.Printf("Instance: %s | CPU P95: %.1f%% | Memory P95: %.1f%% | ",
		r.Instance.Name, r.Summary.CPUP95, r.Summary.MemoryP95Pct)

	if r.Decision.ShouldScale {
		fmt.Printf("Recommendation: Scale from %s to %s",
			r.Decision.CurrentType, r.Decision.RecommendedType)
		if r.Decision.DowntimeExpected {
			fmt.Printf(" (downtime expected)")
		}
	} else {
		fmt.Printf("Recommendation: No scaling needed")
	}
	fmt.Printf("\n")
}
