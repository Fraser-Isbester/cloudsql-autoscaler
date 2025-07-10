# CloudSQL Autoscaler

Automatically scales Google Cloud SQL instances based on CPU and memory utilization patterns. Supports both one-shot CLI usage and continuous daemon mode for production deployments.

## Features

- **Smart Analysis**: Uses 3 days of metrics and P95 percentiles to avoid scaling on temporary spikes
- **Enterprise Aware**: Understands downtime constraints for Enterprise vs Enterprise Plus editions
- **Multiple Modes**: CLI tool, Docker container, or Kubernetes deployment
- **Configurable**: Conservative, default, and aggressive scaling profiles
- **Observable**: Built-in Prometheus metrics and health endpoints

## Quick Start

```bash
# Install
go install github.com/fraser-isbester/cloudsql-autoscaler/cmd/cloudsql-autoscaler@latest

# Analyze all instances (dry-run by default)
cloudsql-autoscaler --project my-gcp-project

# Apply scaling recommendations
cloudsql-autoscaler --project my-gcp-project --dry-run=false

# Analyze specific instances
cloudsql-autoscaler --project my-gcp-project --instance db1 --instance db2
```

## Usage Options

### Basic Commands
```bash
# Core flags
--project string       GCP project ID
--instance strings     Specific instance(s) to analyze (default: all)
--dry-run             Show recommendations without applying (default: true)
--output string       Format: table or json (default: table)

# Daemon mode for continuous operation
--daemon              # Run continuously
--interval duration   # Check interval (default: 30m)
--http-port int       # Health/metrics port (default: 8080)
```

### Example Commands
```bash
# One-shot analysis with JSON output
cloudsql-autoscaler --project my-project --output json

# Conservative scaling for production
cloudsql-autoscaler --project my-project --profile conservative --dry-run=false

# Continuous monitoring with 15-minute intervals
cloudsql-autoscaler --daemon --project my-project --interval=15m
```

## Deployment Options

### Docker
```bash
# Pull and run
docker pull ghcr.io/fraser-isbester/cloudsql-autoscaler:latest
docker run -d \
  -e GOOGLE_APPLICATION_CREDENTIALS=/creds.json \
  -v /path/to/creds.json:/creds.json:ro \
  ghcr.io/fraser-isbester/cloudsql-autoscaler:latest \
  --daemon --project my-gcp-project
```

### Kubernetes
```bash
# Clone and deploy
git clone https://github.com/fraser-isbester/cloudsql-autoscaler.git
cd cloudsql-autoscaler
make deploy-k8s

# Check status
make k8s-status
make k8s-logs
```

**Prerequisites for Kubernetes:**
- GKE cluster with Workload Identity enabled
- Google Cloud Service Account with Cloud SQL Admin permissions

See `deploy/kubernetes/README.md` for complete setup instructions.

## Monitoring

When running in daemon mode, health and metrics endpoints are available:

```bash
curl http://localhost:8080/health   # Health check
curl http://localhost:8080/ready    # Readiness probe
curl http://localhost:8080/status   # Detailed status
curl http://localhost:8080/metrics  # Prometheus metrics
```

**Key Metrics:**
- `cloudsql_autoscaler_instances_total` - Total instances in project
- `cloudsql_autoscaler_instances_scalable` - Instances needing scaling
- `cloudsql_autoscaler_scaling_operations_total` - Scaling operations by result
- `cloudsql_autoscaler_cycle_duration_seconds` - Analysis cycle duration

## How it Works

1. **Collects Metrics**: Gathers 3 days of CPU/memory data from Cloud Monitoring
2. **Analyzes Patterns**: Uses P95 percentiles to identify sustained load vs spikes
3. **Recommends Changes**: Suggests machine type upgrades/downgrades within constraints
4. **Respects Limits**: Understands Enterprise Plus zero-downtime windows vs Enterprise downtime requirements
5. **Applies Safely**: Optionally executes changes with proper error handling and rollback

**Supported Machine Types:**
- Standard: `db-f1-micro`, `db-g1-small`, `db-n1-*`, `db-n2-*`, `db-e2-*`
- Custom: `db-custom-{vcpus}-{memory_mb}`
- Performance: `db-perf-optimized-N-*`