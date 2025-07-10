package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/spf13/cobra"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/analyzer"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

var (
	projectID string
	instances []string
	dryRun    bool
	profile   string
	output    string
)

var rootCmd = &cobra.Command{
	Use:   "cloudsql-autoscaler",
	Short: "Autoscaling controller for Google Cloud SQL instances",
	Long: `cloudsql-autoscaler analyzes Cloud SQL instance metrics and automatically
scales instances based on CPU and memory utilization patterns.

It supports both Enterprise and Enterprise Plus editions with awareness
of scaling constraints and downtime implications.`,
	RunE: runAutoscaler,
}

func init() {
	rootCmd.Flags().StringVar(&projectID, "project", "", "GCP project ID (uses ADC default if not specified)")
	rootCmd.Flags().StringSliceVar(&instances, "instance", []string{}, "Instance name(s) to analyze (analyzes all if not specified)")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", true, "Show what would be done without making changes")
	rootCmd.Flags().StringVar(&profile, "profile", "default", "Scaling profile (default, conservative, aggressive)")
	rootCmd.Flags().StringVar(&output, "output", "table", "Output format (table, json)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type OutputResult struct {
	Instance        string    `json:"instance"`
	CurrentType     string    `json:"current_type"`
	CurrentCPU      int       `json:"current_cpu"`
	CurrentMemoryGB float64   `json:"current_memory_gb"`
	RecommendedType string    `json:"recommended_type,omitempty"`
	Action          string    `json:"action"`
	Reason          string    `json:"reason"`
	DowntimeWarning string    `json:"downtime_warning,omitempty"`
	Applied         bool      `json:"applied"`
	Error           string    `json:"error,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
}

type OutputSummary struct {
	ProjectID         string         `json:"project_id"`
	TotalInstances    int            `json:"total_instances"`
	AnalyzedInstances int            `json:"analyzed_instances"`
	ScalingResults    []OutputResult `json:"scaling_results"`
	Profile           string         `json:"profile"`
	DryRun            bool           `json:"dry_run"`
	Timestamp         time.Time      `json:"timestamp"`
}

type TableRow struct {
	Instance         string
	CurrentType      string
	CurrentResources string
	Action           string
	RecommendedType  string
	Status           string
	Warning          string
}

func printTable(headers []string, rows []TableRow) {
	if len(rows) == 0 {
		return
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}

	for _, row := range rows {
		data := []string{row.Instance, row.CurrentType, row.CurrentResources, row.Action, row.RecommendedType, row.Status, row.Warning}
		for i, cell := range data {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	printRow(headers, widths)
	printSeparator(widths)
	for _, row := range rows {
		data := []string{row.Instance, row.CurrentType, row.CurrentResources, row.Action, row.RecommendedType, row.Status, row.Warning}
		printRow(data, widths)
	}
}

func printRow(data []string, widths []int) {
	row := "| "
	for i, cell := range data {
		if i < len(widths) {
			row += fmt.Sprintf("%-*s | ", widths[i], cell)
		}
	}
	fmt.Println(row)
}

func printSeparator(widths []int) {
	row := "|-"
	for _, width := range widths {
		row += strings.Repeat("-", width) + "-|-"
	}
	fmt.Println(row)
}

func logf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func runAutoscaler(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if projectID == "" {
		var err error
		projectID, err = getDefaultProjectID(ctx)
		if err != nil {
			return fmt.Errorf("project not specified and could not determine default: %w", err)
		}
		logf("Using project: %s\n", projectID)
	}

	cfg := buildConfigFromProfile(profile)
	cfg.ProjectID = projectID
	cfg.DryRun = dryRun

	projectAnalyzer, err := analyzer.NewProjectAnalyzer(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create analyzer: %w", err)
	}
	defer projectAnalyzer.Close()

	if output != "table" && output != "json" {
		return fmt.Errorf("invalid output format: %s (must be 'table' or 'json')", output)
	}

	if len(instances) > 0 {
		return analyzeSpecificInstances(ctx, projectAnalyzer, instances)
	}
	return analyzeAllInstances(ctx, projectAnalyzer)
}

func analyzeSpecificInstances(ctx context.Context, analyzer *analyzer.ProjectAnalyzer, instances []string) error {
	var results []OutputResult
	var tableRows []TableRow

	logf("Analyzing %d specified instance(s)...\n", len(instances))

	var hasErrors bool
	for _, instanceName := range instances {
		logf("Analyzing instance: %s\n", instanceName)

		outputResult := OutputResult{Instance: instanceName, Applied: false, Timestamp: time.Now()}
		tableRow := TableRow{Instance: instanceName}

		result, err := analyzer.AnalyzeInstance(ctx, instanceName)
		if err != nil {
			outputResult.Error = err.Error()
			outputResult.Action = "error"
			outputResult.Reason = "Failed to analyze instance"
			tableRow.Action = "ERROR"
			tableRow.Status = "Failed"
			tableRow.Warning = "Analysis failed"
			logf("  Error: %v\n", err)
			hasErrors = true
			results = append(results, outputResult)
			tableRows = append(tableRows, tableRow)
			continue
		}

		outputResult.CurrentType = result.Instance.MachineType
		outputResult.CurrentCPU = result.Instance.CurrentCPU
		outputResult.CurrentMemoryGB = result.Instance.CurrentMemoryGB
		tableRow.CurrentType = result.Instance.MachineType
		tableRow.CurrentResources = fmt.Sprintf("%d CPU, %.1f GB", result.Instance.CurrentCPU, result.Instance.CurrentMemoryGB)

		if result.Decision.ShouldScale {
			// Determine scale direction
			currentMT, _ := config.GetMachineType(result.Instance.MachineType)
			recommendedMT, _ := config.GetMachineType(result.Decision.RecommendedType)

			var action string
			if recommendedMT.CPU > currentMT.CPU || recommendedMT.MemoryGB > currentMT.MemoryGB {
				action = "SCALE_UP"
			} else {
				action = "SCALE_DOWN"
			}

			outputResult.Action = strings.ToLower(action)
			outputResult.RecommendedType = result.Decision.RecommendedType
			outputResult.Reason = result.Decision.Reason
			tableRow.Action = action
			tableRow.RecommendedType = result.Decision.RecommendedType

			if result.Decision.DowntimeExpected {
				outputResult.DowntimeWarning = result.Decision.DowntimeReason
				tableRow.Warning = "Downtime expected"
			}

			if !dryRun {
				logf("  Applying scaling from %s to %s...\n", result.Instance.MachineType, result.Decision.RecommendedType)
				if err := analyzer.ApplyScaling(ctx, instanceName, result.Decision); err != nil {
					outputResult.Error = err.Error()
					tableRow.Status = "FAILED"
					tableRow.Warning = "Scaling failed"
					logf("  Failed: %v\n", err)
					hasErrors = true
				} else {
					outputResult.Applied = true
					tableRow.Status = "SUCCESS"
					logf("  Success\n")
				}
			} else {
				tableRow.Status = "DRY-RUN"
			}
		} else {
			outputResult.Action = "no_action"
			outputResult.Reason = result.Decision.Reason
			tableRow.Action = "NONE"
			tableRow.Status = "OK"
		}

		results = append(results, outputResult)
		tableRows = append(tableRows, tableRow)
	}

	if output == "json" {
		summary := OutputSummary{
			ProjectID: projectID, TotalInstances: len(instances), AnalyzedInstances: len(instances) - countErrors(results),
			ScalingResults: results, Profile: profile, DryRun: dryRun, Timestamp: time.Now(),
		}
		jsonOutput, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON output: %w", err)
		}
		fmt.Println(string(jsonOutput))
	} else {
		headers := []string{"Instance", "Current Type", "Resources", "Action", "Recommended", "Status", "Warning"}
		printTable(headers, tableRows)
	}

	if hasErrors {
		return fmt.Errorf("some instances had errors")
	}
	return nil
}

func analyzeAllInstances(ctx context.Context, analyzer *analyzer.ProjectAnalyzer) error {
	results, err := analyzer.AnalyzeAllInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to analyze instances: %w", err)
	}

	var outputResults []OutputResult
	var tableRows []TableRow
	scalable := results.GetScalableInstances()

	logf("Total instances: %d, Analyzed: %d, Need scaling: %d\n", results.TotalInstances, results.AnalyzedInstances, len(scalable))

	var hasErrors bool
	for _, result := range results.Results {
		outputResult := OutputResult{
			Instance: result.Instance.Name, CurrentType: result.Instance.MachineType,
			CurrentCPU: result.Instance.CurrentCPU, CurrentMemoryGB: result.Instance.CurrentMemoryGB,
			Applied: false, Timestamp: time.Now(),
		}
		tableRow := TableRow{
			Instance: result.Instance.Name, CurrentType: result.Instance.MachineType,
			CurrentResources: fmt.Sprintf("%d CPU, %.1f GB", result.Instance.CurrentCPU, result.Instance.CurrentMemoryGB),
		}

		if result.Decision.ShouldScale {
			// Determine scale direction
			currentMT, _ := config.GetMachineType(result.Instance.MachineType)
			recommendedMT, _ := config.GetMachineType(result.Decision.RecommendedType)

			var action string
			if recommendedMT.CPU > currentMT.CPU || recommendedMT.MemoryGB > currentMT.MemoryGB {
				action = "SCALE_UP"
			} else {
				action = "SCALE_DOWN"
			}

			outputResult.Action = strings.ToLower(action)
			outputResult.RecommendedType = result.Decision.RecommendedType
			outputResult.Reason = result.Decision.Reason
			tableRow.Action = action
			tableRow.RecommendedType = result.Decision.RecommendedType

			if result.Decision.DowntimeExpected {
				outputResult.DowntimeWarning = result.Decision.DowntimeReason
				tableRow.Warning = "Downtime expected"
			}

			if !dryRun {
				logf("Applying scaling for %s from %s to %s...\n", result.Instance.Name, result.Instance.MachineType, result.Decision.RecommendedType)
				if err := analyzer.ApplyScaling(ctx, result.Instance.Name, result.Decision); err != nil {
					outputResult.Error = err.Error()
					tableRow.Status = "FAILED"
					tableRow.Warning = "Scaling failed"
					logf("  Failed: %v\n", err)
					hasErrors = true
				} else {
					outputResult.Applied = true
					tableRow.Status = "SUCCESS"
					logf("  Success\n")
				}
			} else {
				tableRow.Status = "DRY-RUN"
			}
		} else {
			outputResult.Action = "no_action"
			outputResult.Reason = result.Decision.Reason
			tableRow.Action = "NONE"
			tableRow.Status = "OK"
		}

		outputResults = append(outputResults, outputResult)
		tableRows = append(tableRows, tableRow)
	}

	if output == "json" {
		summary := OutputSummary{
			ProjectID: projectID, TotalInstances: results.TotalInstances, AnalyzedInstances: results.AnalyzedInstances,
			ScalingResults: outputResults, Profile: profile, DryRun: dryRun, Timestamp: time.Now(),
		}
		jsonOutput, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON output: %w", err)
		}
		fmt.Println(string(jsonOutput))
	} else {
		headers := []string{"Instance", "Current Type", "Resources", "Action", "Recommended", "Status", "Warning"}
		printTable(headers, tableRows)
	}

	if hasErrors {
		return fmt.Errorf("some instances had errors during scaling")
	}
	return nil
}

func countErrors(results []OutputResult) int {
	count := 0
	for _, result := range results {
		if result.Error != "" {
			count++
		}
	}
	return count
}

func getDefaultProjectID(ctx context.Context) (string, error) {
	if metadata.OnGCE() {
		project, err := metadata.ProjectID()
		if err == nil {
			return project, nil
		}
	}
	if project := os.Getenv("GOOGLE_CLOUD_PROJECT"); project != "" {
		return project, nil
	}
	return "", fmt.Errorf("unable to determine project ID from Application Default Credentials")
}

func buildConfigFromProfile(profile string) *config.Config {
	cfg := config.DefaultConfig()
	switch profile {
	case "conservative":
		cfg.ScaleUpThreshold = 0.9
		cfg.ScaleDownThreshold = 0.3
		cfg.MinStableDuration = 2 * time.Hour
		cfg.MetricsPeriod = 14 * 24 * time.Hour
	case "aggressive":
		cfg.ScaleUpThreshold = 0.7
		cfg.ScaleDownThreshold = 0.6
		cfg.MinStableDuration = 30 * time.Minute
		cfg.MetricsPeriod = 3 * 24 * time.Hour
	}
	return cfg
}
