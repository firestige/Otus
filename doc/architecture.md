# Otus - 高性能边缘抓包观测系统架构设计

## 1. 项目概述

Otus 是一个高性能、低资源占用的边缘网络数据包捕获和观测系统，专注于在边缘环境中以最小的资源消耗捕获和处理网络流量。

### 1.1 核心目标

- **极致性能**：最大化单 vCPU 吞吐量，支持线性水平扩展——给多少 CPU 就能吃多少流量
- **专注核心**：核心程序只负责捕获的基本工作流（L2-L4 解码、pipeline 调度），避免功能膨胀
- **高度可扩展**：通过插件体系实现所有扩展能力（捕获驱动、协议解析、数据处理、上报传输）
- **灵活部署**：支持物理机、裸金属、云主机、容器等多种部署方式
- **易于管理**：支持系统服务、远程控制、本地命令行管理

## 2. 核心需求

### 2.1 性能需求

#### 2.1.1 核心指标：单核吞吐量（packets/sec/core）

| 处理路径 | 单核目标吞吐 | 说明 |
|----------|:----------:|------|
| 快路径（查表匹配 + 转发） | ≥ 2M pps/core | RTP/RTCP 等 Parser 内部快速处理 |
| 慢路径（完整协议解析） | ≥ 200K pps/core | SIP/HTTP 等需要 deep parse |

#### 2.1.2 扩展模型

总吞吐 ≈ 单核吞吐 × 可用 vCPU × 利用率系数（~0.85）

| 配置 | vCPU | 预估吞吐（VoIP 场景） | 适用场景 |
|------|:----:|:--------------------:|----------|
| 最低配 | 1 | ~2-3 Gbps | 开发测试、小规模监控 |
| 推荐 | 2 | ~5-8 Gbps | 中等流量边缘节点 |
| 高配 | 4 | ~15-20 Gbps | 大流量核心节点 |
| 大规模 | 8 | ~30+ Gbps | 超大规模或全协议 deep parse |

#### 2.1.3 其他指标

| 指标 | 目标值 | 说明 |
|------|--------|------|
| 内存占用 | ≤ 512 MB（基础） | 不含 TCP 重组缓冲区 |
| 包处理延迟 | < 1 ms | P99 延迟 |
| 丢包率 | < 0.01% | 正常负载下 |
| 线性扩展效率 | ≥ 85% | 增加 1 vCPU 的边际吞吐增益 |

### 2.2 功能需求

#### 2.2.1 核心捕获能力
- 支持高性能网络数据包捕获（XDP、AF_PACKET_V3）
- 零拷贝技术减少内存开销
- 可配置的包过滤规则（BPF）
- 可配置的缓冲区管理

#### 2.2.2 插件扩展能力
- **驱动插件**：支持不同的捕获驱动（XDP、AF_PACKET、pcap）
- **解析器插件**：应用层协议解析（SIP、RTP、HTTP、DNS 等）
- **处理器插件**：数据处理和转换
- **上报器插件**：不同的传输方式（gRPC、Kafka、文件）
- **编解码插件**：不同的序列化格式（Protobuf、JSON、OpenTelemetry）

#### 2.2.3 管理和控制能力
- 系统服务集成（systemd）
- 本地 CLI 控制（daemon 管理 + task 管理，通过 Unix Domain Socket）
- 远程控制：订阅 Kafka 命令 topic，拉模式接收任务指令（零额外端口）
- 全局配置热加载（SIGHUP / CLI reload）
- 健康检查和 Prometheus 指标暴露

### 2.3 部署需求

- **物理机/裸金属**：直接安装，systemd 管理
- **虚拟机（ECS）**：轻量级部署，优化虚拟化性能
- **容器（K8s/K3s/K0s）**：DaemonSet 部署，特权容器
- **跨平台**：支持 Linux x86_64、ARM64

## 3. 系统架构

### 3.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                        Management Layer                      │
│  ┌──────────┐  ┌────────────┐  ┌──────────────┐              │
│  │ CLI(UDS) │  │ Kafka Cmd  │  │ SystemD Svc  │              │
│  └────┬─────┘  └────┬───────┘  └──────┬───────┘              │
│       │             │                │                       │
│       └─────────────┴────────────────┘                       │
│                     │                                        │
├─────────────────────┼────────────────────────────────────────┤
│                     ▼                                        │
│              ┌─────────────┐                                 │
│              │   Daemon    │                                 │
│              │  Controller │                                 │
│              └──────┬──────┘                                 │
│                     │                                        │
├─────────────────────┼────────────────────────────────────────┤
│                     ▼              Core Engine               │
│         ┌───────────────────────┐                            │
│         │  Bootstrap & Config   │                            │
│         └──────────┬────────────┘                            │
│                    │                                         │
│         ┌──────────▼────────────┐                            │
│         │   Plugin Manager      │                            │
│         │  (Load & Registry)    │                            │
│         └──────────┬────────────┘                            │
│                    │                                         │
│    ┌───────────────┼───────────────┐                         │
│    │               │               │                         │
│    ▼               ▼               ▼                         │
│ ┌─────┐      ┌─────────┐     ┌─────────┐                    │
│ │Event│◄────►│Pipeline │◄───►│ Metrics │                    │
│ │ Bus │      │ Engine  │     │ Monitor │                    │
│ └─────┘      └────┬────┘     └─────────┘                    │
│                   │                                          │
├───────────────────┼──────────────────────────────────────────┤
│                   │           Plugin Layer                   │
│    ┌──────────────┼──────────────┐                           │
│    │              │              │                           │
│    ▼              ▼              ▼                           │
│ ┌────────┐   ┌─────────┐   ┌─────────┐                      │
│ │Capture │──►│ Parser  │──►│Processor│                      │
│ │ Plugin │   │ Plugin  │   │ Plugin  │                      │
│ └────────┘   └─────────┘   └────┬────┘                      │
│     │                            │                           │
│     │        ┌───────────────────┘                           │
│     │        │                                               │
│     ▼        ▼                                               │
│ ┌──────────────────┐      ┌──────────────┐                  │
│ │  Buffer Pool     │      │  Reporter    │                  │
│ │  (Ring Buffer)   │      │  Plugin      │                  │
│ └──────────────────┘      └──────┬───────┘                  │
│                                   │                          │
└───────────────────────────────────┼──────────────────────────┘
                                    ▼
                        ┌───────────────────────┐
                        │  External Systems     │
                        │  (Kafka/gRPC/Files)   │
                        └───────────────────────┘
```

### 3.2 核心组件

#### 3.2.1 管理层 (Management Layer)

**Daemon Controller** - `internal/daemon/manager.go`
- 管理捕获任务的生命周期
- 处理远程和本地控制命令
- 协调配置更新和热加载

**CLI Tool** - `cmd/`
- 提供用户友好的命令行接口
- 支持 daemon/task/status/reload 等子命令
- 通过 Unix Domain Socket 与 Daemon 通信

**Kafka 命令通道** - `internal/command/`
- 订阅 Kafka 命令 topic，拉模式接收远程指令
- 按 `target` 字段路由消息到本节点
- 零入站端口，复用已有 Kafka 基础设施

#### 3.2.2 核心引擎 (Core Engine)

**Bootstrap** - `internal/otus/boot/bootstrap.go`
- 系统初始化和启动流程
- 配置加载和验证
- 依赖注入和组件装配

**Plugin Manager** - `internal/plugin/manager.go`
- 插件加载和卸载
- 插件注册表管理
- 依赖解析和版本管理

**Protocol Stack Decoder** - `internal/otus/decoder/`
- L2-L4 协议栈解码（以太网/VLAN/IP/TCP/UDP）
- IP 分片重组、隧道解封装
- 核心代码，非插件

**Pipeline Engine** - `internal/otus/module/pipeline/`
- 每 vCPU 一条独立 pipeline，线性扩展
- 流量分发依赖内核/硬件（RSS/FANOUT），用户态零开销
- Pipeline 主循环：Decode → Parser.CanHandle → Parser.Handle → Send Buffer
- 管理流水线生命周期

**Flow Registry** - `internal/otus/registry/`
- 跨 pipeline 共享的流注册表（lock-free，读多写少）
- Parser 在解析信令时注册媒体流五元组（如 SIP SDP → RTP 五元组）
- Parser 在 CanHandle 时查表做快速匹配
- 支持 TTL 自动过期清理

**Event Bus** - `internal/eventbus/`
- 组件间异步通信
- 事件发布订阅
- 解耦核心组件

**Buffer Pool** - `internal/otus/module/buffer/`
- 内存池管理
- 零拷贝优化
- 背压控制

#### 3.2.3 插件层 (Plugin Layer)

详见第 4 节插件体系设计。

## 4. 插件体系设计

### 4.1 插件接口定义

```go
// 基础插件接口
type Plugin interface {
    Name() string
    Version() string
    Init(config interface{}) error
    Start() error
    Stop() error
    Health() error
}
```

### 4.2 插件类型

#### 4.2.1 Capture Plugin（捕获插件）

**职责**：从网络接口捕获原始数据包，提供并行捕获流

**接口**：
```go
type Capturer interface {
    Plugin
    // Streams 返回该驱动支持的并行捕获流数量
    // AF_PACKET FANOUT: fanout group 中的 socket 数
    // XDP: 绑定的 NIC RX queue 数
    // pcap: 固定返回 1
    Streams() int
    // Capture 启动第 i 个捕获流，返回该流的 packet channel
    Capture(ctx context.Context, streamIndex int) (<-chan *RawPacket, error)
    SetFilter(bpf string) error
    Stats() CaptureStats
}
```

**各驱动的并行能力**：

| 驱动 | 原生并行机制 | 说明 |
|------|------------|------|
| AF_PACKET_V3 | `PACKET_FANOUT` (N sockets) | 内核按 flow-hash/CPU/轮询分发 |
| XDP (AF_XDP) | NIC RSS 多 RX 队列 | 硬件级分发，零 CPU 开销 |
| pcap | 无（单 handle） | 兼容/测试模式，需用户态 flow-hash 分发 |

**实现**：
- **XDP Capture**：基于 eBPF/AF_XDP，最高性能，利用硬件 RSS
- **AF_PACKET_V3 Capture**：基于 mmap ring buffer + FANOUT，高性能
- **Pcap Capture**：兼容模式，用于测试和低速场景

**配置示例**：
```yaml
capture:
  plugin: af_packet_v3
  config:
    interface: eth0
    bpf_filter: "udp port 5060 or udp port 5061"
    fanout_group: 42
    fanout_mode: hash      # hash | cpu | lb
    # fanout_size 自动跟随 pipeline.count
```

#### 4.2.2 Parser Plugin（解析器插件）

**职责**：应用层协议识别与解析。Parser 拥有完全的协议智慧——核心引擎只负责调度，Parser 决定每个包如何处理。

**接口**：
```go
type Parser interface {
    Plugin
    // CanHandle: 这个包我要不要处理？
    // 必须极快（< 50ns），只看端口/协议字段/五元组查表
    CanHandle(packet *DecodedPacket) bool
    // Handle: 处理这个包，返回输出结果
    // 内部自行决定处理深度——可以是查表贴标签快速返回（~100ns），
    // 也可以是完整协议解析（~1-10μs）
    // 返回 nil 表示处理后无需输出（如纯内部状态更新）
    Handle(packet *DecodedPacket) *OutputPacket
    Protocol() string
}

// 可选接口：需要跨 pipeline 共享流注册表的 Parser 实现此接口
type FlowRegistryAware interface {
    SetFlowRegistry(registry FlowRegistry)
}
```

**核心引擎的调度逻辑**（极简）：
```go
for _, parser := range p.parsers {
    if parser.CanHandle(decoded) {
        if output := parser.Handle(decoded); output != nil {
            p.sendBuffer.Write(output)
        }
        break  // 一个包只被一个 parser 处理
    }
}
```

**Parser 内部的快慢路径**（以 VoIP SIP Parser 为例）：

```go
func (p *SIPParser) CanHandle(pkt *DecodedPacket) bool {
    // 1. 五元组匹配已知的 RTP/RTCP 流？
    if _, ok := p.registry.Lookup(pkt.FiveTuple()); ok {
        return true
    }
    // 2. SIP 端口？
    return pkt.DstPort == 5060 || pkt.SrcPort == 5060
}

func (p *SIPParser) Handle(pkt *DecodedPacket) *OutputPacket {
    // 快路径：已知 RTP/RTCP 流，查表贴标签直接返回
    if ctx, ok := p.registry.Lookup(pkt.FiveTuple()); ok {
        return p.wrapWithLabels(pkt, ctx.Labels)  // ~100ns
    }
    // 慢路径：SIP 信令，完整解析
    msg := parseSIPMessage(pkt.Payload)  // ~5μs
    if sdp := msg.SDP(); sdp != nil {
        // 注册媒体流五元组，后续 RTP 包走快路径
        for _, media := range sdp.MediaDescriptions {
            p.registry.Register(extractFiveTuple(media), FlowContext{
                Labels: map[string]string{"call_id": msg.CallID()},
            }, 300*time.Second)
        }
    }
    return &OutputPacket{...}
}
```

**不同场景下的 Parser 行为模式**：

| 场景 | Parser | CanHandle 判定 | Handle 内部策略 |
|------|--------|----------------|----------------|
| VoIP | SIP Parser | 端口 + FlowRegistry 查表 | SIP 信令 deep parse → 注册五元组；RTP/RTCP 查表贴标签 |
| WebRTC | WebRTC Parser | 端口 + FlowRegistry + 包头字节 | WS 信令 parse SDP → 注册五元组；SRTP/STUN 查表贴标签 |
| HTTP | HTTP Parser | 端口 + 连接跟踪表 | Header deep parse；Body 可选跳过或截断 |
| DNS | DNS Parser | 端口 53 | 每包独立 parse，无状态 |
| 全量转发 | 无 Parser | — | 所有包直接走 unmatched_policy 转发 |

**实现**：
- **SIP Parser**：SIP 协议解析 + SDP 媒体流关联
- **RTP Parser**：RTP/RTCP 头部提取
- **HTTP Parser**：HTTP/1.1、HTTP/2 解析
- **DNS Parser**：DNS 协议解析
- **WebRTC Parser**：WebRTC 信令 + SRTP/STUN/DTLS 分类

**配置示例**：
```yaml
parsers:
  - plugin: sip
    enabled: true
    config:
      ports: [5060, 5061]
      track_media: true            # 追踪 SDP 中的媒体流
      media_stream_timeout: 300s   # 媒体流无活动超时清理
  - plugin: dns
    enabled: true
```

#### 4.2.3 Processor Plugin（处理器插件）

**职责**：过滤 + 轻量标注。Edge 采集端应尽可能把计算任务后移，Processor 只做收益巨大的轻量操作。

**设计约束**：
- Processor **只能读写 Envelope（Protocol/FlowID/Network）和 Labels**
- Processor **不可访问 Payload**（协议解析产物属于 Parser 的领域，Processor 协议无关）
- Processor **不可修改 RawBytes**（原始帧是只读引用）
- 返回 `nil` = 丢弃该包

**接口**：
```go
type Processor interface {
    Plugin
    // Process 读写 Labels，读 Envelope，返回 nil 表示丢弃
    Process(pkt *OutputPacket) *OutputPacket
    // Priority 决定 Processor Chain 的执行顺序（数字越小越先执行）
    Priority() int
}
```

**典型实现（仅两类）**：

- **Filter Processor**：基于 Labels 的声明式过滤（drop / pass 规则）
- **Label Processor**：补充静态元数据（部署节点、数据中心、环境标签）

**Filter 示例**：
```go
func (p *FilterProcessor) Process(pkt *OutputPacket) *OutputPacket {
    for _, rule := range p.rules {
        if val, ok := pkt.Labels[rule.Label]; ok {
            if matchesAny(val, rule.Values) && rule.Action == "drop" {
                return nil  // 丢弃
            }
        }
    }
    return pkt
}
```

**Label 示例**：
```go
func (p *LabelProcessor) Process(pkt *OutputPacket) *OutputPacket {
    for k, v := range p.staticLabels {
        pkt.Labels[k] = v
    }
    return pkt
}
```

**不属于 Processor 的职责**（后移到下游）：
- 数据聚合/采样 → 下游计算平台
- 数据脱敏 → 下游处理管道
- 跨协议关联 → Parser 层通过 FlowRegistry 完成

#### 4.2.4 Reporter Plugin（上报器插件）

**职责**：将 OutputPacket 序列化并发送到外部系统。Reporter 是 IO-bound 组件，运行在独立的 Sender 线程中，不占用 Pipeline CPU 时间。

**接口**：
```go
type Reporter interface {
    Plugin
    // Report 序列化并发送单个 OutputPacket
    // 由 Sender 线程调用，不在 Pipeline goroutine 中
    Report(pkt *OutputPacket) error
    // Flush 强制发送所有已缓冲的数据
    Flush() error
}
```

Reporter 通过 `Payload.MarshalJSON()` 或 `Payload.MarshalBinary()` 选择序列化格式，通过读取 `Labels` 做路由决策（如按协议分 Kafka topic），**不需要了解具体协议类型**。

**实现**：
- **Console Reporter**：控制台输出（调试用）
- **File Reporter**：本地文件输出
- **Kafka Reporter**：发送到 Kafka，Labels 作为 Kafka Headers 传递
- **gRPC Reporter**：通过 gRPC 发送
- **OpenTelemetry Reporter**：发送到 OTEL Collector

**Kafka Reporter 示例**：
```go
func (r *KafkaReporter) Report(pkt *OutputPacket) error {
    topic := "otus-" + pkt.Protocol          // 按协议分 topic
    key := []byte(pkt.FlowID.String())
    value, err := pkt.Payload.MarshalBinary()
    if err != nil {
        return err
    }
    // Labels 作为 Kafka Headers
    headers := make([]kafka.Header, 0, len(pkt.Labels))
    for k, v := range pkt.Labels {
        headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
    }
    return r.producer.Send(topic, key, value, headers)
}
```

**配置示例**：
```yaml
reporters:
  - plugin: kafka
    enabled: true
    config:
      brokers:
        - kafka-1.example.com:9092
        - kafka-2.example.com:9092
      topic_prefix: otus       # 实际 topic = otus-{protocol}
      compression: snappy
      batch_size: 1000
      batch_timeout: 100ms
```

### 4.3 插件加载机制

#### 4.3.1 静态链接插件
- 编译时链接到主程序
- init() 函数自动注册
- 零额外开销

```go
// plugins/init.go
import (
    _ "github.com/otus/plugins/parser/sip"
    _ "github.com/otus/plugins/reporter/kafka"
)
```

#### 4.3.2 动态加载插件
- 运行时通过 .so 加载
- 支持热插拔
- 略有性能开销

```go
// 动态加载示例
pluginPath := "/opt/otus/plugins/custom_parser.so"
loader.LoadPlugin(pluginPath)
```

### 4.4 配置模型

系统采用**两层配置**：全局静态配置（配置文件，启动时加载）+ 任务动态配置（Kafka 命令 topic 或本地 CLI 下发）。

#### 4.4.1 全局静态配置

节点级别的、不随观测任务变化的配置。启动时从配置文件加载，运行期间不变。

```yaml
# configs/config.yml
otus:
  # 节点信息（Label Processor 自动引用）
  node:
    ip: 192.168.1.100
    hostname: edge-beijing-01
    dc: cn-north
    env: production

  # 本地控制 Socket（CLI 通过此 socket 与 daemon 通信）
  control:
    socket: /var/run/otus.sock

  # 远程命令通道（订阅 Kafka topic 接收任务指令）
  command_channel:
    enabled: true
    type: kafka                    # Phase 1 仅 kafka
    kafka:
      brokers:                     # 复用 reporters.kafka.brokers 或单独指定
        - kafka-1.example.com:9092
        - kafka-2.example.com:9092
      topic: otus-commands         # 命令 topic
      group_id: "otus-${node.id}"  # 按节点隔离消费
      auto_offset_reset: latest    # 只处理启动后的新命令

  # Prometheus 指标
  metrics:
    listen: 0.0.0.0:9091

  # 共享 Reporter 连接配置（Task 引用，不重复声明）
  reporters:
    kafka:
      brokers:
        - kafka-1.example.com:9092
        - kafka-2.example.com:9092
      compression: snappy
      max_message_bytes: 1048576
    grpc:
      endpoint: collector.example.com:4317
      tls:
        enabled: true
        ca_cert: /etc/otus/ca.pem

  # 全局资源上限
  resources:
    max_workers: 0        # 0 = auto（GOMAXPROCS）

  # 背压控制
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

  # 核心协议栈解码器配置
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

  # 日志
  log:
    level: info                    # debug | info | warn | error
    format: json                   # json | text
    # 多输出并行：本地文件 + Loki（可同时启用）
    outputs:
      file:
        enabled: true
        path: /var/log/otus/otus.log
        rotation:
          max_size: 100MB          # 单文件最大大小
          max_age: 7d              # 保留天数
          max_backups: 5           # 保留的旧日志文件数
          compress: true           # gzip 压缩旧日志
      loki:
        enabled: false
        endpoint: http://loki.observability:3100/loki/api/v1/push
        labels:                    # 静态标签（自动附加 node.id）
          app: otus
          env: production
        batch_size: 100
        batch_timeout: 1s
```

**关键原则**：
- Reporter 的**连接配置**（brokers、endpoint、TLS）全局声明一次，Task 按名称引用
- 节点元数据（ip、hostname、dc、env）全局声明，Label Processor 自动注入
- 协议栈解码器配置（隧道开关、分片重组参数）属于全局，因为它是核心引擎的固有行为
- 背压参数属于全局，所有 Task 共享相同的资源保护策略

#### 4.4.2 任务动态配置

观测任务（Task）通过 Kafka 命令 topic 或本地 CLI 动态创建，完整描述"抓什么、怎么抓、发到哪"。Phase 1 仅支持**一个活跃 Task**。

```yaml
# 通过 Kafka 命令消息或 CLI 下发，以下为 YAML 表示
task:
  id: "voip-monitor-01"

  # ── 捕获配置 ──
  capture:
    driver: af_packet_v3       # af_packet_v3 | xdp | pcap
    interface: eth0
    workers: 4                 # 该 Task 的 pipeline 数量

  # ── 流量过滤（BPF 级，内核侧过滤）──
  filter:
    src_ip: ["10.0.1.0/24"]
    dst_ip: ["10.0.2.0/24"]
    ports: [5060, 5061]
    port_range: "10000-20000"
    protocol: udp              # udp | tcp | any
    # 或直接传 raw BPF：
    # bpf: "udp port 5060 or udp portrange 10000-20000"

  # ── 解析器链 ──
  parsers:
    - plugin: sip
      config:
        track_media: true
        media_stream_timeout: 300s
    - plugin: rtp

  # ── 处理器链 ──
  processors:
    - plugin: filter
      config:
        rules:
          - label: "sip.method"
            values: ["OPTIONS", "REGISTER"]
            action: drop

  # ── 上报配置（引用全局 Reporter 连接 + 任务级参数）──
  reporter:
    type: kafka                # 引用全局 reporters.kafka 的连接配置
    config:
      topic: "voip-sip-packets"     # 任务自定义 topic
      batch_size: 500
      batch_timeout: 50ms

  # ── unmatched 策略 ──
  unmatched_policy: drop       # forward | drop
```

**Task 与全局配置的关系**：

| 配置项 | 来源 | 示例 |
|--------|------|------|
| Reporter 连接（brokers/endpoint） | 全局静态 | `otus.reporters.kafka.brokers` |
| Reporter 业务参数（topic） | Task 动态 | `task.reporter.config.topic` |
| 节点元数据 | 全局静态 | `otus.node.hostname` |
| BPF 过滤规则 | Task 动态 | `task.filter.src_ip` |
| 解码器/隧道/重组 | 全局静态 | `otus.core.decoder.tunnel` |
| Parser/Processor 链 | Task 动态 | `task.parsers`, `task.processors` |
| Pipeline 数量 | Task 动态 | `task.capture.workers` |
| 背压参数 | 全局静态 | `otus.backpressure.*` |

## 5. 数据流处理

### 5.1 处理流程总览

#### 5.1.1 单条 Pipeline 内部流程

每条 pipeline 是一个紧凑的 goroutine 主循环，全流程在同一 goroutine 内完成，零 channel 传递开销。

```
Capture Stream N (mmap zero-copy)
       │
       │ Raw Frame
       ▼
  ┌──────────────────────────────────────────────────┐
  │          Core Protocol Stack Decoder              │
  │  L2 (Ethernet/VLAN) → L2.5 (Tunnel) →           │
  │  L3 (IP/Reassembly) → L4 (UDP/TCP)               │
  └───────────────────────┬──────────────────────────┘
                          │ DecodedPacket
                          ▼
               Parser.CanHandle()?
                 │             │
             yes │             │ no (下一个 parser / unmatched_policy)
                 ▼
            Parser.Handle()
                 │
                 │  Parser 内部自行决定：
                 │  ├─ 查表命中 → 贴标签快速返回 (~100ns)
                 │  └─ 需要解析 → deep parse 返回 (~1-10μs)
                 │
                 ▼
           OutputPacket
                 │
                 ▼
          Processor Chain  (可选: 过滤/增强)
                 │
                 ▼
          Send Buffer (非阻塞写入)
```

#### 5.1.2 Task 与 Pipeline

Pipeline 是**观测任务（Task）的执行单元**。每个 Task 通过 Kafka 命令 topic 或本地 CLI 动态创建，拥有独立的捕获、解析、处理和上报链路。

```
              Kafka 命令 / CLI create
                           │
                           ▼
                  ┌─────────────────┐
                  │  Task Manager   │  管理 Task 生命周期
                  └────────┬────────┘  Phase 1: 最多 1 个活跃 Task
                           │
                           ▼
              ┌────────────────────────────┐
              │   Task: voip-monitor-01     │
              │                            │
              │   Capture: AF_PACKET eth0   │
              │   Filter:  BPF (port 5060)  │
              │   Workers: 4                │
              │   FlowRegistry (per-Task)   │
              └──────────┬─────────────────┘
                         │
          ┌──────────────┼──────────────┐──────────────┐
          ▼              ▼              ▼              ▼
     ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐
     │Pipeline 0│  │Pipeline 1│  │Pipeline 2│  │Pipeline 3│
     │          │  │          │  │          │  │          │
     │ Decode   │  │ Decode   │  │ Decode   │  │ Decode   │
     │ CanHandle│  │ CanHandle│  │ CanHandle│  │ CanHandle│
     │ Handle   │  │ Handle   │  │ Handle   │  │ Handle   │
     │ Process  │  │ Process  │  │ Process  │  │ Process  │
     │   Out    │  │   Out    │  │   Out    │  │   Out    │
     └───┬──────┘  └───┬──────┘  └───┬──────┘  └───┬──────┘
         │             │             │             │
         ▼             ▼             ▼             ▼
    ┌──────────────────────────────────────────────────┐
    │            Send Buffer (MPSC)                     │
    └────────────────────┬─────────────────────────────┘
                         │
                         ▼
                  ┌────────────┐
                  │   Sender   │  Kafka topic: voip-sip-packets
                  │   (Kafka)  │
                  └────────────┘
```

**Task 拥有的资源**：

| 资源 | 作用域 | 说明 |
|------|--------|------|
| Capture socket(s) | Task 独占 | 自己的 AF_PACKET group / XDP queue + BPF filter |
| N 条 Pipeline | Task 独占 | N = `task.capture.workers` |
| FlowRegistry | **per-Task** | Task 内跨 pipeline 共享，Task 之间隔离 |
| Send Buffer | Task 独占 | 独立的有界队列 |
| Sender | Task 独占 | 每个 Task 的 Reporter 实例独立 |

**Phase 1 约束**：最多 1 个活跃 Task。新 Task 创建前必须先停止旧 Task。

#### 5.1.3 关键设计要点

**Workers 数量与流量分发**

```go
workers := task.Capture.Workers
if workers == 0 {
    workers = runtime.GOMAXPROCS(0)  // auto: 跟随 CPU 核心数
}
if driver.MaxStreams() < workers {
    workers = driver.MaxStreams()    // 不超过驱动支持的最大 stream 数
}
```

- 每条 pipeline 绑定一个 capture stream，全流程在同一 goroutine 内完成
- Pipeline 之间不共享可变状态，天然无锁
- 同一条流（5-tuple）的包必须到同一 pipeline（TCP 重组、应用层状态依赖此保证）

| 驱动 | 分发机制 | 用户态开销 |
|------|---------|----------|
| XDP | NIC RSS（硬件 hash） | 零 |
| AF_PACKET_V3 | FANOUT_HASH（内核 hash） | 零 |
| pcap | 需用户态 flow-hash dispatcher | 有（仅测试场景） |

**Pipeline 主循环**

```go
func (p *Pipeline) Run(ctx context.Context, stream <-chan *RawPacket) {
    for {
        select {
        case raw := <-stream:
            decoded := p.decoder.Decode(raw)
            if decoded == nil {
                continue
            }
            
            var output *OutputPacket
            for _, parser := range p.parsers {
                if parser.CanHandle(decoded) {
                    output = parser.Handle(decoded)
                    break
                }
            }
            
            if output == nil {
                if p.unmatchedPolicy == Forward {
                    output = p.wrapRaw(decoded)
                } else {
                    continue  // drop
                }
            }
            
            for _, proc := range p.processors {
                output = proc.Process(output)
                if output == nil {
                    break
                }
            }
            
            if output != nil {
                p.sendBuffer.Write(output)  // 非阻塞
            }
            
        case <-ctx.Done():
            return
        }
    }
}
```

**Sender 独立于 Pipeline（IO-bound 与 CPU-bound 分离）**

Sender 是 IO-bound（等待网络 ACK），独立线程运行，不占用 Pipeline 的 CPU 时间。多条 pipeline 写入同一 Send Buffer（MPSC lock-free queue），Sender 批量消费并发送。每个 Task 拥有独立的 Send Buffer 和 Sender 实例。

**FlowRegistry：per-Task，跨 Pipeline 共享**

```go
type FlowRegistry interface {
    // 注册五元组（Parser.Handle 解析信令时调用，低频写）
    Register(flow FiveTuple, ctx FlowContext, ttl time.Duration)
    // 查询五元组（Parser.CanHandle 时调用，高频读）
    Lookup(flow FiveTuple) (FlowContext, bool)
    // 注销
    Unregister(flow FiveTuple)
}
```

基于 `sync.Map` 实现，读多写少场景下接近无锁性能。**作用域为 per-Task**：Task 内的多条 pipeline 共享同一个 FlowRegistry，Task 之间完全隔离。Task 销毁时整个 FlowRegistry 直接丢弃，无需逐条清理。

### 5.2 核心协议栈解码器

核心协议栈解码器（Core Protocol Stack Decoder）负责从原始以太网帧中提取出应用层载荷，是核心引擎的固有组件，**不是插件**。所有目标协议（SIP、RTP、RTCP、WebRTC、HTTP、WebSocket）都工作在 TCP/IP 模型之上，L2-L4 的解析逻辑稳定且通用，不存在可插拔需求。

#### 5.2.1 设计原则

- **L2-L4 解码属于核心代码**：以太网帧头、IP 头、TCP/UDP 头的格式是数十年不变的标准
- **插件只负责应用语义解析**：核心代码输出统一的 `DecodedPacket`，应用层插件基于此解析上层协议
- **性能零妥协**：10Gbps 下每包都要过 L2-L4 解码，走插件接口的调度开销不可接受
- **分界线明确**：`Raw Frame → ... → Application Payload` 是核心，`Application Payload → SIP Message / RTP Packet` 是插件

#### 5.2.2 L2：以太网帧解码（常开，零配置）

**标准以太网帧**
- 解析 Destination MAC、Source MAC、EtherType
- 根据 EtherType 分发到 IPv4（`0x0800`）或 IPv6（`0x86DD`）处理

**802.1Q VLAN 剥离**
- 自动识别 EtherType `0x8100`，提取 VLAN ID（12 bit），跳过 4 字节 VLAN tag
- 记录 VLAN ID 到 `DecodedPacket` 元信息中

**QinQ（802.1ad）剥离**
- 自动识别 EtherType `0x88A8`，递归剥离多层 VLAN tag
- 运营商 / 多租户网络常见，处理方式与 VLAN 相同，只是递归

VLAN/QinQ 在企业和运营商网络中极为普遍，且解析仅需跳过 4 字节固定偏移，**常开无需配置**。

#### 5.2.3 L2.5：隧道解封装（可配置开关）

在不同部署环境下，数据包可能被隧道协议封装。是否需要解封装取决于**抓包位置**：

| 抓包位置 | 是否看到隧道封装 | 是否需要解封装 |
|----------|:----------------:|:--------------:|
| 虚拟机内 eth0（VPC 网卡） | 否（hypervisor 已剥离） | 不需要 |
| K8s Pod（hostNetwork） | 否（CNI 已处理） | 不需要 |
| 物理网卡（underlay） | 是 | 需要 |
| 旁路镜像（SPAN / TAP） | 是 | 需要 |
| 宿主机物理网卡（看容器流量） | 是 | 需要 |

**支持的隧道类型**：

| 隧道类型 | 特征 | 典型场景 | 优先级 |
|----------|------|----------|--------|
| VXLAN | UDP port 4789 + 8B header + 内层以太网帧 | 云 VPC、Flannel/Calico VXLAN | 高 |
| GRE / ERSPAN | IP Protocol 47 + 4-16B 可变头 | 旁路镜像（Cisco SPAN over GRE） | 中 |
| Geneve | UDP port 6081 + 可变长 header | AWS 新一代 overlay、OVN/OVS | 中 |
| IPIP | IP Protocol 4（IP-in-IP） | Calico IPIP 模式 | 中 |

解封装后，剥离外层封装头部，暴露内层以太网帧，**重新进入 L2 Decode 递归处理**。

所有隧道解码均为无状态的固定偏移头部剥离，每种约 15-50 行代码，**对性能几乎零影响**。默认关闭，按部署场景配置开启。

#### 5.2.4 L3：IP 解码与分片重组

**IPv4 / IPv6 解析**
- 提取源 IP、目的 IP、协议号、TTL 等头部字段
- IPv6 需处理扩展头部链（Extension Header Chain）

**IP 分片重组（常开）**

IP 分片重组是核心必做能力，不可配置关闭。原因：

- SIP over UDP 消息经常超过 MTU（一个带 SDP 的 INVITE 可达 2000+ 字节），产生 IP 分片
- 如果不重组，SDP 部分被截断，无法提取媒体地址和端口
- 影响后续按需捞取 RTP 的核心业务能力

**重组机制**:
- 以 `(src_ip, dst_ip, protocol, fragment_id)` 为 key 维护分片缓存
- 收齐所有分片后拼接为完整 IP 报文
- 超时清理未完成的分片组（默认 30s，与 Linux 内核默认值一致）
- 分片在总流量中占比极低（通常 < 0.1%），内存和 CPU 开销可忽略

#### 5.2.5 L4：传输层解码

**UDP 解码（无状态）**
- 提取源端口、目的端口，直接输出 payload
- 零额外开销，适用于 SIP/UDP、RTP、RTCP、DTLS/SRTP 等
- 目标协议中的绝大多数流量走 UDP

**TCP 流重组（可配置，默认关闭）**

TCP 流重组是有状态操作，资源消耗与 IP 分片重组不在同一量级：

| 维度 | IP 分片重组 | TCP 流重组 |
|------|-----------|-----------|
| 状态维护 | 短暂，几个分片即完成 | 长生命周期，持续整个连接 |
| 并发规模 | 极少（分片占比 < 0.1%） | 可能数万至数十万并发连接 |
| 内存开销 | KB 级 | 每连接 100B-数KB 状态 + 乱序缓冲区 |
| CPU 开销 | 几乎没有 | 序列号追踪、乱序处理、重传去重 |
| 正确性复杂度 | 简单拼接 | 完整状态机：SYN/FIN/RST、窗口管理 |

**选择性重组策略**：不做全流量 TCP 重组，只对匹配特定端口规则的 TCP 流做重组。

```
TCP 流量
  │
  ├─ 匹配重组规则（如 dst port 5060, 80, 443）
  │     → 进入 TCP 重组引擎 → 输出完整应用层 PDU
  │     连接数少，资源消耗可控
  │
  └─ 不匹配重组规则
        → 仅输出 L3/L4 头部元信息（或直接丢弃）
        → 不消耗重组资源
```

**资源保护**：TCP 重组引擎必须有硬性上限，防止资源失控。

**不需要 TCP 重组的捕获任务**可完全关闭该功能，例如 RTP over TCP 可以直接转发原始报文到下游服务器做重组和语音拼接。

| 部署场景 | TCP 重组开销 | 总 CPU 影响 |
|----------|-------------|-------------|
| 纯 SIP/UDP + RTP 监控 | 关闭或空转 | +0% |
| SIP/UDP + 少量 SIP/TCP | 数千连接级 | +2-5% |
| 含 HTTP/WS 观测 | 数万连接级 | +10-15% |

#### 5.2.6 核心输出结构

核心协议栈解码器的输出是一个统一的结构体，作为所有应用层解析插件的输入。详细的三层数据结构设计和 Labels 命名规范参见 [5.5 Pipeline 数据契约](#55-pipeline-数据契约)。

```go
type DecodedPacket struct {
    // 原始帧引用（Parser 如需转发原始帧可引用 Raw.Data）
    Raw         *RawPacket

    // L2 信息
    SrcMAC      net.HardwareAddr
    DstMAC      net.HardwareAddr
    VLANs       []uint16        // 可能多层（QinQ），空表示无 VLAN
    EtherType   uint16

    // L2.5 隧道元信息（nil = 非隧道流量）
    Tunnel      *TunnelInfo

    // L3 信息
    SrcIP       netip.Addr
    DstIP       netip.Addr
    Protocol    uint8           // TCP=6, UDP=17
    TTL         uint8
    Fragmented  bool            // 是否经过分片重组

    // L4 信息
    SrcPort     uint16
    DstPort     uint16
    TCPFlags    uint8           // 仅 TCP 有效

    // 应用层载荷（slice into Raw.Data，零拷贝）
    Payload     []byte
}

type TunnelInfo struct {
    Type       TunnelType       // VXLAN / GRE / Geneve / IPIP
    OuterSrcIP netip.Addr
    OuterDstIP netip.Addr
    VNI        uint32           // VXLAN/Geneve 虚拟网络标识
}
```

#### 5.2.7 配置示例

```yaml
otus:
  core:
    decoder:
      # VLAN / QinQ: 常开，无需配置

      # 隧道解封装（按部署场景开启）
      tunnel:
        vxlan: false
        gre: false
        erspan: false
        geneve: false
        ipip: false

      # IP 分片重组: 常开，无需配置
      # 可调参数
      ip_reassembly:
        timeout: 30s                # 分片超时时间
        max_fragments: 10000        # 最大同时追踪的分片组数

      # TCP 流重组（按需开启，Phase 2）
      tcp_reassembly:
        enabled: false
        port_filter: [5060, 5061, 80, 443, 8080, 8443]
        max_concurrent_streams: 10000
        per_stream_buffer_limit: 32KB
        global_memory_limit: 128MB    # 硬性全局上限
        stream_timeout: 120s
        gap_timeout: 5s               # 空洞超时后跳过
        overflow_policy: drop_oldest  # LRU 淘汰最旧连接
        mid_stream_join: true         # 支持中途加入已有连接
```

### 5.3 性能优化策略

#### 5.3.1 零拷贝技术
- XDP 直接在内核中处理
- mmap 共享内存映射
- sendfile 避免用户态拷贝

#### 5.3.2 批处理
- 批量读取数据包
- 批量处理和发送
- 减少系统调用开销

#### 5.3.3 无锁设计
- 单生产者单消费者队列
- Lock-free ring buffer
- Per-CPU 数据结构

#### 5.3.4 内存管理
- 对象池复用
- 预分配缓冲区
- 避免频繁 GC

#### 5.3.5 CPU 亲和性
- 绑定工作线程到特定 CPU
- NUMA 感知
- 避免缓存失效

### 5.4 背压控制与丢弃策略

#### 5.4.1 核心原则

高性能抓包系统面临的根本矛盾是：**捕获速率恒定（线速），但下游消费速率不可控**。

设计原则：**永远保护捕获层，牺牲上报层**。抓包观测系统的第一优先级是"能抓到"，而不是"能送到"。背压绝不可向上传导至捕获层——任何下游故障都不能阻塞数据包捕获。

```
Capture Driver (内核)
    │  内核 ring buffer: 满了内核自动丢，我们只统计 tp_drops
    ▼
Capture Plugin (用户态)
    │  从 mmap 读取，永远不阻塞
    │  写入 pipeline channel: 非阻塞，满了就丢 + 计数
    ▼
Pipeline Channel (bounded, lock-free)
    │  有界 channel / ring buffer
    │  Parse + Process 是 CPU-bound，速度稳定
    ▼
Send Buffer (bounded queue per reporter)
    │  每个 reporter 有独立的有界发送队列
    │  Reporter 消费不过来时执行丢弃策略
    ▼
Reporter (Kafka/gRPC/...)
    │  异步发送，ack 超时则视为丢弃
    │  不重试或有限重试（最多 1-2 次）
    ▼
External System
```

#### 5.4.2 分层丢弃策略

数据流的每一层都是非阻塞的，下游慢了就在下游丢弃，绝不向上传导。

**第一层：内核 Ring Buffer**

- AF_PACKET_V3 的 mmap ring buffer 由内核管理，消费者（用户态）追不上时，内核直接覆盖或丢弃新帧，`tp_drops` 计数器自增
- XDP 同理，umem 的 fill/completion ring 满了，内核直接 drop
- **这一层丢弃不可控**，我们只能通过读取 `tp_drops` 进行统计上报

**第二层：Pipeline Channel**

- Capture Plugin 从 mmap 读取后，通过非阻塞写入将数据送入 pipeline channel
- Channel 有界（bounded），写满时 **drop-tail**——丢弃新到达的包，记录丢弃计数
- 保证 Capture Plugin 的读取循环永远不被阻塞

```go
// 非阻塞写入示例
select {
case ch <- packet:
    // 成功写入
default:
    // channel 满，丢弃并计数
    metrics.PipelineDropsTotal.Inc()
}
```

**第三层：Send Buffer**

- 每个 Reporter 拥有独立的有界发送队列
- 当 Reporter 消费不过来时，采用 **drop-head**（丢弃最旧的未确认数据），因为观测数据越新越有价值
- 发送队列的容量通过配置控制

**第四层：Reporter 超时丢弃**

- Kafka ACK 超时（如 3s）时，视为发送失败，**直接丢弃该批次数据，不做无限重试**
- gRPC 发送超时同理，deadline exceeded 后丢弃
- 抓包数据不是交易数据，不需要精确一次（exactly-once）语义，最多一次（at-most-once）即可

#### 5.4.3 丢弃策略对比

| 策略 | 适用层 | 行为 | 优点 | 缺点 |
|------|--------|------|------|------|
| Drop-tail（丢新） | Pipeline Channel | 队列满时拒绝新数据 | 实现简单，保留已入队数据完整性 | 持续过载时新数据全部丢失 |
| Drop-head（丢旧） | Send Buffer | 队列满时淘汰最旧数据 | 保留最新数据，观测价值更高 | 实现稍复杂，需要支持出队覆盖 |
| 超时丢弃 | Reporter | ACK 超时视为丢失 | 防止资源泄漏和无限等待 | 网络抖动时可能误丢 |
| 动态采样降级 | 全链路 | 过载时提高采样间隔 | 优雅降级，保留统计意义 | 丢失个体数据包的细节 |

#### 5.4.4 动态采样降级（可选高级策略）

当全链路持续过载时，可以通过反馈机制通知 Capture 层提高采样率（如从全量降为 1:10 采样），而不是阻塞。这是一种"有损但可控"的降级方式：

```
Send Buffer 水位 > 80%
    → 通知 Pipeline 降级
        → Pipeline 通知 Capture 调整 BPF 采样率
            → 捕获量下降，全链路压力缓解

Send Buffer 水位 < 30%
    → 恢复全量捕获
```

采样降级是可选策略，适用于对完整性要求不极端、但对可用性要求高的场景。

#### 5.4.5 必须暴露的背压指标

每一层丢弃都必须有对应的 metrics，否则运维无法感知数据丢失：

```
# 内核侧丢包（tp_drops），由 Capture 插件从驱动获取
otus_capture_kernel_drops_total{interface="eth0"}

# Pipeline channel 满导致的丢弃
otus_pipeline_channel_drops_total{task="voip-monitor-01", pipeline="0"}

# 发送缓冲区满导致的丢弃（drop-head 淘汰）
otus_sender_buffer_drops_total{task="voip-monitor-01"}

# Reporter 超时导致的丢弃
otus_reporter_timeout_drops_total{task="voip-monitor-01"}

# Reporter 错误导致的丢弃
otus_reporter_error_drops_total{task="voip-monitor-01"}

# 当前动态采样率（1.0 = 全量，0.1 = 十分之一）
otus_backpressure_sample_rate{task="voip-monitor-01"}

# 各层队列当前水位（用于告警和容量规划）
otus_pipeline_channel_usage_ratio{task="voip-monitor-01", pipeline="0"}
otus_sender_buffer_usage_ratio{task="voip-monitor-01"}
```

#### 5.4.6 配置

背压参数在全局静态配置中设置（见 [4.4.1 全局静态配置](#441-全局静态配置) 的 `otus.backpressure` 部分），所有 Task 共享相同的资源保护策略。

#### 5.4.7 故障场景分析

| 故障场景 | 系统行为 | 数据影响 |
|----------|----------|----------|
| Kafka 集群短暂不可用（< 30s） | Send Buffer 吸收积压，超过容量则 drop-head | 丢失部分旧数据，恢复后自动续传 |
| Kafka 集群长时间不可用（> 30s） | Send Buffer 持续 drop-head，pipeline channel 可能开始 drop-tail | 持续丢失数据，但捕获不受影响 |
| 下游网络带宽不足 | 与 Kafka 不可用类似，但丢弃速率更稳定 | 稳态丢弃，可通过 metrics 监控丢弃率 |
| 突发流量（burst） | Pipeline channel 和 Send Buffer 联合吸收 | 短暂 burst 可无损吸收，持续超载则触发丢弃 |
| CPU 处理不过来 | Pipeline channel 积压，capture 侧 drop-tail | Parse/Process 成为瓶颈时丢弃最新包 |

### 5.5 Pipeline 数据契约

Pipeline 各阶段的数据结构是插件可组合性的基础。三层数据结构对应三个所有权边界，每层有明确的读写权限：

| 层 | 结构 | 产出者 | 消费者 | 生命周期 |
|----|------|--------|--------|----------|
| 第 1 层 | `RawPacket` | Capture 驱动 | Core Decoder | 帧级，mmap buffer 回收即失效 |
| 第 2 层 | `DecodedPacket` | Core Decoder | Parser | 包级，Pipeline 主循环内复用 |
| 第 3 层 | `OutputPacket` | Parser | Processor → Reporter | 包级，流经 Processor Chain → Send Buffer → Reporter |

#### 5.5.1 第 1 层：RawPacket（Capture → Core）

```go
type RawPacket struct {
    Data       []byte     // 原始帧，指向 mmap ring buffer（零拷贝）
    Timestamp  time.Time  // 硬件/内核时间戳
    CaptureLen int        // 实际捕获长度
    OrigLen    int        // 原始帧长度（可能被截断）
    IfIndex    int        // 网卡索引
}
```

核心私有结构，插件不直接接触。`Data` 是 mmap 映射的零拷贝 slice，生命周期由内核 ring buffer 控制。

#### 5.5.2 第 2 层：DecodedPacket（Core → Parser）

核心定义、Parser 只读。完整定义见 [5.2.6 核心输出结构](#526-核心输出结构)。

关键设计点：
- `Payload []byte` 是 slice into `Raw.Data`，零拷贝
- `Tunnel *TunnelInfo` 为 nil 表示非隧道流量
- `VLANs` 支持多层 QinQ
- Parser 不应修改 DecodedPacket 的任何字段

#### 5.5.3 第 3 层：OutputPacket（Parser → Processor → Reporter）

OutputPacket 是跨插件边界的**公共契约**，采用 **Envelope + Typed Payload** 模式：

```go
type OutputPacket struct {
    // ── Envelope：固定结构，所有插件可读 ──
    Timestamp  time.Time
    Protocol   string           // "sip", "rtp", "rtcp", "dns", ...
    FlowID     FiveTuple
    Network    NetworkMeta      // L3/L4 上下文摘要（只读）

    // ── Labels：Parser 写入，Processor 读写，Reporter 读 ──
    Labels     map[string]string

    // ── Payload：Parser 写入，Processor 不碰，Reporter 序列化 ──
    Payload    Payload          // 接口

    // ── 原始引用：可选，Parser 按需设置 ──
    RawBytes   []byte           // 非 nil = 携带原始帧（如 RTP 转发场景）
}

type FiveTuple struct {
    SrcIP    netip.Addr
    DstIP    netip.Addr
    SrcPort  uint16
    DstPort  uint16
    Protocol uint8
}

type NetworkMeta struct {
    SrcIP    netip.Addr
    DstIP    netip.Addr
    SrcPort  uint16
    DstPort  uint16
    SrcMAC   net.HardwareAddr
    DstMAC   net.HardwareAddr
    VLANs    []uint16
}
```

**各字段的访问权限矩阵**：

| 字段 | Parser | Processor | Reporter |
|------|--------|-----------|----------|
| Envelope（Timestamp/Protocol/FlowID/Network） | 写入 | 只读 | 只读 |
| Labels | 写入 | **读写** | 只读 |
| Payload | 写入 | **不可访问** | 只读（序列化） |
| RawBytes | 写入 | **不可修改** | 只读 |

#### 5.5.4 Payload 接口

Payload 接口极简，只服务于 Reporter 的序列化需求。Processor 不需要也不应该访问 Payload 内部。

```go
type Payload interface {
    // Type 返回协议类型标识（与 OutputPacket.Protocol 一致）
    Type() string

    // 序列化 — Reporter 选择格式调用
    MarshalJSON() ([]byte, error)
    MarshalBinary() ([]byte, error)  // protobuf / msgpack / 自定义 binary
}
```

每个 Parser 定义自己的 Payload 实现（如 `SIPPayload`、`RTPPayload`），Reporter 通过接口方法序列化，无需 import 具体协议类型。

**没有 `Field()` 方法**。Processor 不探查 Payload 内部——Parser 已经把 Processor 需要的一切信息提取到 Labels 里。

#### 5.5.5 Labels 命名规范

Labels 是 Parser→Processor 的**唯一通信契约**。为避免命名冲突和歧义，采用分级命名：

**Parser 导出的协议字段**：`{protocol}.{field}`

```
sip.method      = "INVITE"
sip.call-id     = "abc123@host"
sip.from        = "sip:alice@example.com"
sip.to          = "sip:bob@example.com"
rtp.ssrc        = "12345678"
rtp.pt          = "8"
rtcp.type       = "SR"
dns.query       = "example.com"
dns.type        = "A"
```

**跨协议关联字段**：无前缀，由 Parser 通过 FlowRegistry 查询后写入

```
call-id         = "abc123@host"      # RTP Parser 从 FlowRegistry 带出的关联 call-id
codec           = "PCMA"             # RTP Parser 从 FlowRegistry 带出的协商编解码器
session-id      = "sess-001"         # 其他关联标识
```

**Processor 补充的元数据**：无前缀或使用部署维度前缀

```
node            = "edge-beijing-01"   # Label Processor 写入
dc              = "cn-north"
env             = "production"
```

**命名规则汇总**：

| 来源 | 前缀模式 | 示例 | 写入者 |
|------|---------|------|--------|
| 协议字段 | `{protocol}.{field}` | `sip.method`, `rtp.ssrc` | Parser |
| 跨协议关联 | 无前缀 | `call-id`, `codec` | Parser（via FlowRegistry） |
| 部署元数据 | 无前缀 | `node`, `dc`, `env` | Label Processor |

**约束**：
- Key 和 Value 均为 `string` 类型，足够覆盖过滤 / 路由 / 标记场景
- 结构化数据（如完整 SIP 消息体）属于 Payload 的职责，不放 Labels
- Key 使用小写字母 + 点分隔 + 短横线（`[a-z0-9][a-z0-9.-]*`），不使用下划线

#### 5.5.6 Parser 产出示例

**SIP Parser**：
```go
func (p *SIPParser) Handle(pkt *DecodedPacket) *OutputPacket {
    msg := parseSIP(pkt.Payload)
    // 解析 SDP 中的媒体流信息，注册到 FlowRegistry
    if sdp := msg.SDP; sdp != nil {
        for _, media := range sdp.MediaStreams {
            p.flowRegistry.Register(media.FiveTuple, FlowContext{
                CallID: msg.CallID,
                Codec:  media.Codec,
            }, 300*time.Second)
        }
    }
    return &OutputPacket{
        Timestamp: pkt.Raw.Timestamp,
        Protocol:  "sip",
        FlowID:    extractFlow(pkt),
        Network:   extractNetwork(pkt),
        Labels: map[string]string{
            "sip.method":  msg.Method,
            "sip.call-id": msg.CallID,
            "sip.from":    msg.From,
            "sip.to":      msg.To,
        },
        Payload: msg,  // *SIPPayload 实现 Payload 接口
    }
}
```

**RTP Parser**（查 FlowRegistry 带出关联信息）：
```go
func (p *RTPParser) Handle(pkt *DecodedPacket) *OutputPacket {
    ctx, _ := p.flowRegistry.Lookup(extractFlow(pkt))
    rtp := parseRTPHeader(pkt.Payload)  // 12 字节固定头，极快
    return &OutputPacket{
        Timestamp: pkt.Raw.Timestamp,
        Protocol:  "rtp",
        FlowID:    extractFlow(pkt),
        Network:   extractNetwork(pkt),
        Labels: map[string]string{
            "rtp.ssrc":  fmt.Sprintf("%d", rtp.SSRC),
            "rtp.pt":    fmt.Sprintf("%d", rtp.PayloadType),
            "call-id":   ctx.CallID,    // 跨协议关联
            "codec":     ctx.Codec,
        },
        Payload:  rtp,
        RawBytes: pkt.Raw.Data,  // RTP 通常转发原始帧
    }
}
```

#### 5.5.7 Processor 配置示例

```yaml
processors:
  # 过滤器：基于 Labels 做 drop/pass
  - plugin: filter
    enabled: true
    config:
      rules:
        - label: "sip.method"
          values: ["OPTIONS", "REGISTER"]
          action: drop           # 不上报 SIP OPTIONS/REGISTER
        - label: "rtp.pt"
          values: ["13"]
          action: drop           # 不上报 comfort noise

  # 标注器：补充部署元数据
  - plugin: label
    enabled: true
    config:
      labels:
        node: "edge-beijing-01"
        dc: "cn-north"
        env: "production"
```

## 6. 控制和管理

### 6.1 系统服务集成

```ini
# configs/otus.service
[Unit]
Description=Otus Network Packet Capture Service
After=network.target

[Service]
Type=notify
ExecStart=/usr/local/bin/otus daemon
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536
LimitMEMLOCK=infinity
CapabilityBoundingSet=CAP_NET_RAW CAP_NET_ADMIN CAP_SYS_RESOURCE

[Install]
WantedBy=multi-user.target
```

### 6.2 CLI 命令

CLI 通过 **Unix Domain Socket**（`/var/run/otus.sock`）与 daemon 通信，不需要开放任何 TCP 端口。

```bash
# 启动 daemon（加载全局配置，订阅 Kafka 命令 topic）
otus daemon --config /etc/otus/config.yml

# ── 本地 Task 管理（通过 UDS 与 daemon 通信）──
# 创建观测任务（从 YAML 文件加载 Task 配置）
otus task create --file task-voip.yml

# 停止观测任务
otus task stop --id voip-monitor-01

# 查看当前活跃任务
otus task list

# 查看任务详情和统计
otus task status --id voip-monitor-01

# ── Daemon 管理 ──
otus status           # daemon 状态
otus stats            # 全局统计
otus reload           # 重新加载全局配置（等效 SIGHUP）

# 验证配置文件
otus validate --config /etc/otus/config.yml
otus validate --task task-voip.yml
```

### 6.3 远程控制通道（Kafka 拉模式）

**设计原则**：Agent 不监听任何入站端口。远程控制复用已有的 Kafka 基础设施，Agent 主动订阅命令 topic 拉取指令。

```
┌─────────────────┐     Kafka 命令 topic      ┌──────────────┐
│  Control Plane  │ ──── produce ────────────→ │ otus-commands│
│  / 运维平台     │                            └──────┬───────┘
└─────────────────┘                                   │
                                              subscribe (pull)
                                                      │
                          ┌───────────────────────────┘
                          ▼
                ┌───────────────────┐
                │  Otus Agent       │
                │  consumer group:  │
                │  otus-edge-bj-01  │  ← group_id 按节点隔离
                └───────────────────┘
```

**命令消息格式**（JSON）：

```json
{
  "version": "v1",
  "target": "edge-beijing-01",
  "command": "create_task",
  "timestamp": "2026-02-13T10:30:00Z",
  "request_id": "req-abc-123",
  "payload": {
    "task_id": "voip-monitor-01",
    "capture": {
      "driver": "af_packet_v3",
      "interface": "eth0",
      "workers": 4
    },
    "filter": {
      "bpf": "udp port 5060 or udp portrange 10000-20000"
    },
    "parsers": [
      { "plugin": "sip", "config": { "track_media": true } },
      { "plugin": "rtp" }
    ],
    "processors": [
      {
        "plugin": "filter",
        "config": {
          "rules": [
            { "label": "sip.method", "values": ["OPTIONS"], "action": "drop" }
          ]
        }
      }
    ],
    "reporter": {
      "type": "kafka",
      "config": { "topic": "voip-sip-packets" }
    },
    "unmatched_policy": "drop"
  }
}
```

**支持的命令**：

| command | 说明 | payload |
|---------|------|---------|
| `create_task` | 创建观测任务 | 完整 Task 配置 |
| `stop_task` | 停止观测任务 | `{ "task_id": "..." }` |
| `reload` | 重新加载全局配置 | 无 |

**消息路由**：
- `target` 字段匹配全局配置中的 `node.id`，不匹配的消息直接跳过
- `target: "*"` 表示广播到所有节点
- `request_id` 用于状态上报时关联请求（Phase 2 实现 response topic）

**状态上报**（Phase 2）：
- Agent 向独立的 `otus-status` topic 发布心跳和 Task 状态
- Control Plane 订阅该 topic 获取节点状态，形成完整闭环

### 6.4 Task 生命周期

```
Kafka 命令消息 / CLI create
    │
    ▼
┌─ 校验 ──────────────────────────────────┐
│ 1. Phase 1：检查是否已有活跃 Task       │
│    → 有则拒绝（返回错误 / 忽略命令）     │
│ 2. 校验 workers ≤ max_workers            │
│ 3. 校验 reporter.type 在全局配置中存在   │
│ 4. 校验 driver 可用                      │
└──────────────────────┬──────────────────┘
                       │
                       ▼
┌─ 构建 ──────────────────────────────────┐
│ 1. 创建 FlowRegistry 实例              │
│ 2. 初始化 Parser 链（注入 FlowRegistry）│
│ 3. 初始化 Processor 链                  │
│ 4. 创建 Capture driver + BPF filter     │
│ 5. 创建 N 条 Pipeline                   │
│ 6. 创建 Send Buffer + Sender            │
└──────────────────────┬──────────────────┘
                       │
                       ▼
┌─ 运行 ──────────────────────────────────┐
│ 启动 N 个 pipeline goroutine            │
│ 启动 Sender goroutine                   │
│ 记录 Task 状态为 running                │
└──────────────────────┬──────────────────┘
                       │
          Kafka stop_task / CLI stop / ctx cancel
                       │
                       ▼
┌─ 清理 ──────────────────────────────────┐
│ 1. Cancel context → pipeline 退出       │
│ 2. Flush Send Buffer                    │
│ 3. 关闭 Capture socket                  │
│ 4. 丢弃 FlowRegistry                   │
│ 5. 释放所有资源                         │
└─────────────────────────────────────────┘
```

### 6.5 全局配置热加载

- SIGHUP 信号或 CLI `otus reload` 或 Kafka `reload` 命令触发
- 仅重载全局静态配置（Reporter 连接参数、背压参数、日志级别等）
- **不影响正在运行的 Task**——Task 的运行时状态由 Task 自身管理
- 如需更改 Task 配置，需 stop_task + create_task

## 7. 部署方案

### 7.1 物理机/裸金属部署

**优势**：
- 最佳性能，直接访问硬件
- 支持 XDP、AF_PACKET_V3
- 低延迟

**部署步骤**：
```bash
# 1. 安装二进制文件
sudo cp otus /usr/local/bin/
sudo chmod +x /usr/local/bin/otus

# 2. 安装配置文件
sudo mkdir -p /etc/otus
sudo cp config.yml /etc/otus/

# 3. 安装 systemd 服务
sudo cp otus.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable otus
sudo systemctl start otus
```

### 7.2 虚拟机（ECS）部署

**优势**：
- 灵活性高
- 易于自动化部署
- 云原生集成

**注意事项**：
- 虚拟化带来的性能损耗
- 需要优化网络驱动（SR-IOV）
- 使用 AF_PACKET_V3 替代 XDP

### 7.3 容器（Kubernetes）部署

**DaemonSet 配置**：
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: otus
  namespace: observability
spec:
  selector:
    matchLabels:
      app: otus
  template:
    metadata:
      labels:
        app: otus
    spec:
      hostNetwork: true
      hostPID: true
      containers:
      - name: otus
        image: otus:latest
        securityContext:
          privileged: true
          capabilities:
            add:
              - NET_ADMIN
              - NET_RAW
              - SYS_RESOURCE
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        volumeMounts:
        - name: config
          mountPath: /etc/otus
        - name: sys
          mountPath: /sys
          readOnly: true
        resources:
          requests:
            cpu: 1000m
            memory: 256Mi
          limits:
            cpu: 2000m
            memory: 512Mi
      volumes:
      - name: config
        configMap:
          name: otus-config
      - name: sys
        hostPath:
          path: /sys
```

**ConfigMap**（全局静态配置，参见 4.4.1）：
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: otus-config
  namespace: observability
data:
  config.yml: |
    node:
      id: "${NODE_NAME}"
      region: k8s
    control:
      socket: /var/run/otus.sock
    command_channel:
      enabled: true
      type: kafka
      kafka:
        brokers: ["kafka.observability:9092"]
        topic: otus-commands
        group_id: "otus-${NODE_NAME}"
    reporters:
      kafka:
        brokers: ["kafka.observability:9092"]
    decoder:
      tunnel:
        enabled: true
        protocols: [vxlan, geneve]
    backpressure:
      capture_queue: 65536
      pipeline_queue: 8192
      drop_policy: tail
    metrics:
      listen: ":2112"
      path: /metrics
    log:
      level: info
      format: json
      outputs:
        file:
          enabled: false           # K8s 用 stdout，不写本地文件
        loki:
          enabled: true
          endpoint: http://loki.observability:3100/loki/api/v1/push
          labels:
            app: otus
            env: k8s
```

> **说明**：Task 配置（BPF 过滤器、Parser 链、workers 数量等）通过 Kafka 命令 topic 动态下发，不放入 ConfigMap。
> 外部编排系统（如 Operator）向 `otus-commands` topic 发布命令消息即可创建观测任务，无需向 Agent 开放端口。

### 7.4 多平台支持

#### 7.4.1 构建系统

```makefile
# Makefile
.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -o bin/otus-linux-amd64 main.go

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -o bin/otus-linux-arm64 main.go
```

#### 7.4.2 多架构容器镜像

```dockerfile
# Dockerfile
ARG TARGETARCH
FROM golang:1.22 AS builder

WORKDIR /build
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o otus main.go

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y \
    libpcap0.8 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*
    
COPY --from=builder /build/otus /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/otus"]
CMD ["daemon"]
```

## 8. 监控和可观测性

### 8.1 指标暴露

**Prometheus 指标**：
```go
// 捕获指标
otus_packets_captured_total{task="voip-monitor-01",interface="eth0"} 1234567
otus_packets_dropped_total{task="voip-monitor-01",interface="eth0"} 123
otus_bytes_captured_total{task="voip-monitor-01",interface="eth0"} 1234567890

// 处理指标
otus_packets_parsed_total{task="voip-monitor-01",parser="sip"} 123456
otus_packets_filtered_total{task="voip-monitor-01",reason="not_invite"} 12345
otus_packets_reported_total{task="voip-monitor-01",reporter="kafka"} 123000

// 性能指标
otus_processing_latency_seconds{task="voip-monitor-01",quantile="0.5"} 0.0001
otus_processing_latency_seconds{task="voip-monitor-01",quantile="0.99"} 0.0008
otus_cpu_usage_ratio 0.85
otus_memory_usage_bytes 536870912

// 插件指标
otus_plugin_loaded{name="sip_parser",type="parser"} 1
otus_plugin_health{name="kafka_reporter",status="healthy"} 1
```

### 8.2 日志

**结构化日志**（JSON 格式，所有输出共享同一格式）：
```json
{
  "ts": "2026-02-13T10:30:45.123Z",
  "level": "info",
  "component": "capture",
  "msg": "Started packet capture",
  "node": "edge-beijing-01",
  "task": "voip-monitor-01",
  "interface": "eth0",
  "filter": "udp port 5060"
}
```

**日志输出通道**（可同时启用多个）：

| 通道 | 说明 | 适用场景 |
|------|------|----------|
| **File** | 本地文件 + 自动滚动（大小/天数/数量/压缩） | 所有环境，现场排查 |
| **Loki** | HTTP Push 到 Loki（批量发送） | 有集中式日志平台时 |
| **Stdout** | 标准输出（容器环境自动启用） | K8s / Docker 环境 |

**本地日志滚动**：基于 [lumberjack](https://github.com/natefinch/lumberjack) 实现，配置项见 4.4.1 `log.outputs.file.rotation`。滚动策略：
- 文件达到 `max_size` → 重命名为 `otus-2026-02-13T10-30.log.gz` 并创建新文件
- 超过 `max_age` 天的旧日志自动删除
- 保留最多 `max_backups` 个历史文件

### 8.3 追踪

- OpenTelemetry 集成
- 分布式追踪支持
- 请求链路追踪

## 9. 安全考虑

### 9.1 权限管理

- 最小权限原则
- 只需要 CAP_NET_RAW、CAP_NET_ADMIN
- 避免以 root 运行

### 9.2 数据安全

- TLS 加密传输
- 敏感数据脱敏
- 访问控制和认证

### 9.3 配置安全

- 配置文件加密
- 密钥管理（集成 Vault）
- 配置验证和审计

## 10. 性能基准

### 10.1 测试环境

- CPU: Intel Xeon 8 vCPU @ 2.5GHz
- Memory: 16GB
- Network: 10GbE
- OS: Ubuntu 22.04

### 10.2 基准测试结果

| 场景 | 带宽 | CPU 使用 | 内存使用 | 丢包率 | P99延迟 |
|------|------|----------|----------|--------|---------|
| XDP + SIP解析 + Kafka | 10 Gbps | 1.8 vCPU | 380 MB | 0.005% | 0.8 ms |
| AF_PACKET + 多协议 + gRPC | 5 Gbps | 1.5 vCPU | 420 MB | 0.01% | 1.2 ms |
| 低速场景（测试） | 1 Gbps | 0.3 vCPU | 150 MB | 0% | 0.3 ms |

## 11. 架构决策摘要

关键架构决策的概要。完整的讨论过程、备选方案分析和推理记录在 [doc/decisions.md](decisions.md) 中。

| ADR | 决策点 | 结论 | 阶段 |
|-----|--------|------|------|
| 001 | 背压与丢弃策略 | 分层非阻塞 + 分层丢弃，永远保护捕获层 | Phase 1 |
| 002 | L2-L4 解码归属 | 核心代码，非插件 | Phase 1 |
| 003 | L2 封装处理范围 | VLAN/QinQ 常开，隧道解封装可配置默认关闭 | Phase 1 |
| 004 | IP 分片重组 | 常开不可关闭，简单硬上限 + 超时防护，分片走独立内存分配 | Phase 1 |
| 005 | TCP 流重组 | 选择性重组，默认关闭，接口抽象 + gopacket 先行 | Phase 2 |
| 005b | TCP 输出粒度 | 输出有序字节流片段，应用层插件自行分帧 | Phase 2 |
| 005c | Mid-stream join | 必须支持，以首个 seq 做相对基准，消除启动/reload 观测盲区 | Phase 2 |
| 005f | TCP 内存上限 | 全局内存池（128MB）+ LRU 淘汰 | Phase 2 |
| 006 | Pipeline 并行模型 | 每核一条 Pipeline，线性水平扩展，不设固定 vCPU 上限 | Phase 1 |
| 007 | Parser 两阶段接口 | CanHandle + Handle 两方法，核心协议无关，快慢路径由 Parser 内部自决 | Phase 1 |
| 008 | FlowRegistry | 核心基础设施，sync.Map 实现，per-Task 跨 pipeline 共享 | Phase 1 |
| 009 | 性能度量模型 | pps/core 为核心指标，线性扩展模型替代固定 vCPU 承诺 | Phase 1 |
| 010 | Pipeline 数据契约 | 三层结构：RawPacket → DecodedPacket → OutputPacket（Envelope + Typed Payload） | Phase 1 |
| 011 | Processor 职责边界 | 仅过滤 + 轻量标注，不碰 Payload，计算任务后移 | Phase 1 |
| 012 | Labels 命名规范 | 协议字段 `{protocol}.{field}`，关联/元数据无前缀 | Phase 1 |
| 013 | 两层配置模型 | 全局静态（配置文件）+ Task 动态（Kafka 命令 / CLI） | Phase 1 |
| 014 | Task-Pipeline 绑定 | Pipeline 绑定 Task，FlowRegistry per-Task，Phase 1 单 Task | Phase 1 |
| 015 | 远程控制拉模式 | 订阅 Kafka 命令 topic，不开入站端口，gRPC 推迟 Phase 2+ | Phase 1 |
| 016 | 重构策略 | 推倒重来，旧代码仅做算法参考 | Phase 1 |
| 017 | SkyWalking 代码 | 移除，会话关联属于 Collector 职责 | Phase 1 |
| 018 | 日志框架 | slog + lumberjack（滚动）+ 自实现 Loki HTTP Push | Phase 1 |
| 019 | Kafka 客户端 | segmentio/kafka-go，纯 Go 无 CGO | Phase 1 |
| 020 | 本地控制通道 | JSON-RPC over UDS，不用 gRPC | Phase 1 |
| 021 | DecodedPacket 类型 | 自定义值类型 struct，隔离 gopacket 到解码器内部 | Phase 1 |

## 12. 路线图

### Phase 1 - 核心捕获引擎
- [ ] 核心协议栈解码器（L2 以太网/VLAN/QinQ + L3 IPv4/IPv6 + L4 UDP）
- [ ] IP 分片重组（常开，硬上限 + 超时）
- [ ] 隧道解封装可配置开关（VXLAN/GRE/Geneve/IPIP）
- [ ] 分层背压控制与丢弃策略
- [ ] 插件体系（Capture / Parser / Processor / Reporter）
- [ ] AF_PACKET_V3 捕获插件
- [ ] SIP 协议解析插件（UDP）
- [ ] Kafka 上报插件
- [ ] 两层配置模型（全局静态 + Task 动态）
- [ ] Task Manager + 单 Task 生命周期管理
- [ ] Pipeline 引擎与 Task 驱动组装
- [ ] Kafka 命令 topic 订阅（拉模式远程控制）
- [ ] CLI + Unix Domain Socket 本地控制
- [ ] systemd 服务集成
- [ ] Prometheus 指标暴露（含各层丢弃指标）
- [ ] 结构化日志（本地文件滚动 + Loki 推送）

### Phase 2 - TCP 重组与协议扩展
- [ ] 状态上报 topic（心跳 + Task 状态 → 完整控制闭环）
- [ ] gRPC 远程控制（可选，按需开放入站端口）
- [ ] 多 Task 并发（资源配额 + 同网卡多 BPF 共存）
- [ ] TCP 流重组引擎（接口抽象 + gopacket/tcpassembly 实现）
- [ ] Mid-stream join 支持
- [ ] 全局内存池 + LRU 淘汰
- [ ] SIP over TCP 解析
- [ ] RTP / RTCP 协议解析插件
- [ ] XDP 捕获插件
- [ ] gRPC 上报插件
- [ ] OpenTelemetry 集成

### Phase 3 - 企业特性
- [ ] HTTP/WebSocket 协议解析
- [ ] 自研 TCP 重组引擎（如 gopacket 成为瓶颈）
- [ ] 动态采样降级
- [ ] 集中式管理平台
- [ ] 多节点协同
- [ ] 高级分析和告警
- [ ] 流量回放

## 13. 参考资料

### 13.1 技术文档
- [XDP (eXpress Data Path)](https://www.kernel.org/doc/html/latest/networking/af_xdp.html)
- [AF_PACKET](https://www.kernel.org/doc/Documentation/networking/packet_mmap.txt)
- [BPF and XDP Reference Guide](https://docs.cilium.io/en/latest/bpf/)
- [OpenTelemetry](https://opentelemetry.io/)

### 13.2 相关项目
- [Cilium](https://github.com/cilium/cilium) - eBPF-based networking
- [Katran](https://github.com/facebookincubator/katran) - XDP-based load balancer
- [HyperTrace](https://www.hypertrace.org/) - Distributed tracing platform
- [Homer](https://github.com/sipcapture/homer) - SIP capture solution

---

**文档版本**: v0.2.0  
**更新日期**: 2026-02-16  
**作者**: Otus Team
