# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build arguments
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build binaries
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/share-disk/share-disk/internal/version.Version=${VERSION} \
    -X github.com/share-disk/share-disk/internal/version.Commit=${COMMIT} \
    -X github.com/share-disk/share-disk/internal/version.BuildTime=${BUILD_TIME}" \
    -o /build/bin/control-server ./server/cmd/control-server

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/share-disk/share-disk/internal/version.Version=${VERSION} \
    -X github.com/share-disk/share-disk/internal/version.Commit=${COMMIT} \
    -X github.com/share-disk/share-disk/internal/version.BuildTime=${BUILD_TIME}" \
    -o /build/bin/control-worker ./server/cmd/control-worker

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/share-disk/share-disk/internal/version.Version=${VERSION} \
    -X github.com/share-disk/share-disk/internal/version.Commit=${COMMIT} \
    -X github.com/share-disk/share-disk/internal/version.BuildTime=${BUILD_TIME}" \
    -o /build/bin/db-migrate ./server/cmd/db-migrate

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/share-disk/share-disk/internal/version.Version=${VERSION} \
    -X github.com/share-disk/share-disk/internal/version.Commit=${COMMIT} \
    -X github.com/share-disk/share-disk/internal/version.BuildTime=${BUILD_TIME}" \
    -o /build/bin/healthcheck ./server/cmd/healthcheck

# Control Server target
FROM alpine:3.19 AS control-server

# Install CA certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 -S share-disk && \
    adduser -u 1000 -S share-disk -G share-disk

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/bin/control-server /app/control-server
COPY --from=builder /build/bin/healthcheck /app/healthcheck

# Copy migrations
COPY --from=builder /build/server/migrations /app/migrations

# Create directories for runtime
RUN mkdir -p /tmp && \
    chown -R share-disk:share-disk /app /tmp

# Switch to non-root user
USER share-disk

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/app/healthcheck", "healthcheck", "--url=http://127.0.0.1:8080/readyz"]

# Run the server
ENTRYPOINT ["/app/control-server"]

# Control Worker target
FROM alpine:3.19 AS control-worker

# Install CA certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 -S share-disk && \
    adduser -u 1000 -S share-disk -G share-disk

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/bin/control-worker /app/control-worker
COPY --from=builder /build/bin/healthcheck /app/healthcheck

# Copy migrations
COPY --from=builder /build/server/migrations /app/migrations

# Create directories for runtime
RUN mkdir -p /tmp && \
    chown -R share-disk:share-disk /app /tmp

# Switch to non-root user
USER share-disk

# Health check - the worker exposes no HTTP API, so it probes PostgreSQL
# connectivity and schema version directly.
HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/healthcheck", "dbcheck"]

# Run the worker
ENTRYPOINT ["/app/control-worker"]

# DB Migrate target
FROM alpine:3.19 AS db-migrate

# Install CA certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 -S share-disk && \
    adduser -u 1000 -S share-disk -G share-disk

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/bin/db-migrate /app/db-migrate

# Copy migrations
COPY --from=builder /build/server/migrations /app/migrations

# Create directories for runtime
RUN mkdir -p /tmp && \
    chown -R share-disk:share-disk /app /tmp

# Switch to non-root user
USER share-disk

# Run migrations
ENTRYPOINT ["/app/db-migrate"]
