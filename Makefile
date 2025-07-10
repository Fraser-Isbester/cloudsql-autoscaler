.PHONY: build clean test fmt vet install help docker-build docker-push docker-run deploy-k8s undeploy-k8s
.DEFAULT_GOAL := help

# Build variables
BINARY_NAME := cloudsql-autoscaler
BUILD_DIR := .
GO_FILES := $(shell find . -name "*.go" -type f)

# Docker variables
REGISTRY := ghcr.io
IMAGE_NAME := fraser-isbester/cloudsql-autoscaler
IMAGE_TAG := $(shell git rev-parse --short HEAD)
FULL_IMAGE := $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
LATEST_IMAGE := $(REGISTRY)/$(IMAGE_NAME):latest

# Kubernetes variables
K8S_NAMESPACE := cloudsql-autoscaler
K8S_MANIFESTS := deploy/kubernetes

## Build the binary
build: $(BUILD_DIR)/$(BINARY_NAME)

$(BUILD_DIR)/$(BINARY_NAME): $(GO_FILES)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

## Install the binary to $GOPATH/bin
install:
	go install ./cmd/$(BINARY_NAME)

## Run tests
test:
	go test -v ./...

## Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## Format code
fmt:
	go fmt ./...

## Run go vet
vet:
	go vet ./...

## Run all checks (fmt, vet, build, test)
check: fmt vet build test

## Clean build artifacts
clean:
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	rm -f coverage.out coverage.html

## Build Docker image
docker-build:
	docker build -t $(FULL_IMAGE) -t $(LATEST_IMAGE) .

## Push Docker image to registry
docker-push: docker-build
	docker push $(FULL_IMAGE)
	docker push $(LATEST_IMAGE)

## Run Docker container locally
docker-run:
	docker run --rm -it $(LATEST_IMAGE) --help

## Deploy to Kubernetes
deploy-k8s:
	kubectl apply -f $(K8S_MANIFESTS)/namespace.yaml
	kubectl apply -f $(K8S_MANIFESTS)/rbac.yaml
	kubectl apply -f $(K8S_MANIFESTS)/configmap.yaml
	kubectl apply -f $(K8S_MANIFESTS)/deployment.yaml
	kubectl apply -f $(K8S_MANIFESTS)/service.yaml

## Remove from Kubernetes
undeploy-k8s:
	kubectl delete -f $(K8S_MANIFESTS)/ --ignore-not-found=true

## Deploy with Kustomize
deploy-kustomize:
	kubectl apply -k $(K8S_MANIFESTS)/

## Check Kubernetes deployment status
k8s-status:
	kubectl get all -n $(K8S_NAMESPACE)
	kubectl describe deployment cloudsql-autoscaler -n $(K8S_NAMESPACE)

## View Kubernetes logs
k8s-logs:
	kubectl logs -n $(K8S_NAMESPACE) deployment/cloudsql-autoscaler -f

## Port forward to access health endpoints
k8s-port-forward:
	kubectl port-forward -n $(K8S_NAMESPACE) svc/cloudsql-autoscaler 8080:8080

## Run linter
lint:
	golangci-lint run
