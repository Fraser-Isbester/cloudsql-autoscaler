# cloudsql-autoscaler
This is an autoscaling controller for GCP Cloud SQL DB instances. It leverages the Cloud SQL Admin API to monitor and adjust machine type of instances based on various trailing memory & cpu load patterns.

It can be run standalone or as a Kubernetes process.

## Usage

As a CLI
```bash
$ go install github.com/fraser-isbester/cloudsql-autoscaler@latest
...
$ cloudsql-autoscaler --project <gcp-project-id> --instance <cloud-sql-instance-name> --dry-run
...
$ cloudsql-autoscaler --project <gcp-project-id> --dry-run
```

As a Kubernetes controller
```bash
$ kubectl apply -f https://raw.githubusercontent.com/fraser-isbester/cloudsql-autoscaler/main/deploy/kubernetes.yaml
```

## Features

- Automatic scaling of Cloud SQL instances based on historic load.
- Easy integration with existing GCP projects.
- Runnable as a standalone CLI or as a Kubernetes controller.

## How it works
The autoscaler monitors the CPU and memory usage of Cloud SQL instances over time. It can be given an instance or self discover them. Based on the configuration & autoscaling profile it will read historic load data of the instance(s) and adjust the machine type accordingly. Enterprise Plus instances can be autoscaled up about once every ~30 minutes, and scaled down about once every ~3 hours without meaningful downtime (subsecond). Autoscaling outside those bounds is possible but will incur downtime -- the autoscaler will warn you if you attempt to do this.

## Usage

```bash
# Analyze all instances in the project (dry-run by default)
cloudsql-autoscaler

# Analyze specific instance(s)
cloudsql-autoscaler --instance my-instance
cloudsql-autoscaler --instance db1 --instance db2

# Actually apply scaling recommendations
cloudsql-autoscaler --dry-run=false

# Use different scaling profiles
cloudsql-autoscaler --profile conservative  # Less aggressive scaling
cloudsql-autoscaler --profile aggressive    # More responsive to load changes

# Specify project (optional if Application Default Credentials are configured)
cloudsql-autoscaler --project my-gcp-project

# Output as JSON for automation/integration
cloudsql-autoscaler --output json
```

### Scaling Profiles

- **default**: Scale up at 80% utilization, down at 50%, requires 1 hour sustained load, analyzes 7 days of data
- **conservative**: Scale up at 90%, down at 30%, requires 2 hours sustained load, analyzes 14 days of data  
- **aggressive**: Scale up at 70%, down at 60%, requires 30 minutes sustained load, analyzes 3 days of data

### Machine Type Support

- Standard machine types (db-f1-micro, db-g1-small, db-n1-*, db-n2-*, db-e2-*)
- Custom machine types (db-custom-{vcpus}-{memory_mb})
- Performance-optimized machine types (db-perf-optimized-N-*)

The autoscaler respects Cloud SQL constraints and will provide appropriate scaling recommendations within the same machine series.

### Output Formats

- **table** (default): Human-readable table format with clear status messages
- **json**: Structured JSON output perfect for automation, monitoring, and integration with other tools

JSON output includes:
- Project and instance details
- Current and recommended machine types  
- Scaling actions taken or recommended
- Error details if any
- Timestamps and metadata