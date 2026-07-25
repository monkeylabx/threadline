# Threadline 数据分类与处理规则

状态：M0 安全基线草案

适用范围：Private Enterprise v1.0

Issue：#20

## 1. 目的与强制边界

本文定义 Threadline 数据的 Owner、允许出现的位置、加密和保留规则。它是后续 Threat Model、
Schema、日志规范和删除流程的输入，不替代企业自身的法务保留策略。

以下约束不可由实现便利性改变：

- IM Server 只持久化 `Ciphertext Envelope`、加密 Blob 与完成投递和授权所需的 Metadata；不建立消息或
  文件正文的可解密服务端索引。
- Prompt 由授权设备上的本地 Runtime 组装，并直达企业策略批准的模型 Endpoint；IM Server、Runtime
  Gateway 和 Model Control 不接收、代理或记录 Prompt。
- 企业恢复私钥只存在于隔离的 KMS/HSM 权限域。IM Server、Recovery Control 应用进程、本地 Runtime、
  Connector、Model Control 和模型 Endpoint 都不能取得或导出该私钥。
- Agent Session/Run 不是 IM 数据库或 Channel 历史副本；共享卷、Workspace 和 Artifact 目录也不是
  客户端明文消息存储。

## 2. 分类等级

| 等级 | 名称 | 示例 | 默认处理 |
| --- | --- | --- | --- |
| C0 | 公开 | 产品版本、公开文档 | 可公开；仍需完整性保护 |
| C1 | 企业内部 | 服务健康、无内容的聚合指标 | 仅企业网络和授权运维人员；TLS 与静态加密 |
| C2 | 受限元数据 | Actor/Channel ID、时间、序号、ACL、审计元数据 | 最小化收集；Tenant 隔离；按业务或合规期限保留 |
| C3 | 机密内容 | 消息/文件明文、本地索引、Prompt、Run 输入输出、Artifact 明文 | 仅在获权设备或明确批准的接收方出现；禁止日志和普通遥测 |
| C4 | 密钥与凭据 | 设备私钥、Channel/Epoch Key、恢复私钥、短期 Capability/Route Grant | 专用安全存储、最短生命周期、不可导出或最小可见；禁止日志、导出和诊断包 |

密文不会因“不可读”而变成 C0/C1。Ciphertext Envelope 和加密 Blob 至少按 C2 管理，因为大小、参与者、
时间和访问模式仍可泄露敏感关系；失去密钥保护时按其明文最高等级处置。

## 3. 数据清单

表中的“保留”是产品默认约束；企业配置可缩短期限。Legal Hold 或强制合规保留只能通过显式、可审计
策略覆盖，且不能扩大解密主体。

| 数据 | 等级 | 事实 Owner | 允许存储位置 | 传输/静态保护 | 保留与删除 |
| --- | --- | --- | --- | --- | --- |
| 消息明文 | C3 | Channel 成员及企业策略 | 授权设备进程内；本地整库加密 SQLite 的受控物化视图 | TLS；Channel E2EE；本地 DB 加密 | 遵循消息 Retention；撤权后删除 Key、索引和可解密缓存；不得进入 Server、日志、Run 长期 Session 或共享卷 |
| 消息 Ciphertext Envelope | C2（密钥泄露时 C3） | IM Control Plane | PostgreSQL 事件存储、本地加密 SQLite、可靠投递队列 | TLS/mTLS；签名；数据库/备份静态加密 | 按 Channel Retention/Legal Hold；删除必须覆盖事实存储、投影、备份到期和本地缓存 |
| 消息必要 Metadata | C2 | IM Control Plane | PostgreSQL；有限投影；本地 SQLite | TLS/mTLS；Tenant/ACL 隔离；静态加密 | 只保存排序、路由、授权、幂等所需字段；正文、Snippet、可搜索 Token 不属于必要 Metadata |
| 文件明文及提取文本 | C3 | 上传者/Channel 授权域 | 获权设备内存、受控临时目录、加密本地缓存 | 上传前客户端加密；临时文件使用 OS 权限和加密卷 | 临时提取物任务结束即删；缓存按配额和 Retention 清除；撤权后不可再解密 |
| 加密 Attachment Blob | C2（密钥泄露时 C3） | IM File 域 | S3-compatible Object Storage、本地加密缓存 | TLS；客户端内容加密；独立 Blob Key/Checksum | 按关联消息、文件策略和 Tombstone 清理；对象版本与备份按期到期 |
| 文件 Metadata | C2；文件名/Tag 可达 C3 | IM File 域 | PostgreSQL；必要字段可进入本地 SQLite | 敏感字段加密；ACL 与对象访问复检 | 与 Blob/关联资源共同保留和删除；不得通过可读对象 Key 泄露文件名 |
| 本地 FTS/文件/向量索引 | C3 派生数据 | `locald` | 每设备独立的整库加密 SQLite；禁止服务端明文索引 | DB Key 在 OS Secure Storage；每次查询复检 ACL | 可删除、可重建、非事实源；撤权立即删相关 Entry；策略可禁用持久化索引 |
| 设备凭据与私钥 | C4 | 设备身份域/设备用户 | OS Keychain/Keystore/Secure Storage；服务端仅存公钥、状态与不可逆标识 | 不可导出优先；轮换；使用时最小内存驻留 | 设备撤销、注销或重置时失效并清除；不进配置、DB、备份、日志或诊断包 |
| E2EE Group/Epoch 状态与 Key Package | C4（公开包为 C2） | Client Crypto 域 | OS Secure Storage + 加密本地状态；服务端 Key Directory 仅保存公开包/密文封装 | 经审查协议；版本化；签名；Epoch 轮换 | 随成员/设备撤销轮换；旧 Epoch 按 History Policy 最小保留；不得被 Runtime 或 Server 直接读取 |
| Recovery Envelope | C4 密文材料 | Client Crypto/Recovery 域 | Ciphertext Event/Blob 中的版本化封装；审批记录在 Recovery 域 | 仅恢复接收者可解封；完整性和版本绑定 | 与被保护对象/合规策略一致；删除对象时同步删除封装，Legal Hold 除外 |
| 企业恢复私钥 | C4 最高敏感 | 企业 KMS/HSM 权限域 | 仅 KMS/HSM；禁止应用数据库、配置、容器 Secret、客户端和备份导出 | 不可导出；多人审批；硬件内解封；独立网络/IAM；全量审计 | 按企业密钥生命周期轮换/销毁；销毁需审批并记录影响；任何应用服务不得读取私钥字节 |
| Capability Grant / Lease / Fencing Token | C4 | IM Control Plane | 服务端授权状态；调用方短期内存；必要时设备安全存储 | 签名、Scope、Actor/Tenant/Task/Resource、Expiry、Nonce；mTLS | 短期有效；撤权立即影响后续调用；过期后清除；日志只留哈希/ID 和决策结果 |
| Model Route Grant/短期模型凭据 | C4 | Model Control/企业 Secret 域 | Model Control 授权状态；本地 Model Adapter 短期内存 | Endpoint/模型/参数/数据策略绑定；短 TTL；TLS/mTLS | 调用结束或过期即清除；禁止进入 Run Event、Artifact、日志或诊断包 |
| Prompt 与模型响应明文 | C3 | Task 发起者及批准的模型数据域 | 本地 Runtime 内存；批准的模型 Endpoint 按其企业策略处理 | Runtime 直连 Endpoint；显式 Egress Policy；TLS/mTLS | 默认不在 Threadline 服务端或 Runtime Session 持久化；Endpoint 保留必须由企业策略明确并向用户可见 |
| Task / Context Manifest | C2；引用语义可达 C3 | IM Control Plane | PostgreSQL、本地加密 SQLite | 只含引用、策略版本和授权范围，不内嵌消息/文件明文 | 随 Task/审计策略保留；来源撤权后引用不可解析为明文 |
| Run 状态、Event、Session、工具输出 | C2-C3 | 本地 Runtime；状态摘要由 IM Task 域持有 | 独立 Run 目录/Runtime Store；Server 只收最小状态与经批准结果 | 设备静态加密、OS ACL、单 Writer；上传前分类和过滤 | 按 Run Policy 清理临时 Context/Session；不得无限保留来源明文；不把 Channel Transcript 复制为 Session |
| Artifact 明文与加密 Blob | 明文 C3、密文 C2 | Task/Artifact 域 | 明文仅本地暂存；服务端对象存储仅加密 Blob，PostgreSQL 保存 Provenance/ACL | 上传前加密；Hash/签名；关联 Run/Step/Actor | 按 Artifact 与来源策略中更严格者；删除 Blob、Key、预览和派生缓存；保留不可反推出内容的必要审计 |
| 审计事件 | C2；若误含内容则 C3 事故 | Audit 域 | 独立不可变审计存储/受控导出 | mTLS、追加写、完整性保护、独立 RBAC | 独立合规期限；禁止记录消息/文件/Prompt 明文、Token、Key；审批、访问对象 ID、结果和策略版本可保留 |

## 4. 派生数据、日志与导出

### 4.1 本地索引

- 索引只存在于单个授权设备的加密数据库，是可删除重建的 Derived Cache，不是消息事实源。
- Index Entry 带资源 ID、Channel/Epoch 与 `acl_version`；查询返回前按当前权限复检。
- 撤销成员或设备权限时，先让 Key 和索引不可用，再异步清理密文缓存。
- Semantic Embedding 视同 C3 派生内容。发送给外部 Embedding API 等同披露消息正文，必须走模型数据策略。

### 4.2 日志、Metric 与 Trace

- 允许：不透明 ID、事件类型、大小桶、延迟、错误码、策略版本、授权 Allow/Deny、模型/Endpoint 标识和
  Token/Grant 的不可逆指纹。
- 禁止：消息和文件正文、文件路径/文件名（除非脱敏）、Prompt/Response、搜索词、Context Snippet、
  Authorization Header、Cookie、Token、Key、Recovery Envelope 字节和完整 Artifact 内容。
- 错误对象、Span Attribute 和 Panic/Crash Dump 使用同一规则；Debug 模式不能降低分类等级。

### 4.3 诊断包

- 默认不生成、不上传遥测；管理员在本地显式创建、预览并导出脱敏诊断包。
- 只包含版本、配置 Schema（不含值中的 Secret）、健康状态、聚合 Metric、脱敏日志和用户明确选择的
  最小复现材料。
- 打包前运行 Secret/Content Scanner；发现 C3/C4 时默认拒绝生成，而不是静默上传。
- 诊断包自身按 C2 管理，加密保存、设置短期到期，并记录创建者、用途、接收者、导出时间和删除结果。

### 4.4 导出与删除

- 数据导出是高影响动作：必须鉴权、授权复检、明确范围、可见审批、加密交付和不可变审计。
- 普通管理员导出不能调用企业恢复私钥；恢复导出必须走隔离 Recovery Control 的多人审批仪式。
- 删除必须传播到事实源、投影、索引、预览、临时文件、对象版本和设备缓存；备份通过加密擦除或到期
  生命周期完成，不能承诺即时改写所有离线备份。
- Legal Hold 只暂停获批范围的删除，不授予新的读取/解密权限；解除后恢复正常删除队列。

## 5. 实现检查清单

- 新 Schema/API/Event 的设计必须标注数据等级、Owner、Retention 和日志字段。
- 新服务若需要 C3 明文或 C4 材料，必须更新 Threat Model 并获得安全评审，不能通过共享数据库绕过。
- 测试 Fixture 使用合成数据；生产样本进入测试或工单前必须脱敏并得到显式批准。
- 任何共享卷不得存放客户端明文消息数据库、Channel Key、Prompt Cache 或可长期读取的 Context Bundle。
