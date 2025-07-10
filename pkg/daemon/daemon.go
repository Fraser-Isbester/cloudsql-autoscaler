package daemon

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/analyzer"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

// Daemon represents the continuous autoscaler daemon
// Refactored to use composition following Russ Cox's design principles
type Daemon struct {
	config        Config
	runner        CycleRunner
	httpServer    HTTPServerInterface
	signalHandler SignalHandler

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// DaemonConfig holds daemon-specific configuration
type DaemonConfig struct {
	Interval      time.Duration // How often to run autoscaling checks
	HTTPPort      int           // Port for health checks and metrics
	EnableMetrics bool          // Whether to enable Prometheus metrics
}

// NewDaemon creates a new daemon instance with improved composition
func NewDaemon(cfg *config.Config, daemonCfg *DaemonConfig) (*Daemon, error) {
	// Validate configuration early
	if err := validateConfig(cfg, daemonCfg.Interval, daemonCfg.HTTPPort); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create analyzer - keeping this concrete type as it's the main dependency
	projectAnalyzer, err := analyzer.NewProjectAnalyzer(ctx, cfg)
	if err != nil {
		cancel()
		return nil, NewDaemonError("create_analyzer", "startup", err)
	}

	// Create configuration wrapper
	daemonConfig := NewDaemonConfig(cfg, daemonCfg.Interval, daemonCfg.HTTPPort, daemonCfg.EnableMetrics)

	// Create metrics reporter based on configuration
	var metricsReporter MetricsReporter
	if daemonCfg.EnableMetrics {
		metricsReporter = NewPrometheusMetricsReporter()
	} else {
		metricsReporter = NewSimpleMetricsReporter()
	}

	// Create cycle runner with dependencies injected
	runner := NewAutoscalingRunner(projectAnalyzer, daemonConfig, metricsReporter)

	// Create HTTP server for health checks and metrics
	httpServer := &HTTPServer{
		port:   daemonCfg.HTTPPort,
		daemon: nil, // Will be set after daemon creation
	}

	// Create signal handler
	signalHandler := NewOSSignalHandler()

	return &Daemon{
		config:        daemonConfig,
		runner:        runner,
		httpServer:    httpServer,
		signalHandler: signalHandler,
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

// Start begins the daemon operation using improved composition
func (d *Daemon) Start() error {
	log.Printf("Starting CloudSQL Autoscaler daemon (interval: %v, project: %s)",
		d.config.GetInterval(), d.config.GetProjectID())

	// Start HTTP server for health checks and metrics
	if d.config.GetHTTPPort() > 0 {
		d.wg.Add(1)
		go d.startHTTPServer()
	}

	// Start main autoscaling loop
	d.wg.Add(1)
	go d.autoscalingLoop()

	// Wait for shutdown signal
	<-d.signalHandler.WaitForShutdown()

	// Initiate graceful shutdown
	d.Stop()

	// Wait for all goroutines to complete
	d.wg.Wait()

	log.Println("Daemon stopped gracefully")
	return nil
}

// Stop gracefully stops the daemon
func (d *Daemon) Stop() {
	log.Println("Initiating graceful shutdown...")
	d.cancel()
}

// autoscalingLoop runs the main autoscaling logic at regular intervals
// Simplified to use the CycleRunner interface
func (d *Daemon) autoscalingLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.GetInterval())
	defer ticker.Stop()

	// Run once immediately on startup
	d.runAutoscalingCycle()

	for {
		select {
		case <-ticker.C:
			d.runAutoscalingCycle()
		case <-d.ctx.Done():
			log.Println("Autoscaling loop stopped")
			return
		}
	}
}

// runAutoscalingCycle executes a single autoscaling cycle using the CycleRunner
func (d *Daemon) runAutoscalingCycle() {
	if err := d.runner.RunCycle(d.ctx); err != nil {
		// Log error but continue - following the principle of robustness
		log.Printf("Autoscaling cycle failed: %v", err)
		if !IsRecoverable(err) {
			log.Printf("Non-recoverable error detected, continuing anyway")
		}
	}
}

// startHTTPServer starts the HTTP server for health checks and metrics
func (d *Daemon) startHTTPServer() {
	defer d.wg.Done()

	go func() {
		if err := d.httpServer.Start(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-d.ctx.Done()

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := d.httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
}

// GetStatus returns the current daemon status
func (d *Daemon) GetStatus() *DaemonStatus {
	return &DaemonStatus{
		ProjectID: d.config.GetProjectID(),
		Interval:  d.config.GetInterval(),
		DryRun:    d.config.IsDryRun(),
		HTTPPort:  d.config.GetHTTPPort(),
		Running:   true,
		StartTime: time.Now(), // This would be set properly in a real implementation
	}
}

// DaemonStatus represents the current status of the daemon
type DaemonStatus struct {
	ProjectID string        `json:"project_id"`
	Interval  time.Duration `json:"interval"`
	DryRun    bool          `json:"dry_run"`
	HTTPPort  int           `json:"http_port"`
	Running   bool          `json:"running"`
	StartTime time.Time     `json:"start_time"`
	LastCycle time.Time     `json:"last_cycle,omitempty"`
	NextCycle time.Time     `json:"next_cycle,omitempty"`
}
