# Threadline 安全风险台账

状态：Active / M0 Baseline

日期：2026-08-04

Issue：#21

威胁来源：`docs/security/threat-model.md`

## 1. 使用规则

本文件是安全风险状态的事实源。表中的 Owner 是负责任的 Workstream；在具体缓解 Issue 被领取前，必须再绑定一名可追责的个人 Assignee。Due 是受影响能力最迟必须满足关闭标准的 Gate，不是开始处理的日期。

状态只使用：

- `Open`：控制或证据未完成。
- `Mitigating`：已有被领取的缓解 Issue 和进行中的实现。
- `Review`：控制与验证证据齐全，等待 Security Reviewer。
- `Closed`：满足 Threat Model 中的 Critical/High 关闭标准，记录了证据和关闭信息。
- `Accepted`：仅允许 Medium/Low；必须记录接受人、理由、到期日和补偿控制。Critical 不可接受，High 不得以接受代替关闭。

每次 Gate、相关 ADR/Contract 变化、安全事件、依赖安全更新或至少每四周复审一次。风险分值为 `Likelihood × Impact`：Critical 20-25、High 12-19、Medium 6-11、Low 1-5。

## 2. 基线摘要

| 等级 | 数量 | 当前状态 |
| --- | ---: | --- |
| Critical | 14 | 14 Open |
| High | 17 | 17 Open |
| Medium | 0 | 0 |
| Low | 0 | 0 |

此摘要表示 M0 初始风险，不代表已启用的生产漏洞。受影响能力在对应 Due Gate 前保持候选、关闭或受限状态；只有验证证据完成后才能降低残余分值。

## 3. 风险项

### 3.1 身份、设备与密钥目录

| ID | 初始风险 | 必需控制 | 验证方法 | Owner | Due Gate / 日期 | 状态 | 残余目标 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| R-001 Ghost Device 插入 | 5×5=25 Critical | Device Authority；现有 Device/独立管理员批准链；Leaf 添加与追加式 Device/Key 日志；客户端一致性检查。 | 恶意 Control Plane 使用合法 OIDC、伪造审批和分叉目录尝试加 Leaf；多设备对同一日志根做一致性测试。 | Identity/Core + Client Crypto + Security | M2 / 2026-11-13 | Open | ≤8 Medium |
| R-002 OIDC/Device Credential 混淆 | 4×4=16 High | OIDC Session 与 Device Credential 不同 issuer/audience/key purpose；首设备高影响 Enrollment；短期 Credential 与撤销复检。 | Contract 负向测试：OIDC Token、过期 Credential、错误 Tenant/Device/Key 不能发布 KeyPackage 或加入 Group。 | Identity/Core | M1 / 2026-09-18 | Open | ≤6 Medium |
| R-003 KeyPackage 替换或复用 | 4×4=16 High | Credential/Profile/Device 绑定；短 TTL；默认一次性消费；目录原子消费和撤销。 | Golden Vector + 并发消费测试；跨 Tenant/Device/Profile、过期、撤销、重复 KeyPackage 全部拒绝。 | Client Crypto + Contracts | M0 / 2026-08-21 | Open | ≤6 Medium |
| R-004 Device Identity 被备份克隆 | 3×5=15 High | 私钥不可导出优先；备份排除 Identity/Leaf/KeyPackage 私钥；新硬件强制新 Enrollment；同 Device 恢复有平台证明。 | 五平台备份/迁移/设备镜像测试；恢复后旧 Credential 不能在第二硬件并行使用。 | Client Platform + Client Crypto | M1 / 2026-09-18 | Open | ≤8 Medium |

### 3.2 MLS、消息、历史与同步

| ID | 初始风险 | 必需控制 | 验证方法 | Owner | Due Gate / 日期 | 状态 | 残余目标 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| R-005 Commit 重放、乱序与 Group Fork | 4×5=20 Critical | Tenant/Group/Profile/Epoch 绑定；服务端有序 Membership Change；客户端 Transcript/Group State 验证、Gap Repair 与安全失败。 | 独立实现互操作 Harness 注入跨 Group、重复、损坏、乱序、并发 Commit 和分叉视图。 | Client Crypto + Sync + Security | M0 / 2026-08-21 | Open | ≤8 Medium |
| R-006 撤销后继续使用旧 Epoch | 4×5=20 Critical | Removal/Revocation 触发 `rekey_required`；successor Epoch 前阻止新应用消息；Credential/KeyPackage 同步撤销。 | 离线/被撤销 Device 尝试收发；Kill/Resume 和乱序同步下验证消息只留 Local Outbox。 | Client Crypto + Core/Sync | M0 / 2026-08-21 | Open | ≤6 Medium |
| R-007 History/Object 受众越权 | 4×5=20 Critical | 显式 History Sharing；当前 Membership/ACL/Retention 复检；端到端重新封装；窄 ACL 使用独立 Audience Envelope 或禁止该能力。 | 新 Member、新 Device、被撤销 Device、无旧 Device、跨对象 ACL 和恢复失败负向向量。 | Client Crypto + File/Artifact + Security | M0 / 2026-08-21 | Open | ≤8 Medium |
| R-008 被攻陷 Device 暴露保留历史 | 4×4=16 High | OS Secure Storage；加密本地 DB；最短 Retention；History Key/索引/缓存按期密码删除；企业可缩短本地保留。 | 五平台 Root/Jailbreak 假设演练；过期、撤权、离线和缓存/索引清除测试；验证诊断包无 Key。 | Client Crypto + Client Core | M2 / 2026-11-13 | Open | ≤10 Medium（已知限制） |
| R-009 Crypto Profile 降级与状态回滚 | 4×5=20 Critical | 固定 `tl-mls-1`；未知 Profile 安全失败；持久状态抗回滚；库版本与 Wire Version 分离；N-1/Golden Frame。 | 篡改 Profile/版本/未知字段/本地快照；降级 Cipher Suite 和旧库状态恢复测试。 | Client Crypto + Contracts | M0 / 2026-08-21 | Open | ≤6 Medium |
| R-010 Agent Attribution/Provenance 伪造 | 4×4=16 High | Device Identity 绑定 Agent/Task/Run/Grant/Ciphertext/Provenance；Server 复检关系；接收端验证。 | 修改任一 Attribution/AAD 字段、复用其他 Run/Grant、伪造 Artifact Hash；跨端 Contract Test。 | Agent Runtime + Client Crypto + Task Core | M2 / 2026-11-13 | Open | ≤6 Medium |
| R-011 跨 Tenant/Channel IDOR | 4×5=20 Critical | 每次按 Tenant、Device、当前 Membership/ACL、对象父级和 `acl_version` 复检；缓存不是授权事实源。 | 系统化跨 Tenant/Channel/Object/Cursor ID 交换、停用成员、ACL 并发更新和缓存污染测试。 | Core Authorization + Quality | M2 / 2026-11-13 | Open | ≤5 Low |
| R-012 假 Durable ACK 或事件篡改 | 4×4=16 High | Event、`channel_seq`、Transactional Outbox 同事务；同步提交后 ACK；幂等键和连续 Cursor。 | PostgreSQL Failover、进程 Kill、重复 100 次、Gap/乱序、NATS 停机恢复测试；ACK 后零丢失。 | Core Message/Sync + Data | M2 / 2026-11-13 | Open | ≤6 Medium |
| R-013 日志、诊断、备份或元数据泄露 | 4×4=16 High | 字段 Allowlist；内容/Secret Redaction；文件名等敏感元数据加密；诊断和备份独立访问/Retention。 | 扫描 Server、队列、Blob、日志、Trace、Crash Dump、诊断包、共享卷与备份，匹配合成 C3/C4 Canary。 | Observability + Client Platform + Security | M1 / 2026-09-18 | Open | ≤6 Medium |
| R-014 KeyPackage/Commit/热点 Channel DoS | 4×4=16 High | 配额、大小/频率限制、原子 KeyPackage 消费、Commit 冲突退避、Backpressure、Outbox 和 Gap Repair。 | 慢客户端、热点 Channel、冲突 Commit、KeyPackage 耗尽、NATS/Redis/Realtime 故障与恢复负载测试。 | Realtime/Sync + SRE | M2 基线 / 2026-11-13；M6 关闭 / 2027-07-23 | Open | ≤9 Medium |

### 3.3 企业恢复

| ID | 初始风险 | 必需控制 | 验证方法 | Owner | Due Gate / 日期 | 状态 | 残余目标 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| R-015 非恢复域调用 KMS/HSM | 5×5=25 Critical | 独立 Recovery Namespace/IAM/ServiceAccount/NetworkPolicy/DB Role；KMS Policy 仅允许 Recovery 身份和范围绑定操作。 | 使用 Core、Agent、Model Control、普通管理员、集群操作员和伪造 ServiceAccount 调用；全部被 Policy 拒绝并审计。 | Recovery Security + Platform | M5 / 2027-05-21 | Open | ≤5 Low |
| R-016 审批绕过、重放或抵赖 | 4×5=20 Critical | 至少两名不同审批者；请求者不能自批；Case/对象/原因/TTL/Policy Version 绑定；Nonce；不可变审计。 | 单人、自批、重复审批、过期 Case、替换对象/接收者/理由、并发批准和审计删除测试。 | Recovery Control + Audit/Compliance | M5 / 2027-05-21 | Open | ≤6 Medium |
| R-017 Recovery Envelope/输出越权 | 4×5=20 Critical | Envelope 绑定 Tenant/Group/Epoch/Profile/Key Version/对象范围；输出端到端加密给指定 Recipient Device；禁止通用解密 API。 | 跨 Tenant/Group/Epoch 替换、扩大时间/对象范围、错误 Recipient、搜索/Agent/模型调用和输出复用测试。 | Recovery Control + Client Crypto | M5 / 2027-05-21 | Open | ≤6 Medium |
| R-018 Recovery Key/Envelope 灾难恢复失配 | 3×5=15 High | Key Version 清单、HSM 备份/冗余、轮换仪式、恢复演练、Envelope/备份生命周期和破坏性销毁审批。 | 从 PITR/Blob Backup/HSM 灾备组合恢复；覆盖旧 Key Version、部分丢失、轮换中断、审计不可用和错误销毁。 | Recovery Security + SRE | M5 / 2027-05-21 | Open | ≤10 Medium |

### 3.4 Runtime、Connector 与模型出口

| ID | 初始风险 | 必需控制 | 验证方法 | Owner | Due Gate / 日期 | 状态 | 残余目标 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| R-019 Capability Confused Deputy | 4×5=20 Critical | Grant 绑定 Tenant/Actor/Device/Task/Run/Resource/Action/Expiry/Nonce/Policy；本机调用身份；每次使用复检。 | 跨 Actor/Task/Run/Device 重放、过期/撤销、资源替换、并发 Lease/Fencing 和缺字段 Contract Test。 | Capability/Core + Agent Runtime | M4 / 2027-03-19 | Open | ≤6 Medium |
| R-020 Runtime 读取 IM DB 或过量 Context | 4×5=20 Critical | Runtime 不 Mount SQLite；仅 Context API；有界 Ref/Top-K/Window；来源撤权和 Session 清理；Sandbox/Egress 控制。 | 文件系统/IPC 越权、恶意 Agent、Prompt Injection、完整 Transcript 请求、撤权竞态和 Run 结束残留扫描。 | Agent Runtime + Client Core + Security | M4 / 2027-03-19 | Open | ≤8 Medium |
| R-021 Connector 路径逃逸/TOCTOU | 4×4=16 High | 真实路径规范化；目录 Handle/平台安全 API；阻止 symlink/junction/mount/Unicode/大小写逃逸；执行时复检。 | 五平台路径 Fuzz；`..`、symlink race、junction、重命名、挂载切换、网络盘和任意执行参数注入。 | Connector + Client Platform | M4 / 2027-03-19 | Open | ≤8 Medium |
| R-022 高影响动作批准与执行不一致 | 4×4=16 High | Approval 绑定规范化目标、内容 Hash、动作、参数、Actor、Expiry；修改后重新批准；结果不可变审计。 | 批准后替换路径/内容/命令、过期/重复批准、部分执行、崩溃恢复和伪造结果测试。 | Approval/Task Core + Connector | M4 / 2027-03-19 | Open | ≤6 Medium |
| R-023 Model Endpoint/Fallback 越权 | 4×5=20 Critical | Route Grant 绑定 Endpoint/模型/参数/数据 Policy/Region/TTL；TLS/mTLS 身份；Fallback 只在预批准集合。 | DNS/TLS 冒充、跨 Endpoint 重放、区域/保留冲突、故障 Fallback、未知模型和过期 Grant 测试。 | Model Control + Agent Runtime + Security | M4 / 2027-03-19 | Open | ≤6 Medium |
| R-024 Prompt/Tool Output 非预期持久化 | 4×4=16 High | Model Control/Runtime Gateway Contract 禁止正文；字段 Allowlist；Run 结束清理；Tool Secret 分类；stdout/stderr 不直接上报。 | 用合成 Prompt/Secret Canary 扫描 API、Run Store、Event、日志、Trace、Crash、Artifact 和模型调用失败路径。 | Agent Runtime + Observability + Model Control | M4 / 2027-03-19 | Open | ≤6 Medium |

### 3.5 服务、运维与供应链

| ID | 初始风险 | 必需控制 | 验证方法 | Owner | Due Gate / 日期 | 状态 | 残余目标 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| R-025 Workload 身份横向移动 | 4×5=20 Critical | 每 Workload 独立 ServiceAccount/mTLS/DB Role/Secret/NetworkPolicy；Recovery/Model/Domain Schema 分权；默认拒绝。 | 从每个 Pod 身份枚举并尝试访问其他 DB、KMS、Blob、NATS、Admin 和 Recovery Endpoint；混沌环境复测。 | Platform/SRE + Security | M1 / 2026-09-18 | Open | ≤6 Medium |
| R-026 依赖、构建与更新供应链攻陷 | 4×5=20 Critical | 精确锁定、来源/完整性、SBOM、许可证/漏洞审计、可重复构建、最小 CI 权限、签名/证明、离线 Bundle 验证。 | 恶意依赖/feature/build script、缓存污染、未签名/回滚更新、镜像替换、Runner Secret 读取和二进制差异测试。 | Release/Integration + Security | M1 / 2026-09-18 | Open | ≤8 Medium |
| R-027 Proto/FFI/生成 SDK 语义漂移 | 3×4=12 High | Protobuf 事实源；Buf Lint/Breaking；未知字段保留；Golden Frame；FFI Contract；生成物由 Integration Owner 管理。 | Go/TS/Rust/Swift/Kotlin 对相同 Fixture 的序列化、错误、取消、N-1 和未知字段测试。 | Contracts/Integration + Client FFI | M1 / 2026-09-18 | Open | ≤6 Medium |
| R-028 Retention/撤权/删除回滚 | 4×4=16 High | 签名策略版本与单调时钟/状态；离线到期；Key/索引/缓存删除；Tombstone；备份生命周期；撤权复检。 | 时钟/策略/DB 快照回滚、永久离线、撤权竞态、重装/备份恢复、FTS 重建和 Blob Version 到期测试。 | Client Core + Worker/Compliance | M5 / 2027-05-21 | Open | ≤8 Medium |
| R-029 审计篡改、跨 Tenant 注入或内容泄露 | 4×4=16 High | 独立追加式存储；Tenant/Actor/对象/Policy/结果绑定；完整性 Checkpoint；最小字段；独立 RBAC/Retention。 | 删除/重排/覆盖/跨 Tenant 注入、重复事件、Collector 故障、正文/Token/Key Canary 和导出权限测试。 | Audit/Compliance + Security | M5 / 2027-05-21 | Open | ≤6 Medium |
| R-030 非关键子系统拖垮普通 IM | 4×4=16 High | Core 消息事实与 Runtime/Model/Recovery/NATS/Redis/Realtime 解耦；Local Outbox；PostgreSQL 事实源；明确安全降级。 | 逐一关闭 Model、Runtime、Recovery、NATS、Redis、Realtime；验证本地历史可用、消息不假 ACK、恢复后可补洞。 | Architecture + SRE + Client Core | M2 / 2026-11-13 | Open | ≤6 Medium |
| R-031 加密前 Client Scan Hook 泄露或 TOCTOU | 4×4=16 High | 仅允许企业批准的本地 Scanner Adapter；最小权限 Sandbox；加密临时目录；扫描结果绑定内容 Hash、Scanner/Policy Version；上传前复核 Hash；内容不进日志。 | 恶意/崩溃/超时 Scanner、扫描后替换文件、临时目录残留、诊断包 Canary、大文件与失败策略测试。 | File/Client Platform + Security | M3 / 2027-01-08 | Open | ≤8 Medium |

## 4. 关闭记录模板

风险进入 `Review` 或 `Closed` 时，在对应行后添加下面的记录；不得只把状态改单词：

```text
Evidence:
- Mitigation issue / commit / policy:
- Negative and adversarial tests:
- Platforms / services covered:
- Logging and telemetry review:
- Residual likelihood × impact:
- Security reviewer and review date:
- Reopen trigger:
```

## 5. 当前阻断结论

- M0 不能接受 R-001、R-005、R-006、R-007、R-009 等 Crypto/Device Critical 风险；P00-08 必须先产出互操作和负向证据。
- Enterprise Recovery 在 R-015、R-016、R-017 关闭前保持未启用，不能提供临时管理员解密工具。
- Agent/Connector/Model 能力在 R-019、R-020、R-023 关闭前不能进入企业 Beta；普通 IM 不得为这些能力降级安全边界。
- 文件能力在 R-031 关闭前不能宣称“扫描后 E2EE”；扫描失败不能把明文上传给服务端或无限阻塞普通文本消息。
- 任一日志、诊断包、共享卷、备份或遥测中发现 C3/C4 Canary，自动把 R-013 或 R-024 提升为 Release Blocker。
- M7 Gate 要求所有 Critical/High 风险满足关闭标准；Medium/Low 接受必须有未过期的书面记录。
