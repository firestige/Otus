# Bug 验证清单

**验证日期**: 2026-02-17  
**分支**: copilot/review-source-code-for-bugs  
**基于**: CODE_REVIEW.md 中提出的缺陷

本文档提供了代码审查中提出的关键 Bug 的具体验证步骤和当前状态。

---

## 验证方法

对于审查报告中提到的每个 Bug，本文档提供：
1. 具体的文件路径和行号
2. 当前代码状态（代码片段）
3. 验证命令
4. 修复状态：✅ 已修复 | ❌ 仍存在 | 🔄 部分修复

---

## P0 关键缺陷验证

### Bug #1: Goroutine 泄漏 - statsCollectorLoop 未跟踪

**位置**: `internal/task/task.go:206`

**验证命令**:
```bash
# 查看启动 statsCollectorLoop 的代码
sed -n '205,210p' internal/task/task.go

# 查找是否有 WaitGroup.Add 调用
grep -B2 -A2 "statsCollectorLoop" internal/task/task.go
```

**当前代码** (line 206):
```go
go t.statsCollectorLoop()
```

**验证结果**: ❌ **仍存在**
- 没有 `t.pipelineWg.Add(1)` 调用
- 没有 `defer t.pipelineWg.Done()` 包装
- Stop() 方法等待 `pipelineWg` 时不会等待此协程

---

### Bug #2: Goroutine 泄漏 - senderLoop 未跟踪

**位置**: `internal/task/task.go:177`

**验证命令**:
```bash
# 查看启动 senderLoop 的代码
sed -n '175,180p' internal/task/task.go

# 查找是否有 WaitGroup 跟踪
grep -B5 "go t.senderLoop" internal/task/task.go
```

**当前代码** (line 177):
```go
go t.senderLoop()
```

**验证结果**: ❌ **仍存在**
- 未添加到 WaitGroup
- Stop() 可能在 senderLoop 完成发送前返回

---

### Bug #3: Goroutine 泄漏 - captureLoop 未跟踪

**位置**: `internal/task/task.go:194, 199`

**验证命令**:
```bash
# 查看 binding 模式的 capturer 启动
sed -n '190,196p' internal/task/task.go

# 查看 dispatch 模式的 capturer 启动
sed -n '197,201p' internal/task/task.go
```

**当前代码**:
```go
// Line 194 (binding mode)
go t.captureLoop(cap, t.rawStreams[i])

// Line 199 (dispatch mode)
go t.captureLoop(t.Capturers[0], t.captureCh)
```

**验证结果**: ❌ **仍存在**
- 两种模式的 captureLoop 都未跟踪
- 可能导致文件描述符泄漏（AF_PACKET socket）

---

### Bug #4: Goroutine 泄漏 - dispatchLoop 未跟踪

**位置**: `internal/task/task.go:200`

**验证命令**:
```bash
sed -n '198,202p' internal/task/task.go
```

**当前代码** (line 200):
```go
go t.dispatchLoop()
```

**验证结果**: ❌ **仍存在**
- dispatch 模式下的分发协程未跟踪

---

### Bug #5: IP 分片重组功能未实现

**位置**: `internal/core/decoder/reassembly.go:69-71, 98-101`

**验证命令**:
```bash
# 检查 Fragment ID 提取
sed -n '65,75p' internal/core/decoder/reassembly.go

# 检查 offset 和 moreFragments 解析
sed -n '96,105p' internal/core/decoder/reassembly.go

# 查找所有 TODO
grep -n "TODO" internal/core/decoder/reassembly.go
```

**当前代码**:
```go
// Line 69-71
// TODO: Extract fragment ID from IP header
// For now, use a placeholder
id: 0,

// Line 98-101
// TODO: Parse fragment offset and more fragments flag from IP header
// For now, simplified implementation
offset := uint16(0)
moreFragments := false
```

**验证结果**: ❌ **仍存在**
- Fragment ID 硬编码为 0，所有分片共享同一 key
- offset 硬编码为 0，无法正确拼接分片
- moreFragments 硬编码为 false，无法识别是否为最后分片
- **功能完全不可用**

**正确实现应该**:
```go
// 从 IP header 提取 Fragment ID (bytes 4-5)
fragID := binary.BigEndian.Uint16(ipHeader[4:6])

// 从 IP header 提取 flags 和 offset (bytes 6-7)
flagsAndOffset := binary.BigEndian.Uint16(ipHeader[6:8])
offset := (flagsAndOffset & 0x1FFF) * 8  // 偏移量以 8 字节为单位
moreFragments := (flagsAndOffset & 0x2000) != 0  // MF 标志位
```

---

### Bug #6: AFPacket Handle 双重关闭

**位置**: `plugins/capture/afpacket/afpacket.go:141-142, 166`

**验证命令**:
```bash
# 查看 Stop() 方法
sed -n '135,146p' plugins/capture/afpacket/afpacket.go

# 查看 Capture() 的 defer
sed -n '148,167p' plugins/capture/afpacket/afpacket.go
```

**当前代码**:
```go
// Stop() method - lines 141-142
if c.handle != nil {
    c.handle.Close()
    c.handle = nil
}

// Capture() defer - line 166
defer c.handle.Close()
```

**验证结果**: ❌ **仍存在**
- 如果 Stop() 在 Capture() 运行时被调用：
  1. Stop() 关闭 handle
  2. Stop() 设置 handle = nil
  3. Capture() 的 defer 再次调用 Close()
- 存在竞态条件和双重关闭风险

---

### Bug #7: 整数下溢 - 统计增量计算

**位置**: `internal/task/task.go:488-489`

**验证命令**:
```bash
sed -n '485,510p' internal/task/task.go
```

**当前代码** (lines 488-489):
```go
deltaReceived := stats.PacketsReceived - lastPacketsReceived
deltaDropped := stats.PacketsDropped - lastPacketsDropped
```

**验证结果**: ❌ **仍存在**
- 无符号整数相减，当计数器重置时会下溢
- 导致巨大的增量值（如 0 - 1000 = 18446744073709550616）
- Prometheus 指标会出现异常峰值

**修复方案**:
```go
var deltaReceived, deltaDropped uint64
if stats.PacketsReceived >= lastPacketsReceived {
    deltaReceived = stats.PacketsReceived - lastPacketsReceived
} else {
    deltaReceived = stats.PacketsReceived  // 计数器重置
}
```

---

## P1 高优先级缺陷验证

### Bug #8: 启动失败时资源未清理

**位置**: `internal/task/manager.go:214-216`

**验证命令**:
```bash
sed -n '210,220p' internal/task/manager.go
```

**当前代码**:
```go
if err := task.Start(); err != nil {
    return fmt.Errorf("task start failed: %w", err)
}
```

**验证结果**: ❌ **仍存在**
- 如果 task.Start() 失败，前面已分配的资源不会清理：
  - Capturers 已初始化但未停止
  - Reporters 可能已部分启动
  - Parsers/Processors 已初始化
- task 对象未注册到 manager，无法通过 Delete() 清理

**修复方案**:
```go
if err := task.Start(); err != nil {
    // 清理已分配的资源
    if stopErr := task.Stop(); stopErr != nil {
        slog.Error("failed to cleanup after start failure", 
            "task_id", cfg.ID, "error", stopErr)
    }
    return fmt.Errorf("task start failed: %w", err)
}
```

---

### Bug #9: 单任务限制

**位置**: `internal/task/manager.go:48-49`

**验证命令**:
```bash
sed -n '45,52p' internal/task/manager.go
```

**当前代码**:
```go
// Phase 1 limitation: maximum 1 task
if len(m.tasks) >= 1 {
    return fmt.Errorf("phase 1 limitation: maximum 1 task allowed (current: %d)", len(m.tasks))
}
```

**验证结果**: ❌ **仍存在**
- 硬编码限制为 1 个任务
- 阻止横向扩展
- 注释表明这是已知的临时限制

---

## 验证总结

| Bug ID | 描述 | 状态 | 优先级 |
|--------|------|------|--------|
| #1 | statsCollectorLoop 未跟踪 | ❌ 仍存在 | P0 |
| #2 | senderLoop 未跟踪 | ❌ 仍存在 | P0 |
| #3 | captureLoop 未跟踪 | ❌ 仍存在 | P0 |
| #4 | dispatchLoop 未跟踪 | ❌ 仍存在 | P0 |
| #5 | IP 分片重组未实现 | ❌ 仍存在 | P0 |
| #6 | AFPacket 双重关闭 | ❌ 仍存在 | P0 |
| #7 | 整数下溢 | ❌ 仍存在 | P0 |
| #8 | 启动失败资源泄漏 | ❌ 仍存在 | P1 |
| #9 | 单任务限制 | ❌ 仍存在 | P1 |

**当前状态**: 所有关键缺陷均未修复

---

## 自动验证脚本

可以使用以下脚本自动验证所有问题：

```bash
#!/bin/bash
# verify_bugs.sh

echo "=== Bug 验证脚本 ==="
echo ""

# Bug #5: IP 分片重组
echo "Bug #5: IP 分片重组"
if grep -q "id: 0," internal/core/decoder/reassembly.go && \
   grep -q "offset := uint16(0)" internal/core/decoder/reassembly.go; then
    echo "  ❌ 仍存在 TODO，未实现"
else
    echo "  ✅ 可能已修复，请人工确认"
fi
echo ""

# Bug #1-4: Goroutine 泄漏
echo "Bug #1: statsCollectorLoop"
if grep -A1 "go t.statsCollectorLoop()" internal/task/task.go | grep -q "pipelineWg.Add"; then
    echo "  ✅ 已添加 WaitGroup 跟踪"
else
    echo "  ❌ 未添加 WaitGroup 跟踪"
fi
echo ""

echo "Bug #2: senderLoop"
if grep -A1 "go t.senderLoop()" internal/task/task.go | grep -q "pipelineWg.Add"; then
    echo "  ✅ 已添加 WaitGroup 跟踪"
else
    echo "  ❌ 未添加 WaitGroup 跟踪"
fi
echo ""

# Bug #7: 整数下溢
echo "Bug #7: 整数下溢"
if grep -q "if stats.PacketsReceived >= lastPacketsReceived" internal/task/task.go; then
    echo "  ✅ 已添加下溢保护"
else
    echo "  ❌ 未添加下溢保护"
fi
echo ""

# Bug #8: 启动失败清理
echo "Bug #8: 启动失败清理"
if grep -A3 "task.Start()" internal/task/manager.go | grep -q "task.Stop()"; then
    echo "  ✅ 已添加失败清理"
else
    echo "  ❌ 未添加失败清理"
fi
echo ""

# Bug #9: 单任务限制
echo "Bug #9: 单任务限制"
if grep -q "len(m.tasks) >= 1" internal/task/manager.go; then
    echo "  ❌ 单任务限制仍存在"
else
    echo "  ✅ 单任务限制已移除"
fi
echo ""

echo "=== 验证完成 ==="
```

**使用方法**:
```bash
chmod +x verify_bugs.sh
./verify_bugs.sh
```

---

**最后更新**: 2026-02-17  
**验证基于**: commit bc615d3
