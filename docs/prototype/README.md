# Threadline 产品原型

## 唯一入口

- `index.html`：统一的可交互产品原型入口。
- `overview.html`：从统一原型维护的产品画板。

宽度大于 `720px` 时进入完整桌面/Web 产品；窄视口自动进入移动端渲染器。`visual-v2/` 只是同一产品原型的移动实现目录，不再作为另一套设计稿单独维护。

修改规则：直接更新 HTML 与交互，并在桌面和移动视口完成验证。HTML 原型是唯一评审与交付版本，不再生成或维护 Figma 快照。

## 页面

通过 `?screen=<route>` 直接进入页面：

| Route | 页面 |
| --- | --- |
| `channel` | 频道协作 |
| `inbox` | 个人动态（提及、回复与 Agent 结果） |
| `search` | 全局检索 |
| `tasks` | Agent 任务执行现场 |
| `approvals` | 风险审批 |
| `task-result` | 任务交付与 Diff |
| `files` | 文件与产物 |
| `agents` | Agent 目录与权限 |
| `runtime` | Runtime 设备与健康 |
| `sync` | 同步与恢复 |
| `organization` | 工作空间与组织切换 |
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
