# Otus API 参考手册

> 快速查询手册。数据模型直接对应代码中的 Go 结构体，所有字段名均为 JSON/YAML key。
> 设计决策背景见 [decisions.md](decisions.md)，完整架构见 [architecture.md](architecture.md)。

---

## 目录

- [1. 控制面通道对比](#1-控制面通道对比)
- [2. 本地控制：JSON-RPC over UDS](#2-本地控制json-rpc-over-uds)
- [3. 远程控制：Kafka 命令 topic](#3-远程控制kafka-命令-topic)
- [4. 远程响应：Kafka 响应 topic](#4-远程响应kafka-响应-topic)
- [5. 命令参考](#5-命令参考)
- [6. 错误码](#6-错误码)
- [7. Task 配置模型](#7-task-配置模型)
- [8. 全局配置模型](#8-全局配置模型)
- [9. 上报数据结构](#9-上报数据结构)
- [10. Labels 命名规范](#10-labels-命名规范)

---

## 1. 控制面通道对比

| 特性 | UDS（本地 CLI） | Kafka（远程 Web CLI） |
|---|---|---|
| 协议 | JSON-RPC 2.0 over Unix Domain Socket | 自定义 JSON，Kafka topic |
| 方向 | 双向（同步请求-响应） | 命令单向发送，响应异步回写到响应 topic |
| 命令集 | 全部 8 条命令 | 全部 8 条命令 |
| 请求格式 | `JSONRPCRequest` | `KafkaCommand` |
| 响应格式 | `JSONRPCResponse` | `KafkaResponse`（写入 `otus-responses`） |
| 目标路由 | 不需要（本机） | `target` 字段按 hostname 路由 |
| 认证 | socket 文件权限 0600，owner-only | Kafka SASL/TLS |
| 超时 | 客户端 10s（可配置） | 调用方自行设置（推荐 30s） |

---

## 2. 本地控制：JSON-RPC over UDS

Socket 路径由 `otus.control.socket` 配置（默认 `/var/run/otus.sock`）。

### 请求格式

```json
{
  "jsonrpc": "2.0",
  "method":  "task_list",
  "params":  {},
  "id":      "req-1740123456789"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `jsonrpc` | `string` | 固定 `"2.0"` |
| `method` | `string` | 命令名，见 [§5 命令参考](#5-命令参考) |
| `params` | `object\|null` | 命令参数，无参数时传 `null` 或 `{}` |
| `id` | `string` | 请求 ID，格式 `"req-{UnixNano}"` |

### 响应格式（成功）

```json
{
  "jsonrpc": "2.0",
  "id":      "req-1740123456789",
  "result":  { ... }
}
```

### 响应格式（错误）

```json
{
  "jsonrpc": "2.0",
  "id":      "req-1740123456789",
  "error": {
    "code":    -32603,
    "message": "create task failed: task already exists"
  }
}
```

> 每次调用独立建立短连接，请求以换行符 `\n` 分隔。

---

## 3. 远程控制：Kafka 命令 topic

**Topic**：`otus.command_channel.kafka.topic`（默认 `otus-commands`）  
**Kafka message key**：必须设为 `target` 字段值，保证同一节点命令落到同一 partition（顺序保障，见 ADR-026）。

### `KafkaCommand` 消息格式

```json
{
  "version":    "v1",
  "target":     "edge-beijing-01",
  "command":    "task_list",
  "timestamp":  "2026-02-21T10:30:00Z",
  "request_id": "req-abc-123",
  "payload":    {}
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `version` | `string` | ✓ | 协议版本，当前 `"v1"` |
| `target` | `string` | ✓ | 目标节点 hostname；`"*"` 广播至所有节点 |
| `command` | `string` | ✓ | 命令名，见 [§5 命令参考](#5-命令参考) |
| `timestamp` | `string` | ✓ | RFC3339 时间戳，超过 `command_ttl`（默认 5m）的命令被丢弃 |
| `request_id` | `string` | ✓ | Correlation ID；为空时不写响应 |
| `payload` | `object\|null` | - | 命令参数，无参数时传 `{}` 或 `null` |

---

## 4. 远程响应：Kafka 响应 topic

**Topic**：`otus.command_channel.kafka.response_topic`（默认 `otus-responses`，ADR-029）  
**Kafka message key**：Agent 的 `hostname`，保证同一节点响应落到固定 partition。

### `KafkaResponse` 消息格式

```json
{
  "version":    "v1",
  "source":     "edge-beijing-01",
  "command":    "task_list",
  "request_id": "req-abc-123",
  "timestamp":  "2026-02-21T10:30:01Z",
  "result":     { "tasks": ["voip-monitor-01"], "count": 1 },
  "error":      null
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `version` | `string` | 协议版本 `"v1"` |
| `source` | `string` | 应答节点的 hostname |
| `command` | `command` | 对应命令名（从请求中回传） |
| `request_id` | `string` | 与命令中的 `request_id` 对应，用于 correlation |
| `timestamp` | `string` | 响应产生时间（RFC3339 UTC） |
| `result` | `object\|null` | 成功时的返回数据，与 `error` 互斥 |
| `error` | `object\|null` | 失败时的错误信息，与 `result` 互斥 |

### 调用方消费规范

```
1. 记录当前 otus-responses 对应 partition 的最新 offset（在发送命令之前）
2. 发送 KafkaCommand，携带唯一 request_id
3. 从记录的 offset 起消费，按 request_id 过滤属于本次请求的响应
4. 超过超时时间（推荐 30s）未收到响应 → 视为节点无响应
```

**Consumer Group 规范**：每个 Web CLI **实例**（进程/Pod）使用唯一 `group_id`，推荐格式：`webcli-{instance-id}`，其中 `instance-id` 从运行环境变量注入（Kubernetes 用 `$POD_NAME`，裸机/VM 用 `$HOSTNAME`），**不得写死**在配置中。
同一实例内的多个并发 session 共享同一 consumer，以 `request_id` 区分各自响应。

| 运行环境 | 推荐来源 | 示例 group_id |
|---|---|---|
| Kubernetes | `POD_NAME`（`valueFrom.fieldRef`） | `webcli-pod-abc12` |
| 虚拟机 / 裸机 | `HOSTNAME` | `webcli-vm-prod-01` |
| 本地开发 | `os.Hostname()` 或启动时一次性 UUID | `webcli-dev-a3f8c` |

**严禁多个实例共享同一 `group_id`**：Kafka partition rebalance 会将 partition 重新分配给同 group 内的不同实例，导致某实例发出请求的响应被另一实例抢读，双方均无法匹配。

---

## 5. 命令参考

所有命令在 UDS（`method` 字段）和 Kafka（`command` 字段）两个通道均可用。

### `task_create` — 创建观测任务

**params / payload**：

```json
{
  "config": { /* TaskConfig，见 §7 */ }
}
```

**result**：

```json
{ "task_id": "voip-monitor-01", "status": "created" }
```

---

### `task_delete` — 删除观测任务

**params / payload**：

```json
{ "task_id": "voip-monitor-01" }
```

**result**：

```json
{ "task_id": "voip-monitor-01", "status": "deleted" }
```

---

### `task_list` — 列出所有任务

**params / payload**：无（`null` 或 `{}`）

**result**：

```json
{ "tasks": ["voip-monitor-01", "http-monitor-02"], "count": 2 }
```

---

### `task_status` — 查询任务状态

**params / payload**（`task_id` 可选，为空返回全部）：

```json
{ "task_id": "voip-monitor-01" }
```

**result**（指定单个）：

```json
{ "task_id": "voip-monitor-01", "status": "running" }
```

**result**（查询全部，`task_id` 为空）：

```json
{
  "tasks": {
    "voip-monitor-01": "running",
    "http-monitor-02": "stopped"
  }
}
```

Task 状态值：`created` | `starting` | `running` | `stopping` | `stopped` | `failed`

---

### `config_reload` — 热加载全局配置

**params / payload**：无

**result**：

```json
{ "status": "reloaded" }
```

> 仅重载全局静态配置（`configs/config.yml`），不影响正在运行的 Task。

---

### `daemon_status` — 查询 Daemon 状态

**params / payload**：无

**result**：

```json
{
  "version":    "0.1.0",
  "uptime_sec": 3600,
  "tasks":      ["voip-monitor-01"],
  "task_count": 1
}
```

---

### `daemon_stats` — 查询运行时统计

**params / payload**：无

**result**：

```json
{
  "tasks": {
    "voip-monitor-01": { "state": "running" }
  }
}
```

---

### `daemon_shutdown` — 触发优雅关闭

**params / payload**：无

**result**：

```json
{ "status": "shutting_down" }
```

> 响应在 daemon 开始关闭流程之前发送。

---

## 6. 错误码

与 JSON-RPC 2.0 规范兼容，同时用于 Kafka 响应的 `error.code` 字段。

| 代码 | 常量 | 含义 |
|---|---|---|
| `-32700` | `ErrCodeParseError` | 无效 JSON，无法解析 |
| `-32600` | `ErrCodeInvalidRequest` | 请求对象不合法 |
| `-32601` | `ErrCodeMethodNotFound` | 方法/命令不存在 |
| `-32602` | `ErrCodeInvalidParams` | 参数类型或格式错误 |
| `-32603` | `ErrCodeInternalError` | 内部执行错误（如 task 创建失败） |

---

## 7. Task 配置模型

通过 `task_create` 命令的 `config` 字段传入，也可以通过 CLI `-f task.yml` 文件传入（支持 JSON/YAML）。

### 完整结构

```yaml
id: "voip-monitor-01"          # 必填，全局唯一
workers: 2                     # Pipeline 数量，默认 1

capture:
  name: "afpacket"             # 必填，捕获插件名
  interface: "eth0"            # 必填，网卡名
  bpf_filter: "udp port 5060"  # BPF 过滤表达式（可选）
  snap_len: 65535              # 最大捕获长度，默认 65535
  dispatch_mode: "binding"     # "binding"（默认）或 "dispatch"
  dispatch_strategy: "flow-hash"  # "flow-hash"（默认）或 "round-robin"
  config:                      # 插件特定配置（透传给插件 Init()）
    fanout_id: 1

decoder:
  tunnels: []                  # 启用的隧道解封装：vxlan | gre | geneve | ipip
  ip_reassembly: false         # 是否启用 IP 分片重组

parsers:
  - name: "sip"                # Parser 插件名
    config:
      track_media: true        # 追踪 RTP/RTCP 流

processors:
  - name: "filter"
    config:
      rules:
        - label: "sip.method"
          values: ["OPTIONS"]
          action: "drop"       # "drop" 或 "keep"

reporters:
  - name: "kafka"
    batch_size: 100            # 批发包数，默认 100
    batch_timeout: "50ms"      # 批发超时，默认 50ms
    fallback: ""               # 备用 reporter 名（可选）
    config:
      brokers: ["kafka:9092"]  # 未设置时继承 otus.reporters.kafka.brokers
      topic: "voip-packets"    # 固定 topic（与 topic_prefix 互斥）
      topic_prefix: ""         # 动态路由前缀，如 "otus" → "otus-sip", "otus-rtp"
      compression: "snappy"    # none | gzip | snappy | lz4
      max_attempts: 3
      serialization: "json"    # json（默认）| binary（Phase 2）

channel_capacity:
  raw_stream: 1000             # per-pipeline 输入 channel
  send_buffer: 10000           # pipeline→sender channel
  capture_ch: 1000             # dispatch 模式中间 channel
```

### 字段说明

#### `capture`

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `name` | `string` | — | 必填，插件名（当前支持 `"afpacket"`） |
| `interface` | `string` | — | 必填，监听网卡名（如 `"eth0"`） |
| `bpf_filter` | `string` | `""` | BPF 过滤器表达式 |
| `snap_len` | `int` | `65535` | 每包最大捕获字节数 |
| `dispatch_mode` | `string` | `"binding"` | `"binding"` 绑定模式，`"dispatch"` 分发模式 |
| `dispatch_strategy` | `string` | `"flow-hash"` | `"flow-hash"` 或 `"round-robin"` |
| `config` | `object` | `{}` | 透传给插件 `Init()` 的插件特定配置 |

#### `reporters[].config`（Kafka Reporter）

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `brokers` | `[]string` | 继承全局 | Kafka broker 地址列表 |
| `topic` | `string` | — | 固定 topic，与 `topic_prefix` 互斥 |
| `topic_prefix` | `string` | — | 动态 topic 前缀，实际 topic = `{prefix}-{payload_type}` |
| `compression` | `string` | `"snappy"` | `none` \| `gzip` \| `snappy` \| `lz4` |
| `max_attempts` | `int` | `3` | 发送失败重试次数 |
| `serialization` | `string` | `"json"` | `"json"` 或 `"binary"`（Phase 2） |
| `batch_size` | `int` | `100` | 批量发送包数 |
| `batch_timeout` | `string` | `"100ms"` | 批量发送超时（Go duration 格式） |

---

## 8. 全局配置模型

文件路径由 CLI `--config` 指定（默认 `configs/config.yml`）。  
所有字段均在 `otus:` 根 key 下。环境变量优先，格式：`OTUS_` + 点路径全大写（如 `otus.log.level` → `OTUS_LOG_LEVEL`）。

```yaml
otus:

  # ── 节点标识 ──
  node:
    ip: ""                      # 空 = 自动探测；优先级：配置/env > 自动探测 > 报错（ADR-023）
    hostname: ""                # 空 = os.Hostname()
    tags:
      datacenter: "dc1"
      environment: "production"

  # ── 本地控制 ──
  control:
    socket: "/var/run/otus.sock"
    pid_file: "/var/run/otus.pid"

  # ── Kafka 全局默认（ADR-024）──
  # command_channel.kafka 和 reporters.kafka 在各自字段为空时自动继承
  kafka:
    brokers: ["localhost:9092"]
    sasl:
      enabled: false
      mechanism: "PLAIN"        # PLAIN | SCRAM-SHA-256 | SCRAM-SHA-512
      username: ""
      password: ""
    tls:
      enabled: false
      ca_cert: ""
      client_cert: ""
      client_key: ""
      insecure_skip_verify: false

  # ── 远程命令通道 ──
  command_channel:
    enabled: false
    type: "kafka"               # 目前仅支持 "kafka"
    kafka:
      topic: "otus-commands"
      response_topic: "otus-responses"  # 空字符串 = 禁用响应（ADR-029）
      group_id: ""              # 空 = "otus-{hostname}"
      auto_offset_reset: "latest"  # "latest"（仅处理启动后命令）或 "earliest"
    command_ttl: "5m"           # 超过此时间的命令被丢弃（ADR-026）

  # ── 共享 Reporter 连接配置 ──
  reporters:
    kafka:
      compression: "snappy"    # none | gzip | snappy | lz4
      max_message_bytes: 1048576

  # ── 资源上限 ──
  resources:
    max_workers: 0             # 0 = GOMAXPROCS

  # ── 背压控制（ADR-001）──
  backpressure:
    pipeline_channel:
      capacity: 65536
      drop_policy: "tail"      # 满时丢弃新包
    send_buffer:
      capacity: 16384
      drop_policy: "head"      # 满时丢弃最旧数据
      high_watermark: 0.8
      low_watermark: 0.3
    reporter:
      send_timeout: "3s"
      max_retries: 1

  # ── 核心解码器 ──
  core:
    decoder:
      tunnel:
        vxlan: false
        gre: false
        geneve: false
        ipip: false
      ip_reassembly:
        timeout: "30s"
        max_fragments: 10000

  # ── Prometheus 指标 ──
  metrics:
    enabled: true
    listen: ":9091"
    path: "/metrics"
    collect_interval: "5s"

  # ── 日志 ──
  log:
    level: "info"              # debug | info | warn | error
    format: "json"             # json | text
    outputs:
      file:
        enabled: true
        path: "/var/log/otus/otus.log"
        rotation:
          max_size_mb: 100     # ADR-025：单位编码在字段名中
          max_age_days: 30
          max_backups: 5
          compress: true
      loki:
        enabled: false
        endpoint: "http://loki:3100/loki/api/v1/push"
        labels:
          app: "otus"
        batch_size: 100
        batch_timeout: "1s"

  # ── Task 持久化（ADR-030, ADR-031）──
  data_dir: "/var/lib/otus"   # 顶级数据目录；task 记录存储于 {data_dir}/tasks/
  task_persistence:
    enabled: true             # false = 禁用持久化（开发 / 单测场景）
    auto_restart: true        # 重启后自动恢复 running/starting/stopping 状态的 task
    gc_interval: "1h"         # 进程内 GC 触发间隔（清理超出 max_task_history 的终态记录）
    max_task_history: 100     # 终态（stopped/failed）记录最大保留数；0 = 不触发进程内 GC
```

### 字段说明

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `data_dir` | `string` | `/var/lib/otus` | 顶级数据目录，task 状态文件存放于 `{data_dir}/tasks/` |
| `task_persistence.enabled` | `bool` | `true` | `false` 时所有持久化操作降级为 no-op |
| `task_persistence.auto_restart` | `bool` | `true` | Daemon 启动时是否自动重建上次处于 running/starting/stopping 状态的 task |
| `task_persistence.gc_interval` | `string` | `1h` | 进程内 GC goroutine 的触发间隔（Go duration 格式） |
| `task_persistence.max_task_history` | `int` | `100` | 终态（stopped / failed）task 记录的保留上限；超出则按 created_at 升序删除旧记录；`0` = 禁用 |

> **目录初始化**：由 `ExecStartPre=systemd-tmpfiles --create /etc/tmpfiles.d/otus.conf` 负责创建目录并设置权限（ADR-031）。不需要手动 `mkdir`。

---

### 配置继承规则（ADR-024）

```
otus.kafka.brokers
  ├── → command_channel.kafka.brokers（当后者为空时）
  └── → reporters.kafka.brokers（当后者为空时）

otus.kafka.sasl  →  同上（仅当子节点 sasl.enabled=false 且全局 sasl.enabled=true 时）
otus.kafka.tls   →  同上
```

---

## 9. 上报数据结构

数据由 Reporter 插件发送，结构因 Reporter 类型而异。

### 9.1 Kafka Reporter 消息格式（ADR-028）

**Kafka Headers**（Envelope 元数据）：

| Header Key | 值类型 | 说明 |
|---|---|---|
| `task_id` | `string` | 所属 Task ID |
| `agent_id` | `string` | Agent hostname |
| `payload_type` | `string` | 协议类型：`sip` \| `rtp` \| `raw` |
| `src_ip` | `string` | 源 IP |
| `dst_ip` | `string` | 目标 IP |
| `src_port` | `string` | 源端口（数字字符串） |
| `dst_port` | `string` | 目标端口（数字字符串） |
| `timestamp` | `string` | Unix 毫秒时间戳（数字字符串） |
| `l.{label_key}` | `string` | Labels，以 `l.` 前缀区分（如 `l.sip.method`） |

**Kafka message key**：`{src_ip}:{src_port}-{dst_ip}:{dst_port}`（用于一致性分区路由）

**Kafka message Value**（JSON 模式，`serialization: "json"`）：

```json
{
  "task_id":         "voip-monitor-01",
  "agent_id":        "edge-beijing-01",
  "pipeline_id":     0,
  "timestamp":       1740123456789,
  "src_ip":          "192.168.1.10",
  "dst_ip":          "10.0.0.1",
  "src_port":        5060,
  "dst_port":        5060,
  "protocol":        17,
  "payload_type":    "sip",
  "labels": {
    "sip.method":    "INVITE",
    "sip.call_id":   "abc123@192.168.1.10",
    "sip.from_uri":  "sip:alice@example.com",
    "sip.to_uri":    "sip:bob@example.com",
    "sip.status_code": ""
  },
  "raw_payload_len": 512,
  "raw_payload":     "SEVMTE8gV09STEQ=",
  "payload":         { /* 解析后的协议结构体，payload_type=raw 时为 null */ }
}
```

**Kafka message Value 字段说明**：

| 字段 | 类型 | 说明 |
|---|---|---|
| `task_id` | `string` | 所属 Task ID |
| `agent_id` | `string` | Agent hostname |
| `pipeline_id` | `int` | 产生该包的 Pipeline 编号 |
| `timestamp` | `int64` | Unix 毫秒时间戳 |
| `src_ip` / `dst_ip` | `string` | 源/目标 IP |
| `src_port` / `dst_port` | `int` | 源/目标端口 |
| `protocol` | `int` | IP 协议号（6=TCP, 17=UDP） |
| `payload_type` | `string` | 协议类型：`sip` \| `rtp` \| `raw` |
| `labels` | `object` | 解析器/处理器提取的 Labels 键值对 |
| `raw_payload_len` | `int` | 原始载荷字节数，便于统计和告警 |
| `raw_payload` | `string` | 原始载荷的 base64 编码；`payload_type=raw` 时包含完整数据 |
| `payload` | `object\|null` | 解析后的协议结构体（如 SIP 字段树）；`payload_type=raw` 或解析失败时为 `null` |

### 9.2 动态 Topic 路由（ADR-027）

| 配置 | 实际 Topic | 示例 |
|---|---|---|
| `topic: "voip"` | 固定 `voip` | 所有包写同一 topic |
| `topic_prefix: "otus"` | `otus-{payload_type}` | `otus-sip`, `otus-rtp`, `otus-raw` |

---

## 10. Labels 命名规范

格式：`{protocol}.{field}`，小写，点分级（ADR-012）。  
消费 Kafka 消息时，Labels 出现在 Headers（前缀 `l.`）和 Value（`labels` 字段）两处。

### SIP Labels

| Key | 说明 | 示例值 |
|---|---|---|
| `sip.method` | SIP 方法（Request）或空（Response） | `INVITE`, `BYE`, `OPTIONS` |
| `sip.call_id` | Call-ID 头部 | `abc123@192.168.1.10` |
| `sip.from_uri` | From 头部 URI | `sip:alice@example.com` |
| `sip.to_uri` | To 头部 URI | `sip:bob@example.com` |
| `sip.status_code` | 响应状态码（Response）或空（Request） | `200`, `404`, `180` |
| `sip.via` | Via 头部（逗号分隔列表） | `SIP/2.0/UDP proxy1.example.com` |

### 扩展 Labels（由 Processor 标注）

Processor 插件可添加任意 `{protocol}.{field}` 格式的 Labels，遵循同一命名规范。

---

**文档版本**: v1.2.0  
**更新日期**: 2026-02-22  
**对应代码**: `internal/command/`, `internal/config/`, `internal/task/`, `plugins/reporter/kafka/`

**变更历史**

| 版本 | 日期 | 说明 |
|---|---|---|
| v1.2.0 | 2026-02-22 | 新增 §8 `data_dir` + `task_persistence` 字段说明（ADR-030/031）；§9.1 补充 `raw_payload` / `payload` 字段 |
| v1.1.0 | 2026-02-21 | §9.1 Kafka 消息格式 Bug 修复：`raw_payload_len` 先前仅记录长度，现已修复为同时输出字节内容（base64） |
| v1.0.0 | 2026-02-21 | 初始版本 |
