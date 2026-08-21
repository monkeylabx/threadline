# Agent Skills

Threadline 的工程工作流 skill 来自 [`mattpocock/skills`](https://github.com/mattpocock/skills)（MIT）。

skill 文件不进本仓库，每个人在本地装一次即可。下面每个 skill 名都链到它的原文，想看细节点进去就行。

## 安装

Claude Code：

```
/plugin install mattpocock-skills
```

官方 marketplace 里就有，不用先添加源，上游更新会自动同步。

Codex 等其他 agent：

```bash
npx skills@latest add mattpocock/skills --global
```

两条路选一条。都装的话每个 skill 会出现两份。

本仓库已经跑过 `/setup-matt-pocock-skills`（见 `216287d`），装完直接能用，不用再跑一次。

## 推荐的 skill

### 写代码之前

**[`/grill-with-docs`](https://github.com/mattpocock/skills/blob/main/skills/engineering/grill-with-docs/SKILL.md)** — 最该先用的一个。它会围绕你要做的改动反复追问，直到你和 agent 对同一件事的理解一致，同时把过程中定下来的术语写进 `CONTEXT.md`、把架构决策写进 `docs/adr/`。
用在：任何设计决策，以及任何你自己都还没完全想清楚的改动。

**[`/to-tickets`](https://github.com/mattpocock/skills/blob/main/skills/engineering/to-tickets/SKILL.md)** — 把一个 `Pxx-xx` work package 拆成 Agent Task，产出带阻塞关系的 GitHub Issue。
用在：`delivery-plan.md` 里的条目要开工之前。`agent-workstreams.md` 要求 work package 必须先拆再写代码，这就是拆的工具。

**[`/wayfinder`](https://github.com/mattpocock/skills/blob/main/skills/engineering/wayfinder/SKILL.md)** — 比 work package 更大、路线还看不清的东西。在 Issue 上建一张决策地图，一次解一个决策点，直到路清楚为止。
用在：Milestone 级别、一个会话装不下的模糊需求。

**[`/to-spec`](https://github.com/mattpocock/skills/blob/main/skills/engineering/to-spec/SKILL.md)** — 已经聊明白了，直接把对话落成 spec Issue，不再反问。

### 写代码的时候

这几个 agent 会在合适的时机自己调用，不用手打：

**[`tdd`](https://github.com/mattpocock/skills/blob/main/skills/engineering/tdd/SKILL.md)** — red-green-refactor 循环，以及什么样的测试值得留下、哪些是反模式。

**[`diagnosing-bugs`](https://github.com/mattpocock/skills/blob/main/skills/engineering/diagnosing-bugs/SKILL.md)** — 难 bug 和性能回归的排查流程，分阶段推进，不许跳步。

**[`domain-modeling`](https://github.com/mattpocock/skills/blob/main/skills/engineering/domain-modeling/SKILL.md)** — 往 `CONTEXT.md` 加术语、往 `docs/adr/` 记决策时用。

**[`codebase-design`](https://github.com/mattpocock/skills/blob/main/skills/engineering/codebase-design/SKILL.md)** — 深模块（deep module）的设计词汇。定服务边界、设计能力契约的时候有用。

**[`resolving-merge-conflicts`](https://github.com/mattpocock/skills/blob/main/skills/engineering/resolving-merge-conflicts/SKILL.md)** — 先从 commit、PR、Issue 里找出两边改动各自的原始意图，再解冲突，而不是简单二选一。多 workstream 并行时经常用到。

### 周边

**[`/triage`](https://github.com/mattpocock/skills/blob/main/skills/engineering/triage/SKILL.md)** — 用 `docs/agents/triage-labels.md` 里那五个标签给 Issue 分诊，走一遍分类、核实、补信息的流程。

**[`/improve-codebase-architecture`](https://github.com/mattpocock/skills/blob/main/skills/engineering/improve-codebase-architecture/SKILL.md)** — 扫一遍代码库，产出可以「做深」的重构候选清单（HTML 报告），再挑一个深入讨论。是定期体检，不是抢救。

**[`/ask-matt`](https://github.com/mattpocock/skills/blob/main/skills/engineering/ask-matt/SKILL.md)** — 不知道该用哪个的时候问它，它会告诉你当前情况适合哪个 skill 或哪条流程。

## 怎么用

以 `/` 开头的是你手动输入的命令；不带 `/` 的（`tdd`、`diagnosing-bugs` 等）由 agent 在匹配到场景时自动调用。

一个典型回合：

```
/grill-with-docs     # 对齐需求，顺带写好 CONTEXT.md 和 ADR
/to-tickets          # 拆成 Agent Task Issue
                     # 认领 Issue，建 branch 和 worktree，正常开发
                     # tdd / diagnosing-bugs 会自动介入
```

上面是挑出来的常用项。完整的 25 个 skill 见 [上游 README](https://github.com/mattpocock/skills#reference)。
