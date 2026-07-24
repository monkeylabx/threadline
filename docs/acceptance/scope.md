# Threadline Private Enterprise v1.0 Scope Freeze

状态：Frozen 1.1

Gate：G0 / M0

Owner：Product / Architecture

Issue：[#15 T001](https://github.com/monkeylabx/threadline/issues/15)

工程基线：`216287d`

冻结日期：2026-07-24

产品批准人：`monkey-lab-x`

## 1. 冻结目的

本文冻结 Threadline Private Enterprise v1.0 的目标用户、产品承诺、平台范围、核心流程和明确
非目标。后续 PRD、ADR、Threat Model、Proto、客户端和部署任务必须引用本基线；超出本基线的需求
必须经过变更评审和重新排期。

本文冻结的是产品结果和信任边界。群组密码协议、企业恢复封装、Rust FFI 和模型授权格式等实现细节
必须由 M0 ADR 与 Threat Model 评审后才能进入生产实现。

## 2. 产品承诺

Threadline 是一套面向企业内网的 Agent 原生 IM。团队先获得完整、可靠、端到端加密的企业沟通能力，
再将一段对话转换为有权限、可观察、可审批、可接管的 Agent Task。模型或 Runtime 离线时，IM 仍能
独立工作。

v1 的差异化承诺不是“聊天中附带一个机器人”，而是：

- Human、Agent 和 Service 都是具有身份、成员关系、职责和审计记录的 Actor。
- Channel 保存长期协作历史；Task 和 Run 保存一次具体执行的上下文、活动与产物。
- 普通消息不触发模型；Task、消息转任务和管理员配置的规则才触发 Agent。
- Runtime 只通过短期 Capability Grant 获取明确选择的消息、文件和 Workspace。
- 本地文件只由用户授权设备上的 Local Connector 访问，服务端不能浏览成员文件系统。
- 消息正文和附件从发送设备到成员设备保持 E2EE；IM 服务端只保存密文。
- 高影响动作必须在执行前展示目标、范围、期限和影响，并留下审批与审计证据。

## 3. 目标用户

### 3.1 主要使用者

| 用户 | 必须解决的问题 | v1 成功结果 |
| --- | --- | --- |
| 普通员工 | 沟通、查找信息并把工作交给 Agent | 不离开 IM 即可创建并跟踪 Task |
| 团队负责人 | 协调 Human 与 Agent，识别风险和阻塞 | 可以分配、观察、中断、转交和验收工作 |
| 操作审批人 | 判断 Runtime 请求是否安全且必要 | 批准前能看清 Capability、资源和影响范围 |
| Agent 负责人 | 管理 Agent 职责、参与模式、预算和路由 | Agent 行为有 Owner、Policy 和审计依据 |

### 3.2 部署与治理者

| 用户 | 必须解决的问题 | v1 成功结果 |
| --- | --- | --- |
| IT 管理员 | 在内网安装、升级、备份并接入企业基础设施 | 可离线部署并完成升级、回滚和恢复演练 |
| 安全管理员 | 管理设备、恢复权限、保留策略和审计证据 | 服务端不能读正文；恢复必须审批且可追溯 |
| 访客或外包 | 只访问明确共享的协作空间 | 权限不能越过 Organization、Channel 和资源 ACL |

## 4. v1 核心流程

1. **纯人类沟通**：Runtime 和模型全部离线时，成员仍可使用 DM、Channel、Thread、文件、搜索、
   通知和多端同步。
2. **端到端加密同步**：发送设备加密消息和附件；服务器持久化有序 Ciphertext Envelope；授权设备
   获得密钥后解密并建立本地搜索索引。
3. **从讨论到执行**：成员选择消息创建 Task，绑定上下文和 Workspace，选择执行设备并启动 Run。
4. **本地受控执行**：授权 Desktop Runtime 在隔离 Worktree 或 Sandbox 中工作；Local Connector 只
   暴露明确路径和有效时段。
5. **团队观察与审批**：成员在 IM 中查看结构化活动、阻塞、审批、Diff 和 Artifact，并可中断、重试、
   转交或拒绝结果。
6. **移动协作**：Mobile 可以沟通、发起和观察 Task、中断 Run、处理审批，但不执行长任务或访问任意
   本地文件系统；目标 Desktop 离线时任务排队等待。
7. **企业治理**：管理员通过 OIDC、设备清单、RBAC、ACL、Retention、Audit 和受控恢复流程管理风险。
8. **私有化运维**：运维人员在单 Region 企业环境完成安装、监控、密文备份恢复、升级回滚和离线交付。

T002 负责把以上流程拆成可执行的端到端验收步骤；本文只冻结场景边界。

## 5. 平台与客户端范围

| 平台 | v1 技术方向 | v1 范围与限制 |
| --- | --- | --- |
| Desktop | Tauri 2 + React + TypeScript | macOS、Windows、Linux；完整 IM、本地缓存、Agent Runtime、Local Connector |
| iOS | Swift + SwiftUI；复杂列表可使用 UIKit | 完整消息、Task 发起/观察/中断和审批；不执行长任务 |
| Android | Kotlin + Jetpack Compose | 完整消息、Task 发起/观察/中断和审批；不执行长任务 |
| Shared Client Core | Rust，通过稳定 FFI 接入原生端 | E2EE、加密 SQLite、Outbox、Sync、Search、附件加密；不共享页面代码 |
| Admin Web | 私有化管理、治理和运行状态页面 | 不交付完整 Browser IM Client |
| Server | 单 Region 私有化部署、Helm、离线 Bundle、备份恢复、升级回滚 | 不提供 SaaS 多租户运营或跨 Region Active-Active |

Desktop 先完成首个完整垂直切片；iOS 与 Android 基础工程、Rust FFI、密钥存储和消息列表并行建设，
随后接入同一套 E2EE 消息、同步和 Run 审批契约。

## 6. v1 功能范围

### 6.1 企业 IM

- Organization、Member、Role、Guest、Space、DM、Channel、Thread 和 Membership。
- 文本、代码、Mention、Reply、Reaction、Pin、附件、编辑、撤回和 Read Cursor。
- Durable Local Outbox、ACK、幂等、Channel 内排序、重连、Cursor Sync 和 Gap Repair。
- 文件分片上传、断点续传、E2EE Blob、预览、权限复检和客户端扫描 Hook。
- 消息、文件和 Task 的设备本地搜索；索引是可重建缓存，不是权限事实源。
- Inbox、通知、Presence、Typing 和基础组织管理。
- OIDC 登录、管理员邀请和 CSV 成员导入。

### 6.2 E2EE 与企业恢复

- v1 所有消息正文和附件默认使用经过评审的 E2EE 协议，不自创密码算法。
- 应用服务器、Realtime、Worker、Model Control、数据库管理员和普通企业管理员不能解密正文。
- PostgreSQL 保存 Ciphertext Envelope、Channel Sequence、成员关系和必要路由元数据；对象存储只保存
  Ciphertext Blob。
- 每台设备拥有独立设备身份、密钥材料、加密 SQLite 和可重建本地搜索索引。
- 企业可配置恢复接收者；恢复私钥只保存在企业 KMS/HSM，不进入 Threadline 数据库或 Runtime。
- 恢复必须经过多人审批并产生不可删除的审计事件；Agent 和模型路由不得使用恢复私钥。
- v1 不向用户开放“严格不可恢复频道”；协议应允许未来省略企业恢复接收者。
- 新设备授权、成员变更、设备撤销、Group Epoch、History Sharing 和恢复失败必须进入 M0 密码 ADR、
  Threat Model 与端到端测试。

### 6.3 Agent 协作

- Agent Actor、Owner、Channel Membership、`manual`、`task_only` 和 `paused` 参与模式。
- 普通消息不调用 LLM；Composer Task 模式、消息转任务和明确规则负责触发。
- v1 自动规则只能创建或分配 Task；不会授予 Agent 持续读取 Channel 的隐式权限。
- 从消息创建 Task，绑定 Context Reference、逻辑 Workspace、执行设备、参与者和预期 Artifact。
- Channel 只保存共享协作语境；每位用户在每台设备独立绑定本地 Workspace，物理路径不上报服务端。
- 一个 Run 同时只有一个 Execution Owner、设备、Workspace Lease 和 Fencing Token；转交必须重新授权。
- Run 创建、结构化 Event、取消、失败恢复、审批、Artifact、Usage 和 Provenance。
- 短期 Capability Grant、运行中撤权、Workspace Lease、Fencing 和隔离 Worktree。

### 6.4 模型发现与路由

- Model Control 发现企业已接入 Endpoint 的模型、能力、参数、健康、评测和评分，但不接触 Prompt。
- Workflow 声明能力与数据边界，不硬编码模型名称。
- 路由先满足数据位置、工具、上下文和输出格式等硬约束，再按质量、延迟、稳定性和成本评分。
- 一个 Run 固定模型和版本；Fallback 只能使用预先批准的候选，用户可为 Workspace 或 Task 覆盖路由。
- 本地 Runtime 解密授权消息、读取授权文件并组装 Prompt，然后直接调用企业批准的模型 Endpoint。
- UI 必须显示数据将发往本地、企业内网或外部 API；IM 服务端和 Model Control 不记录 Prompt。

### 6.5 私有化与治理

- 企业 OIDC、管理员邀请/CSV 导入、设备注册与撤销、RBAC、资源 ACL、Retention Metadata 和 Audit。
- TLS / mTLS、设备加密 SQLite、加密 Blob Cache、企业 KMS/HSM 和短期服务凭据。
- PostgreSQL HA、NATS JetStream、Redis、S3-compatible Object Storage、Vault/HSM 和 OpenTelemetry。
- 单 Region Helm 部署、签名 OCI 制品、SBOM、Secret Scan、备份恢复、升级回滚和 Air-gap Bundle。
- Standard Private 可经白名单 Proxy 使用 APNs / FCM 和经批准的模型 Endpoint；Air-gapped 模式不要求
  任何公网连接。

## 7. 首个垂直切片

第一个集成版本必须证明产品闭环，而不是分别交付孤立的 IM Demo 和 Agent Demo：

1. 两名用户登录单 Region 私有部署。
2. 在 E2EE Channel 中发送消息，并验证离线 Outbox、重连、幂等和顺序。
3. 用户将一条消息转为 Task，选择本地 Workspace 和执行设备。
4. Model Control 根据能力和数据策略返回固定到本 Run 的模型路由。
5. 本地 Runtime 获取有限消息上下文，读取一个授权文件并生成 Patch 或 Artifact。
6. 另一名成员在 Task Thread 中审查并批准或拒绝。
7. 结果和 Artifact 回到频道；Prompt、本地文件和明文消息不经过 IM 服务端。

首个闭环在 Desktop 完成。Mobile 同期建立原生壳、Rust FFI 与消息基础，随后接入消息、Run 观察、审批
和中断。

## 8. 明确非目标

以下能力不属于 Private Enterprise v1.0，不得作为普通需求并入当前排期：

- SaaS 多租户运营、计费、套餐和公网控制台。
- SAML、SCIM、完整 LDAP / AD 目录同步和 Legal Hold 完整工作流。
- 用户可创建的严格不可恢复频道；v1 只交付带企业可审计恢复的 E2EE。
- Enterprise Runner、Remote Runtime 和服务端任意 Workspace 执行。
- 完整 Browser IM Client。
- 公有模型托管、基础模型部署和企业推理平台建设。
- 音视频会议、直播、白板和多人实时文档。
- 跨 Region Active-Active 和全球 Channel Sequencer。
- 服务端明文内容搜索、服务端正文 DLP、服务端摘要和外部语义索引。
- 区块链、P2P 消息共识或在设备间复制 SQLite 文件。

## 9. 外部依赖边界

- Threadline 接入企业已有 OIDC、对象存储、KMS/HSM、内部模型 Endpoint 和可观测系统，但不负责部署
  或运营这些基础设施。
- OIDC 负责登录；v1 成员生命周期由管理员邀请、停用和 CSV 导入承担。
- Standard Private 默认公网出站仅包含 APNs / FCM 和企业显式批准的模型 Endpoint。
- Air-gapped 安装和运行不能依赖公共 CDN、Package Registry、遥测 SaaS 或在线 License Service。
- Runtime 使用项目自有 `agent-core`；Hermes Agent 仅作为研究参考，不是生产依赖。
- 企业模型 Endpoint 是 Prompt 明文的独立信任边界，必须由企业策略批准；Threadline 不负责替企业
  私有部署基础模型。

## 10. 变更触发条件

出现以下任一情况时，Product Owner 和 Tech Lead 必须暂停相关实现、更新本文件并重新计算里程碑：

| 变更 | 必须采取的动作 |
| --- | --- |
| 改变 E2EE、企业恢复或设备密钥边界 | 重开密码 ADR、Threat Model、兼容性与安全审计 |
| 允许 IM 服务端、Model Control 或 Agent 使用恢复密钥 | 视为产品信任边界变更，必须重新批准 Scope |
| 加入 SAML + SCIM | 增加目录生命周期、离职撤权和兼容测试 |
| 加入 Enterprise Runner | 增加隔离、Fleet、凭据、远程文件和运维范围 |
| 加入完整 Browser IM Client | 增加浏览器密钥存储、离线数据库和安全边界评估 |
| 加入服务端内容搜索、DLP 或摘要 | 与 E2EE 边界冲突，必须重新做产品和安全决策 |
| 加入音视频 | 独立项目评估，不吸收进 v1 |
| 加入跨 Region Active-Active | 独立架构阶段，不吸收进 v1 |
| 改为 SaaS 多租户产品 | 重做租户运营、计费、合规和发布模型 |

任何新增平台、Actor 类型、数据所有权、协议破坏性变更或权限边界扩大，也必须进入正式变更评审。

## 11. M0 产品决策记录

| ID | 决策 | 状态 | 评审证据 |
| --- | --- | --- | --- |
| D-001 | v1 只交付单 Region 私有化部署 | 已接受 | 2026-07-24 产品评审 |
| D-002 | Desktop 执行 Runtime；Mobile 只发起、观察、中断和审批 | 已接受 | 2026-07-24 产品评审 |
| D-003 | v1 从第一天交付 E2EE，并支持企业 KMS/HSM 可审计恢复 | 已接受，待密码 ADR | 2026-07-24 产品评审 |
| D-004 | v1 使用 OIDC + 管理员邀请/CSV，不交付 SAML / SCIM | 已接受 | 2026-07-24 产品评审 |
| D-005 | v1 不交付 Enterprise Runner；Desktop 离线时任务排队 | 已接受 | 2026-07-24 产品评审 |
| D-006 | 完整 Browser IM Client 不属于 v1 | 已接受 | 2026-07-24 产品评审 |
| D-007 | Desktop 使用 Tauri；iOS/Android 使用原生 UI；共享 Rust Core | 已接受，待 Client ADR | 2026-07-24 产品评审 |
| D-008 | Server 保存 Ciphertext 事实源；每台设备保存独立加密本地副本 | 已接受，待 Storage ADR | 2026-07-24 产品评审 |
| D-009 | Channel 共享语境；Workspace 按用户、设备独立绑定 | 已接受 | 2026-07-24 产品评审 |
| D-010 | 普通消息不调用模型；Task 和明确规则触发 Agent | 已接受 | 2026-07-24 产品评审 |
| D-011 | 一个 Run 只有一个 Execution Owner，可显式转交 | 已接受，待 Runtime ADR | 2026-07-24 产品评审 |
| D-012 | Prompt 由本地 Runtime 直达批准的模型 Endpoint | 已接受，待 Threat Model | 2026-07-24 产品评审 |
| D-013 | 能力驱动自动路由；Run 内固定模型；允许用户覆盖 | 已接受，待 Model ADR | 2026-07-24 产品评审 |
| D-014 | 首个垂直切片为 E2EE 消息到本地 Agent 审批与 Artifact | 已接受 | 2026-07-24 产品评审 |

## 12. 后续评审与排期

- Product Owner 已确认目标用户、核心流程和非目标，本文可作为 Frozen 产品范围基线。
- Client、Crypto、Storage、Runtime、Model 和 Recovery 的技术可行性必须分别通过 ADR 与 Threat Model；
  失败时按第 10 节重开范围评审，不得静默降级为服务端明文。
- `docs/delivery-plan.md` 中的 Managed Encryption、Tauri Mobile 和 2027-06-18 GA 基线已失效。
- 交付计划必须在独立 planning 任务中按原生 Mobile、E2EE、密码审计和新垂直切片重新估算；当前
  产品目标为 2027 年 10 月至 11 月 GA，最终日期以重估任务为准。
