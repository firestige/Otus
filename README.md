# capture-agent



高性能网络数据包捕获、解析和上报系统，专为边缘部署和 SIP 协议分析设计。

---

## 特性

- ✅ **高性能捕获**: 基于 AF_PACKET v3，单核 200K+ pps
- ✅ **协议解析**: 零正则 SIP 解析器，L2-L4 完整解码
- ✅ **IP 分片重组**: 生产级 IPv4 fragment reassembly
- ✅ **灵活上报**: Kafka Reporter + Console Reporter；Loki 作为日志输出
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
git clone https://github.com/firestige/capture-agent.git
cd capture-agent

# 构建静态二进制（当前架构）
make build-static

# 或构建所有架构（需要交叉编译工具）
make build-all
# 输出: dist/capture-agent-linux-amd64, dist/capture-agent-linux-arm64
```

#### 方式 B: Docker 构建

```bash
# 构建当前架构镜像
make docker-build

# 提取静态二进制
make docker-extract
# 输出: ./capture-agent-static
```

> **内网离线环境构建**，在执行 `make docker-build` 前需要完成以下配置，详见下方说明。
> - `configs/buildkitd.toml` — 注入内网 DNS
> - `configs/yum.repos.d/*.repo` — 替换 yum 仓库为 Nexus 代理
> - `configs/build.env` — 配置 Go 相关环境变量（GOPROXY、GONOSUMDB 等）
> - 项目根目录放置离线 Go 安装包

---

#### 内网构建：DNS 配置

内网构建时 yum/pip 等包管理器需要通过内网 DNS 解析私有镜像仓库或代理地址。Docker 20 的 buildx 不支持 `--dns` 参数，需通过 `configs/buildkitd.toml` 注入 DNS 配置。

**步骤一**：编辑 [`configs/buildkitd.toml`](configs/buildkitd.toml)，填入内网 DNS 服务器地址：

```toml
[dns]
  nameservers = ["10.0.0.1", "10.0.0.2"]
  # searchdomains = ["corp.example.com"]
```

**步骤二**：创建携带该配置的 buildx builder（每台构建机执行一次，重建后需重新执行）：

```bash
# 若已有旧 builder，先删除
make docker-rm-builder

# 创建新 builder（读取 configs/buildkitd.toml）
make docker-setup-builder
```

> DNS 配置只在 `make docker-setup-builder` 创建时读入。修改 `buildkitd.toml` 后需要先 `make docker-rm-builder` 再重新创建才能生效。

---

#### 内网构建：yum 仓库（Nexus 代理）

构建镜像基于 CentOS 7，`yum install` 需要能访问软件包仓库。内网环境需将默认的公网 CentOS 镜像替换为内网 Nexus 代理。

**步骤**：将 yum `.repo` 文件放置到 `configs/yum.repos.d/` 目录，Dockerfile 会自动将该目录下所有 `*.repo` 文件复制到构建容器的 `/etc/yum.repos.d/`，覆盖默认的公网仓库配置。

参考 [`configs/yum.repos.d/nexus.repo.example`](configs/yum.repos.d/nexus.repo.example) 填写实际 Nexus 地址后，将文件重命名为 `nexus.repo`（或任意 `.repo` 扩展名）：

```ini
[nexus-base]
name=Nexus CentOS Base
baseurl=http://<NEXUS_HOST>/repository/centos-proxy/$releasever/os/$basearch/
enabled=1
gpgcheck=0
```

---

#### 内网构建：Go 离线包

Docker 构建过程中需要安装 Go 工具链。内网环境无法访问 `go.dev`，需提前将安装包下载好并放置到**项目根目录**（与 `Dockerfile` 同级）。

从 [https://go.dev/dl/](https://go.dev/dl/) 下载对应平台的压缩包，命名格式为官网原始格式：

| 构建机架构 | 文件名示例 |
|---|---|
| x86\_64 (amd64) | `go1.23.6.linux-amd64.tar.gz` |
| aarch64 (arm64) | `go1.23.6.linux-arm64.tar.gz` |

Dockerfile 通过 `COPY go*.linux-*.tar.gz` 自动匹配，**文件名中的版本号无需与 Dockerfile 匹配**，升级 Go 版本只需替换文件即可。

---

#### 内网构建：Go 环境变量（Nexus Module Proxy）

所有构建期 Go 环境变量统一在 [`configs/build.env`](configs/build.env) 中配置，Makefile 会自动读取该文件并以 `--build-arg` 方式注入 Dockerfile，无需修改 Makefile 或在命令行传参。

编辑 `configs/build.env`，填入内网 Nexus 地址：

```ini
# Go Module Proxy：指向内网 Nexus Go 仓库
# 完全隔离环境去掉 ,direct
GOPROXY=http://<NEXUS_HOST>/repository/go-proxy,direct

# 禁用 sum.golang.org 校验（内网无法访问时必须设置）
# * 表示对所有模块跳过，也可指定路径前缀，如 corp.example.com/*
GONOSUMDB=*

# 模块模式（Go 1.16 起默认 on，保留以兼容旧工具）
GO111MODULE=on

# 构建容器内的 GOPATH
GOPATH=/go
```

> `GOROOT` 无需配置——Go 会从二进制所在路径自动推断。

配置完成后直接构建，无需额外参数：

```bash
make docker-build
```

---

### 2. 安装

```bash
# 安装二进制
sudo install -m 755 capture-agent /usr/local/bin/

# 创建目录
sudo mkdir -p /etc/capture-agent /var/lib/capture-agent /var/log/capture-agent

# 部署配置文件
sudo cp configs/config.yml /etc/capture-agent/

# 安装 systemd 服务
sudo cp configs/capture-agent.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable capture-agent
```

### 3. 配置

编辑 `/etc/capture-agent/config.yml`：

```yaml
capture-agent:
  control:
    socket: /var/run/capture-agent.sock

  metrics:
    enabled: true
    listen: ":9091"

  log:
    level: info
    format: json
    outputs:
      loki:
        enabled: false
        endpoint: "http://loki:3100/loki/api/v1/push"

  kafka:
    brokers:
      - "kafka:9092"

  command_channel:
    enabled: true
    type: kafka
    kafka:
      topic: capture-agent-commands
```

### 4. 启动

```bash
# 启动服务
sudo systemctl start capture-agent

# 查看状态
sudo systemctl status capture-agent

# 查看日志
sudo journalctl -u capture-agent -f
```

### 5. 创建抓包任务

```bash
# 准备任务配置文件（JSON 或 YAML）
cat > sip-capture.yaml <<EOF
id: sip-capture
interface: eth0
parsers:
  - sip
bpf_filter: "udp port 5060"
EOF

# 通过 UDS 创建任务
capture-agent task create -f sip-capture.yaml

# 查看任务列表
capture-agent task list

# 查看任务状态
capture-agent task status sip-capture

# 删除任务
capture-agent task delete sip-capture
```

---

## 部署场景

### 裸金属/物理服务器

参见[部署文档](doc/DEPLOYMENT.md#裸金属物理服务器部署)

### Kubernetes

```bash
# 创建 DaemonSet（每个节点一个实例）
kubectl apply -f docs/kubernetes/daemonset.yaml

# 查看运行状态
kubectl get pods -n monitoring -l app=capture-agent
```

详见[K8s 部署指南](doc/DEPLOYMENT.md#kubernetes-部署)

### 虚拟机 (VMware/KVM/ECS)

与裸金属部署相同，注意网卡混杂模式配置。

---

## 架构

```
┌─────────────────────────────────────────────────────┐
│                   capture-agent Daemon                       │
├─────────────────────────────────────────────────────┤
│  Task Manager                                       │
│  ├── Task 1 (SIP Capture)                          │
│  │   ├── Capturer (AF_PACKET)                      │
│  │   ├── Decoder (L2-L4 + IP Reassembly)           │
│  │   ├── Pipeline                                   │
│  │   │   ├── Parser (SIP)                          │
│  │   │   ├── Processor (Filter/Label)              │
│  │   │   └── Reporter (Kafka / Console)            │
│  │   └── Metrics Collector                         │
│  └── Task 2 (...)                                   │
├─────────────────────────────────────────────────────┤
│  Command Handler                                    │
│  ├── UDS Server (/var/run/capture-agent.sock)               │
│  └── Kafka Consumer (capture-agent-commands)                │
├─────────────────────────────────────────────────────┤
│  Metrics Server (:9091/metrics)                     │
└─────────────────────────────────────────────────────┘
```

详见[架构文档](doc/architecture.md)

---

## 目录结构

```
capture-agent/
├── cmd/                      # CLI 命令实现
│   ├── root.go              # root command + 全局 flags
│   ├── daemon.go            # daemon 命令
│   ├── task.go              # task 子命令（create/delete/list/status）
│   ├── stop.go              # stop 命令
│   ├── reload.go            # reload 命令
│   ├── status.go            # daemon status 命令
│   ├── stats.go             # daemon stats 命令
│   └── validate.go          # validate 命令
├── configs/                  # 配置文件
│   ├── config.yml           # 默认配置
│   └── capture-agent.service         # systemd unit file
├── internal/                 # 内部实现
│   ├── core/                # 核心解码器
│   │   ├── decoder/         # L2-L4 解码 + IP 重组
│   │   ├── packet.go        # RawPacket / DecodedPacket
│   │   ├── types.go         # EthernetHeader / IPHeader / TransportHeader
│   │   ├── labels.go        # Labels 类型
│   │   └── errors.go        # sentinel errors
│   ├── daemon/              # Daemon 进程管理
│   ├── pipeline/            # Pipeline 引擎
│   ├── task/                # Task 管理器
│   ├── command/             # 命令处理器（UDS + Kafka）
│   ├── metrics/             # Prometheus 指标
│   ├── log/                 # 日志子系统（含 Loki 输出）
│   └── config/              # 配置加载
├── pkg/                      # 公开接口（插件 API）
│   ├── plugin/              # 插件基础接口
│   └── models/              # 数据模型
├── plugins/                  # 插件实现
│   ├── capture/afpacket/    # AF_PACKET v3 捕获器
│   ├── parser/sip/          # SIP 解析器
│   ├── processor/filter/    # 过滤 / 标注 Processor
│   └── reporter/            # 上报插件
│       ├── kafka/           # Kafka Producer
│       └── console/         # 控制台调试输出
├── scripts/                  # 构建脚本
│   └── build.sh             # 交叉编译脚本
├── doc/                      # 文档
│   ├── architecture.md      # 架构设计
│   ├── config-design.md     # 配置设计
│   ├── decisions.md         # ADR 决策记录
│   ├── implementation-plan.md # 实施计划
│   └── DEPLOYMENT.md        # 部署指南
├── voip-simulator/           # VoIP 模拟测试环境
│   ├── docker-compose.yml   # 编排所有服务
│   ├── uas/                 # SIPp UAS（被叫方）
│   ├── uac/                 # SIPp UAC（主叫方）
│   ├── otus/                # capture-agent sidecar 容器
│   └── console/             # Web 控制台（任务下发 + 实时查看）
├── Dockerfile               # 静态构建镜像
├── Makefile                 # 构建任务
├── go.mod                   # Go 模块定义
└── main.go                  # 程序入口
```

---

## VoIP Simulator（端到端验证）

`voip-simulator/` 目录提供了一套基于 **SIPp** 的 VoIP 通话模拟环境，用于端到端验证 capture-agent 的抓包、解析和上报能力。

### 架构概览

```
┌──────────────┐  SIP/RTP   ┌──────────────┐
│  UAC (SIPp)  │ ────────── │  UAS (SIPp)  │
│ 172.20.0.20  │            │ 172.20.0.10  │
└──────┬───────┘            └──────┬───────┘
       │ network_mode:container          │ network_mode:container
┌──────┴───────┐            ┌──────┴───────┐
│  capture-agent (UAC)  │            │  capture-agent (UAS)  │
│  sidecar     │            │  sidecar     │
└──────┬───────┘            └──────┬───────┘
       │                           │
       └─────────┐   ┌─────────────┘
                 ▼   ▼
          ┌──────────────┐
          │    Kafka      │
          │ 172.20.0.30   │
          └──────┬───────┘
                 │
       ┌─────────┴──────────┐
       ▼                    ▼
┌──────────────┐   ┌───────────────┐
│ Web Console  │   │ Redpanda      │
│ :8080        │   │ Console :8081 │
└──────────────┘   └───────────────┘
```

### 组件说明

| 服务 | 说明 |
|------|------|
| **UAS** | SIPp 被叫方，监听 UDP 5060，接听呼叫并回送 RTP |
| **UAC** | SIPp 主叫方，按配置速率发起呼叫（默认 1 cps） |
| **capture-agent sidecar** | 以 `network_mode: container` 方式挂载到 UAC/UAS，共享网络命名空间进行抓包 |
| **Kafka** | KRaft 单节点，接收 capture-agent 上报的 SIP/RTP 解析结果 |
| **Redpanda Console** | Kafka Web UI，查看 topic 数据（http://localhost:8081） |
| **Web Console** | 任务下发与实时数据包查看界面（http://localhost:8080） |

### 快速启动

```bash
cd voip-simulator

# 启动所有服务
docker compose up -d --build

# 查看服务状态
docker compose ps

# 查看 capture-agent 抓包日志
docker logs -f capture-agent-uas

# 打开 Web Console 查看实时数据
# http://localhost:8080

# 打开 Redpanda Console 查看 Kafka 消息
# http://localhost:8081

# 停止所有服务
docker compose down -v
```

### 验证流程

1. 启动环境后，UAC 自动向 UAS 发起 SIP 呼叫
2. capture-agent sidecar 实时捕获 SIP 信令和 RTP 媒体流
3. 解析结果通过 Kafka 上报
4. 在 Web Console 或 Redpanda Console 中查看抓包数据，验证解析正确性

---

## 开发

### 环境准备

```bash
# 安装依赖（AF_PACKET 抓包需要 libpcap）
sudo apt-get install -y libpcap-dev  # Debian/Ubuntu

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
capture_agent_capture_packets_total{task="sip-capture", interface="eth0"}
capture_agent_capture_drops_total{task="sip-capture", stage="kernel"}

# Pipeline metrics
capture_agent_pipeline_packets_total{task="sip-capture", pipeline="1", stage="parsed"}
capture_agent_pipeline_latency_seconds{task="sip-capture", stage="decode"}

# Task status
capture_agent_task_status{task="sip-capture", status="running"}

# Reassembly
capture_agent_reassembly_active_fragments
```

---

## 常见问题

### 1. 权限不足

```bash
# 错误: socket: operation not permitted
# 解决:
sudo systemctl restart capture-agent
# 或添加 capabilities
sudo setcap cap_net_raw,cap_net_admin+eip /usr/local/bin/capture-agent
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
sudo journalctl -u capture-agent -e | grep -i kafka
```

更多问题参见[故障排查](doc/DEPLOYMENT.md#故障排查)

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

- **Issues**: [GitHub Issues](https://github.com/firestige/capture-agent/issues)
- **Docs**: [doc/](doc/)
- **Architecture**: [doc/architecture.md](doc/architecture.md)

---

**版本**: v0.1.0-dev  
**更新时间**: 2026-02-22
