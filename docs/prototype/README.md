# Threadline 产品原型

## 唯一入口

- `index.html`：唯一的可交互产品原型入口。

宽度大于 `720px` 时进入完整桌面/Web 产品；窄视口自动进入 `mobile/` 内部渲染器。不要直接分发内部页面，也不要再创建产品画板、Figma 生成器或第二套视觉稿入口。

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

## Artifact 协作决策原型（#183）

这是一次明确的 throwaway 产品决策比较，不改变默认 `task-result` 页面，也不引入协议、权限或已冻结计划的改动。

- `?screen=task-result&prototype=artifact&variant=A`：审查工作台；把来源任务、Patch 和决定放在同一处。
- `?screen=task-result&prototype=artifact&variant=B`：Artifact 档案；将任务、验证、审查和修订作为 Artifact 的关联链。
- `?screen=task-result&prototype=artifact&variant=C`：交付接力；以任务 → Artifact → 人工决定 → 下一次 Agent 修订的流程呈现。

底部切换器或左右方向键可切换方案；窄视口会进入内部移动渲染器，保留相同参数并提供可审查的 Artifact 面板。默认路由与正常任务交付页保持不变。

## 已实现交互

- 全局导航和页面深链接。
- 从频道消息创建 Agent 任务。
- 发送频道消息。
- 审批一次性 Capability Grant。
- 接受 Agent 交付并创建 PR。
- 模拟 Runtime 离线和同步序列缺口修复。
- Agent Participation Mode、搜索分类和管理导航状态切换。
- 桌面与移动端响应式布局。
