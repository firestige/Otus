# Otus 实施计划

> 本文档是从零重建 Otus 项目的"交接文档"。每个后续工作会话应基于本文档定位自己的任务块，无需重新阅读旧代码。

## 1. 项目元信息

| 项 | 值 |
|---|---|
| Go 版本 | ≥ 1.24 |
| Module path | `firestige.xyz/otus` |
| 仓库路径 | `/workspaces/Otus` |
| 二进制名称 | `otus`（单二进制，daemon / CLI 共用） |
| 许可证 | MIT |

---

## 2. 确认的依赖清单

### 2.1 Go 标准库（零外部依赖部分）

| 用途 | 标准库包 |
|------|----------|
| 结构化日志 | `log/slog` |
| IP 地址值类型 | `net/netip` |
| JSON-RPC 序列化 | `encoding/json` |
| 上下文传递 | `context` |
| 时间处理 | `time` |
| 原子操作 | `sync/atomic`, `sync` |
| 指标暴露 HTTP | `net/http` |

### 2.2 外部依赖

| 用途 | 包 | 备注 |
|------|---|------|
| CLI 框架 | `github.com/spf13/cobra` | 保留不变 |
| 配置管理 | `github.com/spf13/viper` | 保留不变 |
| 日志文件滚动 | `gopkg.in/natefinch/lumberjack.v2` | 实现 `io.Writer` |
| Kafka 客户端 | `github.com/segmentio/kafka-go` | 纯 Go，无 CGO |
| AF_PACKET 捕获 | `github.com/google/gopacket` | 仅用于 `internal/core/decoder` 内部和 AF_PACKET 抓包 |
| Prometheus 指标 | `github.com/prometheus/client_golang` | `/metrics` HTTP 端点 |
| BPF 编译 | `github.com/google/gopacket/pcap` 或 `golang.org/x/net/bpf` | 编译 BPF 过滤器 |

### 2.3 不引入的依赖

| 库 | 原因 |
|---|------|
| `google.golang.org/grpc` + protoc | 本地控制改用 JSON-RPC over UDS |
| `github.com/sirupsen/logrus` | 替换为 stdlib slog |
| `grafana/loki-client-go` | 已不活跃，自实现 HTTP Push ~150 行 |
| `confluent-kafka-go` | 需 CGO + librdkafka，交叉编译困难 |
| `IBM/sarama` | API 复杂度过高，场景不需要底层控制 |

---

## 3. 目标包结构

```
firestige.xyz/otus
├── main.go                          // 入口，仅调用 cmd.Execute()
├── go.mod
├── go.sum
├── Makefile
├── configs/
│   ├── config.yml                   // 全局静态配置模板
│   └── otus.service                 // systemd unit file
├── cmd/                             // CLI 命令定义（cobra）
│   ├── root.go                      // root command + 全局 flags
│   ├── daemon.go                    // `otus daemon` — 启动守护进程
│   ├── task.go                      // `otus task {create|delete|list|status}`
│   ├── stop.go                      // `otus stop` — 停止守护进程
│   └── reload.go                    // `otus reload` — 重载配置
├── internal/
│   ├── config/                      // 全局配置加载（viper）
│   │   ├── config.go                // GlobalConfig 结构体 + Load()
│   │   └── task.go                  // TaskConfig 结构体
│   ├── core/                        // 核心数据结构（零外部依赖）
│   │   ├── packet.go                // RawPacket, DecodedPacket, OutputPacket
│   │   ├── labels.go                // Labels 类型 + 命名常量
│   │   ├── errors.go                // sentinel errors
│   │   └── types.go                 // EthernetHeader, IPHeader, TransportHeader
│   ├── core/decoder/                // L2-L4 协议栈解码器
│   │   ├── decoder.go               // Decoder 接口 + 主解码流程
│   │   ├── ethernet.go              // L2 解码（含 VLAN/QinQ）
│   │   ├── tunnel.go                // 隧道解封装（VXLAN/GRE/Geneve/IPIP）
│   │   ├── ip.go                    // L3 IPv4/IPv6 解码
│   │   ├── transport.go             // L4 UDP/TCP 头部解码
│   │   └── reassembly.go            // IPv4 分片重组
│   ├── pipeline/                    // Pipeline 引擎
│   │   ├── pipeline.go              // Pipeline 主循环（单 goroutine 处理链）
│   │   ├── builder.go               // 从 TaskConfig 构建 Pipeline 实例
│   │   └── metrics.go               // Pipeline 级别计数器
│   ├── task/                        // Task 生命周期管理
│   │   ├── manager.go               // TaskManager — Task CRUD + 状态机
│   │   ├── task.go                  // Task 实体（TaskConfig + 运行时状态）
│   │   └── flow_registry.go         // FlowRegistry — per-Task sync.Map
│   ├── command/                     // 控制面
│   │   ├── handler.go               // 命令处理器（task.create / task.delete 等）
│   │   ├── kafka.go                 // Kafka 命令 topic 订阅
│   │   ├── uds_server.go            // JSON-RPC over UDS 服务端
│   │   └── uds_client.go            // JSON-RPC over UDS 客户端（CLI 使用）
│   ├── daemon/                      // 守护进程生命周期
│   │   └── daemon.go                // Daemon — 初始化 + graceful shutdown
│   ├── log/                         // 日志子系统
│   │   ├── logger.go                // slog 初始化 + 多输出配置
│   │   └── loki.go                  // Loki HTTP Push Writer（~150 行）
│   └── metrics/                     // Prometheus 指标
│       ├── metrics.go               // 全局指标注册
│       └── server.go                // /metrics HTTP 端点
├── pkg/                             // 插件接口定义（插件可见）
│   ├── plugin/
│   │   ├── capturer.go              // Capturer 接口
│   │   ├── parser.go                // Parser 接口（CanHandle + Handle）
│   │   ├── processor.go             // Processor 接口
│   │   ├── reporter.go              // Reporter 接口
│   │   └── lifecycle.go             // 通用生命周期接口（Init/Start/Stop/Health）
│   └── models/
│       └── packet.go                // RawPacket, DecodedPacket, OutputPacket 的 re-export
├── plugins/                         // 内置插件实现
│   ├── capture/
│   │   └── afpacket/
│   │       └── afpacket.go          // AF_PACKET_V3 Capturer
│   ├── parser/
│   │   └── sip/
│   │       ├── sip.go               // SIP Parser (CanHandle + Handle)
│   │       └── sip_test.go
│   ├── processor/
│   │   └── filter/
│   │       └── filter.go            // BPF 过滤 + Labels 标注 Processor
│   └── reporter/
│       ├── kafka/
│       │   └── kafka.go             // Kafka Producer Reporter
│       └── console/
│           └── console.go           // 控制台调试 Reporter
├── doc/
│   ├── architecture.md
│   ├── decisions.md
│   └── implementation-plan.md       // 本文档
└── scripts/
    └── build.sh
```

---

## 4. 核心数据结构定义

### 4.1 RawPacket

```go
// internal/core/packet.go

// RawPacket 是捕获层输出的原始数据，零拷贝引用 ring buffer。
type RawPacket struct {
    Data       []byte    // 原始帧数据，零拷贝切片
    Timestamp  time.Time // 捕获时间戳（内核时间戳优先）
    CaptureLen uint32    // 实际捕获长度
    OrigLen    uint32    // 原始帧长度
    InterfaceIndex int   // 网卡索引
}
```

### 4.2 DecodedPacket

```go
// internal/core/types.go

type EthernetHeader struct {
    SrcMAC    [6]byte
    DstMAC    [6]byte
    EtherType uint16   // 0x0800=IPv4, 0x86DD=IPv6, 0x8100=VLAN
    VLANs     []uint16 // 0~2 个 VLAN ID（QinQ 场景下有 2 个）
}

type IPHeader struct {
    Version  uint8
    SrcIP    netip.Addr  // Go 标准库值类型，零分配
    DstIP    netip.Addr
    Protocol uint8       // TCP=6, UDP=17, SCTP=132
    TTL      uint8
    TotalLen uint16
    // 隧道解封装后的内层 IP（非隧道场景为零值）
    InnerSrcIP netip.Addr
    InnerDstIP netip.Addr
}

type TransportHeader struct {
    SrcPort  uint16
    DstPort  uint16
    Protocol uint8  // 冗余存储，方便查询
    // TCP 特有字段
    TCPFlags uint8
    SeqNum   uint32
    AckNum   uint32
}

// internal/core/packet.go

type DecodedPacket struct {
    Timestamp  time.Time
    Ethernet   EthernetHeader
    IP         IPHeader
    Transport  TransportHeader
    Payload    []byte   // 应用层载荷，零拷贝切片
    CaptureLen uint32
    OrigLen    uint32
    Reassembled bool    // 是否经过分片重组
}
```

### 4.3 OutputPacket

```go
// internal/core/packet.go

type OutputPacket struct {
    // Envelope
    TaskID     string
    AgentID    string
    PipelineID int
    Timestamp  time.Time
    
    // Network context
    SrcIP      netip.Addr
    DstIP      netip.Addr
    SrcPort    uint16
    DstPort    uint16
    Protocol   uint8
    
    // Labels — Parser / Processor 标注
    Labels     Labels
    
    // Typed Payload — Parser 解析结果
    PayloadType string      // e.g. "sip", "rtp", "raw"
    Payload     any         // 具体类型由 PayloadType 决定，Reporter 做 type assertion
    RawPayload  []byte      // 原始载荷（可选保留）
}

// internal/core/labels.go

type Labels map[string]string

// 命名常量示例
const (
    LabelSIPMethod   = "sip.method"
    LabelSIPCallID   = "sip.call_id"
    LabelSIPFromURI  = "sip.from_uri"
    LabelSIPToURI    = "sip.to_uri"
    LabelSIPStatusCode = "sip.status_code"
)
```

### 4.4 Sentinel Errors

```go
// internal/core/errors.go

var (
    ErrTaskNotFound      = errors.New("otus: task not found")
    ErrTaskAlreadyExists = errors.New("otus: task already exists")
    ErrTaskStartFailed   = errors.New("otus: task start failed")
    ErrPipelineStopped   = errors.New("otus: pipeline stopped")
    ErrPacketTooShort    = errors.New("otus: packet too short")
    ErrUnsupportedProto  = errors.New("otus: unsupported protocol")
    ErrReassemblyTimeout = errors.New("otus: fragment reassembly timeout")
    ErrReassemblyLimit   = errors.New("otus: fragment reassembly limit exceeded")
    ErrPluginNotFound    = errors.New("otus: plugin not found")
    ErrPluginInitFailed  = errors.New("otus: plugin init failed")
    ErrConfigInvalid     = errors.New("otus: invalid configuration")
    ErrDaemonNotRunning  = errors.New("otus: daemon not running")
)
```

---

## 5. 插件接口定义

```go
// pkg/plugin/lifecycle.go

type Plugin interface {
    Name() string
    Init(cfg map[string]any) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

// pkg/plugin/capturer.go

type Capturer interface {
    Plugin
    // Capture 持续捕获，将 RawPacket 写入 output channel
    // 调用方负责关闭 ctx 来停止捕获
    Capture(ctx context.Context, output chan<- RawPacket) error
    // Stats 返回捕获统计（收到包数、丢弃数等）
    Stats() CaptureStats
}

type CaptureStats struct {
    PacketsReceived uint64
    PacketsDropped  uint64
    PacketsIfDropped uint64
}

// pkg/plugin/parser.go

type Parser interface {
    Plugin
    // CanHandle 快速判断是否能解析此包（基于端口/魔数/特征字节）
    // 应在 <100ns 内返回，不分配内存
    CanHandle(pkt *DecodedPacket) bool
    // Handle 完整解析，填充 Labels 并返回 typed payload
    Handle(pkt *DecodedPacket) (payload any, labels Labels, err error)
}

// pkg/plugin/processor.go

type Processor interface {
    Plugin
    // Process 对 OutputPacket 做过滤/标注
    // 返回 false 表示丢弃此包（过滤掉）
    Process(pkt *OutputPacket) (keep bool)
}

// pkg/plugin/reporter.go

type Reporter interface {
    Plugin
    // Report 将 OutputPacket 发送到外部系统
    Report(ctx context.Context, pkt *OutputPacket) error
    // Flush 强制刷新缓冲区（graceful shutdown 时调用）
    Flush(ctx context.Context) error
}
```

---

## 6. 配置结构

### 6.1 全局静态配置 (`configs/config.yml`)

```yaml
# 守护进程配置
daemon:
  pid_file: /var/run/otus.pid
  socket_path: /var/run/otus.sock
  
# 日志
log:
  level: info             # debug / info / warn / error
  format: json            # json / text
  outputs:
    - type: file
      path: /var/log/otus/otus.log
      max_size_mb: 100
      max_backups: 5
      max_age_days: 30
      compress: true
    - type: loki
      endpoint: http://loki:3100/loki/api/v1/push
      labels:
        app: otus
        env: production
      batch_size: 100
      flush_interval: 5s
    - type: stdout          # 开发调试时启用

# Kafka
kafka:
  brokers:
    - kafka-1:9092
    - kafka-2:9092
    - kafka-3:9092
  command_topic: otus-commands       # 命令 topic（输入）
  command_group: otus-agent-group    # Consumer Group
  
# 指标
metrics:
  enabled: true
  listen: :9090
  path: /metrics

# Agent 标识
agent:
  id: ""                  # 留空则自动生成（hostname-based）
  tags:
    region: cn-east-1
    role: edge
```

### 6.2 Task 动态配置（Kafka 命令 / CLI 创建）

```json
{
  "command": "task.create",
  "task": {
    "id": "sip-capture-01",
    "capture": {
      "type": "afpacket",
      "interface": "eth0",
      "bpf_filter": "udp port 5060",
      "fanout_size": 4,
      "snap_len": 65535
    },
    "decoder": {
      "tunnels": ["vxlan"],
      "ip_reassembly": true
    },
    "parsers": ["sip"],
    "processors": [
      {
        "type": "filter",
        "config": {
          "drop_if": "sip.method == 'OPTIONS'"
        }
      }
    ],
    "reporters": [
      {
        "type": "kafka",
        "config": {
          "topic": "otus-sip-data",
          "batch_size": 500,
          "flush_interval": "1s"
        }
      }
    ]
  }
}
```

---

## 7. 分步实施任务

每个步骤（Step）设计为一个独立的工作会话可以完成的量。步骤之间有依赖顺序。

### Step 1: 项目骨架搭建
**前置**: 无  
**目标**: 清空旧代码，搭建新的目录骨架和 go.mod

**任务清单**:
1. 备份/归档旧代码（`git tag v0-legacy`）
2. 删除所有旧源代码文件（保留 `doc/`, `configs/`, `LICENSE`, `README.md`, `Makefile`）
3. 重写 `go.mod`（确认 module path `firestige.xyz/otus`，Go 1.24）
4. 创建完整目录结构（按第 3 节）
5. 创建空的占位文件（`package xxx` 声明 + 简短 doc comment）
6. 验证 `go build ./...` 通过

**交付物**: 可编译的空骨架

---

### Step 2: 核心数据结构 + 错误定义
**前置**: Step 1  
**目标**: 实现 `internal/core/` 下所有数据结构

**任务清单**:
1. `internal/core/types.go` — EthernetHeader, IPHeader, TransportHeader
2. `internal/core/packet.go` — RawPacket, DecodedPacket, OutputPacket
3. `internal/core/labels.go` — Labels 类型 + 命名常量
4. `internal/core/errors.go` — 所有 sentinel errors
5. 单元测试：结构体零值、Labels 操作、error 判断

**交付物**: 核心数据结构可被其他包导入

---

### Step 3: 插件接口定义
**前置**: Step 2  
**目标**: 实现 `pkg/plugin/` 下所有接口

**任务清单**:
1. `pkg/plugin/lifecycle.go` — Plugin 基础接口
2. `pkg/plugin/capturer.go` — Capturer 接口 + CaptureStats
3. `pkg/plugin/parser.go` — Parser 接口（CanHandle + Handle）
4. `pkg/plugin/processor.go` — Processor 接口
5. `pkg/plugin/reporter.go` — Reporter 接口
6. `pkg/models/packet.go` — re-export core types（供外部使用）

**交付物**: 插件接口稳定，后续步骤实现具体插件

---

### Step 4: 全局配置加载
**前置**: Step 1  
**目标**: 实现配置加载和解析

**任务清单**:
1. `internal/config/config.go` — GlobalConfig 结构体 + `Load(path string)` 函数
2. `internal/config/task.go` — TaskConfig 结构体 + 校验
3. `configs/config.yml` — 更新为新格式模板
4. 单元测试：配置加载、默认值、校验

**交付物**: `config.Load("config.yml")` 返回结构化配置

---

### Step 5: 日志子系统
**前置**: Step 4  
**目标**: slog 初始化 + lumberjack 文件滚动 + Loki HTTP Push

**任务清单**:
1. `internal/log/logger.go` — 根据 GlobalConfig.Log 初始化 slog
   - 多输出 Handler（file + stdout + loki）
   - JSON / Text 格式切换
   - Level 动态设置
2. `internal/log/loki.go` — 实现 Loki Push API Writer
   - HTTP POST `/loki/api/v1/push`
   - 批量缓冲 + 定时刷新
   - 失败重试（简单指数退避）
   - 实现 `io.Writer` 接口，可被 slog Handler 使用
3. 单元测试：日志格式、级别过滤
4. 集成测试：Loki push mock server

**交付物**: `log.Init(cfg)` 后全局 slog 可用

---

### Step 6: L2-L4 协议栈解码器
**前置**: Step 2  
**目标**: 实现核心解码链路 RawPacket → DecodedPacket

**任务清单**:
1. `internal/core/decoder/decoder.go` — Decoder 接口 + 主解码入口 `Decode(raw RawPacket) (DecodedPacket, error)`
2. `internal/core/decoder/ethernet.go` — L2 以太网解码（含 VLAN/QinQ 剥离）
3. `internal/core/decoder/tunnel.go` — 隧道解封装（VXLAN/GRE/Geneve/IPIP）
4. `internal/core/decoder/ip.go` — IPv4/IPv6 头部解码
5. `internal/core/decoder/transport.go` — UDP/TCP 头部解码
6. `internal/core/decoder/reassembly.go` — IPv4 分片重组
   - 硬上限：最大分片数、最大重组帧大小、超时时间
   - 独立内存分配（非零拷贝）
7. 基准测试：正常包 decode 延迟、吞吐量
8. 单元测试：各协议头解码、VLAN 剥离、分片重组

**交付物**: `decoder.Decode(raw)` 返回填充完整的 DecodedPacket

---

### Step 7: Pipeline 引擎
**前置**: Step 3, Step 6  
**目标**: 实现 Pipeline 主循环

**任务清单**:
1. `internal/pipeline/pipeline.go` — Pipeline 结构体
   - 单 goroutine 主循环：从 Capturer 读取 → Decode → Parser 链 → Processor 链 → Reporter 链
   - 零 channel 内部传递（函数调用链）
   - 背压控制：非阻塞 drop + 计数器
2. `internal/pipeline/builder.go` — 从 TaskConfig 构建 Pipeline
   - 插件查找和实例化
   - 配置注入
3. `internal/pipeline/metrics.go` — Pipeline 级 Prometheus 计数器
   - received / decoded / parsed / processed / reported / dropped per-stage
4. 集成测试：mock capturer → mock parser → mock reporter

**交付物**: Pipeline 可端到端处理数据包

---

### Step 8: Task 管理器
**前置**: Step 7  
**目标**: Task 生命周期管理

**任务清单**:
1. `internal/task/task.go` — Task 实体
   - 状态机：Created → Starting → Running → Stopping → Stopped / Failed
   - 持有 Pipeline 实例集合（fanout_size 决定数量）
2. `internal/task/manager.go` — TaskManager
   - Create / Delete / List / Status / Get
   - Phase 1 限制：最多 1 个 Task
3. `internal/task/flow_registry.go` — FlowRegistry
   - sync.Map 实现
   - per-Task 独立实例
   - 基于 5-tuple 的流查找
4. 单元测试：状态机转换、create/delete 生命周期

**交付物**: TaskManager 可管理 Task 生命周期

---

### Step 9: AF_PACKET 捕获插件
**前置**: Step 3  
**目标**: 实现 AF_PACKET_V3 Capturer 插件

**任务清单**:
1. `plugins/capture/afpacket/afpacket.go`
   - TPacket_V3 ring buffer
   - PACKET_FANOUT（FANOUT_HASH）
   - BPF 过滤器设置
   - 零拷贝读取 → RawPacket
2. 参考旧代码：`internal/otus/module/capture/handle/handle_afpacket.go`
3. BPF 编译工具：参考 `internal/utils/bpf.go`
4. 集成测试（需要 root 权限 + 实际网卡，CI 中可能跳过）

**交付物**: AF_PACKET Capturer 可正常抓包

---

### Step 10: SIP 解析插件
**前置**: Step 3  
**目标**: 实现 SIP Parser 插件

**任务清单**:
1. `plugins/parser/sip/sip.go`
   - `CanHandle()`: 检查端口 5060 或 payload 前缀特征（"SIP/2.0", "INVITE", "REGISTER" 等）
   - `Handle()`: 解析 SIP 头部，返回 SIP message struct + Labels
2. 参考旧代码：`plugins/parser/sip/sip_parser.go`
3. 单元测试：各种 SIP 方法、malformed 输入
4. 基准测试：CanHandle + Handle 延迟

**交付物**: SIP Parser 可解析 SIP 消息

---

### Step 11: Kafka Reporter 插件
**前置**: Step 3  
**目标**: 实现 Kafka Producer Reporter

**任务清单**:
1. `plugins/reporter/kafka/kafka.go`
   - segmentio/kafka-go Writer（批量 + 异步）
   - OutputPacket → JSON/Protobuf 序列化
   - 失败重试策略
   - Flush() 实现
2. `plugins/reporter/console/console.go` — 控制台调试输出
3. 单元测试：序列化格式、mock Writer

**交付物**: Reporter 可将数据发送到 Kafka

---

### Step 12: 控制面 — Kafka 命令订阅
**前置**: Step 8  
**目标**: 实现远程控制通道

**任务清单**:
1. `internal/command/handler.go` — 命令路由
   - `task.create` → TaskManager.Create()
   - `task.delete` → TaskManager.Delete()
   - `task.list` → TaskManager.List()
   - `task.status` → TaskManager.Status()
   - `reload` → 重载全局配置
2. `internal/command/kafka.go` — Kafka Consumer
   - segmentio/kafka-go Reader
   - Consumer Group 订阅命令 topic
   - JSON 反序列化 → 命令分发
3. 集成测试：mock Kafka → 命令执行

**交付物**: 从 Kafka 接收命令可驱动 Task 生命周期

---

### Step 13: 控制面 — UDS 本地通信
**前置**: Step 8  
**目标**: 实现 CLI ↔ daemon 本地控制

**任务清单**:
1. `internal/command/uds_server.go` — JSON-RPC Server
   - Unix Domain Socket 监听
   - 协议：`{"jsonrpc":"2.0","method":"task.create","params":{...},"id":1}`
   - 路由到 handler.go 同一套处理函数
2. `internal/command/uds_client.go` — JSON-RPC Client（CLI 使用）
   - Dial UDS + 发送 request + 等待 response
3. 单元测试：request/response 序列化、error 码

**交付物**: CLI 命令可通过 UDS 控制 daemon

---

### Step 14: CLI 命令重写
**前置**: Step 13  
**目标**: 重写 cobra CLI

**任务清单**:
1. `cmd/root.go` — root command + 全局 flags（`--config`, `--socket`）
2. `cmd/daemon.go` — `otus daemon` 子命令
   - 前台启动（默认）或后台 daemon 化
   - PID file 管理
   - 信号处理（SIGTERM graceful shutdown, SIGHUP reload）
3. `cmd/task.go` — `otus task` 子命令组
   - `otus task create -f task.json`
   - `otus task delete <task-id>`
   - `otus task list`
   - `otus task status <task-id>`
4. `cmd/stop.go` — `otus stop`（发送 shutdown 命令到 daemon）
5. `cmd/reload.go` — `otus reload`（发送 reload 命令到 daemon）
6. `main.go` — 调用 `cmd.Execute()`

**交付物**: CLI 可完整控制 daemon 和 Task

---

### Step 15: Daemon 组装 + Graceful Shutdown
**前置**: Step 5, 7, 8, 12, 13  
**目标**: 组装完整 daemon

**任务清单**:
1. `internal/daemon/daemon.go`
   - 加载配置 → 初始化日志 → 启动指标 → 启动 UDS Server → 启动 Kafka 命令订阅 → 等待信号
   - Graceful shutdown 顺序：停止 Kafka consumer → 停止所有 Task → Flush reporters → 关闭 UDS → 关闭日志
   - PID file 管理
2. 集成测试：启动 → 创建 Task → 停止

**交付物**: `otus daemon` 可完整运行

---

### Step 16: Prometheus 指标
**前置**: Step 15  
**目标**: 暴露 Prometheus 指标端点

**任务清单**:
1. `internal/metrics/metrics.go` — 全局指标注册
   - `otus_capture_packets_total` (counter, labels: task, interface)
   - `otus_capture_drops_total` (counter, labels: task, stage)  
   - `otus_pipeline_packets_total` (counter, labels: task, pipeline, stage)
   - `otus_pipeline_latency_seconds` (histogram, labels: task, stage)
   - `otus_task_status` (gauge, labels: task, status)
   - `otus_reassembly_active_fragments` (gauge)
2. `internal/metrics/server.go` — HTTP server
3. 验证 Prometheus scrape

**交付物**: `/metrics` 端点返回完整指标

---

### Step 17: systemd 集成 + 部署
**前置**: Step 15  
**目标**: 生产就绪的部署配置

**任务清单**:
1. 更新 `configs/otus.service` — systemd unit file
2. 更新 `Makefile` — build / install / clean targets
3. 编写 `scripts/build.sh` — 多架构交叉编译（amd64 / arm64）
4. 更新 `README.md` — 安装和使用说明
5. 编写 Dockerfile（可选）

**交付物**: 可部署运行的完整系统

---

## 8. 旧代码参考索引

以下旧代码文件包含可参考的算法或配置，但**不应直接复制**——需要按新接口重写。

| 旧文件路径 | 参考价值 | 新代码对应位置 |
|------------|---------|---------------|
| `internal/utils/bpf.go` | BPF 过滤器编译逻辑 | `plugins/capture/afpacket/` |
| `internal/otus/module/capture/handle/handle_afpacket.go` | AF_PACKET TPacket V3 配置（ring buffer 参数、fanout 模式） | `plugins/capture/afpacket/` |
| `internal/otus/module/capture/codec/assembly_ipv4.go` | IPv4 分片重组算法思路 | `internal/core/decoder/reassembly.go` |
| `plugins/parser/sip/sip_parser.go` | SIP 协议检测和完整解析逻辑 | `plugins/parser/sip/` |
| `internal/daemon/manager.go` | daemon 进程管理模式（PID file, signal） | `internal/daemon/daemon.go` |
| `cmd/*.go` | cobra 命令结构和 UDS 通信模式 | `cmd/` |
| `internal/otus/module/pipeline/pipeline.go` | Pipeline 连接模型参考 | `internal/pipeline/pipeline.go` |
| `plugins/reporter/consolelog/reporter.go` | 控制台输出 Reporter 格式 | `plugins/reporter/console/` |

---

## 9. 验收标准（Phase 1 完成时）

- [ ] `otus daemon` 可以前台启动，加载配置，监听 UDS
- [ ] `otus task create -f task.json` 可以通过 UDS 创建 SIP 抓包任务
- [ ] AF_PACKET 捕获 → L2-L4 解码 → SIP 解析 → Kafka 上报 全链路跑通
- [ ] Kafka 命令 topic 可以远程创建/删除 Task
- [ ] IP 分片重组正常工作，有硬上限保护
- [ ] 日志输出到文件（lumberjack 滚动）和 Loki
- [ ] Prometheus `/metrics` 端点返回各层指标
- [ ] `otus stop` 可以 graceful shutdown
- [ ] 性能：单核 ≥200K pps (SIP 完整解析)
- [ ] 静态编译二进制，支持 amd64 / arm64
- [ ] systemd service 可正常运行

---

**文档版本**: v0.1.0  
**创建日期**: 2026-02-16  
**作者**: Otus Team
