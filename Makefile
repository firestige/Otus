.PHONY: all build build-static build-all proto clean install uninstall test run docker-build docker-extract

# Variables
BINARY_NAME=otus
INSTALL_PATH=/usr/local/bin
SYSTEMD_PATH=/etc/systemd/system
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S_UTC')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown')
LDFLAGS=-w -s -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.GitCommit=$(GIT_COMMIT)'

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

# Docker build (multi-architecture)
docker-build:
	@echo "Building Docker image with static binary..."
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t otus:$(VERSION) \
		-t otus:latest \
		--load \
		.

# Extract static binary from Docker image
docker-extract:
	@echo "Extracting static binary from Docker image..."
	@docker create --name otus-extract otus:latest
	@docker cp otus-extract:/otus ./otus-static
	@docker rm otus-extract
	@echo "Binary extracted to ./otus-static"
	@file ./otus-static
	@ldd ./otus-static 2>&1 || true

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
	sudo cp configs/otus.service ${SYSTEMD_PATH}/
	sudo systemctl daemon-reload
	sudo systemctl enable otus
	@echo "Run 'sudo systemctl start otus' to start the service"

# 卸载
uninstall:
	@echo "Uninstalling ${BINARY_NAME}..."
	sudo systemctl stop otus 2>/dev/null || true
	sudo systemctl disable otus 2>/dev/null || true
	sudo rm -f ${SYSTEMD_PATH}/otus.service
	sudo rm -f ${INSTALL_PATH}/${BINARY_NAME}
	sudo systemctl daemon-reload

# Clean build artifacts
clean:
	@echo "Cleaning up..."
	rm -f ${BINARY_NAME}
	rm -f otus-static
	rm -rf dist/
	rm -f /tmp/otus.sock
	rm -f /tmp/otus.pid
	rm -f /tmp/otus.log

# 测试
test:
	go test -v ./...

# 本地运行（调试）
run: build
	./${BINARY_NAME}

# 查看日志
logs:
	tail -f /tmp/otus.log

# 开发模式（前台运行）
dev: build
	./${BINARY_NAME} start --foreground