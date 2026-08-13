# Threadline Private Enterprise v1.0 测试计划

状态：T017 Draft / 依赖收敛前不得作为 Gate 通过证据

Gate：G0 / M0 至 G7 / M7、Pilot Hardening 与 GA

Owner：Quality / SDET；各能力 Owner 对其实现与证据负责

Issue：[#31 T017](https://github.com/monkeylabx/threadline/issues/31)

范围基线：[Frozen Scope 1.1](../acceptance/scope.md)

## 1. 目的、边界与当前依赖

本文把功能、权限、离线、兼容、E2EE/Recovery、Rust FFI、性能、安全和五平台要求映射到测试层级、责任人、证据与 Gate。它定义如何证明候选制品满足要求，不把测试替代产品签字、安全控制、独立评审或企业试点。

当前文本是可并行评审的 Draft，不表示任何 Gate 已通过：

- T002 / Issue #16 的 `docs/acceptance/v1-scenarios.md` 仍是未合入的工作 Draft。本文按其 AC-001 至 AC-012 结构起草；T002 合入后必须逐项校验 ID、步骤、失败路径、证据包和签字 Owner，没有覆盖漂移才能定稿。
- T014 / Issue #28 的 Proto Skeleton 与兼容规则仍是未合入 Draft。本文采用其本地生成、Buf `FILE` breaking、五语言生成、Golden Frame field `50000` canary 和未知字段保留方向；T014 合入后必须以主分支命令和路径重新验证。T014 当前 `proto/golden/v1/manifest.json` 与 T011 的 `test/crypto/e2ee-interop-v1.vector` 也尚未满足本文的 fixture manifest schema：它们是收敛 blocker，必须分别由 Contracts/Integration Owner 与 Crypto Owner 在其 Owned Paths 中补齐 schema 字段和迁移验证。T017 不越界修改、复制或重新解释这些文件。
- T005 已进入主分支，但 [ADR-0003](../adr/0003-group-e2ee-recovery.md) 仍是 `proposed`。T011 已明确判定 OpenMLS `0.8.1` 生产准入失败，见 [互操作 Spike](../spikes/e2ee-interop.md)。在 Security Owner 重新批准候选库、未豁免漏洞与损坏密文 panic 被解决、独立 RFC 9420 互操作及真机证据完成前，Crypto 相关 Gate 保持 `HOLD`。
- T010-A 的 Simulator/Emulator FFI 证据不能替代 T010-B 的 iOS/Android 真机、签名、后台回收、安全存储和内存证据。
- T019 尚未冻结 Device、Epoch、History 和 Recovery Envelope。本文只规定必须测试的外部语义，不猜测字段或错误契约。

定稿收敛条件：上述输入均进入可获取的主分支 Commit；本文只引用已合入契约；所有测试 ID 可追溯到最终验收步骤；执行命令可由独立人员从干净 checkout 重复；Security、Contracts、Client-core、iOS、Android、Recovery 和 Quality Owner 完成评审。依赖未满足时只允许评审测试设计和合成 Fixture 方案，不允许把猜测接口合入实现或将 `NOT RUN` 报为 `PASS`。

## 2. 通过规则与证据强度

候选 Gate 只有同时满足下列条件才可通过：

1. 测试绑定唯一 Repo Commit、制品 Digest、Proto/Bridge/Crypto Profile/Schema、配置和 Policy Digest。
2. 主路径、权限拒绝、离线、超时、重复、撤权、损坏输入、升级和回滚等适用失败路径均执行；跳过项为 `NOT RUN`。
3. 自动化结果、真机记录、故障注入时间线和外部评审可由未参与实现的人复核，不依赖未提交 worktree。
4. 日志、Trace、截图、Crash Dump、诊断包、共享卷、对象存储和备份经过内容与 Secret 扫描，C3/C4 canary 零命中。
5. Critical/High 风险按 [风险台账](../security/risk-register.md) 的 `Due` 收敛：到期 Gate 及更早 Gate 的风险必须满足关闭标准；尚未到 Due Gate 且对应能力未启用的风险可保持 `Open`，但必须有 Owner、缓解 Issue 和到期证据计划。Critical 不得接受，High 不得用业务接受越过其 Due Gate。
6. 每项结果只取 `PASS / FAIL / NOT RUN / approved N/A`。`N/A` 必须写明范围依据、批准人和重新适用条件。
7. Flaky 失败不能用重跑覆盖。证据包保留全部尝试；隔离测试必须关联缺陷、Owner 和到期 Gate，阻断项不得被 quarantine。

证据强度从低到高为：静态/单元结果、Contract/Integration、跨实现 Golden Vector、端到端、负载/混沌、真机/发布制品、独立安全评审、企业 Pilot。低层证据可以快速定位回归，但不能证明更高层边界。

## 3. 测试层级与责任

| 层级 | 证明范围 | 必需输入 | 主要 Owner | 不可替代 |
| --- | --- | --- | --- | --- |
| Unit / state-machine | 单模块值域、状态转换、权限纯函数、错误映射 | 版本化合成值；仅公共/合成不透明输入可用确定性 seed | 各实现 Workstream | 跨进程/跨平台契约、真实存储和网络 |
| Property / fuzz | 重复、乱序、损坏、边界大小、路径和状态空间 | seed policy、最小化 shrink trace、corpus digest、资源上限；敏感输入 seed 不进 Git | Client-core、Crypto、Contracts、Connector | 业务 E2E 和外部 Crypto Review |
| Integration | 真实 DB/队列/对象存储/IPC/FFI adapter 的事务与失败语义 | 隔离私有栈、合成 Tenant、故障注入 | Server、Client-core、Runtime、Platform | 五平台发布或真机生命周期 |
| Contract | Proto、WSS、IPC、FFI、Grant、错误与未知字段的共同语义 | 主分支 schema、N-1 制品、Golden Frame | Contracts/Integration + 消费端 Owner | 密码正确性和业务授权实现 |
| Crypto Golden Vector | RFC transcript、Epoch、History/Recovery 绑定、稳定拒绝与跨实现结果 | 精确依赖、SBOM、公共 KAT/语义向量 | Crypto Owner + 独立 Reviewer | 独立 Crypto Review、KMS/HSM 仪式 |
| E2E | 用户旅程、跨服务授权、离线/撤权和负证据 | 签名候选制品、标准合成拓扑 | Quality + 产品能力 Owner | SLO、长时稳定性和故障容量 |
| Load / soak | p95、容量、队列、热点、内存/磁盘/耗电和长期泄漏 | 代表性规模模型、封闭数据生成器 | SRE/Performance + 能力 Owner | Chaos、功能正确性和 Pilot |
| Chaos / recovery drill | 故障隔离、ACK/Outbox/Cursor、备份恢复和 Runbook | 可回滚环境、明确稳态与故障时间线 | SRE + Data/Recovery/Runtime Owner | 安全边界与 Pentest |
| Real-device / release | 五平台安装、签名、FFI、Secure Storage、生命周期、升级/回滚 | 物理设备/受支持 OS、候选包 | Desktop/iOS/Android/Release Owner | 模拟器、Rust 单测或交叉编译 |
| Independent assurance | 对密码设计、攻击面和企业工作流的独立判断 | 冻结候选、完整证据、整改复测 | 外部 Crypto Reviewer、Pentest、Pilot Owner | 任何自动化测试或内部签字 |

## 4. 平台与系统 Owner

Quality 维护矩阵、执行协议与证据索引，不替代下列交付 Owner：

| 范围 | Accountable Owner | 验证职责 |
| --- | --- | --- |
| macOS / Windows / Linux Desktop | Desktop + Client Platform | Tauri/locald IPC、Sidecar 隔离、安装签名、三 OS 升级、键盘/读屏/200% 缩放 |
| iOS | iOS Owner | Swift FFI、Keychain、真机 arm64、后台/进程回收、APNs、企业签名与最低 OS |
| Android | Android Owner | Kotlin/JNI、Keystore、真机 ABI/API 28 与当前稳定版、进程回收、FCM、企业签名 |
| Shared Core / FFI | Client-core / FFI Owner | 单 Writer、Outbox/Cursor、错误/取消/流/内存、Crash/Resume、N-1 Bridge |
| Server / Sync / Data | Core/Sync + Realtime/Worker + Data Owner | Durable ACK、排序、幂等、Tenant 隔离、PG 事实源、NATS/Redis 降级 |
| Recovery | Recovery Security + Recovery Control | 隔离身份、双人审批、KMS/HSM、Recipient 交付、失败/轮换/灾备 |
| 本地 Runtime / Connector | Runtime + Connector Owner | Capability、Lease/Fencing、Context 有界、Workspace 路径、崩溃/转交/撤权 |
| Model route / egress | Model Control + Runtime Security | 无 Prompt 路由契约、Endpoint/Region/Retention/Fallback、Route Grant 重放 |
| Protocol / generated adapters | Contracts + Integration | Buf、Breaking、五语言生成、Golden Frame、未知字段与 N-1 |
| Admin Web v1 管理子集 | Admin Web + Contracts | 只验证 Organization/Member/Role/Device/Session/Runtime/Key 等 Frozen Scope 管理面；消费 generated TypeScript contract，执行浏览器 integration、授权拒绝和安全错误状态；不扩成 Browser IM Client |
| 私有部署与可观测性 | Platform/SRE + Release | 离线安装、七工作负载、NetworkPolicy、SLO、Chaos、备份、脱敏和制品证明 |
| Product / security acceptance | Product、Architecture、Security | Scope、风险、Gate 决策和 `N/A` 批准；不代跑实现测试 |

## 5. M0–M7 测试矩阵与质量门

| Gate | 候选结果 | 必须执行的测试与失败路径 | 必需证据 | 签字 Owner |
| --- | --- | --- | --- | --- |
| M0 / G0 | Scope、Crypto 与 Native Bridge 候选可进入实现 | AC-001 M0 子集与 AC-004；首台/后续 Device 批准；普通管理员、恶意 Control Plane、伪造/截断/跨 Tenant 批准链、Ghost Device、split-view 和追加式 Device/Key 日志根负向；RFC 9420 KAT、独立实现 transcript、Golden Vector；Device add/remove/revoke、Epoch 连续/乱序/重放/fork/rollback/unknown profile；History/Device History/Recovery 负向向量；Rust/Desktop IPC/Swift 真机/Kotlin 真机 FFI 的调用、错误、取消、可靠流、背压、Crash/Resume、内存 | Frozen Scope、已决 Client/Server/Crypto ADR、Threat Model/数据分类/信任边界/Risk Register；两类批准链、日志根/split-view 报告；精确 lock、SBOM、漏洞/许可证、五端 harness、真机报告、独立 Crypto/Security 设计评审及发现关闭、Proto skeleton build | Product、Architecture、Security、Identity/Device Authority、Crypto、Client-core、Desktop/Native Bridge、Contracts、iOS、Android |
| M1 / G1 | 工程基础可重复且契约可并行消费 | AC-001/009/011；PR required checks 的正例与故意失败负例；Build、Test、Secret Scan、Dependency Source/Integrity Scan；最小权限 workflow；干净 checkout、cache miss 与断网复跑；Buf lint/build/breaking/generate；五语言 Golden Frame unknown-field；N-1 read/write；OIDC/Device Credential 分离；Secure Storage adapter；私有开发栈和默认拒绝身份/网络 | required-check 配置/运行记录和阻断合并负例、runner/tool/lock 版本、clean/cache-miss/offline 结果、最小权限与 fork 无 Secret 证明、依赖来源/digest、生成 diff、frame digest、N-1 matrix、签名/SBOM/secret scan、开发栈重建记录 | Integration/Release、Contracts、Client Platform、Identity、Platform、Security |
| M2 / G2 | Desktop 首个 E2EE-to-Agent 垂直切片 | AC-002/003/004/006/007/008/012；Runtime/Model/Recovery/Worker/Realtime/队列故障下 IM；Outbox crash、100 次重试、Gap；Message → Task → Context → Approval → encrypted Artifact；运行中撤权；单 Owner、Lease/Fencing、同 Run 转交与 Retry 新 Run；成员/Epoch 负向；Retention 与可观测性负证据；跨 Tenant/Attribution 负向 | 事务/ACK、Cursor/Outbox digest、Epoch/Retention/故障时间线、Task/Run/Grant/Approval 时间线、Artifact provenance、全链路 C3/C4 扫描 | Product、Core/Sync、Client-core、Desktop、Runtime、Model、Crypto、Security、Quality |
| M3 / G3 | Collaboration 与原生 Mobile Alpha | AC-003/005/009；跨 Desktop/iOS/Android 复跑 Outbox/重复/顺序/Gap；DM/Channel/Thread/Reply/Reaction/Edit/Redact/File/Search；上传中断、Scanner 故障、ACL 撤权；iOS/Android 真机消息、Outbox、进程恢复；Push 丢失/Air-gap Cursor；输入法/读屏 | 五平台最终状态、每设备 Cursor/Outbox、文件/索引/缓存清理、真机与最低 OS 记录、Push 降级、无障碍报告 | Desktop、iOS、Android、Client-core/Sync、File/Search、Product/Design、Security、Quality |
| M4 / G4 | Runtime/Capability/Approval/Model Beta | AC-006/007/008/009；Grant 跨 Actor/Device/Task/Run 重放；Connector path fuzz/TOCTOU；Approval 目标/Hash 修改；Model Endpoint 冒充/Fallback 越权；Mobile 只控制不执行；Context/Prompt/Tool output 清理 | 授权拒绝矩阵、path fuzz corpus、Approval digest、egress capture、Run 清理与 canary 扫描 | Runtime、Connector、Core/Approval、Model、Desktop、iOS、Android、Security、Quality |
| M5 / G5 | Private Deployment 与 Recovery Beta | AC-005/008/010/011/012；文件/搜索 ACL 与撤权复跑；运行中 Grant/Approval 撤权和 Recovery 越权负向；Recovery 双人/非自批、过期/损坏/替换/重放；所有非恢复身份调用 KMS/HSM 拒绝；Recipient 范围；Key 轮换/HSM 灾备；Helm/七工作负载；PITR、upgrade/rollback；Retention/audit/diagnostic | 文件/索引/撤权清理、Grant/Approval digest、IAM/Network/KMS policy、审批链、恢复输出 digest、备份与 RPO/RTO、升级/回滚、Runbook、全表面扫描 | Recovery Security、Platform/SRE、Data、File/Search、Runtime/Approval、Audit/Compliance、Release、Security、企业运维代表 |
| M6 / G6 | Release Candidate / Feature Complete | AC-001 至 AC-012 全部在同一候选制品通过；五平台安装/签名/升级/失败迁移；Load/soak/backpressure；PG/NATS/Redis/Realtime/Runtime/Model/KMS/network/client chaos；ACK 后零丢失；容量/耗电/内存/磁盘/无障碍 | 完整追踪矩阵、SLO 报告、长时曲线、Chaos 时间线、五平台制品/设备、缺陷复跑、canary 零命中 | Product、Architecture、Security、五平台 Owner、SDET、SRE、Release |
| M7 / G7 | Security / 第一轮企业 Pilot Candidate | AC-010/012；对冻结生产实现执行第二阶段独立 Crypto Review，覆盖 provider、adapter、FFI、Message/History/Recovery Envelope 与 KMS/HSM；Pentest；全部 Critical/High 关闭与复测；第一轮企业 UAT；供应链和离线包复核 | 冻结实现 digest、全新 M7 外部报告、整改 commit 与复测函、Recovery/Retention/可观测性负证据、Risk Register 关闭记录、企业 UAT 签字、候选制品 provenance；M0 设计评审报告、签字或关闭证据不得复用为 M7 证据 | 独立 Crypto Reviewer、Pentest Owner、Recovery Security、Enterprise Pilot Owner、Product、Security、Release |
| Pilot Hardening | 第二轮企业 Pilot / GA 输入 | 独立环境升级、恢复、真实组织工作流和支持 Runbook；第一轮问题复测；平台差异；最终安全复测 | 第二轮 Pilot 签字、升级/恢复演练、支持记录、阻断缺陷关闭 | 第二家/第二轮企业代表、Pilot Owner、SRE、Security、Product、Release |

M7 的第一轮 Pilot 与 Pilot Hardening 的第二轮 Pilot 是两个独立日历和环境证据。自动化 E2E、内部 dogfood、模拟负载或一次 UAT 均不能替代两轮企业 Pilot。

### 5.1 AC × Gate 追踪

下表逐字追踪 T002 每个场景的“覆盖 Gate”。此外，M6 的 Feature Complete 清单要求同一候选制品复跑 AC-001 至 AC-012；这项全量回归不改写各场景声明的较早 Gate。

| AC | T002 声明的覆盖 Gate | 本计划必须复跑的位置 |
| --- | --- | --- |
| AC-001 | M0、M1、M6 | G0 anti-ghost 子集；G1 完整设备流程；G6 全量回归 |
| AC-002 | M2、M6 | G2、G6 |
| AC-003 | M2、M3、M6 | G2、G3、G6 |
| AC-004 | M0、M2、M6 | G0、G2、G6 |
| AC-005 | M3、M5、M6 | G3、G5、G6 |
| AC-006 | M2、M4、M6 | G2、G4、G6 |
| AC-007 | M2、M4、M6 | G2、G4、G6 |
| AC-008 | M2、M4、M5、M6 | G2、G4、G5、G6 |
| AC-009 | M1、M3、M4、M6 | G1、G3、G4、G6 |
| AC-010 | M5、M6、M7 | G5、G6、G7 |
| AC-011 | M1、M5、M6 | G1、G5、G6 |
| AC-012 | M2、M5、M6、M7 | G2、G5、G6、G7 |

### 5.2 G0 / G1 证据协议

G0 先验证边界再批准实现：Frozen Scope、Client/Server/Crypto ADR、Threat Model、数据分类、信任边界与 Risk Register 必须引用同一候选；AC-001 M0 子集由 Identity/Device Authority Owner 负责首台与后续设备批准、恶意 Control Plane/普通管理员/伪造链、Ghost Device、追加式日志根与 split-view 负向；独立 Crypto/Security 设计评审必须分别审查候选 provider、Threadline adapter、FFI、Message/History/Recovery Envelope 和 KMS/HSM 设计/集成，并把每项发现绑定关闭 commit 与复测。任一必决 ADR 仍为 `proposed`、Threat/Risk 缺口、到 G0 已到期但未关闭的 Critical/High、独立评审缺项或真机/互操作缺项都使 G0 为 `HOLD`；尚未到 Due Gate 且能力未启用的风险依照风险台账保持 `Open`，不冒充 G0 阻断或关闭。这是 M0 基线；M2 才关闭完整成员/消息运行语义。

G1 的 required checks 至少包括 Build、Test、Secret Scan 和依赖来源/完整性扫描。每项既要保留绿色正例，也要用隔离 PR/受控测试变更证明失败会阻断合并；只展示 workflow YAML 或可手动跳过的 job 不算证据。Workflow 顶层和 job 权限最小化，非受信 fork 不接触 Secret、写 Token、签名或发布环境。可重复构建证据必须来自干净 checkout，并分别记录正常、cache miss 和禁网/离线输入三次结果、工具/lock/制品 digest；cache、全局工具或公网下载不得成为隐含依赖。

## 6. 关键能力覆盖矩阵

| 测试域 | 必须覆盖的主路径与负向路径 | 层级 | 平台/系统 | 最晚 Gate | Owner / 证据 |
| --- | --- | --- | --- | --- | --- |
| Identity / Device | 首设备 Device Authority、后续设备批准、普通管理员/恶意 Control Plane/伪造链、Ghost Device、追加式 Device/Key 日志根与 split-view、Credential/KeyPackage 过期/复用/跨 Tenant、撤销 | Contract、E2E、security adversarial、真机 | 五客户端 + Core | M0 基线 / M2 完整 | Identity/Device Authority + Crypto；批准链、日志根、split-view 与拒绝矩阵 |
| Message / Sync | Local Outbox、Durable ACK、100 次幂等、排序、Cursor/Gap、PG/NATS/Redis/Realtime 故障 | Property、Integration、E2E、Load、Chaos | Client Core + Server | M2 基线 / M6 关闭 | Core/Sync + Client-core + SRE；ACK/Cursor/zero-loss |
| Epoch / membership | add/remove/revoke/self-update、`rekey_required`、离线/未来/重复/乱序/fork/rollback、unknown Profile | Crypto vector、property/fuzz、E2E、N-1 | Rust + Swift + Kotlin + 独立 RFC 实现 | M0 | Crypto + Security；transcript、外部状态 digest |
| History | 新成员显式共享、同成员新设备、无旧设备、撤销后请求、Retention/ACL、跨 Tenant/Group/Epoch | Library-independent vector、Contract、E2E | Crypto/Client + Recovery boundary | M0 语义 / M5 完整 | Crypto；向量、稳定错误、无 Key 释放 |
| Recovery | 成功 Recipient、单人/自批、过期/损坏/替换/重放、KMS/审计故障、非恢复身份、轮换/灾备 | Contract、E2E、Chaos、security drill | Recovery Control + HSM + recipient device | M5 | Recovery Security；Policy、审批、HSM audit |
| FFI / IPC | version negotiation、错误/unknown、取消前后 commit、bounded reliable stream、backpressure、stale handle、panic isolation、1,000 lifecycle、Crash/Resume | Contract、fault injection、memory、真机 | Desktop IPC、iOS arm64、Android arm64/API floor | M0 / M1 | FFI + platform Owners；host reports、leak/memory、cursor |
| N-1 compatibility | Proto unknown fields、Golden canary、Bridge minor/major、Crypto Profile、server/client rolling upgrade、schema read/write/rollback | Contract、Golden Frame、E2E upgrade | 五语言、五客户端、Server/Sidecar/DB | M1 基线 / M5-M6 | Contracts + Release；pairwise matrix、old/new frames |
| File / local search | pre-encryption scan、multipart resume、checksum、ACL narrowing gate、local index rebuild、revoke/cache erase | Integration、E2E、fuzz、load | 五客户端 + Blob | M3 | File/Search + platform Owners；blob/index/ACL evidence |
| Runtime / Connector | bounded Context、no IM DB、Lease/Fencing、same-run transfer、retry new run、path traversal/symlink/TOCTOU、revocation | Contract、E2E、fuzz、Chaos | Desktop Runtime + locald/connectord + Gateway | M4 | Runtime/Connector；grant/time/path and old-writer rejection |
| Model egress | no-Prompt Model Control contract、Endpoint identity/region/retention、Run pin、approved fallback、expiry | Contract、E2E、adversarial network | Runtime + Model Control + Endpoint | M4 | Model/Runtime Security；schema allowlist、egress capture |
| Private operations | offline install, default deny, seven workloads, PITR, migration, rolling upgrade/rollback, observability redaction | Integration、E2E、Load、Chaos, drill | Server stack + five release artifacts | M5-M6 | Platform/SRE/Release；provenance、RPO/RTO、runbook |
| Retention / Audit | online/offline expiry、clock/policy/snapshot rollback、tombstone、backup lifecycle、audit tamper/tenant injection | Property、E2E、Chaos、security scan | Client/Worker/Audit/Backup | M5-M6 | Compliance + Client-core + SRE；deletion state、audit checkpoint |

## 7. Contract、Golden Frame 与 N-1 策略

主分支 Protobuf 是跨端契约事实源。每个 Contract PR 先在隔离输出目录执行固定工具链，禁止 Feature Workstream 手改生成物或以 JSON/handwritten DTO 建第二套协议。

必须验证：

- `buf lint`、T014 后固定的 main breaking baseline、`buf generate` 和五语言 native compile/test；T014 bootstrap 例外只允许一次。
- 持久化 Envelope 保留未知字段 `50000` canary；decode/re-encode 和修改已知字段后 canary 仍存在。
- Golden Frame 只含代表性公共字段、合成不透明字节和 canary，不含生产 ID、正文、Token、Key 或真实 Ciphertext。
- Go、TypeScript、Rust、Swift、Kotlin 对已知字段、未知 enum/error fallback、absent field 和授权语义一致。
- N-1 reader/current writer、current reader/N-1 writer、N-1 client/current server、current client/N-1 server、current App/N-1 Sidecar，以及可逆 Schema 双向窗口。
- Crypto library 版本不等于 Wire Version；仍支持固定 `tl-mls-1` 的 N-1 客户端可同步，不支持目标 Profile 的客户端只读或明确不兼容，绝不降级 Cipher Suite。
- 生成器/runtime 升级必须重放全部历史 frames。未知字段不能保留的 adapter 禁止写持久状态，即使 Buf breaking 通过。

每份 N-1 证据记录版本对、读取方/写入方、Profile/Schema、输入 frame digest、结果状态、保留 canary digest、升级/回滚路径和支持窗口。

## 8. Crypto Golden Vector 策略

Crypto 最高测试 seam 是 library-independent 的 `client-crypto` Protocol Harness。测试断言外部 Group/Epoch/Envelope 状态和稳定错误，不断言 OpenMLS 内部类型、序列化或调用顺序。

向量族至少包括：

- RFC 9420 Known Answer 与至少一个独立实现可消费的 byte-level transcript。
- KeyPackage 生成/消费、Welcome、Device add/remove/revoke、自更新、并发 Commit、离线设备和 successor Epoch。
- replay、old/future Epoch、损坏、跨 Tenant/Group、fork、rollback、unknown Profile/version 和 downgrade。
- History Sharing 与 Device History Sharing：授权、未授权、无旧设备、撤销、Retention、ACL、跨绑定和损坏输入。
- Recovery Envelope：Tenant/Group/Epoch/Profile/Key Version/object/Recipient 绑定，缺少 Recipient、拒绝、损坏、过期、跨域、KMS/审计不可用。
- Crash/Resume、加密持久化、Key 清除、Retention 到期、备份恢复、目标 Group size/Epoch 频率/移动内存。

向量输出只记录状态、稳定错误、公共元数据、长度桶和不可逆 digest。公开 KAT 与纯 `synthetic_opaque` parser/state-machine 输入可使用记录的固定 seed。C3/C4 canary、测试私钥、nonce 和任何可还原敏感输入必须由运行时 CSPRNG 逐 run 生成，不得从仓库 seed 派生；进程内使用后清零，不得写入仓库 Fixture、日志、Crash Dump 或 evidence。敏感 fuzz 复现只把 seed 暂存于访问受限、短期加密的测试 vault，Git 和常规 artifact 仅保留 seed policy、最小化 shrink trace 与 keyed digest。公开标准 KAT 如必须包含公开的测试密钥材料，须标记 `public_test_material`、记录规范出处和 hash，并证明从未用于任何真实身份、Tenant 或制品签名。

当前 T011 semantic vector 是起点，不是生产批准：缺少真实 Swift/Kotlin FFI、独立 MLS 实现、KMS/HSM 恢复解密，且 OpenMLS 0.8.1 有生产阻断。所有这些必须作为 M0/M5 的显式 `NOT RUN/FAIL` 保留，不能用 wrapper `catch_unwind` 或服务端明文修饰成通过。

## 9. E2E、Load、Soak 与 Chaos 策略

E2E 使用唯一合成 Tenant、Actor、Device、Channel、Workspace 和 Recovery Case；每次运行从干净逻辑状态开始。测试通过 API/UI 观察公开状态，通过独立只读探针核验事实源、审计、密文边界和负证据，不能直接改数据库制造“成功”。

Load/soak 先定义代表性规模模型和预算，再执行：消息 ACK/投递、热点 Channel、慢消费者、Commit 冲突、KeyPackage 耗尽、文件 multipart、Runtime dispatch、Recovery queue、客户端 Timeline/FTS、移动电量/内存和长期 handle/queue 泄漏。报告必须给出样本数、持续时间、预热、环境、p50/p95/p99、错误率、资源曲线、阈值和原始脱敏测量 artifact，不能只截 Dashboard。

Chaos 每次声明稳态、不变量、故障开始/结束、预期退化和恢复界限。至少注入 PG failover/暂停、NATS/Redis/Realtime/Worker、Runtime/Model/Recovery/KMS、对象存储、网络分区/乱序/丢包、client/locald/agentd/connectord crash、磁盘满/WAL/migration 中断。必须证明：无假 ACK、ACK 后零丢失、Pending Outbox 保留、Cursor 补洞、旧 Fencing Token 拒绝、普通 IM 与非关键子系统隔离、恢复失败不降级权限。

故障演练与 Runbook 双向校验：测试观察与 Runbook 不一致时，修实现或修 Runbook 后重跑；不能把未知行为作为通过。

## 10. 真机与五平台 Fixture 策略

模拟器/Emulator 负责快速 Contract 与故障回归；以下项目必须在物理设备或受支持发布环境执行：

- iOS/Android arm64 FFI 加载、签名、Keychain/Keystore、锁屏/生物识别、后台/进程回收、内存压力、通知、设备备份/迁移和 Crash/Resume。
- Android API 28 最低运行时、当前稳定 API 和预览 forward-smoke；iOS 17 最低运行时和当前稳定 iOS。
- macOS、Windows、Linux 分别验证 Sidecar 身份/权限、安装、签名、WebView/系统依赖、升级、失败迁移、回滚和离线包。
- 五平台对同一 Contract Fixture、Crypto Profile、错误、Cursor 和最终物化状态给出一致语义；平台 UI 与生命周期证据分别保留。

真机证据记录匿名设备资产 ID、型号、OS/build、CPU ABI、App/Bridge/Core/Profile/Schema、签名与制品 digest、测试 run、内存/电量工具、结果和复核人。不得采集个人设备内容、UDID 原值、完整本地路径、通知正文、系统账户或生产凭据。

## 11. 非功能需求追踪矩阵

下表逐项覆盖 [PRD 第 12 节](../product-requirements.md)。Owner 对控制与证据负责；Quality 负责完整性和复核。

### 11.1 可靠性

| ID | 要求 | Owner | 测试层级 / 证据类型 | Gate |
| --- | --- | --- | --- | --- |
| NFR-R01 | 同区域消息发送确认 p95 < 300ms | Core/Sync + SRE | 受控 Load/soak；延迟分布、环境和容量模型 | M6 |
| NFR-R02 | 在线接收投递 p95 < 1s | Realtime/Sync + SRE | Load + 网络扰动；端到端时间戳 digest 与 p95/p99 | M6 |
| NFR-R03 | Channel 有序、At-least-once、Consumer 幂等 | Core/Worker + Quality | Property/Integration/Chaos；重复/乱序/最终状态 digest | M2 / M6 |
| NFR-R04 | Durable Cursor 恢复并修复 Gap | Client-core/Sync | Property/E2E/Chaos；Gap 注入、Cursor 不越洞、收敛 | M2 |
| NFR-R05 | Client Outbox crash 后恢复，ACK 前不丢 | Client-core + Platform | Crash/Resume E2E；崩溃前后 outbox/ACK/zero-loss | M2 |
| NFR-R06 | 每设备独立 Cursor，不同步 SQLite 文件 | Client-core + 五平台 Owner | Contract/E2E；独立 DB identity、五端 Cursor 与共享卷扫描 | M3 |
| NFR-R07 | SQLite 单 Writer，Busy/Crash/WAL 有明确重试 | Client-core/Desktop/iOS/Android | Integration/fault；多窗口/进程回收/WAL recovery | M2-M3 |
| NFR-R08 | Run Lease + Fencing 阻止旧 Writer | Runtime/Core | State-machine/E2E/Chaos；generation、旧 token 拒绝 | M2 / M4 |
| NFR-R09 | Runtime 故障不阻塞消息主链路 | Architecture + Runtime + Core | 组件停机 E2E/Chaos；IM 可用、Task 明确退化 | M2 |
| NFR-R10 | 通用版本可用性 >= 99.9% | SRE + Product | 预生产 soak/SLO 与 Pilot；错误预算、事件记录 | M6 / Pilot |
| NFR-R11 | RPO < 5 分钟 | Data/SRE | PITR/灾难演练；最后持久点、丢失窗口、审计 | M5 |
| NFR-R12 | RTO < 60 分钟 | Data/SRE + 企业运维 | 黑盒恢复演练；时间线、Runbook、恢复后校验 | M5 / Pilot |

### 11.2 安全与治理

| ID | 要求 | Owner | 测试层级 / 证据类型 | Gate |
| --- | --- | --- | --- | --- |
| NFR-S01 | TLS 传输与静态加密 | Platform/Security + Client Platform | 配置/网络负向、存储检查、证书轮换 | M1 / M5 |
| NFR-S02 | Device DB/Blob Cache 加密，Key 在 OS Secure Storage | Client-core + 五平台 Owner | 真机/文件扫描/备份迁移；Key 不落普通存储 | M1-M3 |
| NFR-S03 | 消息/附件 E2EE，Server 仅 Ciphertext 且无可解密索引 | Crypto + Core/File + Security | Crypto/E2E/全表面 canary scan | M0 语义 / M3 |
| NFR-S04 | 移除 Member/Device 推进 Epoch，覆盖离线和乱序 Commit | Crypto + Core/Sync | Golden Vector/property/E2E；`rekey_required` 与 successor | M0 / M2 |
| NFR-S05 | Recovery Key 仅 KMS/HSM，多审批、不可删除审计 | Recovery Security + Audit | Policy adversarial/E2E/HSM audit | M5 |
| NFR-S06 | Server、Model、Agent、Runtime、Endpoint 不得取得 Recovery Key | Recovery Security + Platform | 全身份拒绝矩阵、Network/IAM/KMS policy | M5 |
| NFR-S07 | 每次存储查询与对象 Key Tenant 隔离 | Core Authorization + Quality | IDOR/property/E2E；跨 Tenant ID 交换和缓存污染 | M2 |
| NFR-S08 | 服务与 Runtime Credential 使用统一 Secret Manager | Platform + Runtime Security | 配置/Pod/环境/日志扫描、过期与轮换 | M1 / M4 |
| NFR-S09 | Audit 不可变且独立 Retention | Audit/Compliance + Security | Tamper/重排/跨 Tenant/collector 故障、checkpoint | M5 |
| NFR-S10 | Retention、Legal Hold/Export Hook、删除可配置且不扩解密主体 | Compliance + Client-core/Worker | 策略/离线/回滚/E2E/backup lifecycle | M5 |
| NFR-S11 | Rate limit、滥用防护、Client scan hook、异常登录 | Identity/Core + File + Security | Load/adversarial/scanner TOCTOU/login anomaly | M3 / M6 |
| NFR-S12 | 高影响管理员动作二次确认或审批 | Admin/Core + Security | UI/E2E/replay/TOCTOU；目标/内容/policy digest | M4-M5 |
| NFR-S13 | Data Residency 与 Customer-managed Key 兼容 | Architecture/Platform/Model/Recovery | 部署/egress policy/KMS integration/Pilot | M5 / Pilot |

### 11.3 客户端质量

| ID | 要求 | Owner | 测试层级 / 证据类型 | Gate |
| --- | --- | --- | --- | --- |
| NFR-C01 | Desktop、iOS、Android 使用同一 IM 协议与状态语义；Admin Web 只消费 v1 管理子集协议 | Contracts + Desktop/iOS/Android/Admin Web Owner | 五语言 Contract、五平台 IM E2E、Admin Web generated TypeScript contract/browser integration/授权拒绝与安全错误状态；不得以此扩展 Browser IM | M3 / M6 |
| NFR-C02 | Tauri Desktop、SwiftUI iOS、Compose Android 技术边界 | Client Platform/Architecture | Build/package inspection、平台发布制品 | M1 |
| NFR-C03 | 共享 Rust Core/Proto/Token，不共享页面代码 | Client-core/Contracts/Design | 架构静态检查、Contract 与生成 provenance | M1-M3 |
| NFR-C04 | 短期断网可离线阅读和排队发送 | Client-core + 五平台 Owner | 飞行模式/网络断开/进程恢复 E2E | M2-M3 |
| NFR-C05 | 无障碍、键盘、国际化和时区正确 | Product/Design + 五平台 Owner | axe/XCTest/Compose/人工读屏、IME/locale/timezone matrix | M3 / M6 |
| NFR-C06 | 大 Channel windowed rendering 与分页历史 | Desktop/iOS/Android + Performance | 10k 会话/大 timeline load、内存/帧率/分页正确性 | M6 |
| NFR-C07 | Agent Streaming 不导致 Timeline 抖动或重排 | Desktop/iOS/Android + Runtime UI | UI perf/E2E；帧率、layout shift、稳定 message order | M4 / M6 |

### 11.4 私有化部署

| ID | 要求 | Owner | 测试层级 / 证据类型 | Gate |
| --- | --- | --- | --- | --- |
| NFR-P01 | 私有部署是默认产品形态 | Product + Platform | 安装/UAT；默认配置和文档审查 | M5 |
| NFR-P02 | 全栈在企业内网运行且无公网入站 | Platform/Security | Standard/Air-gap E2E、网络 capture、deny policy | M5 |
| NFR-P03 | Desktop Runtime 仅主动出站，不开放工作站入站端口 | Runtime + Desktop Security | 端口/防火墙/连接方向测试 | M4-M5 |
| NFR-P04 | 支持内网 DNS/CA/OIDC/S3/KMS/模型；不误收 SAML/SCIM/LDAP | Platform + Integration | 私有依赖集成、scope/static check | M5 |
| NFR-P05 | 签名 OCI/Helm/SBOM/Checksum/Migration/offline rollback，无在线依赖 | Release/Platform | Air-gap install、验签、provenance、upgrade/rollback | M5-M6 |
| NFR-P06 | Standard 仅白名单 Proxy；Air-gap 无公网出站 | Platform/Security + Model | egress capture、policy bypass、fallback 拒绝 | M5 |
| NFR-P07 | Air-gap Mobile 被挂起不承诺唤醒，恢复后 Cursor 补齐 | iOS/Android + Sync | 真机 suspend/push-off/reopen E2E | M3 / M6 |
| NFR-P08 | 遥测默认关闭；诊断包显式、预览、脱敏导出 | Observability/Security + Client Platform | 默认流量、生成审批、C3/C4 scan、expiry | M5-M6 |

## 12. Fixture 与测试数据总则

所有测试数据遵循 [数据分类](../security/data-classification.md)、[信任边界](../security/trust-boundaries.md) 和 [`test/fixtures/README.md`](../../test/fixtures/README.md)：

- 只使用生成的 Tenant/Actor/Device/Object ID；禁止导入生产数据库、客户导出、真实消息、真实文件、真实路径、真实 Token/Key 或真实模型 Prompt。
- Repo 内 Fixture 只允许公共 metadata、枚举/状态、长度、合成不透明字节、公开 KAT 和不可逆 digest；不持久化消息/文件/Artifact/Prompt 明文或可用凭据/私钥。
- E2E 所需 C3/C4 canary 在隔离测试进程内临时生成，原文只驻留内存或 tmpfs；跨边界后只能是密文。报告只记录分类、surface、命中数和不可逆 digest。
- 公开 KAT 与 `synthetic_opaque` 可记录固定 seed；C3/C4、私钥、nonce 和敏感 fuzz 输入使用 runtime CSPRNG。敏感 seed 只进短期加密测试 vault，证据仅含 policy、shrink trace 和 keyed digest。
- 每个 fixture set 有 manifest、schema/version、owner、generator/version、seed policy、classification、allowed surfaces、cleanup、license/provenance 和 SHA-256。
- 证据导出前执行内容/Secret Scanner。任何非预期 C3/C4 命中使测试失败并触发 R-013/R-024 Release Blocker 处理。

## 13. 独立评审、Pentest 与 Pilot 不可替代规则

- 独立 Crypto Review 必须覆盖上游库/provider、Threadline adapter、FFI、Message/History/Recovery Envelope、Key 生命周期和 KMS/HSM 流程。内部单元、fuzz、Golden Vector、cargo audit 或“使用标准库”均不能替代。
- Pentest 必须覆盖身份/跨 Tenant/Ghost Device、E2EE/重放、Runtime/Connector、Model egress、Recovery、供应链、管理面和私有部署。SAST/DAST/secret scan 不能替代有范围和复测的独立 Pentest。
- 第一轮企业 Pilot 验证 UAT、数据处理和运营准备；第二轮在独立环境验证 hardening、升级、恢复和支持。内部 dogfood、一次试点或合成 E2E 不能替代两轮。
- 外部发现必须关联修复 Commit 和复测证据。Critical/High 未关闭时保持 `HOLD`，不能以风险接受、临时管理员解密、明文回退或测试环境特例通过。

## 14. 每次候选的证据包

证据包至少包含：

- Gate、候选版本、Repo Commit、全部制品/镜像/配置/Policy digest。
- Proto、Bridge、Crypto Profile、Schema、toolchain 和 N-1 配对版本。
- 环境、匿名设备/OS、执行者、复核者、开始/结束时间、全部尝试，以及允许公开的固定 seed；敏感输入只记录 seed policy、shrink trace 与 keyed digest。
- 场景与 NFR 结果、故障时间线、预期/实际转换、性能原始脱敏测量和缺陷链接。
- Audit/Metric/Trace/object digest 和所有表面 C3/C4 扫描摘要。
- `PASS/FAIL/NOT RUN/approved N/A`、显式关闭能力、剩余风险、签字与重新打开条件。

证据包不可包含正文、Prompt、搜索词、完整路径、Authorization/Cookie、Token、私钥、Channel/Epoch/Content/History Key、Recovery Envelope 原始字节或可复用恢复输出。易变 Dashboard、口头确认、个人目录和单次绿色 CI 链接不构成独立证据；必须有不可变或内容寻址的 manifest 与 digest。

## 15. 重新打开与维护

以下变化必须重跑受影响 Contract/Golden/E2E/NFR 并由 Quality 与 Security 决定是否重开 Gate：Crypto Provider/Profile、Device Authority、History/Recovery、Proto major/持久化字段、FFI major、DB migration、KMS/HSM、模型 Endpoint/数据策略、Connector 类型、平台最低 OS、签名根、部署 topology、Retention 或 Frozen Scope。

每个 Gate 前更新矩阵；每个 Candidate 固化证据；至少每四周与风险台账对齐一次。远期测试可以保持设计状态，但在输入 Contract Integrated 前不得实现一套猜测 fixture 或把远期 `NOT RUN` 隐藏为覆盖率。

## 16. 依据

- [v1 Scope Freeze](../acceptance/scope.md)
- [产品需求文档](../product-requirements.md)
- [交付计划](../delivery-plan.md)
- [Client Platform ADR](../adr/0001-client-platform.md)
- [Server / Protocol / Storage ADR](../adr/0002-server-protocol-storage.md)
- [Group E2EE / Recovery ADR](../adr/0003-group-e2ee-recovery.md)
- [数据分类](../security/data-classification.md)
- [威胁模型](../security/threat-model.md)
- [风险台账](../security/risk-register.md)
- [平台兼容性基线](../build/platform-compatibility.md)
- [可重复构建 Runbook](../build/reproducible-builds.md)
- [T011 E2EE 互操作 Spike](../spikes/e2ee-interop.md)
