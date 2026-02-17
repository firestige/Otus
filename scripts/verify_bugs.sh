#!/bin/bash
# verify_bugs.sh - 自动验证 CODE_REVIEW.md 中提到的 Bug

echo "========================================="
echo "   Otus Bug 验证脚本"
echo "========================================="
echo ""
echo "基于: docs/CODE_REVIEW.md"
echo "分支: $(git branch --show-current)"
echo "提交: $(git rev-parse --short HEAD)"
echo ""
echo "========================================="
echo ""

ISSUES_FOUND=0
ISSUES_FIXED=0

# Bug #5: IP 分片重组
echo "🔍 Bug #5: IP 分片重组 (reassembly.go:69-71, 98-101)"
if grep -q "id: 0," internal/core/decoder/reassembly.go 2>/dev/null && \
   grep -q "offset := uint16(0)" internal/core/decoder/reassembly.go 2>/dev/null; then
    echo "   ❌ 仍存在 - Fragment ID 和 offset 为硬编码值"
    echo "      文件: internal/core/decoder/reassembly.go"
    echo "      - Line 71: id: 0 (应从 IP header 提取)"
    echo "      - Line 100: offset := uint16(0) (应从 IP header 提取)"
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
else
    echo "   ✅ 可能已修复 - 请人工确认 IP header 解析逻辑"
    ISSUES_FIXED=$((ISSUES_FIXED + 1))
fi
echo ""

# Bug #1: statsCollectorLoop
echo "🔍 Bug #1: statsCollectorLoop Goroutine 泄漏 (task.go:206)"
if grep -B2 "go t.statsCollectorLoop()" internal/task/task.go 2>/dev/null | grep -q "pipelineWg.Add"; then
    echo "   ✅ 已修复 - 已添加 WaitGroup.Add"
    ISSUES_FIXED=$((ISSUES_FIXED + 1))
else
    echo "   ❌ 仍存在 - statsCollectorLoop 未被 WaitGroup 跟踪"
    echo "      文件: internal/task/task.go:206"
    echo "      建议: 在 'go t.statsCollectorLoop()' 前添加 pipelineWg.Add(1)"
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
fi
echo ""

# Bug #2: senderLoop
echo "🔍 Bug #2: senderLoop Goroutine 泄漏 (task.go:177)"
if grep -B2 "go t.senderLoop()" internal/task/task.go 2>/dev/null | grep -q "pipelineWg.Add"; then
    echo "   ✅ 已修复 - 已添加 WaitGroup.Add"
    ISSUES_FIXED=$((ISSUES_FIXED + 1))
else
    echo "   ❌ 仍存在 - senderLoop 未被 WaitGroup 跟踪"
    echo "      文件: internal/task/task.go:177"
    echo "      影响: Stop() 可能在数据发送完成前返回"
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
fi
echo ""

# Bug #3: captureLoop (binding mode)
echo "🔍 Bug #3: captureLoop Goroutine 泄漏 (task.go:194)"
if grep -B2 "go t.captureLoop(cap, t.rawStreams" internal/task/task.go 2>/dev/null | grep -q "Add(1)"; then
    echo "   ✅ 已修复 - captureLoop (binding) 已被跟踪"
    ISSUES_FIXED=$((ISSUES_FIXED + 1))
else
    echo "   ❌ 仍存在 - captureLoop (binding mode) 未被跟踪"
    echo "      文件: internal/task/task.go:194"
    echo "      影响: AF_PACKET socket 文件描述符泄漏"
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
fi
echo ""

# Bug #4: dispatchLoop
echo "🔍 Bug #4: dispatchLoop Goroutine 泄漏 (task.go:200)"
if grep -B2 "go t.dispatchLoop()" internal/task/task.go 2>/dev/null | grep -q "Add(1)"; then
    echo "   ✅ 已修复 - dispatchLoop 已被跟踪"
    ISSUES_FIXED=$((ISSUES_FIXED + 1))
else
    echo "   ❌ 仍存在 - dispatchLoop 未被跟踪"
    echo "      文件: internal/task/task.go:200"
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
fi
echo ""

# Bug #6: AFPacket 双重关闭
echo "🔍 Bug #6: AFPacket Handle 双重关闭 (afpacket.go:141,166)"
STOP_CLOSE=$(grep -c "c.handle.Close()" plugins/capture/afpacket/afpacket.go 2>/dev/null)
if [ "$STOP_CLOSE" -ge 2 ]; then
    echo "   ❌ 仍存在 - 发现 $STOP_CLOSE 处 handle.Close() 调用"
    echo "      文件: plugins/capture/afpacket/afpacket.go"
    echo "      - Stop() method 和 Capture() defer 都调用 Close()"
    echo "      风险: 双重关闭可能导致 panic"
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
else
    echo "   ✅ 可能已修复 - Close() 调用数量: $STOP_CLOSE"
    ISSUES_FIXED=$((ISSUES_FIXED + 1))
fi
echo ""

# Bug #7: 整数下溢
echo "🔍 Bug #7: 整数下溢 - 统计增量计算 (task.go:488-489)"
if grep -A5 "deltaReceived :=" internal/task/task.go 2>/dev/null | grep -q "if.*>=.*lastPacketsReceived"; then
    echo "   ✅ 已修复 - 已添加下溢保护"
    ISSUES_FIXED=$((ISSUES_FIXED + 1))
else
    echo "   ❌ 仍存在 - 无符号整数相减无保护"
    echo "      文件: internal/task/task.go:488-489"
    echo "      风险: 计数器重置时产生巨大增量值"
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
fi
echo ""

# Bug #8: 启动失败清理
echo "🔍 Bug #8: 启动失败时资源未清理 (manager.go:214-216)"
if grep -A5 "task.Start()" internal/task/manager.go 2>/dev/null | grep -q "task.Stop()"; then
    echo "   ✅ 已修复 - 失败时调用 task.Stop() 清理"
    ISSUES_FIXED=$((ISSUES_FIXED + 1))
else
    echo "   ❌ 仍存在 - Start() 失败时未清理资源"
    echo "      文件: internal/task/manager.go:214-216"
    echo "      影响: Capturers/Reporters 资源泄漏"
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
fi
echo ""

# Bug #9: 单任务限制
echo "🔍 Bug #9: 单任务限制 (manager.go:48-49)"
if grep -q "len(m.tasks) >= 1" internal/task/manager.go 2>/dev/null; then
    echo "   ❌ 仍存在 - 硬编码单任务限制"
    echo "      文件: internal/task/manager.go:48-49"
    echo "      影响: 无法多任务并行"
    ISSUES_FOUND=$((ISSUES_FOUND + 1))
else
    echo "   ✅ 已修复 - 单任务限制已移除"
    ISSUES_FIXED=$((ISSUES_FIXED + 1))
fi
echo ""

# 额外检查：Reassembler cleanup goroutine 泄漏
echo "🔍 额外检查: Reassembler cleanup() Goroutine 泄漏"
if grep -A20 "func (r \*Reassembler) cleanup()" internal/core/decoder/reassembly.go 2>/dev/null | grep -q "ctx.Done()"; then
    echo "   ✅ 已修复 - cleanup() 有停止机制"
    ISSUES_FIXED=$((ISSUES_FIXED + 1))
else
    echo "   ⚠️  可能存在 - cleanup() 可能无停止机制"
    echo "      文件: internal/core/decoder/reassembly.go:162-186"
fi
echo ""

# 总结
echo "========================================="
echo "   验证总结"
echo "========================================="
echo ""
echo "  发现的问题: $ISSUES_FOUND"
echo "  已修复问题: $ISSUES_FIXED"
echo "  总计检查: $((ISSUES_FOUND + ISSUES_FIXED))"
echo ""

if [ $ISSUES_FOUND -eq 0 ]; then
    echo "✅ 恭喜！所有检查的问题都已修复"
    echo ""
    exit 0
else
    echo "⚠️  仍有 $ISSUES_FOUND 个问题需要修复"
    echo ""
    echo "详细信息请参考:"
    echo "  - docs/CODE_REVIEW.md (完整审查报告)"
    echo "  - docs/BUG_VERIFICATION.md (验证清单)"
    echo ""
    exit 1
fi
