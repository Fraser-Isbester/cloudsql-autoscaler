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