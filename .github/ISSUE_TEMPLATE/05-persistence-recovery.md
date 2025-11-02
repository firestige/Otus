---
name: "Vertical slice — Job persistence & restart recovery"
about: "为 daemon 增加简单持久化：job metadata/state 存盘，在 daemon 重启后能恢复或合理标记状态。"
title: "[slice][persistence] "
labels: ["vertical-slice","persistence","reliability"]
assignees: ["firestige"]
---

描述
- 目标：实现轻量持久化（json/state file 或 bolt/db），提交 job 时写入状态；daemon 重启后读取状态并能决定恢复、重新调度或标记 failed。

用户故事
- 作为运维，我希望 daemon 重启不会完全丢失正在运行的 job 的元信息，能看到历史与状态，或做出可控恢复。

验收标准
- submit 后 job 元数据被写入持久层（例如 /var/lib/pcapd/jobs/{id}.json）。
- daemon 在启动时读取持久层并能列出已知 jobs 与其 last-known state。
- 提交并在重启后能查询状态（若不恢复运行则显示 last-known state 并有 clear 指示）。

任务拆分
- [ ] 选定持久化方案（file json / bolt / sqlite）并实现最小读写 API。
- [ ] 在 job submit / status 更新逻辑中加入持久化步骤（atomic write）。
- [ ] 实现 daemon 启动时的恢复/加载逻辑（先列出并显示，后可扩展为恢复）。
- [ ] 集成测试：submit -> restart daemon -> status 显示已持久化数据。

估时
- 3–6 小时

测试说明
- 持久化和恢复逻辑需有单元测试与一个重启集成测试（模拟 restart）。