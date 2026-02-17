# Otus 修复计划

**基于**: REVIEW_REPORT.md (2026-02-17)  
**制定日期**: 2026-02-17  
**最后更新**: 2026-02-17（v2 — 整合评审反馈）  
**状态**: 待审查

---

## 〇、报告勘误

在制定修复计划前，对报告中的技术表述与实际代码进行了逐项核对，发现以下不准确之处。修复计划已基于**实际代码行为**而非报告描述来制定。

### 勘误 1：Section 1.3 — sendBuffer 关闭竞态的描述不符合实际代码

**报告描述**：`senderLoop()` 内部使用 `select{}` 监听 `ctx.Done()` 和 `sendBuffer`，存在 panic 风险。

**实际代码** ([task.go](internal/task/task.go#L431-L445))：
```go
func (t *Task) senderLoop() {
    defer close(t.doneCh)
    for pkt := range t.sendBuffer {   // 简单 for-range，无 select{}
        for i, rep := range t.Reporters {
            if err := rep.Report(t.ctx, &pkt); err != nil { ... }
        }
    }
}
```

**实际问题**（不同于报告描述）：
- `senderLoop` 是简单的 `for pkt := range t.sendBuffer`，**不存在** select/ctx.Done() 路径
- 不会产生 "send on closed channel" panic（`pipelineWg.Wait()` 确保所有写者已退出后才关闭 channel）
- **真正的 bug**：`Stop()` 在 `close(t.sendBuffer)` 之前调用了 `t.cancel()`，导致 senderLoop 仍在处理残余包时 `rep.Report(t.ctx, &pkt)` 收到 `context.Canceled` 错误，**残余数据包丢失**

### 勘误 2：Section 2.7 — CanHandle() 性能估算

**报告描述**：`SIPParser.CanHandle()` 内部调用 `FlowRegistry.Get()`（sync.Map.Load），总耗时 ~65ns。

**实际代码** ([sip_parser.go](plugins/parser/sip/sip_parser.go#L95-L120))：
```go
func (p *SIPParser) CanHandle(pkt *core.DecodedPacket) bool {
    // 仅检查端口 (5060/5061) 和 SIP 魔数字节
    // 无 FlowRegistry 调用
}
```

**结论**：`CanHandle()` 不涉及 sync.Map 操作，实际耗时远低于 65ns。**无需优化**。

### 勘误 3：Section 3.1 — flowHash() 算法描述错误

**报告描述**：`flowHash()` 使用 `bytes.Buffer + crc32`，耗时 ~100ns。

**实际代码** ([task.go](internal/task/task.go#L340-L430))：
```go
func flowHash(pkt core.RawPacket) uint32 {
    h := fnv.New32a()           // FNV-1a 哈希（非 crc32）
    // 直接 h.Write() 切片，无 bytes.Buffer 分配
    h.Write(ipHdr[12:16])      // src IP
    h.Write(ipHdr[16:20])      // dst IP
    ...
    return h.Sum32()
}
```

**结论**：已使用 FNV-1a（零分配，~20-30ns），性能已达标。**无需替换为 xxhash**。

### 勘误 4：Section 5.4 — FlowRegistry 类型安全性

**报告描述**：`FlowRegistry.Get()` 使用 `interface{}` 作为 key，缺乏类型安全。

**实际代码** ([flow_registry.go](internal/task/flow_registry.go#L26-L28))：
```go
func (r *FlowRegistry) Get(key plugin.FlowKey) (any, bool) {
    return r.data.Load(key)   // 使用 plugin.FlowKey 类型（非裸 interface{}）
}
```

**结论**：key 已有类型约束（`plugin.FlowKey`）。value 仍为 `any`，但这是 Go sync.Map 的固有限制，泛型化改造收益有限。**降低优先级**。

### 勘误 5：Section 2.6 — Stop() 并发竞态

**报告描述**：`Stop()` 释放锁过早，两个 goroutine 可能同时进入 stop 逻辑。

**实际代码** ([task.go](internal/task/task.go#L221-L229))：
```go
func (t *Task) Stop() error {
    t.mu.Lock()
    if t.state != StateRunning {
        t.mu.Unlock()
        return fmt.Errorf("cannot stop task in state %s", t.state)
    }
    t.setState(StateStopping)   // ← 在锁内修改状态
    t.mu.Unlock()
    // ...
}
```

**结论**：`setState(StateStopping)` 在持锁期间执行，第二个并发 `Stop()` 在 `Lock()` 后会看到 `StateStopping` 并返回错误。**并发竞态实际不存在**，当前代码逻辑正确。

---

## 一、P0 — 生产阻塞（立即修复）

> 这些 bug 在生产环境中会导致资源泄漏、监控数据错误或数据丢失。必须在合入主分支前修复。

### P0-1：Task 启动失败时的资源泄漏

| 属性 | 值 |
|------|-----|
| **报告章节** | 1.1 |
| **文件** | `internal/task/manager.go:214-216` |
| **严重程度** | 高危 — 已启动的 Reporter goroutine 和连接泄漏 |
| **报告准确性** | ✅ 完全准确 |
| **预计工时** | 2 小时 |

**问题**：`task.Start()` 内部按 `Reporters → Sender → Pipelines → Capturers` 顺序启动。如果第 N 个 Reporter 启动失败，前 N-1 个已启动的 Reporter 不会被清理。`manager.go` 直接返回错误，Task 对象被丢弃。

**修复方案**：在 `task.Start()` 内部实现回滚（而非 manager 侧），保持单一职责：

```go
// internal/task/task.go — Start() 方法内
// Step 1: Start Reporters (data sinks)
startedReporters := 0
for i, rep := range t.Reporters {
    if err := rep.Start(t.ctx); err != nil {
        // Rollback: stop already-started reporters
        rollbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        for j := 0; j < startedReporters; j++ {
            if stopErr := t.Reporters[j].Stop(rollbackCtx); stopErr != nil {
                slog.Error("rollback: failed to stop reporter",
                    "task_id", t.Config.ID, "reporter_id", j, "error", stopErr)
            }
        }
        t.setState(StateFailed)
        t.failureReason = fmt.Sprintf("reporter[%d] start failed: %v", i, err)
        return fmt.Errorf("reporter[%d] start failed: %w", i, err)
    }
    startedReporters++
}
```

**补充**：`manager.go:214-216` 处也应增加 `task.cancel()` 调用以释放 context 资源：

```go
if err := task.Start(); err != nil {
    task.cancel()  // 释放 context
    return fmt.Errorf("task start failed: %w", err)
}
```

**测试用例**：
- `TestTask_StartFailureRollback` — 第 3 个 Reporter 启动失败时，验证前 2 个被 Stop()
- `TestTask_StartFailureState` — 验证 Task State 变为 Failed

---

### P0-2：statsCollectorLoop Delta 计算错误（多 Capturer 共享变量）

| 属性 | 值 |
|------|-----|
| **报告章节** | 1.2 + 2.2 |
| **文件** | `internal/task/task.go:475-521` |
| **严重程度** | 高危 — Prometheus 指标完全错误 |
| **报告准确性** | ✅ Bug 描述准确 |
| **预计工时** | 2 小时 |

**问题**：`lastPacketsReceived` / `lastPacketsDropped` 是单一变量，在 `for i, cap := range t.Capturers` 循环中被共享。第 2 个 Capturer 的 delta 基于第 1 个 Capturer 的基线值计算，结果完全错误。Binding 模式下（多 Capturer）必现。

**修复方案**：改用 per-Capturer 的 map 存储上次统计值，同时增加 uint64 下溢保护：

```go
func (t *Task) statsCollectorLoop() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    type capStats struct {
        packetsReceived uint64
        packetsDropped  uint64
    }
    lastStats := make([]capStats, len(t.Capturers))

    for {
        select {
        case <-t.ctx.Done():
            return
        case <-ticker.C:
            for i, cap := range t.Capturers {
                stats := cap.Stats()

                // 下溢保护（Capturer 重启/计数器重置）
                deltaReceived := stats.PacketsReceived - lastStats[i].packetsReceived
                if stats.PacketsReceived < lastStats[i].packetsReceived {
                    deltaReceived = stats.PacketsReceived // 视为从零开始
                }

                deltaDropped := stats.PacketsDropped - lastStats[i].packetsDropped
                if stats.PacketsDropped < lastStats[i].packetsDropped {
                    deltaDropped = stats.PacketsDropped
                }

                if deltaReceived > 0 {
                    ifaceName, _ := t.Config.Capture.Config["interface"].(string)
                    metrics.CapturePacketsTotal.WithLabelValues(
                        t.Config.ID, ifaceName,
                    ).Add(float64(deltaReceived))
                }
                if deltaDropped > 0 {
                    metrics.CaptureDropsTotal.WithLabelValues(
                        t.Config.ID, "capture",
                    ).Add(float64(deltaDropped))
                }

                lastStats[i] = capStats{
                    packetsReceived: stats.PacketsReceived,
                    packetsDropped:  stats.PacketsDropped,
                }
            }
        }
    }
}
```

**测试用例**：
- `TestStatsCollector_MultipleCapturers` — 3 个 Capturer 分别产生不同的计数，验证各 delta 独立
- `TestStatsCollector_CounterReset` — 模拟计数器从 10000 重置为 0，验证下溢保护

---

### P0-3：Stop() 中 cancel() 与 close(sendBuffer) 的顺序导致残余包丢失

| 属性 | 值 |
|------|-----|
| **报告章节** | 1.3 |
| **文件** | `internal/task/task.go:256-258` |
| **严重程度** | 高危 — 优雅关闭时数据丢失 |
| **报告准确性** | ⚠️ 描述的 panic 场景不存在（见勘误 1），但确实存在数据丢失问题 |
| **预计工时** | 1 小时 |

**问题**：当前 Stop() 步骤 4：
```go
t.cancel()              // ctx 取消 → Report(t.ctx, ...) 将返回 context.Canceled
close(t.sendBuffer)     // senderLoop 开始退出
```

`senderLoop` 是 `for pkt := range t.sendBuffer`，不响应 ctx.Done()。`close(t.sendBuffer)` 触发退出。但在 drain 过程中 `rep.Report(t.ctx, &pkt)` 已经收到取消的 ctx，剩余包无法成功发送。

**修复方案**：调换 cancel 与 close 的顺序，确保 senderLoop 使用有效 ctx 完成 drain：

```go
// Step 4: Close sendBuffer (安全：pipelineWg.Wait() 确保无写者)
close(t.sendBuffer)

// Step 5: Wait for sender to finish draining
<-t.doneCh

// Step 6: Now cancel context (senderLoop 已退出)
t.cancel()

// Step 7: Flush and stop reporters...
```

这样 senderLoop 可以用有效 ctx 向 Reporter 发送所有残余包，然后 sendBuffer 关闭 → for-range 退出 → doneCh 关闭 → 最后 cancel context。

**测试用例**：
- `TestTask_StopDrainsRemainingPackets` — 停止前向 sendBuffer 注入 N 个包，验证 Reporter 收到全部 N 个

---

## 二、P1 — 短期优化（2 周内）

### P1-1：Daemon.Stop() 未清理信号处理器

| 属性 | 值 |
|------|-----|
| **报告章节** | 1.4 |
| **文件** | `internal/daemon/daemon.go:174, 122-165` |
| **严重程度** | 中危 — goroutine 泄漏 |
| **报告准确性** | ✅ 准确 |
| **预计工时** | 0.5 小时 |

**修复方案**：

1. 将 `sigChan` 从 `Run()` 局部变量提升为 `Daemon` 结构体字段
2. 在 `Stop()` 末尾调用 `signal.Stop(d.sigChan)`

```go
type Daemon struct {
    // ... existing fields ...
    sigChan chan os.Signal
}

func (d *Daemon) Run() error {
    d.sigChan = make(chan os.Signal, 1)
    signal.Notify(d.sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
    // ...
}

func (d *Daemon) Stop() {
    // ... existing cleanup ...
    if d.sigChan != nil {
        signal.Stop(d.sigChan)
    }
}
```

---

### P1-2：dispatchLoop 零除防护

| 属性 | 值 |
|------|-----|
| **报告章节** | 2.1 |
| **文件** | `internal/task/task.go:315` |
| **严重程度** | 中危 — 虽然 NewTask 有 `numPipelines >= 1` 保护，但防御性编程应在使用处也加 guard |
| **报告准确性** | ✅ 准确（虽然当前触发概率低） |
| **预计工时** | 0.5 小时 |

**修复方案**：在 `dispatchLoop` 入口添加 guard：

```go
func (t *Task) dispatchLoop() {
    numPipelines := len(t.rawStreams)
    if numPipelines == 0 {
        slog.Error("dispatchLoop: no pipelines configured", "task_id", t.Config.ID)
        return
    }
    // ...
}
```

---

### P1-3：ReporterWrapper — 攒批 + Fallback 架构

| 属性 | 值 |
|------|-----|
| **报告章节** | 3.1 |
| **原始文件** | `plugins/reporter/kafka/kafka.go:255` |
| **严重程度** | 高 — 每包一次 `WriteMessages()`，限制吞吐量至 ~1K pps |
| **预计工时** | 8 小时 |

**架构决策**：在 senderLoop 和 Reporter 插件之间插入 **ReporterWrapper** 层，而非修改各 Reporter 插件内部。这让攒批和降级逻辑集中管理，Reporter 插件保持简洁。

**层次关系**：
```
senderLoop → ReporterWrapper → Reporter 插件
              ├─ 攒批 (batchSize / batchTimeout)
              ├─ 批量调用 Reporter.ReportBatch()
              └─ Fallback 降级 (primary 失败 → fallback Reporter)
```

**新增接口**：

```go
// pkg/plugin/reporter.go — 在现有 Reporter 接口基础上新增可选接口
type BatchReporter interface {
    Reporter
    // ReportBatch 批量发送。支持此接口的 Reporter 获得批处理优势。
    // 不支持的 Reporter 由 Wrapper 逐条调用 Report()。
    ReportBatch(ctx context.Context, pkts []*core.OutputPacket) error
}
```

**ReporterWrapper 实现**：

```go
// internal/task/reporter_wrapper.go
type ReporterWrapper struct {
    primary   plugin.Reporter
    fallback  plugin.Reporter   // 可选，nil 表示无降级
    batchSize int               // 默认 100
    batchTimeout time.Duration  // 默认 50ms
    batchCh   chan *core.OutputPacket
    doneCh    chan struct{}
}

func (w *ReporterWrapper) Start(ctx context.Context) error {
    w.batchCh = make(chan *core.OutputPacket, 10000)
    w.doneCh = make(chan struct{})
    go w.batchLoop(ctx)
    return w.primary.Start(ctx)
}

// Send 由 senderLoop 调用（替代直接调用 Reporter.Report）
func (w *ReporterWrapper) Send(pkt *core.OutputPacket) {
    w.batchCh <- pkt
}

func (w *ReporterWrapper) batchLoop(ctx context.Context) {
    defer close(w.doneCh)
    batch := make([]*core.OutputPacket, 0, w.batchSize)
    ticker := time.NewTicker(w.batchTimeout)
    defer ticker.Stop()

    flush := func() {
        if len(batch) == 0 { return }
        err := w.sendBatch(ctx, batch)
        if err != nil && w.fallback != nil {
            // 降级：逐条发给 fallback Reporter
            for _, pkt := range batch {
                if fbErr := w.fallback.Report(ctx, pkt); fbErr != nil {
                    slog.Warn("fallback reporter also failed", "error", fbErr)
                }
            }
        }
        batch = batch[:0]
    }

    for {
        select {
        case pkt, ok := <-w.batchCh:
            if !ok {
                flush()  // channel 关闭，发送剩余
                return
            }
            batch = append(batch, pkt)
            if len(batch) >= w.batchSize {
                flush()
            }
        case <-ticker.C:
            flush()
        }
    }
}

func (w *ReporterWrapper) sendBatch(ctx context.Context, batch []*core.OutputPacket) error {
    // 优先使用 BatchReporter 接口
    if br, ok := w.primary.(plugin.BatchReporter); ok {
        return br.ReportBatch(ctx, batch)
    }
    // 回退：逐条发送
    var lastErr error
    for _, pkt := range batch {
        if err := w.primary.Report(ctx, pkt); err != nil {
            lastErr = err
        }
    }
    return lastErr
}
```

**Kafka Reporter 适配**：

```go
// plugins/reporter/kafka/kafka.go — 实现 BatchReporter 接口
func (r *KafkaReporter) ReportBatch(ctx context.Context, pkts []*core.OutputPacket) error {
    msgs := make([]kafka.Message, 0, len(pkts))
    for _, pkt := range pkts {
        value, err := r.serializeValue(pkt)
        if err != nil { continue }
        msgs = append(msgs, kafka.Message{
            Topic:   r.resolveTopic(pkt),
            Key:     []byte(fmt.Sprintf("%s:%d-%s:%d", pkt.SrcIP, pkt.SrcPort, pkt.DstIP, pkt.DstPort)),
            Value:   value,
            Headers: r.buildHeaders(pkt),
        })
    }
    return r.writer.WriteMessages(ctx, msgs...)
}
```

**senderLoop 适配**：

```go
// internal/task/task.go — senderLoop 改为调用 Wrapper
func (t *Task) senderLoop() {
    defer close(t.doneCh)
    for pkt := range t.sendBuffer {
        for _, w := range t.reporterWrappers {
            w.Send(&pkt)
        }
    }
    // 关闭所有 wrapper 的 batchCh，等待 flush 完成
    for _, w := range t.reporterWrappers {
        close(w.batchCh)
        <-w.doneCh
    }
}
```

**配置示例**：
```yaml
tasks:
  - id: sip-capture
    reporters:
      - name: kafka
        batch_size: 200
        batch_timeout: 100ms
        fallback: console   # Kafka 不可用时降级到 console
      - name: console
```

---

### P1-4：Channel 容量配置化

| 属性 | 值 |
|------|-----|
| **报告章节** | 2.8 |
| **文件** | `internal/task/task.go:90, 97, 107` |
| **严重程度** | 低 — 当前硬编码值对一般场景足够 |
| **报告准确性** | ✅ 准确（代码中有 TODO 注释） |
| **预计工时** | 2 小时 |

**修复方案**：从 `TaskConfig` 中读取：

```go
// internal/config/task.go
type TaskConfig struct {
    // ... existing fields ...
    ChannelCapacity struct {
        RawStream  int `yaml:"raw_stream" default:"1000"`
        SendBuffer int `yaml:"send_buffer" default:"10000"`
        CaptureCh  int `yaml:"capture_ch" default:"1000"`
    } `yaml:"channel_capacity"`
}

// internal/task/task.go — NewTask()
rawCap := cfg.ChannelCapacity.RawStream
if rawCap <= 0 { rawCap = 1000 }
sendCap := cfg.ChannelCapacity.SendBuffer
if sendCap <= 0 { sendCap = 10000 }
```

---

### P1-5：Pipeline 丢包采样日志

| 属性 | 值 |
|------|-----|
| **报告章节** | 2.4 |
| **文件** | `internal/pipeline/pipeline.go:83-88` |
| **严重程度** | 低 — 影响可观测性 |
| **报告准确性** | ✅ 准确 |
| **预计工时** | 0.5 小时 |

**修复方案**：使用 atomic counter + 采样日志：

```go
// Pipeline 结构体新增字段
dropCount atomic.Uint64

// 在 drop 处
default:
    p.metrics.Dropped.Add(1)
    if p.dropCount.Add(1) % 1000 == 1 {  // 每 1000 次记一条
        slog.Warn("pipeline output full, dropping packets",
            "task_id", p.taskID, "pipeline_id", p.id,
            "total_dropped", p.dropCount.Load())
    }
```

---

### P1-6：IPv4 重组功能实现（移植 BSD-Right 算法）

| 属性 | 值 |
|------|-----|
| **报告章节** | 7.3 |
| **文件** | `internal/core/decoder/reassembly.go`（重写） |
| **严重程度** | 高 — SIP INVITE 消息常超 1500 字节，无重组无法解析分片包 |
| **原始优先级** | P3-5（报告）→ **提升至 P1**（SIP 核心功能前置依赖） |
| **参考实现** | [skywalking-satellite ip4defrag](https://github.com/firestige/skywalking-satellite/blob/feature/sniffer-with-rtp/plugins/fetcher/hep/ip4defrag/defrag.go)（gopacket 官方 fork） |
| **方案** | **方案 A：移植核心算法**，适配 raw bytes + `core.IPHeader` 输入，不引入 gopacket layers 依赖 |
| **预计工时** | 4 小时 |

**背景**：当前 `reassembly.go` 仅有骨架代码 — fragment ID 为占位符 0，offset 固定为 0，实际无法重组任何分片。SIP INVITE（含 SDP body）典型大小 2000-4000 字节，超过 1500 MTU 后必定分片，这是 SIP 抓包的**核心依赖**。

**为什么不直接用 gopacket ip4defrag**：Otus 的 Decoder 手动解析 raw bytes 到 `core.IPHeader`（不用 gopacket `layers.IPv4`）。直接引入 ip4defrag 需要在 raw bytes → `layers.IPv4` → defrag → 转回 `core.IPHeader` 之间来回转换，增加不必要的开销。移植核心算法直接复用已解析的元数据更高效。

> **决策记录**：gopacket 已是项目依赖（Capturer 层使用），但 Decoder 层（`internal/core/decoder/`）当前全部自研手动解析 raw bytes。先保持自研路线实现 defrag。后续如果性能基准测试表明自研 L2-L4 解析与直接用 gopacket layers 无显著差异，则整体切换 Decoder 层回 gopacket（包括 defrag 可直接用 `ip4defrag` 包），统一技术栈。

**移植内容**（从参考实现提取）：

1. **`fragmentList`**：BSD-Right 有序插入策略 + `Highest`/`Current`/`FinalReceived` 计数器判断完整性
2. **`build()`**：前向遍历拼接 payload，处理重叠分片（`startAt` 跳过重复部分）
3. **安全检查**：最小分片大小、最大 offset、分片溢出检测、最大分片列表长度（8192）
4. **flow key**：`(srcIP, dstIP, protocol, fragmentID)` 四元组

**适配改动**（与参考实现的差异）：

| 参考实现 | Otus 适配 |
|----------|----------|
| 输入 `*layers.IPv4` | 输入 raw IP packet bytes + 已解析的 `core.IPHeader` |
| `in.FragOffset`, `in.Flags`, `in.Id` 从 layers 结构读取 | 从 raw bytes 手动解析（offset 4-5: Id, offset 6-7: Flags+FragOffset） |
| `in.Length - 20` 硬编码 IHL | 用 `(data[0] & 0x0F) * 4` 计算实际 IHL |
| `in.Payload` 直接取 | 用 `data[ihl:]` 提取 payload |
| 输出 `*layers.IPv4` | 输出 `[]byte`（重组后的完整 IP payload） |
| `sync.RWMutex` 全局锁 | 保持不变（单 Pipeline 单线程调用，锁竞争低） |
| 无 Prometheus 指标 | 集成 `metrics.ReassemblyActiveFragments` |
| 无超时清理 goroutine | 保留现有 cleanup goroutine（已实现） |

**核心结构**：

```go
// internal/core/decoder/reassembly.go — 重写

const (
    ipv4MinFragSize       = 1
    ipv4MaxSize           = 65535
    ipv4MaxFragOffset     = 8183
    ipv4MaxFragListLen    = 8192
)

// fragmentKey: (srcIP, dstIP, protocol, id) 四元组
type fragmentKey struct {
    srcIP    [4]byte   // 用固定数组避免 string 分配
    dstIP    [4]byte
    protocol uint8
    id       uint16
}

// fragmentList: BSD-Right 有序链表
type fragmentList struct {
    list          list.List
    highest       uint16  // 已知的最大 offset + fragLen
    current       uint16  // 已收到的总字节数
    finalReceived bool    // 是否已收到最后一片（MF=0）
    lastSeen      time.Time
}

type Reassembler struct {
    mu     sync.Mutex
    flows  map[fragmentKey]*fragmentList
    config ReassemblyConfig
}

// Process 接收 raw IP packet bytes（含 IP header），返回重组后的 payload
// 返回值：(payload, complete, error)
//   - 非分片包：直接返回 (payload, true, nil)
//   - 分片未完成：返回 (nil, false, nil)
//   - 分片完成：返回 (reassembledPayload, true, nil)
func (r *Reassembler) Process(ipData []byte, timestamp time.Time) ([]byte, bool, error) {
    // 1. 解析分片相关字段
    id, flags, fragOffset, ihl, totalLen := parseFragInfo(ipData)

    // 2. 快速路径：非分片包直接返回
    if flags&0x1 == 0 && fragOffset == 0 { // MF=0 且 offset=0
        return ipData[ihl:], true, nil
    }

    // 3. 安全检查（移植自参考实现 securityChecks）
    fragSize := totalLen - uint16(ihl)
    if err := r.securityChecks(fragSize, fragOffset, totalLen); err != nil {
        return nil, false, err
    }

    // 4. 查找/创建 flow
    key := buildKey(ipData)
    r.mu.Lock()
    fl, exists := r.flows[key]
    if !exists {
        fl = &fragmentList{}
        r.flows[key] = fl
        metrics.ReassemblyActiveFragments.Inc()
    }
    r.mu.Unlock()

    // 5. BSD-Right 插入 + 完整性检查
    payload := ipData[ihl:totalLen]
    out, err := fl.insert(payload, fragOffset, flags, totalLen-uint16(ihl), timestamp)

    // 6. 超过最大分片数 → 丢弃整个 flow
    if out == nil && fl.list.Len()+1 > ipv4MaxFragListLen {
        r.flush(key)
        return nil, false, fmt.Errorf("fragment list exceeded max size %d", ipv4MaxFragListLen)
    }

    // 7. 重组完成
    if out != nil {
        r.flush(key)
        return out, true, nil
    }

    return nil, false, err
}
```

**Decoder 集成点**：`decoder.go:125-141` 已有 `isIPFragment()` 检查 + `reassembler.Process()` 调用框架，现在 `Process()` 签名从 `(core.IPHeader, []byte, time.Time)` 改为 `([]byte, time.Time)`（直接传入完整的 raw IP bytes），内部自行解析分片字段。

**测试用例**：
- `TestReassembler_SIPFragment` — 构造 3 片 SIP INVITE 包（~3000 字节），验证重组后 payload 完整
- `TestReassembler_OverlappingFragments` — 重叠分片正确处理（BSD-Right: 保留先到的数据）
- `TestReassembler_SecurityChecks` — 畸形分片（过小、过大 offset、溢出）被拒绝
- `TestReassembler_MaxFragListLen` — 超过 8192 片后整 flow 被丢弃
- `TestReassembler_Timeout` — 超时分片被 cleanup goroutine 清理
- `TestReassembler_NonFragment` — 非分片包快速路径直接返回

---

### P1-7：热更新全局配置

| 属性 | 值 |
|------|-----|
| **报告章节** | 7.4 |
| **严重程度** | 中 — 已有 reload 命令和 SIGHUP 处理，但 Reload() 不影响运行中的 Task |
| **原始优先级** | P3-3（报告）→ **提升至 P1**（reload 命令已存在但不完整） |
| **预计工时** | 3 小时 |

**当前状态**：`Daemon.Reload()` 已实现：
- ✅ 加载新配置文件
- ✅ 重新初始化日志
- ❌ 不影响运行中的 Task

**设计决策**：Task 一旦启动就不应被配置变更中断。全局软配置（日志、metrics 间隔等）可热更新，Task 配置变更需要创建新 Task。这是合理的。

**需修复的问题**：

1. **Reload 范围应明确文档化**：当前 Reload() 的注释只说 "reload global configuration"，应明确说明哪些可以 reload、哪些不行
2. **新增可热更新的全局配置项**：
   - 日志级别和格式（✅ 已实现）
   - Metrics 收集间隔（❌ 未实现，statsCollectorLoop ticker 是硬编码 5s）
   - Channel 容量阈值告警（新增可观测性配置）

```go
// internal/daemon/daemon.go — Reload() 增强
func (d *Daemon) Reload() error {
    slog.Info("reloading configuration", "path", d.configPath)

    newConfig, err := config.Load(d.configPath)
    if err != nil {
        return fmt.Errorf("failed to load new config: %w", err)
    }

    // 1. 日志系统热切换（已有）
    d.config = newConfig
    if err := d.initLogging(); err != nil {
        slog.Error("failed to reinitialize logging", "error", err)
    }

    // 2. Metrics 间隔更新（新增）
    d.taskManager.UpdateMetricsInterval(newConfig.Metrics.CollectInterval)

    // 3. 明确记录不可热更新项
    if newConfig.Node.Hostname != d.config.Node.Hostname {
        slog.Warn("node.hostname changed, requires daemon restart")
    }

    slog.Info("configuration reloaded",
        "hot_reloaded", []string{"log", "metrics_interval"},
        "requires_restart", []string{"node.hostname", "tasks"})

    return nil
}
```

**测试用例**：
- `TestDaemon_ReloadLogLevel` — reload 后日志级别变更生效
- `TestDaemon_ReloadPreservesRunningTasks` — reload 不影响正在运行的 Task

---

### P1-8：补充关键单元测试

| 属性 | 值 |
|------|-----|
| **报告章节** | 4.3, 附录 C |
| **预计工时** | 6 小时（不含集成测试） |

**优先补充清单**（按 P0/P1 修复对应关系）：

| 测试 | 覆盖的修复项 | 文件 |
|------|-------------|------|
| `TestTask_StartFailureRollback` | P0-1 | `internal/task/task_test.go` |
| `TestStatsCollector_MultipleCapturers` | P0-2 | `internal/task/task_test.go` |
| `TestStatsCollector_CounterReset` | P0-2 | `internal/task/task_test.go` |
| `TestTask_StopDrainsRemaining` | P0-3 | `internal/task/task_test.go` |
| `TestDispatchLoop_ZeroPipelines` | P1-2 | `internal/task/task_test.go` |
| `TestDispatchLoop_HashDistribution` | 现有代码 | `internal/task/task_test.go` |
| `TestReporterWrapper_BatchFlush` | P1-3 | `internal/task/reporter_wrapper_test.go` |
| `TestReporterWrapper_Fallback` | P1-3 | `internal/task/reporter_wrapper_test.go` |
| `TestReassembler_SIPFragment` | P1-6 | `internal/core/decoder/reassembly_test.go` |

---

## 三、P2 — 中期改进（下一版本）

### P2-1：Plugin Registry 泛型重构

| 属性 | 值 |
|------|-----|
| **报告章节** | 4.1 |
| **文件** | `pkg/plugin/registry.go` |
| **预计工时** | 4 小时 |

**方案**：使用 Go 泛型合并 4 套重复注册逻辑为 `Registry[T]`。当前代码可工作且稳定，优先级不高。

---

### P2-2：FlowRegistry Count() O(1) 优化

| 属性 | 值 |
|------|-----|
| **报告章节** | 2.5 |
| **文件** | `internal/task/flow_registry.go:53-62` |
| **预计工时** | 1 小时 |

**方案**：新增 `atomic.Int64` 计数器，在 `Set` / `Delete` / `Clear` 时维护。

**注意**：`Count()` 当前仅在调试路径调用，非性能关键路径。可先加注释说明复杂度，延后优化。

---

### P2-3：Kafka Consumer 关闭健壮性

| 属性 | 值 |
|------|-----|
| **报告章节** | 2.3 |
| **文件** | `internal/daemon/daemon.go:125-131` |
| **预计工时** | 0.5 小时 |

**方案**：`Stop()` 出错时仍确保底层资源释放。

---

### P2-4：新增 Prometheus 指标

| 属性 | 值 |
|------|-----|
| **报告章节** | 1.5, 附录 B |
| **预计工时** | 3 小时 |

**新增指标清单**：

| 指标 | 类型 | 说明 |
|------|------|------|
| `otus_reporter_batch_size` | Histogram | Kafka 批次大小分布（P1-3 后可加） |
| `otus_reporter_errors_total` | Counter | 按 reporter 名称和错误类型分标签 |
| `otus_flow_registry_size` | Gauge | FlowRegistry 当前 flow 数量 |

---

## 四、P3 — 长期规划（未来版本）

### P3-1：插件生命周期扩展（Pause / Resume / Reconfigure）

- **报告章节**：5.2
- **影响**：支持不停服更新过滤规则、动态调整 Kafka topic
- **预计工时**：16 小时

### P3-2：Dispatcher 策略模式

- **报告章节**：5.3
- **影响**：支持 round-robin、weighted 等负载均衡策略
- **预计工时**：12 小时

### P3-3：Per-IP 分片速率限制

- **报告章节**：6.1
- **影响**：防止 IP 分片 DoS 攻击
- **预计工时**：8 小时

---

## 五、搁置项（待条件成熟）

### 搁置-1：移除单任务限制

| 属性 | 值 |
|------|-----|
| **报告章节** | 5.1 |
| **文件** | `internal/task/manager.go:48` |
| **搁置原因** | 多任务需要多个 AF_PACKET handle，当前 Capturer 绑定网卡后会占用 handle。除非使用 AF_PACKET v3 多队列或切换为用户态过滤（放弃 BPF 早期过滤），否则多任务资源冲突。代价过大，暂不实施。 |

---

## 六、不采纳项

以下是报告中提出但经代码核实后认为无需修复的项目：

| 报告章节 | 描述 | 理由 |
|----------|------|------|
| 2.6 | Stop() 并发竞态 | `setState(StateStopping)` 在持锁期间执行，并发 Stop() 已被正确阻止 |
| 2.7 | CanHandle() 性能优化 | 实际不调用 FlowRegistry，性能已达标 |
| 3.1 | flowHash() 替换为 xxhash | 已使用 FNV-1a（~20ns），无需更换 |
| 5.4 | FlowRegistry 泛型化 | key 已用 plugin.FlowKey 类型约束，收益有限 |

---

## 七、实施顺序与里程碑

### 里程碑 1：修复生产阻塞（1 天）

1. P0-1（Task 启动回滚）
2. P0-2（statsCollectorLoop 修复）
3. P0-3（Stop 顺序修复）
4. 上述修复的单元测试

### 里程碑 2：核心功能补全（第 1 周）

1. P1-6（IP 重组 — 移植 BSD-Right 算法）← **SIP INVITE 分片的前置依赖**
2. P1-7（热更新全局配置）
3. P1-1（signal cleanup）
4. P1-2（zero-divide guard）

### 里程碑 3：性能与可观测性（第 2 周）

1. P1-3（ReporterWrapper 攒批 + Fallback）← **最大吞吐量提升项**
2. P1-4（channel config）
3. P1-5（drop sampling log）
4. P1-8（单元测试）

### 里程碑 4：中期改进（下一版本）

1. P2-1（Registry 泛型）
2. P2-2（Count O(1)）
3. P2-3（Kafka consumer 关闭）
4. P2-4（新指标）

### 里程碑 5：长期规划

按优先级逐步安排到后续版本迭代中。

### 里程碑 Final：集成测试（所有功能完成后）

使用 **testcontainers-go** 搭建端到端集成测试：

- 启动真实 Kafka container + 本地 loopback 网卡
- 验证正常抓包场景：发送 SIP 包 → 捕获 → 解析 → Reporter 输出
- 验证 Kafka batch 写入、Fallback 降级
- 在 CI 中运行（需 Docker-in-Docker 或 privileged mode）

---

**请审查此计划后告知修改意见或批准执行。**
