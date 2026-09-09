# Share Disk Makefile
# This Makefile provides build, packaging, and production-source checks.

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOVET=$(GOCMD) vet
GOFMT=gofmt
GOIMPORTS=goimports

# Build parameters
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME?=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags "-X github.com/share-disk/share-disk/internal/version.Version=$(VERSION) \
	-X github.com/share-disk/share-disk/internal/version.Commit=$(COMMIT) \
	-X github.com/share-disk/share-disk/internal/version.BuildTime=$(BUILD_TIME)"

# Binary names. build/ is transient; release/ contains versioned deliverables.
CONTROL_SERVER_BIN=build/bin/control-server
CONTROL_WORKER_BIN=build/bin/control-worker
DB_MIGRATE_BIN=build/bin/db-migrate
HEALTHCHECK_BIN=build/bin/healthcheck
AGENT_BIN=build/bin/agent
CLI_BIN=build/bin/share-disk-cli

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
	@mkdir -p $(@D)
	$(GOBUILD) $(LDFLAGS) -o $@ ./server/cmd/control-server

$(CONTROL_WORKER_BIN):
	@mkdir -p $(@D)
	$(GOBUILD) $(LDFLAGS) -o $@ ./server/cmd/control-worker

$(DB_MIGRATE_BIN):
	@mkdir -p $(@D)
	$(GOBUILD) $(LDFLAGS) -o $@ ./server/cmd/db-migrate

$(HEALTHCHECK_BIN):
	@mkdir -p $(@D)
	$(GOBUILD) $(LDFLAGS) -o $@ ./server/cmd/healthcheck

$(AGENT_BIN):
	@mkdir -p $(@D)
	$(GOBUILD) $(LDFLAGS) -o $@ ./client/ubuntu/cmd/agent

$(CLI_BIN):
	@mkdir -p $(@D)
	$(GOBUILD) $(LDFLAGS) -o $@ ./client/ubuntu/cmd/cli

# Clean build artifacts
.PHONY: clean
clean:
	$(GOCLEAN)
	rm -rf build/
	rm -rf coverage/

# Debian package parameters
DEB_VERSION?=0.1.0
DEB_ARCH?=amd64
DEB_PACKAGE=share-disk_$(DEB_VERSION)_$(DEB_ARCH)
DEB_STAGING=build/package/deb/$(DEB_PACKAGE)
DEB_RELEASE=release/ubuntu/$(DEB_PACKAGE).deb
ANDROID_VERSION?=0.3.0-lan
ANDROID_APK=release/android/share-disk_$(ANDROID_VERSION).apk
WINDOWS_VERSION?=0.1.0

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
	-o $(DEB_STAGING)/usr/lib/share-disk/share-disk-agent ./client/ubuntu/cmd/agent
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -ldflags "-X github.com/share-disk/share-disk/internal/version.Version=$(DEB_VERSION) \
	-X github.com/share-disk/share-disk/internal/version.Commit=$(COMMIT) \
	-X github.com/share-disk/share-disk/internal/version.BuildTime=$(BUILD_TIME)" \
	-o $(DEB_STAGING)/usr/bin/share-disk-cli ./client/ubuntu/cmd/cli
	cp -r client/ubuntu/migrations/sqlite $(DEB_STAGING)/usr/lib/share-disk/migrations/sqlite
	install -m 0644 client/ubuntu/packaging/deb/etc/share-disk/agent.env $(DEB_STAGING)/etc/share-disk/agent.env
	install -m 0644 client/ubuntu/packaging/deb/usr/lib/systemd/system/share-disk-agent.service $(DEB_STAGING)/usr/lib/systemd/system/share-disk-agent.service
	install -m 0644 client/ubuntu/packaging/deb/usr/share/doc/share-disk/copyright $(DEB_STAGING)/usr/share/doc/share-disk/copyright
	installed_size=$$(du -sk $(DEB_STAGING)/usr $(DEB_STAGING)/etc | awk '{total += $$1} END {print total}'); \
		sed -e 's/@VERSION@/$(DEB_VERSION)/g' -e 's/@ARCH@/$(DEB_ARCH)/g' -e "s/@INSTALLED_SIZE@/$$installed_size/g" client/ubuntu/packaging/deb/DEBIAN/control > $(DEB_STAGING)/DEBIAN/control
	install -m 0755 client/ubuntu/packaging/deb/DEBIAN/postinst $(DEB_STAGING)/DEBIAN/postinst
	install -m 0755 client/ubuntu/packaging/deb/DEBIAN/prerm $(DEB_STAGING)/DEBIAN/prerm
	install -m 0755 client/ubuntu/packaging/deb/DEBIAN/postrm $(DEB_STAGING)/DEBIAN/postrm
	install -m 0644 client/ubuntu/packaging/deb/DEBIAN/conffiles $(DEB_STAGING)/DEBIAN/conffiles
	chmod -R go-w $(DEB_STAGING)
	mkdir -p release/ubuntu
	dpkg-deb --root-owner-group --build $(DEB_STAGING) $(DEB_RELEASE)
	@echo "Built $(DEB_RELEASE)"

# Build a signed Android release APK. Signing credentials stay outside the
# repository and are consumed by client/android/app/build.gradle.
.PHONY: apk
apk:
	@test -n "$$SHARE_DISK_ANDROID_KEYSTORE" || { echo "ERROR: SHARE_DISK_ANDROID_KEYSTORE is required"; exit 1; }
	@test -n "$$SHARE_DISK_ANDROID_KEYSTORE_PASSWORD" || { echo "ERROR: SHARE_DISK_ANDROID_KEYSTORE_PASSWORD is required"; exit 1; }
	@test -n "$$SHARE_DISK_ANDROID_KEY_ALIAS" || { echo "ERROR: SHARE_DISK_ANDROID_KEY_ALIAS is required"; exit 1; }
	@test -n "$$SHARE_DISK_ANDROID_KEY_PASSWORD" || { echo "ERROR: SHARE_DISK_ANDROID_KEY_PASSWORD is required"; exit 1; }
	./client/android/gradlew -p client/android --no-daemon lintRelease assembleRelease
	@test -f client/android/app/build/outputs/apk/release/app-release.apk || { echo "ERROR: signed release APK was not produced"; exit 1; }
	mkdir -p release/android
	cp client/android/app/build/outputs/apk/release/app-release.apk $(ANDROID_APK)
	@echo "Built $(ANDROID_APK)"

# Build the Windows desktop client and portable release archive.
.PHONY: windows
windows:
	pwsh -NoProfile -File client/windows/build.ps1 -Version $(WINDOWS_VERSION)

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
		echo "ERROR: staticcheck not installed."; \
		exit 1; \
	fi

# Run linters
.PHONY: lint
lint: vet staticcheck

# Verify generated protocol assets.
.PHONY: contract
contract:
	@echo "Checking generated protocol assets..."
	@echo "Checking Protocol Buffers..."
	@command -v protoc >/dev/null 2>&1 || { echo "ERROR: protoc not installed."; exit 1; }
	@command -v protoc-gen-go >/dev/null 2>&1 || { echo "ERROR: protoc-gen-go not installed."; exit 1; }
	@tmp=$$(mktemp -d); \
	trap 'rm -rf "$$tmp"' EXIT; \
	protoc --proto_path=contracts/proto --go_out="$$tmp" --go_opt=module=github.com/share-disk/share-disk contracts/proto/sharedisk/v1/transfer.proto contracts/proto/sharedisk/v1/local.proto; \
	diff -u contracts/proto/sharedisk/v1/transfer.pb.go "$$tmp/contracts/proto/sharedisk/v1/transfer.pb.go" >/dev/null && \
	diff -u contracts/proto/sharedisk/v1/local.pb.go "$$tmp/contracts/proto/sharedisk/v1/local.pb.go" >/dev/null || \
	{ echo "ERROR: generated protobuf code is out of date. Run 'make generate'."; exit 1; }
	@echo "Protocol assets are current"

# Enforce product ownership. Shared code may live under root internal/ and
# contracts/, but product-private packages must not cross this boundary.
.PHONY: architecture
architecture:
	@echo "Checking product dependency boundaries..."
	@if $(GOCMD) list -deps ./server/... | grep -q '^github.com/share-disk/share-disk/client/'; then \
		echo "ERROR: server packages must not import client packages"; exit 1; \
	fi
	@if $(GOCMD) list -deps ./client/ubuntu/... | grep -q '^github.com/share-disk/share-disk/server/'; then \
		echo "ERROR: Ubuntu client packages must not import server packages"; exit 1; \
	fi
	@echo "Product dependency boundaries are clean"

# Run vulnerability check
.PHONY: vuln
vuln:
	@echo "Running vulnerability check..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "ERROR: govulncheck not installed."; \
		exit 1; \
	fi

# Generate code
.PHONY: generate
generate:
	@command -v protoc >/dev/null 2>&1 || { echo "ERROR: protoc not installed."; exit 1; }
	@command -v protoc-gen-go >/dev/null 2>&1 || { echo "ERROR: protoc-gen-go not installed."; exit 1; }
	protoc --proto_path=contracts/proto --go_out=. --go_opt=module=github.com/share-disk/share-disk contracts/proto/sharedisk/v1/transfer.proto contracts/proto/sharedisk/v1/local.proto

# Run production-source checks.
.PHONY: check
check: fmt-check lint contract architecture vuln build
	@echo "All checks passed!"

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
	@echo "  windows       Build the Windows desktop client"
	@echo "  fmt           Format code"
	@echo "  fmt-check     Check formatting"
	@echo "  vet           Run go vet"
	@echo "  staticcheck   Run staticcheck"
	@echo "  lint          Run all linters"
	@echo "  contract      Verify generated protocol assets"
	@echo "  architecture  Verify server/client dependency boundaries"
	@echo "  vuln          Run vulnerability check"
	@echo "  generate      Generate code"
	@echo "  check         Run production-source checks"
	@echo "  image         Build Docker images"
	@echo "  compose-config Validate Compose configuration"
	@echo "  image-scan    Scan Docker images for vulnerabilities"
	@echo "  deb           Build the Ubuntu .deb package"
	@echo "  apk           Build a signed Android APK"
	@echo "  help          Show this help"

# Docker image names
IMAGE_PREFIX?=share-disk
IMAGE_TAG?=$(VERSION)
CONTROL_SERVER_IMAGE=$(IMAGE_PREFIX)/control-server:$(IMAGE_TAG)
CONTROL_WORKER_IMAGE=$(IMAGE_PREFIX)/control-worker:$(IMAGE_TAG)
DB_MIGRATE_IMAGE=$(IMAGE_PREFIX)/db-migrate:$(IMAGE_TAG)

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
		-f server/deploy/docker/Dockerfile .
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--target control-worker \
		-t $(CONTROL_WORKER_IMAGE) \
		-f server/deploy/docker/Dockerfile .
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--target db-migrate \
		-t $(DB_MIGRATE_IMAGE) \
		-f server/deploy/docker/Dockerfile .
	@echo "Docker images built successfully"
	@echo "  $(CONTROL_SERVER_IMAGE)"
	@echo "  $(CONTROL_WORKER_IMAGE)"
	@echo "  $(DB_MIGRATE_IMAGE)"

# Validate Compose configuration
.PHONY: compose-config
compose-config:
	@echo "Validating Compose configuration..."
	./server/deploy/server.py config --quiet
	@echo "Compose configuration is valid"

# Enforce the production boundary: Compose must not run the Ubuntu Agent,
# declare the Agent volume, or publish the Agent's LAN port. Ubuntu packages
# install the Agent as a systemd service.
.PHONY: compose-boundary
compose-boundary:
	@echo "Checking production Compose boundary..."
	@services="$$(./server/deploy/server.py config --services)"; \
	if printf '%s\n' "$$services" | grep -qx 'agent'; then \
		echo "ERROR: default Compose must not contain an 'agent' service"; exit 1; \
	fi
	@volumes="$$(./server/deploy/server.py config --volumes)"; \
	if printf '%s\n' "$$volumes" | grep -qx 'agent_data'; then \
		echo "ERROR: default Compose must not declare an 'agent_data' volume"; exit 1; \
	fi
	@if grep -Eq '(^|[^0-9])9090:9090([^0-9]|$$)' server/deploy/compose.yaml; then \
		echo "ERROR: default Compose must not publish the Agent 9090 LAN port"; exit 1; \
	fi
	@echo "Compose boundary OK: backend-only default; Agent is a systemd service"

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
