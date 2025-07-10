package analyzer

import (
	"context"
	"fmt"
	"sort"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/cloudsql"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

// ProjectAnalyzer analyzes all instances in a project
type ProjectAnalyzer struct {
	*Analyzer
}

// NewProjectAnalyzer creates a new project-wide analyzer
func NewProjectAnalyzer(ctx context.Context, cfg *config.Config) (*ProjectAnalyzer, error) {
	analyzer, err := NewAnalyzer(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &ProjectAnalyzer{
		Analyzer: analyzer,
	}, nil
}

// AnalyzeAllInstances analyzes all Cloud SQL instances in the project
func (p *ProjectAnalyzer) AnalyzeAllInstances(ctx context.Context) (*ProjectAnalysisResult, error) {
	fmt.Println("Listing all Cloud SQL instances in the project...")

	// First, get the raw list to know total count
	rawResp, err := p.sqlClient.Service.Instances.List(p.config.ProjectID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	totalCount := len(rawResp.Items)

	// Now get detailed info for instances we can process
	instances, err := p.sqlClient.ListInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance details: %w", err)
	}

	if totalCount == 0 {
		return &ProjectAnalysisResult{
			ProjectID: p.config.ProjectID,
			Results:   []*AnalysisResult{},
		}, nil
	}

	fmt.Printf("Found %d instances (%d processable). Analyzing each instance...\n\n", totalCount, len(instances))

	results := make([]*AnalysisResult, 0, len(instances))
	for _, instance := range instances {
		fmt.Printf("Analyzing instance: %s\n", instance.Name)
		result, err := p.AnalyzeInstance(ctx, instance.Name)
		if err != nil {
			fmt.Printf("  Error analyzing instance %s: %v\n", instance.Name, err)
			continue
		}
		results = append(results, result)
		fmt.Println()
	}

	return &ProjectAnalysisResult{
		ProjectID:         p.config.ProjectID,
		Results:           results,
		TotalInstances:    totalCount,
		AnalyzedInstances: len(results),
	}, nil
}

// ProjectAnalysisResult contains analysis results for all instances in a project
type ProjectAnalysisResult struct {
	ProjectID         string
	Results           []*AnalysisResult
	TotalInstances    int
	AnalyzedInstances int
}

// GetScalableInstances returns instances that need scaling
func (p *ProjectAnalysisResult) GetScalableInstances() []*AnalysisResult {
	var scalable []*AnalysisResult
	for _, result := range p.Results {
		if result.Decision.ShouldScale {
			scalable = append(scalable, result)
		}
	}
	return scalable
}

// PrintProjectSummary prints a summary of all instances
func (p *ProjectAnalysisResult) PrintProjectSummary() {
	fmt.Printf("\n=== Project Analysis Summary ===\n")
	fmt.Printf("Project ID: %s\n", p.ProjectID)
	fmt.Printf("Total Instances: %d\n", p.TotalInstances)
	fmt.Printf("Analyzed: %d\n", p.AnalyzedInstances)

	scalable := p.GetScalableInstances()
	fmt.Printf("Instances Needing Scaling: %d\n\n", len(scalable))

	if len(scalable) == 0 {
		fmt.Println("No instances require scaling at this time.")
		return
	}

	// Group by scaling action
	var scaleUp, scaleDown []*AnalysisResult
	totalSavings := 0.0

	for _, result := range scalable {
		if result.Decision.EstimatedSavings > 0 {
			scaleDown = append(scaleDown, result)
			totalSavings += result.Decision.EstimatedSavings
		} else {
			scaleUp = append(scaleUp, result)
			totalSavings += result.Decision.EstimatedSavings
		}
	}

	if len(scaleUp) > 0 {
		fmt.Printf("Instances to Scale Up (%d):\n", len(scaleUp))
		for _, r := range scaleUp {
			fmt.Printf("  - %s: %s → %s (CPU P95: %.1f%%, Memory P95: %.1f%%)\n",
				r.Instance.Name, r.Decision.CurrentType, r.Decision.RecommendedType,
				r.Summary.CPUP95, r.Summary.MemoryP95Pct)
			if r.Decision.DowntimeExpected {
				fmt.Printf("    ⚠️  %s\n", r.Decision.DowntimeReason)
			}
		}
		fmt.Println()
	}

	if len(scaleDown) > 0 {
		fmt.Printf("Instances to Scale Down (%d):\n", len(scaleDown))
		for _, r := range scaleDown {
			fmt.Printf("  - %s: %s → %s (CPU P95: %.1f%%, Memory P95: %.1f%%)\n",
				r.Instance.Name, r.Decision.CurrentType, r.Decision.RecommendedType,
				r.Summary.CPUP95, r.Summary.MemoryP95Pct)
			if r.Decision.EstimatedSavings > 0 {
				fmt.Printf("    💰 Estimated monthly savings: $%.2f\n", r.Decision.EstimatedSavings)
			}
			if r.Decision.DowntimeExpected {
				fmt.Printf("    ⚠️  %s\n", r.Decision.DowntimeReason)
			}
		}
		fmt.Println()
	}

	if totalSavings > 0 {
		fmt.Printf("Total Estimated Monthly Savings: $%.2f\n", totalSavings)
	} else if totalSavings < 0 {
		fmt.Printf("Total Estimated Monthly Cost Increase: $%.2f\n", -totalSavings)
	}
}

// GenerateScalingPlan creates an ordered scaling plan
func (p *ProjectAnalysisResult) GenerateScalingPlan() *ScalingPlan {
	scalable := p.GetScalableInstances()

	plan := &ScalingPlan{
		Operations: make([]ScalingOperation, 0, len(scalable)),
	}

	for _, result := range scalable {
		op := ScalingOperation{
			Instance:         result.Instance.Name,
			CurrentType:      result.Decision.CurrentType,
			TargetType:       result.Decision.RecommendedType,
			Reason:           result.Decision.Reason,
			DowntimeExpected: result.Decision.DowntimeExpected,
			Priority:         calculatePriority(result),
		}
		plan.Operations = append(plan.Operations, op)
	}

	// Sort by priority (highest first)
	sort.Slice(plan.Operations, func(i, j int) bool {
		return plan.Operations[i].Priority > plan.Operations[j].Priority
	})

	return plan
}

// ScalingPlan represents an ordered plan for scaling operations
type ScalingPlan struct {
	Operations []ScalingOperation
}

// ScalingOperation represents a single scaling operation
type ScalingOperation struct {
	Instance         string
	CurrentType      string
	TargetType       string
	Reason           string
	DowntimeExpected bool
	Priority         int
}

// calculatePriority determines the priority of a scaling operation
func calculatePriority(result *AnalysisResult) int {
	priority := 0

	// High CPU/memory usage increases priority
	if result.Summary.CPUP95 > 90 || result.Summary.MemoryP95Pct > 90 {
		priority += 50
	} else if result.Summary.CPUP95 > 80 || result.Summary.MemoryP95Pct > 80 {
		priority += 30
	}

	// No downtime operations have higher priority
	if !result.Decision.DowntimeExpected {
		priority += 20
	}

	// Cost savings increase priority for scale-down operations
	if result.Decision.EstimatedSavings > 100 {
		priority += 10
	}

	return priority
}

// ApplyScaling applies the recommended scaling to an instance
func (a *Analyzer) ApplyScaling(ctx context.Context, instanceName string, decision *cloudsql.ScalingDecision) error {
	if !decision.ShouldScale {
		return fmt.Errorf("no scaling recommended for instance %s", instanceName)
	}

	// Validate the scaling decision
	if err := a.rulesEngine.ValidateScalingDecision(decision, a.config.Force); err != nil {
		return err
	}

	fmt.Printf("Scaling instance %s from %s to %s...\n",
		instanceName, decision.CurrentType, decision.RecommendedType)

	if a.config.DryRun {
		fmt.Println("DRY RUN: No changes will be made")
		return nil
	}

	// Perform the scaling operation
	if err := a.sqlClient.UpdateMachineType(ctx, instanceName, decision.RecommendedType); err != nil {
		return fmt.Errorf("failed to update machine type: %w", err)
	}

	fmt.Printf("Successfully scaled instance %s to %s\n", instanceName, decision.RecommendedType)
	return nil
}
