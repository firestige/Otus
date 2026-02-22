# Otus TODO

后续待完成工作清单。Phase 1 已全部完成（含 Step 16/17/18）。

---

## Phase 2 — 近期规划

### P2-1: RTP / RTCP 解析插件 ⭐（本期实现）

`plugins/parser/rtp/rtp.go`

**设计要点**（参见 architecture.md §5.5.6）：

- **`CanHandle`**: 先查 FlowRegistry（命中则确定是 RTP）；未命中则做轻量 RTP 包头校验（V=2, PT < 128, 固定 12 字节头）
- **`Handle`**: 解析 RTP 固定头（V, PT, SeqNum, Timestamp, SSRC）；从 FlowRegistry 取出 `call_id`/`codec` 写入 labels
- **Labels 输出**: `rtp.ssrc`, `rtp.payload_type`, `rtp.seq`, `rtp.timestamp`, `rtp.call_id`, `rtp.codec`
- **RTCP 区分**: PT 200–209 为 RTCP；CanHandle 时区分类型，labels 用 `rtcp.pt` 等
- 注册到 `plugins/init.go`: `plugin.RegisterParser("rtp", rtp.NewRTPParser)`

**前置依赖**: FlowRegistry 已由 SIP Parser 写入（SIP INVITE + 200 OK 后可查）

---

### P2-2: 状态上报 Topic（控制闭环）

`internal/task/reporter.go` 或接入 `internal/daemon/daemon.go`

- 心跳：每 30s 向 `otus-heartbeat` topic 推送节点状态（hostname、版本、运行时间、active task 数）
- Task 状态变更事件：Task 进入 running/stopped/failed 时推送到 `otus-events` topic
- 远端平台通过这两个 topic 判断节点可用性和任务执行结果

---

### P2-3: 多 Task 并发

`internal/task/manager.go`

- 移除 Phase 1 的单 Task 限制（`ErrTaskAlreadyExists`）
- 同网卡多 BPF：AF_PACKET 多 socket + 不同 fanout_group_id 共存
- 资源配额：每 Task 限制 CPU workers 总量上限（`resources.max_workers`）

---

### P2-4: SIP over TCP 解析

`plugins/parser/sip/sip.go` 扩展

- 当前仅处理 UDP（`CanHandle` 仅检查 UDP 端口 + SIP 魔数）
- TCP 场景需要 Content-Length 分帧（SIP 消息边界）
- 依赖 P2-5 TCP 重组引擎提供有序字节流

---

### P2-5: TCP 流重组引擎

`internal/core/decoder/tcp_assembly.go`

- 使用 gopacket/tcpassembly 接口抽象（ADR-005）
- Mid-stream join 支持：以首个 seq 做相对基准（ADR-005c）
- 全局内存池 128MB + LRU 淘汰（ADR-005f）
- 输出有序字节流片段，应用层 Parser 自行分帧（ADR-005b）

---

### P2-6: XDP 捕获插件

`plugins/capture/xdp/xdp.go`

- 使用 `github.com/cilium/ebpf` 驱动 XDP program
- 高性能场景替代 AF_PACKET（内核旁路，零拷贝）
- 需要内核 4.8+，Capturer 接口与 afpacket 保持一致

---

### P2-7: gRPC 上报插件（可选）

`plugins/reporter/grpc/grpc.go`

- OpenTelemetry Collector gRPC 接入（otlp/grpc）
- 依赖 `google.golang.org/grpc`（Phase 1 未引入）
- 适用于与 OpenTelemetry 生态对接的场景

---

### P2-8: OpenTelemetry 集成

- Traces：将 SIP Call-ID 映射为 Trace-ID，RTP 包作为 Span
- Metrics：通过 OTLP Exporter 补充 Prometheus 指标
- 依赖 P2-7 gRPC Reporter

---

## Phase 3 — 长期规划

### P3-1: HTTP / WebSocket 协议解析

`plugins/parser/http/`, `plugins/parser/websocket/`

- HTTP/1.1 深度解析（Method、URL、Status、Host、Content-Type）
- HTTP/2 帧解析（HEADERS 帧提取伪头部）
- WebRTC 信令（WebSocket 承载的 SDP offer/answer）

---

### P3-2: 自研 TCP 重组引擎

当 gopacket/tcpassembly 成为性能瓶颈时替代。

- 零拷贝字节流管理
- 基于 radix tree 的 sequence number 索引
- 更细粒度的内存管理（per-flow 限额）

---

### P3-3: 动态采样降级

`plugins/processor/sampler/`

- 当 pps 超过阈值时自动按比例采样（基于哈希/随机）
- 支持按 flow、按协议类型差异化采样率
- 采样状态上报到 `otus-events` topic

---

### P3-4: 集中式管理平台

独立服务（超出本仓库范围）：

- 多节点注册与发现（基于 `otus-heartbeat` topic）
- Web UI：节点状态、Task 管理、流量统计
- 配置下发：通过 `otus-commands` topic 批量管理节点
- 告警规则：基于 Prometheus AlertManager

---

### P3-5: 多节点协同

- 跨节点 Call-ID 关联（网关侧 SIP、媒体侧 RTP 在不同节点）
- 分布式 FlowRegistry（Redis/etcd 后端）
- 链路追踪：将 SIP dialog 关联为完整通话事件

---

### P3-6: 流量回放

`plugins/capture/pcap_replay/`

- 从 PCAP 文件读取历史流量驱动完整处理链（用于测试、复现）
- 支持按时间戳比例回放（保持原始时间间隔或加速）

---

## 遗留 / 技术债

| 项目 | 说明 | 优先级 |
|------|------|--------|
| 性能基准验证 | 实际硬件测试单核 ≥200K pps（SIP 完整解析） | P1 补充 |
| AF_PACKET 集成测试 | 需要 root + 实际网卡，`//go:build integration` | P1 补充 |
| README.md | 安装、快速开始、配置说明 | P1 补充 |
| `daemon_status` / `daemon_stats` 命令 | handler.go 中尚未注册 | P1 补充 |
| filter processor 配置语法 | 当前 `drop_if` 表达式语法未完整文档化 | P2 |

---

**更新日期**: 2026-02-22  
**维护者**: Otus Team
