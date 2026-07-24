# Threadline Private Enterprise v1.0 交付计划

状态：排期基线草案 0.1

计划起点：2026-07-27

基准 GA：2027-06-18

估算口径：12 名技术成员，2320 人日，41 周实施 + 6 周风险储备

## 1. 交付目标

交付一套可在企业内网安装、升级和运维的 Agent 原生企业 IM：

- Desktop：macOS、Windows、Linux。
- Mobile：iOS、Android。
- 企业成员可以私聊、频道沟通、Thread、搜索和传递文件。
- 消息支持离线发送、断线重连、幂等、频道内排序、Cursor 补洞和多端同步。
- Desktop 运行本地 Agent Runtime；Mobile 可以发起、观察、中断和审批任务，但不执行长任务。
- Agent 只能通过短期 Capability Grant 读取明确选择的消息和工作区。
- 支持企业 OIDC、设备管理、审计、保留策略、模型发现与路由。
- 提供单 Region 私有化 Helm 部署、离线安装包、备份恢复、升级回滚和运维手册。

### 1.1 本版本不包含

- SaaS 多租户运营、计费、套餐和公网控制台。
- SAML、SCIM、Legal Hold 完整工作流。
- 音视频会议、直播、白板和多人实时文档。
- 跨 Region Active-Active 和全球 Channel Sequencer。
- 公有模型托管或企业模型部署。
- 严格 MLS E2EE。v1 使用 Managed Encryption、设备加密数据库和企业 KMS；MLS 需要独立安全阶段。
- Enterprise Runner。v1 Agent Runtime 运行在授权 Desktop/Workstation。

## 2. 技术架构定案

### 2.1 客户端

| 层 | 选择 | 说明 |
| --- | --- | --- |
| 应用壳 | Tauri 2 | 同一工程构建 macOS、Windows、Linux、iOS、Android；系统能力通过 Rust、Swift、Kotlin 插件接入 |
| UI | React + TypeScript + Vite | 与现有 HTML 原型一致；Desktop 与 Mobile 使用独立页面结构，共享领域组件和设计 Token，不做单纯缩放 |
| UI 状态 | TanStack Query + Zustand | RPC/缓存状态与短期 UI 状态分开；消息事实仍在 locald |
| Client Core | Rust | 加密 SQLite、Outbox、Cursor、同步归并、设备密钥和本地搜索 |
| Desktop 本地服务 | `locald`、`agentd`、`connectord` | UI 不直接写 SQLite，不直接访问任意文件系统 |
| Mobile 本地能力 | Client Core 进程内 Actor | 使用同一 Rust Core；没有 `agentd` 和 `connectord` |
| 列表与编辑器 | 虚拟化列表 + 受控消息 Composer | Desktop/Mobile 分别适配键盘、手势、输入法和无障碍 |

必须在第 2 周完成 Tauri Mobile 风险门：验证 iOS/Android 输入法、10,000 条消息虚拟列表、
Keychain/Keystore、文件选择、推送唤醒和后台恢复。任一关键项不通过，Mobile 切换为 React Native，
计划增加 8 至 12 周且 UI 共享范围缩小为协议、领域模型和设计 Token。

### 2.2 服务端

| 层 | 选择 | 说明 |
| --- | --- | --- |
| 语言 | Go | 复用现有 `agent-core` 能力与团队执行契约；适合网络服务和单二进制私有化交付 |
| RPC | Protobuf + ConnectRPC | Browser、Desktop、Mobile 走 Connect；服务间和 Runtime Stream 走 gRPC/mTLS |
| Realtime | WSS + Protobuf Binary Frame | WSS 只负责连接与在线提示；可靠性由 Outbox、ACK、Cursor Sync 保证 |
| 数据访问 | pgx + sqlc | 显式 SQL、事务和 Schema Ownership，不使用隐式 ORM |
| 事实源 | PostgreSQL HA | Domain Event、状态表、Transactional Outbox、Job 和 Audit |
| 事件层 | NATS JetStream | Fan-out、投影、通知和任务派发；不是消息事实源 |
| 短期状态 | Redis | Presence、Typing、连接目录、限流；允许丢失重建 |
| Blob | S3-compatible / MinIO | 加密附件、Artifact、诊断包 |
| 密钥 | Vault/HSM/KMS | 服务凭据、设备 Grant、模型短期授权和数据密钥包装 |
| 可观测 | OpenTelemetry | 内网 Collector、Metric、Trace、脱敏 Log；禁止记录消息正文和 Prompt |

### 2.3 服务工作负载

生产保持 6 个服务端工作负载：

1. `threadline-web`：静态 Web 与管理页面。
2. `threadline-core`：Identity、Organization、Channel、Message、Sync、File、Task、Approval、Audit。
3. `threadline-realtime`：Client WSS、Presence、Typing、Fan-out、Backpressure。
4. `threadline-runtime-gateway`：Desktop Runtime Enrollment、Heartbeat、Dispatch、Run Event。
5. `threadline-worker`：Outbox Relay、Projection、Notification、Retention、Scan、DLQ。
6. `threadline-model-control`：模型发现、能力、健康、评测、评分和路由授权。

Core 是模块化单体。P0 不继续拆 Identity、Message、Task 等内部模块，避免分布式事务和过早运维成本。

### 2.4 协议与数据规则

- `.proto` 是跨端契约事实源，使用 Buf CLI 做 Lint、Generate 和 Breaking Check，不依赖公网 BSR。
- 公开字段只能新增；删除字段必须 `reserved`；持久化 Envelope 需要 Golden Frame 测试。
- Message Command、`channel_seq`、Event 和 Transactional Outbox 在同一 PostgreSQL 事务提交。
- 客户端先写 Durable Local Outbox，收到 Durable ACK 后才从 Pending 转为 Committed。
- 所有 Consumer 都按 At-least-once 和业务幂等设计。
- Desktop/Mobile 各自维护本地数据库，不复制或共享 SQLite 文件。
- Agent Runtime 只通过 Context API 获取有限明文，不 Mount IM 数据库，不读取整个 Channel。

## 3. 建议仓库结构

```text
threadline/
  apps/
    client/                  # React UI + Tauri desktop/mobile shell
    admin-web/               # 私有化管理页面
  crates/
    client-core/             # Rust sync/crypto/sqlite/search core
    locald/                  # Desktop single-writer daemon
    connectord/              # Workspace capability and sandbox
    tauri-plugins/           # Keychain, push, share, updater adapters
  services/
    core/
    realtime/
    runtime-gateway/
    worker/
    model-control/
    agentd/                  # Go; wraps agent-core
  packages/
    ui/
    design-tokens/
    client-domain/
    generated-ts/
  proto/
  db/
    migrations/
    queries/
  deploy/
    compose/
    helm/
    offline-bundle/
  test/
    contract/
    e2e/
    load/
    chaos/
    fixtures/
  docs/
```

Monorepo 使用 `pnpm workspace + Cargo workspace + Go workspace + just/Makefile`。不在第一阶段引入
Bazel；当增量构建和跨语言依赖真的成为瓶颈后再评估。

## 4. 估算假设

### 4.1 标准团队

| 角色 | 人数 | 主要责任 |
| --- | ---: | --- |
| Tech Lead / Architect | 1 | 架构、契约、关键评审、跨工程依赖 |
| Backend / Data Engineer | 4 | Core、Realtime、Worker、Model Control、数据库 |
| Client Engineer | 2 | Tauri、React、Desktop、设计系统 |
| Mobile / Native Engineer | 2 | iOS/Android、Push、后台、Swift/Kotlin 插件 |
| Runtime / Security Engineer | 2 | Rust Client Core、agentd、connectord、Capability、Crypto |
| SDET / SRE | 1 | 自动化、性能、故障测试、私有部署与发布 |

产品、设计、安全评审和企业试点负责人必须持续参与，但不计入 12 名技术成员。

### 4.2 计算规则

- 1 人日 = 6 小时有效工程时间；5 人日 = 1 人周。
- 估算已包含编码、Code Review、单元测试和工程文档。
- 41 周实施计划已经考虑并行依赖；另保留 6 周用于移动框架风险、企业环境差异和安全整改。
- 未通过验收、没有迁移和回滚、没有监控和文档的功能不能记为完成。

## 5. 工程任务清单

### P00 架构、仓库与风险验证：70 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P00-01 | 冻结 v1 Scope、用户流和验收场景 | 6 | 无 | PRD、非目标、验收场景签字 |
| P00-02 | ADR：Client、Server、Protocol、Storage、Encryption | 8 | P00-01 | 每个关键决策包含替代方案和回退条件 |
| P00-03 | Threat Model、数据分类和信任边界 | 12 | P00-01 | STRIDE 表、数据流、风险 Owner |
| P00-04 | Monorepo、工具链锁定和开发环境 | 10 | P00-02 | 五个平台可重复构建最小应用 |
| P00-05 | CI 基线、制品签名、SBOM、Secret Scan | 12 | P00-04 | PR 必须通过 Build/Test/Scan |
| P00-06 | Tauri Mobile 技术 Spike | 18 | P00-04 | 真机通过输入法、列表、密钥、文件、Push、恢复测试 |
| P00-07 | 版本、Release Train 和变更控制 | 4 | P00-04 | SemVer、迁移策略、分支策略落文档 |

### P01 设计系统与交互工程：90 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P01-01 | HTML 原型清点和页面状态矩阵 | 8 | P00-01 | Loading/Empty/Error/Offline/Permission 状态齐全 |
| P01-02 | Token：颜色、字体、间距、层级、Motion | 10 | P01-01 | Desktop/Mobile 共用机器可读 Token |
| P01-03 | 基础组件与图标系统 | 20 | P01-02 | Keyboard、Touch、Screen Reader 可用 |
| P01-04 | Desktop 导航、窗口和多栏布局 | 14 | P01-03 | 1280/1440/超宽/缩放 200% 不溢出 |
| P01-05 | Mobile 根页、推进层级和 Sheet 规范 | 14 | P01-03 | iPhone/Android 小屏和安全区通过 |
| P01-06 | 消息、Task、Approval、Artifact 渲染规范 | 14 | P01-03 | 同一 Event 在各端语义一致 |
| P01-07 | 视觉回归和无障碍基线 | 10 | P01-04,P01-05 | 关键帧快照和 axe/人工检查通过 |

### P02 Protocol、SDK 与兼容性：95 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P02-01 | Identity、Channel、Message、Sync Proto | 18 | P00-02 | Go/TS/Rust 代码生成成功 |
| P02-02 | Task、Run、Approval、Artifact Proto | 16 | P02-01 | 状态机和错误码明确 |
| P02-03 | Realtime Envelope、ACK、Cursor、Resume | 16 | P02-01 | Golden Frame 跨三语言一致 |
| P02-04 | Capability Grant、Device、Runtime Proto | 14 | P02-02 | Scope/Expiry/Fencing 字段完整 |
| P02-05 | Connect/gRPC Client SDK 和 Interceptor | 12 | P02-01 | Auth、Retry、Deadline、Trace 一致 |
| P02-06 | Buf Lint/Breaking/Generate CI | 8 | P02-01 | 破坏兼容的 PR 自动失败 |
| P02-07 | Fake Server、Fixture 和 Contract Test | 11 | P02-05 | Client 可脱离真实后端开发 |

### P03 Core、身份、组织与权限：170 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P03-01 | PostgreSQL Schema、Migration、sqlc | 24 | P02-01 | 前进/回滚迁移和测试数据可重复 |
| P03-02 | OIDC PKCE、Session、Token Rotation | 26 | P03-01 | 登录、续期、注销、撤销通过 |
| P03-03 | Device Enrollment 与设备清单 | 18 | P03-02 | 新设备审批、撤销和重新登录通过 |
| P03-04 | Organization、Member、Space、Channel、DM | 32 | P03-01 | 生命周期与唯一约束完整 |
| P03-05 | Membership、RBAC、Resource ACL | 28 | P03-04 | 权限矩阵自动化测试通过 |
| P03-06 | Capability Grant 签发与复检 | 20 | P03-05 | Scope/Expiry/Revocation 不可绕过 |
| P03-07 | Audit Event、Retention 元数据 | 12 | P03-05 | 高风险操作可追溯且无消息明文 |
| P03-08 | Transactional Outbox 基础 | 10 | P03-01 | Domain 与 Outbox 原子提交 |

### P04 Message、Sync 与 Realtime：230 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P04-01 | Message Event、Channel Sequencer | 28 | P03-01,P02-03 | Channel 内稳定单调顺序 |
| P04-02 | Send、Durable ACK、Idempotency | 26 | P04-01,P03-08 | 重试 100 次仍只有一个逻辑事件 |
| P04-03 | History Pagination、Cursor Sync、Gap Repair | 34 | P04-01 | 任意断点可补齐且不跳 Cursor |
| P04-04 | Edit、Redact、Reply、Thread、Reaction、Pin | 34 | P04-02 | 全部以新 Event 表达并可重放 |
| P04-05 | Read Cursor、Mute、Notification Preference | 20 | P04-03 | 多设备已读状态最终收敛 |
| P04-06 | WSS Auth、Connection Directory、Backpressure | 28 | P02-03,P03-02 | 慢客户端不拖垮 Gateway |
| P04-07 | Presence、Typing、Reconnect、Jitter | 18 | P04-06 | Redis 丢失不影响消息事实 |
| P04-08 | Worker Outbox Relay、Fan-out、DLQ | 22 | P03-08,P04-06 | NATS 停机后恢复可重放 |
| P04-09 | 热点 Channel 和断网/重启负载测试 | 20 | P04-01..08 | 达到 SLO，ACK 后零丢失 |

### P05 Client Core 与 locald：200 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P05-01 | Encrypted SQLite Schema 与 Migration | 26 | P02-01 | 升降级、WAL、损坏检测通过 |
| P05-02 | 单 Writer Actor、Desktop IPC | 24 | P05-01 | 多窗口不能并发写坏 DB |
| P05-03 | Local Outbox、Pending/ACK Merge | 28 | P04-02 | Crash 后自动续传且不重复 |
| P05-04 | Cursor、Gap Repair、Event Materialization | 30 | P04-03 | 随机乱序/重复输入状态一致 |
| P05-05 | Keychain/Keystore、DB Key、Device Key | 24 | P00-03 | Key 不落日志、配置或普通文件 |
| P05-06 | FTS5、本地权限复检和索引重建 | 20 | P05-04 | 撤权后不可检索；索引可删除重建 |
| P05-07 | Encrypted Blob Cache 与清理 | 14 | P05-05 | 缓存遵循 Retention 和磁盘配额 |
| P05-08 | WAL Checkpoint、Crash Recovery、诊断包 | 18 | P05-02 | Kill/断电测试后可恢复 |
| P05-09 | Desktop Daemon 与 Mobile Library 封装 | 16 | P05-02..08 | 两种形态通过同一 Contract Test |

### P06 文件、Artifact 与搜索：130 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P06-01 | Multipart Upload Session、Resume | 24 | P03-05 | 中断后从已确认分片继续 |
| P06-02 | Client-side Encryption、Checksum | 20 | P05-05 | 服务端只收到密文 Blob |
| P06-03 | Blob Metadata、ACL、Attachment Link | 18 | P03-05 | 权限继承和收窄可验证 |
| P06-04 | Download、Preview、Cache、Quota | 18 | P06-02 | 失败重试、校验和缓存清理正确 |
| P06-05 | Message/File/Task Local Search | 24 | P05-06 | 过滤、分页和权限复检正确 |
| P06-06 | Virus Scan Hook、Lifecycle、Tombstone | 14 | P06-01 | 扫描故障不阻塞文本消息 |
| P06-07 | Artifact Provenance 和 Run 关联 | 12 | P09-06 | Artifact 可追溯到 Run/Step/Hash |

### P07 Desktop 客户端：220 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P07-01 | Tauri Shell、Sidecar、权限 Manifest | 22 | P00-06,P05-09 | 最小权限启动 locald/agentd/connectord |
| P07-02 | 登录、设备注册、组织切换 | 18 | P03-02,P03-03 | 过期、撤销、离线状态完整 |
| P07-03 | 消息根页、Channel/DM 列表、导航 | 24 | P01-04,P05-04 | 10,000 会话可流畅滚动 |
| P07-04 | Timeline、Thread、Composer、Mention | 34 | P04-04 | 输入法、引用、编辑、撤回可用 |
| P07-05 | File、Search、Preview | 20 | P06 | 拖放、上传、搜索和权限错误完整 |
| P07-06 | Task Activity、Approval、Artifact/Diff | 30 | P09 | 可观察、批准、拒绝、中断和接收结果 |
| P07-07 | Runtime、Workspace、Model 状态 | 18 | P09,P10 | 设备、路径、模型路由边界可见 |
| P07-08 | 通知、快捷键、深链接、系统托盘 | 18 | P04-05 | 三平台行为定义并通过测试 |
| P07-09 | Auto Update、签名、安装和卸载 | 20 | P12-07 | 三平台升级/回滚不丢本地数据 |
| P07-10 | Desktop Accessibility 与性能 | 16 | P07-03..08 | 键盘、读屏、200% 缩放和内存门槛通过 |

### P08 Mobile 客户端：230 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P08-01 | iOS/Android Shell 与 Native Plugin | 30 | P00-06,P05-09 | 真机构建、启动、前后台恢复通过 |
| P08-02 | 登录、Device Enrollment、安全存储 | 22 | P03-02,P05-05 | 生物识别/锁屏后的密钥行为明确 |
| P08-03 | 消息首页、筛选、Channel/DM 导航 | 24 | P01-05,P05-04 | 小屏、安全区、手势层级正确 |
| P08-04 | Timeline、Thread、Composer、输入法 | 34 | P04-04 | 中英文输入、键盘遮挡、长消息通过 |
| P08-05 | Offline Outbox、Reconnect、Sync | 28 | P05-03,P05-04 | 飞行模式和进程回收后恢复 |
| P08-06 | File Picker、Camera、Share Sheet、Preview | 18 | P06 | iOS/Android 权限拒绝路径完整 |
| P08-07 | Task 发起、观察、中断、Approval | 24 | P09 | Mobile 不获取任意文件系统权限 |
| P08-08 | APNs/FCM Adapter 与 Air-gap 降级 | 22 | P04-08 | Standard Private 可推送；隔离环境清楚降级 |
| P08-09 | Signing、MDM、企业分发 | 14 | P12-07 | IPA/APK/AAB 和企业安装说明齐全 |
| P08-10 | Mobile Accessibility、耗电和内存 | 14 | P08-03..08 | 两个平台质量门槛通过 |

### P09 Agent Runtime、Task 与 Connector：240 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P09-01 | Runtime Enrollment、mTLS、Heartbeat | 24 | P03-03,P02-04 | Runtime 只主动出站连接 |
| P09-02 | Task/Run 状态机、Dispatch | 28 | P03-06,P03-08 | Task 与 Run 历史不可覆盖 |
| P09-03 | Lease、Fencing、Retry、Crash Recovery | 30 | P09-02 | 旧 Writer 不能提交新状态 |
| P09-04 | agentd Agent Loop、Session、Cancel | 36 | P09-02 | 多轮、取消、恢复和预算限制通过 |
| P09-05 | Context Manifest、Context API | 26 | P05-04,P03-06 | 只返回授权引用和有限窗口 |
| P09-06 | Run Event、Projection、Artifact | 20 | P09-04 | UI 只展示结构化活动，不刷原始日志 |
| P09-07 | connectord Path Grant、Sandbox | 32 | P03-06 | 无法越过路径、动作和时限范围 |
| P09-08 | Approval 请求、决策、撤权 | 24 | P09-02,P09-07 | 撤权后下一动作立即失败 |
| P09-09 | Workspace Lease、Worktree、并发隔离 | 12 | P09-03,P09-07 | 同 Repo 多 Run 不互相覆盖 |
| P09-10 | Usage、Cost、Model Route 接入 | 8 | P10 | 每次调用可追溯但不记录 Prompt |

### P10 Model Control：85 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P10-01 | Provider Adapter 与 Model Discovery | 18 | P00-02 | 能发现内部 Endpoint 当前模型 |
| P10-02 | Registry、Capability、Parameter Schema | 14 | P10-01 | 能表达上下文、工具、结构化输出能力 |
| P10-03 | Health Probe、熔断和冷却 | 14 | P10-01 | 故障模型自动摘除但不抖动 |
| P10-04 | Route Policy、Fallback、Short-lived Grant | 18 | P10-02,P03-06 | Workflow 不硬编码模型名 |
| P10-05 | Evaluation Case、Score、定期调度 | 14 | P10-02 | 同版本评分可复现并保留证据 |
| P10-06 | Admin 可见性和审计 | 7 | P10-03..05 | 路由原因、分数、版本和失败可解释 |

### P11 Admin、治理与审计：110 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P11-01 | Organization、Member、Role 管理 | 18 | P03 | 管理操作权限和审计完整 |
| P11-02 | Channel Policy、Retention、Guest | 16 | P03-05,P03-07 | 策略继承与例外清楚 |
| P11-03 | Device、Session、Runtime 撤销 | 18 | P03-03,P09-01 | 撤销后连接和 Grant 失效 |
| P11-04 | Agent Directory、Owner、参与模式 | 18 | P09 | Agent 是正式 Actor，可暂停和移除 |
| P11-05 | Audit Viewer、Filter、Export | 18 | P03-07 | 不暴露无权限内容和密钥 |
| P11-06 | Runtime/Model/Queue 健康页面 | 12 | P09,P10,P12 | 管理员可定位故障域 |
| P11-07 | 管理操作二次确认和高风险审批 | 10 | P03-06 | 高影响动作不能单击误触 |

### P12 私有化部署与 SRE：190 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P12-01 | 本地 Compose/Kind 开发栈 | 16 | P00-04 | 一条命令启动全部依赖 |
| P12-02 | OCI Image、Helm Chart、Values Schema | 26 | 服务骨架 | 无公网依赖安装成功 |
| P12-03 | Private CA、mTLS、Vault/KMS 接入 | 26 | P00-03 | 轮换和撤销演练通过 |
| P12-04 | PostgreSQL/NATS/Redis/MinIO HA 基线 | 28 | P12-02 | 节点故障行为符合架构 |
| P12-05 | Migration、Backup、PITR、Restore Drill | 24 | P03-01 | 从备份恢复到目标 RPO/RTO |
| P12-06 | OTel、Dashboard、Alert、Redaction | 22 | 各服务 | Sev-1/2 告警可操作且无敏感正文 |
| P12-07 | 离线 Bundle、SBOM、签名、校验 | 20 | P00-05 | Air-gap 环境可验签和安装 |
| P12-08 | Rolling Upgrade、Rollback、Compatibility | 18 | P02-06,P12-02 | N-1 Client 与滚动升级测试通过 |
| P12-09 | Capacity Guide、Runbook、Support Bundle | 10 | P12-04,P12-06 | 企业运维可独立定位常见故障 |

### P13 QA、安全与发布：260 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P13-01 | Test Plan、Fixture、测试数据生成器 | 18 | P00-01 | 功能与非功能需求均有测试 Owner |
| P13-02 | Unit、Integration、Contract Coverage | 28 | 各工程 | 核心状态机和权限路径强制覆盖 |
| P13-03 | Desktop/Mobile E2E Matrix | 34 | P07,P08 | 五个平台关键流程自动化 |
| P13-04 | Sync Property Test、Fuzz、Replay | 28 | P04,P05 | 重复、乱序、断点随机测试收敛 |
| P13-05 | Load、Soak、Backpressure、Capacity | 30 | P04,P12 | 达到 SLO 并产出容量模型 |
| P13-06 | Chaos：PG/NATS/Redis/Runtime/Network | 26 | P12 | 故障行为与 Runbook 一致 |
| P13-07 | Threat Model 复审、Crypto Review | 24 | P05,P06,P09 | Critical/High 风险关闭 |
| P13-08 | Penetration Test 与整改 | 28 | Feature Complete | 高危问题关闭并复测 |
| P13-09 | Accessibility、IME、时区、语言和升级测试 | 18 | P07,P08 | 平台矩阵通过 |
| P13-10 | 两轮企业 Pilot、UAT、发布决策 | 26 | RC | 阻断问题关闭，签署 GA Checklist |

## 6. 总工作量

| 工程 | 人日 |
| --- | ---: |
| P00 架构与风险验证 | 70 |
| P01 设计系统 | 90 |
| P02 Protocol 与 SDK | 95 |
| P03 Core 与权限 | 170 |
| P04 Message/Sync/Realtime | 230 |
| P05 Client Core/locald | 200 |
| P06 文件与搜索 | 130 |
| P07 Desktop | 220 |
| P08 Mobile | 230 |
| P09 Agent Runtime | 240 |
| P10 Model Control | 85 |
| P11 Admin 与治理 | 110 |
| P12 私有化与 SRE | 190 |
| P13 QA、安全与发布 | 260 |
| **合计** | **2320 人日 / 464 人周** |

## 7. 里程碑与日期

| 里程碑 | 周期 | 完成日期 | 可验收结果 |
| --- | --- | --- | --- |
| M0 架构与 Mobile Gate | W1-W2 | 2026-08-07 | ADR、Threat Model、Proto 骨架、Tauri 真机结论 |
| M1 工程基础 | W3-W6 | 2026-09-04 | Monorepo、CI、OIDC 骨架、Client Core、私有开发栈 |
| M2 IM Vertical Slice | W7-W12 | 2026-10-16 | Desktop/Mobile 登录、Channel 文本消息、离线 Outbox、补洞 |
| M3 Collaboration Alpha | W13-W18 | 2026-11-27 | DM、Thread、Reaction、File、Search、Push |
| M4 Agent Team Beta | W19-W25 | 2027-01-15 | Task、Runtime、Context、Connector、Approval、Artifact、Model Route |
| M5 Private Pilot | W26-W31 | 2027-02-26 | Admin、Audit、OIDC、Helm、Backup、Upgrade、Air-gap Bundle |
| M6 Release Candidate | W32-W37 | 2027-04-09 | 性能、故障、安全、五平台 E2E 达标 |
| M7 Pilot Hardening | W38-W41 | 2027-05-07 | 两轮真实企业环境 UAT 完成 |
| Risk Reserve / GA | W42-W47 | 2027-06-18 | 安全整改、平台差异、迁移修复、正式发布 |

## 8. 并行工程流与关键路径

### 8.1 五条并行线

1. Contract/Core：P02 -> P03 -> P04 -> P06/P11。
2. Client Core：P02 -> P05 -> P07/P08。
3. Runtime：P02/P03 -> P09 -> P10 -> P07/P08 Task UI。
4. Platform：P00 -> P12，持续跟随所有服务。
5. Quality：P01/P02 后启动 P13，不允许等到 Feature Complete 才开始。

### 8.2 关键路径

```text
Scope/Threat Model
  -> Proto Contract
  -> Message Sequencer + Sync
  -> Encrypted locald + Outbox
  -> Desktop/Mobile Messaging
  -> Capability + Runtime + Approval
  -> Private Deployment + Upgrade
  -> Load/Chaos/Pentest
  -> Enterprise Pilot
  -> GA
```

关键路径上的任务延迟不能通过后期增加人员完全追回，因为协议兼容、同步正确性、安全评审和 Pilot
具有真实的串行依赖。

## 9. 每项功能的 Definition of Done

一个任务只有同时满足以下条件才能标记完成：

- 产品验收场景通过，Desktop 和 Mobile 行为明确。
- 权限拒绝、离线、超时、重复、撤销、升级等失败路径通过。
- Unit/Integration/Contract/E2E 按风险等级补齐。
- Metric、Trace、脱敏 Log 和告警已接入。
- 数据 Migration、兼容和 Rollback 已验证。
- 不记录消息明文、Prompt、Token、Key 或用户文件内容。
- API/Proto/Schema/Runbook/用户说明同步更新。
- 性能预算、内存预算、磁盘预算和无障碍标准通过。
- Code Review 和安全 Review 完成，没有未接受的 Critical/High 风险。

## 10. 排期变化规则

以下变更必须重新排期，不能吸收到普通迭代：

| 变更 | 预计增加 |
| --- | ---: |
| Tauri Mobile Gate 失败，切换 React Native | 8-12 周 |
| v1 加入 MLS E2EE、设备恢复和 History Sharing | 12-18 周 |
| v1 加入 SAML + SCIM | 5-8 周 |
| v1 加入 Enterprise Runner | 6-10 周 |
| v1 加入完整服务端内容搜索/DLP | 6-10 周 |
| v1 加入音视频 | 至少 20-30 周，建议独立项目 |
| v1 要求跨 Region Active-Active | 至少 16-24 周，建议独立架构阶段 |

## 11. 人数变化对应日历时间

| 技术团队 | 合理 GA 周期 | 说明 |
| --- | --- | --- |
| 12 人 | 44-47 周 | 本文基准；5 条工程线可并行 |
| 8 人 | 64-72 周 | Mobile、Runtime、SRE 并行度下降 |
| 5 人 | 96-112 周 | 只能分阶段发布，安全与平台仍不可省略 |
| 3 人 | 150 周以上 | 适合做 Pilot，不适合承诺企业 GA |

编码 Agent 可以减少样板代码、测试生成和文档时间，但不能替代移动真机验证、加密评审、故障演练、
企业 Pilot 和发布责任，因此不单独从 GA 基准中扣减安全储备。

## 12. 第一批必须建立的工作项

启动时先创建以下 Epic，禁止直接从 UI 页面开始散点编码：

1. `EPIC-000 Product Scope and Acceptance`
2. `EPIC-010 Threat Model and Data Classification`
3. `EPIC-020 Monorepo and Build Reproducibility`
4. `EPIC-030 Protocol v1 and Compatibility`
5. `EPIC-040 Tauri Desktop/Mobile Technology Gate`
6. `EPIC-100 Identity, Device and Organization`
7. `EPIC-200 Message Ordering and Durable Sync`
8. `EPIC-300 Encrypted Client Core and locald`
9. `EPIC-400 Desktop IM`
10. `EPIC-500 Mobile IM`
11. `EPIC-600 File, Blob and Search`
12. `EPIC-700 Task, Runtime and Capability`
13. `EPIC-800 Approval, Artifact and Model Routing`
14. `EPIC-900 Private Deployment and Operations`
15. `EPIC-950 Quality, Security and Enterprise Pilot`

## 13. 技术依据

- Tauri 2 官方支持单代码库构建 Linux、macOS、Windows、Android 和 iOS，并允许通过 Rust、Swift、
  Kotlin 接入系统能力：https://v2.tauri.app/
- Tauri 官方建议 SPA 使用 Vite，并将应用与 API 保持明确 Client-Server 关系：
  https://v2.tauri.app/start/frontend/
- ConnectRPC 以 Protobuf 定义浏览器与 gRPC 兼容 API，并提供 Go、TypeScript、Swift、Kotlin、Dart
  等客户端：https://connectrpc.com/
- Buf CLI 支持本地代码生成、Lint 和 Breaking Change 检测，可在 CI 中对 Git 基线执行，不要求使用
  公网 Registry：https://buf.build/docs/breaking/

## 14. Agent 异步实施规则

本文中的 116 个 `Pxx-xx` 是排期工作包，不直接等于一个 Agent Task。执行前必须按照
[`agent-workstreams.md`](./agent-workstreams.md) 拆成 0.5 至 2 Agent 日的独立 Issue，并明确：

- 唯一 Workstream 和路径 Owner。
- 输入 Contract、Base Commit 和阻塞依赖。
- 可单独运行的验收命令。
- 是否修改 Proto、Migration、Generated Code 或 Lockfile。
- 完成后的 Commit、Handoff 和需要解锁的后继任务。

不同 Agent 使用独立 Git Worktree；跨工程依赖通过 Contract、Fake 和 Fixture 解耦；`main` 只由
Integration Owner 通过 Review/Merge Queue 更新。任何需要两个 Agent 同时修改同一文件的拆分都应视为
无效拆分并重新划分边界。
