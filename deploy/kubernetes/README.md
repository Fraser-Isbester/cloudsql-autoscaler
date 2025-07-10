# Kubernetes Deployment

This directory contains Kubernetes manifests for deploying the CloudSQL Autoscaler.

## Prerequisites

1. **GKE Cluster** with Workload Identity enabled
2. **Google Cloud Service Account** with Cloud SQL Admin permissions
3. **Kubernetes Service Account** linked to GCP Service Account via Workload Identity

## Setup Instructions

### 1. Create Google Cloud Service Account

```bash
# Set your project ID
export PROJECT_ID="your-gcp-project-id"

# Create service account
gcloud iam service-accounts create cloudsql-autoscaler \
  --project=$PROJECT_ID \
  --description="CloudSQL Autoscaler service account" \
  --display-name="CloudSQL Autoscaler"

# Grant necessary permissions
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:cloudsql-autoscaler@$PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/cloudsql.admin"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:cloudsql-autoscaler@$PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/monitoring.viewer"
```

### 2. Configure Workload Identity

```bash
# Get your GKE cluster credentials
gcloud container clusters get-credentials YOUR_CLUSTER_NAME --zone=YOUR_ZONE

# Enable Workload Identity binding
gcloud iam service-accounts add-iam-policy-binding \
  cloudsql-autoscaler@$PROJECT_ID.iam.gserviceaccount.com \
  --role roles/iam.workloadIdentityUser \
  --member "serviceAccount:$PROJECT_ID.svc.id.goog[cloudsql-autoscaler/cloudsql-autoscaler]"
```

### 3. Deploy the Application

#### Option A: Using kubectl

```bash
# Update the project ID in deployment.yaml
sed -i "s/YOUR_PROJECT_ID/$PROJECT_ID/g" deployment.yaml

# Apply all manifests
kubectl apply -f namespace.yaml
kubectl apply -f rbac.yaml
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
```

#### Option B: Using Kustomize

```bash
# Update kustomization.yaml with your project ID
sed -i "s/YOUR_PROJECT_ID/$PROJECT_ID/g" kustomization.yaml

# Apply with kustomize
kubectl apply -k .
```

### 4. Verify Deployment

```bash
# Check pods
kubectl get pods -n cloudsql-autoscaler

# Check logs
kubectl logs -n cloudsql-autoscaler deployment/cloudsql-autoscaler

# Check health
kubectl port-forward -n cloudsql-autoscaler svc/cloudsql-autoscaler 8080:8080
curl http://localhost:8080/health
```

### 5. Monitor with Prometheus (Optional)

If you have Prometheus Operator installed:

```bash
# The ServiceMonitor is automatically created
kubectl get servicemonitor -n cloudsql-autoscaler
```

## Configuration

### Environment Variables

The deployment supports these configuration options via ConfigMap:

- `profile`: Scaling profile (default, conservative, aggressive)
- `interval`: Check interval (e.g., "30m", "1h")
- `dry-run`: Whether to actually apply changes ("true"/"false")
- `http-port`: HTTP server port (default: "8080")
- `metrics`: Enable Prometheus metrics ("true"/"false")

### Scaling Profiles

- **default**: Moderate scaling with 3-day analysis period
- **conservative**: Careful scaling with 14-day analysis period
- **aggressive**: Quick scaling with 3-day analysis period

## Troubleshooting

### Common Issues

1. **Permission Denied**: Ensure Workload Identity is properly configured
2. **No Instances Found**: Verify the GCP project ID is correct
3. **Health Check Failures**: Check resource limits and requests

### Useful Commands

```bash
# View detailed events
kubectl describe pod -n cloudsql-autoscaler

# Check service account annotations
kubectl get sa cloudsql-autoscaler -n cloudsql-autoscaler -o yaml

# Test connectivity to Cloud SQL API
kubectl exec -it -n cloudsql-autoscaler deployment/cloudsql-autoscaler -- /bin/sh
```

## Security Considerations

- The deployment runs as non-root user (65532)
- Uses read-only root filesystem
- Drops all Linux capabilities
- Network policies can be added for additional isolation