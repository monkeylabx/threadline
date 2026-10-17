# Threadline 产品原型

## 唯一入口

- `index.html`：唯一的可交互产品原型入口。

宽度大于 `720px` 时进入完整桌面/Web 产品；窄视口自动进入 `mobile/` 内部渲染器。不要直接分发内部页面，也不要再创建产品画板、Figma 生成器或第二套视觉稿入口。

修改规则：直接更新 HTML 与交互，并在桌面和移动视口完成验证。HTML 原型是唯一评审与交付版本，不再生成或维护 Figma 快照。

## 页面

### 当前分支待评审：V34 两栏布局

用户同意尝试“单侧栏 + 主内容 + 按需详情”的方向。本分支在唯一入口的融合方案 A 中实现 V34，V33 的确认记录保留如下，V34 仍需产品评审。

在本分支仓库根目录运行 `python3 -m http.server 4174 --bind 127.0.0.1 --directory docs/prototype`，打开：

[V34 两栏原型](http://127.0.0.1:4174/?screen=channel&prototype=im-agent-fusion&variant=A&ui=v34&viewport=desktop)。

- 企业切换、消息/工作/待办、上下文列表和个人菜单合到一个侧栏；默认 240px，支持 208–320px 拖动、键盘调节及整体收起。
- 默认展示频道。私人工作仍独立，通过顶部切换访问；搜索与访问历史继续可用。
- 从频道“查看审批”或待办项打开右侧详情。关闭、Escape、点击遮罩可返回原处，焦点回到触发入口。
- 批准/拒绝、确认成果和待办数量仅为内存中的模拟状态，刷新重置。不会访问文件、执行命令或自动发布私人内容。
- 当前 A 视图隐藏历史方案切换浮条，避免遮挡输入框；B/C 历史探索仍可通过 URL 访问。


### 当前已确认：V33（2026-09-06）

从仓库根目录执行 `python3 -m http.server 4175 --directory docs/prototype`，打开：

[V33 消息 / 个人工作 / 处理原型](http://localhost:4175/?screen=channel&prototype=im-agent-fusion&variant=A&ui=v33&viewport=desktop)。

- 第一栏默认104px，拖动88–160px，折叠后48px；企业和个人入口左对齐，应用图标仍可见。
- 第二栏默认180px，拖动112–360px，双击恢复180px；不因第一栏折叠而消失。
- 外层橙点提示有更新；会话行尾深橙数字，无底色，频道图标与私聊头像保留。
- 工作主区有持续 AI 对话、底部固定输入框与显式上下文；频道内容不会自动变成 Agent Session。

这是静态交互样例：发送/创建/发布、会话历史、通知数量、目录与 Runtime 状态不能视为真实后端能力。
V33 确认仅覆盖桌面信息架构；原生窗口、移动端同等交互和完整无障碍仍需独立验证。
`prototype=private-publish`、`prototype=artifact` 及融合方案B/C保留为历史探索，不是同时生效的产品规范。
详见 [产品需求](../product-requirements.md)、[文档一致性审查](../design/v33-documentation-audit.md)。

### 领域路由（不等于常驻一级导航）

通过 `?screen=<route>` 直接进入页面：

| Route | 页面 |
| --- | --- |
| `channel` | 频道协作 |
| `inbox` | 个人动态（提及、回复与 Agent 结果） |
| `search` | 全局检索 |
| `tasks` | Agent 任务执行现场 |
| `approvals` | 风险审批 |
| `task-result` | Artifact 接力、详细审查与下一次 Agent 修订 |
| `files` | 文件与产物 |
| `agents` | Agent 目录与权限 |
| `runtime` | Runtime 设备与健康 |
| `sync` | 同步与恢复 |
| `organization` | 工作空间与组织切换 |
| `admin` | 企业管理后台 |

创建任务弹窗使用 `?screen=channel&modal=task`。

## 已有演示交互（不代表生产实现）

- 全局导航和页面深链接。
- 从频道消息创建 Agent 任务。
- 发送频道消息。
- 审批一次性 Capability Grant。
- 接受 Agent 交付并创建 PR。
- 在 Artifact 交付链中审查完整 Diff、验证证据，并接受或要求下一次 Agent 修订。
- 模拟 Runtime 离线和同步序列缺口修复。
- Agent Participation Mode、搜索分类和管理导航状态切换。
- 桌面与移动端响应式布局。
