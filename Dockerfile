# Multi-stage Dockerfile for cloudsql-autoscaler
# Stage 1: Build environment
FROM golang:1.24.4-alpine AS builder

# Set working directory
WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o cloudsql-autoscaler \
    ./cmd/cloudsql-autoscaler

# Stage 2: Runtime environment using distroless
FROM gcr.io/distroless/static-debian12:nonroot

# Copy timezone data and certificates from builder
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary
COPY --from=builder /app/cloudsql-autoscaler /usr/local/bin/cloudsql-autoscaler

# Use non-root user from distroless
USER nonroot:nonroot

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/cloudsql-autoscaler"]

# Default to help command
CMD ["--help"]

# Health check (will be implemented when we add health endpoints)
# HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
#   CMD ["/usr/local/bin/cloudsql-autoscaler", "health"]

# Labels for metadata
LABEL org.opencontainers.image.title="CloudSQL Autoscaler"
LABEL org.opencontainers.image.description="Automatic scaling for Google Cloud SQL instances"
LABEL org.opencontainers.image.vendor="fraser-isbester"
LABEL org.opencontainers.image.source="https://github.com/fraser-isbester/cloudsql-autoscaler"
LABEL org.opencontainers.image.licenses="Apache-2.0"