---
name: "Spike — 真实抓包接口验证 (libpcap / af_packet)"
about: "快速验证在目标开发环境下能否用 libpcap 或 af_packet 抓取数据，确定权限、性能与 API 可用性。"
title: "[spike][capture] "
labels: ["spike","capture","platform-risk"]
assignees: ["firestige"]
---

描述
- 目标：在受控开发机上做一次短时 spike，读取若干个包并将其送入现有 pipeline（可用 fake sink），以确认能否打开抓包接口与数据格式。

用户故事
- 作为开发者，我需要确认真实抓包在目标环境下的可行性（权限、依赖、性能），以决定后续实现细节。

验收标准
- 成功捕获本机生成的若干个数据包（例如 10 条）并把 Packet 传入 pipeline。
- 能在开发机上运行，无需立即处理高吞吐或长期稳定性。
- 完成 spike 后写出短结论：需要额外权限、依赖、或是否直接可用。

任务拆分
- [ ] 在 dev 环境下选用抓包库（gopacket/libpcap 或 AF_PACKET binding）。
- [ ] 编写 demo 程序读取接口并打印/转为 Packet 结构。
- [ ] 将 demo 与现有 pipeline 的 runner 连接，验证数据能流到 sink。
- [ ] 记录运行说明与遇到的问题（权限、cap_net_raw、依赖版本）。

估时
- 2–6 小时（时间盒）

测试说明
- 该任务是 spike，重在验证与记录，不必写大规模测试.