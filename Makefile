.PHONY: all build build-static build-all proto clean install install-systemd uninstall test run docker-build dist simulator-build

# Variables
BINARY_NAME=capture-agent
INSTALL_PATH=/usr/local/bin
SYSTEMD_PATH=/etc/systemd/system
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')
ARCH:=$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
DIST_NAME=capture-agent-$(VERSION)-linux-$(ARCH)
DIST_DIR=dist/$(DIST_NAME)
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S_UTC')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown')
LDFLAGS=-w -s -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.GitCommit=$(GIT_COMMIT)'

# Docker build-arg values are read automatically from configs/build.env.
# Each non-comment, non-empty line becomes a --build-arg flag.
# Configure DNS1, DNS2, GOPROXY, GONOSUMDB, etc. in that file.
BUILD_ENV_FILE ?= configs/build.env
BUILD_ARGS := $(if $(wildcard $(BUILD_ENV_FILE)),\
	$(shell grep -v '^\s*\#' $(BUILD_ENV_FILE) | grep -v '^\s*$$' | sed 's/^/--build-arg /'),)

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
# For true static binary, use Docker build: make docker-build && make dist
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

# Docker build — uses plain 'docker build' (default docker driver) so the
# build container inherits the host network stack. DNS and yum/Go proxy
# settings are injected via --build-arg from configs/build.env.
docker-build:
	@echo "Building Docker image..."
	docker build \
		$(BUILD_ARGS) \
		-t capture-agent:$(VERSION) \
		-t capture-agent:latest \
		.

# Build a self-contained distribution package.
# Produces: dist/capture-agent-{version}-linux-{arch}.tar.gz
# The tarball contains the binary, default config, service file, tmpfiles.d
# config, and a setup.sh installer script.
dist: docker-build
	@echo "Creating distribution package: $(DIST_NAME).tar.gz"
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)/bin $(DIST_DIR)/configs/tmpfiles.d
	@docker create --name _ca_dist capture-agent:latest
	@docker cp _ca_dist:/capture-agent $(DIST_DIR)/bin/capture-agent
	@docker rm _ca_dist
	@cp configs/config.yml        $(DIST_DIR)/configs/
	@cp configs/capture-agent.service $(DIST_DIR)/configs/
	@cp configs/tmpfiles.d/capture-agent.conf $(DIST_DIR)/configs/tmpfiles.d/
	@cp scripts/setup.sh          $(DIST_DIR)/setup.sh
	@chmod +x $(DIST_DIR)/bin/capture-agent $(DIST_DIR)/setup.sh
	@tar -czf dist/$(DIST_NAME).tar.gz -C dist $(DIST_NAME)
	@rm -rf $(DIST_DIR)
	@echo "Package ready: dist/$(DIST_NAME).tar.gz"

# Build all voip-simulator Docker images.
simulator-build:
	@echo "Building voip-simulator images..."
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