# Otus

**O**ptimized **T**raffic **U**nveiling **S**uite

高性能网络数据包捕获、解析和上报系统，专为边缘部署和 SIP 协议分析设计。

---

## 特性

- ✅ **高性能捕获**: 基于 AF_PACKET v3，单核 200K+ pps
- ✅ **协议解析**: 零正则 SIP 解析器，L2-L4 完整解码
- ✅ **IP 分片重组**: 生产级 IPv4 fragment reassembly
- ✅ **灵活上报**: Kafka + Loki 双通道日志
- ✅ **动态管理**: 支持 UDS/Kafka 远程命令
- ✅ **可观测性**: Prometheus 指标 + 结构化日志
- ✅ **跨平台**: 静态链接二进制，支持 x86_64 + ARM64
- ✅ **多场景部署**: 裸金属/VM/K8s/ECS 通用

---

## 快速开始

### 1. 构建

#### 方式 A: 本地构建（推荐）

```bash
# 前置依赖
sudo apt-get install -y libpcap-dev  # Debian/Ubuntu
# sudo dnf install libpcap-devel      # Fedora/RHEL

# 克隆仓库
git clone https://github.com/firestige/otus.git
cd otus

# 构建静态二进制（当前架构）
make build-static

# 或构建所有架构（需要交叉编译工具）
make build-all
# 输出: dist/otus-linux-amd64, dist/otus-linux-arm64
```

#### 方式 B: Docker 构建

```bash
# 多架构构建
make docker-build

# 提取静态二进制
make docker-extract
# 输出: ./otus-static
```

### 2. 安装

```bash
# 安装二进制
sudo install -m 755 otus /usr/local/bin/

# 创建目录
sudo mkdir -p /etc/otus /var/lib/otus /var/log/otus

# 部署配置文件
sudo cp configs/config.yml /etc/otus/

# 安装 systemd 服务
sudo cp configs/otus.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable otus
```

### 3. 配置

编辑 `/etc/otus/config.yml`：

```yaml
global:
  log_level: info

daemon:
  unix_socket: /tmp/otus.sock
  metrics:
    enabled: true
    listen: :9091

log:
  appenders:
    - type: kafka
      brokers: ["kafka:9092"]
      topic: otus-logs

command:
  channel: kafka
  brokers: ["kafka:9092"]
  consumer_topic: otus-commands
```

### 4. 启动

```bash
# 启动服务
sudo systemctl start otus

# 查看状态
sudo systemctl status otus

# 查看日志
sudo journalctl -u otus -f
```

### 5. 创建抓包任务

```bash
# 通过 UDS 创建任务
otus task create --name sip-capture \
  --interface eth0 \
  --protocol sip \
  --bpf-filter "udp port 5060"

# 查看任务
otus task list

# 停止任务
otus task stop sip-capture
```

---

## 部署场景

### 裸金属/物理服务器

参见[部署文档](docs/DEPLOYMENT.md#裸金属物理服务器部署)

### Kubernetes

```bash
# 创建 DaemonSet（每个节点一个实例）
kubectl apply -f docs/kubernetes/daemonset.yaml

# 查看运行状态
kubectl get pods -n monitoring -l app=otus
```

详见[K8s 部署指南](docs/DEPLOYMENT.md#kubernetes-部署)

### 虚拟机 (VMware/KVM/ECS)

与裸金属部署相同，注意网卡混杂模式配置。

---

## 架构

```
┌─────────────────────────────────────────────────────┐
│                   Otus Daemon                       │
├─────────────────────────────────────────────────────┤
│  Task Manager                                       │
│  ├── Task 1 (SIP Capture)                          │
│  │   ├── Capturer (AF_PACKET)                      │
│  │   ├── Decoder (L2-L4 + IP Reassembly)           │
│  │   ├── Pipeline                                   │
│  │   │   ├── Parser (SIP)                          │
│  │   │   ├── Processor (Business Logic)            │
│  │   │   └── Reporter (Kafka)                      │
│  │   └── Metrics Collector                         │
│  └── Task 2 (...)                                   │
├─────────────────────────────────────────────────────┤
│  Command Handler                                    │
│  ├── UDS Server (/tmp/otus.sock)                   │
│  └── Kafka Consumer (otus-commands)                │
├─────────────────────────────────────────────────────┤
│  Metrics Server (:9091/metrics)                     │
└─────────────────────────────────────────────────────┘
```

详见[架构文档](doc/architecture.md)

---

## 目录结构

```
otus/
├── api/v1/                   # gRPC/Protobuf 定义
├── cmd/                      # CLI 命令实现
│   ├── daemon.go            # daemon 命令
│   ├── start.go             # start 命令（已废弃，用 daemon）
│   ├── stop.go              # stop 命令
│   └── execute.go           # task 子命令
├── configs/                  # 配置文件
│   ├── config.yml           # 默认配置
│   └── otus.service         # systemd unit file
├── internal/                 # 内部实现
│   ├── core/                # 核心解码器
│   │   ├── decoder/         # L2-L4 解码 + IP 重组
│   │   └── types.go         # 数据结构
│   ├── daemon/              # Daemon 进程管理
│   ├── pipeline/            # Pipeline 引擎
│   ├── task/                # Task 管理器
│   ├── command/             # 命令处理器
│   ├── metrics/             # Prometheus 指标
│   ├── log/                 # 日志子系统
│   └── config/              # 配置加载
├── pkg/                      # 公开接口（插件 API）
│   ├── plugin/              # 插件基础接口
│   ├── processor/           # Processor 接口
│   └── models/              # 数据模型
├── plugins/                  # 插件实现
│   ├── capture/afpacket/    # AF_PACKET v3 捕获器
│   ├── parser/sip/          # SIP 解析器
│   ├── processor/api/       # Processor 基础接口
│   ├── reporter/            # 上报插件
│   │   ├── consolelog/      # 控制台输出
│   │   └── skywalkingtracing/ # SkyWalking 上报
│   ├── handler/skywalking/  # SkyWalking 协议处理
│   └── fallbacker/none/     # 降级处理器
├── scripts/                  # 构建脚本
│   └── build.sh             # 交叉编译脚本
├── docs/                     # 文档
│   └── DEPLOYMENT.md        # 部署指南
├── Dockerfile               # 静态构建镜像
├── Makefile                 # 构建任务
├── go.mod                   # Go 模块定义
└── main.go                  # 程序入口
```

---

## 开发

### 环境准备

```bash
# 安装依赖
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 生成 protobuf 代码
make proto

# 运行测试
make test

# 本地运行（开发模式）
go run main.go daemon
```

### 代码风格

- 遵循 Go 官方代码规范
- 使用 `golangci-lint` 进行静态检查
- 注释使用中文（内部团队约定）

---

## 性能指标

| 场景 | 吞吐量 | CPU | 内存 |
|------|--------|-----|------|
| SIP 完整解析 | 200K+ pps | 1 核 | 512 MB |
| 仅 L2-L4 解码 | 1M+ pps | 1 核 | 256 MB |
| IP 分片重组 | 100K frags/s | 0.5 核 | 128 MB |

**测试环境**: Intel Xeon E5-2680 v4, 10G NIC, Linux 5.15

---

## Prometheus 指标

```
# Capture metrics
otus_capture_packets_total{task="sip-capture", interface="eth0"}
otus_capture_drops_total{task="sip-capture", stage="kernel"}

# Pipeline metrics
otus_pipeline_packets_total{task="sip-capture", pipeline="1", stage="parsed"}
otus_pipeline_latency_seconds{task="sip-capture", stage="decode"}

# Task status
otus_task_status{task="sip-capture", status="running"}

# Reassembly
otus_reassembly_active_fragments
```

---

## 常见问题

### 1. 权限不足

```bash
# 错误: socket: operation not permitted
# 解决:
sudo systemctl restart otus
# 或添加 capabilities
sudo setcap cap_net_raw,cap_net_admin+eip /usr/local/bin/otus
```

### 2. 无法抓到流量

```bash
# 启用混杂模式
sudo ip link set eth0 promisc on

# 验证抓包
sudo tcpdump -i eth0 -n -c 10
```

### 3. Kafka 连接失败

```bash
# 检查网络
telnet kafka-broker 9092

# 查看日志
sudo journalctl -u otus -e | grep -i kafka
```

更多问题参见[故障排查](docs/DEPLOYMENT.md#故障排查)

---

## 贡献

欢迎提交 Issue 和 Pull Request！

### 开发计划

- [x] Step 1-16: 核心功能实现
- [x] Step 17: 部署配置和文档
- [ ] Step 18: 性能优化和压测
- [ ] Step 19: 安全加固
- [ ] Step 20: 监控和告警集成

详见[实施计划](doc/implementation-plan.md)

---

## 许可证

[MIT License](LICENSE)

---

## 联系方式

- **Issues**: [GitHub Issues](https://github.com/firestige/otus/issues)
- **Docs**: [docs/](docs/)
- **Architecture**: [doc/architecture.md](doc/architecture.md)

---

**版本**: v0.1.0-dev  
**更新时间**: 2026-02-17
