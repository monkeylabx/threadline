# ADR-0001: Desktop、原生 Mobile 与 Shared Rust Client Core

- 状态：Accepted
- 日期：2026-07-25
- 决策者：Threadline Architecture Workstream
- 关联：Issue #17、Epic #5、`docs/acceptance/scope.md` D-007

## 背景

Threadline v1 必须交付 macOS、Windows、Linux、iOS 和 Android 五个平台。客户端既要提供符合各平台习惯的消息、文件、通知、后台和企业分发体验，也要在离线 Outbox、Cursor Sync、加密 SQLite、搜索和 E2EE 状态上保持同一套可验证语义。

产品范围已经冻结为 Tauri Desktop、原生 iOS/Android UI 和 Shared Rust Client Core。本 ADR 不重新评估“跨端 Web UI 还是原生 UI”，而是明确该决策下的进程、模块、数据、并发、Bridge 和发布边界。

客户端必须同时遵守以下信任边界：

- IM 在模型和 Agent Runtime 离线时仍然可用。
- Desktop UI、Mobile UI 不直接写消息数据库。
- Agent Runtime 不挂载或读取 IM 数据库，只能通过显式 Context API 获取被授权的有限明文。
- 本地 Workspace 只能通过用户授权的 Connector 访问。
- Mobile 可以发起、观察、中断和审批 Task，但不运行长任务。

## 决策驱动因素

- 五个平台上的消息事实、离线恢复和密码状态必须一致且可做 Contract Test。
- 消息列表、系统导航、输入法、通知、后台调度、安全存储和企业分发需要原生平台能力。
- 任何一个 UI 进程崩溃或窗口并发都不能破坏本地消息状态。
- Swift/Kotlin 不应依赖 Rust 内部类型、线程模型或密码库 API。
- Desktop 的 IM、Agent Runtime 和 Workspace 权限必须是可独立故障、升级和审计的边界。
- Bridge 和 Shared Core 是高风险、难逆转决策，必须具备明确的 M0 验证和退出条件。

## 决策

### 1. 平台与页面层

| 平台 | 页面与宿主 | Shared Core 接入 | 平台专属能力 |
| --- | --- | --- | --- |
| macOS | Tauri 2 + React + TypeScript + Vite | 通过版本化 Desktop IPC 访问 `locald` | Keychain、通知、文件选择器、签名/公证、Universal Link |
| Windows | Tauri 2 + React + TypeScript + Vite | 通过版本化 Desktop IPC 访问 `locald` | Credential Manager/DPAPI、Toast、文件选择器、MSIX/企业签名 |
| Linux | Tauri 2 + React + TypeScript + Vite | 通过版本化 Desktop IPC 访问 `locald` | Secret Service、桌面通知、Portal、发行版制品与签名 |
| iOS | Swift + SwiftUI，复杂列表可局部使用 UIKit | 进程内版本化 Rust FFI Facade | Keychain/Secure Enclave、APNs、Background Tasks、File Provider、企业分发 |
| Android | Kotlin + Jetpack Compose | 进程内版本化 Rust FFI Facade | Keystore、FCM、WorkManager、Storage Access Framework、企业分发 |

页面代码不跨 Desktop、iOS 和 Android 共享。三端共享协议契约、领域语义、测试向量和 Rust Core；视觉 Token 可由各平台生成或消费，但原生交互和无障碍实现留在各自页面层。

Desktop 是同一套 Tauri 前端的三个编译目标，不允许用平台条件分支改变消息或权限语义。平台条件代码只处理 OS Adapter、窗口、更新、通知、安全存储和文件选择等宿主差异。

### 2. 模块、进程与信任边界

#### Desktop

```text
Tauri WebView / React UI
        |
        | versioned local IPC; commands, queries, event stream
        v
locald (Rust, single writer)
  |- client-core actors
  |- encrypted SQLite / local search / blob cache
  |- server Connect RPC + realtime WSS
  `- authorization-checked Context API

agentd (separate process) <---- runtime stream ----> Runtime Gateway
  |                                      |
  | capability-scoped Context API        `- route grant only
  v
locald                         approved model endpoint
  ^                                      ^
  | capability-scoped tool calls         | prompt sent directly
connectord (separate process) <----------- agentd
  `- user-authorized workspace roots and audit log
```

- `locald` 是 Desktop 本地消息事实的唯一写者。UI 多窗口只能通过 IPC 提交命令和订阅投影，不能打开 SQLite 文件。
- `agentd` 与 `connectord` 是独立进程、独立 OS Identity 和最小文件权限。二者不得访问消息数据库或其密钥。
- `agentd` 只能用短期 Capability 通过 `locald` Context API 请求有限消息上下文，通过 `connectord` 请求有限 Workspace 操作。
- `locald`、`agentd`、`connectord` 可独立崩溃和重启。`agentd`/`connectord` 不可用不得阻塞普通 IM、Outbox 或 Sync。
- Tauri Rust 宿主只负责窗口生命周期、Sidecar 启停、IPC 权限和 OS Adapter，不复制 `locald` 的消息状态机。

#### Mobile

```text
SwiftUI/UIKit or Compose UI
        |
        | generated language binding
        v
versioned client-ffi Facade
        |
        v
client-core actor runtime (in process)
  |- encrypted SQLite (single writer)
  |- Outbox / Sync / Search / E2EE state
  `- OS adapters supplied by the host
```

- 每个 Mobile App 进程内只创建一个 Core Runtime；它拥有数据库连接、Actor 调度器和网络会话。
- UI 通过 Facade 提交命令、执行分页查询并订阅事件，不持有 SQLite、密码库或网络客户端对象。
- iOS Extension、Android 辅助进程或 Widget 不直接打开主数据库。它们通过受限 App Group/平台 IPC 交付输入，主 Runtime 再导入；共享数据库文件不作为进程间协议。
- Mobile 不打包、不启动 `agentd` 或 `connectord`，不直接执行长任务，也不接受 Workspace Path Grant。
- Mobile 的 Task 操作是发送到服务端的控制命令；执行所有权属于已注册且获授权的 Desktop/Workstation Runtime。

### 3. 数据所有权与 Actor 模型

`client-core` 共享下列语义和实现：E2EE 状态适配、加密 SQLite Schema 与 Migration、Durable Local Outbox、ACK Merge、Cursor/Gap Repair、事件物化、本地权限复检、Search 和附件加密/缓存。密码协议算法由独立的 reviewed adapter 提供，不在 FFI 或 UI 中实现。

Core Runtime 使用显式 Actor 边界：

- `StoreActor` 独占 SQLite 写连接并串行执行事务；读取使用受控快照，不把连接或行引用暴露出 Core。
- `SyncActor` 拥有 Cursor、重连和补洞状态，只通过 Store 命令提交持久化变化。
- `CryptoActor` 拥有密码会话与敏感内存，只接收值对象和不透明标识；密钥不进入 UI、日志或跨语言异常。
- `OutboxActor` 负责重试、幂等键和 ACK 归并，UI 只能观察 Pending/Committed/Failed 投影。
- `SearchActor` 只查询本机已授权且已解密的索引；撤权触发删除或重建。

Actor Handle 绑定一个 Runtime 实例。Host 可以从任意 UI 线程发起异步调用，但 Core 在内部调度；回调只在 Host 注册的 Dispatcher/Executor 上投递。不得假设 Rust 执行线程就是 Main Thread。

### 4. 版本化 Native Bridge

`client-ffi` 是 Swift 和 Kotlin 唯一可见的 Rust API。Desktop 不调用该 ABI，而使用语义等价、版本化的 `locald` IPC。两种边界必须运行同一套 Core Contract Fixture。

#### 版本和兼容

- Facade 使用独立的 `major.minor` Bridge Contract Version，并在启动时协商 `min_supported`、`max_supported` 和 feature flags。
- 同一 App 制品固定绑定一个经过测试的 Rust Core 版本；不支持从网络单独热替换动态库。
- Minor 版本只能新增可选字段、枚举 unknown fallback 或能力；删除、改义、所有权变化和必填字段属于 Major 变更。
- Swift/Kotlin 生成 Binding，不手写镜像 Rust Struct；生成器版本、Schema Hash 和 Core Build ID 写入诊断信息。
- 持久化 Schema 和 Wire Protocol 各自版本化，不以 Bridge 版本替代 Migration 或协议兼容策略。

#### 值、错误与未知数据

- 跨边界只传固定宽度标量、UTF-8/byte buffer、不可变值对象和不透明 Handle；不暴露 Rust 泛型、Borrow、Trait、Pointer 或内部枚举布局。
- 所有调用返回稳定错误 Envelope：`domain`、`code`、`retryable`、安全的用户提示键、可选 trace ID。Swift/Kotlin 映射为受控错误类型。
- 未知错误码必须映射为 `unknown`，不得导致 Binding 崩溃；错误文本不得包含消息正文、Prompt、Token、Key、路径内容或底层 SQL。
- Panic 不得穿越 FFI。Facade 捕获边界故障、使相关 Handle 失效并返回稳定的内部错误；debug build 可额外终止以暴露缺陷。

#### 取消、流与背压

- 每个长调用返回 `OperationHandle`；Host 的取消是幂等请求，不承诺撤销已提交事务。最终完成事件明确区分 `cancelled_before_commit`、`completed` 和 `failed`。
- Host 页面销毁时必须取消订阅和不再需要的操作；Runtime Shutdown 会取消所有未提交操作并等待有界 drain。
- 事件流使用有界缓冲和显式 demand/ack。可合并的 Presence/Typing/进度事件允许 coalesce；Message、ACK、Approval 等可靠事件不能静默丢弃，消费者落后时关闭流并要求按 Cursor 重订阅。
- 每条 Stream Event 带单调序号或业务 Cursor；断流恢复依赖持久化 Cursor/重新查询，而不是无限内存队列。

#### 内存所有权

- 创建方释放：Core 返回的 Buffer/Handle 只能由配套的 Facade release 函数释放；Swift/Kotlin Wrapper 负责 exactly-once close，并提供自动生命周期兜底。
- 传入数据在同步调用返回前复制或完成消费；若异步保留，Facade 必须复制，绝不借用 Host 临时内存。
- 回调只携带在回调期间有效的不可变值，Binding 在交给应用层前转换为语言所有值。
- Handle 包含 generation，释放、Runtime 重启或版本不匹配后继续使用必须返回 `invalid_handle`，不能 use-after-free。
- Secret Buffer 使用专用类型、尽早清零且不实现调试打印；跨边界的普通 DTO 不携带原始密钥。

### 5. 构建、打包与发布边界

#### SwiftPM

- `client-ffi` 产出版本固定的 XCFramework，覆盖项目支持的 iOS device/simulator 架构；Swift Package 只公开生成的 Facade Wrapper 和资源清单。
- App、Extension 和测试 Host 显式链接同一 XCFramework 版本。Code Signing、Entitlement、Privacy Manifest、最低系统版本和 dSYM 由 iOS 发布流水线验证。
- Core Schema Migration 随 App 版本发布；失败必须保留旧库并进入可诊断的只读/恢复状态，不能由 App Store 回滚假设兜底。

#### Gradle

- `client-ffi` 产出版本固定的 AAR，包含受支持 ABI 的 `.so`、生成 Kotlin Facade、consumer rules 和 native symbols 映射。
- 构建按 ABI 检查缺失和重复库；R8/ProGuard 不得移除 JNI 入口。签名、最低 SDK、target SDK、native debug symbols 和 Play/企业分发元数据由 Android 发布流水线验证。
- Core Schema Migration 随 App 版本发布，Android 进程回收和后台限制纳入恢复测试。

#### Tauri Sidecar

- `locald`、`agentd`、`connectord` 按 `{product version, target triple}` 与 Desktop App 一起签名和发布，不从网络独立下载可执行代码。
- Tauri Capability/Permission Manifest 只允许访问明确 IPC 命令和 Sidecar；UI 不能传任意可执行路径、数据库路径或 Connector Root。
- 启动时校验 Sidecar 签名/哈希和 IPC Contract 范围；版本不兼容时普通 IM 优先以安全降级模式启动，Agent/Connector 单独标为不可用。
- Auto Update 必须把 App、Sidecar、Schema Migration 和回滚元数据视为一个发布单元。不得在数据库已不可逆迁移后仅回滚 UI。

#### 五平台发布门

每个候选版本至少验证：可重复构建、Binding/IPC Contract、安装/升级/失败迁移、Crash/Resume、离线 Outbox、后台/进程回收、安全存储、通知/深链接、符号化诊断和制品签名。macOS、Windows、Linux 分别验证 Sidecar 权限；iOS、Android 分别在真机验证 FFI 的调用、流、取消、错误和内存压力。

## 替代方案

### A. 全平台共享 Web UI（包括 Mobile）

不采用。它能提高页面代码复用率，但会把复杂消息列表、输入法、系统返回/手势、后台、Push、安全存储、文件和企业分发差异集中到 Web 容器适配层，并削弱 iOS/Android 的原生性能与无障碍体验。此选项也会重新打开已冻结的产品决策。

### B. Tauri 2 同时承载 Desktop 和 Mobile

不采用。单一框架看似减少工程数量，但 Mobile 仍需要大量平台插件和生命周期处理，且会把 M0 的 Native Bridge 与 UI 容器风险耦合。v1 保留 Tauri Desktop、原生 Mobile 的边界。

### C. 三端各自实现完整 Client Core

不采用。Swift、Kotlin、Rust 三套 Outbox、Sync、E2EE 和 Migration 会扩大协议漂移与安全审计面，难以证明离线和密码行为一致。原生 UI 带来的收益不要求复制状态机。

### D. Desktop 将 Core 静态链接进每个 Tauri 窗口

不采用。多窗口会形成多个数据库写者和重复网络会话，UI 崩溃也会扩大到消息事实。独立 `locald` 提供稳定的单写者与故障隔离。

### E. Mobile 通过本机守护进程访问 Core

不采用。iOS 不提供可靠的常驻 Sidecar 模型，Android 后台限制也会使其脆弱。Mobile 使用进程内 Actor，并通过 Durable Storage 在进程重启后恢复。

## 代价与后果

### 正面后果

- 高风险的同步、密码、存储和恢复逻辑只有一套实现与测试语义。
- iOS/Android 保留平台原生交互、性能、无障碍和企业能力。
- Desktop 的消息、Agent 和 Workspace 权限边界可独立故障与审计。
- FFI Facade 隔离 Rust 内部重构，Swift/Kotlin 只依赖稳定 Contract。
- Mobile 明确不具备本地长任务与任意文件访问能力，减少权限面。

### 负面后果

- 团队必须维护 React、SwiftUI/UIKit、Compose 三套页面实现和五平台发布流水线。
- Rust FFI、生成 Binding、跨语言异步/内存测试形成额外工程成本。
- Desktop IPC 与 Mobile FFI 是两种宿主边界，需要共享 Fixture 防止语义漂移。
- 原生 Crash、符号化、Schema Migration 和后台恢复必须分别验证，不能只靠 Rust 单元测试。
- UI 功能到 Core Contract 的变化需要跨 Workstream 排期，短期开发速度可能低于直接调用内部 API。

## 迁移与实施边界

1. M0 先建立最小 `client-core`、`client-ffi` 和 Host Harness，只验证调用、错误、取消、可靠流、背压、Crash/Resume 与内存所有权；不在 Spike 中承诺完整 UI。
2. 同一 Core Contract Test 必须运行于 Rust、Swift 真机 Host、Kotlin 真机 Host 和 Desktop `locald` Harness。
3. M1 建立加密 SQLite、单写者、Migration、OS Secure Storage Adapter 和可重复的 XCFramework/AAR/Sidecar 构建。
4. M2 先由 Desktop 完成 Message -> Local Agent -> Approval -> Artifact 垂直切片；普通 IM 对 Agent 服务故障保持独立。
5. iOS/Android 先交付 Enrollment、消息和进程恢复，再接 Task 控制；不得为赶进度在 Mobile 引入 `agentd`/`connectord`。
6. Bridge Contract 的破坏性变化必须先作为 Client-core Contract Task 合入，再由 Swift/Kotlin 消费；UI Workstream 不直接修改 Rust FFI Public Facade。

数据库迁移必须支持至少一个发布窗口的兼容读取或明确的不可逆门禁。Bridge 迁移不能假设服务端、数据库和 App 同步升级；每层独立协商版本并提供安全失败状态。

## 重新评审与退出条件

以下任一条件满足时，本 ADR 退回 Proposed，阻断依赖该能力的 Gate，并由 Architecture、Client-core、Mobile、Security 和 Release Owner 共同重新评审：

- M0 真机 Spike 无法在 iOS 和 Android 稳定通过错误、取消、流、Crash/Resume 与内存压力 Contract Test。
- 发现无法通过 Facade 隔离的 Rust ABI、运行时、链接、App Store/Play/企业签名或许可证限制。
- FFI 在代表性 Timeline/Sync 负载下相对平台预算产生不可接受的延迟、内存或电量开销，且批处理/背压优化后仍不达标。
- Shared Core 无法满足任一平台的安全存储、后台恢复、数据库 Migration 或无障碍所需数据语义。
- `locald` Sidecar 无法在 macOS、Windows、Linux 的签名、沙箱、更新或最小权限模型下可靠运行。
- Shared Core 迫使 UI 获得数据库、原始密钥或内部密码状态访问权，或迫使 Mobile 运行长任务/Workspace Connector。
- Bridge Major 版本无法提供可操作的双版本迁移窗口，导致升级必须清库、丢 Outbox 或丢密码状态。

优先回退顺序是：更换 Binding 生成器或 Bridge 技术但保留 Facade Contract；其次按平台替换 Host Adapter；最后才评估放弃 Shared Core。放弃 Shared Core 或改用非原生 Mobile UI 属于 Scope、Threat Model、交付计划和外部安全评审的重大变更，不能由单个实现 Issue 决定。

## 验证要求

- ADR 文档检查：决策、替代方案、代价、迁移边界和重新评审条件完整。
- M0 技术验证：Swift/iOS 与 Kotlin/Android 真机调用、流、取消、错误、Crash/Resume 和内存测试。
- Desktop Harness：三个目标平台验证 Sidecar 启动、版本协商、单写者、Agent/Connector 故障隔离和签名/权限。
- Contract/Golden Fixture：Rust、Desktop IPC、Swift、Kotlin 对相同输入产生相同状态与稳定错误码。
- Release Gate：XCFramework、AAR、三个 Desktop target 制品可重复构建，升级/失败迁移/回滚路径有证据。

## 安全与可观测性约束

- FFI、IPC、Crash Report、Metric 和 Trace 不记录消息正文、Prompt、Token、Key 或用户文件内容。
- 诊断只记录稳定错误码、版本、Schema Hash、匿名化 Handle/Operation ID、耗时和队列水位。
- 任何新增 IPC/FFI 能力都作为授权决策评审；“本机调用”不等于可信调用。
- Secret Material 只存在于 OS Secure Storage 或 `CryptoActor` 的受控内存中，不通过普通 DTO、UI State 或日志传播。

