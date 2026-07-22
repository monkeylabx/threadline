# Threadline 产品原型

## 入口

- `overview.html`：完整产品画板，汇总桌面和移动端关键帧。
- `index.html`：可交互高保真原型。
- `visual-v2/index.html`：新版视觉基准稿，包含频道、任务 Activity Sheet 与权限审批 Sheet。

新版视觉基准稿通过 `?view=channel`、`?view=task`、`?view=approval` 切换场景。它用于确定最终 UI 语言，在确认前不会覆盖下方完整产品原型。

## 页面

通过 `?screen=<route>` 直接进入页面：

| Route | 页面 |
| --- | --- |
| `organization` | 工作空间与组织切换 |
| `inbox` | 统一收件箱 |
| `channel` | 频道协作 |
| `search` | 全局检索 |
| `files` | 文件与产物 |
| `tasks` | Agent 任务执行现场 |
| `approvals` | 风险审批 |
| `task-result` | 任务交付与 Diff |
| `agents` | Agent 目录与权限 |
| `runtime` | Runtime 设备与健康 |
| `sync` | 同步与恢复 |
| `admin` | 企业管理后台 |

创建任务弹窗使用 `?screen=channel&modal=task`。

## 已实现交互

- 全局导航和页面深链接。
- 从频道消息创建 Agent 任务。
- 发送频道消息。
- 审批一次性 Capability Grant。
- 接受 Agent 交付并创建 PR。
- 模拟 Runtime 离线和同步序列缺口修复。
- Agent Participation Mode、搜索分类和管理导航状态切换。
- 桌面与移动端响应式布局。
