# Threadline Private Enterprise v1.0 Scope Freeze

状态：M0 评审候选稿 1.0

Gate：G0 / M0

Owner：Product / Architecture

Issue：[#15 T001](https://github.com/monkeylabx/threadline/issues/15)

工程基线：`216287d`

## 1. 冻结目的

本文冻结 Threadline Private Enterprise v1.0 的目标用户、产品承诺、平台范围、核心流程和明确
非目标。后续 PRD、ADR、Threat Model、Proto、客户端和部署任务必须引用本基线；超出本基线的需求
必须经过变更评审和重新排期。

## 2. 产品承诺

Threadline 是一套面向企业内网的 Agent 原生 IM。团队先获得完整、可靠的企业沟通能力，再将一段
对话转换为有权限、可观察、可审批、可接管的 Agent Task。模型或 Runtime 离线时，IM 仍能独立工作。

v1 的差异化承诺不是“聊天中附带一个机器人”，而是：

- Human、Agent 和 Service 都是具有身份、成员关系、职责和审计记录的 Actor。
- Channel 保存长期协作历史；Task 和 Run 保存一次具体执行的上下文、活动与产物。
- Runtime 只通过短期 Capability Grant 获取明确选择的消息、文件和 Workspace。
- 本地文件只由用户授权设备上的 Local Connector 访问，服务端不能浏览成员文件系统。
- 高影响动作必须在执行前展示目标、范围、期限和影响，并留下审批与审计证据。

## 3. 目标用户

### 3.1 主要使用者

| 用户 | 必须解决的问题 | v1 成功结果 |
| --- | --- | --- |
| 普通员工 | 沟通、查找信息并把工作交给 Agent | 不离开 IM 即可创建并跟踪 Task |
| 团队负责人 | 协调 Human 与 Agent，识别风险和阻塞 | 可以分配、观察、中断、接管和验收工作 |
| 操作审批人 | 判断 Runtime 请求是否安全且必要 | 批准前能看清 Capability、资源和影响范围 |
| Agent 负责人 | 管理 Agent 职责、参与模式、预算和路由 | Agent 行为有 Owner、Policy 和审计依据 |

### 3.2 部署与治理者

| 用户 | 必须解决的问题 | v1 成功结果 |
| --- | --- | --- |
| IT 管理员 | 在内网安装、升级、备份并接入企业基础设施 | 可离线部署并完成升级、回滚和恢复演练 |
| 安全管理员 | 管理设备、权限、保留策略和审计证据 | 可撤权、追溯高风险动作且日志不泄露正文 |
| 访客或外包 | 只访问明确共享的协作空间 | 权限不能越过 Organization、Channel 和资源 ACL |

## 4. v1 核心流程

1. **纯人类沟通**：Runtime 和模型全部离线时，成员仍可使用 DM、Channel、Thread、文件、搜索、
   通知和多端同步。
2. **从讨论到执行**：成员选择消息创建 Task，绑定上下文和 Workspace，分配 Agent 并启动 Run。
3. **本地受控执行**：授权 Desktop Runtime 在隔离 Worktree 或 Sandbox 中工作；Local Connector 只
   暴露明确路径和有效时段。
4. **团队观察与审批**：成员在 IM 中查看结构化活动、阻塞、审批、Diff 和 Artifact，并可中断、重试、
   接管或拒绝结果。
5. **移动协作**：Mobile 可以沟通、发起和观察 Task、中断 Run、处理审批，但不执行长任务或访问任意
   本地文件系统。
6. **企业治理**：管理员通过 OIDC、设备清单、RBAC、ACL、Retention、Audit 和 Runtime 撤销控制风险。
7. **私有化运维**：运维人员在单 Region 企业环境完成安装、监控、备份恢复、升级回滚和离线交付。

T002 负责把以上流程拆成可执行的端到端验收步骤；本文只冻结场景边界。

## 5. 平台范围

| 平台 | v1 范围 | 明确限制 |
| --- | --- | --- |
| Desktop | macOS、Windows、Linux；完整 IM、本地缓存、Agent Runtime、Local Connector | Runtime 只主动连接内网，不开放工作站入站端口 |
| Mobile | iOS、Android；完整消息、Task 发起/观察/中断和审批 | 不运行长任务，不暴露任意文件系统，不承诺 Air-gap 后台实时唤醒 |
| Admin Web | 私有化管理、治理和运行状态页面 | 不承诺完整 Browser IM Client |
| Server | 单 Region 私有化部署、Helm、离线 Bundle、备份恢复、升级回滚 | 不提供 SaaS 多租户运营或跨 Region Active-Active |

Tauri 2 是当前客户端候选。M0 Mobile Gate 必须验证输入法、10,000 条消息列表、Keychain/Keystore、
文件选择、Push 唤醒和后台恢复；Gate 失败时 Mobile 切换 React Native 并重新排期 8 至 12 周。

## 6. v1 功能范围

### 6.1 企业 IM

- Organization、Member、Role、Guest、Space、DM、Channel、Thread 和 Membership。
- 文本、代码、Mention、Reply、Reaction、Pin、附件、编辑、撤回和 Read Cursor。
- Durable Local Outbox、ACK、幂等、Channel 内排序、重连、Cursor Sync 和 Gap Repair。
- 文件分片上传、断点续传、加密 Blob、预览、权限复检和病毒扫描 Hook。
- 消息、文件和 Task 搜索；本地索引是可重建缓存，不是权限事实源。
- Inbox、通知、Presence、Typing 和基础组织管理。

### 6.2 Agent 协作

- Agent Actor、Owner、Channel Membership、`manual` 和 `task_only` 参与模式。
- 从消息创建 Task，绑定 Context Reference、Workspace、参与者和预期 Artifact。
- Run 创建、结构化 Event、取消、失败恢复、审批、Artifact、Usage 和 Provenance。
- 短期 Capability Grant、运行中撤权、Workspace Lease、Fencing 和隔离 Worktree。
- 内部模型 Endpoint 的发现、能力登记、健康、评测和策略路由；Workflow 不硬编码模型名称。

### 6.3 安全与私有化

- 企业 OIDC、设备注册与撤销、RBAC、资源 ACL、Retention Metadata 和 Audit。
- TLS / mTLS、Managed Encryption、设备加密 SQLite、加密 Blob Cache 和企业 KMS。
- PostgreSQL HA、NATS JetStream、Redis、S3-compatible Object Storage、Vault/HSM 和 OpenTelemetry。
- 单 Region Helm 部署、签名 OCI 制品、SBOM、Secret Scan、备份恢复、升级回滚和 Air-gap Bundle。
- Standard Private 可经白名单 Proxy 使用 APNs / FCM；Air-gapped 模式不要求任何公网连接。

## 7. 明确非目标

以下能力不属于 Private Enterprise v1.0，不得作为普通需求并入当前排期：

- SaaS 多租户运营、计费、套餐和公网控制台。
- SAML、SCIM、完整 LDAP / AD 目录同步和 Legal Hold 完整工作流。
- 严格 MLS E2EE、设备恢复、Key Escrow 和新成员 History Sharing。
- Enterprise Runner、Remote Runtime 和服务端任意 Workspace 执行。
- 完整 Browser IM Client。
- 公有模型托管、基础模型部署和企业推理平台建设。
- 音视频会议、直播、白板和多人实时文档。
- 跨 Region Active-Active 和全球 Channel Sequencer。
- 完整服务端明文内容搜索、DLP 和外部语义索引。
- 区块链、P2P 消息共识或在设备间复制 SQLite 文件。

## 8. 外部依赖边界

- Threadline 接入企业已有 OIDC、对象存储、KMS、内部模型 Endpoint 和可观测系统，但不负责部署
  或运营这些基础设施。
- Standard Private 唯一默认允许讨论的公网出站是 APNs / FCM；其他出站必须由企业显式批准。
- Air-gapped 安装和运行不能依赖公共 CDN、Package Registry、遥测 SaaS 或在线 License Service。
- Runtime 使用项目自有 `agent-core`；Hermes Agent 仅作为研究参考，不是生产依赖。

## 9. 变更触发条件

出现以下任一情况时，Product Owner 和 Tech Lead 必须暂停相关实现、更新本文件并重新计算里程碑：

| 变更 | 必须采取的动作 | 当前计划影响 |
| --- | --- | ---: |
| Tauri Mobile Gate 失败 | 记录 Gate 证据并决策 React Native 回退 | 增加 8-12 周 |
| 加入 MLS E2EE | 新建安全阶段、协议评审和恢复策略 | 增加 12-18 周 |
| 加入 SAML + SCIM | 增加目录生命周期与兼容测试 | 增加 5-8 周 |
| 加入 Enterprise Runner | 增加隔离、Fleet、凭据和运维范围 | 增加 6-10 周 |
| 加入完整服务端搜索或 DLP | 增加索引、权限和数据治理范围 | 增加 6-10 周 |
| 加入音视频 | 独立项目评估，不吸收进 v1 | 至少增加 20-30 周 |
| 加入跨 Region Active-Active | 独立架构阶段，不吸收进 v1 | 至少增加 16-24 周 |
| 改为 SaaS 多租户产品 | 重做租户运营、计费、合规和发布模型 | 必须重新立项估算 |

任何新增平台、Actor 类型、数据所有权、协议破坏性变更或权限边界扩大，也必须进入正式变更评审。

## 10. M0 评审决策记录

| ID | 决策 | 状态 | Owner | 评审证据 |
| --- | --- | --- | --- | --- |
| D-001 | v1 只交付单 Region 私有化部署 | 待签字 | Product | 本文第 5、7 节 |
| D-002 | Desktop 执行 Runtime；Mobile 只发起、观察、中断和审批 | 待签字 | Product / Client | 本文第 4、5 节 |
| D-003 | v1 使用 Managed Encryption，不交付严格 MLS E2EE | 待签字 | Security | 本文第 6、7 节 |
| D-004 | v1 使用 OIDC，不交付 SAML / SCIM | 待签字 | Identity | 本文第 6、7 节 |
| D-005 | v1 不交付 Enterprise Runner | 待签字 | Runtime | 本文第 5、7 节 |
| D-006 | 完整 Browser IM Client 不属于 v1 | 待签字 | Product / Client | 本文第 5、7 节 |

### 签字条件

- Product Owner 确认目标用户、核心流程和非目标。
- Tech Lead 确认范围与 `docs/delivery-plan.md` 一致。
- Security Owner 确认加密、Runtime 和本地文件边界可进入 Threat Model。
- Mobile Owner 接受 Tauri Gate 和 React Native 回退条件。
- 所有评审异议记录为明确的变更请求，不以口头约定覆盖本文件。

签字完成后，将本文状态改为“Frozen”，填写评审日期和批准人，并把 Issue #15 的三个交付项勾选。
