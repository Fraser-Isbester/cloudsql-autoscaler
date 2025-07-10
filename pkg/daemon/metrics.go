package daemon

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Global flag to track if metrics are enabled
	metricsEnabled = false

	// Prometheus metrics
	autoscalingCycleDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloudsql_autoscaler_cycle_duration_seconds",
		Help: "Duration of the last autoscaling cycle in seconds",
	})

	autoscalingCyclesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cloudsql_autoscaler_cycles_total",
		Help: "Total number of autoscaling cycles completed",
	})

	autoscalingErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudsql_autoscaler_errors_total",
			Help: "Total number of autoscaling errors by type",
		},
		[]string{"error_type"},
	)

	instancesTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloudsql_autoscaler_instances_total",
		Help: "Total number of Cloud SQL instances in the project",
	})

	instancesAnalyzed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloudsql_autoscaler_instances_analyzed",
		Help: "Number of instances successfully analyzed",
	})

	instancesScalable = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloudsql_autoscaler_instances_scalable",
		Help: "Number of instances that need scaling",
	})

	scalingOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudsql_autoscaler_scaling_operations_total",
			Help: "Total number of scaling operations by instance and result",
		},
		[]string{"instance", "result"},
	)

	instanceMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cloudsql_autoscaler_instance_cpu_utilization",
			Help: "Current CPU utilization of Cloud SQL instances",
		},
		[]string{"instance", "project"},
	)

	instanceMemoryMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cloudsql_autoscaler_instance_memory_utilization",
			Help: "Current memory utilization of Cloud SQL instances",
		},
		[]string{"instance", "project"},
	)
)

// InitMetrics initializes Prometheus metrics
func InitMetrics() {
	metricsEnabled = true

	// Register all metrics
	prometheus.MustRegister(
		autoscalingCycleDuration,
		autoscalingCyclesTotal,
		autoscalingErrors,
		instancesTotal,
		instancesAnalyzed,
		instancesScalable,
		scalingOperations,
		instanceMetrics,
		instanceMemoryMetrics,
	)
}

// GetMetricsHandler returns the Prometheus metrics handler
func GetMetricsHandler() http.Handler {
	return promhttp.Handler()
}

// UpdateInstanceMetrics updates the instance-specific metrics
func UpdateInstanceMetrics(projectID, instanceName string, cpuUtil, memoryUtil float64) {
	if metricsEnabled {
		instanceMetrics.WithLabelValues(instanceName, projectID).Set(cpuUtil)
		instanceMemoryMetrics.WithLabelValues(instanceName, projectID).Set(memoryUtil)
	}
}

// RecordScalingOperation records a scaling operation result
func RecordScalingOperation(instanceName, result string) {
	if metricsEnabled {
		scalingOperations.WithLabelValues(instanceName, result).Inc()
	}
}

// RecordError records an error occurrence
func RecordError(errorType string) {
	if metricsEnabled {
		autoscalingErrors.WithLabelValues(errorType).Inc()
	}
}
