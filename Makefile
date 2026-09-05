# Share Disk Makefile
# This Makefile provides standard commands for building, testing, and checking the project.

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOVET=$(GOCMD) vet
GOFMT=gofmt
GOIMPORTS=goimports

# Build parameters
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME?=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
CONTAINER_TEST_PORT?=18080
LAN_AGENT_TEST_PORT?=19090
LDFLAGS=-ldflags "-X github.com/share-disk/share-disk/internal/version.Version=$(VERSION) \
	-X github.com/share-disk/share-disk/internal/version.Commit=$(COMMIT) \
	-X github.com/share-disk/share-disk/internal/version.BuildTime=$(BUILD_TIME)"

# Binary names
CONTROL_SERVER_BIN=bin/control-server
CONTROL_WORKER_BIN=bin/control-worker
DB_MIGRATE_BIN=bin/db-migrate
HEALTHCHECK_BIN=bin/healthcheck
AGENT_BIN=bin/agent
CLI_BIN=bin/share-disk-cli

# All binaries
BINARIES=$(CONTROL_SERVER_BIN) $(CONTROL_WORKER_BIN) $(DB_MIGRATE_BIN) $(HEALTHCHECK_BIN) $(AGENT_BIN) $(CLI_BIN)

# Default target
.PHONY: all
all: check

# Build all binaries
.PHONY: build
build: $(BINARIES)

# Always rebuild binaries to ensure they are up to date
.PHONY: $(CONTROL_SERVER_BIN) $(CONTROL_WORKER_BIN) $(DB_MIGRATE_BIN) $(HEALTHCHECK_BIN) $(AGENT_BIN) $(CLI_BIN)

$(CONTROL_SERVER_BIN):
	$(GOBUILD) $(LDFLAGS) -o $@ ./cmd/control-server

$(CONTROL_WORKER_BIN):
	$(GOBUILD) $(LDFLAGS) -o $@ ./cmd/control-worker

$(DB_MIGRATE_BIN):
	$(GOBUILD) $(LDFLAGS) -o $@ ./cmd/db-migrate

$(HEALTHCHECK_BIN):
	$(GOBUILD) $(LDFLAGS) -o $@ ./cmd/healthcheck

$(AGENT_BIN):
	$(GOBUILD) $(LDFLAGS) -o $@ ./cmd/agent

$(CLI_BIN):
	$(GOBUILD) $(LDFLAGS) -o $@ ./cmd/cli

# Clean build artifacts
.PHONY: clean
clean:
	$(GOCLEAN)
	rm -rf bin/
	rm -rf dist/
	rm -rf build/
	rm -rf coverage/

# Debian package parameters
DEB_VERSION?=0.1.0
DEB_ARCH?=amd64
DEB_PACKAGE=share-disk_$(DEB_VERSION)_$(DEB_ARCH)
DEB_STAGING=build/deb/$(DEB_PACKAGE)

# Build the Ubuntu .deb. The version is taken from DEB_VERSION (unified release
# metadata), not the developer's git checkout. Binaries are built for this exact
# version so /version, the CLI, and the package metadata all report the same
# release.
.PHONY: deb
deb:
	@command -v dpkg-deb >/dev/null 2>&1 || { echo "ERROR: dpkg-deb not found"; exit 1; }
	@echo "Assembling Debian package $(DEB_PACKAGE)..."
	rm -rf $(DEB_STAGING)
	mkdir -p $(DEB_STAGING)/DEBIAN
	mkdir -p $(DEB_STAGING)/usr/bin
	mkdir -p $(DEB_STAGING)/usr/lib/share-disk/migrations
	mkdir -p $(DEB_STAGING)/usr/lib/systemd/system
	mkdir -p $(DEB_STAGING)/etc/share-disk
	mkdir -p $(DEB_STAGING)/usr/share/doc/share-disk
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -ldflags "-X github.com/share-disk/share-disk/internal/version.Version=$(DEB_VERSION) \
	-X github.com/share-disk/share-disk/internal/version.Commit=$(COMMIT) \
	-X github.com/share-disk/share-disk/internal/version.BuildTime=$(BUILD_TIME)" \
	-o $(DEB_STAGING)/usr/lib/share-disk/share-disk-agent ./cmd/agent
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -ldflags "-X github.com/share-disk/share-disk/internal/version.Version=$(DEB_VERSION) \
	-X github.com/share-disk/share-disk/internal/version.Commit=$(COMMIT) \
	-X github.com/share-disk/share-disk/internal/version.BuildTime=$(BUILD_TIME)" \
	-o $(DEB_STAGING)/usr/bin/share-disk-cli ./cmd/cli
	cp -r migrations/sqlite $(DEB_STAGING)/usr/lib/share-disk/migrations/sqlite
	install -m 0644 deploy/deb/etc/share-disk/agent.env $(DEB_STAGING)/etc/share-disk/agent.env
	install -m 0644 deploy/deb/usr/lib/systemd/system/share-disk-agent.service $(DEB_STAGING)/usr/lib/systemd/system/share-disk-agent.service
	install -m 0644 deploy/deb/usr/share/doc/share-disk/copyright $(DEB_STAGING)/usr/share/doc/share-disk/copyright
	installed_size=$$(du -sk $(DEB_STAGING)/usr $(DEB_STAGING)/etc | awk '{total += $$1} END {print total}'); \
		sed -e 's/@VERSION@/$(DEB_VERSION)/g' -e 's/@ARCH@/$(DEB_ARCH)/g' -e "s/@INSTALLED_SIZE@/$$installed_size/g" deploy/deb/DEBIAN/control > $(DEB_STAGING)/DEBIAN/control
	install -m 0755 deploy/deb/DEBIAN/postinst $(DEB_STAGING)/DEBIAN/postinst
	install -m 0755 deploy/deb/DEBIAN/prerm $(DEB_STAGING)/DEBIAN/prerm
	install -m 0755 deploy/deb/DEBIAN/postrm $(DEB_STAGING)/DEBIAN/postrm
	install -m 0644 deploy/deb/DEBIAN/conffiles $(DEB_STAGING)/DEBIAN/conffiles
	chmod -R go-w $(DEB_STAGING)
	mkdir -p dist
	dpkg-deb --root-owner-group --build $(DEB_STAGING) dist/$(DEB_PACKAGE).deb
	@echo "Built dist/$(DEB_PACKAGE).deb"

# Generated protobuf code is gofmt-clean but not goimports-clean; it must not
# be reformatted or checked by goimports.
GO_SOURCES := $(shell find . -name '*.go' -not -name '*.pb.go' -not -path './vendor/*')

# Format code
.PHONY: fmt
fmt:
	$(GOFMT) -s -w $(GO_SOURCES)
	$(GOIMPORTS) -w $(GO_SOURCES)

# Check formatting
.PHONY: fmt-check
fmt-check:
	@echo "Checking formatting..."
	@out=$$($(GOFMT) -l $(GO_SOURCES)); \
	if [ -n "$$out" ]; then \
		echo "Files not formatted:"; \
		echo "$$out"; \
		exit 1; \
	fi
	@echo "Checking imports..."
	@out=$$($(GOIMPORTS) -l $(GO_SOURCES)); \
	if [ -n "$$out" ]; then \
		echo "Files with incorrect imports:"; \
		echo "$$out"; \
		exit 1; \
	fi
	@echo "Formatting OK"

# Run go vet
.PHONY: vet
vet:
	$(GOVET) ./...

# Run staticcheck
.PHONY: staticcheck
staticcheck:
	@echo "Running staticcheck..."
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
	else \
		echo "ERROR: staticcheck not installed. Run 'make tools' to install it."; \
		exit 1; \
	fi

# Run linters
.PHONY: lint
lint: vet staticcheck

# Run unit tests
.PHONY: test
test:
	$(GOTEST) -v -count=1 ./...

# Run unit tests with race detection
.PHONY: test-race
test-race:
	$(GOTEST) -v -race -count=1 ./...

# Run integration tests against a real PostgreSQL. If
# SHARE_DISK_TEST_POSTGRESQL_URL is set (e.g. the CI service container), it is
# used directly. Otherwise a throwaway postgres container is started and torn
# down afterward. Fails closed when neither a DSN nor Docker PostgreSQL is
# available, so a "pass" here always means the tests really talked to PostgreSQL.
.PHONY: test-int
test-int:
	@set -e; \
	if [ -n "$$SHARE_DISK_TEST_POSTGRESQL_URL" ]; then \
		echo "Using SHARE_DISK_TEST_POSTGRESQL_URL for integration tests"; \
		$(GOTEST) -v -count=1 -tags=integration ./...; \
	else \
		port=$${SHARE_DISK_TEST_PG_PORT:-55432}; \
		echo "Starting throwaway PostgreSQL on port $$port..."; \
		docker rm -f share-disk-test-pg >/dev/null 2>&1 || true; \
		docker run -d --name share-disk-test-pg -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=postgres -p $$port:5432 postgres:15 >/dev/null || { echo "ERROR: cannot start PostgreSQL and SHARE_DISK_TEST_POSTGRESQL_URL is not set"; exit 1; }; \
		cleanup() { docker rm -f share-disk-test-pg >/dev/null 2>&1 || true; }; \
		trap cleanup EXIT; \
		for i in $$(seq 1 30); do docker exec share-disk-test-pg pg_isready -U postgres >/dev/null 2>&1 && break; sleep 1; done; \
		SHARE_DISK_TEST_POSTGRESQL_URL="postgres://postgres:postgres@localhost:$$port/postgres?sslmode=disable" $(GOTEST) -v -count=1 -tags=integration ./...; \
	fi

# Run contract tests
.PHONY: contract
contract:
	@echo "Running contract tests..."
	@echo "Checking Protocol Buffers..."
	@command -v protoc >/dev/null 2>&1 || { echo "ERROR: protoc not installed. Run 'make tools' to install it."; exit 1; }
	@command -v protoc-gen-go >/dev/null 2>&1 || { echo "ERROR: protoc-gen-go not installed. Run 'make tools' to install it."; exit 1; }
	@tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	protoc --proto_path=proto --go_out="$$tmp" --go_opt=module=github.com/share-disk/share-disk proto/sharedisk/v1/transfer.proto proto/sharedisk/v1/local.proto; \
	diff -u proto/sharedisk/v1/transfer.pb.go "$$tmp/proto/sharedisk/v1/transfer.pb.go" >/dev/null && \
	diff -u proto/sharedisk/v1/local.pb.go "$$tmp/proto/sharedisk/v1/local.pb.go" >/dev/null || \
	{ echo "ERROR: generated protobuf code is out of date. Run 'make generate'."; exit 1; }
	@echo "Checking OpenAPI contract..."
	$(GOTEST) ./internal/contract/...
	@echo "Contract tests completed"

# Run vulnerability check
.PHONY: vuln
vuln:
	@echo "Running vulnerability check..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "ERROR: govulncheck not installed. Run 'make tools' to install it."; \
		exit 1; \
	fi

# Generate code
.PHONY: generate
generate:
	@command -v protoc >/dev/null 2>&1 || { echo "ERROR: protoc not installed."; exit 1; }
	@command -v protoc-gen-go >/dev/null 2>&1 || { echo "ERROR: protoc-gen-go not installed."; exit 1; }
	protoc --proto_path=proto --go_out=. --go_opt=module=github.com/share-disk/share-disk proto/sharedisk/v1/transfer.proto proto/sharedisk/v1/local.proto

# Run all checks (CI gate)
.PHONY: check
check: fmt-check lint test test-race contract vuln
	@echo "All checks passed!"

# Install development tools (versions pinned via internal/tools/tools.go)
.PHONY: tools
tools:
	@echo "Installing development tools..."
	$(GOCMD) install golang.org/x/tools/cmd/goimports
	$(GOCMD) install honnef.co/go/tools/cmd/staticcheck
	$(GOCMD) install golang.org/x/vuln/cmd/govulncheck
	$(GOCMD) install google.golang.org/protobuf/cmd/protoc-gen-go
	@echo "Installed: goimports staticcheck govulncheck protoc-gen-go"

# Run development server
.PHONY: run-server
run-server: build
	$(CONTROL_SERVER_BIN)

# Run development worker
.PHONY: run-worker
run-worker: build
	$(CONTROL_WORKER_BIN)

# Run development agent
.PHONY: run-agent
run-agent: build
	$(AGENT_BIN)

# Show help
.PHONY: help
help:
	@echo "Share Disk Build System"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all           Run all checks (default)"
	@echo "  build         Build all binaries"
	@echo "  clean         Clean build artifacts"
	@echo "  fmt           Format code"
	@echo "  fmt-check     Check formatting"
	@echo "  vet           Run go vet"
	@echo "  staticcheck   Run staticcheck"
	@echo "  lint          Run all linters"
	@echo "  test          Run unit tests"
	@echo "  test-race     Run unit tests with race detection"
	@echo "  test-int      Run integration tests"
	@echo "  contract      Run contract tests"
	@echo "  vuln          Run vulnerability check"
	@echo "  generate      Generate code"
	@echo "  check         Run all checks (CI gate)"
	@echo "  tools         Install development tools"
	@echo "  run-server    Run development server"
	@echo "  run-worker    Run development worker"
	@echo "  run-agent     Run development agent"
	@echo "  image         Build Docker images"
	@echo "  compose-config Validate Compose configuration"
	@echo "  container-test Run container tests"
	@echo "  lan-e2e       Run the Docker control + Ubuntu Agent LAN data-path E2E"
	@echo "  lan-e2e-docker Run the fully containerized LAN data-path E2E (Agent in Docker)"
	@echo "  image-scan    Scan Docker images for vulnerabilities"
	@echo "  deb           Build the Ubuntu .deb package"
	@echo "  help          Show this help"

# Docker image names
IMAGE_PREFIX?=share-disk
IMAGE_TAG?=$(VERSION)
CONTROL_SERVER_IMAGE=$(IMAGE_PREFIX)/control-server:$(IMAGE_TAG)
CONTROL_WORKER_IMAGE=$(IMAGE_PREFIX)/control-worker:$(IMAGE_TAG)
DB_MIGRATE_IMAGE=$(IMAGE_PREFIX)/db-migrate:$(IMAGE_TAG)
AGENT_IMAGE=$(IMAGE_PREFIX)/agent:$(IMAGE_TAG)

# Build Docker images
.PHONY: image
image:
	@echo "Building Docker images..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--target control-server \
		-t $(CONTROL_SERVER_IMAGE) \
		-f deploy/docker/Dockerfile .
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--target agent \
		-t $(AGENT_IMAGE) \
		-f deploy/docker/Dockerfile .
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--target control-worker \
		-t $(CONTROL_WORKER_IMAGE) \
		-f deploy/docker/Dockerfile .
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--target db-migrate \
		-t $(DB_MIGRATE_IMAGE) \
		-f deploy/docker/Dockerfile .
	@echo "Docker images built successfully"
	@echo "  $(CONTROL_SERVER_IMAGE)"
	@echo "  $(CONTROL_WORKER_IMAGE)"
	@echo "  $(DB_MIGRATE_IMAGE)"
	@echo "  $(AGENT_IMAGE)"

# Validate Compose configuration
.PHONY: compose-config
compose-config:
	@echo "Validating Compose configuration..."
	./deploy/compose/server.py config --quiet
	@echo "Compose configuration is valid"

# Enforce the production boundary (BND-001): the default Compose must not run
# the Ubuntu Agent, must not declare the agent_data volume, and must not publish
# the Agent's 9090 LAN port. The Agent only exists as a test-only overlay or a
# .deb-installed systemd service.
.PHONY: compose-boundary
compose-boundary:
	@echo "Checking production Compose boundary..."
	@services="$$(./deploy/compose/server.py config --services)"; \
	if printf '%s\n' "$$services" | grep -qx 'agent'; then \
		echo "ERROR: default Compose must not contain an 'agent' service"; exit 1; \
	fi
	@volumes="$$(./deploy/compose/server.py config --volumes)"; \
	if printf '%s\n' "$$volumes" | grep -qx 'agent_data'; then \
		echo "ERROR: default Compose must not declare an 'agent_data' volume"; exit 1; \
	fi
	@if grep -Eq '(^|[^0-9])9090:9090([^0-9]|$$)' deploy/compose/compose.yaml; then \
		echo "ERROR: default Compose must not publish the Agent 9090 LAN port"; exit 1; \
	fi
	@echo "Compose boundary OK: backend-only default, Agent only in test overlay"

# Run container tests: build images, start a real stack, wait for readiness,
# exercise a protected API, then tear down. Fails closed on any error and
# always tears down and removes generated secrets via a trap.
.PHONY: container-test
container-test: image compose-config
	@echo "Preparing ephemeral secrets for the test stack..."
	@set -e; \
	sec_dir=deploy/compose/secrets; \
	mkdir -p "$$sec_dir"; \
	for s in bootstrap_token; do \
		openssl rand -hex 32 > "$$sec_dir/$$s"; \
	done; \
	openssl genpkey -algorithm ED25519 -out "$$sec_dir/access_private_key"; \
	openssl pkey -in "$$sec_dir/access_private_key" -pubout -out "$$sec_dir/access_public_key"; \
	pg_password="$$(openssl rand -hex 32)"; \
	printf '%s' "$$pg_password" > "$$sec_dir/postgres_password"; \
	printf 'postgres://share_disk:%s@postgres:5432/share_disk?sslmode=disable' "$$pg_password" > "$$sec_dir/postgresql_url"; \
	cleanup() { \
		echo "Tearing down container stack..."; \
		(cd deploy/compose && docker compose down -v) 2>/dev/null || true; \
		rm -f "$$sec_dir/bootstrap_token" "$$sec_dir/access_private_key" "$$sec_dir/access_public_key" "$$sec_dir/postgres_password" "$$sec_dir/postgresql_url"; \
	}; \
	trap cleanup EXIT; \
	echo "Starting container stack..."; \
	(cd deploy/compose && \
	  IMAGE_PREFIX=$(IMAGE_PREFIX) \
	  IMAGE_TAG=$(IMAGE_TAG) \
	  CONTROL_SERVER_BIND_ADDRESS=127.0.0.1 \
	  CONTROL_SERVER_PORT=$(CONTAINER_TEST_PORT) \
	  POSTGRES_PASSWORD_FILE=./secrets/postgres_password \
	  POSTGRESQL_URL_FILE=./secrets/postgresql_url \
	  BOOTSTRAP_TOKEN_FILE=./secrets/bootstrap_token \
	  ACCESS_PRIVATE_KEY_FILE=./secrets/access_private_key \
	  docker compose up -d --wait postgres db-migrate control-server control-worker); \
	echo "Verifying readiness..."; \
	(cd deploy/compose && \
	  curl -fsS --retry 30 --retry-connrefused --retry-delay 2 \
	    http://127.0.0.1:$(CONTAINER_TEST_PORT)/readyz >/dev/null) || { echo "ERROR: server never became ready"; exit 1; }; \
		echo "Container stack passed readiness."

# Prove the deployable LAN vertical slice using the real Docker control plane,
# a real Ubuntu Agent process, real TCP sockets and persisted file bytes.
.PHONY: lan-e2e
lan-e2e: build image compose-config
	IMAGE_PREFIX=$(IMAGE_PREFIX) IMAGE_TAG=$(IMAGE_TAG) \
		CONTROL_SERVER_TEST_PORT=$(CONTAINER_TEST_PORT) LAN_AGENT_TEST_PORT=$(LAN_AGENT_TEST_PORT) \
		bash test/e2e/lan_upload_download.sh

# Prove the fully containerized LAN shape: the Agent runs as a Docker container
# (not a host process) and still serves authenticated uploads, list, Range/full
# download, SHA-256, file lifecycle operations and survives a container restart.
.PHONY: lan-e2e-docker
lan-e2e-docker: image compose-config
	IMAGE_PREFIX=$(IMAGE_PREFIX) IMAGE_TAG=$(IMAGE_TAG) \
		CONTROL_SERVER_TEST_PORT=$(CONTAINER_TEST_PORT) LAN_AGENT_TEST_PORT=$(LAN_AGENT_TEST_PORT) \
		bash test/e2e/lan_docker_closed_loop.sh

# Scan Docker images for vulnerabilities
.PHONY: image-scan
image-scan: image
	@echo "Scanning Docker images for vulnerabilities..."
	@if command -v trivy >/dev/null 2>&1; then \
		trivy image $(CONTROL_SERVER_IMAGE); \
		trivy image $(CONTROL_WORKER_IMAGE); \
		trivy image $(DB_MIGRATE_IMAGE); \
	elif command -v docker scout >/dev/null 2>&1; then \
		docker scout cves $(CONTROL_SERVER_IMAGE); \
		docker scout cves $(CONTROL_WORKER_IMAGE); \
		docker scout cves $(DB_MIGRATE_IMAGE); \
	else \
		echo "WARNING: No vulnerability scanner found. Install trivy or use Docker Scout."; \
		exit 1; \
	fi
	@echo "Image scan completed"
