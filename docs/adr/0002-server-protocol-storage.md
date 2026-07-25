---
status: accepted
date: 2026-07-25
---

# Server、Protocol、Storage 与服务边界

Threadline v1 采用 Go 模块化 Core、Protobuf-first 的 Connect/gRPC API、WSS 实时传输、PostgreSQL 事实源，以及由 Transactional Outbox 驱动的 NATS JetStream 事件层。生产环境交付七个 Threadline 服务端工作负载；Recovery Control 必须作为独立的安全域部署，不能合入日常 Core。该选择优先保证消息 ACK 可恢复、客户端离线可用和企业恢复私钥不可被日常服务调用，同时避免把事务性领域过早拆成分布式系统。

## 决策范围

### 七个服务端工作负载

| 工作负载 | 责任 | 持久数据与禁止事项 | 故障影响 |
| --- | --- | --- | --- |
| `threadline-web` | 发布 Web IM、管理页面和静态资源 | 不拥有业务数据，不访问 PostgreSQL | Web 页面无法加载；Desktop、Mobile 和 API 不受影响 |
| `threadline-core` | Go 模块化单体；处理 Identity、Organization、Channel、Message、Sync、File、Task、Approval、Audit、Retention 与 Capability Grant | Domain Schema 与 Transactional Outbox 的唯一写入者；不能把模块边界退化为共享表写入 | 新命令和状态变更暂停；客户端保留 Local Outbox 和本地历史 |
| `threadline-realtime` | WSS 鉴权、连接目录、Presence、Typing、在线 Fan-out、Backpressure 与优雅摘流 | 无持久业务数据，不写 Message 或 Domain 表，不生成 Durable ACK | 客户端重连并按 Cursor 补洞；已提交事实不丢失 |
| `threadline-runtime-gateway` | Desktop Runtime Enrollment、出站 mTLS Stream、Heartbeat、Dispatch、Run Event、Lease/Fencing 代理 | 不访问 Domain 表，不保存 Workspace、消息明文或 Prompt；所有状态变化回到 Core | Agent Task 暂停派发或重连恢复；普通 IM 继续工作 |
| `threadline-worker` | Outbox Relay、Projection、通知、Retention、扫描 Hook、Retry 和 DLQ | 只更新 Outbox Delivery、Job 与自有 Projection；禁止修改 Domain Row | 异步投影和通知延迟；Core 仍可提交，恢复后重放 |
| `threadline-model-control` | 模型发现、能力清单、健康、评测、评分、路由策略与短期 Route Grant | 独占 Model Schema；不接收、代理、记录或存储用户 Prompt | 无法签发新 Route Grant；未过期 Grant 和 IM 不受影响 |
| `threadline-recovery-control` | 隔离执行企业恢复审批、阈值授权、KMS/HSM 解封装和恢复审计 | 独占 Recovery Schema 和恢复密钥调用权；禁止日常消息查询、模型路由和通用解密 API | 新恢复操作暂停；日常 IM、E2EE 投递和模型路由不受影响 |

Core 内部模块通过显式接口协调，并在确有原子性要求时共用一个 PostgreSQL 事务。只有独立容量、合规、发布节奏或故障域的证据出现后，才允许把模块拆成额外网络服务；拆分前先冻结 Protobuf Contract、数据所有权和迁移方案。

## 协议边界

- 第一方 Command、Query 和 Sync API 使用 Protobuf + Connect RPC over HTTPS，使 Desktop、Web 和 Mobile 共享版本化契约。
- 服务间调用使用 Protobuf + gRPC over HTTP/2 + mTLS，并统一 Deadline、取消、错误码和 Trace 传播。
- Client Realtime 使用 WSS + Protobuf Binary Envelope。WSS 负责低延迟双向传输、在线状态和同步提示，不承担消息持久性或历史恢复。
- Runtime Gateway 只接受 Desktop Runtime 主动建立的 mTLS Stream；企业工作站不开放入站端口。
- 附件和 Artifact 通过 HTTPS Resumable Multipart 传输加密块，不占用 WSS 数据通道。
- 协议字段只新增；移除字段必须 `reserved`。持久化 Envelope 需要 Golden Frame 和 Breaking Check。

Server、Realtime、Worker 和 Model Control 只处理 Ciphertext Envelope 与授权、路由、排序、幂等、长度和时间等必要元数据。它们不需要消息或文件正文，也不需要 Prompt。明文 Context 仅能在授权设备上由 Runtime 凭短期 Capability Grant 从本机 Context API 获取；模型 Endpoint 是另一个必须显式授权的数据接收方。

## 事实源与事务边界

PostgreSQL HA 是 Domain Event、状态、Channel Sequence、Transactional Outbox、Job 和 Audit 的事实源。S3-compatible Object Storage 是加密 Blob 的事实源，Vault/HSM/KMS 是密钥与服务凭据的事实源。NATS JetStream 和 Redis 都不是业务事实源。

```mermaid
sequenceDiagram
  participant C as Client/locald
  participant R as Realtime
  participant Core as Core
  participant PG as PostgreSQL HA
  participant W as Worker
  participant N as NATS JetStream
  participant D as Recipient Device

  C->>C: persist Local Outbox(event_id, pending)
  C->>R: WSS Send(ciphertext, idempotency_key)
  R->>Core: gRPC MessageCommand
  Core->>PG: BEGIN
  Core->>PG: authorize + allocate channel_seq
  Core->>PG: insert ciphertext event + outbox row
  Core->>PG: COMMIT synchronously to durability policy
  PG-->>Core: durable commit
  Core-->>R: Durable ACK(event_id, channel_seq)
  R-->>C: ACK; pending -> committed
  W->>PG: claim committed outbox row
  W->>N: publish(event_id)
  N-->>R: fan-out hint
  R-->>D: realtime notification
  D->>Core: Sync(after_cursor)
  Core-->>D: ordered ciphertext events + next_cursor
```

`event_id` 在 Tenant 内唯一，所有 Command 和 Consumer 都必须幂等。Core 只能在 Ciphertext Event、`channel_seq` 和 Outbox Row 同一事务完成持久提交后返回 Durable ACK；PostgreSQL 达不到约定的同步提交条件时不允许 ACK。Cursor 只在连续事件应用成功后推进，发现 Gap 时停止推进并向 Core 补洞。Edit、Redact、Reaction 等变化以新 Event 表达，不原地改写历史事件。

## 缓存、事件层与恢复语义

- Redis 只保存 Presence、Typing、连接目录、限流计数和可过期 Session Hint；数据丢失后通过重新鉴权、Heartbeat 或 TTL 自愈，不影响消息事实。
- JetStream 承担已提交事件的 Fan-out、投影、通知和 Task Dispatch。Worker 使用 Durable Consumer、显式 ACK、Backoff、DLQ/Parking Stream，并以业务 `event_id` 去重。
- NATS 不可用时，Core 继续向 PostgreSQL 提交，Outbox 保持未投递状态；NATS 恢复后 Worker 从 Outbox 重放。NATS 中的 Stream 可以从 PostgreSQL 事实重建。
- Realtime 重启或 WSS 断开时，客户端指数退避并加入 Jitter；连接恢复后以 PostgreSQL Cursor Sync 为准，而不是相信错过的在线通知。
- Worker 或投影故障只造成派生视图延迟，不能阻断 Core 的事务提交。超过重试上限的事件进入 DLQ，并保留可审计的人工重放路径。
- PostgreSQL 故障转移期间客户端继续写 Local Outbox；服务端暂停 Durable ACK。恢复后客户端按相同幂等键重试，禁止以假 ACK 换取表面可用性。

## Recovery Control 隔离

Recovery Control 与日常 Core、Model Control 和 Runtime Gateway 采用不同的网络、身份和密钥权限边界：

1. 部署到独立 Namespace 或等价隔离区，使用独立 ServiceAccount、mTLS 身份、NetworkPolicy、数据库 Role 和审计 Sink。
2. 仅审批入口和受控恢复出口可以访问 Recovery Control；Client、Realtime、Worker、Runtime Gateway、Model Control 与模型 Endpoint 都不能直接调用它。
3. 只有 Recovery Control 身份可请求 KMS/HSM 执行企业恢复私钥相关操作。Core 只能创建不可变的恢复申请、校验业务授权并接收不含私钥的状态结果。
4. 恢复必须满足组织策略定义的多方审批、短 TTL、用途绑定、Tenant/Channel/对象 Scope 和防重放要求；每次尝试都写入独立、不可变且不含正文的审计记录。
5. 恢复密钥材料不得进入 Pod 文件系统、环境变量、日志、Trace、诊断包或通用 Secret。KMS/HSM 返回值必须限定为完成当前恢复仪式所需的封装结果，不能暴露可复用私钥。
6. 不提供可供搜索、DLP、Agent 或模型调用的通用服务端解密接口。恢复输出只交付给审批中指定的受信接收方，并继续受 Retention 和审计策略约束。

## 单 Region 私有部署边界

- v1 只有一个写 Region；不实现跨 Region Active-Active、全局 Channel Sequencer 或跨 Region 一致事务。
- 无状态工作负载至少两个副本并跨可用区调度。PostgreSQL 在 Region 内 HA，Durable ACK 前满足同步提交策略，并配套 PITR 与恢复演练。
- JetStream 使用 Region 内三节点、三副本 Stream；Redis 使用 HA，但两者失效都不能改变事实源语义。
- PostgreSQL、NATS、Redis、对象存储、Vault/HSM/KMS 与 OTel Collector 只暴露在企业内网和私有 CA 信任域内，不向 Client Network 或公网开放。
- Standard Private 只允许经白名单代理的受控出站；Air-gapped 模式无公网出入站。APNs/FCM 是可关闭的同步提示，不属于可靠消息链路。
- 发布物包含离线 OCI Bundle、Helm Chart、SBOM、签名、数据库迁移、备份恢复与回滚说明；运行时不能依赖公共 Registry、CDN 或遥测 SaaS。

## 考虑过的替代方案

### 每个领域都拆成微服务

拒绝。Identity、Channel、Message、Task 和 Approval 需要一致的权限复检与事务边界；P0 拆分会引入分布式事务、更多运维面和协议漂移。只有负载或合规证据触发时再沿已定义模块接口拆分。

### 以 JetStream 作为消息事实源

拒绝。JetStream 适合作为 At-least-once 内部事件层，但不能把 Message、Task、Approval、Audit 的事务状态与 Channel Sequence 一起原子提交。PostgreSQL + Transactional Outbox 提供单一提交点和可审计恢复路径。

### 依赖 WSS 投递保证消息可靠

拒绝。连接可中断，Gateway 可重启，Mobile 也可能被挂起。可靠性必须来自 Local Outbox、幂等 Command、Durable ACK、Cursor Sync 和 Gap Repair；WSS 只优化时延。

### 将 Recovery Control 合入 Core 或直接委托 KMS

拒绝。Core 的日常网络暴露面和调用频率远高于恢复路径，合并会让 Core 身份获得企业恢复私钥的调用能力，也无法证明 Model、Runtime 和常规管理员不能借道解密。独立工作负载和身份是可审计最小权限边界。

### 使用 REST/JSON 作为第一方主协议

拒绝。并行维护 Desktop、Web、iOS、Android、Rust Core 和 Go 服务需要单一、可做 Breaking Check 与 Golden Frame 的契约。REST/JSON 仅保留在 OIDC、Webhook 等外部标准边界。

## 回退与重新评审条件

- 如果单个 Core 模块持续违反容量、发布节奏、数据驻留或故障隔离 SLO，则先冻结 Protobuf Contract 和 Schema Ownership，再把该模块拆成服务；不允许共享数据库写入作为过渡常态。
- 如果企业网络设备无法稳定支持 WSS，则可增加 Long Polling 或未来的 WebTransport 适配器，但 Local Outbox、ACK、Cursor 和 Event 语义保持不变。
- 如果 JetStream 在目标私有环境无法达到运维或恢复要求，可替换内部事件实现；替代品必须支持 Durable Consumer、显式 ACK、重放和 DLQ，并继续由 PostgreSQL Outbox 驱动。
- 如果 PostgreSQL 无法满足 Channel 热点容量，先按 `hash(tenant_id, channel_id)` 分区 Sequencer；只有负载证据证明单 Region PostgreSQL 架构不足时才重开存储 ADR。
- 如果 KMS/HSM 产品无法提供用途绑定、审计和不可导出密钥操作，则企业恢复功能保持关闭，不能降级为把恢复私钥交给 Core、管理员 Pod 或应用配置。
- 任何跨 Region Active-Active、服务端内容搜索/DLP、共享模型 Prompt 代理或 Enterprise Runner 需求，都会改变事实源或信任边界，必须重开 Scope、Threat Model 和本 ADR。

## 后果

- IM 在模型、Runtime、NATS、Redis 或 Realtime 离线时仍可恢复并继续使用；确认过的消息以 PostgreSQL 提交为准。
- 服务数量固定为七个 Threadline 服务端工作负载，其中 Recovery Control 是安全隔离而非容量拆分。现有把 P0 描述为六个工作负载的架构草案需要由其 Owner 后续同步。
- 系统接受 At-least-once 传输带来的重复，并将幂等性作为 Command、Outbox Relay 和 Consumer 的强制契约。
- 单 Region 简化了排序和事务，但 Region 级灾难恢复依赖备份/PITR，不能声称零 RPO 或自动跨 Region 写入接管。
