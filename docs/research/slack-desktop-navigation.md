# Slack 桌面导航：可核实事实与 Threadline 设计边界

研究日期：2026-09-06。沿用仓库 `docs/research/` 的中文 Markdown + 来源链接约定。仅使用 Slack 与 Apple 第一方资料；历史设计复盘不作为当前版本的像素规范。

## 结论

Threadline 可以采用「顶部全局导航、左侧组织与功能导航、会话局部工具」的层级。企业头像应进入左侧导航内容区，不挤占 macOS 左上窗口控件。这是基于平台边界的 **Threadline 设计建议**，不能包装成“Slack 从来不会把 workspace 入口放在顶部”的竞品事实。

## 已核实

| 问题 | 第一方证据 | 证据边界 |
| --- | --- | --- |
| macOS 左上角是什么？ | Apple 将关闭、最小化、全屏按钮放在应用窗口左上角；屏幕顶端的系统菜单栏是另一层界面。[^apple-window][^apple-menu] | 因此应为真实窗口控件保留空间；具体预留像素与拖动区域应由 Threadline 宿主决定，不是 Slack 文档给出的标准。 |
| 历史导航与搜索为什么在顶部？ | Slack 的 2020 设计复盘明确说，用户最容易在应用顶部理解 history/navigation；同篇说明为 Mac、Windows 分别调整过顶栏。当前搜索帮助仍将搜索框描述为在 Slack 顶部。[^design-2020][^search] | 支持层级和功能位置，不证明 2026 年每个平台上各按钮的精确顺序与尺寸。 |
| 后退、前进是否为真实导航？ | 官方快捷键表明确列有返回/前进历史：Mac 为 `⌘[` / `⌘]`；Windows/Linux 为 `Alt+←` / `Alt+→`。[^shortcuts] | 应改变导航历史，不是频道列表上一项/下一项，也不是只显示提示。 |
| 企业/workspace 图标在哪里？ | 当前帮助写的是左上 workspace 图标；另页将 workspace switcher 定义为侧边栏外侧的工作区列，或折叠为 navigation bar 顶部的单一 workspace 图标。该页明确 navigation bar 装载 Home、Activity、Later 等标签。[^workspace][^sidebar] | 这里的 navigation bar 是功能导航，不应把“top of navigation bar”误译成“macOS 原生标题栏左上角”。文档也不支持声称 workspace 入口永远不在顶部。 |
| “更多”在哪里？ | 导航偏好中未选为常驻的工具，会出现在侧边栏的 More。[^sidebar] | 不能把 Slack 的侧边导航 More 与 Threadline 顶栏应用菜单混为一谈。顶栏放更多菜单可以是产品选择，但不是本次核实到的 Slack 固定布局。 |
| 折叠的是哪一层？ | 当前帮助明确支持显示/隐藏 workspace switcher；快捷键是 Mac `⌘⇧S`、Windows/Linux `Ctrl+Shift+S`。[^workspace] 2020 年 6 月更新记录另有左侧栏整体折叠快捷键。[^updates] | workspace 列折叠 ≠ 会话列表折叠。2020 年的 `⌘⇧D` / `Ctrl+Shift+D` 只能列为历史行为，本次未从当前快捷键表确认其仍有效。 |

## 应用于 Threadline 的建议（推论，而非 Slack 事实）

1. **窗口层**：macOS 左上保留原生窗口控件区域；企业头像移到其下方的左侧组织/功能栏。浏览器原型不应把假的红黄绿按钮当成功能控件交付。
2. **全局层**：顶部保留侧栏开关、后退、前进、历史、全局搜索；“更多”仅收纳确实存在的应用级操作。通话、频道成员、会话详情属于会话层。
3. **组织层**：企业头像负责工作区身份与切换，不承担收起侧栏；两者是不同动作，应有独立命名与反馈。
4. **折叠契约**：明确开关控制的是会话列表还是整个左侧导航；推荐保留窄功能栏与企业入口，仅收起会话列表，恢复时保留当前会话与草稿。这是 Threadline 的选择，不宣称完全复刻 Slack。
5. **实际行为**：无历史时禁用后退/前进；历史菜单能跳转已访问位置；搜索有结果或明确空态；菜单项执行实际操作。避免按钮只有 toast。

## 尚未验证

- 未对已登录的当前 Slack macOS/Windows 客户端做像素级检查；官方历史配图、帮助文字不能替代最新版客户端实测。
- 未核实“macOS 顶栏必须按侧栏开关、后退、前进、时钟、搜索、更多的唯一顺序排列”，也没有依据要求 Threadline 照抄该顺序。
- 原生宿主的窗口安全区、全屏状态与 Web 浏览器的外层窗口属于不同布局条件；本笔记不提供未经测试的固定安全区数值。

## 来源

[^apple-window]: [Apple：Are you new to Mac? — View and manage windows](https://support.apple.com/en-lamr/guide/macbook-air/apd1f14ec646/mac)，2026-09-06 访问。
[^apple-menu]: [Apple：What’s in the menu bar on Mac?](https://support.apple.com/en-gb/guide/mac-help/-mchlp1446/mac)，2026-09-06 访问。
[^design-2020]: [Slack Design：Designing teamwork: How our customers shaped the future of Slack](https://slack.design/articles/designing-teamwork-how-our-customers-shaped-the-future-of-slack/)，2020 年设计复盘；2026-09-06 访问。文章包含 Mac/Windows 顶栏差异配图，但本文未将该历史图片作为当前客户端截图使用。
[^search]: [Slack：Search in Slack](https://slack.com/help/articles/202528808-Search-in-Slack)，2026-09-06 访问。
[^shortcuts]: [Slack：Slack keyboard shortcuts](https://slack.com/help/articles/201374536-Slack-keyboard-shortcuts-and-commands)，2026-09-06 访问。
[^workspace]: [Slack：Switch between workspaces](https://slack.com/help/articles/1500002200741-Switch-between-workspaces)，2026-09-06 访问。
[^sidebar]: [Slack：Adjust your sidebar preferences](https://slack.com/intl/en-gb/help/articles/212596808-Adjust-your-sidebar-preferences)，2026-09-06 访问。
[^updates]: [Slack：Slack updates and changes — June 2020](https://slack.com/help/articles/115004846068-Slack-updates-and-changes/)，2026-09-06 访问；左栏折叠条目是历史记录。
