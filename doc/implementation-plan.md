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
│   │   ├── handler.go               // 命令处理器（task_create / task_delete 等）
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
│   │       └── filter.go            // 应用层过滤（基于字段/Labels 条件 keep/drop）+ Labels 标注 Processor
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

> 完整 YAML 层级、Go 结构体映射和设计决策详见 [config-design.md](config-design.md)。以下为精简示例。

```yaml
otus:
  node:
    ip: ""                       # 空 = 自动探测（ADR-023: env > auto-detect > error）
    hostname: edge-beijing-01
    tags:
      datacenter: cn-north
      environment: production

  control:
    socket: /var/run/otus.sock
    pid_file: /var/run/otus.pid

  # Kafka 全局默认（ADR-024: command_channel 和 reporters 继承）
  kafka:
    brokers:
      - kafka-1:9092
      - kafka-2:9092
      - kafka-3:9092
    sasl:
      enabled: false
    tls:
      enabled: false

  command_channel:
    enabled: true
    type: kafka
    kafka:
      # brokers/sasl/tls 继承自 otus.kafka
      topic: otus-commands
      response_topic: otus-responses   # ADR-029: 响应回写 topic，空字符串禁用
      group_id: "otus-${node.hostname}"
      auto_offset_reset: latest

  reporters:
    kafka:
      # brokers/sasl/tls 继承自 otus.kafka
      compression: snappy
      max_message_bytes: 1048576

  resources:
    max_workers: 0               # 0 = auto（GOMAXPROCS）

  backpressure:
    pipeline_channel:
      capacity: 65536
      drop_policy: tail
    send_buffer:
      capacity: 16384
      drop_policy: head
      high_watermark: 0.8
      low_watermark: 0.3
    reporter:
      send_timeout: 3s
      max_retries: 1

  core:
    decoder:
      tunnel:
        vxlan: false
        gre: false
        geneve: false
        ipip: false
      ip_reassembly:
        timeout: 30s
        max_fragments: 10000

  metrics:
    enabled: true
    listen: :9091
    path: /metrics

  log:
    level: info
    format: json
    outputs:
      file:
        enabled: true
        path: /var/log/otus/otus.log
        rotation:
          max_size_mb: 100       # ADR-025: 数值字段
          max_age_days: 30
          max_backups: 5
          compress: true
      loki:
        enabled: false
        endpoint: http://loki:3100/loki/api/v1/push
        labels:
          app: otus
          env: production
        batch_size: 100
        batch_timeout: 1s
```

### 6.2 Task 动态配置（Kafka 命令 / CLI 创建）

```json
{
  "command": "task_create",
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

## 7. 工程规范

### 7.1 旧代码处理

1. 实施前打 tag 归档：`git tag v0-legacy`
2. 删除所有旧 Go 源码，只保留 `doc/`、`configs/`、`LICENSE`、`README.md`、`Makefile`
3. 不做渐进式迁移——旧代码在仓库中只产生干扰（IDE 跳转、import 冲突、编译噪音）
4. 需要参考算法时通过 `git show v0-legacy:path/to/file` 查看

### 7.2 版本兼容

**不保留向后兼容。** Otus 目前没有外部使用者依赖其 API，不存在兼容义务。Module path 保持 `firestige.xyz/otus` 不变，但接口全部重新定义。API 稳定性承诺从新接口的 v1.0 开始。

### 7.3 测试策略

- **旧测试**：随旧代码一并删除，不尝试修复
- **新测试**：每个 Step 的交付标准是"该 Step 的测试全部通过"
- 测试失败优先修改实现而非跳过测试
- 需要 root 权限或真实硬件的集成测试用 `//go:build integration` 标记，CI 中可跳过

### 7.4 编译问题处理

| 情况 | 处理方式 |
|------|---------|
| 类型不匹配 / 循环依赖 | 说明问题根因，提出包结构调整方案，确认后执行 |
| 外部依赖 API 变更 | 查文档定位正确 API，降级版本或换用替代库 |
| 设计层面的矛盾 | 停下来，报告问题，提出 2-3 个修改方案由用户决策 |
| 多次尝试仍无法编译通过 | 回退到上一个可编译状态，拆成更小增量步骤重试 |

**核心原则：每次 commit 必须是可编译的。** 不提交编译不过的代码。如果某个 Step 做到一半发现方向不对，宁可回退也不留半成品。

### 7.5 决策保护红线（⚠️ 最高优先级）

> **AI Agent 不被允许自主违背或修改 `doc/decisions.md` 和 `doc/architecture.md` 中已确认的设计决策来解决实现问题。**

具体约束：

1. **禁止自主变更决策**：如果编码过程中发现某个 ADR 的决定在实现中存在困难，不得擅自更换技术选型、修改接口设计、或改变架构模式来绕过问题
2. **两次失败即停止**：如果同一个问题连续尝试 **2 次** 仍无法在既定设计框架内解决，必须 **立即停止编码**
3. **记录问题**：将问题的具体表现、尝试过的方案、失败原因记录下来
4. **向用户求助**：报告问题并等待用户决策，由用户判断是否需要调整设计

**触发此规则的典型场景**：
- 选定的库 API 与设计的接口不匹配
- 包结构出现循环依赖无法在现有布局下解决
- 性能目标与设计模式存在根本矛盾
- 数据结构定义在实际编码中发现不合理

**正确处理流程**：
```
发现问题 → 第 1 次尝试在设计框架内解决 → 失败
         → 第 2 次尝试换一种实现方式 → 仍失败
         → 停止 → 记录问题 → 向用户报告并等待决策
```

---

## 8. 分步实施任务

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

### Step 7.5: 插件 Registry
**前置**: Step 3  
**目标**: 实现全局插件注册表（见 ADR-022）

**任务清单**:
1. `pkg/plugin/registry.go` — 全局 Registry 实现
   - Factory 类型定义：`CapturerFactory`, `ParserFactory`, `ProcessorFactory`, `ReporterFactory`
   - 注册 API：`RegisterCapturer(name, factory)`, `RegisterParser(...)` 等
   - 查找 API：`GetCapturerFactory(name)`, `GetParserFactory(...)` 等
   - 枚举 API：`ListCapturers()`, `ListParsers()` 等（调试/状态查询用）
   - 安全约束：重复注册同名同类型 panic，查找不到返回 `ErrPluginNotFound`
2. `pkg/plugin/registry_test.go` — 单元测试
   - 注册 + 查找 + 类型安全
   - 重复注册 panic
   - 查找不存在返回 error
   - List 枚举
3. `internal/config/task.go` — 更新 TaskConfig
   - `Parsers []string` → `Parsers []ParserConfig`（含 Plugin + Config 字段）
   - 对齐架构文档 4.4.2 节的 Task 配置 YAML 结构
4. 更新 `internal/config/task_test.go` — 适配新结构

**交付物**: `plugin.RegisterParser("sip", factory)` + `plugin.GetParserFactory("sip")` 可用

---

### ✅ Step 8: Task 管理器
**前置**: Step 7, Step 7.5  
**目标**: Task 生命周期管理
**状态**: ✅ 已完成

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

### ✅ Step 9: AF_PACKET 捕获插件
**前置**: Step 3  
**目标**: 实现 AF_PACKET_V3 Capturer 插件
**状态**: ✅ 已完成 (commit e76715c)

**任务清单**:
1. ✅ `plugins/capture/afpacket/afpacket.go`
   - TPacket_V3 ring buffer (4MB block, 128 blocks)
   - PACKET_FANOUT (hash mode, configurable fanout_id)
   - BPF 过滤器 (pcap.CompileBPFFilter → bpf.RawInstruction)
   - 零拷贝读取 → RawPacket (gopacket.NoCopy = true)
2. ✅ BPF 编译: 使用 pcap.CompileBPFFilter + 转换为 bpf.RawInstruction
3. ✅ 注册插件: plugins/init.go 中注册 "afpacket"
4. ⚠️ 集成测试: 需要 root 权限 + 实际网卡（暂未实现，待后续）

**交付物**: ✅ AF_PACKET Capturer 编译通过，插件已注册

**实现要点**:
- 使用 `afpacket.OptInterface`、`OptTPacketVersion3`、`OptBlockSize` 等配置选项
- Fanout 模式通过 `handle.SetFanout()` 设置（在 TPacket 创建后）
- BPF 过滤器编译流程：字符串 → pcap.CompileBPFFilter → 转换 Code→Op → bpf.RawInstruction → handle.SetBPF
- 统计信息从 `handle.SocketStats()` 获取 Drops()
- 非阻塞发送到 output channel，满了则丢包并计数

---

### ✅ Step 10: SIP 解析插件
**前置**: Step 3  
**目标**: 实现 SIP Parser 插件  
**状态**: ✅ 已完成 (commit 43c0db9)

**任务清单**:
1. ✅ `plugins/parser/sip/sip.go` (550+ 行)
   - `CanHandle()`: 端口 5060/5061 + SIP 魔数匹配（~1.9ns）
   - `Handle()`: 完整 SIP 头部解析 + SDP 解析（~1.45μs）
   - 会话状态管理：INVITE offer → 200 OK answer 关联（go-cache, 24h TTL）
   - FlowRegistry 双向注册：RTP/RTCP 流（支持 rtcp-mux）
   - BYE/CANCEL 清理逻辑
2. ✅ 单元测试：`sip_test.go` (600+ 行)
   - CanHandle (端口 + 魔数), extractURI, parseSIPMessage
   - SDP 解析（单/多媒体流, RTCP-MUX, 显式 RTCP 端口）
   - INVITE/200 OK/BYE 完整流程
   - 多通道场景测试（2 audio + 1 video = 12 FlowKeys）
3. ✅ 基准测试：CanHandle ~1.9ns, Handle ~1.45μs

**实现细节**:
- **SIP 头部解析**（无正则，手写状态机）：
  - Request-Line/Status-Line, Call-ID, From/To URI, Via（逗号分隔列表）, CSeq
  - Header folding 支持（多行头部）
- **SDP 解析**（m=, a=, c= 行）：
  - `m=`: 媒体类型、RTP 端口、协议
  - `a=rtcp-mux`: RTCP 复用 RTP 端口
  - `a=rtcp:端口`: 显式 RTCP 端口（优先级高于端口+1）
  - `a=rtpmap:`: 编解码器信息（仅保留第一个）
  - `c=`: 连接地址（会话级/媒体级）
- **多通道支持**：
  - 所有 m= 行都被解析并存储到 `mediaStreams[]` 数组
  - `registerMediaFlows()` 遍历所有媒体流，每个注册双向 FlowKey
  - 例如：2 audio + 1 video → 12 FlowKeys（6 RTP + 6 RTCP，双向）
- **会话状态**（Call-ID → sipSession）：
  - INVITE: 存储 offer SDP
  - 200 OK: 取出 offer，组合 answer，注册完整五元组
  - BYE/CANCEL: 清理 FlowRegistry 和 session cache
- **Labels 输出**：
  - `sip.method`, `sip.call_id`, `sip.from_uri`, `sip.to_uri`, `sip.via`, `sip.status_code`
  - Payload 返回 `nil`（原始报文在 `OutputPacket.RawPayload`）

**交付物**: SIP Parser 可解析 SIP 消息，提取关键头部，注册 RTP/RTCP 流到 FlowRegistry

---

### Step 11: Kafka Reporter 插件
**前置**: Step 3  
**目标**: 实现 Kafka Producer Reporter  
**状态**: ✅ 已完成

**任务清单**:
1. ✅ `plugins/reporter/kafka/kafka.go` (257 行)
   - segmentio/kafka-go Writer（批量配置：BatchSize=100, BatchTimeout=100ms）
   - OutputPacket → JSON 序列化（timestamp as UnixMilli, 5-tuple as key）
   - Labels → Kafka Headers 映射
   - 失败重试策略（MaxAttempts=3）
   - 压缩支持（none/gzip/snappy/lz4，默认 snappy）
   - Flush() 实现（依赖 Writer 自动批量刷新）
2. ✅ `plugins/reporter/console/console.go` (150 行)
   - 两种输出格式：JSON（结构化）和 Text（人类可读）
   - 统计计数器（reportedCount）
   - Stdout 自动刷新，Flush() 为空操作
3. ✅ 单元测试：
   - `console_test.go`: 配置解析、格式验证、生命周期测试
   - `kafka_test.go`: 配置解析、序列化格式验证、压缩类型测试

**实现细节**:
- **Kafka Reporter**:
  - 使用 kafka.Hash Balancer 实现一致性分区路由
  - Message Key = 五元组字符串（SrcIP:SrcPort-DstIP:DstPort）
  - 同步写入模式（Async=false）以正确处理错误
  - CompressionCodec 通过 compress.Compression.Codec() 方法获取
- **Console Reporter**:
  - JSON 格式：完整 OutputPacket 字段（timestamp 为 RFC3339）
  - Text 格式：[HH:MM:SS.mmm] SrcIP:Port → DstIP:Port proto=N type=X
  - 配置验证：format 只允许 "json" 或 "text"

**交付物**: Reporter 可将数据发送到 Kafka 或控制台调试输出

---

### Step 12: 控制面 — Kafka 命令订阅
**前置**: Step 8  
**目标**: 实现远程控制通道  
**状态**: ✅ 已完成

**任务清单**:
1. ✅ `internal/command/handler.go` (270 行)
   - CommandHandler 结构体，包含 TaskManager 和 ConfigReloader
   - Command/Response 结构体（类 JSON-RPC 协议）
   - 错误码定义（ParseError, InvalidRequest, MethodNotFound, InvalidParams, InternalError）
   - 5 个命令处理器：
     - `task_create` → TaskManager.Create()
     - `task_delete` → TaskManager.Delete()
     - `task_list` → TaskManager.List()
     - `task_status` → TaskManager.Status() (支持查询单个或全部)
     - `config_reload` → ConfigReloader.Reload()
2. ✅ `internal/command/kafka.go` (180 行)
   - KafkaCommandConsumer 结构体
   - segmentio/kafka-go Reader（Consumer Group 模式）
   - 配置：Brokers, Topic, GroupID, StartOffset (earliest/latest), PollInterval, MaxRetries
   - Start() 方法：阻塞式消费循环，自动提交 offset
   - processMessage() 方法：JSON 反序列化 → 调用 handler.Handle()
   - 优雅停止：Stop() 关闭 reader
3. ✅ 单元测试：
   - `handler_test.go`: 7 个测试用例（task_create/delete/list/status, config_reload, 未知方法, 非法参数）
   - `kafka_test.go`: 4 个测试用例（配置验证、默认值、生命周期、StartOffset）

**实现细节**:
- **命令协议格式**（类 JSON-RPC）：
  ```json
  {
    "method": "task_create",
    "params": {...},
    "id": "req-123"
  }
  ```
- **响应格式**：
  ```json
  {
    "id": "req-123",
    "result": {...},
    "error": {"code": -32603, "message": "..."}
  }
  ```
- **Kafka Consumer 配置**：
  - MinBytes=1, MaxBytes=10MB
  - CommitInterval=1s (自动提交)
  - MaxWait=PollInterval (默认 1s)
- **错误处理**：
  - 消息处理失败不中断消费循环
  - 记录详细日志（partition, offset, error message）
  - 支持优雅停止（context 取消传播）

**交付物**: 从 Kafka 接收命令可驱动 Task 生命周期

---

### Step 12.5: Kafka 命令响应通道 (ADR-029)
**前置**: Step 12  
**目标**: 实现命令执行结果回写，打通近端→远端响应链路  
**状态**: ✅ 已完成

#### 背景

`processMessage()` 调用 `handler.Handle()` 后得到 `Response` 对象，当前该对象被直接丢弃。
所有需要返回数据的命令（`task_list`, `task_status`, `daemon_status`, `daemon_stats`）
均无法将结果送达远端调用方。本步骤在 `KafkaCommandConsumer` 中新增 Kafka Producer，
在命令执行后将响应写入 `otus-responses` topic（见 [ADR-029](decisions.md#adr-029-kafka-命令响应通道)）。

#### 任务清单

**A. 新增 `KafkaResponse` 线格式**  
在 `internal/command/kafka.go` 中定义：

```go
// KafkaResponse is the wire format for command responses written to the response topic (ADR-029).
type KafkaResponse struct {
    Version   string      `json:"version"`    // "v1"
    Source    string      `json:"source"`     // agent hostname
    Command   string      `json:"command"`    // echoed from KafkaCommand
    RequestID string      `json:"request_id"` // correlation ID
    Timestamp time.Time   `json:"timestamp"`  // when the response was produced
    Result    interface{} `json:"result,omitempty"`
    Error     *ErrorInfo  `json:"error,omitempty"`
}
```

**B. `KafkaCommandConsumer` 新增 writer 字段**

```go
type KafkaCommandConsumer struct {
    ccConfig config.CommandChannelConfig
    hostname string
    reader   *kafka.Reader
    writer   *kafka.Writer   // nil when response_topic is empty
    handler  *CommandHandler
    ttl      time.Duration
}
```

**C. 更新 `NewKafkaCommandConsumer`**
- 当 `ccConfig.Kafka.ResponseTopic != ""` 时创建 `kafka.Writer`：
  ```go
  writer = &kafka.Writer{
      Addr:         kafka.TCP(kc.Brokers...),
      Topic:        kc.ResponseTopic,
      Balancer:     &kafka.Hash{},       // hostname 做 key → 固定 partition
      RequiredAcks: kafka.RequireOne,
      Async:        false,               // 同步写入，失败可 log
  }
  ```
- `ResponseTopic` 为空时 `writer` 保持 `nil`，跳过写回逻辑（向后兼容）

**D. 更新 `processMessage`**

在步骤 5（`handler.Handle()`）之后新增步骤 6：

```go
// 6. Write response back to Kafka if response channel is configured (ADR-029)
if c.writer != nil && cmd.ID != "" {
    if err := c.writeResponse(ctx, kCmd.Command, response); err != nil {
        slog.Error("failed to write kafka response",
            "request_id", cmd.ID,
            "error", err,
        )
        // intentionally not returned: command already executed
    }
}
```

```go
func (c *KafkaCommandConsumer) writeResponse(ctx context.Context, command string, resp Response) error {
    kr := KafkaResponse{
        Version:   "v1",
        Source:    c.hostname,
        Command:   command,
        RequestID: resp.ID,
        Timestamp: time.Now().UTC(),
        Result:    resp.Result,
        Error:     resp.Error,
    }
    data, err := json.Marshal(kr)
    if err != nil {
        return fmt.Errorf("marshal response: %w", err)
    }
    return c.writer.WriteMessages(ctx, kafka.Message{
        Key:   []byte(c.hostname), // consistent partition routing
        Value: data,
    })
}
```

**E. 更新 `Stop()`**

关闭时显式 flush + 关闭 writer：

```go
func (c *KafkaCommandConsumer) Stop() error {
    var errs []error
    if c.writer != nil {
        writer := c.writer
        c.writer = nil
        if err := writer.Close(); err != nil {
            errs = append(errs, fmt.Errorf("close writer: %w", err))
        }
    }
    if c.reader != nil {
        reader := c.reader
        c.reader = nil
        if err := reader.Close(); err != nil {
            errs = append(errs, fmt.Errorf("close reader: %w", err))
        }
    }
    return errors.Join(errs...)
}
```

**F. 单元测试** — `kafka_test.go` 新增用例：

| 测试名 | 验证点 |
|--------|--------|
| `TestKafkaResponse_WrittenWhenResponseTopicSet` | `response_topic` 非空时构造函数创建 writer |
| `TestKafkaResponse_SkippedWhenResponseTopicEmpty` | `response_topic` 为空时 writer 为 nil，不写回 |
| `TestKafkaResponse_SkippedWhenRequestIDEmpty` | `request_id` 为空时不写回 |
| `TestWriteResponse_MarshalAndKey` | 响应 JSON 包含正确字段，message key = hostname |
| `TestStop_ClosesWriter` | Stop() 关闭 writer 不 panic |

由于测试不连真实 Kafka，测试 writer 实例化和序列化逻辑使用接口注入或
`newTestConsumerWithMockWriter()` helper 覆盖 writer 字段。

#### 边界条件

| 场景 | 处理 |
|------|------|
| `response_topic` 为空 | `writer = nil`，直接跳过写回，完全向后兼容 |
| `request_id` 为空 | 跳过写回（无 correlation ID 无意义）|
| Kafka broker 不可达 | 写回失败记 ERROR 日志；命令已执行，**不回滚**，不影响 consumer 循环 |
| 命令执行失败 | `response.Error` 非 nil，仍写回（让调用方知道失败原因）|
| 广播命令 (`target: "*"`) | 每个节点各自写回，调用方按 `source` + `request_id` 聚合 |

#### 交付物验证

```bash
# 近端 consumer 日志应出现：
# "kafka response written" request_id=req-001 source=edge-beijing-01

# 远端可从 otus-responses 消费到：
{
  "version": "v1",
  "source": "edge-beijing-01",
  "command": "task_list",
  "request_id": "req-001",
  "timestamp": "2026-02-21T...",
  "result": {"tasks": ["voip-monitor-01"], "count": 1}
}
```

**交付物**: `task_list`/`task_status`/`daemon_status`/`daemon_stats` 的执行结果可从
`otus-responses` topic 消费；所有现有测试继续通过

---

### Step 13: 控制面 — UDS 本地通信
**前置**: Step 8  
**目标**: 实现 CLI ↔ daemon 本地控制  
**状态**: ✅ 已完成

**任务清单**:
1. ✅ `internal/command/uds_server.go` (210 行)
   - UDSServer 结构体，管理 listener 和多个连接
   - Start() 方法：创建 Unix listener，设置权限 0600，接受连接循环
   - acceptLoop() 方法：并发接受多个连接
   - handleConnection() 方法：每个连接一个 goroutine，处理 JSON-RPC 请求
   - Stop() 方法：优雅停止（关闭 listener，关闭所有连接，等待 goroutine 结束）
   - JSONRPCRequest/JSONRPCResponse 结构体（JSON-RPC 2.0 协议）
2. ✅ `internal/command/uds_client.go` (145 行)
   - UDSClient 结构体，包含 socketPath 和 timeout
   - Call() 方法：通用调用方法（连接 UDS，发送请求，接收响应，ID 验证）
   - 便捷方法：TaskCreate(), TaskDelete(), TaskList(), TaskStatus(), ConfigReload(), Ping()
   - 超时控制：默认 10s，支持 context deadline
   - 错误处理：连接失败、超时、ID 不匹配
3. ✅ 单元测试：`uds_test.go` (8 个测试用例)
   - 集成测试：Server + Client 完整交互
   - 错误测试：连接失败、超时
   - 并发测试：多个客户端同时连接
   - 便捷方法测试：所有便捷方法调用

**实现细节**:
- **JSON-RPC 2.0 协议格式**：
  ```json
  # Request
  {"jsonrpc":"2.0","method":"task_create","params":{...},"id":"req-123"}
  
  # Response
  {"jsonrpc":"2.0","id":"req-123","result":{...}}
  # or
  {"jsonrpc":"2.0","id":"req-123","error":{"code":-32601,"message":"..."}}
  ```
- **请求 ID 格式**：字符串 "req-{timestamp}" 避免 JSON 数字精度问题
- **连接管理**：
  - 每个连接独立 goroutine，使用 bufio.Scanner 读取行分隔的 JSON
  - 连接追踪（map[net.Conn]struct{}）用于优雅停止
  - WaitGroup 确保所有 handler 完成后才退出
- **权限设置**：socket 文件权限 0600（仅 owner 可读写）
- **资源清理**：
  - Server Stop 时删除 socket 文件
  - Client 每次调用后关闭连接（短连接模式）

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

### Step 14.5: CLI 补全 + YAML 支持 + 配置/Kafka 决策落地
**前置**: Step 14  
**目标**: 补充缺失的 CLI 命令和输入格式；落地 config-design.md 中的新增设计决策

**任务清单**:

#### A. CLI 补全
1. CLI 支持 YAML 格式 task 文件：`otus task create -f task.yaml`
   - 自动检测 JSON/YAML（基于扩展名或内容探测）
   - 内部统一转换为 `TaskConfig` 结构
2. `cmd/status.go` — `otus status`：查询 daemon 整体状态（版本、运行时间、Task 数量）
3. `cmd/stats.go` — `otus stats`：查询运行时统计（抓包速率、丢包数）
4. `cmd/validate.go` — `otus validate -f task.yaml`：预校验 task 配置文件（不创建 Task）
5. 补充 `daemon_shutdown` 命令到 CommandHandler（当前 `otus stop` 直接 os.Kill，应改为命令优雅停止）

#### B. 全局配置重构（ADR-023/024/025）
6. `internal/config/config.go` — 重构 GlobalConfig 结构体
   - 结构体对齐 [config-design.md](config-design.md) 附录 A（含 `GlobalKafkaConfig`）
   - `validateAndApplyDefaults()` 实现 Kafka 继承逻辑（ADR-024: `otus.kafka` → `command_channel.kafka` / `reporters.kafka`）
   - `resolveNodeIP()` 实现 Node IP 解析（ADR-023: env > auto-detect > error）
   - 日志滚动字段改为 `MaxSizeMB` / `MaxAgeDays`（ADR-025）
7. `configs/config.yml` — 更新为 `otus:` 嵌套格式（对齐 config-design.md §2）
8. 单元测试：Kafka 继承合并、Node IP 解析、配置加载

#### C. Kafka 命令格式升级（ADR-026）
9. `internal/command/kafka.go` — `processMessage()` 升级
   - 解析 `KafkaCommand{version, target, command, timestamp, request_id, payload}` 格式
   - 增加 target 过滤（匹配 `node.hostname` 或 `"*"`）
   - 增加 timestamp TTL 检查（可配置 `command_ttl`，默认 5m）
   - 转换为内部 `Command{Method, Params, ID}` 后调用 handler
10. 使用新的 `CommandChannelConfig`（继承 `otus.kafka` 全局默认）
11. 单元测试：target 过滤、TTL 拒绝过期命令、KafkaCommand 解析

#### D. Kafka Reporter 增强（ADR-027/028）
12. `plugins/reporter/kafka/kafka.go` — 增加动态 topic 路由
    - `TopicPrefix` 配置 + `resolveTopic()` 方法（`topic_prefix` 与 `topic` 互斥）
    - Envelope 信息迁移到 Kafka Headers（task_id, agent_id, payload_type, src_ip, dst_ip, timestamp, l.* labels）
    - 可配置 `serialization: json | binary`（Phase 1 默认 json）
13. 连接配置从 `otus.reporters.kafka`（继承 `otus.kafka`）读取
14. 单元测试：动态路由、Headers 序列化、serialization 切换

**交付物**: CLI 功能完整覆盖日常运维操作；全局配置/Kafka 命令/Kafka Reporter 对齐设计文档

---

### Step 15: Task 持久化与历史清理
**前置**: Step 8（TaskManager）, Step 4（GlobalConfig）  
**目标**: 守护进程重启后自动恢复运行中的抓包任务；防止任务历史文件无界增长  
**设计依据**: [ADR-030](decisions.md#adr-030-task-持久化每任务独立状态文件), [ADR-031](decisions.md#adr-031-task-历史清理委托-systemd-tmpfilesd)

**任务清单**:

#### A. 全局配置扩展
1. `internal/config/config.go` — 新增 `DataDir` 和 `TaskPersistence` 字段
   ```yaml
   otus:
     data_dir: /var/lib/otus
     task_persistence:
       enabled: true
       auto_restart: true
       gc_interval: 1h
       max_task_history: 100
   ```
2. 单元测试：默认值、`enabled=false` 禁用路径

#### B. FileTaskStore
3. `internal/task/store.go` — `TaskStore` 接口 + `FileTaskStore` 实现
   - 接口：`Save(task PersistedTask)`, `Load(id string)`, `Delete(id string)`, `List()`
   - `PersistedTask` 结构体（version, config, state, timestamps, restart_count）
   - temp-file + atomic rename 写入（`{id}.json.tmp` → `{id}.json`）
   - JSON 序列化/反序列化
4. `internal/task/store_test.go` — 单元测试
   - Save/Load/Delete 基本操作
   - 并发 Save（多 goroutine 同时写）
   - 损坏文件被跳过（不影响其他任务加载）
   - 原子写入（模拟写入中断）

#### C. TaskManager 集成
5. `internal/task/manager.go` — 集成 `TaskStore`
   - `NewTaskManager(agentID string, store TaskStore)` 接受可选 store（nil = 禁用持久化）
   - `Create()` 成功后调用 `store.Save()`
   - `Delete()` 完成后更新 store（state=stopped）
   - Task 进入 Failed 状态时更新 store
   - Graceful shutdown 时更新所有 running task 的 state=stopped
6. `internal/task/manager.go` — 新增 `Restore()` 方法
   - 扫描 store.List()，按 ADR-030 恢复策略分类处理：
     - running/starting/stopping → 调用 Create()，成功则 restart_count++
     - stopped/failed/created  → 加载为只读历史（不占用 Phase 1 slot）
   - 恢复失败单个任务记 ERROR，不中断整体恢复
7. 更新 `internal/task/manager_test.go` — 适配新构造函数，mock store

#### D. Daemon 集成
8. `internal/daemon/daemon.go` — Start() 步骤 4.5
   - 当 `task_persistence.enabled=true` 时创建 `FileTaskStore(data_dir/tasks/)`
   - 调用 `manager.Restore()` 后再启动 UDS Server 和 Kafka Consumer
   - 启动后台 GC goroutine（`task_persistence.gc_interval`，`max_task_history`）

#### E. systemd-tmpfiles.d 集成
9. `configs/tmpfiles.d/otus.conf` — 新增文件
   ```
   D /var/lib/otus             0750 root root -
   d /var/lib/otus/tasks       0750 root root -
   e /var/lib/otus/tasks       -    -    -    7d
   ```
10. 更新 `configs/otus.service` — 新增 `ExecStartPre`
    ```ini
    ExecStartPre=systemd-tmpfiles --create /etc/tmpfiles.d/otus.conf
    ```

**交付物**: 守护进程重启后自动恢复 running 任务；任务历史文件由 systemd-tmpfiles.d + 程序内 GC 双重保障清理；`task_list` / `task_status` 可查询历史记录

---

### Step 16: Daemon 组装 + Graceful Shutdown
**前置**: Step 5, 7, 8, 12, 13  
**目标**: 组装完整 daemon

**任务清单**:
1. `internal/daemon/daemon.go` — daemon 主逻辑
   - 重构：将当前 `cmd/daemon.go` 中的组装逻辑迁移到此文件
   - 加载配置 → 初始化日志 → 启动指标 → 启动 UDS Server → 启动 Kafka 命令订阅 → 等待信号
   - Graceful shutdown 顺序：停止 Kafka consumer → 停止所有 Task → Flush reporters → 关闭 UDS → 关闭日志
   - PID file 管理
2. `internal/command/handler.go` — 新增 `daemon_shutdown` 命令
   - 触发优雅停止流程（通过 channel 或 context 取消传播给 daemon）
   - `otus stop` 通过 UDS 发送 `daemon_shutdown` 命令而非直接 kill
3. 集成测试：启动 → 创建 Task → 停止

**交付物**: `otus daemon` 可完整运行

---

### Step 17: Prometheus 指标
**前置**: Step 16  
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

### Step 18: systemd 集成 + 部署
**前置**: Step 16  
**目标**: 生产就绪的部署配置

**任务清单**:
1. 更新 `configs/otus.service` — systemd unit file
2. 更新 `Makefile` — build / install / clean targets
3. 编写 `scripts/build.sh` — 多架构交叉编译（amd64 / arm64）
4. 更新 `README.md` — 安装和使用说明
5. 编写 Dockerfile（可选）

**交付物**: 可部署运行的完整系统

---

## 9. 旧代码参考索引

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

## 10. 验收标准（Phase 1 完成时）

- [ ] `otus daemon` 可以前台启动，加载配置，监听 UDS
- [ ] `otus task create -f task.json` 可以通过 UDS 创建 SIP 抓包任务
- [ ] AF_PACKET 捕获 → L2-L4 解码 → SIP 解析 → Kafka 上报 全链路跑通
- [ ] Kafka 命令 topic 可以远程创建/删除 Task
- [ ] 交互式命令（`task_list`, `task_status`, `daemon_status`, `daemon_stats`）执行结果可从 `otus-responses` topic 消费（ADR-029）
- [ ] IP 分片重组正常工作，有硬上限保护
- [ ] 日志输出到文件（lumberjack 滚动）和 Loki
- [ ] Prometheus `/metrics` 端点返回各层指标
- [ ] `otus stop` 可以 graceful shutdown
- [ ] 性能：单核 ≥200K pps (SIP 完整解析)
- [ ] 静态编译二进制，支持 amd64 / arm64
- [ ] systemd service 可正常运行

---

**文档版本**: v0.3.0  
**更新日期**: 2026-02-21  
**作者**: Otus Team
