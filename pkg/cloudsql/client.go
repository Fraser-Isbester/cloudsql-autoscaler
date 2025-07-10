package cloudsql

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

// Client wraps the Cloud SQL Admin API client
type Client struct {
	Service   *sqladmin.Service // Exported for raw API access
	projectID string
}

// NewClient creates a new Cloud SQL client
func NewClient(ctx context.Context, projectID string, opts ...option.ClientOption) (*Client, error) {
	service, err := sqladmin.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud SQL service: %w", err)
	}

	return &Client{
		Service:   service,
		projectID: projectID,
	}, nil
}

// GetInstance retrieves information about a Cloud SQL instance
func (c *Client) GetInstance(ctx context.Context, instanceName string) (*config.InstanceInfo, error) {
	instance, err := c.Service.Instances.Get(c.projectID, instanceName).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get instance %s: %w", instanceName, err)
	}

	// Parse machine type to get CPU and memory
	machineType, err := config.GetMachineType(instance.Settings.Tier)
	if err != nil {
		return nil, fmt.Errorf("unknown machine type %s: %w", instance.Settings.Tier, err)
	}

	// Determine edition from settings
	edition := config.EditionEnterprise
	if instance.Settings.Edition == "ENTERPRISE_PLUS" {
		edition = config.EditionEnterprisePlus
	}

	// Parse last operation time if available
	var lastScaledTime time.Time
	// Note: This would need to be determined from operation history

	info := &config.InstanceInfo{
		Name:             instance.Name,
		Project:          c.projectID,
		DatabaseVersion:  instance.DatabaseVersion,
		MachineType:      instance.Settings.Tier,
		Edition:          edition,
		State:            instance.State,
		LastScaledTime:   lastScaledTime,
		CurrentCPU:       machineType.CPU,
		CurrentMemoryGB:  machineType.MemoryGB,
		BackupEnabled:    instance.Settings.BackupConfiguration.Enabled,
		HighAvailability: instance.Settings.AvailabilityType == "REGIONAL",
		Region:           instance.Region,
	}

	// Extract zone from gceZone if available
	if instance.GceZone != "" {
		info.Zone = instance.GceZone
	}

	// Get max connections from database flags if set
	for _, flag := range instance.Settings.DatabaseFlags {
		if flag.Name == "max_connections" {
			// Parse max connections value
			// Note: Proper parsing would be needed here
		}
	}

	return info, nil
}

// ListInstances lists all Cloud SQL instances in the project
func (c *Client) ListInstances(ctx context.Context) ([]*config.InstanceInfo, error) {
	var instances []*config.InstanceInfo

	resp, err := c.Service.Instances.List(c.projectID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	for _, instance := range resp.Items {
		info, err := c.GetInstance(ctx, instance.Name)
		if err != nil {
			// Log error but continue with other instances
			fmt.Printf("Warning: failed to get details for instance %s: %v\n", instance.Name, err)
			continue
		}
		instances = append(instances, info)
	}

	return instances, nil
}

// UpdateMachineType updates the machine type of an instance
func (c *Client) UpdateMachineType(ctx context.Context, instanceName string, newMachineType string) error {
	// Get current instance to preserve settings
	instance, err := c.Service.Instances.Get(c.projectID, instanceName).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to get instance for update: %w", err)
	}

	// Create patch request with new machine type
	instance.Settings.Tier = newMachineType

	// Perform the update
	operation, err := c.Service.Instances.Update(c.projectID, instanceName, instance).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to update instance machine type: %w", err)
	}

	// Wait for operation to complete
	if err := c.waitForOperation(ctx, operation); err != nil {
		return fmt.Errorf("machine type update operation failed: %w", err)
	}

	return nil
}

// GetRecentOperations retrieves recent operations for an instance
func (c *Client) GetRecentOperations(ctx context.Context, instanceName string, limit int) ([]*sqladmin.Operation, error) {
	resp, err := c.Service.Operations.List(c.projectID).
		MaxResults(int64(limit)).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list operations: %w", err)
	}

	// Filter operations for the target instance
	var filteredOps []*sqladmin.Operation
	for _, op := range resp.Items {
		if op.TargetId == instanceName || op.TargetLink == fmt.Sprintf("https://sqladmin.googleapis.com/sql/v1beta4/projects/%s/instances/%s", c.projectID, instanceName) {
			filteredOps = append(filteredOps, op)
		}
	}

	return filteredOps, nil
}

// waitForOperation waits for a Cloud SQL operation to complete
func (c *Client) waitForOperation(ctx context.Context, operation *sqladmin.Operation) error {
	for {
		op, err := c.Service.Operations.Get(c.projectID, operation.Name).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to get operation status: %w", err)
		}

		if op.Status == "DONE" {
			if op.Error != nil {
				return fmt.Errorf("operation failed: %v", op.Error)
			}
			return nil
		}

		// Wait before checking again
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			// Continue checking
		}
	}
}

// GetLastScalingTime determines when the instance was last scaled
func (c *Client) GetLastScalingTime(ctx context.Context, instanceName string) (time.Time, error) {
	operations, err := c.GetRecentOperations(ctx, instanceName, 50)
	if err != nil {
		return time.Time{}, err
	}

	for _, op := range operations {
		// Look for update operations that changed the machine type
		if op.OperationType == "UPDATE" && op.Status == "DONE" {
			// Parse the operation insertTime
			insertTime, err := time.Parse(time.RFC3339, op.InsertTime)
			if err != nil {
				continue
			}
			// Note: Would need to inspect operation details to confirm it was a scaling operation
			return insertTime, nil
		}
	}

	return time.Time{}, fmt.Errorf("no recent scaling operations found")
}
