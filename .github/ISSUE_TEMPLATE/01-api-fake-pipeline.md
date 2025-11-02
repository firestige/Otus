---
name: "Vertical slice — API + fake pipeline"
about: "提交 job (UDS/HTTP) 并启动一个使用 fake source/processor/sink 的最小 pipeline，用于端到端验证设计与契约。"
title: "[slice][api] "
labels: ["vertical-slice","api","backend"]
assignees: ["firestige"]
---

描述
- 目标：实现 daemon 在 Unix Domain Socket 上的 POST /jobs 接口，接受 YAML/JSON job，返回 job_id，并启动一个 fake pipeline（fake source 生成 N 条“包”，processor 简单处理，sink 写入文件）。用于验证端到端的请求→执行→可观测输出流程。

用户故事
- 作为 CLI 用户，我希望提交一个抓包 job 并能在完成后查看输出文件，以验证 pipeline 行为。

验收标准 (Acceptance Criteria)
- POST /jobs 返回 201 并包含 job_id。
- GET /jobs/{job_id} 返回 status 字段（running → finished）和基本 metrics（packets_in）。
- 输出文件存在（例如 /tmp/job-{job_id}.out）且包含预期 N 行。
- 有一个自动化集成测试：在测试中以 in-process 或 spawned daemon 提交该 job 并断言输出文件与状态。

任务拆分（可各自作为子任务 issue）
- [ ] 定义最小 Job struct 与 YAML ↔ struct 解析（包含 fake source 配置项）。
- [ ] 实现 HTTP-over-UDS endpoint POST /jobs -> spawn job runner。
- [ ] 实现 in-memory job runner 能启动 goroutine 运行 fake pipeline。
- [ ] fake source / fake processor / file sink 的实现（测试友好）。
- [ ] 集成测试：启动 test daemon、submit job、poll status、断言输出文件。
- [ ] 更新 README/one-pager 以记录 API 使用示例（curl --unix-socket ...）。

估时（粗略）
- 2–6 小时（可拆为多个 1–3 小时的小任务）

测试说明
- 集成测试为关键（acceptance-first）：确保端到端流程可被自动化执行并在 CI 中通过。
- 单元测试覆盖 processor 的纯逻辑部分。

备注 / 风险
- 使用 fake 实现避免早期依赖 pcap 权限或平台差异。后续可替换为真实 source adapter.