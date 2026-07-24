# Threadline Agent 异步工作流

状态：Frozen Scope 1.1 实施基线 1.0

计划来源：[Private Enterprise v1.0 交付计划](./delivery-plan.md)

## 1. 目标

让多个 Coding Agent 可以从同一个稳定基线并行工作，同时避免以下问题：

- 同时修改同一文件或同一数据库迁移。
- Client 在 Server Contract 未确定前猜测字段。
- Agent 依赖另一个 Worktree 中未提交的文件。
- 任务看似完成，但没有独立测试、迁移、回滚或交接信息。
- 多个大 PR 在里程碑末尾一次性集成。

异步工作不是“所有任务同时开始”。允许并行的是已经满足输入 Contract 的 Task；依赖未满足时只能
产出 Draft、Fake 或测试设计，不能把猜测接口合入主分支。

## 2. 工作单元

### 2.1 三层结构

```text
Milestone              M0-M7，集成与产品验收门
  -> Work Package      delivery-plan.md 中的 Pxx-xx，约 1-7 人周
      -> Agent Task    GitHub Issue，0.5-2 Agent 日，一个 Branch/Worktree/PR
```

一个 Agent Task 必须满足：

- 只有一个主要目标和一个 Primary Workstream。
- 正常情况下修改不超过一个所有权目录。
- 可以在没有其他 Agent 本地文件的情况下构建和测试。
- 验收命令在任务开始前定义。
- 失败或中断后，另一 Agent 能根据 Issue、Commit 和 Handoff 接管。

### 2.2 状态机

```text
draft -> ready -> claimed -> working -> review -> integrated -> done
                    |          |
                    +-> blocked+
```

GitHub Issue 是状态事实源。不要让多个 Agent 更新仓库内同一个 Task Board 文件。

建议标签：

- `status/draft`、`status/ready`、`status/claimed`、`status/blocked`、`status/review`。
- `stream/contracts`、`stream/core`、`stream/client-core` 等 Workstream 标签。
- `gate/M0` 至 `gate/M7`。
- `contract-change`、`migration`、`security-review`、`platform-test`。

Claim 必须先完成 Issue Assignee 和 `status/claimed` 更新，再创建 Worktree。一个 Agent 同时只 Claim 一个
主任务；等待 CI 或 Review 时可以 Claim 一个不修改相同路径的短任务。

## 3. Workstream 与路径所有权

| Workstream | 主要路径 | 可以并行产出 | 禁止直接修改 |
| --- | --- | --- | --- |
| `architecture` | `docs/architecture/`、ADR | Threat Model、领域设计、评审结论 | 产品代码、Proto |
| `contracts` | `proto/`、`packages/generated-*` | Proto、Golden Frame、Fake SDK | 业务实现、手改生成代码 |
| `design-system` | `packages/ui/`、`packages/design-tokens/` | Token、基础组件、状态规范 | Desktop/Mobile 页面业务 |
| `core` | `services/core/`、`db/migrations/`、`db/queries/core/` | 事务模块、权限、Message Command | Realtime、Client DB |
| `realtime-worker` | `services/realtime/`、`services/worker/` | WSS、Presence、Outbox Relay、DLQ | Core Domain Row |
| `crypto-recovery` | `crates/client-crypto/`、`services/recovery-control/`、`test/crypto/` | Group E2EE Adapter、Golden Vector、Recovery Control | 自创算法、IM Core 明文路径、模型路由 |
| `client-core` | `crates/client-core/`、`crates/client-ffi/`、`crates/locald/` | SQLite、Outbox、Sync、Search、稳定 FFI Facade | UI 页面、密码协议实现、agentd |
| `desktop` | `apps/desktop/` | Tauri/React 页面、Window、快捷键 | iOS/Android 页面、Client Core |
| `mobile-ios` | `apps/ios/` | SwiftUI/UIKit、APNs、Keychain、Share、后台恢复 | Android、Desktop、Rust Core |
| `mobile-android` | `apps/android/` | Compose、FCM、Keystore、Share、后台恢复 | iOS、Desktop、Rust Core |
| `runtime` | `services/agentd/`、`services/runtime-gateway/`、`crates/connectord/` | Run、Lease、Context、Workspace | IM SQLite、Core 表直写 |
| `model-control` | `services/model-control/` | Discovery、Evaluation、Route Policy、Grant | Workflow 硬编码模型名、代理或记录 Prompt |
| `admin-web` | `apps/admin-web/` | 管理页面、Audit Viewer | Core Policy 实现 |
| `platform` | `deploy/`、镜像和 CI Release | Helm、PKI、Observability、Offline Bundle | 业务 Contract |
| `quality` | `test/` | Contract/E2E/Load/Chaos、Fixture | 为通过测试而修改业务语义 |
| `integration` | 根 Workspace 文件、Lockfile、Release Manifest | Merge Queue、版本、生成物更新 | 独立业务功能 |

路径尚未创建时仍按本表预留。一个任务确实需要跨两个 Workstream 时，拆成 Contract Task 与两个实现
Task，由 Integration Owner 串联合并。

## 4. 共享表面规则

以下文件属于高冲突表面，只允许指定 Owner 修改：

| 共享表面 | Owner | 规则 |
| --- | --- | --- |
| `proto/` | Contracts Agent | 先合 Proto 与 Fixture，再并行实现 Client/Server |
| `db/migrations/` | Core Migration Owner | 每个 Issue 预留迁移 ID；禁止两个 Agent 改同一迁移 |
| Generated SDK | Contracts/Integration | 只由固定命令生成，禁止手改 |
| `pnpm-lock.yaml` | Integration | Feature Agent 只改 Package Manifest，交接时说明依赖 |
| Root Cargo/Go Workspace | Integration | 子工程先在自身 Manifest 声明，统一接入由 Integration 完成 |
| SwiftPM/Gradle 根配置 | Integration | iOS/Android 任务只改各自模块，根版本和生成任务串行接入 |
| Rust FFI Public Facade | Client-core Agent | Swift/Kotlin 只消费版本化接口；破坏性变更先走 Contract Task |
| Crypto Protocol Adapter / Golden Vector | Crypto-recovery Agent | 其他 Agent 不直接调用底层密码库，不修改测试向量 |
| Design Token | Design-system Agent | TS/Swift/Kotlin 页面只消费 Token，不创建近似颜色和间距 |
| Release/Helm Values Schema | Platform Agent | 服务通过独立配置 Schema 提交需求 |

## 5. Contract-first 并行方式

跨工程功能按以下顺序拆分：

```text
Contract Task
  -> Proto / Interface / Error / Fixture / Fake
       -> Server Implementation Task
       -> Crypto/Client Core Implementation Task
       -> Desktop UI Task
       -> iOS UI Task
       -> Android UI Task
       -> Quality Task
  -> Integration Task
```

Contract Task 合入后，各实现 Agent 从同一个 Commit 创建 Worktree。Server 未完成时 Client 使用 Fake；
Client 未完成时 UI 使用 Repository Interface 和 Fixture。Fake 必须通过同一 Contract Test，不能成为第二套
随意定义的 API。

## 6. Branch、Worktree 与 Commit

- Branch：`agent/<task-id>-<short-slug>`，例如 `agent/P04-03-gap-repair`。
- Worktree：仓库外同级目录 `.worktrees/<task-id>` 或编排器提供的隔离 Worktree。
- Base Commit 写入 Issue；工作超过一天时每日同步 `main`，但不能重写已交付给其他 Agent 的 Commit。
- Commit 必须原子化并包含 Task ID，例如 `P04-03 implement cursor gap detection`。
- Agent 不直接 Push/Commit 到 `main`，不使用共享工作目录，不依赖未跟踪文件。
- 一个 PR 默认只对应一个 Agent Task；机械生成文件可以跟随所属 Contract PR。

## 7. Handoff 与接管

每个 Agent 完成或阻塞时都必须使用
[`templates/agent-handoff.md`](./templates/agent-handoff.md) 提交交接内容，最少包含：

- Issue、Branch、Base Commit、Head Commit。
- 已完成和未完成范围。
- 修改路径和所有 Contract/Migration 变化。
- 实际执行的测试命令与结果。
- 已知风险、失败尝试、敏感安全决策。
- 解锁的后继 Issue，或当前阻塞条件和 Owner。

Agent 中断时，接管者从最后一个可构建 Commit 开始，不继续使用原 Agent 的未提交工作目录。

## 8. Merge Queue 与集成门

Integration Owner 串行处理共享表面，普通 Feature PR 可以并行 Review。合入顺序：

1. Contract / Migration Reservation。
2. Server 和 Client Core。
3. Desktop、iOS、Android UI。
4. Contract/E2E/Load Test。
5. Generated SDK、Lockfile、Release Manifest。

每个 Milestone 设置 Integration Freeze：冻结前 2 个工作日不再接受新 Contract，只修复阻断问题。

| Gate | 必须通过 |
| --- | --- |
| G0 / M0 | Scope、Client/Crypto/Storage ADR、Threat Model、Rust FFI/E2EE Interop Gate、Proto Skeleton |
| G1 / M1 | Reproducible Build、CI、Contract Compatibility、Dev Stack |
| G2 / M2 | Desktop E2EE Message -> Local Agent -> Approval -> Artifact Vertical Slice |
| G3 / M3 | Collaboration Feature Matrix、iOS/Android E2EE Message Alpha、File/Search/Push |
| G4 / M4 | Runtime/Capability/Approval/Artifact/Model Route、Mobile Task Control |
| G5 / M5 | Recovery、Helm、Backup/Restore、Upgrade/Rollback、Audit |
| G6 / M6 | SLO、Chaos、Five-platform E2E、Feature Complete |
| G7 / M7 | Independent Crypto Review、Pentest、Enterprise Pilot Candidate |

## 9. 首阶段并行启动矩阵

下列任务可以从同一 Baseline 并行启动，但只有 Gate 条件满足后才能标记 Done：

| Agent Lane | 初始任务 | 产物 | Merge 依赖 |
| --- | --- | --- | --- |
| Product/Architecture | P00-01、P00-02 Draft | Scope、Client/Server/Storage ADR | 产品评审 |
| Crypto/Security | P00-03、P00-04 Draft | Crypto ADR、Threat Model、Recovery Boundary | Scope Freeze |
| Native Bridge | P00-07 | Swift/Kotlin FFI 真机证据 | Client ADR、Monorepo Skeleton |
| Crypto Interop | P00-08 | 三端 Golden Vector、Epoch/History/Recovery 证据 | Crypto ADR、Monorepo Skeleton |
| Contracts | P02-01、P02-04 Draft | Proto Skeleton、Ciphertext/Key Envelope 方案 | Domain/Crypto ADR |
| Platform | P00-05、P12-01 Skeleton | Workspace、Local Dev Stack | Toolchain ADR |
| Quality | P13-01 Draft | Test Matrix、Fixture 方案 | Acceptance Scenario |
| Design System | P01-01、P01-02 Draft | State Matrix、Token | Scope Freeze |

初始任务合并到 G0 后，才批量创建 M1 的 Ready Issue。不要一次性把全部工作包标记 Ready；远期
任务会因 Contract 和 Pilot 反馈变化，过早 Claim 只会制造返工。

## 10. Agent Task 拆分检查

创建 Issue 前逐项确认：

- [ ] 目标可以在两天内完成。
- [ ] 只有一个 Primary Workstream。
- [ ] Owned Paths 与其他 Ready Task 不重叠。
- [ ] 所有依赖已经 Integrated，或任务明确只产出 Draft/Fake。
- [ ] Contract、输入 Fixture 和错误语义已确定。
- [ ] 验收命令可以由另一 Agent 独立执行。
- [ ] 包含失败、权限、离线或升级路径中的相关项。
- [ ] 不需要读取另一个 Worktree 的未提交文件。
- [ ] Handoff 后下一个 Agent 能仅凭 Issue 和 Commit 接手。
