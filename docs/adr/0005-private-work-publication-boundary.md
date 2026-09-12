---
status: proposed
date: 2026-09-06
---

# ADR-0005: 私人工作与团队 Task 分开，以显式成果发布连接

V33 产品评审确认 Threadline 是 IM + Agent Runtime，个人工作与团队协作都保留。建议将 **Private Work**
作为不同于共享 Task Thread 的个人工作边界；用户选定 Artifact 版本、交付说明和目标会话后显式发布，
而不是把私人 Agent 会话的可见性直接扩大到频道。这使团队能接手成果，同时保留作者的私人制作过程。

## 状态与已有契约

产品交互已获用户确认；本 ADR 的存储、协议和权限实施方案仍为 **Proposed**，不是新协议的批准或实现证据。
[ADR-0003](./0003-group-e2ee-recovery.md) 的“Task Thread 继承 Channel/DM，可见性不能在同一 Thread 内私自收窄”继续成立。
Private Work 不是 Channel 内的隐藏 Task，也不能只靠前端隐藏共享任务来模拟隐私。本提案不更改 ADR-0001 的进程边界、
ADR-0002 的七个服务端工作负载或 ADR-0003/0004 的密码规则。

## 建议边界

- IM 消息、频道和私聊与 Agent 执行独立；私人工作可以没有来源频道。引用消息仍需逐次复检可读范围和 Context 授权。
- 私人 Prompt、草稿、目录和工具过程不因引用频道而公开；运行状态也不能自动投影到团队频道。Presence 与 Run 状态不是同一件事。
- 共享 Task 保留团队内获权观察、审批和交付；私人工作不应强迫进入共享 Task。选择哪条路径必须在发起时清楚表达。
- 发布前明确接收会话、版本、说明及来源披露范围，复检来源分享策略、目标发布权与有效密码受众。可读来源不等于可向任意频道转发。
- 接收者只获得已发布内容及允许披露的来源信息；修订请求不授予私人过程访问权，下一版仍需显式发布。
- 普通成员与管理员身份本身不授予私人内容访问权。保留既有合规与恢复边界，不新增绕过授权的“监控开关”或付费隐私特权。

## 考虑过的替代方案

- 所有工作都从共享 Agent Chat 开始：团队能中途参与，但会公开个人试错，也把普通 IM 的注意力转移到 Agent 对话。
- 在 Channel 的 Task Thread 内加一个 private 标记：表面简单，却与父会话的密码受众继承冲突，不能靠 ACL 或隐藏 UI 补救。
- 做完只贴链接或截图：保留私人过程，但丢失可持续审查、版本与团队决定的连接；本产品应验证是否确实降低了这部分交接成本。

## 实施前必须补齐

| 工作 | Owner | Gate / 状态 |
| --- | --- | --- |
| Private Work 标识、持久化、恢复/保留与多设备语义 | Client-core / Runtime / Security | 未设计完成；本地原型文案不是存储承诺 |
| Publication 的版本、受众、幂等、失败/重试与撤权契约 | Contracts / Core / Crypto | 未定义；不得复用已有共享 Task 接口伪造隐私 |
| 私人过程与团队投影、通知、搜索、审计的数据最小化 | Runtime / Core / Security | 需负向测试与字段审查 |
| Desktop 与 Mobile 入口、离线及跨平台授权表达 | Desktop / Mobile | V33 为桌面静态交互；真机与后端 NOT RUN |
| 更新 Frozen Scope、AC 场景与工作包排期 | Product / Architecture / Integration | 单独评审；本提案不静默更改既有 M0/M2 完成标准 |

拟议验收见 [私人工作与发布场景](../acceptance/private-work-publication.md)。商业采用和付费意愿仍待真实团队验证。
