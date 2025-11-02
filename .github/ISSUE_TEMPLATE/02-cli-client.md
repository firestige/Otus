---
name: "Vertical slice — CLI client (submit / status / cancel)"
about: "实现一个轻量 CLI 用于向 daemon 提交 job、查询状态与取消 job，方便本地开发与演示。"
title: "[slice][cli] "
labels: ["vertical-slice","cli","tooling"]
assignees: ["firestige"]
---

描述
- 目标：实现 pcapctl CLI 的核心命令：submit <job.yaml>、status <job_id>、cancel <job_id>，通过 Unix Domain Socket 与 daemon 的 HTTP API 通信。

用户故事
- 作为开发者/运维，我需要通过 CLI 快速提交 job 并查看状态，不必每次用 curl 手动构造请求。

验收标准
- pcapctl submit <file> 成功打印 job_id 并退出（exit 0）。
- pcapctl status <job_id> 显示与 daemon 返回一致的状态和简要 metrics。
- pcapctl cancel <job_id> 将 job 状态置为 cancelled，并返回成功码。
- CLI 的基础集成测试：在 test daemon 环境中 run submit/status/cancel 并断言输出与 exit code。

任务拆分
- [ ] 设计 CLI 子命令接口（flag: --socket, --timeout 等）。
- [ ] 实现 POST /jobs 调用并解析响应（打印 job_id）。
- [ ] 实现 GET /jobs/{id} 与 DELETE /jobs/{id} 的操作。
- [ ] CLI 的集成测试（调用本地 test daemon）。
- [ ] 文档：CLI 使用示例写入 README。

估时
- 1–3 小时

测试说明
- 把 CLI 的 e2e 测试纳入 CI（在单个 job 中启动 daemon 或用 mock server）。
- 单元测试可覆盖请求/响应解析逻辑与错误处理分支。

备注
- CLI 保持轻量，先实现 JSON 输出与人类可读输出两种模式（--json）。