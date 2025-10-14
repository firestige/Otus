.PHONY: all build proto clean install uninstall test run

# 变量
BINARY_NAME=otus
INSTALL_PATH=/usr/local/bin
SYSTEMD_PATH=/etc/systemd/system

all: proto build

# 生成 protobuf 代码
proto:
	@echo "Generating protobuf code..."
	protoc --go_out=. --go-grpc_out=. api/v1/daemon.proto

# 构建
build:
	@echo "Building ${BINARY_NAME}..."
	go build -o ${BINARY_NAME} main.go

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

# 清理
clean:
	@echo "Cleaning up..."
	rm -f ${BINARY_NAME}
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