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

---

**文档版本**: v0.1.0  
**创建日期**: 2026-02-13  
**作者**: Otus Team
