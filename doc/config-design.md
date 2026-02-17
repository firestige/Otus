# 配置结构 & Kafka 设计文档

> **用途**: 供评审的完整配置层级设计 + Kafka 命令/数据面关键设计决策  
> **状态**: 已决 — 待实现  
> **日期**: 2026-02-17

---

## 1. 设计原则

1. **嵌套命名空间**：所有配置项在 `otus:` 根节点下，与架构文档一致
2. **两层配置**：全局静态配置（YAML 文件）+ 任务动态配置（Kafka/CLI）
3. **全局不可被 Task 覆盖**的项：decoder、backpressure、resources、reporters 连接
4. **Decoder 是全局的**：L2-L4 解码属于核心引擎固有行为，不是 Task 维度的配置
5. **Reporter 连接全局声明，Task 按名引用**：避免每个 Task 重复声明 brokers/TLS

---

## 2. 全局静态配置 — 完整 YAML 层级

```yaml
otus:
  # ────────────── 节点身份 ──────────────
  node:
    ip: ""                          # 节点 IP — 解析优先级：OTUS_NODE_IP 环境变量 > 自动探测（首个非 loopback/非 link-local IPv4）> 启动报错
    hostname: "edge-beijing-01"    # 节点主机名（空则自动获取 os.Hostname()）
    tags:                          # 自由标签（Label Processor 自动注入到所有 OutputPacket）
      datacenter: "cn-north"
      environment: "production"
      region: "cn-east-1"
      role: "edge"

  # ────────────── 本地控制 ──────────────
  control:
    socket: "/var/run/otus.sock"   # CLI ↔ daemon 通信的 Unix socket 路径
    pid_file: "/var/run/otus.pid"  # PID 文件路径

  # ────────────── Kafka 全局默认 ──────────────
  # command_channel.kafka 和 reporters.kafka 继承此处的 brokers/sasl/tls
  # 子节点中显式设置的字段覆盖全局默认
  kafka:
    brokers:
      - "kafka-1.example.com:9092"
      - "kafka-2.example.com:9092"
    sasl:                          # 认证配置（可选）
      enabled: false
      mechanism: "PLAIN"           # PLAIN | SCRAM-SHA-256 | SCRAM-SHA-512
      username: ""
      password: ""
    tls:                           # TLS 配置（可选）
      enabled: false
      ca_cert: ""
      client_cert: ""
      client_key: ""
      insecure_skip_verify: false

  # ────────────── 远程命令通道 ──────────────
  command_channel:
    enabled: true                  # 开关（false = 仅接受本地 CLI 命令）
    type: "kafka"                  # Phase 1 仅 kafka
    kafka:
      # brokers/sasl/tls 继承自 otus.kafka，如需覆盖可在此显式设置
      topic: "otus-commands"       # 命令 topic
      group_id: "otus-${node.hostname}"  # 按节点隔离消费
      auto_offset_reset: "latest"  # latest | earliest

  # ────────────── 共享 Reporter 连接 ──────────────
  # Task 的 reporter 部分通过 name 引用此处的连接配置
  # 例如 task.reporters[].name = "kafka" 则使用 otus.reporters.kafka 的连接
  reporters:
    kafka:
      # brokers/sasl/tls 继承自 otus.kafka，如需覆盖可在此显式设置
      compression: "snappy"        # none | gzip | snappy | lz4 | zstd
      max_message_bytes: 1048576   # 单条消息最大字节数
    # grpc:                        # Phase 2
    #   endpoint: "collector.example.com:4317"
    #   tls:
    #     enabled: true
    #     ca_cert: "/etc/otus/ca.pem"

  # ────────────── 全局资源上限 ──────────────
  resources:
    max_workers: 0                 # 0 = auto（GOMAXPROCS）

  # ────────────── 背压控制 ──────────────
  backpressure:
    pipeline_channel:
      capacity: 65536              # Pipeline 输入 channel 容量
      drop_policy: "tail"          # tail（丢新包）| head（丢旧包）
    send_buffer:
      capacity: 16384              # Send Buffer 容量
      drop_policy: "head"          # tail | head
      high_watermark: 0.8          # 触发背压信号的水位
      low_watermark: 0.3           # 解除背压信号的水位
    reporter:
      send_timeout: "3s"           # 单次发送超时
      max_retries: 1               # 发送失败重试次数

  # ────────────── 核心协议栈解码器 ──────────────
  # L2-L4 解码属于核心引擎固有行为，全局生效，不可被 Task 覆盖
  core:
    decoder:
      tunnel:
        vxlan: false               # VXLAN 解封装（UDP 4789）
        gre: false                 # GRE/ERSPAN 解封装
        geneve: false              # Geneve 解封装（UDP 6081）
        ipip: false                # IP-in-IP 解封装
      ip_reassembly:
        timeout: "30s"             # 分片超时时间
        max_fragments: 10000       # 全局最大跟踪分片数

  # ────────────── Prometheus 指标 ──────────────
  metrics:
    enabled: true
    listen: "0.0.0.0:9091"        # HTTP 监听地址
    path: "/metrics"               # 端点路径

  # ────────────── 日志 ──────────────
  log:
    level: "info"                  # debug | info | warn | error
    format: "json"                 # json | text
    outputs:
      file:
        enabled: true
        path: "/var/log/otus/otus.log"
        rotation:
          max_size_mb: 100           # 单文件最大（MB）
          max_age_days: 7            # 保留天数
          max_backups: 5           # 保留旧日志个数
          compress: true           # gzip 压缩旧日志
      loki:
        enabled: false
        endpoint: "http://loki.observability:3100/loki/api/v1/push"
        labels:
          app: "otus"
          env: "production"
        batch_size: 100
        batch_timeout: "1s"
```

---

## 3. Go 结构体层级映射

```
GlobalConfig                         mapstructure:"otus"
├── Node       NodeConfig            mapstructure:"node"
│   ├── IP         string            mapstructure:"ip"          // 空 = 自动探测
│   ├── Hostname   string            mapstructure:"hostname"
│   └── Tags       map[string]string mapstructure:"tags"
├── Control    ControlConfig         mapstructure:"control"
│   ├── Socket     string            mapstructure:"socket"
│   └── PIDFile    string            mapstructure:"pid_file"
├── Kafka      GlobalKafkaConfig     mapstructure:"kafka"       // 全局 Kafka 默认
│   ├── Brokers    []string          mapstructure:"brokers"
│   ├── SASL       SASLConfig        mapstructure:"sasl"
│   └── TLS        TLSConfig         mapstructure:"tls"
├── CommandChannel CommandChannelConfig  mapstructure:"command_channel"
│   ├── Enabled    bool              mapstructure:"enabled"
│   ├── Type       string            mapstructure:"type"
│   └── Kafka      CommandKafkaConfig mapstructure:"kafka"
│       ├── Brokers         []string mapstructure:"brokers"     // 空 = 继承 otus.kafka
│       ├── Topic           string   mapstructure:"topic"
│       ├── GroupID         string   mapstructure:"group_id"
│       ├── AutoOffsetReset string   mapstructure:"auto_offset_reset"
│       ├── SASL            SASLConfig mapstructure:"sasl"      // 零值 = 继承 otus.kafka
│       └── TLS             TLSConfig  mapstructure:"tls"       // 零值 = 继承 otus.kafka
├── Reporters  ReportersConfig       mapstructure:"reporters"
│   └── Kafka      KafkaConnectionConfig mapstructure:"kafka"
│       ├── Brokers          []string mapstructure:"brokers"    // 空 = 继承 otus.kafka
│       ├── Compression      string   mapstructure:"compression"
│       ├── MaxMessageBytes  int      mapstructure:"max_message_bytes"
│       ├── SASL             SASLConfig mapstructure:"sasl"     // 零值 = 继承 otus.kafka
│       └── TLS              TLSConfig  mapstructure:"tls"      // 零值 = 继承 otus.kafka
├── Resources  ResourcesConfig       mapstructure:"resources"
│   └── MaxWorkers int               mapstructure:"max_workers"
├── Backpressure BackpressureConfig  mapstructure:"backpressure"
│   ├── PipelineChannel PipelineChannelConfig mapstructure:"pipeline_channel"
│   │   ├── Capacity   int           mapstructure:"capacity"
│   │   └── DropPolicy string        mapstructure:"drop_policy"
│   ├── SendBuffer SendBufferConfig  mapstructure:"send_buffer"
│   │   ├── Capacity       int       mapstructure:"capacity"
│   │   ├── DropPolicy     string    mapstructure:"drop_policy"
│   │   ├── HighWatermark  float64   mapstructure:"high_watermark"
│   │   └── LowWatermark   float64   mapstructure:"low_watermark"
│   └── Reporter ReporterBackpressureConfig mapstructure:"reporter"
│       ├── SendTimeout string        mapstructure:"send_timeout"
│       └── MaxRetries  int           mapstructure:"max_retries"
├── Core       CoreConfig            mapstructure:"core"
│   └── Decoder    DecoderConfig     mapstructure:"decoder"
│       ├── Tunnel     TunnelConfig  mapstructure:"tunnel"
│       │   ├── VXLAN   bool         mapstructure:"vxlan"
│       │   ├── GRE     bool         mapstructure:"gre"
│       │   ├── Geneve  bool         mapstructure:"geneve"
│       │   └── IPIP    bool         mapstructure:"ipip"
│       └── IPReassembly IPReassemblyConfig mapstructure:"ip_reassembly"
│           ├── Timeout       string  mapstructure:"timeout"
│           └── MaxFragments  int     mapstructure:"max_fragments"
├── Metrics    MetricsConfig         mapstructure:"metrics"
│   ├── Enabled bool                 mapstructure:"enabled"
│   ├── Listen  string               mapstructure:"listen"
│   └── Path    string               mapstructure:"path"
└── Log        LogConfig             mapstructure:"log"
    ├── Level   string               mapstructure:"level"
    ├── Format  string               mapstructure:"format"
    └── Outputs LogOutputsConfig     mapstructure:"outputs"
        ├── File FileOutputConfig    mapstructure:"file"
        │   ├── Enabled  bool        mapstructure:"enabled"
        │   ├── Path     string      mapstructure:"path"
        │   └── Rotation RotationConfig mapstructure:"rotation"
        │       ├── MaxSizeMB  int    mapstructure:"max_size_mb"
        │       ├── MaxAgeDays int    mapstructure:"max_age_days"
        │       ├── MaxBackups int    mapstructure:"max_backups"
        │       └── Compress   bool   mapstructure:"compress"
        └── Loki LokiOutputConfig    mapstructure:"loki"
            ├── Enabled      bool             mapstructure:"enabled"
            ├── Endpoint     string           mapstructure:"endpoint"
            ├── Labels       map[string]string mapstructure:"labels"
            ├── BatchSize    int              mapstructure:"batch_size"
            └── BatchTimeout string           mapstructure:"batch_timeout"

共享子结构体:
├── SASLConfig
│   ├── Enabled    bool   mapstructure:"enabled"
│   ├── Mechanism  string mapstructure:"mechanism"   # PLAIN | SCRAM-SHA-256 | SCRAM-SHA-512
│   ├── Username   string mapstructure:"username"
│   └── Password   string mapstructure:"password"
└── TLSConfig
    ├── Enabled            bool   mapstructure:"enabled"
    ├── CACert             string mapstructure:"ca_cert"
    ├── ClientCert         string mapstructure:"client_cert"
    ├── ClientKey          string mapstructure:"client_key"
    └── InsecureSkipVerify bool   mapstructure:"insecure_skip_verify"
```

---

## 4. Task 动态配置 — 层级（无变更）

Task 配置由 Kafka 命令或 CLI 下发，结构与架构文档一致：

```
TaskConfig
├── ID           string
├── Workers      int
├── Capture      CaptureConfig
│   ├── Name          string
│   ├── DispatchMode  string
│   ├── Interface     string
│   ├── BPFFilter     string
│   ├── SnapLen       int
│   └── Config        map[string]any   # 插件特有（fanout_group 等）
├── Parsers      []ParserConfig
│   ├── Name    string
│   └── Config  map[string]any
├── Processors   []ProcessorConfig
│   ├── Name    string
│   └── Config  map[string]any
├── Reporters    []ReporterConfig
│   ├── Name    string                 # 引用 otus.reporters.{name} 的连接配置
│   └── Config  map[string]any         # 业务参数（topic, batch_size 等）
└── UnmatchedPolicy string             # "forward" | "drop"
```

**关键变更**:
- **移除 `Decoder DecoderConfig`**：Decoder 配置提升到全局 `otus.core.decoder`，Task 不再携带解码器配置
- **新增 `UnmatchedPolicy`**：当 Pipeline 中所有 Parser 都不匹配（`CanHandle()` 全部返回 false）时：
  - `forward`：将原始 payload 作为 `RawPayload` 传递给 Processor 链和 Reporter
  - `drop`：直接丢弃该包（不进入后续处理链）

---

## 5. 与当前代码的 diff 总结

### 5.1 需要变更的 Go 文件

| 文件 | 变更内容 |
|------|---------|
| `internal/config/config.go` | 整体重构：`GlobalConfig` 嵌套层级对齐本文档 |
| `internal/config/task.go` | 移除 `Decoder DecoderConfig`，新增 `UnmatchedPolicy string` |
| `configs/config.yml` | 重写为 `otus:` 嵌套格式 |
| `internal/config/config_test.go` | 测试更新 |
| `internal/pipeline/pipeline.go` | 从全局配置读取 decoder 参数而非 TaskConfig |
| `internal/task/task.go` | 传递全局 decoder 配置给 Pipeline |
| `internal/command/kafka.go` | 使用新的 `CommandChannelConfig` |
| `internal/log/log.go` | 适配新的 log output 结构（命名 map → 专用类型） |
| `cmd/daemon.go` | 使用新的嵌套路径读取配置 |

### 5.2 命名空间迁移对照

| 当前 (flat) | 目标 (nested) |
|---|---|
| `daemon.pid_file` | `otus.control.pid_file` |
| `daemon.socket_path` | `otus.control.socket` |
| `kafka.brokers` | `otus.kafka.brokers`（全局默认） |
| `kafka.command_topic` | `otus.command_channel.kafka.topic` |
| `kafka.command_group` | `otus.command_channel.kafka.group_id` |
| `agent.id` | `otus.node.hostname`（空则自动获取） |
| `agent.tags` | `otus.node.tags` |
| `log.*` | `otus.log.*` |
| `metrics.*` | `otus.metrics.*` |
| — (新增) | `otus.node.ip` |
| — (新增) | `otus.kafka.*`（全局 Kafka 默认） |
| — (新增) | `otus.reporters.*` |
| — (新增) | `otus.resources.*` |
| — (新增) | `otus.backpressure.*` |
| — (新增) | `otus.core.decoder.*` |
| `task.decoder.*` (移除) | `otus.core.decoder.*` |

### 5.3 环境变量覆盖

Viper `SetEnvPrefix("OTUS")` + `AutomaticEnv()` 保持不变。嵌套路径通过 `_` 分隔：
```
OTUS_NODE_IP=10.0.0.1
OTUS_CONTROL_SOCKET=/tmp/otus.sock
OTUS_KAFKA_BROKERS=kafka-1:9092,kafka-2:9092
OTUS_COMMAND_CHANNEL_ENABLED=false
OTUS_REPORTERS_KAFKA_COMPRESSION=lz4
OTUS_CORE_DECODER_TUNNEL_VXLAN=true
OTUS_LOG_OUTPUTS_FILE_ROTATION_MAX_SIZE_MB=200
```

---

## 6. 设计决策

### 6.1 `otus.node.ip` 获取方式 [已决]

**决策**：混合方案 — 环境变量 > 自动探测 > 启动报错

解析优先级：
1. **环境变量 `OTUS_NODE_IP`**：若已设置，直接使用（Viper `AutomaticEnv()` 自动映射 `otus.node.ip`）
2. **YAML 显式配置** `otus.node.ip`：若 YAML 中已填值，使用该值
3. **自动探测**：遍历 `net.Interfaces()`，取第一个满足以下条件的地址：
   - 网卡 UP 且非 loopback
   - IPv4 地址
   - 非 link-local（排除 169.254.x.x）
4. **启动报错**：以上全部失败 → `log.Fatal("cannot resolve node IP: set OTUS_NODE_IP or otus.node.ip")`

```go
func resolveNodeIP(cfg *NodeConfig) (string, error) {
    // 1. 环境变量 / YAML 显式配置（Viper 已合并）
    if cfg.IP != "" {
        return cfg.IP, nil
    }
    // 2. 自动探测
    ifaces, _ := net.Interfaces()
    for _, iface := range ifaces {
        if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
            continue
        }
        addrs, _ := iface.Addrs()
        for _, addr := range addrs {
            ipNet, ok := addr.(*net.IPNet)
            if !ok { continue }
            ip4 := ipNet.IP.To4()
            if ip4 == nil { continue }
            if ip4[0] == 169 && ip4[1] == 254 { continue } // link-local
            return ip4.String(), nil
        }
    }
    return "", fmt.Errorf("cannot resolve node IP: set OTUS_NODE_IP or otus.node.ip")
}
```

### 6.2 Kafka 连接配置继承 [已决]

**决策**：全局 `otus.kafka` 节提供共享默认值；`command_channel.kafka` 和 `reporters.kafka` 继承全局，显式设置的字段覆盖。

**继承规则**：
- `otus.kafka` 定义 `brokers`、`sasl`、`tls` 的默认值
- `otus.command_channel.kafka` 中未设置的 `brokers`/`sasl`/`tls` 从 `otus.kafka` 继承
- `otus.reporters.kafka` 中未设置的 `brokers`/`sasl`/`tls` 从 `otus.kafka` 继承
- 子节点显式设置的字段（非零值）覆盖全局默认

**合并逻辑**在 `validateAndApplyDefaults()` 中实现：

```go
func (g *GlobalConfig) validateAndApplyDefaults() error {
    // Kafka 继承：command_channel.kafka
    cc := &g.CommandChannel.Kafka
    if len(cc.Brokers) == 0 {
        cc.Brokers = g.Kafka.Brokers
    }
    if !cc.SASL.Enabled && g.Kafka.SASL.Enabled {
        cc.SASL = g.Kafka.SASL
    }
    if !cc.TLS.Enabled && g.Kafka.TLS.Enabled {
        cc.TLS = g.Kafka.TLS
    }

    // Kafka 继承：reporters.kafka
    rk := &g.Reporters.Kafka
    if len(rk.Brokers) == 0 {
        rk.Brokers = g.Kafka.Brokers
    }
    if !rk.SASL.Enabled && g.Kafka.SASL.Enabled {
        rk.SASL = g.Kafka.SASL
    }
    if !rk.TLS.Enabled && g.Kafka.TLS.Enabled {
        rk.TLS = g.Kafka.TLS
    }

    // node.ip 解析
    ip, err := resolveNodeIP(&g.Node)
    if err != nil {
        return err
    }
    g.Node.IP = ip

    return nil
}
```

**典型场景**：
| 场景 | otus.kafka.brokers | command_channel.kafka.brokers | 生效值 |
|------|---|---|---|
| 同一集群 | `[k1, k2]` | — (空) | `[k1, k2]` |
| 不同集群 | `[k1, k2]` | `[k3, k4]` | `[k3, k4]` |
| 仅命令通道用 Kafka | — (空) | `[k3, k4]` | `[k3, k4]` |

### 6.3 `max_size` / `max_age` 字段格式 [已决]

**决策**：使用数值字段，单位编码在字段名中

```yaml
rotation:
  max_size_mb: 100     # int —— MB
  max_age_days: 7      # int —— 天
  max_backups: 5
  compress: true
```

```go
type RotationConfig struct {
    MaxSizeMB  int  `mapstructure:"max_size_mb"`
    MaxAgeDays int  `mapstructure:"max_age_days"`
    MaxBackups int  `mapstructure:"max_backups"`
    Compress   bool `mapstructure:"compress"`
}
```

**理由**：无需解析函数、无单位歧义、Viper 环境变量覆盖直接传数值（`OTUS_LOG_OUTPUTS_FILE_ROTATION_MAX_SIZE_MB=200`）。

---

## 附录 A: 完整 Go 结构体参考

```go
// ─── 顶层 ───
type GlobalConfig struct {
    Node           NodeConfig           `mapstructure:"node"`
    Control        ControlConfig        `mapstructure:"control"`
    Kafka          GlobalKafkaConfig    `mapstructure:"kafka"`
    CommandChannel CommandChannelConfig `mapstructure:"command_channel"`
    Reporters      ReportersConfig      `mapstructure:"reporters"`
    Resources      ResourcesConfig      `mapstructure:"resources"`
    Backpressure   BackpressureConfig   `mapstructure:"backpressure"`
    Core           CoreConfig           `mapstructure:"core"`
    Metrics        MetricsConfig        `mapstructure:"metrics"`
    Log            LogConfig            `mapstructure:"log"`
}

// ─── 节点身份 ───
type NodeConfig struct {
    IP       string            `mapstructure:"ip"`       // 空 = 自动探测（见 §6.1）
    Hostname string            `mapstructure:"hostname"`
    Tags     map[string]string `mapstructure:"tags"`
}

// ─── 控制面 ───
type ControlConfig struct {
    Socket  string `mapstructure:"socket"`
    PIDFile string `mapstructure:"pid_file"`
}

// ─── Kafka 全局默认（见 §6.2）───
type GlobalKafkaConfig struct {
    Brokers []string   `mapstructure:"brokers"`
    SASL    SASLConfig `mapstructure:"sasl"`
    TLS     TLSConfig  `mapstructure:"tls"`
}

// ─── 命令通道 ───
type CommandChannelConfig struct {
    Enabled bool              `mapstructure:"enabled"`
    Type    string            `mapstructure:"type"` // "kafka"
    Kafka   CommandKafkaConfig `mapstructure:"kafka"`
}

type CommandKafkaConfig struct {
    Brokers         []string   `mapstructure:"brokers"`
    Topic           string     `mapstructure:"topic"`
    GroupID         string     `mapstructure:"group_id"`
    AutoOffsetReset string     `mapstructure:"auto_offset_reset"`
    SASL            SASLConfig `mapstructure:"sasl"`
    TLS             TLSConfig  `mapstructure:"tls"`
}

// ─── 共享认证 ───
type SASLConfig struct {
    Enabled   bool   `mapstructure:"enabled"`
    Mechanism string `mapstructure:"mechanism"`
    Username  string `mapstructure:"username"`
    Password  string `mapstructure:"password"`
}

type TLSConfig struct {
    Enabled            bool   `mapstructure:"enabled"`
    CACert             string `mapstructure:"ca_cert"`
    ClientCert         string `mapstructure:"client_cert"`
    ClientKey          string `mapstructure:"client_key"`
    InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
}

// ─── 共享 Reporter 连接 ───
type ReportersConfig struct {
    Kafka KafkaConnectionConfig `mapstructure:"kafka"`
    // GRPC  GRPCConnectionConfig `mapstructure:"grpc"` // Phase 2
}

type KafkaConnectionConfig struct {
    Brokers         []string   `mapstructure:"brokers"`
    Compression     string     `mapstructure:"compression"`
    MaxMessageBytes int        `mapstructure:"max_message_bytes"`
    SASL            SASLConfig `mapstructure:"sasl"`
    TLS             TLSConfig  `mapstructure:"tls"`
}

// ─── 资源与背压 ───
type ResourcesConfig struct {
    MaxWorkers int `mapstructure:"max_workers"`
}

type BackpressureConfig struct {
    PipelineChannel PipelineChannelConfig        `mapstructure:"pipeline_channel"`
    SendBuffer      SendBufferConfig             `mapstructure:"send_buffer"`
    Reporter        ReporterBackpressureConfig   `mapstructure:"reporter"`
}

type PipelineChannelConfig struct {
    Capacity   int    `mapstructure:"capacity"`
    DropPolicy string `mapstructure:"drop_policy"` // "tail" | "head"
}

type SendBufferConfig struct {
    Capacity      int     `mapstructure:"capacity"`
    DropPolicy    string  `mapstructure:"drop_policy"`
    HighWatermark float64 `mapstructure:"high_watermark"`
    LowWatermark  float64 `mapstructure:"low_watermark"`
}

type ReporterBackpressureConfig struct {
    SendTimeout string `mapstructure:"send_timeout"`
    MaxRetries  int    `mapstructure:"max_retries"`
}

// ─── 核心解码器 ───
type CoreConfig struct {
    Decoder DecoderConfig `mapstructure:"decoder"`
}

type DecoderConfig struct {
    Tunnel       TunnelConfig       `mapstructure:"tunnel"`
    IPReassembly IPReassemblyConfig `mapstructure:"ip_reassembly"`
}

type TunnelConfig struct {
    VXLAN  bool `mapstructure:"vxlan"`
    GRE    bool `mapstructure:"gre"`
    Geneve bool `mapstructure:"geneve"`
    IPIP   bool `mapstructure:"ipip"`
}

type IPReassemblyConfig struct {
    Timeout      string `mapstructure:"timeout"`
    MaxFragments int    `mapstructure:"max_fragments"`
}

// ─── 指标 ───
type MetricsConfig struct {
    Enabled bool   `mapstructure:"enabled"`
    Listen  string `mapstructure:"listen"`
    Path    string `mapstructure:"path"`
}

// ─── 日志 ───
type LogConfig struct {
    Level   string           `mapstructure:"level"`
    Format  string           `mapstructure:"format"`
    Outputs LogOutputsConfig `mapstructure:"outputs"`
}

type LogOutputsConfig struct {
    File FileOutputConfig `mapstructure:"file"`
    Loki LokiOutputConfig `mapstructure:"loki"`
}

type FileOutputConfig struct {
    Enabled  bool           `mapstructure:"enabled"`
    Path     string         `mapstructure:"path"`
    Rotation RotationConfig `mapstructure:"rotation"`
}

type RotationConfig struct {
    MaxSizeMB  int  `mapstructure:"max_size_mb"`  // MB
    MaxAgeDays int  `mapstructure:"max_age_days"` // 天
    MaxBackups int  `mapstructure:"max_backups"`
    Compress   bool `mapstructure:"compress"`
}

type LokiOutputConfig struct {
    Enabled      bool              `mapstructure:"enabled"`
    Endpoint     string            `mapstructure:"endpoint"`
    Labels       map[string]string `mapstructure:"labels"`
    BatchSize    int               `mapstructure:"batch_size"`
    BatchTimeout string            `mapstructure:"batch_timeout"`
}
```

---

## 7. H2: Kafka 命令消息格式设计

### 7.1 背景与用途

Kafka 命令通道的核心用途是**远程控制面**：运维平台/中控系统通过 Kafka topic 向分布在各节点的 Otus Agent 下发任务管理指令（创建/删除/查询 Task、重载配置）。

与本地 CLI（UDS 直连）的区别：
- **CLI**：点对点、同步请求-响应、可靠交付
- **Kafka**：广播/定向、异步、至少一次交付（at-least-once）

### 7.2 当前实现 vs 架构文档

| 维度 | 当前实现（简化 JSON-RPC） | 架构文档（完整命令格式） |
|------|---|---|
| 消息结构 | `{method, params, id}` | `{version, target, command, timestamp, request_id, payload}` |
| 节点定向 | 无（所有节点消费所有消息） | `target` 字段（匹配 node.id 或 `"*"` 广播） |
| 协议版本 | 无 | `version: "v1"`（向前兼容用） |
| 时间戳 | 无 | `timestamp`（命令发出时间） |
| 去重 | 无 | `request_id` 可用于幂等去重 |

### 7.3 需要解决的问题

#### 7.3.1 去重（Idempotency）[已决]

Kafka at-least-once 语义下，同一消息可能被投递多次（consumer 重平衡、提交失败重试等）。

**问题场景**：`task_create` 被投递 2 次 → 第一次成功，第二次报错（ID 冲突）但不是严重问题；`task_delete` 被投递 2 次 → 第一次成功，第二次 not found（也无害）。

**分析**：Phase 1 中命令都是幂等或无害重试的。但 `request_id` 字段留给 Phase 2 用于精确去重（agent 侧维护 LRU 缓存记录已处理的 request_id）。

**决策**：Phase 1 在 `KafkaCommand` 结构中增加 `request_id` 字段，日志记录时关联 request_id 便于链路追踪。Phase 2 实现基于 LRU 的精确去重（`lru.Cache` 缓存最近 N 条已处理的 request_id）。

#### 7.3.2 排序（Ordering）[已决]

Kafka 同一 partition 内保证顺序。

**问题场景**：先 `task_create`、后 `task_delete`，如果两条消息落在不同 partition 且消费速度不一致，可能先收到 delete。

**分析**：使用 `group_id = "otus-${node.hostname}"` 做节点隔离消费，每个 agent 独立消费。如果 `target` 指定了节点，可以用 `target` 做 Kafka message key，让同一目标节点的消息落到同一 partition → 保证顺序。

**决策**：
1. **Agent 侧**：在 `task_create` 时做冲突检查（已有同名 Task），在 `task_delete` 时做存在性检查（不存在则忽略），实现天然的乱序容忍。
2. **发送端要求**（需文档化）：发送端**必须**使用 `target` 作为 Kafka message key，确保同一目标节点的命令落到同一 partition。此要求应写入 API 文档 / 运维手册的 "Kafka 命令通道接入指南" 中。

> **发送端约束文档**：
> ```
> Kafka Message Key = KafkaCommand.Target
> 
> 示例（Go）:
>   msg := kafka.Message{
>       Topic: "otus-commands",
>       Key:   []byte(cmd.Target),   // 保证同目标节点的命令有序
>       Value: payload,
>   }
> ```

#### 7.3.3 过期命令（Stale Command）[已决]

Agent 重启后消费到旧命令（如果 `auto_offset_reset=earliest` 或 consumer group 未提交 offset）。

**决策**：
1. 默认 `auto_offset_reset=latest` 并持久化 consumer offset（Kafka 自动管理），正常情况下不会消费旧命令。
2. `timestamp` 字段用于防御性检查：拒绝超过 TTL（配置项 `command_ttl`，默认 `5m`）的命令，记录 WARN 日志。

### 7.4 推荐方案

对齐架构文档的完整格式，同时保持与 UDS（JSON-RPC）的内部转换：

```
Kafka 消息                              内部 Command
┌────────────────┐                    ┌──────────────┐
│ version: "v1"  │                    │              │
│ target: "..."  │ ─── 过滤+转换 ──→  │ method       │
│ command: "..."  │                   │ params       │
│ timestamp: ... │                    │ id           │
│ request_id: .. │                    │              │
│ payload: {...} │                    └──────────────┘
└────────────────┘
```

**Kafka 消息结构（对齐架构文档）**：
```go
type KafkaCommand struct {
    Version   string          `json:"version"`     // "v1"
    Target    string          `json:"target"`      // node hostname 或 "*"
    Command   string          `json:"command"`     // "task_create" 等
    Timestamp time.Time       `json:"timestamp"`   // 命令发出时间
    RequestID string          `json:"request_id"`  // 唯一请求 ID
    Payload   json.RawMessage `json:"payload"`     // 命令参数
}
```

**处理流程**：
1. 反序列化为 `KafkaCommand`
2. 过滤：`target` 不匹配本节点 → 跳过
3. 防御性检查：`timestamp` 超过 TTL（配置项，默认 5m） → 跳过并记录告警
4. 转换为内部 `Command{Method: kc.Command, Params: kc.Payload, ID: kc.RequestID}`
5. 调用 `handler.Handle(ctx, cmd)`

### 7.5 变更影响

| 文件 | 变更 |
|------|------|
| `internal/command/kafka.go` | `processMessage()` 解析 `KafkaCommand` 而非直接解析 `Command`；增加 target 过滤和 timestamp TTL 检查 |
| `internal/command/handler.go` | `Command.Method` 字段不变（已用下划线风格），`Command.ID` 存储 `request_id` |
| 无需改 UDS | UDS 继续使用 JSON-RPC 格式 |

---

## 8. H3: Kafka Reporter 动态 Topic 路由

### 8.1 架构文档设计

架构文档 §4.2 Reporter 章节明确描述了按协议分 topic 的路由模式：

```go
// 架构文档中的示例代码
func (r *KafkaReporter) Report(pkt *OutputPacket) error {
    topic := "otus-" + pkt.Protocol  // 按协议分 topic
    ...
}
```

配置示例使用 `topic_prefix` 而非固定 `topic`：
```yaml
reporters:
  - name: kafka
    config:
      topic_prefix: otus    # 实际 topic = otus-{protocol}
```

### 8.2 当前实现

当前 Kafka Reporter 使用固定 topic：
```go
// kafka.go 当前实现
writerConfig := kafka.WriterConfig{
    Topic: cfg.Topic,  // 固定 topic
}
```

### 8.3 推荐方案

支持两种模式，由配置决定：

| 配置 | 路由行为 | topic 示例 |
|------|---------|-----------|
| `topic: "voip-packets"` | 固定 topic | `voip-packets` |
| `topic_prefix: "otus"` | 动态路由 | `otus-sip`, `otus-rtp`, `otus-raw` |

当 `topic_prefix` 存在时优先使用动态路由；`topic` 和 `topic_prefix` 互斥。

**路由键**：使用 `OutputPacket.PayloadType`（即 Parser 返回的协议类型：`"sip"`, `"rtp"`, `"raw"` 等）。

**实现变更**：
```go
func (r *KafkaReporter) resolveTopic(pkt *core.OutputPacket) string {
    if r.config.TopicPrefix != "" {
        proto := pkt.PayloadType
        if proto == "" {
            proto = "raw"
        }
        return r.config.TopicPrefix + "-" + proto
    }
    return r.config.Topic
}
```

`kafka.Writer` 需要改为不设 `Topic`（留空），而在每个 `kafka.Message` 上设 `Topic`：
```go
msg := kafka.Message{
    Topic: r.resolveTopic(pkt),
    Key:   ...,
    Value: ...,
}
```

**Config 变更**：
```go
type Config struct {
    Brokers      []string      `json:"brokers"`
    Topic        string        `json:"topic"`         // 固定 topic（与 topic_prefix 互斥）
    TopicPrefix  string        `json:"topic_prefix"`  // 动态路由前缀
    // ... 其余不变
}
```

### 8.4 变更影响

| 文件 | 变更 |
|------|------|
| `plugins/reporter/kafka/kafka.go` | Config 增加 `TopicPrefix`；Init 校验互斥；Report 中调用 `resolveTopic()`；Writer 不预设 Topic |
| `plugins/reporter/kafka/kafka_test.go` | 增加动态路由测试用例 |

---

## 9. H4: Kafka 数据序列化策略

### 9.1 问题陈述

当前 Kafka Reporter 使用纯 JSON 序列化所有 `OutputPacket`。用户提问：**Kafka 是否只能传文本？能否用二进制序列化？**

### 9.2 事实澄清

**Kafka `message.value` 是 `[]byte`，支持任意二进制内容**。Kafka 本身对消息内容不做格式假设，JSON、Protobuf、Avro、MessagePack、裸二进制都可以。

### 9.3 架构文档的设计

架构文档已经为此预留了双序列化接口：

```go
type Payload interface {
    Type() string
    MarshalJSON() ([]byte, error)
    MarshalBinary() ([]byte, error)  // protobuf / msgpack / 自定义 binary
}
```

架构文档 Kafka Reporter 示例代码使用的是 `MarshalBinary()`：
```go
value, err := pkt.Payload.MarshalBinary()
```

### 9.4 序列化方案对比

| 方案 | Envelope | Payload | 优点 | 缺点 |
|------|----------|---------|------|------|
| A. 全 JSON | JSON | JSON | 可读性好、调试方便 | 体积大、CPU 开销高 |
| B. 全二进制 | Protobuf | Protobuf | 体积最小、性能最高 | 需要 schema 管理、不可直接读 |
| C. **混合：Envelope JSON + Payload Binary** | JSON | 二进制 (base64 in JSON) | Envelope 可读 + Payload 紧凑 | base64 膨胀 33% |
| D. **混合：Envelope JSON + Payload 字段原生** | JSON | JSON (SIP) / base64 (RTP) | 文本协议可读、二进制协议紧凑 | 条件逻辑 |
| E. **Kafka 原生：Headers + Binary Value** | Kafka Headers | `MarshalBinary()` | 零膨胀、Headers 可被 Kafka Streams 过滤 | Consumer 需要了解 binary 格式 |

### 9.5 推荐方案

**方案 E：Kafka Headers 承载 Envelope，Value 承载 Payload 二进制**

这与架构文档的设计完全一致：

```go
func (r *KafkaReporter) Report(ctx context.Context, pkt *core.OutputPacket) error {
    // Payload → binary (Protobuf/MessagePack/自定义)
    value, err := pkt.Payload.MarshalBinary()  // 或根据配置选择 MarshalJSON()
    
    // Envelope 信息 → Kafka Headers
    headers := []kafka.Header{
        {Key: "task_id",      Value: []byte(pkt.TaskID)},
        {Key: "agent_id",     Value: []byte(pkt.AgentID)},
        {Key: "payload_type", Value: []byte(pkt.PayloadType)},
        {Key: "src_ip",       Value: []byte(pkt.SrcIP.String())},
        {Key: "dst_ip",       Value: []byte(pkt.DstIP.String())},
        {Key: "timestamp",    Value: []byte(fmt.Sprintf("%d", pkt.Timestamp.UnixMilli()))},
    }
    // Labels → Headers
    for k, v := range pkt.Labels {
        headers = append(headers, kafka.Header{Key: "l." + k, Value: []byte(v)})
    }
    
    msg := kafka.Message{
        Topic:   r.resolveTopic(pkt),
        Key:     []byte(flowKey),          // 5-tuple 用于 partition 分配
        Value:   value,                     // 纯二进制 payload
        Headers: headers,
    }
    return r.writer.WriteMessages(ctx, msg)
}
```

**可配置的序列化格式**：

```yaml
reporters:
  - name: kafka
    config:
      topic_prefix: otus
      serialization: binary    # "json" | "binary"  默认 "json"
```

- `json`：Value = `pkt.Payload.MarshalJSON()` — 调试友好，Phase 1 默认
- `binary`：Value = `pkt.Payload.MarshalBinary()` — 生产推荐

Phase 1 可先实现 JSON 模式（当前已有），binary 模式在 Payload 接口实现完善后启用。

### 9.6 前置条件

当前 `OutputPacket.Payload` 类型是 `any`，需要演进为架构文档中定义的 `Payload` 接口：

```go
type Payload interface {
    Type() string
    MarshalJSON() ([]byte, error)
    MarshalBinary() ([]byte, error)
}
```

各 Parser 需要实现具体的 Payload 类型（如 `SIPPayload`），这是 Phase 2 的工作。Phase 1 中：
- SIP Parser 已返回结构化 labels，Payload 为 nil，原始报文在 `RawPayload`
- Reporter 可以用当前的 `serializePacket()` JSON 模式，同时预留 `serialization` 配置项

### 9.7 变更影响

| 文件 | 变更 | Phase |
|------|------|-------|
| `internal/core/packet.go` | `Payload any` → `Payload` 接口（或保持 `any` + type switch） | Phase 2 |
| `plugins/reporter/kafka/kafka.go` | Config 增加 `Serialization`；Report 方法重构为 Headers + Value 分离 | Phase 1 可开始 |
| 各 Parser 插件 | 实现 `Payload` 接口（`SIPPayload.MarshalJSON()` / `MarshalBinary()`） | Phase 2 |
