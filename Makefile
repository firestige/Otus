.PHONY: all build build-static build-all proto clean install uninstall test run docker-build docker-setup-builder docker-rm-builder docker-extract simulator-build

# Variables
BINARY_NAME=capture-agent
INSTALL_PATH=/usr/local/bin
SYSTEMD_PATH=/etc/systemd/system
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S_UTC')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown')
LDFLAGS=-w -s -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.GitCommit=$(GIT_COMMIT)'

# Target platform for docker-build.
# Defaults to the host's native architecture — build amd64 on amd64, arm64 on arm64.
# No cross-compilation: the builder uses CGO (libpcap) which requires a native
# C toolchain.
# Override only when targeting the same architecture:
#   make docker-build PLATFORM=linux/amd64
#   make docker-build PLATFORM=linux/arm64
PLATFORM ?= linux/$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

# Dedicated buildx builder.
BUILDER ?= capture-agent-builder

# buildkitd configuration file.
# Contains the [dns] section with internal nameservers so build containers
# resolve names correctly without modifying the host's /etc/resolv.conf.
# Edit configs/buildkitd.toml to set your DNS IPs before running docker-setup-builder.
BUILDKIT_CONFIG ?= configs/buildkitd.toml

# Go module proxy for Docker builds.
# Set to your internal Nexus Go proxy URL for offline/internal builds.
# Example: make docker-build GOPROXY=http://nexus.corp/repository/go-proxy,direct
# Leave empty to use the default inside the Dockerfile (GOPROXY=direct).
GOPROXY ?=

all: proto build

# 生成 protobuf 代码
proto:
	@echo "Generating protobuf code..."
	protoc --go_out=. --go-grpc_out=. api/v1/daemon.proto

# Build (dynamic linking - development only)
build:
	@echo "Building ${BINARY_NAME} (dynamic)..."
	go build -ldflags "$(LDFLAGS)" -o ${BINARY_NAME} main.go

# Build static binary (for production deployment)
# Note: Complete static linking requires many dependencies (libsystemd, libgcrypt, etc.)
# For true static binary, use Docker build: make docker-build && make docker-extract
build-static:
	@echo "Building ${BINARY_NAME} with minimal dependencies..."
	@echo "Note: This binary has glibc dependency. For fully static binary, use Docker build."
	CGO_ENABLED=1 go build \
		-tags netgo,osusergo \
		-ldflags "$(LDFLAGS)" \
		-o ${BINARY_NAME} \
		main.go
	@echo ""
	@echo "✓ Binary built successfully: ${BINARY_NAME}"
	@if command -v ldd >/dev/null 2>&1; then \
		echo "Dependencies:"; \
		ldd ${BINARY_NAME} 2>&1 | head -10; \
	fi

# Build all architectures (requires cross-compilation tools)
build-all:
	@echo "Building for all architectures..."
	@chmod +x scripts/build.sh
	@./scripts/build.sh

# Build single architecture
build-amd64:
	@chmod +x scripts/build.sh
	@./scripts/build.sh --arch=amd64

build-arm64:
	@chmod +x scripts/build.sh
	@./scripts/build.sh --arch=arm64

# Docker build — native architecture only.
docker-build: docker-setup-builder
	@echo "Building Docker image for platform: $(PLATFORM)..."
	docker buildx build \
		--builder $(BUILDER) \
		--platform $(PLATFORM) \
		$(if $(GOPROXY),--build-arg GOPROXY=$(GOPROXY)) \
		-t capture-agent:$(VERSION) \
		-t capture-agent:latest \
		--load \
		.

# Create (once) the dedicated buildx builder.
# --config $(BUILDKIT_CONFIG)  → injects [dns] nameservers into build containers
#                                so internal packages resolve without modifying
#                                the host /etc/resolv.conf.
# Edit configs/buildkitd.toml with your DNS IPs before running this target.
# Idempotent: skips creation if the builder already exists.
docker-setup-builder:
	@docker buildx inspect $(BUILDER) >/dev/null 2>&1 || \
		docker buildx create \
			--name $(BUILDER) \
			--driver docker-container \
			--config $(BUILDKIT_CONFIG) \
			--use && \
			docker buildx inspect --bootstrap $(BUILDER)

# Remove the dedicated builder (e.g. to reset DNS config)
docker-rm-builder:
	docker buildx rm $(BUILDER) 2>/dev/null || true

# Extract static binary from Docker image
docker-extract:
	@echo "Extracting static binary from Docker image..."
	@docker create --name capture-agent-extract capture-agent:latest
	@docker cp capture-agent-extract:/capture-agent ./capture-agent-static
	@docker rm capture-agent-extract
	@echo "Binary extracted to ./capture-agent-static"
	@file ./capture-agent-static
	@ldd ./capture-agent-static 2>&1 || true

# Build all voip-simulator Docker images.
simulator-build:
	@echo "Building voip-simulator images for platform: $(PLATFORM)..."
	DOCKER_BUILDKIT=1 docker compose -f voip-simulator/docker-compose.yml build

# 构建插件
build-plugins:
	@echo "Building plugins..."
	@./scripts/build_plugins.sh gatherers pcap
	@./scripts/build_plugins.sh processors dns_processor
	@./scripts/build_plugins.sh outputs file_output

# 安装
install: build
	@echo "Installing ${BINARY_NAME} to ${INSTALL_PATH}..."
	sudo cp ${BINARY_NAME} ${INSTALL_PATH}/
	sudo chmod +x ${INSTALL_PATH}/${BINARY_NAME}

# 安装 systemd 服务
install-systemd: install
	@echo "Installing systemd service..."
	sudo cp configs/capture-agent.service ${SYSTEMD_PATH}/
	sudo systemctl daemon-reload
	sudo systemctl enable capture-agent
	@echo "Run 'sudo systemctl start capture-agent' to start the service"

# 卸载
uninstall:
	@echo "Uninstalling ${BINARY_NAME}..."
	sudo systemctl stop capture-agent 2>/dev/null || true
	sudo systemctl disable capture-agent 2>/dev/null || true
	sudo rm -f ${SYSTEMD_PATH}/capture-agent.service
	sudo rm -f ${INSTALL_PATH}/${BINARY_NAME}
	sudo systemctl daemon-reload

# Clean build artifacts
clean:
	@echo "Cleaning up..."
	rm -f ${BINARY_NAME}
	rm -f capture-agent-static
	rm -rf dist/
	rm -f /tmp/capture-agent.sock
	rm -f /tmp/capture-agent.pid
	rm -f /tmp/capture-agent.log

# 测试
test:
	go test -v ./...

# 本地运行（调试）
run: build
	./${BINARY_NAME}

# 查看日志
logs:
	tail -f /tmp/capture-agent.log

# 开发模式（前台运行）
dev: build
	./${BINARY_NAME} start --foreground