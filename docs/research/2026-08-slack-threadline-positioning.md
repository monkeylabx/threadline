# Slack 对 Threadline 定位的压力测试

状态：产品/商业命题调研（不是市场规模、定价或付费意愿证据）

调研快照：2026-08-27

范围：只采用 Slack/Salesforce 的第一方产品页、帮助中心和官方博客；不以第三方解读补全功能。

问题：Slack 已是成熟的**人对人 IM**，并在 2026 年加入 Agent、多人 Agent 工作空间和富 Artifact。它实际做到了哪里？Threadline 所说的“人际 IM 为主、个人私下 AI 制作、显式向人际频道交付 Artifact”还剩下什么真实差异和商业命题？

## 结论先行

Slack 不是“只有机器人文字回复的普通 IM”。它已经是 Threadline 的直接产品级反例：

- Slack 的频道本来就是人际协作空间；Canvas、List、Workflow 是频道头部的独立表面，不是只能塞进消息气泡的附件。[^channels][^tabs]
- Agent 可以私聊、被加入频道、在频道中 `@` 对话；频道内一次 Agent 互动可以只让发起人看见，也可以公开给频道。[^agents]
- 新推出的 **Slack Code** 会从频道或 DM 中拉起一个独立、临时的 code channel；人和 Agent 在其中多人计划、Prompt、看状态、看代码 Diff/Canvas/HTML preview、评论并签收，主频道只得到回链和更新。[^slack-code-help][^slack-code]
- Slackbot 也已宣传能生成可交互报告、仪表盘、演示文稿等，并可把实时视图固定到频道；它把自己的定位称为 Slack 的“agentic operating system”。[^slackbot][^agentic-os]

因此 Threadline **不能再主张**：

1. “Slack/IM 中 AI 只能以 Bot 文本或卡片存在。”
2. “Slack 会让复杂的 Agent 工作把人际主频道淹没。”Slack Code 正是为把重型工作移出主频道而设计。
3. “多人看 Agent 过程、审阅 rich output、在同处给反馈”是 Threadline 独有。
4. “Slack 没有私密 AI 操作。”官方明确有只对发起人可见的频道 Agent interaction，也有私有 code channel。

Threadline 仍可有一个**不同的、尚未被验证的产品契约**，但必须把话说准：

> **Slack Code 默认把一群人和 Agent 放进同一个临时制作会话，主张“公开共制”。Threadline 应把个人 AI 制作与团队人际 IM 分为两个默认空间：作者的制作过程默认私有；作者显式发布的 Artifact 才进入人际频道，并在该频道走反馈、接受/退回、流转和后续版本。**

这不是“Slack 没有隐私”，也不是“Threadline 的 preview 更漂亮”。它是一个不同的默认工作边界：**多人共同制作** vs. **个人制作、团队接收交付**。官方资料未明确描述 Slack 是否支持“从一个单人私有 Agent session，选择性地发布一个与原制作会话脱钩、可持续退回同一作者修订的版本化 Artifact 到另一个纯人际频道”。这只是本次资料中**未见明确文档**，绝不是 Slack 没有该能力的证明。

商业定位也必须随之修正。Threadline 不是“团队多 Agent 协作软件”，也不能只用“更现代的 Slack 替代品”这种功能清单来销售；它试图定义一种不同默认契约的完整 IM：

> **个人 Agent 客户端把 AI 劳动留在个人电脑/个人 Chat；团队 IM 只得到截图、链接或一句结论。Threadline 让个人保有制作自由，同时让团队得到一个可接收、可评论、可决定、可继续流转的交付物。**

这仍只是商业假设。Slack 的功能文档不能证明客户会为 Threadline 付费，也不能证明“私下制作后交付”比 Slack Code 的“公开共制”更受欢迎。两种模式适合不同工作；是否存在足够高频的后一类，必须用设计伙伴验证。

## Slack 的实际对象与界面模型

### 1. Slack 的人际 IM 不是只有消息流

Slack 把频道描述为把合适的人与信息聚到一起的透明协作空间；频道用于持续讨论，并可容纳文件、自动化等内容。[^channels] 频道或 DM 的默认 `Messages` tab 之外，可以在头部加入 Canvas、List、Workflow、消息、链接和文件等 tab。[^tabs]

Canvas 是 Slack 内置的富内容表面，不是普通 attachment：它可独立创建、再分享至任意 conversation，也可被放入频道/DM 的 tab；内容支持格式化、表格、栏目、内嵌音视频和文件预览。[^canvas] Canvas 有锚定评论、评论线程、解决评论和版本历史；管理员可控制版本历史与共享。[^canvas][^canvas-settings] Slack 还允许把 Canvas/Lists 设为 invite-only、view/comment/edit、限制转发，或显式共享给某个人或频道。[^canvas-permissions]

这意味着“人际 IM + 一等的富内容/审阅表面”本身，Slack 已经提供。Threadline 的 Artifact 不能只是换一种 Canvas、Tab 或附件样式。

### 2. Agent、Slackbot 与 Agentforce：中心仍是对话/会话

Slack 的官方 Agent 模型是“像另一个 teammate 一样开始对话”：用户可以一对一 DM Agent，也能将 Agent 添加到频道并以 `@` 开始聊天；`Agents & tools` 能恢复既有 session，并显示 multiplayer AI conversation 的状态。[^agents] 在频道中，互动可公开给全体频道成员，也可只让发起者看见。[^agents]

Agentforce 亦如此：管理员把 Agent 加入工作区后，成员在 `Agents` 中发起或查看历史 DM；把 Agent 加入频道后，以 `@` 消息互动。Agentforce 的说明还强调 prompt 可以作为链接分享至频道、Canvas、Workflow 等位置，点击者得到“为自己定制”的回答。[^agentforce]

Slackbot 的官方定位更进一步：它是“personal AI agent for work”，可搜索用户有权访问的 Slack、连接应用与网页并返回带引用的报告；Slack 宣称它能建立 workflow、交互式 dashboard/report/poll/calculator/simulation，或从上下文和模板中制作演示稿、文档。[^slackbot] Slackbot 页面明确写道“你的 interactions 是 private”，并把产品定位为把对话、数据、应用和 Agent 放进同一处的 agentic operating system。[^slackbot][^agentic-os]

这说明 Threadline 不应把“个人 AI 不公开”或“一个 IM 可以产出交互式内容”本身当成差异。Slack 已明确把个人 Agent interaction 和团队内共享的 Canvas/Prompt/频道内容并置。

### 3. Slack Code：最接近、也最需要正面比较的能力

Slack 官方于 2026 年 8 月推出 Slack Code。它把复杂 Agent 工作从普通 thread 中迁出，但做法不是“个人私下制作后发布”，而是创建一个专门的、临时的 **code channel**：

- 可从 Agent 被提及的频道/DM 自动创建，也可在 `Agents & tools` 手动创建；可选 public/private。[^agents][^slack-code-help]
- 团队成员可以加入、交换消息/文件、共同 prompt Agent；官方直接称它为面向 multiplayer use case 的空间。[^slack-code-help]
- 侧边栏会展示 agent 正在工作、需要注意、完成等状态；工作完成可关闭/归档，消息仍可搜索。[^slack-code-help]
- `Artefacts` 可呈现 Agent 生成/更新的代码 Diff、Agent 创建的 Canvas、实时 HTML preview，以及相关文件与链接；代码可按行评论，多条评论可组成 review 交给 Agent 或团队。[^slack-code-help]
- Slack 表述其价值为“团队共同计划、prompt、review，Agent 创建产物”，而且 code channel 向原始消息回传更新，以保持主频道干净。[^slack-code-help][^slack-code]

可以把 Slack 的主路径概括为：

```text
人际频道 / DM
  → @ Agent 或发起 code channel
  → 临时 code channel：人 + Agent 多人共同制作、过程可见
  → Artefacts（Diff / Canvas / HTML 等）和 review
  → 原频道收到状态/回链；code channel 归档但可搜索
```

这已经反驳了“多人 Agent Chat 的界面一定只能是普通聊天记录”和“主频道必须承受 Agent 过程”两种说法。Slack 的 Code channel 有状态、Artifact 展示、行级评论、预览和独立空间。

但 Slack 自己的产品语言也清楚表明其价值选择：官方博客把私有 AI tab 描绘为孤立，把 Slack Code 定义为让开发“在公开协作中完成”，使团队可以实时看过程、途中纠偏；它认为共同制作的对话是工作本身。[^slack-code-blog] 这与 Threadline 可选择的“**制作对个人私有、交付对频道公开**”并不是同一种默认。

## 不是功能罗列：两个产品的边界差异

下表中 Slack 一栏仅指上述官方明确说明的行为；Threadline 一栏是待实现的产品要求，并非现成事实。

| 维度 | Slack Code / Agent 会话 | Threadline 应确立的模型 |
| --- | --- | --- |
| 频道的主关系 | Slack 是人际 IM，且 Agent 可在同一频道或专用 code channel 与人共同聊天、制作。[^agents][^slack-code-help] | 项目频道默认保持**人对人**；不把 Agent 生产对话置为频道的主对象。 |
| 制作空间 | code channel 是临时多人空间；public/private 依 workspace/channel 可见性选择；成员可加入、prompt、看过程。[^slack-code-help] | 每位作者有自己的 Agent 制作空间；Prompt、试错、草稿、工具过程默认只属于作者。需要多人共制时可另开，不应是默认交付路径。 |
| 从主频道到制作 | 频道/DM 内提及 Agent 会拉起 code channel；原消息保持链接/更新。[^slack-code-help] | 人际频道提出 brief/需求后，某人可在自己的制作空间中接单；人际频道不自动成为制作会话的参与者。 |
| 制作过程的公开性 | Slack 支持 private Agent interaction 和 private code channel；但 code channel 的官方叙事是让该频道成员共同 prompt、共同看过程。[^agents][^slack-code-help] | 默认仅作者可见；“分享过程/邀请共同制作”是附加、可选操作。公开默认仅在发布阶段发生。 |
| 交付到人际频道 | 现有资料明确的是：code channel 向发起消息回传更新；Canvas/Prompt 可被分享至频道。[^slack-code-help][^agentforce][^canvas] | 发布应是明确的一等动作：选择目标频道、选择要带出的 Artifact/版本/交付说明，而不是把制作频道或 Agent 对话本身分享过去。 |
| 审阅对象 | Artefacts、Canvas、代码均可在 code channel 内查看与评论；Canvas 有版本和锚定评论。[^slack-code-help][^canvas] | 人际频道审阅的最小单位是稳定的 `Artifact@version`；反馈、接受、退回、后续流转都针对该版本，而不是一段会话或一个临时频道。 |
| 修订去向 | 官方资料描述在 code channel 中继续与 Agent 对话、评论，及 channel 归档；未说明跨空间“退回作者私有制作会话”的显式契约。[^slack-code-help] | 人际频道的“请求修订”形成一个带回原作者制作空间的请求；作者发布同一 Artifact 的新版本。 |
| 团队看到什么 | 可看被加入的公开/私有 code channel、其 Agent conversation、Artifact 和 review；Slack 同时提供个人 private interaction。[^agents][^slack-code-help] | 团队看到**交付透明度**：Artifact、版本、发布说明、反馈和决定；不会因为收到成品就默认获得作者原始 Prompt/失败尝试/私有工具记录。 |

这个差异不能写成“Slack 没有 Artifact”或“Slack 没有隐私”。更严谨的说法是：

> Slack 将 Agent 工作的协作单位做成了 **session/channel**，其旗舰路径是让相关人共同在其中制作并审阅；Threadline 要把人际协作的接收单位做成 **published Artifact**，其默认路径是先由个人完成私有制作，再让团队围绕被发布的版本沟通与决定。

“Slack 只有 session、Threadline 才有 Artifact”也过度了，因为 Slack Code 的 `Artefacts`、Canvas version history 和 line comments 已经具备部分 Artifact 属性。要成立，Threadline 需要把 `Artifact` 变成跨制作空间和人际频道的稳定领域对象，而不是给 preview 起一个新名字。

## Slack 已经否定的 Threadline 叙事

以下说法不应进入 README、销售材料或产品 PRD：

1. **“主流 IM 没有富产物预览和审阅。”** Slack Code 官方列有 live HTML、code diff、Canvas，代码可评论；Canvas 有内联预览、锚定评论、resolved 状态和历史版本。[^slack-code-help][^canvas]
2. **“Agent 只能作为 Bot 在主群发消息。”** Slack 提供 Agent DM、频道 `@`、split view、专门的 Code channels 和 Agent session 状态。[^agents]
3. **“团队使用 AI 只能在某人的个人 Chat 或一个群里二选一。”** Slack 同时支持 private interaction、私有 code channel、公开 code channel、私有 Canvas/精确共享。[^agents][^slack-code-help][^canvas-permissions]
4. **“把 Agent 过程移出主频道”就是我们的独家体验。** Slack Code 的明确目的之一就是让复杂 Agent 工作从普通线程移出、同时让发起频道保持聚焦。[^slack-code-help][^slack-code]
5. **“Slack 是人对人、Agent 产品都是个人聊天，所以不存在直接竞争。”** Slack 自己已经是人际 IM，并将 Agent、个人 Agent 会话、多人 code channel、Artifact 预览和工作流纳入同一客户端。[^channels][^agents][^slack-code-help]

## 仍可能成立、但必须验证的差异

### 1. “private making, public delivery”是否是比“build in the open”更好的默认？

这是 Threadline 唯一值得严肃验证的体验假设，而非一个被资料证明的市场事实。

它适用于这样的情况：作者需要与个人 Agent 多轮试错、换模型/工具、丢弃草稿，却不愿把这些操作变成同事要阅读或可审视的工作记录；同时团队又不能只收到模糊文字，需要对最终交付作反馈与决定。

Slack Code 的官方材料恰好提供反方向观点：工作越复杂，越需要团队中途加入、共享 context 和实时纠偏。[^slack-code-blog] Threadline 不能假装这不成立。它必须允许作者在需要时主动把制作转为共同制作；差异只能是**默认私人、可选择公开共制**，而不是禁止透明协作。

### 2. `Artifact@version`能否成为跨空间的工作单元？

若只做到“从个人制作页发一张预览到频道”，这很容易被 Slack Canvas 分享、Slack Code 的 origin update、文件链接或任何现有 IM 的发布功能替代。核心检验应是：

```text
个人制作空间：作者 + Agent
  └─ 发布 Artifact v1（选择交付说明、目标人际频道）
         ↓
人际频道：人对人讨论 / 针对 v1 评论 / 接受、退回、流转
         ↓ 请求修订（保留 Artifact ID、目标与反馈）
个人制作空间：作者制作 v2
  └─ 发布 v2 到同一个交付链
```

要成为不可被“链接 + 群消息”取代的对象，至少需要：稳定 ID、版本关系、发布者、目标频道、可呈现内容、对具体版本/区域的反馈、接受/退回/转交状态，以及把修订请求返回原制作链的关系。这里不是强调技术权限控制；频道成员关系已定义了公开受众。关键是**人控制何时把制作转换为交付**。

Slack 官方资料没有明确描述这一整条“选择性发布—频道决定—回到原作者私有制作空间—发布同一 Artifact 新版”的契约。这个缺口是一个合理的设计方向，不是竞品缺功能的定论。

### 3. Threadline 不应把“团队可追踪”误写成“制作过程透明”

对团队有价值的可追踪性应是：谁交付了什么、哪一版本正在等什么决定、反馈是否被处理、结果流转到了哪一个频道。它不是公司看到每条 prompt、每次失败或每个私人工具调用。

这一点与 Slack 的隐私功能并不冲突；Slackbot 已公开主张个人 interaction private，Canvas 也有精细共享。[^slackbot][^canvas-permissions] Threadline 的机会仅在于把这个隐私边界和人际交付边界做成日常默认，而不是让用户手动组合个人 Agent chat、canvas/link、频道消息和临时制作频道。

## 较一致的产品定位与商业命题

### 产品定位：Agent 原生 IM，但不能停在抽象词

“Agent 原生 IM”容易被误解为：把一堆 Agent 加进群聊，让大家与 Agent 聊天。这正是 Slack 已经在做、也正是用户不希望 Threadline 变成的产品。Threadline 仍然是一个完整的人对人 IM，而不是 Slack 的插件或个人 Agent 客户端外的一层交付服务；团队消息、成员关系、频道和决定是产品本体。

更准确的内部定位应是：

> **以人际频道为主界面的 IM；每个人在其中拥有私人的 AI 制作空间，完成后把可复用、可审阅的 Artifact 显式发布给团队。**

它有三个不可缺少的产品面：

```text
人际频道            私人制作空间              发布的 Artifact
人 ↔ 人沟通/决定  |  作者 ↔ AI 探索/制作   |  版本、反馈、决定、流转
```

少了人际频道，它退化成 Codex/Claude 式个人 Agent 客户端；少了私人制作，它退化成 Slack Code 式多人 Agent Chat；少了 Artifact 发布链，它退化成“私下做完、再手动发链接/附件”的拼装工作法。

### 商业价值：让个人 AI 客户端进入组织的交付链，而非监控个人 AI

个人 Agent 客户端优化的是个人生产力：一个人和 AI 对话、改稿、运行工具、做出结果。人际 IM 优化的是团队沟通：讨论、分工、决定。两者之间今天常靠人工完成“导出/截图/贴链接/整理结论”。

Threadline 要卖的不是“我们有更强的 Agent”，而是这条断层的组织价值：

> **私人 AI 的产出可以成为团队的正式交付，而不要求团队进入作者的 Agent Chat，也不要求作者公开自己的制作过程。**

它降低的可能是三种协作摩擦：

- **交付摩擦：** 接收者从“一个链接/一段摘要”变为可直接审阅的 Artifact 版本。
- **反馈摩擦：** 反馈不散落为“再改一下”的消息，而回到同一个 Artifact 的修订链。
- **交接摩擦：** 团队保留的是被接受/退回/流转的交付历史，而不是依赖某个人解释他在私有客户端里做过什么。

这里的“可追踪”是**公开交付链可追踪**，不是私人操作被追踪。这个界限若被打破，制作者会绕开 Threadline，继续使用个人 AI 客户端；若交付对象不足够好，审阅者会继续要求截图、附件或完整聊天记录。

### 与 Slack 的商业关系

Slack Code 把“团队和 Agent 共同制作”做得已经很完整，因此 Threadline 不应对 Slack 客户声称“我们才让团队能用 Agent 协作”。这不是可信的 IM 替换理由。反过来，Threadline 要求一个团队迁入频道时，确实是在竞争其日常 IM 位置；它必须同时把人际沟通做得足够可靠、自然，不能假定自己只是外挂。

相反，Threadline 的商业假设是：存在一类团队，其主协作仍是人际 IM，成员大量使用个人 Agent 客户端，但不希望把日常制作变成多人 Agent session；他们愿意为**个人制作 → 正式交付 → 人际频道决策 → 私下修订**的一条默认路径迁移工作习惯。

这在现有 Slack 用户中也可能成立，也可能被 Canvas + private code channel + 手动分享充分满足；功能资料无法判断。对于不使用 Slack Code 的组织，更不能仅凭“竞品不在本地市场”就推导预算。它需要通过真实交付频次、对过程私密性的强度、以及是否愿意放弃现有拼装法来验证。

## 下一步应验证什么（不是功能清单）

在设计伙伴中，用同一个真实交付任务对比两条路径，而不是问用户“你喜不喜欢这个概念”：

| 要证伪的问题 | 可观察的失败信号 |
| --- | --- |
| 作者是否真需要私下制作？ | 作者愿意直接在多人 Agent code channel 中工作，且认为过程公开没有成本。 |
| 团队是否只需看 Artifact 就能决策？ | 审阅者必须索要完整 Prompt/过程才能判断结果，说明交付包定义不够。 |
| Artifact 发布是否优于 link/message？ | 用户发布后仍回到群聊描述版本、重新传附件、重新开 thread。 |
| “请求修订”是否真的回到同一条链？ | 每轮修改都变成新的临时对话、文件和消息，没有连续版本。 |
| 这是否比 Slack Code/现有 IM 的组合好？ | 团队认为现有 Slack Canvas/private code channel 或现有个人 AI + 附件流程足够，且不愿改变默认行为。 |
| 是否存在经济买家？ | 只要演示就觉得新鲜，但任务结束后没有团队愿意持续把真实交付放进该闭环。 |

在上述至少一个完整闭环重复发生前，不应把“替换飞书/钉钉/Slack”或“成为企业 AI 的核心 IM”当成已验证的近期商业结论。产品定位仍是完整 IM；较小、可检验的首步是：让一个真实团队在 Threadline 的人际频道中，使某类高频 AI 交付不再断在个人 Agent 客户端与人际 IM 的缝里。

## 证据边界

- 本文只核对 Slack 官方公开资料，功能区域、套餐、受支持 Agent、发布时间会变化；阅读日期为 2026-08-27。
- Slack Code 是 2026 年 8 月的新增功能；资料显示它已经发布/逐步可用，但不能据此推断每个客户都已部署、每个地区/套餐均可获得，或实际采用效果。
- “官方帮助中心未描述某项跨空间发布能力”不能推出 Slack 绝无此能力；本文统一表述为“本次资料未见明确说明”。
- 功能、产品定位和厂商客户引述都不是 Threadline 的市场规模、客户迁移意愿或付费意愿证据。那些需要访谈、真实任务对照和预算承诺验证。

## 来源

[^channels]: [Slack：Channels](https://slack.com/features/channels)，2026-08-27 访问；[Keep work organized with channels](https://slack.com/help/articles/1500000019361-Keep-work-organized-with-channels)，2026-08-27 访问。
[^tabs]: [Slack：Add and manage tabs in channels and direct messages](https://slack.com/help/articles/32562841868307-Add-and-manage-tabs-in-channels-and-direct-messages)，2026-08-27 访问。
[^canvas]: [Slack：Use a canvas in Slack](https://slack.com/help/articles/203950418-Use-a-canvas-in-Slack)，2026-08-27 访问。
[^canvas-settings]: [Slack：Manage canvas settings in Slack](https://slack.com/help/articles/33536064287891-Manage-canvas-settings-in-Slack)，2026-08-27 访问。
[^canvas-permissions]: [Slack：Manage access permissions for canvases and lists](https://slack.com/help/articles/15678967614611-Manage-access-permissions-for-canvases-and-lists)，2026-08-27 访问。
[^agents]: [Slack：Work with AI agents in Slack](https://slack.com/help/articles/33076000248851-Work-with-AI-agents-in-Slack)，2026-08-27 访问。
[^agentforce]: [Slack：Use Agentforce in Slack](https://slack.com/help/articles/36218786859667-Use-Agentforce-in-Slack)，2026-08-27 访问。
[^slackbot]: [Slack：Slackbot, Personal AI Agent for Work](https://slack.com/features/slackbot)，2026-08-27 访问。
[^agentic-os]: [Slack：What Is an Agentic OS?](https://slack.com/blog/productivity/what-is-an-agentic-os)，2026-08-27 访问。
[^slack-code-help]: [Slack：Build with AI as a team using Slack Code](https://slack.com/intl/en-gb/help/articles/54310833022355-Build-with-AI-as-a-team-using-Slack-Code)，2026-08-27 访问。
[^slack-code]: [Slack：Introducing Slack Code](https://slack.com/features/code-channels)，2026-08-27 访问。
[^slack-code-blog]: [Slack：Slack Code: Where Your Team and Agents Build Together](https://slack.com/blog/news/slack-code-channels-for-agents)，2026-08-27 访问。
