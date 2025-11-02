---
name: "Vertical slice — Pipeline core + interfaces + unit tests"
about: "定义并实现 Source/Processor/Sink 接口与核心调度逻辑，确保纯逻辑可单元测试，并为后续真实实现留出 adapter seam。"
title: "[slice][pipeline] "
labels: ["vertical-slice","pipeline","design"]
assignees: ["firestige"]
---

描述
- 目标：设计 pipeline 的最小可运行内核：定义 Source/Processor/Sink 接口（Go 接口），实现 fake 的各组件，并保证 processor 的纯函数可单元测试。

用户故事
- 作为开发者，我希望 pipeline 的业务逻辑可以通过单元测试验证，且不同 source/sink 可互换。

验收标准
- 有明确的 Go 接口（或等价接口定义）：
  - Source: Start(ctx) (<-chan Packet, error)、Stop()
  - Processor: Process(Packet) (Packet, error)
  - Sink: Send(Packet) error、Close()
- 至少一个 processor 的单元测试覆盖常见输入/边界（含 error case）。
- 在集成中，fake source -> processor -> sink 的数据流通畅且可断言。

任务拆分
- [ ] 设计 Packet 数据模型（必要字段，便于测试）。
- [ ] 实现 interfaces 文件与简单的 runner。
- [ ] 实现并测试至少 1 个 processor 的纯逻辑（例如 counter/transform）。
- [ ] fake source（产生 N 条 Packet）与 file sink（写行到文件）的实现与测试。

估时
- 2–4 小时

测试说明
- 强制将业务逻辑移动到可独立测试的函数/方法，I/O 由 adapter 封装并通过 fake 替代.