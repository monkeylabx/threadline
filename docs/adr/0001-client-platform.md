# ADR-0001：Desktop、原生 Mobile 与 Shared Rust Client Core

- 状态：Accepted，受 T010 Native Bridge Spike 验证门约束
- 日期：2026-08-05
- 决策者：Product / Architecture
- 关联 Issue：[#17 T003](https://github.com/monkeylabx/threadline/issues/17)
- 产品基线：[Frozen Scope 1.1](../acceptance/scope.md)

## 背景

Threadline Private Enterprise v1.0 同时交付 macOS、Windows、Linux、iOS 和 Android 客户端。客户端必须在服务端只持有密文的前提下完成 E2EE、加密本地存储、离线 Outbox、同步归并、本地搜索和附件加密。Desktop 还承载本地 Agent Runtime 与 Workspace 访问；Mobile 只负责沟通、发起和观察 Task、中断 Run 与审批，不执行长任务，也不运行 `agentd` 或 `connectord`。

Frozen Scope 1.1 已接受以下产品选择，本 ADR 不重新比较跨端 Web UI 与原生 UI：Desktop 使用 Tauri 2、React 和 TypeScript；iOS 使用 Swift/SwiftUI，必要时使用 UIKit；Android 使用 Kotlin 和 Jetpack Compose；三端共享版本化 Rust Client Core，但不共享页面代码。

本 ADR 决定这些客户端的进程、模块、数据、并发、Bridge、打包和迁移边界。具体密码协议和 Recovery Envelope 由 Crypto ADR 负责，公开消息与同步字段由 Protobuf Contract 负责。

## 决策驱动因素

- E2EE、同步和本地事实在三个客户端实现中必须具有相同语义，不能形成三套安全实现。
- iOS 和 Android 必须保留平台原生的生命周期、后台、通知、密钥存储、文件与无障碍能力。
- Desktop 必须隔离 IM 本地事实、本地 Agent Runtime 和任意 Workspace 文件访问。
- FFI 必须明确版本、错误、取消、流、线程和内存所有权，失败可被 Contract/Fault Test 复现。
- 五个平台需要独立签名、升级、回滚和兼容窗口，不能要求同日升级。
- IM 在 Runtime、Connector 或模型离线时仍必须独立工作。

## 决策

### 1. 平台与模块边界

| 表面 | 实现 | 拥有的职责 | 禁止承担 |
| --- | --- | --- | --- |
| Desktop UI | Tauri 2 + React + TypeScript | 窗口、导航、渲染、用户意图、Runtime 状态展示 | 直接写 SQLite、持有 Channel Key、直接访问任意 Workspace |
| Desktop `locald` | Rust sidecar | Client Core Actor、加密 SQLite、Outbox、Sync、Search、附件加密、本地 IPC | Agent Loop、模型调用、任意 Workspace 访问 |
| Desktop `agentd` | 独立本地进程 | Agent Loop、Run Session、取消、有限 Context 消费 | 读取 IM 数据库、使用恢复私钥 |
| Desktop `connectord` | 独立本地进程 | 基于短期 Path Grant 的 Workspace 操作 | 浏览未授权路径、读取 IM 数据库 |
| iOS Host | Swift/SwiftUI/UIKit | 原生 UI、生命周期、APNs、Keychain、文件、后台恢复 | `agentd`、`connectord`、长任务执行 |
| Android Host | Kotlin/Compose | 原生 UI、生命周期、FCM、Keystore、文件、后台恢复 | `agentd`、`connectord`、长任务执行 |
| Shared Rust Client Core | Rust | E2EE Adapter、加密 SQLite、Outbox、Cursor/Sync、Search、附件加密 | UI、平台凭据实现、密码协议发明、Agent Runtime |
| Native Bridge | 版本化窄 Facade | Command、事件流、取消、错误和稳定数据传输 | 暴露内部 Rust 类型、数据库句柄或密码库对象 |

Desktop UI 只能通过版本化本地 IPC 调用 `locald`。`agentd` 获取消息上下文时必须使用受 Capability Grant 限制的 Context API，不能挂载或查询 Client SQLite。`connectord` 只消费明确路径、动作和期限的 Path Grant。三者崩溃和升级相互隔离；`locald` 不依赖 Runtime 才能完成消息同步。

iOS 和 Android 在应用进程中各自持有一个 Client Core Actor。每台设备拥有独立数据库、设备身份和缓存；禁止在设备间复制 SQLite 文件。

### 2. 数据所有权

Client Core 是以下本地事实的唯一 writer：加密 SQLite、Durable Outbox、同步 Cursor、消息物化、E2EE 状态、本地搜索索引和加密附件缓存元数据。Host 只持有可丢弃的 View State、导航状态和由 Core 返回的不可变 DTO。

平台安全存储分别由 Keychain、Android Keystore 和 Desktop OS Credential Store Adapter 实现。安全存储只保存设备身份或数据库密钥的包装材料；Channel Key、消息明文、Prompt、Token 和恢复私钥不得进入普通配置、日志、崩溃报告或诊断包。Enterprise Recovery 私钥只存在于企业 KMS/HSM，不属于任何 Client API。

### 3. 并发和线程边界

每个设备只运行一个逻辑 Client Core Actor，并由它串行化数据库写入、Epoch 变更、Outbox 状态转换和 Cursor 提交。网络、密码运算、索引和附件 I/O 可以在受控 worker 上执行，但结果必须回到 Actor 后才能改变本地事实。

FFI 调用不得阻塞 Swift MainActor 或 Android Main Dispatcher。Host 在 UI 线程发起 Command，Bridge 立即返回 Request Handle；结果和事件通过平台异步机制投递。回调进入 Host 前切换到 Host 指定的 Executor/Dispatcher，Core 不假设回调线程。

同一 `client_instance_id` 的事件具有单调 `event_seq`。事件流断开、Host 进入后台或消费落后时，Host 使用 Cursor 从 Core 恢复；事件通知不是事实源。Core 对有界队列实施背压，低价值状态通知可以合并，但消息、审批、错误和最终状态不能静默丢弃。

### 4. 版本化 Native Bridge

Bridge 以一个显式 ABI major 和一个可查询的 capability set 开始握手。Host 在创建 Client 前比较支持范围；major 不兼容时拒绝启动数据路径并显示可恢复的升级错误，不能尝试部分调用。

Bridge 版本采用 `major.minor`：major 只在删除、重命名或改变既有语义等破坏性变更时递增；minor 只允许新增可选 Command、字段、错误 code 或 capability。Host 必须忽略未知 Envelope 字段；缺少必需 capability 时拒绝对应功能并返回稳定的 `unsupported_capability`，缺少可选 capability 时使用文档化降级路径，不能猜测调用。每个 Core release 声明支持的 Host major、最低 Host minor 和 capability set；每个 Host release 声明可消费的 Core major/minor 范围。弃用项至少保留一个 N-1 Host 发布窗口，并由 Contract Test 同时验证当前与 N-1 组合后才能删除。

公开 Facade 只包含：

- 使用稳定标量、字节缓冲和带版本 Envelope 的 Command/Response。
- `request_id`、`stream_id`、`client_instance_id` 和单调 `event_seq`。
- `cancel(request_id)` 与幂等 `close(stream_id)`。
- 结构化错误 Envelope：稳定 code、retryability、safe user message key、可选 redacted diagnostic id。
- 明确的 create/retain/release 或语言绑定生成的等价生命周期；不跨 FFI 暴露 Rust 引用。

取消是尽力而为但结果确定：尚未提交的操作返回 `cancelled`；已经跨过持久化提交点的操作返回其真实最终状态，不能伪报取消。Host 释放 Request、Stream 或 Client 后，Core 不再回调该对象。重复取消和关闭必须安全。

Rust panic 不得跨越 FFI。Bridge 将可恢复失败映射为稳定错误；不可恢复 panic 终止当前 Client Instance，保留崩溃安全的数据库恢复标记，并在下次启动执行完整性检查。错误和诊断不得包含消息正文、密钥、Prompt、Token 或用户文件内容。

### 5. 平台绑定与发布

- iOS：Rust 产物和生成绑定由版本化 SwiftPM binary target 消费；签名、Keychain entitlement、后台模式和企业分发属于 iOS Host。
- Android：Rust `.so` 与生成 Kotlin/JNI 绑定由 Android Gradle module 消费；ABI splits、Keystore、后台限制和企业分发属于 Android Host。
- Desktop：Tauri shell 与 `locald`、`agentd`、`connectord` 使用独立版本 Manifest 打包。Tauri 权限 Manifest 只授予启动和访问所需 IPC 的最小权限。

以上是 T010 需要验证的发布边界，不冻结具体绑定生成器或二进制容器格式；T010 可以在不改变 Facade Contract 的前提下选择更可靠的 SwiftPM/Gradle 集成方式。

生成绑定、FFI headers、Rust Core library 和 Fault Fixture 必须来自同一 Core release。禁止 Host 手改生成文件。制品必须固定版本、校验和并进入各平台签名/SBOM 流程。

Host 与 Core 支持 N-1 兼容窗口。升级顺序为：先验证数据库迁移和 Bridge compatibility，再原子替换 Core/Sidecar，最后启动新 Host。失败时回滚二进制；若已执行不可逆数据库迁移，则必须由迁移策略提供前向修复或恢复副本，不能让旧二进制打开未知 schema。

### 6. 五平台能力差异

| 能力 | Desktop：macOS/Windows/Linux | iOS | Android |
| --- | --- | --- | --- |
| 完整 IM、E2EE、Outbox、Sync、Search | 是，通过 `locald` | 是，进程内 Core | 是，进程内 Core |
| Agent 长任务 | 是，通过授权 `agentd` | 否 | 否 |
| 任意 Workspace 操作 | 仅经 `connectord` Path Grant | 否 | 否 |
| Task 发起、观察、中断、审批 | 是 | 是 | 是 |
| 密钥包装 | OS Credential Store | Keychain | Keystore |
| 后台执行 | Sidecar 生命周期 | 受 iOS Background Task 限制 | 受 Android 后台/WorkManager 限制 |
| Push | Desktop 通知/内网连接 | APNs；Air-gap 明确降级 | FCM；Air-gap 明确降级 |
| UI 代码 | React/TypeScript | SwiftUI/UIKit | Compose |

平台后台限制只影响连接和恢复时机，不改变 Durable Outbox、ACK、Cursor 和 E2EE 状态语义。

### 7. 五平台构建和原生发布差异

| 平台 | 构建/制品边界 | 签名与系统集成 | 本地安全存储和生命周期 |
| --- | --- | --- | --- |
| macOS | Tauri `.app`/安装制品 + 同版本 sidecars | Developer ID、Notarization、Keychain、通知和登录项权限 | Keychain adapter；由 app/sidecar supervisor 管理退出与恢复 |
| Windows | Tauri Windows 安装制品 + 同版本 sidecars | Authenticode、安装/卸载、通知与防火墙提示 | Credential Manager/DPAPI adapter；处理会话退出和服务终止 |
| Linux | Tauri 发行制品 + 同版本 sidecars；按支持发行版验证 WebView/system library | 仓库/制品签名、桌面入口与通知集成 | Secret Service adapter；处理桌面会话和进程 supervisor 差异 |
| iOS | Xcode Host + SwiftPM 消费的 Core/binding 制品 | Apple signing、entitlement、APNs、Background Task、企业/MDM 分发 | Keychain adapter；遵守 foreground/background/termination 生命周期 |
| Android | Gradle Host + JNI/Kotlin binding + ABI 对应 `.so` | Android signing、FCM、WorkManager、企业/MDM 分发 | Keystore adapter；遵守 Activity/Process 和后台执行限制 |

五个平台分别产生和签名制品，但必须消费同一 Core release manifest、Facade Contract 和 Fault Fixture。平台 adapter 只能实现安全存储、通知、后台调度和生命周期接口，不得改变 E2EE、Outbox、Sync 或错误语义。

## 备选方案

### 全平台共享 Web UI

拒绝。它与 Frozen Scope 1.1 的原生 Mobile 决策冲突，并削弱后台、通知、密钥存储、文件和无障碍集成。本 ADR不重新打开该产品决策。

### Swift、Kotlin 和 Desktop 分别实现 Client Core

拒绝。三套 E2EE、Outbox 和同步状态机会放大安全审计与兼容成本，并使跨端 Golden Vector 不能覆盖同一实现。代价是 Rust Bridge 成为关键风险，因此必须由 T010 真机 Spike 提前验证。

### 在 Mobile 运行 `locald` sidecar

拒绝。移动平台不提供与 Desktop 等价的常驻 sidecar 生命周期。Mobile 使用进程内 Actor，但消费相同版本化 Core Facade 和 Fault Fixture。

### 将 Agent Runtime 合入 Client Core

拒绝。它会让 IM 可用性依赖模型和 Workspace 执行，并扩大明文、文件与密钥信任边界。Runtime 必须是 Desktop 独立进程且通过受限 API 获取上下文。

## 代价与风险

- Rust FFI、生成绑定和跨平台发布链增加工程复杂度，需要真机、内存、取消、Fault 和 Crash/Resume 测试。
- Desktop 采用 sidecar、Mobile 采用进程内 Actor，部署形态不同；必须用同一 Facade Contract 验证语义而不是假设进程模型一致。
- 原生 UI 形成三套页面实现，但平台行为清晰，且页面不复制密码、存储和同步逻辑。
- N-1、数据库迁移和独立签名要求增加发布成本，但避免强制五个平台同步升级。
- Mobile 后台限制会延迟同步和 Task 状态刷新；最终一致性由 Durable Local State 和恢复协议保证。

## 重新评审和迁移边界

出现以下任一条件时必须停止相关实现，把本 ADR 状态改为 `Rejected by validation`，并用替代 ADR 重新决策：

- T010 在 iOS 与 Android 真机上无法证明无 use-after-free、无 UI 主线程阻塞，并且取消、事件流、错误映射或 Crash/Resume 无法形成确定 Contract。
- Bridge 的稳定性或性能无法满足消息列表、同步和加密附件基线，且通过批处理、有界流或绑定生成仍无法修复。
- Shared Core 需要暴露内部数据库、密码库对象或平台 UI 类型才能工作。
- 需要更换 Bridge 技术、放弃 Shared Rust Core、改变 Desktop/Mobile 进程模型或共享页面代码。
- 任何实现允许 Server、Runtime、Model Control 或 Host UI 绕过 Client Core 读取密钥/数据库，或允许 Mobile 执行长任务。

若 Bridge 失败，允许评估更窄的 C ABI、UniFFI 等生成方案，或缩小 Shared Core 边界；不得静默复制 E2EE 协议或退化为服务端明文。更换 Bridge 预计触发交付计划中的 8–14 周重新排期，并重新执行 Threat Model、兼容测试和平台发布评审。

## 验证门

本 ADR 的 Accepted 表示架构方向被接受，不代表 Native Bridge 已证明可生产使用。T010 必须在 iOS 与 Android 至少各一台真机验证：

- Facade 版本握手、Command/Response、事件流和背压。
- 取消提交点、重复 cancel/close 和 Host 释放后的零回调。
- 错误映射、panic 隔离、Crash/Resume、数据库恢复和资源释放。
- UI 主线程不阻塞，内存增长有界，敏感内容不进入日志或诊断。

T010 通过后，本 ADR 状态更新为 `Accepted / Validated`；失败时更新为 `Rejected by validation`，并在新 ADR 中记录替代 Bridge 或缩小后的 Shared Core 边界。替代 ADR 合入前不得继续依赖本决策实施 Native Bridge。
