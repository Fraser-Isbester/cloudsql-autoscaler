package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/analyzer"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

var (
	projectID string
	instance  string
	period    string
	dryRun    bool
	force     bool
)

var rootCmd = &cobra.Command{
	Use:   "cloudsql-autoscaler",
	Short: "Autoscaling tool for Google Cloud SQL instances",
	Long: `cloudsql-autoscaler analyzes Cloud SQL instance metrics and provides
scaling recommendations based on CPU and memory utilization patterns.

It supports both Enterprise and Enterprise Plus editions with awareness
of scaling constraints and downtime implications.`,
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze instance metrics and show utilization patterns",
	Long:  `Fetches historical metrics for a Cloud SQL instance and analyzes CPU and memory utilization patterns.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cfg := buildConfig()

		analyzer, err := analyzer.NewAnalyzer(ctx, cfg)
		if err != nil {
			return fmt.Errorf("failed to create analyzer: %w", err)
		}
		defer analyzer.Close()

		result, err := analyzer.AnalyzeInstance(ctx, instance)
		if err != nil {
			return fmt.Errorf("analysis failed: %w", err)
		}

		result.PrintAnalysisReport()
		return nil
	},
}

var suggestCmd = &cobra.Command{
	Use:   "suggest",
	Short: "Suggest optimal machine type based on usage patterns",
	Long:  `Analyzes metrics and suggests an optimal machine type for the instance based on historical usage.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cfg := buildConfig()

		analyzer, err := analyzer.NewAnalyzer(ctx, cfg)
		if err != nil {
			return fmt.Errorf("failed to create analyzer: %w", err)
		}
		defer analyzer.Close()

		result, err := analyzer.AnalyzeInstance(ctx, instance)
		if err != nil {
			return fmt.Errorf("analysis failed: %w", err)
		}

		result.PrintAnalysisReport()

		if result.Decision.ShouldScale && !dryRun {
			fmt.Println("\nApplying scaling recommendation...")
			if err := analyzer.ApplyScaling(ctx, instance, result.Decision); err != nil {
				return fmt.Errorf("failed to apply scaling: %w", err)
			}
		}

		return nil
	},
}

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Show current instance configuration and constraints",
	Long:  `Displays detailed information about a Cloud SQL instance including its current configuration and scaling constraints.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cfg := buildConfig()

		analyzer, err := analyzer.NewAnalyzer(ctx, cfg)
		if err != nil {
			return fmt.Errorf("failed to create analyzer: %w", err)
		}
		defer analyzer.Close()

		// Get instance info without full analysis
		instance, err := analyzer.GetInstance(ctx, instance)
		if err != nil {
			return fmt.Errorf("failed to get instance info: %w", err)
		}

		printInstanceInfo(instance)
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all instances with scaling recommendations",
	Long:  `Lists all Cloud SQL instances in the project with basic scaling recommendations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cfg := buildConfig()

		projectAnalyzer, err := analyzer.NewProjectAnalyzer(ctx, cfg)
		if err != nil {
			return fmt.Errorf("failed to create analyzer: %w", err)
		}
		defer projectAnalyzer.Close()

		results, err := projectAnalyzer.AnalyzeAllInstances(ctx)
		if err != nil {
			return fmt.Errorf("analysis failed: %w", err)
		}

		results.PrintProjectSummary()
		return nil
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&projectID, "project", "", "GCP project ID (required)")
	rootCmd.MarkPersistentFlagRequired("project")

	// Analyze command flags
	analyzeCmd.Flags().StringVar(&instance, "instance", "", "Cloud SQL instance name (required)")
	analyzeCmd.Flags().StringVar(&period, "period", "7d", "Analysis period (e.g., 24h, 7d, 30d)")
	analyzeCmd.MarkFlagRequired("instance")

	// Suggest command flags
	suggestCmd.Flags().StringVar(&instance, "instance", "", "Cloud SQL instance name (required)")
	suggestCmd.Flags().StringVar(&period, "period", "7d", "Analysis period (e.g., 24h, 7d, 30d)")
	suggestCmd.Flags().BoolVar(&dryRun, "dry-run", true, "Show recommendation without applying")
	suggestCmd.Flags().BoolVar(&force, "force", false, "Force scaling even if it causes downtime")
	suggestCmd.MarkFlagRequired("instance")

	// Inspect command flags
	inspectCmd.Flags().StringVar(&instance, "instance", "", "Cloud SQL instance name (required)")
	inspectCmd.MarkFlagRequired("instance")

	// List command flags
	listCmd.Flags().StringVar(&period, "period", "7d", "Analysis period (e.g., 24h, 7d, 30d)")

	// Add commands to root
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(suggestCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(listCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// buildConfig creates a config from command line flags
func buildConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.ProjectID = projectID
	cfg.Instance = instance
	cfg.DryRun = dryRun
	cfg.Force = force

	// Parse period duration
	if period != "" {
		if dur, err := parseDuration(period); err == nil {
			cfg.MetricsPeriod = dur
		}
	}

	return cfg
}

// parseDuration parses duration strings like "7d", "24h"
func parseDuration(s string) (time.Duration, error) {
	// Handle day suffix
	if len(s) > 1 && s[len(s)-1] == 'd' {
		days := s[:len(s)-1]
		var d int
		_, err := fmt.Sscanf(days, "%d", &d)
		if err != nil {
			return 0, err
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}

	// Try standard duration parsing
	return time.ParseDuration(s)
}

// printInstanceInfo prints formatted instance information
func printInstanceInfo(instance *config.InstanceInfo) {
	fmt.Printf("\n=== Cloud SQL Instance Information ===\n")
	fmt.Printf("Name: %s\n", instance.Name)
	fmt.Printf("Project: %s\n", instance.Project)
	fmt.Printf("Database Version: %s\n", instance.DatabaseVersion)
	fmt.Printf("State: %s\n", instance.State)
	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("  Machine Type: %s\n", instance.MachineType)
	fmt.Printf("  Edition: %s\n", instance.Edition)
	fmt.Printf("  CPU: %d vCPUs\n", instance.CurrentCPU)
	fmt.Printf("  Memory: %.1f GB\n", instance.CurrentMemoryGB)
	fmt.Printf("  Region: %s\n", instance.Region)
	if instance.Zone != "" {
		fmt.Printf("  Zone: %s\n", instance.Zone)
	}
	fmt.Printf("  High Availability: %v\n", instance.HighAvailability)
	fmt.Printf("  Backup Enabled: %v\n", instance.BackupEnabled)

	if !instance.LastScaledTime.IsZero() {
		fmt.Printf("\nLast Scaled: %s (%s ago)\n",
			instance.LastScaledTime.Format(time.RFC3339),
			time.Since(instance.LastScaledTime).Round(time.Minute))
	}

	// Show scaling constraints
	constraints := config.GetScalingConstraints(instance.Edition)
	fmt.Printf("\nScaling Constraints:\n")
	fmt.Printf("  Minimum Upscale Interval: %s\n", constraints.MinUpscaleInterval)
	fmt.Printf("  Minimum Downscale Interval: %s\n", constraints.MinDownscaleInterval)
	fmt.Printf("  Downtime on Scale: %v\n", constraints.DowntimeOnScale)

	fmt.Println()
}
