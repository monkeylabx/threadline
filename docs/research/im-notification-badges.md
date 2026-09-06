# IM 通知标记：未读与提及的视觉层级

研究日期：2026-09-06。仅阅读官方文档，未截图、未测量竞品像素、未修改原型。目的是回应 Threadline 导航图标上的 `@1` 与数量叠层难看、难读的问题。

## 官方事实

| 产品 | 普通未读与需要注意的信息 | 位置与计数口径 |
| --- | --- | --- |
| Slack | 侧栏会话名称加粗表示未读；被提及时另有数字标记。 | macOS 应用图标上的点表示未读活动，数字涵盖私信、提及、关键词和所关注线程的回复等；不能把所有数字理解成普通频道消息总量。[官方通知说明](https://slack.com/help/articles/360025446073-Guide-to-Slack-notifications) |
| Discord | Inbox 将 Mentions 与 Unreads 分开。Unreads 包含未静音频道的未读信息，私信和群组私信使用红色数字标记。 | 数字在此明确表达私信消息数量，不能据此推断所有服务器标记的口径。[官方 Inbox FAQ](https://support.discord.com/hc/en-us/articles/360045027712-Inbox-FAQ) |
| Microsoft Teams | 任务栏数字汇总 Chat、Teams、Activity，但三者规则不同。 | 未静音的未读会话按会话计数；频道个人提及、标签提及、关注线程也贡献计数。列表顶部快速视图及折叠分组显示紫色数字；Activity 数字表示上次访问后的新活动，进入 Activity 即清除。[官方计数说明](https://support.microsoft.com/en-us/teams/notifications-settings/catch-up-with-and-manage-badge-count-activity-in-microsoft-teams) |

Discord 曾有通知设置实验：普通未读用频道/服务器旁的小点；仅提及模式下普通活动更弱，提及时出现红色标记。该资料当前仍在搜索索引中，但直接打开返回 404，且标题明确是实验，**不作为现行全量产品事实或实现依据**。[历史实验资料](https://support.discord.com/hc/en-us/articles/21084266106775-New-Server-Notification-Settings-Experiment)

## 官方设计与无障碍依据

- Microsoft Fluent 2 要求 badge 靠近其描述对象；允许放在对象上方或旁边，并非必须覆盖图标。同一上下文避免混用尺寸，颜色应有明确优先级。仅图标的 badge 信息需进入无障碍名称；如果附属于另一组件，应由该组件的名称表达。[Fluent 2 Badge](https://fluent2.microsoft.design/components/web/react/core/badge/usage)
- 状态不能仅靠颜色区分，还需要文字、形状等可见线索；给屏幕阅读器添加名称并不能替代这一视觉要求。[W3C：Use of Color](https://www.w3.org/WAI/WCAG22/Understanding/use-of-color)

## 对 Threadline 的建议（设计判断，不是竞品事实）

1. **导航图标不叠 `@1` 或极小数字。** 全局入口只承担“这里有需关注内容”的提示；必要时在图标旁的固定位置使用单一状态点，让图标轮廓保持完整。
2. **普通未读更安静，精确数量移到对象旁。** 会话名称加重表达普通未读；仅确有意义的数量放在会话行末端的固定槽位，不压在频道图标上。同类 badge 用统一可读字号与高度，而不是为塞进图标不断缩小。
3. **提及语义与数量分开。** 沿用用户要求，不新增独立“提及”导航入口；精确提及数量留在频道行，必要时通过短标签及提示解释。不要把 `@` 与数字挤成微型复合符号，也不要同时堆普通未读数与提及数。
4. **明确数的定义和清除行为。** “未读会话数”“未读消息数”“未读提及数”择一说明，不能互换；不要直接相加有重叠的未读与提及。阅读、标为已读、静音后的变化要与口径一致。
5. **保留无障碍与移动端语义。** 入口名称可表达“提及，1 条未读”；列表项可表达“产品讨论，有未读消息”。数字/点本身不必成为独立焦点；在窄视图也保留同样语义。

结论：最值得借鉴的是 Slack 的“普通未读与直接相关提醒分层”，而不是把 Teams 或 Discord 的图标角标照搬到 Threadline。移除导航图标上的微型复合计数，保留清晰的列表级提示，更符合安静、工作导向的 IM。

## 交接

- Identity: #183 产品原型调研；分支 `codex/183-ui-v11`；Base `d4aa6b6`，Head 为本文所在提交。
- Outcome: 完成官方资料对比与建议；未修改原型，也未替用户确认头像对齐方案。
- Surfaces/contracts/migrations: 仅本文；无契约、迁移或依赖变化。
- Verification: 官方来源查阅；当前原型 DOM 确认导航角标文字为 6px、频道标记为 7px；`git diff --check` 通过。
- Security/data: 只读公开资料及本地原型，无新权限或用户数据传输。
- Risks/Next: 未验证竞品现行客户端像素样式；下一步由用户确认标记方案后实施，无技术阻塞。
