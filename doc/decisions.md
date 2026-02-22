# Otus - 架构决策记录 (Architecture Decision Records)

本文档记录 Otus 项目在架构设计过程中的关键决策讨论、备选方案分析和最终结论。
每个决策包含完整的上下文和推理过程，便于后续回溯和审查。

---

## ADR-001: 背压控制与丢弃策略

**状态**: 已决定  
**日期**: 2026-02-13  
**关联章节**: 架构文档 5.4

### 背景

在 10Gbps 高性能抓包场景下，捕获速率恒定（线速），但下游消费速率不可控。当 Reporter（如 Kafka）出现网络问题时（ACK 迟迟不来），pipeline 积压会传导至 Ring Buffer，导致 Capture 驱动写入阻塞。需要决定如何处理这种反压。

### 核心问题

丢弃策略应该在哪一层执行？丢弃未写入 Ring Buffer 的包，还是丢弃 Kafka 未确认的数据？

### 备选方案

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| A. 全链路阻塞 | 下游慢时阻塞上游 | 不丢数据 | 捕获层被阻塞，内核开始丢包且不可统计 |
| B. 仅在 Reporter 层丢弃 | 下游慢只影响 Reporter | 简单 | 如果上游也积压，pipeline channel 也会满 |
| C. **分层非阻塞 + 分层丢弃** | 每一层都非阻塞，各自有丢弃策略 | 完全保护捕获层，资源可预测 | 实现稍复杂 |
| D. 全链路丢弃 + 动态采样 | 在 C 基础上增加反馈降级 | 优雅降级 | 丢失细节 |

### 业界参考

- **tcpdump / libpcap**：ring buffer 写满直接覆盖（内核侧 drop）
- **Cilium Hubble**：channel 满时 drop 新事件，记录 `lost_events` 计数器
- **Elastic PacketBeat**：output 队列满时丢弃新事件
- **heplify（Homer）**：发送失败直接丢弃，不重试不缓存
- **OpenTelemetry Collector**：exporterhelper 内置 `sending_queue`，满了直接拒绝

### 决定

采用 **方案 C：分层非阻塞 + 分层丢弃**，核心原则是**永远保护捕获层，牺牲上报层**。

四层丢弃策略：

1. **内核 Ring Buffer**：内核自动丢（不可控），只统计 `tp_drops`
2. **Pipeline Channel**：有界 channel，满时 **drop-tail**（丢弃新包），非阻塞写入
3. **Send Buffer**：每 reporter 独立有界队列，满时 **drop-head**（丢弃最旧数据，因为越新越有观测价值）
4. **Reporter**：ACK 超时（3s）直接丢弃，不做无限重试。at-most-once 语义即可

动态采样降级作为可选高级策略，默认不启用。

### 理由

- 抓包观测数据不是交易数据，不需要 exactly-once 语义
- AF_PACKET/XDP 的 mmap ring buffer 由内核管理，用户态消费不过来时内核会直接丢弃，这一层不可控
- 阻塞 Capture 层的后果是内核侧开始丢包，且完全不可统计——最坏的情况
- 每一层都需要对应的 metrics，否则运维无法感知丢失

---

## ADR-002: L2-L4 协议栈解码归属——核心代码 vs 插件

**状态**: 已决定  
**日期**: 2026-02-13  
**关联章节**: 架构文档 5.2

### 背景

从网卡捕获的原始帧包含 L2（以太网）到 L4（TCP/UDP）的头部数据。需要决定这部分解码是做成插件还是归入核心代码。

### 备选方案

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| A. **L2-L4 解码作为核心代码** | 固化在核心引擎中 | 性能最优，无接口调度开销 | 不可扩展 |
| B. L2-L4 解码作为插件 | 通过插件接口加载 | 可插拔替换 | 每包多几十ns开销，10Gbps下不可接受 |

### 决定

采用 **方案 A：L2-L4 解码作为核心代码**。

分界线明确：
- **核心代码**：`Raw Frame → Ethernet → IP → TCP/UDP → Application Payload`
- **插件**：`Application Payload → SIP Message / RTP Packet / HTTP Request`

### 理由

1. **稳定性**：以太网帧头、IPv4/IPv6、TCP/UDP 头部格式几十年不变，没有"可插拔"需求
2. **性能**：10Gbps 下每包都过 L2-L4 解码，插件接口的调度开销在百万 pps 下不可接受
3. **普遍性**：所有目标协议（SIP/RTP/RTCP/WebRTC/HTTP/WS）都在 TCP/IP 之上
4. **确定性**：核心输出边界清晰—— `(src_ip, dst_ip, src_port, dst_port, protocol, payload)`

### RoCE/RDMA 网络备注

RoCE v2 网络上的 RDMA 流量走内核旁路（kernel bypass），AF_PACKET/XDP 挂在内核协议栈上，天然抓不到 RDMA 流量。目标协议（SIP/RTP/HTTP 等）仍走正常内核协议栈，不受影响。这是已知约束，不影响架构设计。

---

## ADR-003: L2 层封装处理范围

**状态**: 已决定  
**日期**: 2026-02-13  
**关联章节**: 架构文档 5.2.2, 5.2.3

### 背景

不同部署环境下，以太网帧可能被各种封装协议包裹（VLAN、VXLAN、GRE 等）。需要决定哪些封装要处理，以及是否做成可配置。

### 部署环境封装分析

**物理机/裸金属**：
| 封装 | EtherType | 出现概率 |
|------|-----------|----------|
| 802.1Q VLAN | `0x8100` | 高 |
| QinQ (802.1ad) | `0x88A8` | 中（运营商） |

**云/虚拟化环境**：
| 封装 | 特征 | 出现概率 |
|------|------|----------|
| VXLAN | UDP:4789 | 高 |
| GRE/ERSPAN | IP Proto 47 | 中 |
| Geneve | UDP:6081 | 中 |
| IPIP | IP Proto 4 | 中 |

**关键发现**：是否看到隧道封装取决于抓包位置：
- 虚拟机内 eth0 / K8s Pod 内：hypervisor/CNI 已剥离封装，**不需要解封装**
- 物理网卡 / 旁路镜像：保留原始封装，**需要解封装**

### 决定

分两个层次处理：

**L2（常开，零配置）**：
- 标准以太网帧解析
- 802.1Q VLAN 剥离（自动识别，记录 VLAN ID）
- QinQ 剥离（递归处理）

**L2.5 隧道解封装（可配置开关，默认关闭）**：
- VXLAN、GRE/ERSPAN、Geneve、IPIP
- 解封装后重新进入 L2 Decode 递归处理
- 记录隧道外层元信息（OuterSrcIP、OuterDstIP、VNI）用于溯源

### 理由

- VLAN/QinQ 极其普遍（跳过 4 字节即可），解析代价为零，常开
- 隧道解封装仅在特定部署位置需要，默认关闭避免误判
- 所有封装解析均为无状态的固定偏移头部剥离，每种 15-50 行代码，总计不超过 200 行，性能零影响

---

## ADR-004: IP 分片重组策略

**状态**: 已决定  
**日期**: 2026-02-13  
**关联章节**: 架构文档 5.2.4  
**实施阶段**: Phase 1

### 背景

IP 分片在 SIP over UDP 场景中是刚需——一个带 SDP 的 INVITE 可达 2000+ 字节，超过 1500 MTU 后产生分片。不重组则 SDP 被截断，无法提取媒体地址和端口，直接影响按需捞取 RTP 的核心业务能力。

### 决定

IP 分片重组是核心代码，**常开，不可配置关闭**。

### 子决策点

#### ADR-004a: 分片重组攻击防护

**问题**：IP 分片重组是经典 DoS 攻击面（永不完成的分片组、重叠分片 Teardrop、分片洪泛）。

**备选方案**：
| 方案 | 描述 |
|------|------|
| A. 完整防护（如 Linux 内核） | 重叠检测、内存上限、per-source 限速 |
| B. **简单硬上限 + 超时** | `max_fragments` 上限 + 30s 超时清理 |

**决定**：采用 **方案 B**。

**理由**：Otus 是被动观测程序，不暴露网络服务，攻击者不知道有抓包程序在运行。不存在被定向攻击的可能。简单上限 + 超时足以防止资源泄漏。

#### ADR-004b: 分片重组与零拷贝的矛盾

**问题**：正常包走零拷贝路径（`Payload` 直接指向 mmap 内存），但分片重组必须将多个分片拷贝到新内存拼接，两条路径不一致。

**备选方案**：
| 方案 | 描述 |
|------|------|
| A. 为重组包分配独立内存池 | 避免 GC 压力 |
| B. **直接 `make([]byte)` 分配** | 简单实现 |

**决定**：采用 **方案 B**，Phase 1 直接分配。

**理由**：分片在总流量中占比极低（< 0.1%），分配开销可忽略。预留优化空间，如果后续 profiling 显示 GC 压力大，Phase 2 可引入内存池。

---

## ADR-005: TCP 流重组策略

**状态**: 已决定  
**日期**: 2026-02-13  
**关联章节**: 架构文档 5.2.5  
**实施阶段**: Phase 2

### 背景

目标协议中 SIP over TCP、SIP over WebSocket、HTTP、WebSocket 都需要 TCP 流重组才能正确解析应用层消息。但全流量 TCP 重组在 10Gbps / ≤2vCPU / ≤512MB 约束下不可行。

### 决定

**选择性 TCP 重组**：不做全流量重组，只对匹配特定端口规则的 TCP 流做重组。默认关闭，按需开启。

### 子决策点

#### ADR-005a: 自研 vs 开源库

**问题**：TCP 重组引擎选型。

**备选方案**：
| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| A. gopacket/tcpassembly | Google 维护的开源实现 | 功能完整，社区验证 | 内存模型不够高效 |
| B. 自研轻量重组器 | 只做需要的功能 | 深度优化 | 开发成本高 |
| C. CGo 调用 libnids/libndpi | 成熟 C 库 | 稳定 | CGo 开销 + 跨语言复杂 |

**决定**：采用 **方案 A** 作为起点，但**必须做好接口抽象**。

**关键约束**：TCP 重组引擎必须隐藏在接口后面，外部代码通过接口调用，底层实现的替换不能让影响穿透接口。Phase 2 先用 gopacket/tcpassembly 验证正确性，如果后续 profiling 显示成为瓶颈，可无缝替换为自研实现。

```go
// 接口定义（核心代码拥有）
type TCPReassembler interface {
    // 输入一个 TCP 包
    HandlePacket(packet *DecodedPacket) error
    // 注册应用层数据回调
    OnStreamData(callback func(streamID uint64, data []byte, seq uint64))
    // 注册流关闭回调
    OnStreamClose(callback func(streamID uint64))
    // 资源统计
    Stats() ReassemblyStats
    // 关闭并释放资源
    Close() error
}
```

#### ADR-005b: TCP 重组输出粒度——流 vs PDU

**问题**：TCP 是字节流，没有消息边界。重组后输出什么？

**备选方案**：
| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| A. **输出有序字节流片段** | 每次收到数据就输出当前有序字节，插件自行分帧 | 核心不耦合应用协议 | 插件需维护分帧状态 |
| B. 核心做协议分帧 | 核心感知 SIP/HTTP 消息边界，输出完整 PDU | 插件简单 | 核心膨胀，耦合应用协议知识 |

**决定**：采用 **方案 A**。

**理由**：
- 核心代码不应耦合应用协议知识
- 分帧逻辑各协议差异很大（SIP vs HTTP/1.1 vs HTTP/2 vs WebSocket），放核心会膨胀
- 对 `DecodedPacket` 的影响：TCP 重组模式下 `Payload` 语义是"流中的一段有序字节"而非"完整消息"
- 应用层解析插件需要具备从字节流中间同步到消息边界的能力

#### ADR-005c: Mid-stream Join（中途加入已有连接）

**问题**：如果 Otus 启动时 TCP 连接已存在（看不到 SYN），如何处理？

**备选方案**：
| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| A. 丢弃 mid-stream 流 | 只处理观察到完整 SYN 的连接 | 实现最简单 | **长连接场景下大量数据丢失** |
| B. **Mid-stream join** | 以首个观察到的 seq 做相对基准 | 消除观测盲区 | 首条消息可能截断 |

**决定**：采用 **方案 B：Mid-stream Join**。

**关键分析——为什么不能丢弃 mid-stream 流**：

SIP over TCP（trunk 长连接）、WebSocket 等连接可能存活数小时甚至数天。丢弃 mid-stream 流意味着：
- Otus 每次 start / reload 后，所有存量 TCP 连接都是 mid-stream
- SIP trunk 长连接场景下，可能几小时内整条线路的信令全部丢失
- 运维无法感知"为什么某些呼叫看不到信令"

| 协议 | 连接存活时间 | 丢弃 mid-stream 影响 |
|------|-------------|---------------------|
| SIP/TCP（短连接） | 秒级 | 小 |
| SIP/TCP（trunk 长连接） | 小时~天 | **严重** |
| HTTP/1.1 Keep-Alive | 分钟级 | 中等 |
| WebSocket（SIP over WS） | 小时级 | **严重** |

**Mid-stream join 工作方式**：
1. 首次观察到未知流的包时，记录当前 seq 作为相对基准（relative seq = 0）
2. 从此刻起按序输出该流的后续数据
3. 第一个输出的字节流片段可能不完整（截断在消息中间）
4. 应用层插件需容忍开头的"半截消息"并跳过——通过同步能力找到下一个消息起始边界

**各协议同步能力评估**：
| 协议 | 能否从中间同步 | 方法 |
|------|:------------:|------|
| SIP | 能 | 扫描 `\r\n` 后找方法行（INVITE/REGISTER/SIP/2.0） |
| HTTP/1.1 | 能 | 扫描 `\r\n` 后找 GET/POST/HTTP/1.1 |
| WebSocket | 较难 | 帧头无固定 magic，可能卡住 |
| HTTP/2 | 能 | 有 24-bit magic prefix + frame header |

**对 WebSocket 的特殊处理**：WebSocket 的 mid-stream join 同步困难，可考虑对 WebSocket 流不做 mid-stream join，作为 per-port 的可配置策略。

#### ADR-005d: TCP 连接生命周期管理

**决定**：**不做完整 TCP 状态机，纯靠超时驱逐**。

- 允许 mid-stream join（不依赖 SYN）
- FIN/RST 丢失不影响清理逻辑
- `stream_timeout` 超时后自动淘汰连接
- RST 伪造对被动观测影响有限（最多丢失一个连接的后续数据）

#### ADR-005e: 乱序和重传处理

**决定**：
- **乱序**：要处理，但抓包点通常是本机网卡，乱序比例极低
- **重传**：要去重，否则应用层收到重复数据
- **空洞（gap）**：设短超时（5s），超时后跳过空洞继续输出（标记 gap），**不能无限等**

#### ADR-005f: TCP 重组内存上限

**问题**：`max_concurrent_streams × per_stream_buffer_limit` 理论最大值可能远超 512MB 内存预算。

**决定**：采用**全局内存池 + LRU 淘汰**。

```yaml
tcp_reassembly:
  max_concurrent_streams: 10000
  per_stream_buffer_limit: 32KB
  global_memory_limit: 128MB    # 硬性全局上限，优先于 per-stream 计算
  stream_timeout: 120s
  overflow_policy: drop_oldest  # LRU 淘汰最旧连接
```

当 `global_memory_limit` 触达时，按 LRU 淘汰最旧的流，释放空间。

---

### ADR-006: Pipeline 并行模型

**问题**：如何利用多 vCPU 提升吞吐？固定 2 vCPU 上限，还是自适应多核扩展？

**结论**：**每核一条 Pipeline，线性水平扩展**。

**理由**：

1. 捕获本身不是瓶颈（mmap zero-copy），**内核到用户态拷贝 + 解码/解析才是 CPU 消耗主体**，M >= N worker 是正确方向
2. 固定 vCPU 上限人为制造天花板，不符合 edge 节点异构硬件现实
3. 同一五元组的包必须到同一 pipeline（有状态解析、TCP 重组依赖此保证）
4. 流量分发下推至内核/硬件，用户态零开销：
   - AF_PACKET: `PACKET_FANOUT` (FANOUT_HASH / FANOUT_CPU)
   - XDP: NIC RSS 硬件分流到多 RX queue
   - pcap: 仅测试场景，用户态 flow-hash dispatcher

**设计要点**：

- `pipeline.count` 默认 = `runtime.GOMAXPROCS(0)`（auto），受 driver `MaxStreams()` 上限约束
- Pipeline 之间不共享可变状态，天然无锁
- per-Task 跨 pipeline 共享结构：FlowRegistry（sync.Map，读多写少，接近无锁）
- 每条 pipeline 单 goroutine 主循环，decode → canHandle → handle → process → send buffer write 全在同一 goroutine

---

### ADR-007: Parser 两阶段接口设计

**问题**：Parser 是否需要核心提供 FastForward / DeepParse 等快慢路径概念？

**结论**：**不需要。核心只定义 CanHandle + Handle 两个方法，快慢路径由 Parser 内部自行决定。**

**理由**：

1. 核心引擎完全协议无关，不应知道"什么是快""什么是慢"
2. FastForward 作为核心概念会导致接口膨胀（FastForward/SlowParse/Parse 三个 action），且核心需要理解每种协议的快慢逻辑
3. Parser 自己最清楚何时能走捷径（如 SIP parser 查 FlowRegistry 命中 → 直接贴标签返回），何时需要深度解析
4. 两方法接口足够简洁且强大：
   - `CanHandle(DecodedPacket) bool`：路由判断，O(1) 运算
   - `Handle(DecodedPacket) *OutputPacket`：全权处理，内部自决快慢
5. 引擎只管遍历 parser 列表，首个 CanHandle=true 的 parser 独占处理

**接口定义**：

```go
type Parser interface {
    CanHandle(pkt *DecodedPacket) bool
    Handle(pkt *DecodedPacket) *OutputPacket
}

// 需要跨 pipeline 查询流表的 parser 实现此接口
type FlowRegistryAware interface {
    SetFlowRegistry(registry FlowRegistry)
}
```

---

### ADR-008: FlowRegistry 作为核心基础设施

**问题**：信令-媒体流关联（如 SIP SDP 解析出 RTP 五元组）需要跨 pipeline 共享状态，如何设计？

**结论**：**FlowRegistry 作为核心提供的 per-Task 跨 pipeline 共享结构**（见 ADR-014）。

**理由**：

1. 信令/媒体关联是多种协议的共性需求（SIP→RTP, H.248→RTP, MGCP→RTP），不是个别 parser 的私事
2. 写入低频（信令量 << 媒体量），读取高频（每个媒体包都要查），sync.Map 是最合适的实现
3. 作为核心基础设施，所有 parser 通过 `FlowRegistryAware` 接口获取引用，统一管理生命周期
4. TTL-based 过期 + 手动 Unregister 双重清理，防止内存泄漏

**接口**：

```go
type FlowRegistry interface {
    Register(flow FiveTuple, ctx FlowContext, ttl time.Duration)
    Lookup(flow FiveTuple) (FlowContext, bool)
    Unregister(flow FiveTuple)
}
```

---

### ADR-009: 性能度量模型

**问题**：如何衡量和承诺系统性能？固定 "2 vCPU 处理 1Gbps" 还是其他方式？

**结论**：**以 packets/sec/core 为核心指标，线性扩展模型**。

**理由**：

1. 固定 vCPU 承诺（如 "2 vCPU 处理 1Gbps"）隐含包大小、协议分布等假设，实际偏差大
2. "每核吞吐"是可测量、可复现的指标，benchmark 跑出来就是硬数据
3. 线性扩展意味着：给 N 核 → 获得 N × 每核吞吐，用户按需评估资源

**性能目标**：

| 路径 | 目标 pps/core | 典型场景 |
|------|-------------|---------|
| Fast path（CanHandle 查表命中，Handle 贴标签） | ≥ 2M | 已知 RTP 流，SIP 中转 |
| Slow path（Handle 深度解析） | ≥ 200K | SIP INVITE/200OK, 首次建连 |

**扩展示例**：

| vCPU | 预期快路径吞吐 | 预期慢路径吞吐 |
|------|--------------|--------------|
| 1 | 2M pps | 200K pps |
| 4 | 8M pps | 800K pps |
| 8 | 16M pps | 1.6M pps |
| 16 | 32M pps | 3.2M pps |

---

### ADR-010: Pipeline 数据契约

**问题**：Pipeline 各阶段（Capture → Core → Parser → Processor → Reporter）之间用什么数据结构传递？如何保证不同插件可组合？

**决定**：**三层数据结构，Envelope + Typed Payload 模式**。

**理由**：

1. **RawPacket**（第 1 层）：Capture 驱动产出，核心私有，mmap 零拷贝 slice，插件不直接接触
2. **DecodedPacket**（第 2 层）：核心 Decoder 产出，Parser 只读。L2-L4 字段固定且标准化，核心自己定义无争议
3. **OutputPacket**（第 3 层）：跨插件边界的公共契约，核心矛盾在这里解决：
   - Parser 产出的 Payload 是**协议相关**的（SIPPayload, RTPPayload...）
   - Processor 和 Reporter 应**协议无关**——不应被迫 import 具体协议类型
4. 解决方案：Envelope（固定字段 + Labels map）所有插件可读写，Payload 用接口封装协议细节，Reporter 通过 MarshalJSON/MarshalBinary 序列化，无需 type assert

**OutputPacket 结构**：

```go
type OutputPacket struct {
    Timestamp  time.Time           // Envelope
    Protocol   string
    FlowID     FiveTuple
    Network    NetworkMeta
    Labels     map[string]string   // Parser 写入，Processor 读写，Reporter 读
    Payload    Payload             // Parser 写入，Processor 不碰，Reporter 序列化
    RawBytes   []byte              // 可选：原始帧引用
}
```

---

### ADR-011: Processor 职责边界

**问题**：Edge 采集端的 Processor 应该做多少事？

**决定**：**Processor 严格限于过滤 + 轻量标注。不碰 Payload，不碰 RawBytes。计算任务后移至下游。**

**理由**：

1. Edge 采集端占用宿主计算资源，应最小化本地计算，只做收益巨大的操作
2. 收益巨大的操作只有两类：
   - **过滤**：决定谁上报谁不上报，直接减少网络带宽和下游负载（如丢弃 SIP OPTIONS/REGISTER）
   - **轻量标注**：补充部署元数据（节点名、数据中心、环境标签），下游消费时需要但采集时才知道
3. 跨协议关联（如 VoIP 场景 SIP→RTP 关联）在 **Parser 层通过 FlowRegistry 完成**，CanHandle 路由本身就查 FlowRegistry，顺手带出关联信息是零成本
4. 数据聚合、脱敏、复杂计算等操作应后移至下游计算平台

**Processor 只能做**：
- 读写 Labels
- 读 Envelope（Protocol/FlowID/Network）
- 返回 nil 表示丢弃

**Processor 不能做**：
- 访问 Payload 内部（不允许 type assert）
- 修改 RawBytes
- 任何 IO 操作

---

### ADR-012: Labels 命名规范

**问题**：Labels 是 Parser→Processor 的唯一通信契约，需要防止命名冲突和歧义。

**决定**：**分级命名：协议字段 `{protocol}.{field}`，关联/元数据无前缀**。

**理由**：

1. 带协议前缀的字段（`sip.method`、`rtp.ssrc`）不会和部署元数据（`node`、`dc`）冲突
2. 跨协议关联字段（`call-id`、`codec`）无前缀，因为它们是多个协议共享的概念，不属于任何单一协议
3. Processor 的 filter 规则可以清晰引用：`label: "sip.method"` 或 `label: "call-id"`
4. Key 格式约束 `[a-z0-9][a-z0-9.-]*`（小写 + 点 + 短横线），与 Prometheus labels、Kafka headers 兼容
5. Value 统一为 string，足够覆盖过滤/路由/标记场景；结构化数据属于 Payload 的职责

---

### ADR-013: 两层配置模型

**问题**：配置应该全部静态（配置文件）还是支持动态？Reporter topic 写死在配置文件中不合理，外部系统应能通过 API 下发观测任务。

**决定**：**全局静态配置（配置文件）+ 任务动态配置（Kafka 命令 topic 或本地 CLI 下发）**。

**理由**：

1. 系统需要支持外部调用启动观测任务，任务的 BPF 过滤、Parser 链、Reporter topic 等都应**动态传入**
2. 但某些配置不会随任务变化：节点 IP、UDS 地址、Kafka 命令通道配置、Reporter 连接参数（brokers/endpoint）、背压策略——这些属于**全局静态**
3. 两层分离让 Task 配置保持精简：Reporter 只需写 `type: kafka` + `topic: xxx`，连接细节由全局配置提供
4. 全局配置支持 SIGHUP 热加载，但不影响正在运行的 Task

**全局静态（配置文件）**：节点信息、UDS 控制地址、Kafka 命令通道配置、Metrics 监听地址、Reporter 连接配置、背压参数、解码器配置、资源上限

**任务动态（Kafka 命令 / CLI）**：Capture 驱动/网卡/workers、BPF 过滤规则、Parser/Processor 链、Reporter 业务参数（topic）、unmatched_policy

---

### ADR-014: Task-Pipeline 绑定与 FlowRegistry 作用域

**问题**：Pipeline 是全局的还是绑定到观测任务？FlowRegistry 是全局共享还是 per-Task？

**决定**：
- **Pipeline 是 Task 的执行单元**，每个 Task 拥有独立的 N 条 Pipeline
- **FlowRegistry 作用域为 per-Task**
- **Phase 1 仅支持 1 个活跃 Task**，workers 不足时拒绝创建

**理由**：

1. 不同观测任务抓不同流量、用不同 Parser、发不同 Reporter，共享 Pipeline 没有意义
2. FlowRegistry per-Task 而非全局共享：
   - Task A 的 SIP 通话关联和 Task B 的 DNS 监控完全无关，共享会导致误匹配
   - Task 销毁时直接丢弃整个 FlowRegistry，无需逐条清理
   - 隔离防止一个任务的 FlowRegistry 泄漏影响另一个
3. Phase 1 单 Task 简化实现：无需考虑多 Task 资源竞争、同网卡多 BPF 共存等复杂问题
4. Phase 2 扩展多 Task 时，只需增加资源配额管理，核心 Task 模型不变

---

### ADR-015: 远程控制拉模式（Kafka 命令 topic）

**问题**：远程控制通道采用推模式（Agent 监听 gRPC 端口）还是拉模式（Agent 主动拉取命令）？

**决定**：**拉模式——Agent 订阅 Kafka 命令 topic 接收任务指令，不开放任何入站端口。gRPC 推模式推迟到 Phase 2+ 按需决策。**

**理由**：

1. 边缘节点网络环境复杂——防火墙、NAT、安全策略普遍禁止入站端口，推模式要求宿主机/容器开放 gRPC 监听端口，部署阻力大
2. Kafka 已是系统依赖（Reporter 上报数据用 Kafka），复用 Kafka 做命令通道**零新增依赖**
3. 拉模式天然适合异步控制：Control Plane 发布命令到 topic，Agent 上线后自动消费，支持离线重连后补偿
4. `group_id` 按节点隔离（`otus-${node.id}`），`target` 字段按节点路由，确保消息精准投递
5. 本地调试通过 CLI + Unix Domain Socket 解决，不依赖远程通道

**命令通道设计**：
- 命令 topic：`otus-commands`，JSON 格式，`target` 字段路由
- 本地控制：CLI → Unix Domain Socket（`/var/run/otus.sock`）→ daemon
- Phase 2 补充：`otus-status` topic 上报心跳和 Task 状态，形成完整闭环

**被否决的方案**：
- gRPC Server（推模式）：需要开放入站端口，Phase 1 不实施
- gRPC 双向流（Agent 拨出到 Control Plane）：增加 gRPC 依赖，连接管理复杂，收益不如 Kafka 方案

---

### ADR-016: 重构策略——推倒重来

**问题**：现有代码库有部分可用模块（AF_PACKET capture、IPv4 重组、SIP parser、SkyWalking handler），重构时采用渐进式还是推倒重来？

**决定**：**推倒重来**。按新架构从零搭建骨架，可复用的算法逻辑（BPF 编译、IP 重组算法等）参考旧代码重写。

**理由**：

1. 现有代码存在双套插件系统（`pkg/plugin` 生命周期式 + `internal/config` 反射注册式），耦合严重，渐进重构需要同时维护两套
2. 数据模型完全不同：旧代码用 `EventContext`（泛型 map）传递数据，新架构用强类型三层结构（RawPacket → DecodedPacket → OutputPacket）
3. Pipeline 模型根本不同：旧代码是 channel 连接的 capture→processor→sender 管道，新架构是单 goroutine 主循环 + 零 channel 传递
4. 控制面完全重写：旧 gRPC DaemonService → 新 Kafka 命令 channel + UDS
5. SkyWalking 相关代码（dialog/transaction 状态机、tracing 构建）不再需要（见 ADR-017）
6. 推倒重来反而更快——不用处理新旧代码的兼容层

**保留参考价值的旧代码**：
- `internal/utils/bpf.go` — BPF 过滤器编译逻辑
- `internal/otus/module/capture/codec/assembly_ipv4.go` — IPv4 分片重组算法思路
- `internal/otus/module/capture/handle/handle_afpacket.go` — AF_PACKET TPacket v3 配置参数
- `plugins/parser/sip/sip_parser.go` — SIP 协议检测和解析逻辑

---

### ADR-017: SkyWalking 代码移除

**问题**：`plugins/handler/skywalking/` 下有完整的 RFC 3261 SIP Dialog/Transaction 状态机 + SkyWalking Span 构建代码，在新架构中如何处置？

**决定**：**移除，不纳入新代码库**。

**理由**：

1. 这套逻辑本质是 **Collector 的职责**——从 Otus 收集到观测数据后，在 Collector 侧整理计算得到 Tracing Span
2. 需要缓存完整 SIP 会话（Dialog 状态 + Transaction 状态机），内存占用和计算复杂度远超抓包引擎应有的范围
3. 会话关联计算（跨多个包的状态追踪）会拖慢 hot path 性能，违背"核心只做捕获+解码+分发"的原则
4. Otus 的定位是**边缘抓包 Agent**，输出原始观测数据（OutputPacket），由下游 Collector 负责关联分析和 APM 集成

---

### ADR-018: 日志框架——slog + lumberjack + Loki HTTP Push

**问题**：日志框架选型。当前用 logrus，需要评估替换方案。

**决定**：
- **结构化日志**：Go 标准库 `log/slog`（Go 1.21+）
- **文件滚动**：`natefinch/lumberjack`（实现 `io.Writer`，与 slog 正交组合）
- **Loki 推送**：自实现 HTTP Push（~150 行），不引入 loki-client-go

**理由**：

1. slog 是 Go 标准库，零依赖、结构化、性能优于 logrus 30%+
2. slog 输出到 `io.Writer`，lumberjack 实现 `io.Writer` + 滚动，完美正交组合：
   ```go
   logger := slog.New(slog.NewJSONHandler(&lumberjack.Logger{...}, nil))
   ```
3. Loki Push API 极简（HTTP POST + JSON body），自实现 ~150 行代码，零额外依赖
4. grafana/loki-client-go 已不活跃，依赖树太重
5. Promtail sidecar 增加部署复杂度，边缘节点不想多一个独立进程
6. 多输出通过 `io.MultiWriter` 或多 Handler 组合实现

---

### ADR-019: Kafka 客户端——segmentio/kafka-go

**问题**：Kafka 客户端库选型。影响命令通道（Consumer）和 Reporter（Producer）两个核心组件。

**决定**：**segmentio/kafka-go**

**选型对比**：

| 维度 | segmentio/kafka-go | IBM/sarama | confluent-kafka-go |
|------|:---:|:---:|:---:|
| CGO | **无** | **无** | **需要 librdkafka** |
| API 风格 | 高层 Reader/Writer | 底层、灵活但复杂 | librdkafka 回调 |
| Consumer Group | ✅ | ✅ | ✅ |
| 交叉编译 | **✅ 无障碍** | **✅** | ❌ 需 C 工具链 |
| 容器镜像 | 小（静态编译） | 小 | 大（动态库） |
| 学习曲线 | **低** | 中高 | 中 |

**理由**：

1. **CGO 是硬伤**：Otus 部署在边缘节点，需要简单的静态二进制 + 多架构交叉编译（amd64/arm64），confluent-kafka-go 排除
2. Otus 的 Kafka 使用场景简单：一个 Consumer Group 订阅命令 topic + Producer 发送数据，不需要事务/Exactly-Once
3. kafka-go 的 `Reader`/`Writer` 抽象与 Otus 场景精确匹配，代码量约为 sarama 的 1/3

---

### ADR-020: 本地控制通道——JSON-RPC over Unix Domain Socket

**问题**：CLI 与 daemon 之间的本地通信协议选型。

**决定**：**JSON-RPC over Unix Domain Socket**，不使用 gRPC。

**理由**：

1. 本地控制只在 CLI ↔ daemon 之间通信，不需要 gRPC 的跨语言/跨网络能力
2. JSON-RPC 协议简单（request: `{method, params, id}`，response: `{result, error, id}`），实现轻量
3. UDS 不占用任何网络端口，文件权限控制即可实现访问控制
4. 减少一个重量级依赖（protoc 工具链 + grpc-go 库）

**保留的框架决策**：
- CLI 框架：`spf13/cobra`（保持不变）
- 配置框架：`spf13/viper`（保持不变）

---

### ADR-021: DecodedPacket 自定义值类型——隔离 gopacket 依赖

**问题**：核心数据结构 `DecodedPacket` 的 L2/L3/L4 Header 如何表达？直接用 gopacket layers 还是自定义？

**决定**：**纯自定义值类型 struct**，不暴露 gopacket 类型到核心接口。

**设计**：

```go
type EthernetHeader struct {
    SrcMAC, DstMAC [6]byte
    EtherType      uint16
    VLANs          []uint16       // 0~2 个 VLAN tag
}

type IPHeader struct {
    Version    uint8
    SrcIP      netip.Addr         // Go 标准库，值类型零分配
    DstIP      netip.Addr
    Protocol   uint8              // TCP=6, UDP=17
    TTL        uint8
    TotalLen   uint16
}

type TransportHeader struct {
    SrcPort  uint16
    DstPort  uint16
    Protocol uint8              // TCP=6, UDP=17
    // TCP 特有（仅 TCP 填充）
    TCPFlags uint8
    SeqNum   uint32
    AckNum   uint32
}

type DecodedPacket struct {
    Timestamp  time.Time
    Ethernet   EthernetHeader
    IP         IPHeader
    Transport  TransportHeader
    Payload    []byte             // 应用层载荷，零拷贝切片
    CaptureLen uint32
    OrigLen    uint32
}
```

**理由**：

1. 核心数据结构零外部依赖，API 稳定不受 gopacket 版本变更影响
2. `netip.Addr` 是 Go 标准库值类型，比 `net.IP`（slice）零分配、可比较
3. 值类型 struct 在 hot path 上可栈分配，对 GC 零压力
4. 核心解码器内部使用 gopacket 做实际解析，输出转换为自定义类型——gopacket 限制在 `internal/core/decoder` 包内部

---

### ADR-022: 插件注册机制——静态链接 + init() 全局 Registry

**问题**：TaskConfig 中通过字符串名称引用插件（如 `"sip"`, `"afpacket"`, `"kafka"`），需要一个机制将名称映射到具体的插件实例。

**决定**：**纯静态链接 + init() 自动注册到全局 Registry**。不支持运行时动态加载 `.so`。

**方案对比**：

| 方案 | 优点 | 缺点 |
|------|------|------|
| A. 静态链接 + init() Registry | 零开销，编译期可知，简单可靠 | 新增插件需重新编译 |
| B. Go plugin (.so) | 运行时可扩展 | 需要 CGO，必须同版本编译，不可卸载，Linux only，实际几乎不可用 |
| C. 子进程 + IPC | 语言无关 | 巨大性能开销（序列化 + IPC），hot path 不可接受 |

**理由**（选 A）：

1. Go 的 `plugin.Open()` 限制严格——必须同 Go 版本、同依赖版本编译，不可卸载，仅 Linux/macOS，社区普遍不推荐
2. Otus 是边缘采集 Agent，部署时重新编译二进制是正常流程（交叉编译 amd64/arm64），不需要运行时扩展
3. 静态链接的性能最优——函数调用无间接层，编译器可内联优化
4. 模式成熟：`database/sql`、`image`、`hash` 等标准库均使用 init() + Register 模式

**Registry 设计要点**：

1. **位置**：`pkg/plugin/registry.go`（`pkg/` 下，插件实现可以 import）
2. **按类型分表**：capturer / parser / processor / reporter 各有独立 map，避免命名冲突，查找时类型安全
3. **Factory 签名**：`func() T`（零参数返回空实例），不做 Init——配置来自 TaskConfig，注册时不可知
4. **安全约束**：注册重名 panic（编译期 bug），查找不到返回 error
5. **线程安全**：init() 阶段单线程写入，运行期只读不写，无需 sync

**生命周期分离**：
```
Factory(构造空实例) → Init(注入配置) → Wire(注入共享资源) → Start(启动) → Stop(停止)
```

---

### ADR-023: Node IP 解析策略

**状态**: 已决定  
**日期**: 2026-02-17  
**关联文档**: config-design.md §6.1

### 背景

`otus.node.ip` 用于 Label 注入（每个 OutputPacket 携带采集节点 IP）和人类识读。需要在"必须手动配置"和"支持自动探测"之间选择。

### 决定

**混合方案：环境变量 > 自动探测 > 启动报错**。

解析优先级：
1. 环境变量 `OTUS_NODE_IP`（Viper `AutomaticEnv()` 映射）
2. YAML 配置 `otus.node.ip` 显式值
3. 自动探测：`net.Interfaces()` 遍历，取首个 UP 且非 loopback 的 IPv4 地址（排除 169.254.x.x link-local）
4. 全部失败 → `log.Fatal("cannot resolve node IP: set OTUS_NODE_IP or otus.node.ip")`

### 理由

- 容器/K8s 环境通过 `OTUS_NODE_IP` env 注入最方便（Downward API）
- 裸机部署自动探测减少配置负担
- 不允许静默成功（如 fallback 到 127.0.0.1），宁可启动失败也要获得正确的节点 IP

---

### ADR-024: Kafka 全局配置继承

**状态**: 已决定  
**日期**: 2026-02-17  
**关联文档**: config-design.md §6.2

### 背景

`command_channel.kafka` 和 `reporters.kafka` 经常连接同一 Kafka 集群，brokers/sasl/tls 配置重复声明。

### 决定

新增顶层 `otus.kafka` 全局配置节，提供 `brokers`/`sasl`/`tls` 默认值。`command_channel.kafka` 和 `reporters.kafka` 自动继承，显式设置的字段覆盖全局默认。

### 继承规则

- `otus.kafka.brokers` → 被 `command_channel.kafka.brokers`（空时）和 `reporters.kafka.brokers`（空时）继承
- `otus.kafka.sasl` → 被子节点的 `sasl`（零值时）继承
- `otus.kafka.tls` → 被子节点的 `tls`（零值时）继承
- 合并逻辑在 `GlobalConfig.validateAndApplyDefaults()` 中实现

### 理由

- 消除 90%+ 场景下的配置重复
- 显式覆盖保证灵活性（命令通道和数据面连不同集群时各自声明）
- 合并在加载后一次完成，运行时无间接层

---

### ADR-025: 日志滚动字段格式

**状态**: 已决定  
**日期**: 2026-02-17  
**关联文档**: config-design.md §6.3

### 背景

日志滚动配置是用人类可读格式（`"100MB"`, `"7d"`）还是纯数值？

### 决定

**纯数值字段，单位编码在字段名中**：`max_size_mb: 100`, `max_age_days: 7`。

### 理由

- 无需解析函数，无单位歧义
- Viper 环境变量覆盖直接传数值（`OTUS_LOG_OUTPUTS_FILE_ROTATION_MAX_SIZE_MB=200`）
- 与 lumberjack 库的 API 直接映射（`MaxSize int` 单位 MB，`MaxAge int` 单位天）

---

### ADR-026: Kafka 命令可靠性策略

**状态**: 已决定  
**日期**: 2026-02-17  
**关联文档**: config-design.md §7.3

### 背景

Kafka at-least-once 语义下需要处理三个可靠性问题：重复投递、乱序消费、过期命令。

### 决定

#### 去重

- Phase 1：`KafkaCommand` 结构体包含 `request_id` 字段，日志中关联记录便于链路追踪
- Phase 2：Agent 侧维护 LRU 缓存（最近 N 条已处理 request_id），实现精确去重

#### 排序

- **发送端要求**：必须使用 `KafkaCommand.Target` 作为 Kafka message key，保证同一目标节点的命令落到同一 partition（文档化到 API 接入指南）
- **Agent 侧**：`task_create` 做冲突检查，`task_delete` 做存在性检查，天然容忍乱序

#### 过期命令

- 默认 `auto_offset_reset=latest` + 持久化 offset
- `KafkaCommand.Timestamp` + 可配置 TTL（`command_ttl`，默认 5m），超时命令跳过并记 WARN

### 理由

- Phase 1 命令天然幂等/无害重试，不需要立即实现精确去重
- target 做 partition key 是最小侵入的排序保证
- TTL 防御性检查避免 Agent 重启后消费到历史积压命令

---

### ADR-027: Kafka Reporter 动态 Topic 路由

**状态**: 已决定  
**日期**: 2026-02-17  
**关联文档**: config-design.md §8

### 背景

架构文档示例代码使用 `"otus-" + pkt.Protocol` 的动态 topic 路由，但当前实现使用固定 `topic`。

### 决定

支持两种互斥模式，配置项决定：

| 配置 | 路由行为 | topic 示例 |
|------|---------|-----------|
| `topic: "voip-packets"` | 固定 topic | `voip-packets` |
| `topic_prefix: "otus"` | 动态路由 | `otus-sip`, `otus-rtp`, `otus-raw` |

`topic_prefix` 存在时优先使用动态路由。路由键为 `OutputPacket.PayloadType`（`"sip"`, `"rtp"`, `"raw"` 等）。

### 理由

- 按协议分 topic 便于下游独立消费和独立 retention 策略
- 保留固定 topic 模式兼容简单部署场景
- `PayloadType` 由 Parser 返回，核心协议无关

---

### ADR-028: Kafka 数据序列化——Headers + Value 分离

**状态**: 已决定  
**日期**: 2026-02-17  
**关联文档**: config-design.md §9

### 背景

Kafka `message.value` 是 `[]byte`，支持任意格式。需要确定 Envelope（元数据）和 Payload（协议数据）的序列化方案。

### 决定

**Kafka Headers 承载 Envelope，Value 承载 Payload**：

- Kafka Headers: `task_id`, `agent_id`, `payload_type`, `src_ip`, `dst_ip`, `timestamp`, Labels（以 `l.` 前缀区分）
- Kafka Value: `Payload.MarshalJSON()` 或 `Payload.MarshalBinary()`（由 `serialization` 配置项决定）

可配置的序列化格式：
- `serialization: json`（默认，Phase 1）— 调试友好
- `serialization: binary`（生产推荐）— 零膨胀，性能最高

### 理由

- Headers 可被 Kafka Streams / ksqlDB 过滤，无需反序列化 Value
- Value 纯 binary 零膨胀（对比 base64 in JSON 膨胀 33%）
- 与架构文档 `Payload.MarshalBinary()` 接口设计完全一致
- Phase 1 先用 JSON 模式（已实现），binary 模式在 Payload 接口完善后启用

---

### ADR-029: Kafka 命令响应通道

**状态**: 已决定  
**日期**: 2026-02-21  
**关联文档**: architecture.md §6.3

### 背景

现有 Kafka 命令通道为单向拉模式：远端写入 `otus-commands`，近端（Otus Agent）消费并执行，
但命令执行结果无回写路径。`CommandHandler` 对每条命令都构造了 `Response`，结果被丢弃。
这使得所有需要返回数据的交互式命令（`task_list`、`task_status`、`daemon_status`、
`daemon_stats`）在 Web CLI 场景下完全无法工作——命令发出后调用方永远收不到响应。

### 问题根源

```
远端 ──► [otus-commands] ──► Agent.Handle() ──► Response{ result } ──► ✗ 丢弃
```

### 决定

新增固定 `otus-responses` topic 作为命令响应通道，形成完整的请求-响应闭环。

#### 响应消息格式（`KafkaResponse`）

```json
{
  "version":    "v1",
  "source":     "edge-beijing-01",
  "command":    "task_list",
  "request_id": "req-abc-123",
  "timestamp":  "2026-02-21T10:30:00Z",
  "result":     { ... },
  "error":      { "code": -32603, "message": "..." }
}
```

`result` 与 `error` 互斥，与现有 `handler.Response` 结构直接对应。

#### 消息路由：hostname 作为 Kafka message key

Agent 写响应时以自身 `hostname` 为 Kafka message key。Kafka 一致性哈希保证同一节点
的所有响应落到同一 partition，天然按节点聚合。

```
node-01 的响应 ──► key="edge-beijing-01" ──► partition-P1 ─┐
node-02 的响应 ──► key="edge-shanghai-02" ──► partition-P2  ├─ otus-responses
node-03 的响应 ──► key="edge-guangzhou-03" ──► partition-P3 ┘
```

#### 消费端：per-instance 唯一 consumer group

Web CLI（或任何调用方）每个**实例**（进程/Pod）使用唯一 `group_id`（推荐格式：`webcli-{instance-id}`），
独立消费 `otus-responses` 全量消息，以 `request_id` 过滤属于本实例本请求的响应。
同一实例内的多个并发 session 共享同一 consumer，无需各自建立独立 consumer group。

`instance-id` 必须**从运行环境注入，不得写死**在配置文件中：

```yaml
# Kubernetes：通过 Downward API 将 Pod 名称注入环境变量
env:
  - name: WEBCLI_INSTANCE_ID
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
```

```bash
# 虚拟机 / 裸机：使用 HOSTNAME
WEBCLI_INSTANCE_ID=$HOSTNAME
```

```
instance-A (group_id="webcli-pod-abc12") → 消费全量，多 session 共享，按 request_id 过滤
instance-B (group_id="webcli-pod-xyz99") → 消费全量，多 session 共享，按 request_id 过滤
```

**关键约束**：多个实例绝不能共享同一 `group_id`。
若共享，Kafka partition rebalance 会将 partition 重新分配给 group 内不同实例，
实例 A 发出请求的响应可能被实例 B 消费，实例 A 永远收不到匹配响应。

#### 完整交互流程

```
远端 (Web CLI session)              Kafka                    Otus (近端)
        │                                                         │
        │  1. 记录当前 offset                                     │
        │  fetch_offset(otus-responses)                           │
        │                                                         │
        │  2. 发送命令                                            │
        │──► KafkaCommand ────────► [otus-commands] ────────────► │
        │    request_id: "req-001"                                │  3. 执行命令
        │    target: "edge-beijing-01"                            │     task_list()
        │                                                         │
        │                        [otus-responses]                 │  4. 写响应
        │◄── KafkaResponse ◄──── partition-P1 ◄── key=hostname ◄──│
        │    request_id: "req-001"                                │
        │    source: "edge-beijing-01"                            │
        │    result: { "tasks": [...] }                           │
        │                                                         │
        │  5. 匹配 request_id，展示结果或超时报错                  │
```

#### 响应时机与可靠性

- Agent 在命令执行完成后（无论成功或失败）写响应，at-most-once 语义
- 调用方负责设置超时（推荐 30s），超时视为节点无响应
- Agent 写响应失败（如 Kafka 不可达）只记录 ERROR 日志，不影响命令已执行的结果
- Fire-and-forget 命令（`task_create`、`task_delete`、`config_reload`、
  `daemon_shutdown`）同样写响应，确认执行已触发

#### 不写响应的情况

- `request_id` 为空字符串（来自不支持 request_id 的旧客户端）
- `response_topic` 配置为空（显式禁用响应通道）

#### 配置变更

`command_channel.kafka` 新增 `response_topic` 字段：

```yaml
command_channel:
  kafka:
    topic: otus-commands
    response_topic: otus-responses   # 新增，空字符串表示禁用响应
    group_id: "otus-${node.hostname}"
```

#### 与 otus-status 的关系

`otus-status`（Phase 2）是节点主动发布的心跳和 Task 状态快照，属于事件驱动的
状态上报，与本 ADR 的命令响应通道**用途不同、topic 不同、不可混用**。

| topic | 方向 | 触发 | 内容 |
|---|---|---|---|
| `otus-commands` | 远端→近端 | 调用方主动 | 命令请求 |
| `otus-responses` | 近端→远端 | 命令执行后 | 命令结果 |
| `otus-status` (Phase 2) | 近端→远端 | 定时/事件 | 节点心跳、Task 状态 |

### 理由

- **固定 2 个 topic 不随节点数增长**：per-node topic 方案在边缘大规模部署时 topic
  数量线性增长，运维成本不可接受；固定 topic + message key 路由复用 Kafka 已有
  partition 机制，扩容只需保证 partition 数量 ≥ 节点数
- **per-instance group_id 而非共享 group**：共享 consumer group 在多副本部署时，
  Kafka partition rebalance 会将 partition 分配给 group 内不同实例，导致某实例的响应
  被另一实例抢读；per-instance group（group_id 从 `$POD_NAME`/`$HOSTNAME` 注入）
  让每个实例独立消费全量，实例内多 session 共享 consumer 并以 `request_id` 区分，无争用
- **hostname 作为 message key**：与 ADR-026 中 `target` 作为命令 message key 的
  规范对称，方便按节点追踪完整的请求-响应链路
- **at-most-once 响应**：命令本身是 at-most-once 语义（ADR-026），响应保持一致，
  不引入额外复杂度

---

### ADR-030: Task 持久化——每任务独立状态文件

**状态**: 已决定  
**日期**: 2026-02-22  
**关联文档**: implementation-plan.md Step 15（待补充）

### 背景

`TaskManager` 的 `tasks` map 是纯内存结构。守护进程重启（主动升级、系统崩溃、OS 重启）后，
所有运行中的抓包任务丢失，必须由运维人员手动重新下发命令才能恢复。在边缘节点无人值守的
生产环境中，这会导致长时间的抓包服务中断。

### 核心约束

- 边缘节点 AF_PACKET 抓包必须以 root 身份运行
- 数据目录不得放在用户 HOME（root 的 `/root` 或普通用户 `~`）
- 磁盘占满时必须能在程序未运行的情况下由外部机制清理（避免"程序不启动就无法清理"死锁）

### 决定

#### 存储结构

```
/var/lib/otus/            ← data_dir（全局配置，符合 FHS 标准）
  tasks/
    sip-capture-01.json   ← 每任务一个文件，文件名 = task_id.json
    voip-monitor-02.json
```

#### 持久化格式（`PersistedTask` v1）

```json
{
  "version":        "v1",
  "config":         { ...TaskConfig 原文... },
  "state":          "running",
  "created_at":     "2026-02-21T10:00:00Z",
  "started_at":     "2026-02-21T10:00:01Z",
  "stopped_at":     null,
  "failure_reason": "",
  "restart_count":  0
}
```

#### 写入时机（状态机钩子）

| 事件 | 写入内容 |
|------|---------|
| `task_create` 成功 | 完整 PersistedTask，state=running |
| `task_delete` 完成 | state=stopped，stopped_at=now |
| Task 进入 Failed 状态 | state=failed，failure_reason=err.Error() |
| Graceful shutdown | state=stopped，stopped_at=now |
| 进程崩溃 | 保留上次写入状态（重启时以此触发恢复路径） |

写入使用 **temp-file + rename** 原子操作：
```
写 /var/lib/otus/tasks/.{id}.json.tmp
→ os.Rename → /var/lib/otus/tasks/{id}.json
```

#### 重启恢复策略

`Daemon.Start()` 步骤 4.5（TaskManager 创建后，UDS/Kafka 启动前）执行：

```
扫描 /var/lib/otus/tasks/*.json
  ↓
state == running / starting / stopping  →  auto-restart（下面详述）
state == stopped / failed              →  加载为只读历史记录，不启动
state == created（进程在 Starting 前崩溃）→ 视为 failed，写文件，不启动
```

**auto-restart 流程**：
1. 反序列化 `PersistedTask.config` → `TaskConfig`
2. 调用 `TaskManager.Create(cfg)`（走完整 7-phase 装配流程）
3. 成功 → `restart_count++` 写文件，记录 `INFO` 日志
4. 失败 → state=failed，failure_reason=err，写文件，记录 `ERROR` **但不阻断守护进程启动**

**单任务恢复失败不阻断其他任务的恢复，也不阻断守护进程启动。**

#### 与 Phase 1 单任务限制的交互

Phase 1 的"最多 1 个 Task"限制仅针对**活跃任务（非终止态）**：
- running/starting/stopping → 占用 1 个 slot，触发 auto-restart
- stopped/failed → 加载为只读历史，不占用 slot，不受 1 个任务限制

重启后如果有 1 个 running 任务文件 + N 个 stopped/failed 文件，可以正常恢复——running 的
任务被重启（占 1 slot），N 个历史记录作为元数据加载到 manager 供 `task_status` 查询。

#### 全局配置新增字段

```yaml
otus:
  data_dir: /var/lib/otus     # FHS 标准路径，需要目录已存在或 systemd 创建

  task_persistence:
    enabled: true              # false = 禁用（开发/测试用），重启后不恢复
    auto_restart: true         # 默认 true：重启后自动恢复 running 任务
```

### 备选方案

| 方案 | 否决原因 |
|------|---------|
| 单个 `tasks.json` 全量文件 | 并发写需全量锁；单文件损坏影响所有任务 |
| BoltDB / BadgerDB | 引入新依赖；功能远超需要 |
| 外部存储（Redis/etcd） | 违反单节点零运行时外部依赖原则 |
| 崩溃后恢复所有状态（含 stopping）| Stopping 状态下崩溃说明任务正在主动关闭，不应重启 |

### 理由

- **每任务独立文件**：原子 rename 写单个文件，无争用；文件名即 ID，删除 = `os.Remove`
- **`/var/lib/otus`**：FHS 标准持久数据目录，与 root 运行的系统服务一致，避免用户 HOME
- **temp + rename**：系统崩溃时绝对不会写出损坏的半成品 JSON
- **重启后 auto-restart 默认 true**：边缘无人值守场景下，运维期望"重启自愈"而非"重启后人工重下发命令"

---

### ADR-031: Task 历史清理——委托 systemd-tmpfiles.d

**状态**: 已决定  
**日期**: 2026-02-22  
**关联文档**: ADR-030

### 背景

ADR-030 持久化方案在磁盘上累积终止态任务文件（stopped/failed）。长时间运行的节点
如果状态频繁变化（任务反复创建/删除），加上程序内的 GC 逻辑只在进程运行时触发，
存在"磁盘写满 → 守护进程无法启动 → 无法触发程序内 GC → 磁盘继续满"的死锁。

### 核心目标

**文件清理能力必须独立于 otus 进程存活，在 otus 未运行时也能被系统清理。**

### 决定

#### 主清理机制：systemd-tmpfiles.d

提供 `configs/tmpfiles.d/otus.conf`（随包安装到 `/etc/tmpfiles.d/otus.conf`）：

```
# systemd-tmpfiles(5) 配置：清理 otus 任务历史文件
#  类型  路径                          模式    UID   GID   期限
   D     /var/lib/otus                  0750    root  root  -       # 确保目录存在
   d     /var/lib/otus/tasks            0750    root  root  -
   e     /var/lib/otus/tasks            -       -     -     7d      # 超过 7 天的文件由 systemd 删除
```

- `D` 指令：创建目录（如不存在），由 `systemd-tmpfiles --create` 在服务启动前执行
- `e` 指令：age-based 清理，由 `systemd-tmpfiles --clean`（系统每日定时任务）执行

**结果**：即使 otus 进程从未启动，系统的 `systemd-tmpfiles-clean.timer`（默认每日运行）
也会自动清理超过 TTL 的任务历史文件。

#### 辅助清理机制：程序内 GC（守护进程运行期间）

程序内 GC 作为额外保障层，处理"文件数量膨胀但单文件不超龄"的场景：

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `gc_interval` | `1h` | GC goroutine 周期 |
| `max_task_history` | `100` | 终止态历史记录上限 |

GC 触发条件（满足任一即删除对应文件）：
- 终止态（stopped/failed）且最后活跃时间超过 `task_ttl`（由 tmpfiles.d 的期限决定，默认 7d）
- 终止态历史总数超过 `max_task_history`，按 `stopped_at` 升序删除最旧的（超出 100 的部分）

永不清理：
- 任何非终止态（running/starting/stopping/paused）
- 可以通过 将 `max_task_history: 0` 设置为 0 完全禁用程序内 GC（完全依赖 tmpfiles.d）

```yaml
otus:
  task_persistence:
    enabled: true
    auto_restart: true
    gc_interval: 1h
    max_task_history: 100    # 0 = 禁用程序内 GC，仅依靠 systemd-tmpfiles.d
```

#### 部署集成

```
安装时：
  systemd-tmpfiles --create /etc/tmpfiles.d/otus.conf  # 创建 /var/lib/otus/tasks/

运行时（系统自动）：
  systemd-tmpfiles-clean.timer → systemd-tmpfiles --clean → 删除 >7d 的 .json 文件
```

与 `otus.service` 的集成（写入 systemd unit 文件）：
```ini
[Service]
ExecStartPre=systemd-tmpfiles --create /etc/tmpfiles.d/otus.conf
```
确保目录在服务启动前一定存在，即便 tmpfiles.d 尚未被手动执行过。

### 两层清理机制对比

| 层次 | 触发 | 解决的问题 |
|------|------|-----------|
| systemd-tmpfiles.d（外部） | 系统每日定时 / 服务启动前 | 进程未运行时的磁盘清理、磁盘满死锁 |
| 程序内 GC goroutine | 每 `gc_interval` | 数量膨胀（文件未超龄但总数爆炸）、即时清理 |

### 备选方案

| 方案 | 否决原因 |
|------|---------|
| 仅依赖程序内 GC | 进程不运行时无法清理，存在死锁风险 |
| cron + shell 脚本 | 引入外部运维依赖，systemd-tmpfiles.d 是更标准的 Linux 机制 |
| logrotate | 设计用于日志文件轮转，语义不匹配 |
| inotify 监控 + 触发清理 | 过度设计，tmpfiles.d 完全覆盖需求 |

### 理由

- **委托 systemd-tmpfiles.d 是解决"程序未运行时清理"的标准 Linux 方案**，无需额外依赖
- 两层清理互补：外部清理保证安全底线，程序内 GC 提供即时和数量控制
- `max_task_history: 0` 提供退出门——在 systemd-tmpfiles.d 已覆盖所有场景的环境中
  可完全禁用程序内 GC，减少代码复杂度

---

## 决策优先级总览

| ADR | 决策点 | 结论 | 实施阶段 |
|-----|--------|------|----------|
| 001 | 背压与丢弃策略 | 分层非阻塞 + 分层丢弃 | Phase 1 |
| 002 | L2-L4 解码归属 | 核心代码，非插件 | Phase 1 |
| 003 | L2 封装处理范围 | VLAN/QinQ 常开，隧道可配置 | Phase 1 |
| 004 | IP 分片重组 | 常开，简单硬上限+超时 | Phase 1 |
| 004a | 分片攻击防护 | 简单硬上限，不做深度防护 | Phase 1 |
| 004b | 分片 vs 零拷贝 | 分片走独立分配，正常包零拷贝 | Phase 1 |
| 005 | TCP 流重组 | 选择性重组，默认关闭 | Phase 2 |
| 005a | TCP 重组选型 | 接口抽象 + gopacket 先行 | Phase 2 |
| 005b | TCP 输出粒度 | 输出有序字节流，插件分帧 | Phase 2 |
| 005c | Mid-stream join | 必须支持，首 seq 做相对基准 | Phase 2 |
| 005d | 连接生命周期 | 无状态机，纯超时驱逐 | Phase 2 |
| 005e | 乱序/重传/空洞 | 去重+排序+空洞超时跳过 | Phase 2 |
| 005f | TCP 内存上限 | 全局内存池 + LRU 淘汰 | Phase 2 |
| 006 | Pipeline 并行模型 | 每核一条 Pipeline，线性扩展 | Phase 1 |
| 007 | Parser 两阶段接口 | CanHandle + Handle，核心协议无关 | Phase 1 |
| 008 | FlowRegistry | 核心基础设施，sync.Map 实现，per-Task 跨 pipeline 共享 | Phase 1 |
| 009 | 性能度量模型 | pps/core 为核心指标，线性扩展 | Phase 1 |
| 010 | Pipeline 数据契约 | 三层结构，Envelope + Typed Payload | Phase 1 |
| 011 | Processor 职责边界 | 仅过滤 + 标注，不碰 Payload | Phase 1 |
| 012 | Labels 命名规范 | `{protocol}.{field}` 分级命名 | Phase 1 |
| 013 | 两层配置模型 | 全局静态 + Task 动态（Kafka 命令 / CLI） | Phase 1 |
| 014 | Task-Pipeline 绑定 | Pipeline 绑定 Task，FlowRegistry per-Task，Phase 1 单 Task | Phase 1 |
| 015 | 远程控制拉模式 | 订阅 Kafka 命令 topic，不开入站端口 | Phase 1 |
| 016 | 重构策略 | 推倒重来，旧代码仅做算法参考 | Phase 1 |
| 017 | SkyWalking 代码 | 移除，会话关联属于 Collector 职责 | Phase 1 |
| 018 | 日志框架 | slog + lumberjack 滚动 + Loki HTTP Push 自实现 | Phase 1 |
| 019 | Kafka 客户端 | segmentio/kafka-go，纯 Go 无 CGO | Phase 1 |
| 020 | 本地控制通道 | JSON-RPC over UDS，不用 gRPC | Phase 1 |
| 021 | DecodedPacket 类型 | 自定义值类型 struct，隔离 gopacket | Phase 1 |
| 022 | 插件注册机制 | 静态链接 + init() 全局 Registry，不支持动态 .so | Phase 1 |
| 023 | Node IP 解析策略 | 环境变量 > 自动探测 > 启动报错 | Phase 1 |
| 024 | Kafka 全局配置继承 | `otus.kafka` 提供 brokers/sasl/tls 默认，子节点显式覆盖 | Phase 1 |
| 025 | 日志滚动字段格式 | 数值字段 `max_size_mb` / `max_age_days`，单位编码在字段名中 | Phase 1 |
| 026 | Kafka 命令可靠性 | 去重 Phase2(LRU)、排序(target 做 key)、TTL 过期检查 | Phase 1 |
| 027 | Kafka Reporter 动态 Topic | `topic_prefix` 优先动态路由，与 `topic` 互斥 | Phase 1 |
| 028 | Kafka 数据序列化 | Headers 承载 envelope，Value 承载 binary/json payload | Phase 1 |
| 029 | Kafka 命令响应通道 | 固定 `otus-responses` topic，per-instance group_id（环境变量注入），hostname 作 key | Phase 1 |
| 030 | Task 持久化 | 每任务独立 JSON 文件（`/var/lib/otus/tasks/`），temp+rename 原子写，重启 auto-restart 默认 true | Phase 1 |
| 031 | Task 历史清理 | 主清理委托 systemd-tmpfiles.d（解决进程未运行时死锁），程序内 GC 作辅助（数量上限 + 周期 1h） | Phase 1 |

---

**文档版本**: v0.5.0
**更新日期**: 2026-02-22
**作者**: Otus Team
