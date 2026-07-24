# Threadline Private Enterprise v1.0 交付计划

状态：Frozen Scope 1.1 交付基线 1.0

计划起点：2026-07-27

基准 GA：2027-10-29

估算口径：12 名技术成员，2937 人日，58 周实施 + 8 周 Pilot/风险储备

## 1. 交付目标

交付一套可在企业内网安装、升级和运维的 Agent 原生企业 IM：

- Desktop：macOS、Windows、Linux。
- Mobile：iOS、Android。
- 企业成员可以私聊、频道沟通、Thread、搜索和传递文件。
- 消息支持离线发送、断线重连、幂等、频道内排序、Cursor 补洞和多端同步。
- Desktop 运行本地 Agent Runtime；Mobile 可以发起、观察、中断和审批任务，但不执行长任务。
- Agent 只能通过短期 Capability Grant 读取明确选择的消息和工作区。
- 消息正文和附件默认 E2EE；应用服务只保存密文，企业恢复由隔离的 KMS/HSM 权限控制。
- 支持企业 OIDC、设备管理、审计、保留策略、模型发现与路由。
- 提供单 Region 私有化 Helm 部署、离线安装包、备份恢复、升级回滚和运维手册。

### 1.1 本版本不包含

- SaaS 多租户运营、计费、套餐和公网控制台。
- SAML、SCIM、Legal Hold 完整工作流。
- 音视频会议、直播、白板和多人实时文档。
- 跨 Region Active-Active 和全球 Channel Sequencer。
- 公有模型托管或企业模型部署。
- 用户可创建的严格不可恢复 Channel。v1 只交付 E2EE + 企业 KMS/HSM 可审计恢复。
- Enterprise Runner。v1 Agent Runtime 运行在授权 Desktop/Workstation。

## 2. 技术架构定案

### 2.1 客户端

| 层 | 选择 | 说明 |
| --- | --- | --- |
| Desktop | Tauri 2 + React + TypeScript + Vite | 构建 macOS、Windows、Linux；负责完整 IM、本地 Runtime 与 Workspace UI |
| Desktop UI 状态 | TanStack Query + Zustand | RPC/缓存状态与短期 UI 状态分开；消息事实仍在 `locald` |
| iOS | Swift + SwiftUI，必要时使用 UIKit | 原生消息列表、Composer、Push、后台、Keychain、文件和企业分发 |
| Android | Kotlin + Jetpack Compose | 原生消息列表、Composer、Push、后台、Keystore、文件和企业分发 |
| Shared Client Core | Rust | E2EE、加密 SQLite、Outbox、Cursor、同步归并、本地搜索和附件加密 |
| Native Bridge | 版本化 Rust FFI | Swift/Kotlin 只依赖稳定 Facade；错误、取消、流和内存所有权必须 Contract Test |
| Desktop 本地服务 | `locald`、`agentd`、`connectord` | UI 不直接写 SQLite，不直接访问任意文件系统 |
| Mobile 本地能力 | Rust Core 进程内 Actor | 不包含 `agentd` 和 `connectord`，不执行长任务 |

M0 验证两条不可逆风险：Rust Core 在 Swift/Kotlin 中的 FFI、取消和恢复，以及候选 Group E2EE 库
在三端的互操作、Key Package、Epoch、History Sharing 和恢复封装。失败时必须更换 Bridge 或密码库
并重开 ADR，不能退化为服务端明文。

### 2.2 服务端

| 层 | 选择 | 说明 |
| --- | --- | --- |
| 语言 | Go | 复用现有 `agent-core` 能力与团队执行契约；适合网络服务和单二进制私有化交付 |
| RPC | Protobuf + ConnectRPC | Admin Web、Desktop、iOS、Android 走 Connect；服务间和 Runtime Stream 走 gRPC/mTLS |
| Realtime | WSS + Protobuf Binary Frame | WSS 只负责连接与在线提示；可靠性由 Outbox、ACK、Cursor Sync 保证 |
| 数据访问 | pgx + sqlc | 显式 SQL、事务和 Schema Ownership，不使用隐式 ORM |
| 事实源 | PostgreSQL HA | Domain Event、状态表、Transactional Outbox、Job 和 Audit |
| 事件层 | NATS JetStream | Fan-out、投影、通知和任务派发；不是消息事实源 |
| 短期状态 | Redis | Presence、Typing、连接目录、限流；允许丢失重建 |
| Blob | S3-compatible / MinIO | 加密附件、Artifact、诊断包 |
| 密钥 | Vault/HSM/KMS | 服务凭据、设备 Grant、企业恢复私钥、模型短期授权和数据密钥包装 |
| 可观测 | OpenTelemetry | 内网 Collector、Metric、Trace、脱敏 Log；禁止记录消息正文和 Prompt |

### 2.3 服务工作负载

生产规划 7 个服务端工作负载：

1. `threadline-web`：静态 Web 与管理页面。
2. `threadline-core`：Identity、Organization、Channel、Message、Sync、File、Task、Approval、Audit。
3. `threadline-realtime`：Client WSS、Presence、Typing、Fan-out、Backpressure。
4. `threadline-runtime-gateway`：Desktop Runtime Enrollment、Heartbeat、Dispatch、Run Event。
5. `threadline-worker`：Outbox Relay、Projection、Notification、Retention、Scan、DLQ。
6. `threadline-model-control`：模型发现、能力、健康、评测、评分和路由授权。
7. `threadline-recovery-control`：隔离的恢复审批、KMS/HSM 调用和恢复审计；不在日常消息链路上。

Core 是模块化单体。P0 不继续拆 Identity、Message、Task 等内部模块，避免分布式事务和过早运维成本。
`recovery-control` 的网络、身份和密钥权限必须与 Core 隔离；具体 Group E2EE 与恢复封装由 M0 ADR 决定。

### 2.4 协议与数据规则

- `.proto` 是跨端契约事实源，使用 Buf CLI 做 Lint、Generate 和 Breaking Check，不依赖公网 BSR。
- 公开字段只能新增；删除字段必须 `reserved`；持久化 Envelope 需要 Golden Frame 测试。
- Message Command、`channel_seq`、Event 和 Transactional Outbox 在同一 PostgreSQL 事务提交。
- 客户端先写 Durable Local Outbox，收到 Durable ACK 后才从 Pending 转为 Committed。
- 所有 Consumer 都按 At-least-once 和业务幂等设计。
- Desktop/Mobile 各自维护本地数据库，不复制或共享 SQLite 文件。
- Server、Realtime、Worker 和 Model Control 只处理 Ciphertext Envelope 和必要 Metadata。
- Agent Runtime 只通过 Context API 获取有限明文，不 Mount IM 数据库，不读取整个 Channel。
- Runtime 在本地组装 Prompt 并直连企业批准的模型 Endpoint；Model Control 不代理 Prompt。

## 3. 建议仓库结构

```text
threadline/
  apps/
    desktop/                 # React UI + Tauri desktop shell
    ios/                     # SwiftUI/UIKit native client
    android/                 # Kotlin/Compose native client
    admin-web/               # 私有化管理页面
  crates/
    client-core/             # Rust sync/crypto/sqlite/search core
    client-crypto/           # reviewed Group E2EE adapter and recovery envelope
    client-ffi/              # stable Swift/Kotlin facade and generated bindings
    locald/                  # Desktop single-writer daemon
    connectord/              # Workspace capability and sandbox
    tauri-plugins/           # desktop keychain, file, updater adapters
  services/
    core/
    realtime/
    runtime-gateway/
    worker/
    model-control/
    recovery-control/
    agentd/                  # Go; wraps agent-core
  packages/
    ui/
    design-tokens/
    client-domain/
    generated-ts/
    generated-swift/
    generated-kotlin/
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

Monorepo 使用 `pnpm workspace + Cargo workspace + Go workspace + SwiftPM + Gradle + just/Makefile`。
不在第一阶段引入 Bazel；当增量构建和跨语言依赖真的成为瓶颈后再评估。

## 4. 估算假设

### 4.1 标准团队

| 角色 | 人数 | 主要责任 |
| --- | ---: | --- |
| Tech Lead / Architect | 1 | 架构、契约、关键评审、跨工程依赖 |
| Backend / Data Engineer | 4 | Core、Realtime、Worker、Model/Recovery Control、数据库 |
| Client Engineer | 2 | Tauri、React、Desktop、设计系统 |
| Mobile / Native Engineer | 2 | 1 名 iOS、1 名 Android；原生 UI、Push、后台、企业分发 |
| Runtime / Security Engineer | 2 | Rust Core/FFI、agentd、connectord、Capability、E2EE |
| SDET / SRE | 1 | 自动化、性能、故障测试、私有部署与发布 |

产品、设计、安全评审和企业试点负责人必须持续参与，但不计入 12 名技术成员。Group E2EE 方案还必须
安排阶段性密码协议顾问和独立安全审计方；其日历等待时间计入计划，供应商费用不折算为内部人日。

### 4.2 计算规则

- 1 人日 = 6 小时有效工程时间；5 人日 = 1 人周。
- 估算已包含编码、Code Review、单元测试和工程文档。
- 58 周实施计划已经考虑并行依赖；另保留 8 周用于企业 Pilot、密码审计整改和平台差异。
- 未通过验收、没有迁移和回滚、没有监控和文档的功能不能记为完成。

## 5. 工程任务清单

### P00 架构、仓库与风险验证：118 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P00-01 | 冻结 v1 Scope、用户流和验收场景 | 8 | 无 | Scope、非目标、首个垂直切片和签字记录完整 |
| P00-02 | ADR：Client、Server、Protocol、Storage | 10 | P00-01 | 每个关键决策包含替代方案、约束和回退条件 |
| P00-03 | ADR：Group E2EE、设备密钥与企业恢复 | 14 | P00-01 | 选定候选协议/库，定义 Epoch、History、Recovery 和版本边界 |
| P00-04 | Threat Model、数据分类和信任边界 | 16 | P00-01,P00-03 Draft | STRIDE、数据流、Prompt/Recovery 边界和风险 Owner |
| P00-05 | Monorepo、工具链锁定和开发环境 | 14 | P00-02 | Desktop、iOS、Android、Go/Rust 服务可重复构建 |
| P00-06 | CI 基线、制品签名、SBOM、Secret Scan | 12 | P00-05 | PR 必须通过 Build/Test/Scan |
| P00-07 | Rust FFI Swift/Kotlin 技术 Spike | 18 | P00-02,P00-05 | 真机通过调用、流、取消、错误映射、Crash/Resume 和内存测试 |
| P00-08 | Group E2EE 与 Recovery 互操作 Spike | 22 | P00-03,P00-05 | 三端 Golden Vector、Epoch、离线设备、新设备和恢复路径通过 |
| P00-09 | 版本、Release Train 和变更控制 | 4 | P00-05 | SemVer、迁移策略、分支策略落文档 |

### P01 设计系统与交互工程：104 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P01-01 | HTML 原型清点和页面状态矩阵 | 8 | P00-01 | Loading/Empty/Error/Offline/Permission 状态齐全 |
| P01-02 | Token：颜色、字体、间距、层级、Motion | 12 | P01-01 | TS、Swift、Kotlin 消费同一机器可读 Token |
| P01-03 | 分平台基础组件与图标规范 | 24 | P01-02 | Desktop、iOS、Android 的 Keyboard/Touch/Screen Reader 可用 |
| P01-04 | Desktop 导航、窗口和多栏布局 | 14 | P01-03 | 1280/1440/超宽/缩放 200% 不溢出 |
| P01-05 | iOS/Android 根页、推进层级和 Sheet 规范 | 18 | P01-03 | 两个平台小屏、安全区和系统手势通过 |
| P01-06 | 消息、Task、Approval、Artifact 渲染规范 | 16 | P01-03 | 同一 Event 在各端语义一致 |
| P01-07 | 视觉回归和无障碍基线 | 12 | P01-04,P01-05 | 关键帧、axe、XCTest/Compose 人工检查通过 |

### P02 Protocol、SDK 与兼容性：125 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P02-01 | Identity、Channel、Message、Sync Proto | 18 | P00-02 | Go/TS/Rust 代码生成成功 |
| P02-02 | Task、Run、Approval、Artifact Proto | 16 | P02-01 | 状态机和错误码明确 |
| P02-03 | Realtime Envelope、ACK、Cursor、Resume | 16 | P02-01 | Golden Frame 跨三语言一致 |
| P02-04 | Device、Key Package、Epoch、Recovery Envelope | 22 | P00-03,P02-01 | 版本、轮换、恢复和未知扩展字段明确 |
| P02-05 | Capability Grant、Execution Owner、Runtime Proto | 14 | P02-02 | Scope/Expiry/Fencing/Transfer 字段完整 |
| P02-06 | Connect/gRPC SDK 和 Interceptor | 18 | P02-01 | Go/TS/Swift/Kotlin/Rust 的 Auth、Retry、Deadline、Trace 一致 |
| P02-07 | Buf Lint/Breaking/Generate CI | 8 | P02-01 | 破坏兼容的 PR 自动失败 |
| P02-08 | Fake Server、Golden Vector 与 Contract Test | 13 | P02-04,P02-06 | Client 可脱离真实后端开发且密文帧跨端一致 |

### P03 Core、身份、组织与权限：192 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P03-01 | PostgreSQL Schema、Migration、sqlc | 24 | P02-01 | 前进/回滚迁移和测试数据可重复 |
| P03-02 | OIDC PKCE、Session、Token Rotation | 26 | P03-01 | 登录、续期、注销、撤销通过 |
| P03-03 | Device Enrollment、Key Package 与设备清单 | 24 | P03-02,P02-04 | 新设备审批、撤销、密钥发布和重新登录通过 |
| P03-04 | Organization、Member、Space、Channel、DM | 32 | P03-01 | 生命周期与唯一约束完整 |
| P03-05 | Membership、RBAC、Resource ACL | 28 | P03-04 | 权限矩阵自动化测试通过 |
| P03-06 | Capability Grant 签发与复检 | 20 | P03-05 | Scope/Expiry/Revocation 不可绕过 |
| P03-07 | Audit、Retention 与 Recovery 元数据 | 18 | P03-05,P00-03 | 高风险操作和恢复审批可追溯且无消息明文 |
| P03-08 | Transactional Outbox 基础 | 10 | P03-01 | Domain 与 Outbox 原子提交 |
| P03-09 | 管理员邀请、停用与 CSV 成员导入 | 10 | P03-04,P03-05 | 批量导入、重复、撤权和错误报告通过 |

### P04 Message、Sync 与 Realtime：256 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P04-01 | Ciphertext Message Event、Channel Sequencer | 28 | P03-01,P02-03 | Channel 内稳定单调顺序且 Server 不解析正文 |
| P04-02 | Send、Durable ACK、Idempotency | 26 | P04-01,P03-08 | 重试 100 次仍只有一个逻辑事件 |
| P04-03 | History Pagination、Cursor Sync、Gap Repair | 34 | P04-01 | 任意断点可补齐且不跳 Cursor |
| P04-04 | Edit、Redact、Reply、Thread、Reaction、Pin | 34 | P04-02 | 全部以新 Event 表达并可重放 |
| P04-05 | Read Cursor、Mute、Notification Preference | 20 | P04-03 | 多设备已读状态最终收敛 |
| P04-06 | WSS Auth、Connection Directory、Backpressure | 28 | P02-03,P03-02 | 慢客户端不拖垮 Gateway |
| P04-07 | Presence、Typing、Reconnect、Jitter | 18 | P04-06 | Redis 丢失不影响消息事实 |
| P04-08 | Worker Outbox Relay、Fan-out、DLQ | 22 | P03-08,P04-06 | NATS 停机后恢复可重放 |
| P04-09 | Membership Commit、Epoch 与消息顺序协调 | 24 | P02-04,P04-01 | 离线设备、乱序 Commit 和 Epoch 切换不丢消息 |
| P04-10 | 热点 Channel 和断网/重启负载测试 | 22 | P04-01..09 | 达到 SLO，ACK 后零丢失 |

### P05 Client Core、Crypto 与 locald：320 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P05-01 | Encrypted SQLite Schema 与 Migration | 26 | P02-01 | 升降级、WAL、损坏检测通过 |
| P05-02 | 单 Writer Actor、Desktop IPC | 24 | P05-01 | 多窗口不能并发写坏 DB |
| P05-03 | Local Outbox、Pending/ACK Merge | 28 | P04-02 | Crash 后自动续传且不重复 |
| P05-04 | Cursor、Gap Repair、Event Materialization | 30 | P04-03 | 随机乱序/重复输入状态一致 |
| P05-05 | OS Secure Storage、DB Key 与 Device Identity | 30 | P00-04 | Key 不落日志、配置或普通文件，撤销后不可复用 |
| P05-06 | Group E2EE State、Membership 与 Epoch | 46 | P00-08,P02-04 | 三端互操作、乱序、重放、并发 Commit 和轮换通过 |
| P05-07 | 新设备授权、History Sharing 与恢复 | 32 | P05-05,P05-06 | 新设备、丢失设备、离线成员和恢复失败可验证 |
| P05-08 | 企业 Recovery Envelope 与客户端仪式 | 24 | P00-03,P05-06 | 恢复接收者可选封装，不向 Agent/Server 暴露私钥 |
| P05-09 | FTS5、本地权限复检和索引重建 | 20 | P05-04,P05-06 | 撤权后不可检索；索引可删除重建 |
| P05-10 | Encrypted Blob Cache 与清理 | 14 | P05-05 | 缓存遵循 Retention 和磁盘配额 |
| P05-11 | WAL Checkpoint、Crash Recovery、诊断包 | 18 | P05-02 | Kill/断电测试后可恢复且诊断包无明文 |
| P05-12 | Desktop Daemon、Swift/Kotlin FFI 封装 | 28 | P00-07,P05-02..11 | 三种宿主通过同一 Core Contract Test |

### P06 文件、Artifact 与搜索：150 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P06-01 | Multipart Upload Session、Resume | 24 | P03-05 | 中断后从已确认分片继续 |
| P06-02 | E2EE File Encryption、Checksum、Key Wrap | 26 | P05-05,P05-06 | 服务端只收到密文 Blob 且成员变更后 Key 行为明确 |
| P06-03 | Blob Metadata、ACL、Attachment Link | 18 | P03-05 | 权限继承和收窄可验证 |
| P06-04 | Download、Preview、Cache、Quota | 20 | P06-02 | 失败重试、校验、密钥缺失和缓存清理正确 |
| P06-05 | Message/File/Task Local Search | 28 | P05-09 | 本地过滤、分页、权限复检和索引重建正确 |
| P06-06 | 客户端 Scan Hook、Lifecycle、Tombstone | 18 | P06-01,P06-02 | 扫描在加密前完成，故障不阻塞文本消息 |
| P06-07 | Artifact E2EE、Provenance 和 Run 关联 | 16 | P09-06 | Artifact 可追溯到 Run/Step/Hash 且对象存储无明文 |

### P07 Desktop 客户端：235 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P07-01 | Tauri Shell、Sidecar、权限 Manifest | 22 | P00-05,P05-12 | 最小权限启动 locald/agentd/connectord |
| P07-02 | 登录、设备注册、密钥授权、组织切换 | 20 | P03-02,P03-03,P05-05 | 过期、撤销、离线和 Key Package 状态完整 |
| P07-03 | 消息根页、Channel/DM 列表、导航 | 26 | P01-04,P05-04 | 10,000 会话可流畅滚动 |
| P07-04 | Timeline、Thread、Composer、Mention | 36 | P04-04,P05-06 | 输入法、引用、编辑、撤回和解密失败可用 |
| P07-05 | File、Search、Preview | 22 | P06 | 拖放、上传、搜索、密钥和权限错误完整 |
| P07-06 | Task Activity、Approval、Artifact/Diff | 32 | P09 | 可观察、批准、拒绝、中断和接收结果 |
| P07-07 | Runtime、Workspace、Model/数据边界状态 | 20 | P09,P10 | 设备、路径、模型 Endpoint 和数据去向可见 |
| P07-08 | 通知、快捷键、深链接、系统托盘 | 18 | P04-05 | 三平台行为定义并通过测试 |
| P07-09 | Auto Update、签名、安装和卸载 | 20 | P12-07 | 三平台升级/回滚不丢本地数据 |
| P07-10 | Desktop Accessibility、性能与恢复 UI | 19 | P07-03..08 | 键盘、读屏、200% 缩放、恢复审批和内存门槛通过 |

### P08 原生 Mobile 客户端：362 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P08-01 | iOS SwiftUI/UIKit 工程与模块边界 | 26 | P00-05 | 真机构建、启动、导航和测试 Host 可重复 |
| P08-02 | Android Compose 工程与模块边界 | 26 | P00-05 | 真机构建、启动、导航和测试 Host 可重复 |
| P08-03 | Rust FFI Binding、流、取消和恢复 | 24 | P00-07,P05-12 | Swift/Kotlin 通过同一 Contract/Fault Test |
| P08-04 | iOS Enrollment、Keychain、E2EE 状态 | 22 | P03-03,P05-05..08 | 锁屏、生物识别、撤销、恢复和重装行为明确 |
| P08-05 | Android Enrollment、Keystore、E2EE 状态 | 22 | P03-03,P05-05..08 | 锁屏、生物识别、撤销、恢复和重装行为明确 |
| P08-06 | iOS 消息首页、Channel/DM 导航 | 24 | P01-05,P05-04 | 小屏、安全区、手势层级正确 |
| P08-07 | Android 消息首页、Channel/DM 导航 | 24 | P01-05,P05-04 | 小屏、系统返回和手势层级正确 |
| P08-08 | iOS Timeline、Thread、Composer | 30 | P04-04,P05-06 | 中英文输入、键盘、长消息和解密失败通过 |
| P08-09 | Android Timeline、Thread、Composer | 30 | P04-04,P05-06 | 中英文输入、键盘、长消息和解密失败通过 |
| P08-10 | Offline Outbox、Reconnect、进程恢复 | 30 | P05-03,P05-04 | 两端飞行模式和进程回收后收敛 |
| P08-11 | File Picker、Camera、Share、Preview | 18 | P06 | 两端权限拒绝、加密和恢复路径完整 |
| P08-12 | Task 发起、观察、中断、Approval | 24 | P09 | Mobile 不获取 Workspace 文件权限 |
| P08-13 | APNs/FCM 与 Air-gap 降级 | 24 | P04-08 | Standard Private 可推送；隔离环境清楚降级 |
| P08-14 | iOS/Android Signing、MDM、企业分发 | 18 | P12-07 | IPA/APK/AAB 和企业安装说明齐全 |
| P08-15 | Mobile Accessibility、耗电、内存和启动 | 20 | P08-06..13 | 两个平台质量预算通过 |

### P09 Agent Runtime、Task 与 Connector：265 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P09-01 | Runtime Enrollment、mTLS、Heartbeat | 24 | P03-03,P02-04 | Runtime 只主动出站连接 |
| P09-02 | Task/Run 状态机、Execution Owner、Dispatch | 30 | P03-06,P03-08 | Task/Run 历史不可覆盖且同一 Run 只有一个 Owner |
| P09-03 | Lease、Fencing、Transfer、Crash Recovery | 32 | P09-02 | 旧 Writer 和旧 Grant 不能提交新状态 |
| P09-04 | agentd Agent Loop、Session、Cancel | 38 | P09-02 | 多轮、取消、恢复和预算限制通过 |
| P09-05 | Context Manifest、本地解密 Context API | 28 | P05-04,P05-06,P03-06 | 只返回授权引用和有限窗口，Server 不接触明文 |
| P09-06 | Run Event、Projection、Artifact | 20 | P09-04 | UI 只展示结构化活动，不刷原始日志 |
| P09-07 | connectord Path Grant、Sandbox | 32 | P03-06 | 无法越过路径、动作和时限范围 |
| P09-08 | Approval 请求、决策、撤权 | 24 | P09-02,P09-07 | 撤权后下一动作立即失败 |
| P09-09 | Workspace Lease、Worktree、并发隔离 | 16 | P09-03,P09-07 | 同 Repo 多 Run 不互相覆盖 |
| P09-10 | 本地 Model Client、Route Grant、Usage | 10 | P10 | Runtime 直连 Endpoint；可追溯但不记录 Prompt |
| P09-11 | 首个 E2EE-to-Agent 垂直切片集成 | 11 | P04,P05,P09-02..10,P10 | 消息转 Task、审批和 Artifact 闭环通过 |

### P10 Model Control：105 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P10-01 | Provider Adapter 与动态 Model Discovery | 20 | P00-02 | 能发现、刷新和下线企业 Endpoint 当前模型 |
| P10-02 | Registry、Capability、Parameter、Data Boundary | 16 | P10-01 | 能表达上下文、工具、输出和数据位置硬约束 |
| P10-03 | Health Probe、熔断和冷却 | 14 | P10-01 | 故障模型自动摘除但不抖动 |
| P10-04 | Policy-first Route、Run Pin、Fallback Grant | 20 | P10-02,P03-06 | Workflow 不硬编码模型名，Run 内不静默漂移 |
| P10-05 | Evaluation Case、Score、定期调度 | 18 | P10-02 | 分阶段能力评分可复现并保留证据 |
| P10-06 | 本地 Runtime Route API 与短期授权 | 10 | P10-04,P09-01 | Model Control 不代理 Prompt，Grant 不能越过数据策略 |
| P10-07 | Admin 可见性和审计 | 7 | P10-03..06 | 路由原因、分数、版本和失败可解释 |

### P11 Admin、治理与审计：130 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P11-01 | Organization、Member、Role、CSV 管理 | 20 | P03 | 管理操作、批量导入、停用和审计完整 |
| P11-02 | Channel Policy、Retention、Guest | 16 | P03-05,P03-07 | 策略继承与例外清楚 |
| P11-03 | Device、Session、Runtime、Key 撤销 | 18 | P03-03,P09-01 | 撤销后连接、Grant 和未来 Epoch 访问失效 |
| P11-04 | Agent Directory、Owner、参与模式 | 16 | P09 | Agent 是正式 Actor，可暂停和移除 |
| P11-05 | Audit Viewer、Filter、Export | 18 | P03-07 | 不暴露无权限内容和密钥 |
| P11-06 | Recovery 双人审批、状态与不可变审计 | 20 | P00-03,P03-07,P12-03 | Core 无法代替审批人调用恢复私钥 |
| P11-07 | Runtime/Model/Queue 健康页面 | 12 | P09,P10,P12 | 管理员可定位故障域 |
| P11-08 | 管理操作二次确认和高风险审批 | 10 | P03-06 | 高影响动作不能单击误触 |

### P12 私有化部署与 SRE：215 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P12-01 | 本地 Compose/Kind 开发栈 | 16 | P00-05 | 一条命令启动全部依赖 |
| P12-02 | OCI Image、Helm Chart、Values Schema | 28 | 服务骨架 | 7 个工作负载无公网依赖安装成功 |
| P12-03 | Private CA、mTLS、KMS/HSM Recovery 隔离 | 34 | P00-04 | 服务凭据轮换、双人恢复授权和撤销演练通过 |
| P12-04 | PostgreSQL/NATS/Redis/MinIO HA 基线 | 28 | P12-02 | 节点故障行为符合架构 |
| P12-05 | Migration、Backup、PITR、Restore Drill | 24 | P03-01 | 从备份恢复到目标 RPO/RTO |
| P12-06 | OTel、Dashboard、Alert、Redaction | 22 | 各服务 | Sev-1/2 告警可操作且无敏感正文 |
| P12-07 | 离线 Bundle、SBOM、签名、校验 | 20 | P00-06 | Air-gap 环境可验签和安装 |
| P12-08 | Rolling Upgrade、Crypto Version、Rollback | 20 | P02-04,P02-07,P12-02 | N-1 Client、Epoch 和滚动升级测试通过 |
| P12-09 | Capacity Guide、Runbook、Support Bundle | 12 | P12-04,P12-06 | 企业运维可独立定位常见故障且诊断无明文 |
| P12-10 | Recovery Backup、HSM Disaster Drill | 11 | P12-03,P12-05 | 应用备份不含恢复私钥，HSM 灾备独立演练通过 |

### P13 QA、安全与发布：360 人日

| ID | 任务 | 人日 | 依赖 | 完成标准 |
| --- | --- | ---: | --- | --- |
| P13-01 | Test Plan、Fixture、测试数据生成器 | 20 | P00-01 | 功能、E2EE 和非功能需求均有测试 Owner |
| P13-02 | Unit、Integration、Contract Coverage | 32 | 各工程 | 核心状态机、权限和 FFI 路径强制覆盖 |
| P13-03 | Desktop/iOS/Android E2E Matrix | 48 | P07,P08 | 五个平台关键流程自动化并保留真机证据 |
| P13-04 | Crypto Interop、Golden Vector、Property/Fuzz | 44 | P05,P06 | 三端 Epoch、History、Recovery、乱序和损坏输入收敛 |
| P13-05 | Sync Property Test、Fuzz、Replay | 30 | P04,P05 | 重复、乱序、断点随机测试收敛 |
| P13-06 | Load、Soak、Backpressure、Capacity | 32 | P04,P12 | 达到 SLO 并产出容量模型 |
| P13-07 | Chaos：PG/NATS/Redis/Runtime/Network/KMS | 30 | P12 | 故障行为与 Runbook 一致且不泄露密钥 |
| P13-08 | Threat Model 复审与独立 Crypto Review | 42 | P05,P06,P09 | Critical/High 风险关闭并取得外部复审证据 |
| P13-09 | Penetration Test 与整改 | 32 | Feature Complete | 高危问题关闭并复测 |
| P13-10 | Accessibility、IME、时区、语言和升级 | 22 | P07,P08 | 平台矩阵通过 |
| P13-11 | 两轮企业 Pilot、UAT、发布决策 | 28 | RC | 阻断问题关闭，签署 GA Checklist |

## 6. 总工作量

| 工程 | 人日 |
| --- | ---: |
| P00 架构与风险验证 | 118 |
| P01 设计系统 | 104 |
| P02 Protocol 与 SDK | 125 |
| P03 Core 与权限 | 192 |
| P04 Message/Sync/Realtime | 256 |
| P05 Client Core/Crypto/locald | 320 |
| P06 文件与搜索 | 150 |
| P07 Desktop | 235 |
| P08 原生 Mobile | 362 |
| P09 Agent Runtime | 265 |
| P10 Model Control | 105 |
| P11 Admin 与治理 | 130 |
| P12 私有化与 SRE | 215 |
| P13 QA、安全与发布 | 360 |
| **合计** | **2937 人日 / 587.4 人周** |

## 7. 里程碑与日期

| 里程碑 | 周期 | 完成日期 | 可验收结果 |
| --- | --- | --- | --- |
| M0 架构、Crypto 与 Native Bridge Gate | W1-W4 | 2026-08-21 | Scope、ADR、Threat Model、Proto 骨架、FFI/E2EE 互操作证据 |
| M1 工程基础 | W5-W8 | 2026-09-18 | Monorepo、CI、OIDC、Key Package 骨架、Client Core、私有开发栈 |
| M2 首个产品垂直切片 | W9-W16 | 2026-11-13 | Desktop E2EE 消息、离线 Outbox、本地 Agent、审批与 Artifact 闭环 |
| M3 Collaboration Alpha | W17-W24 | 2027-01-08 | DM、Thread、Reaction、File、Local Search；iOS/Android E2EE 消息 Alpha |
| M4 Agent Team Beta | W25-W34 | 2027-03-19 | Runtime、Context、Connector、Approval、Artifact、Model Route；Mobile Task 控制 |
| M5 Private Deployment Beta | W35-W43 | 2027-05-21 | Admin、Recovery、Audit、Helm、Backup、Upgrade、Air-gap 和企业分发 |
| M6 Release Candidate | W44-W52 | 2027-07-23 | 可靠性、性能、故障、五平台 E2E 和 Feature Complete |
| M7 Security/Pilot Candidate | W53-W58 | 2027-09-03 | 独立 Crypto Review、Pentest、整改和首轮企业 UAT |
| Pilot Hardening | W59-W64 | 2027-10-15 | 第二轮企业 Pilot、升级恢复演练和阻断问题关闭 |
| Risk Reserve / GA | W65-W66 | 2027-10-29 | 最终安全复测、平台差异修复和正式发布 |

## 8. 并行工程流与关键路径

### 8.1 七条并行线

1. Contract/Core：P02 -> P03 -> P04 -> P06/P11。
2. Crypto/Recovery：P00-03/P00-08 -> P02-04 -> P05-05..08 -> P11-06/P12-03 -> P13-04/P13-08。
3. Client Core：P02 -> P05 -> P07/P08。
4. Native Mobile：P00-07 -> P08 iOS/Android 两条原生实现线。
5. Runtime/Model：P02/P03 -> P09/P10 -> Desktop/Mobile Task UI。
6. Platform：P00 -> P12，持续跟随 7 个服务端工作负载和五个平台制品。
7. Quality：P01/P02/P00-03 后启动 P13，不允许等到 Feature Complete 才开始。

### 8.2 关键路径

```text
Scope/Threat Model
  -> Crypto ADR + FFI/E2EE Interop Gate
  -> Proto + Ciphertext Envelope Contract
  -> Message Sequencer + Epoch-aware Sync
  -> E2EE Client Core + Encrypted locald + Outbox
  -> Desktop First Product Vertical
  -> Native iOS/Android Messaging
  -> Capability + Runtime + Approval
  -> Recovery Control + Private Deployment + Upgrade
  -> Five-platform E2E + Load/Chaos/Crypto Review/Pentest
  -> Two Enterprise Pilots
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
- E2EE 任务包含跨端 Golden Vector、离线设备、成员变更、密钥撤销和恢复失败路径。
- Recovery 任务证明 IM Server、Model Control、Agent 和模型 Endpoint 无法调用企业恢复私钥。
- API/Proto/Schema/Runbook/用户说明同步更新。
- 性能预算、内存预算、磁盘预算和无障碍标准通过。
- Code Review 和安全 Review 完成，没有未接受的 Critical/High 风险。

## 10. 排期变化规则

以下变更必须重新排期，不能吸收到普通迭代：

| 变更 | 预计增加 |
| --- | ---: |
| 更换 Group E2EE 协议或生产密码库 | 10-18 周，并重新外部评审 |
| 更换 Rust Native Bridge 或放弃 Shared Core | 8-14 周 |
| v1 加入 SAML + SCIM | 5-8 周 |
| v1 加入 Enterprise Runner | 6-10 周 |
| v1 加入完整 Browser IM Client | 8-14 周，并新增浏览器密钥安全评审 |
| v1 加入服务端内容搜索/DLP/摘要 | 与 E2EE 冲突，必须重开 Scope 和 Threat Model |
| v1 加入音视频 | 至少 20-30 周，建议独立项目 |
| v1 要求跨 Region Active-Active | 至少 16-24 周，建议独立架构阶段 |

## 11. 人数变化对应日历时间

| 技术团队 | 合理 GA 周期 | 说明 |
| --- | --- | --- |
| 12 人 | 64-66 周 | 本文基准；七条工程线部分并行，含两轮 Pilot |
| 8 人 | 90-100 周 | Native Mobile、Crypto、Runtime 和 SRE 并行度下降 |
| 5 人 | 140-155 周 | 只能分阶段发布，安全、双原生端和平台仍不可省略 |
| 3 人 | 230 周以上 | 只能做受限技术 Pilot，不适合承诺企业 GA |

编码 Agent 可以减少样板代码、测试生成和文档时间，但不能替代移动真机验证、加密评审、故障演练、
企业 Pilot 和发布责任，因此不单独从 GA 基准中扣减安全储备。

## 12. 第一批必须建立的工作项

启动时先创建以下 Epic，禁止直接从 UI 页面开始散点编码：

1. `EPIC-000 Product Scope and Acceptance`
2. `EPIC-010 Threat Model and Data Classification`
3. `EPIC-020 Monorepo and Build Reproducibility`
4. `EPIC-030 Protocol v1 and Compatibility`
5. `EPIC-040 Client Platform, Native Bridge and Crypto Gate`
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

- Tauri 2 用于 Desktop，并允许通过 Rust Command/Plugin 接入本地能力：https://v2.tauri.app/
- Tauri 官方建议 SPA 使用 Vite，并将 Desktop 应用与 API 保持明确 Client-Server 关系：
  https://v2.tauri.app/start/frontend/
- Element X 的公开工程采用 SwiftUI iOS、Jetpack Compose Android 与共享 Matrix Rust SDK，证明
  “原生 UI + Shared Rust Core”是实时 E2EE 客户端的可行参考：
  https://github.com/element-hq/element-x-ios 和 https://github.com/element-hq/element-x-android
- MLS 标准定义群组 Epoch、成员变更和前向安全语义；是否采用 MLS 及企业恢复扩展仍由 Crypto ADR
  决定，不能只凭标准名称跳过实现库审查：https://www.rfc-editor.org/rfc/rfc9420
- ConnectRPC 以 Protobuf 定义浏览器与 gRPC 兼容 API，并提供 Go、TypeScript、Swift、Kotlin、Dart
  等客户端：https://connectrpc.com/
- Buf CLI 支持本地代码生成、Lint 和 Breaking Change 检测，可在 CI 中对 Git 基线执行，不要求使用
  公网 Registry：https://buf.build/docs/breaking/

## 14. Agent 异步实施规则

本文中的 `Pxx-xx` 是排期工作包，不直接等于一个 Agent Task。执行前必须按照
[`agent-workstreams.md`](./agent-workstreams.md) 拆成 0.5 至 2 Agent 日的独立 Issue，并明确：

- 唯一 Workstream 和路径 Owner。
- 输入 Contract、Base Commit 和阻塞依赖。
- 可单独运行的验收命令。
- 是否修改 Proto、Migration、Generated Code 或 Lockfile。
- 完成后的 Commit、Handoff 和需要解锁的后继任务。

不同 Agent 使用独立 Git Worktree；跨工程依赖通过 Contract、Fake 和 Fixture 解耦；`main` 只由
Integration Owner 通过 Review/Merge Queue 更新。任何需要两个 Agent 同时修改同一文件的拆分都应视为
无效拆分并重新划分边界。
