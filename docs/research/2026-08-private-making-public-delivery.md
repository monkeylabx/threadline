# 从私人 AI 制作到公开 Artifact 交付：商业命题调研

状态：产品/商业假设调研（不构成市场规模、定价或客户付费意愿证据）  
调研快照：2026-08-27  
问题：把**个人私下与 AI 制作**，与**向人际 IM 频道显式发布一个结构化 Artifact**分开，是否与私有 AI Chat、多人 Agent Chat、IM 机器人有实质区别？若有，应该卖给谁，又如何证伪？

## 结论先行

这不是“让 Agent 在群里更好地说话”，也不是“把每个人的 AI 过程变得透明”。正确的产品边界是：

> **私人制作，公共交付。**
>
> 人际频道默认是团队公开的；只有作者主动发布的 Artifact、其版本、交付说明、反馈和决定进入频道。Prompt、试错、草稿、工具调用和未发布对话不进入频道。

这个模式与现有产品有**有意义的体验和对象关系差异**，但不是未被实现过的独家能力。ChatGPT Business 已有私人历史和选择性分享；Claude 已有私有 Project 和 Artifact 内部分享；Microsoft Copilot Pages 已明确允许别人协作页面而不访问作者的 Copilot Chat，并可把页面以实时组件贴到 Teams。后者尤其接近本命题。

因此，不能把商业命题写成“别家没有私密 AI”或“别家没有 Artifact”。可成立、也值得验证的命题是：

> **现有产品把私密制作、团队沟通和交付审阅拆成 Chat、链接、页面、附件或 Bot 消息。Threadline 将它们收敛为人际频道里的一个“交付对象”：作者可私下制作，显式发布一个可审阅、可退回修订、可接受并继续流转的 Artifact。**

它的价值也不是“过程可见”。恰恰相反：团队只获得**足够接手和决策的交付透明度**，不获得作者的完整制作过程。

## 已核实的能力边界

下表只陈述供应商官方资料明确说明的能力；“未见”不等于该供应商绝无该能力。

| 类别/产品 | 官方资料已经证明什么 | 对 Threadline 命题的含义 |
| --- | --- | --- |
| 私人 AI Chat：ChatGPT Business | 每个成员有自己的 Chat/Codex history，其他成员默认看不到；用户可共享特定 Chat 链接。链接接收者继续对话时，会得到自己的新私有会话。[^openai-business] | “私人操作不暴露”不是差异化本身。链接只是交付后的对话快照/入口，接收者的后续工作又分叉为新的私有 Chat，不是一个团队交付对象。 |
| 共享 AI 项目：ChatGPT shared Projects | Shared Project 可以让成员查看和互动其中的 chats、files、instructions；成员也能把 Chat 移出项目。[^openai-projects] | 这是“把工作上下文共享给一群人”，中心仍是 Project/Chat 集合。它证明团队共享 AI 上下文可行，但不等于“作者私下制作后只向人际频道交付一个 Artifact”。 |
| 私人 Project 与分享：Claude for Work | 私人 Project 中的 chats、knowledge、artifacts 对他人不可见；Project 即使公开，chat 仍默认私有。分享 chat 时，分享的是此前消息和其中 Artifact 的快照；之后消息仍默认私有。[^claude-projects] | 这强力证明“私下制作、选择性公开”是成熟需求，不是 Threadline 独创。Threadline 若要胜出，必须把公开物从“Chat snapshot”提升为频道可持续审阅的交付物。 |
| Artifact 分享：Claude for Work | Team/Enterprise 可以内部分享某一 Artifact 版本；但官方也说明，分享 Artifact 时，查看者同时获得产生它的会话附件和文件访问权。[^claude-artifacts] | 这揭示了一个可以做得更好的交付边界：Threadline 的发布应是作者挑选的**交付包**，不能因发布成品而默认泄露制作会话的所有附件。 |
| IM 内与 Agent 聊天：Slack | Agent 可一对一 DM，也可加到频道；在频道里通过 `@` 开始聊天，互动可公开或仅自己可见。Slack 还有可公可私的临时代码频道，团队和 Agent 在其中一起工作。[^slack-agents] | 这正是“人和 Agent 聊天”的主模型，且已经可以多人。Threadline 的主频道不能退化成这种 Agent 对话页；制作空间才可容纳这种密集互动。 |
| IM Bot：Microsoft Teams | Teams 的 Bot 直接接入 messaging framework；群聊/频道靠 `@` 交互，Bot 的 Adaptive Card 仍位于 chat bubble 内。[^teams-bots] | 它支持富卡片，不应被描述成“只有纯文字”；但 API/呈现语义仍是**Bot 消息/消息附件**。Artifact 若只是卡片，就仍是这个范式。 |
| 频道 Agent：Microsoft Teams | Agent 被安装进 Team 或 group chat 后，默认只在被 `@` 时接收消息；官方指引强调在群聊/频道中保持简短、少噪声。[^teams-channel-agent] | Teams 也将 Agent 作为频道对话参与者。Threadline 可以借鉴“少噪声”，但不同点必须是把制作从人际频道移出，而非让 Agent 更会回复。 |
| Chat 与共享产物分离：Microsoft Copilot Pages | 别人可协作某个 Copilot Page 而无需访问作者的 Copilot Chat；Page 可作为链接或实时更新的组件贴到 Teams。[^copilot-pages] | **最接近的反例。**“不分享 AI chat、只分享可协作产物”已经成立。Threadline 的差别只能在于：它把发布、版本、审阅决定和“退回作者私下修订”的闭环，做成 IM 的一等交付模型，而不是跨 Copilot/Loop/Teams 的手动分享动作。 |
| 国内 IM 机器人接口：飞书 | 飞书官方的 Aily“飞书消息”能力把向群发送的内容列为文本、富文本、消息卡片；其开放平台将机器人定位为基于会话/用户交互并聚合通知、审批、文档和第三方系统的应用。[^feishu-aily][^feishu-platform] | 这只说明**机器人接口**以消息/卡片为主，不能据此声称飞书整体没有文档或丰富对象。Threadline 的论点应是“不要把 AI 交付降格为一个机器人消息”，而非“国内 IM 只有文字”。 |

## 真正的对象模型差异

Threadline 需要明确有两个不同的空间，而不是两个频道里都在 Chat：

```text
个人制作空间（作者 + AI）
  - 私有 Prompt、试错、草稿、工具/Agent 对话
  - 默认不被目标频道成员看到
  - 作者决定何时结束制作、何时发布
                  │ 显式发布：选择目标频道与交付包
                  ▼
人际项目频道（人 + 人）
  - Artifact 当前版本、交付说明、适用范围、发布人
  - 对 Artifact 的反馈、接受/退回/转交决定
  - 不显示私人制作记录
                  │ “请求修订”带回作者的制作空间
                  ▼
个人制作空间产生新版本，再次显式发布
```

这里的 Artifact 不是文件附件，也不是 Agent 的最后一句回复。至少需要有：稳定身份、版本、来源作者、目标频道、交付说明、可审阅内容、反馈锚点、决定状态和后续版本关系。**制作过程是否分享**应是作者额外选择的材料，而不是 Artifact 的默认组成部分。

频道的成员关系就是公共受众边界；不应另外设计“AI 是否有权看频道”的前台卖点。与外部系统或本地文件的访问治理仍是底层安全问题，但它不能替代上述产品价值。

## 与 Pull Request 的类比（仅是产品语言）

GitHub 的 Pull Request 提供了一个有用但有限的类比：作者在独立 branch/fork 中修改；准备好后创建 PR；审阅者围绕提案评论、要求修改或批准；同一 PR 随新 commit 演进，合并才使它进入主线。GitHub 官方也把 PR 定义为在变化进入项目之前，用于讨论、审阅和保留可审查历史的协作位置。[^github-pr]

Threadline 可以被理解为把这个**交付边界**泛化到非代码 AI 工作：

```text
个人 AI 制作空间  ≈  branch（不是必须公开的聊天记录）
发布的 Artifact   ≈  PR（可被频道审阅的提案/交付件）
频道反馈与决定    ≈  review / request changes / accept
再次发布版本       ≈  push 新 commit 到同一 PR
```

这不是说 Threadline 要复制 Git，也不是付费意愿证据。它的作用是约束产品：**审阅的对象应是交付物及其版本，而非作者的 Prompt 历史；反馈应能回到同一条修订链，而不是散落为群消息。**

## 商业价值应如何表述

### 不是这些说法

- 不是“让 AI 更透明”。这会直接伤害采用：私人试错被监控时，人会继续回到个人 ChatGPT/Claude。
- 不是“在频道里接入机器人”。Slack、Teams、飞书均已支持这个模型。
- 不是“有漂亮预览/卡片”。Teams 已有 Adaptive Cards，Copilot Pages 还可实时嵌入 Teams。
- 不是“公司拥有全部 AI 对话”。这既不是用户承诺，也不是有吸引力的日常体验。

### 可以成立的价值

**把个人 AI 劳动变成团队可接收的交付，而不把个人 AI 操作变成团队公共记录。**

它同时解决两件彼此冲突的事：

1. **作者自由。**作者可以探索、失败、改变 Prompt、换工具；不必在团队面前表演如何使用 AI。
2. **团队可协作。**接收者不用进入某人的 Agent Chat，也不用看一长串制作记录；他们看到一个有版本和明确状态的交付件，可以反馈、接受、退回或转交。

结果不是“过程透明”，而是**交付透明**：团队知道交付了什么、哪一版、谁发布、现在等待什么决定；但不知道作者在私下进行了多少次尝试，除非作者选择披露。

这是一个“AI 时代的团队交付界面”假设。它只有在“个人制作 → 团队审阅/决定 → 修订/流转”高频发生时才有经济价值；对只需单人问答或一次性通知的场景没有价值。

## 谁是用户、谁付钱

| 角色 | 要得到的东西 | 不是其购买理由 |
| --- | --- | --- |
| 制作者（高频使用者） | 一个可私下制作、但可把成品正式交付给团队的空间 | 被要求公开 Prompt、被迫在群里和 Agent 表演 |
| 频道协作者/审阅者 | 不进入 Agent Chat 也能判断、反馈并接手交付物 | 阅读完整 Agent 过程或接收更多机器人消息 |
| 经济买家 | 对一类反复发生的交付负责的人：他/她关心从个人草稿到团队决定的速度、返工和交接 | “公司能看到谁用了多少 AI” |
| IT/安全采购方 | 私有部署、身份、审计和集成可通过采购 | 它们是进入门槛，不是 Threadline 的核心购买动机 |

初始买家不应被宽泛地定义为“所有使用 IM 的企业”。更可检验的定义是：**拥有稳定人际频道、每周反复接收并决定 AI 辅助产物、且制作人不愿把完整 AI 对话公开的一个完整团队。**行业可以以后再选；先验证的是这个工作条件，而不是用“AI 企业 IM”争夺已有 IM 的全量替换。

### 一个可售的最小承诺

不是“替换飞书/钉钉/Slack”，而是：

> 让一个真实团队把它现有的“私下用 AI 做出来，再截图/贴链接/整理几句发群里”的交付环，换成“私下制作 → 发布 Artifact → 频道审阅/决定 → 退回修订 → 新版交付”。

这是侧车式进入，而不是先要求全公司迁移聊天历史。若这个环能形成习惯，再谈成为人际 IM 本身才有依据。

## 必须被证伪的采用条件

以下不是预测指标，而是应在设计伙伴试点中逐项验证/否决的条件。每一条失败都意味着不要把它包装成商业价值。

1. **真实私密性需求。**制作者是否明确表示：愿意使用 AI，但不愿把 Prompt、失败尝试和完整对话放进团队频道？若他们不在意，普通多人 Agent Chat 已经足够。
2. **真实交付需求。**同一团队是否每周会对同类 AI 辅助 Artifact 作出采纳、退回、修改或转交决定？若只有一次性问答/通知，Bot 或共享链接更便宜。
3. **接收者无需过程即可决定。**审阅者能否只看 Artifact、作者选定的交付说明和版本差异就给出有用反馈？如果必须打开完整 AI Chat 才能判断，私人制作边界并不成立。
4. **修订会回到同一条交付链。**一次“请改”能否产生同一 Artifact 的新版本，而不是重新开一个群聊、重新发附件或复制一个新的 Chat 链接？若不能，所谓一等 Artifact 只是展示层。
5. **用户愿意放弃现有拼装法。**让团队在真实工作中比较“ChatGPT/Claude 私人制作 + 链接/附件贴到现有 IM”与 Threadline 流程；如果他们选择前者并说缺少的只是更好预览，命题失败。
6. **至少有一个角色愿意为持续使用承担预算。**免费试用完成后，负责该交付链的人是否愿意为持续的私有制作、发布、审阅和版本闭环付费，而不是只为一次搭建/集成付费？若没有，这只是有价值的开源协作模式，不是已验证的产品业务。

最小成功闭环不是“发出一条 Agent 消息”，而是：一位作者私下完成制作 → 显式发布 Artifact → 另一位人类给出决定/反馈 → 作者不公开私人过程地交付新版本或完成流转。没有反复发生这个闭环，就不应扩张到多 Agent、复杂权限或全量 IM 替换。

## 定价与当前项目现实

官方产品资料能证明现有厂商面向团队/企业提供计划和共享能力；它**不能**证明客户会为 Threadline 的上述流程付费。付费意愿需要设计伙伴的真实预算承诺来验证，不能从竞品功能或“市场需要 AI”推导。

若未来做 SaaS，较贴合该价值的待测假设是“**活跃制作者席位**付费、频道审阅者低门槛参与”，因为隐私制作价值主要发生在制作者，而交付价值要求整个频道能无摩擦查看和决定。这个只是定价实验，不是建议立即定价。

更直接的现实是：当前仓库采用 Apache-2.0；README 只说未来*可能*提供私有化、集成、安全评审、升级和商业支持，且明确没有 paid plan 或 SLA。[^threadline-readme] 当前 v1 交付计划也排除了 SaaS 多租户运营、计费、套餐和公网控制台。[^threadline-delivery-plan]

所以现在应区分两件事：

- **产品商业价值**：上述交付闭环是否让完整团队持续换用、并愿意续费。
- **当前变现路径**：在尚未有产品化计费前，最多是有偿设计伙伴/私有化试点、集成或支持服务；这验证的是部署服务需求，不能冒充已验证的产品需求。

## 对 Threadline 的产品约束

1. 主频道永远先是**人对人**的 IM，不把 Agent Chat 放进默认时间线。
2. “发布”必须是清楚、可撤回/可替换的作者动作；发布物默认最小化，只含 Artifact 和作者选择的交付上下文。
3. 频道中的反馈必须贴在 Artifact/版本/区域上，并能生成给作者的修订请求；不把私人 Agent 会话复制进频道。
4. 制作空间可以容纳单 Agent 或多 Agent，但这只是制作方式；前台卖的不是多 Agent 协作。
5. 若 Artifact 发布后仍只能显示为一张机器人消息卡片或一个外链，产品就没有跨越现有 IM Bot/链接拼装法。

## 来源与证据边界

[^openai-business]: [OpenAI：Managing data, sharing, and privacy in ChatGPT Business](https://help.openai.com/en/articles/8798634)，2026-08-27 访问。官方说明私人 history 默认不对成员公开、共享链接和接收者后续私有会话的行为。
[^openai-projects]: [OpenAI：Projects in ChatGPT](https://help.openai.com/en/articles/10169521)，2026-08-27 访问。官方说明 Shared Projects 的 chats/files/instructions 上下文、成员权限与移出 Chat 的行为。
[^claude-projects]: [Anthropic：Manage project visibility and sharing](https://support.claude.com/en/articles/9519189-manage-project-visibility-and-sharing)，2026-08-27 访问。官方说明 Project 可见性、私有 chats/artifacts 与 chat snapshot 分享。
[^claude-artifacts]: [Anthropic：Publish and share artifacts](https://support.claude.com/en/articles/9547008-publish-and-share-artifacts)，2026-08-27 访问。官方说明 Team/Enterprise 内部 Artifact 分享、版本与会话附件访问边界。
[^slack-agents]: [Slack：Work with AI agents in Slack](https://slack.com/help/articles/33076000248851-Work-with-AI-agents-in-Slack)，2026-08-27 访问。官方说明 Agent DM、频道 `@` Chat、public/private interaction 与 Code channels。
[^teams-bots]: [Microsoft Learn：Designing your bot](https://learn.microsoft.com/en-us/microsoftteams/platform/bots/design/bots)，2026-08-27 访问。官方说明 Bot 位于 Teams messaging framework 和 Adaptive Cards 位于 chat bubble。
[^teams-channel-agent]: [Microsoft Learn：Channel and group chat conversations for agents](https://learn.microsoft.com/en-us/microsoftteams/platform/bots/how-to/conversations/channel-and-group-conversations)，2026-08-27 访问。官方说明 Agent 在频道/群聊的 `@` 触发和少噪声设计原则。
[^copilot-pages]: [Microsoft Support：Share a Microsoft Copilot Page](https://support.microsoft.com/en-us/microsoft-365-copilot/share-a-microsoft-365-copilot-page)，2026-08-27 访问。官方说明可分享 Page 而不让协作者访问 Copilot chat，以及实时组件进入 Teams 等应用。
[^feishu-aily]: [飞书：飞书智能伙伴 Aily 之飞书消息](https://www.feishu.cn/content/mtb6n3ah)，2026-08-27 访问。官方说明向用户/群聊发送文本、富文本、消息卡片。
[^feishu-platform]: [飞书：飞书开放平台介绍](https://www.feishu.cn/hc/zh-CN/articles/950476906644-%E9%A3%9E%E4%B9%A6%E5%BC%80%E6%94%BE%E5%B9%B3%E5%8F%B0%E4%BB%8B%E7%BB%8D?slug=true)，2026-08-27 访问。官方描述机器人基于会话/用户交互并聚合企业应用能力。
[^github-pr]: [GitHub Docs：About pull requests](https://docs.github.com/en/pull-requests/get-started/about-pull-requests)，2026-08-27 访问；[Writing code for a project](https://docs.github.com/en/pull-requests/concepts/writing-code-for-a-project)，2026-08-27 访问。前者说明讨论/审阅/历史，后者说明隔离工作空间、准备好后提案与审阅。
[^threadline-readme]: [Threadline README：Commercial use and support](../../README.md#commercial-use-and-support)，仓库当前版本，2026-08-27 访问。
[^threadline-delivery-plan]: [Threadline 交付计划：v1 不包含](../delivery-plan.md#11-本版本不包含)，仓库当前版本，2026-08-27 访问。
