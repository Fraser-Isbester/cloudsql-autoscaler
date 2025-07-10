package cloudsql

import (
	"context"
	"fmt"
	"sort"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

// MetricsClient handles Cloud Monitoring metrics retrieval
type MetricsClient struct {
	client    *monitoring.MetricClient
	projectID string
}

// NewMetricsClient creates a new metrics client
func NewMetricsClient(ctx context.Context, projectID string) (*MetricsClient, error) {
	client, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics client: %w", err)
	}

	return &MetricsClient{
		client:    client,
		projectID: projectID,
	}, nil
}

// Close closes the metrics client
func (m *MetricsClient) Close() error {
	return m.client.Close()
}

// GetInstanceMetrics retrieves metrics for a Cloud SQL instance
func (m *MetricsClient) GetInstanceMetrics(ctx context.Context, instanceID string, cfg *config.Config) (*config.MetricsData, error) {
	endTime := time.Now()
	startTime := endTime.Add(-cfg.MetricsPeriod)

	metrics := &config.MetricsData{
		Timestamps:     []time.Time{},
		CPUUtilization: []float64{},
		MemoryUsageGB:  []float64{},
		MemoryPercent:  []float64{},
		Connections:    []int{},
		DiskUsageGB:    []float64{},
		DiskIOPS:       []float64{},
	}

	// Fetch CPU utilization
	cpuData, err := m.fetchMetric(ctx, instanceID, "cloudsql.googleapis.com/database/cpu/utilization", startTime, endTime, cfg.MetricsInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch CPU metrics: %w", err)
	}

	// Fetch memory utilization
	memoryData, err := m.fetchMetric(ctx, instanceID, "cloudsql.googleapis.com/database/memory/utilization", startTime, endTime, cfg.MetricsInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch memory metrics: %w", err)
	}

	// Fetch memory usage in bytes
	memoryBytesData, err := m.fetchMetric(ctx, instanceID, "cloudsql.googleapis.com/database/memory/usage", startTime, endTime, cfg.MetricsInterval)
	if err != nil {
		// Non-fatal: some instances might not report this metric
		memoryBytesData = make(map[time.Time]float64)
	}

	// Fetch active connections
	connectionsData, err := m.fetchMetric(ctx, instanceID, "cloudsql.googleapis.com/database/postgresql/num_backends", startTime, endTime, cfg.MetricsInterval)
	if err != nil {
		// Non-fatal: metric name varies by database type
		connectionsData = make(map[time.Time]float64)
	}

	// Combine all metrics into aligned time series
	allTimestamps := make(map[time.Time]bool)
	for ts := range cpuData {
		allTimestamps[ts] = true
	}

	// Convert to sorted slice
	for ts := range allTimestamps {
		metrics.Timestamps = append(metrics.Timestamps, ts)
	}
	sort.Slice(metrics.Timestamps, func(i, j int) bool {
		return metrics.Timestamps[i].Before(metrics.Timestamps[j])
	})

	// Align all metrics to common timestamps
	for _, ts := range metrics.Timestamps {
		if cpu, ok := cpuData[ts]; ok {
			metrics.CPUUtilization = append(metrics.CPUUtilization, cpu*100) // Convert to percentage
		} else {
			metrics.CPUUtilization = append(metrics.CPUUtilization, 0)
		}

		if memPct, ok := memoryData[ts]; ok {
			metrics.MemoryPercent = append(metrics.MemoryPercent, memPct*100) // Convert to percentage
		} else {
			metrics.MemoryPercent = append(metrics.MemoryPercent, 0)
		}

		if memBytes, ok := memoryBytesData[ts]; ok {
			metrics.MemoryUsageGB = append(metrics.MemoryUsageGB, memBytes/1024/1024/1024) // Convert to GB
		} else {
			metrics.MemoryUsageGB = append(metrics.MemoryUsageGB, 0)
		}

		if conns, ok := connectionsData[ts]; ok {
			metrics.Connections = append(metrics.Connections, int(conns))
		} else {
			metrics.Connections = append(metrics.Connections, 0)
		}
	}

	return metrics, nil
}

// fetchMetric retrieves a specific metric time series
func (m *MetricsClient) fetchMetric(ctx context.Context, instanceID string, metricType string, startTime, endTime time.Time, interval time.Duration) (map[time.Time]float64, error) {
	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", m.projectID),
		Filter: fmt.Sprintf(`resource.type="cloudsql_database" AND resource.labels.database_id="%s:%s" AND metric.type="%s"`, m.projectID, instanceID, metricType),
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamppb.New(startTime),
			EndTime:   timestamppb.New(endTime),
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:    durationpb.New(interval),
			PerSeriesAligner:   monitoringpb.Aggregation_ALIGN_MEAN,
			CrossSeriesReducer: monitoringpb.Aggregation_REDUCE_MEAN,
		},
	}

	data := make(map[time.Time]float64)
	it := m.client.ListTimeSeries(ctx, req)

	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error iterating time series: %w", err)
		}

		for _, point := range resp.Points {
			timestamp := point.Interval.EndTime.AsTime()
			value := extractValue(point.Value)
			data[timestamp] = value
		}
	}

	return data, nil
}

// extractValue extracts the numeric value from a metric point
func extractValue(v *monitoringpb.TypedValue) float64 {
	switch v.Value.(type) {
	case *monitoringpb.TypedValue_DoubleValue:
		return v.GetDoubleValue()
	case *monitoringpb.TypedValue_Int64Value:
		return float64(v.GetInt64Value())
	default:
		return 0
	}
}

// CalculateMetricsSummary calculates statistical summary from metrics data
func CalculateMetricsSummary(data *config.MetricsData) *config.MetricsSummary {
	summary := &config.MetricsSummary{
		DataPoints: len(data.Timestamps),
	}

	// Handle empty data gracefully
	if len(data.Timestamps) == 0 {
		summary.Period = 0
		return summary
	}

	summary.Period = data.Timestamps[len(data.Timestamps)-1].Sub(data.Timestamps[0])

	// Calculate CPU statistics
	summary.CPUAvg = calculateAverage(data.CPUUtilization)
	summary.CPUP95 = calculatePercentile(data.CPUUtilization, 95)
	summary.CPUP99 = calculatePercentile(data.CPUUtilization, 99)
	summary.CPUMax = calculateMax(data.CPUUtilization)

	// Calculate Memory statistics
	summary.MemoryAvgGB = calculateAverage(data.MemoryUsageGB)
	summary.MemoryP95GB = calculatePercentile(data.MemoryUsageGB, 95)
	summary.MemoryP99GB = calculatePercentile(data.MemoryUsageGB, 99)
	summary.MemoryMaxGB = calculateMax(data.MemoryUsageGB)

	summary.MemoryAvgPct = calculateAverage(data.MemoryPercent)
	summary.MemoryP95Pct = calculatePercentile(data.MemoryPercent, 95)
	summary.MemoryP99Pct = calculatePercentile(data.MemoryPercent, 99)

	// Calculate connection statistics
	summary.ConnectionsAvg = calculateAverage(toFloat64Slice(data.Connections))
	summary.ConnectionsMax = calculateMaxInt(data.Connections)

	return summary
}

// Statistical helper functions
func calculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateMax(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}

func calculateMaxInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}

func calculatePercentile(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Copy and sort values
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	// Calculate percentile index
	index := (percentile / 100) * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	// Linear interpolation
	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func toFloat64Slice(ints []int) []float64 {
	floats := make([]float64, len(ints))
	for i, v := range ints {
		floats[i] = float64(v)
	}
	return floats
}
