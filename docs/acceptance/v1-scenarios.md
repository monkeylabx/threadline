# Threadline Private Enterprise v1.0 端到端验收场景

状态：Frozen Scope 1.1 验收基线草案

Gate：G0 / M0 至 G7 / M7

Owner：Product / Architecture

Issue：[#16 T002](https://github.com/monkeylabx/threadline/issues/16)

范围基线：[Private Enterprise v1.0 Scope Freeze](./scope.md)

## 1. 目的与通过规则

本文把 v1 核心用户旅程、安全不变量和里程碑结果定义成可重复执行的验收场景。场景描述业务输入、状态和可观察结果，不绑定具体测试框架、云厂商或 CI 产品。

场景只有同时满足以下条件才算 `PASS`：

1. 在记录的 Commit、制品、配置和环境上执行全部主路径步骤。
2. 预期结果和列出的失败路径都通过；跳过或“计划后补”均为 `NOT RUN`。
3. 证据可由未参与实现的人复核，且不依赖其他 Worktree 的未提交状态。
4. 日志、Trace、截图、诊断包和报告经过内容与 Secret 扫描，不含 C3/C4 明文或凭据。
5. 对应 Gate 的 Product、Architecture、Security 和交付 Owner 完成签字；Critical/High 风险不得用产品接受代替关闭。

执行记录必须保留总尝试数、失败次数、原因和最终修复 Commit。失败时相关能力保持关闭或停留在前一 Gate，不得降级到服务端明文、扩大 Capability 或绕过审批。

本文定义验收，不代表当前 Gate 已通过。基线时 ADR-0003 仍为 `Proposed`，T011 已证明 OpenMLS `0.8.1` 不满足生产准入，Swift 真机运行和独立 RFC 9420 实现互操作证据也不完整；这些项目关闭前，M0 的 Crypto 签字栏必须保持未签。

## 2. 全局安全与产品不变量

任一断言失败，即使业务 UI 看似成功也判为 `FAIL`：

- IM、Control Plane、Realtime、Worker 和对象存储只接触 Ciphertext Envelope、加密 Blob 与必要 C2 Metadata，不接触消息、文件或 Artifact 明文。
- Model Control 只接收不含 Prompt 的路由需求并签发短期 Route Grant；Prompt 由本地 Runtime 组装后直达企业批准的模型 Endpoint。Runtime Gateway、Model Control 和普通遥测均不接触 Prompt。
- Runtime 不打开、不 Mount、不复制 IM SQLite；只凭短期 Capability Grant 从本机 Context API 取得明确引用和有界窗口。Agent Session 不是 Channel 历史副本。
- Connector 只访问用户在当前设备明确授权的真实路径、动作和时段；物理路径不上报服务端。
- Mobile 不接收 Workspace Path Grant，也不运行 `agentd` 或 `connectord`。
- 只有获权 Device 是 E2EE Group 的 Cryptographic Member。Agent、Service、IM Server、Model Control、Runtime Gateway 和 Recovery Control 不持有持续 Channel/Epoch Key。
- 企业恢复私钥只存在于隔离 KMS/HSM 权限域；应用进程、普通管理员、Agent、Runtime、Model Control 和模型 Endpoint 均不能取得私钥字节或通用解密能力。
- 普通消息不会触发模型。只有显式 Task、消息转任务或已批准规则可以触发 Agent；规则本身不授予持续读取 Channel 的权限。
- Runtime、Model、Recovery、NATS、Redis 或 Realtime 故障不能破坏普通 IM 的本地历史、Durable Outbox 或消息事实源。
- 目标 Desktop 离线时 Run 保持 `waiting_for_runtime` 或等价等待状态，不漂移到 Mobile、另一设备、Runtime Gateway 或 Server。
- 显式转交必须重新选择设备和 Workspace，并重新签发 Capability Grant、Lease 与 Fencing Token。
- 高影响动作执行前展示规范化目标、动作、影响、数据目的地、有效期和批准依据；批准与实际执行内容以不可变 Hash/ID 绑定，任何变更都要求重新审批。
- 未知 Crypto Profile、损坏或乱序 Commit、不完整恢复审批、权限状态不明和不合规模型出口均安全失败，不猜测、不静默降级。

## 3. 标准验收拓扑

除场景另有说明，使用下列合成数据拓扑：

| 对象 | 标准配置 |
| --- | --- |
| Tenant | `tenant-a`；另建 `tenant-b` 作为跨 Tenant 负向受众 |
| Human | Alice（发起者）、Bob（审查/审批者）、Carol（待加入或撤销成员） |
| Agent | `agent-code`，模式为 `manual` 或 `task_only`，有明确 Owner |
| Device | Alice Desktop、Bob Desktop、Carol Desktop、Alice iOS、Alice Android；每台独立 Device Identity 与本地 DB |
| Channel | `channel-a` 对应一个 E2EE Group；另建无成员关系的 `channel-b` |
| Workspace | Alice Desktop 将逻辑 `workspace-a` 映射到仅含合成文件的本地目录 |
| Model | 一个策略允许的测试 Endpoint；一个地域、身份或 Retention 不合规的拒绝 Endpoint |
| Recovery | 隔离 Recovery Control、测试 KMS/HSM、两名不同审批者和一台 Recovery Recipient Device |
| 私有栈 | 七个服务端工作负载及 PostgreSQL、NATS、Redis、对象存储、KMS/HSM、OTel Collector |

每次运行生成唯一 Canary，分别嵌入合成消息、文件、Prompt、Token 和 Key 测试值。Canary 原文只保存在受控输入中；扫描报告只记录类型、位置、命中数和不可逆摘要。

## 4. 平台职责与共同语义

| 能力 | Desktop（macOS / Windows / Linux） | iOS | Android | 共同验收语义 |
| --- | --- | --- | --- | --- |
| IM 与 E2EE | 完整；经 `locald` 单写者 | 完整；进程内 Rust Core | 完整；进程内 Rust Core | 相同 Ciphertext、Epoch、Outbox、Cursor、错误码和物化结果 |
| 本地存储 | 独立加密 SQLite；UI 不直写 | 独立加密 SQLite；Keychain | 独立加密 SQLite；Keystore | 不复制 SQLite；撤权、Retention 和恢复语义一致 |
| Runtime | 可运行独立 `agentd` | 不运行长任务 | 不运行长任务 | 执行所有权只在获权 Desktop |
| Workspace | `connectord` 路径级授权 | 不接收 Path Grant | 不接收 Path Grant | 只同步逻辑引用，物理路径不上报 |
| Task 控制 | 发起、观察、中断、审批、转交、交付 | 发起、观察、中断、审批 | 发起、观察、中断、审批 | Mobile 控制排队或现有 Run，不成为 Execution Owner |
| 生命周期 | Sidecar 独立崩溃和重启 | 进程回收后恢复 | 进程回收后恢复 | Cursor 连续、Pending 不丢、可靠事件不静默丢弃 |
| Push | 桌面通知是提示 | APNs 可选提示 | FCM 可选提示 | Push 不是事实源；丢 Push 后 Cursor 补齐 |
| 发布证据 | 三个 OS 分别验签和升级 | 真机、签名、企业分发 | 真机、ABI、签名、企业分发 | 模拟器或 Rust 单测不能替代真机 Gate |

三端消费同一协议、Golden Frame、Crypto Vector 和稳定错误语义，但页面、系统手势、输入法、通知、安全存储和发布制品分别验收。

## 5. 标准证据包

每个场景的证据包至少包含：

- 场景 ID、Gate、执行时间、执行者、复核者、结果和全部尝试记录。
- Repo Commit、制品 Digest、Proto/Bridge/Crypto Profile/Schema 版本、配置与 Policy Digest。
- 服务和客户端版本、OS 与设备型号；真机证据明确标注物理设备。
- 前置状态、故障注入时间线、操作步骤、预期与实际状态转换。
- 自动断言、必要的脱敏截图或录屏、审计 Event ID、Metric/Trace ID 和对象/Envelope Digest。
- Server、队列、对象存储、日志、Trace、Crash Dump、诊断包、共享卷与备份的 Canary 扫描结果。
- 所有失败、豁免、剩余风险、关联 Issue/ADR 和复跑条件。

证据包禁止包含消息、文件或 Artifact 明文、Prompt/Response、搜索词、完整本地路径、Authorization Header、Cookie、Token、私钥、Channel/Epoch/Content/History Key、Recovery Envelope 字节或可复用恢复输出。

需要证明数据相等时使用稳定测试 ID、长度桶、状态码和经批准的不可逆 Digest。Gate 签字必须引用证据包的不可变位置和 Digest；易变 Dashboard、口头确认或个人目录不构成证据。

## 6. 可执行验收场景

### AC-001 OIDC 登录、设备注册与撤销

**覆盖 Gate：** M0（Device Authority、批准链与 anti-ghost 基线）、M1、M6

**前置条件：** Tenant 已接入测试 OIDC；准备一个尚无 Cryptographic Member 的新 Tenant，以及 Alice 已有一台获权 Desktop 的既有 Tenant；准备未注册 Mobile、过期 OIDC Token、跨 Tenant Token、普通 OIDC 管理员身份、伪造批准链、被撤销 Device Credential，以及五平台各自可恢复到第二台测试硬件/虚拟硬件的受控备份或设备镜像。

**步骤：**

1. Alice 完成 OIDC PKCE 登录；记录 Session 的 issuer、audience 和用途，不记录 Token 值。
2. 在新 Tenant 中，由独立 Device Authority 或硬件保护的引导管理员批准首台 Device；确认 Control Plane、普通 OIDC 管理员和仅持登录 Session 的主体均不能单独创建首个 Cryptographic Member。
3. 在既有 Tenant 中，由现有获权 Device 端到端批准新设备 Enrollment；若现有设备全部丢失，只能改走独立、可审计的高影响恢复流程。两条路径都生成独立 Device Identity、Device Credential 和一次性 KeyPackage。
4. 让新设备加入当前 E2EE Group，验证批准链和追加式 Device/Key 日志；从两个已获权 Device 独立取得并比较已签名日志根与 inclusion/consistency proof，再向其中一个 Device 注入省略该新设备或插入未批准 Device 的 split-view 日志响应，确认根或 proof 不一致被检测、客户端 fail closed，且不产生 Ghost Device。
5. 撤销新设备，作废其 Credential 与未消费 KeyPackage，完成 Membership Change 和 successor Epoch。
6. 分别用 OIDC Token 代替 Device Credential、重复 KeyPackage、过期 Credential、跨 Tenant Token 和已撤销 Device 重放加入或发送请求。
7. 在每个平台生成包含应用数据但按 Policy 排除 Device Identity、Leaf 与 KeyPackage 私钥的备份/设备镜像，将其恢复到第二台硬件；尝试让原设备与恢复设备用同一旧 Credential 并行加入或发送。确认恢复设备不能取得或使用原设备不可导出私钥、旧 Credential 不能在第二硬件并行使用，并必须以新的平台证明和新 Device Identity 重新 Enrollment；若平台支持“同一 Device 恢复”，还必须验证平台证明、硬件绑定与 Policy 明确允许该路径。

**预期结果：** 登录会话、Device Credential 与 KeyPackage 用途分离；首台 Device 只能由独立 Device Authority 或硬件保护的引导管理员建立，后续 Device 只能由现有获权 Device 或独立高影响流程批准。只有具备完整批准链的新 Device 成为 Cryptographic Member。已获权 Device 对同一追加式 Device/Key 日志收敛到可验证的根，省略或插入 Device 的 split-view 被检测并阻断。备份或镜像不携带可用的 Device Identity、Leaf/KeyPackage 私钥；第二硬件不能复用旧 Credential 并行充当同一 Device，默认必须重新 Enrollment。撤销后 Device 不能进入 successor Epoch、发送新消息或获得 History Sharing；现有成员 IM 继续可用。

**失败路径：** IdP 不可用时，已登录设备保留符合 Policy 的本地 IM 能力但不能伪造续期。恶意 Control Plane、普通 OIDC 管理员、伪造或缺失批准链、Token/Device/Tenant/Profile 不匹配、重复消费、过期输入、备份/镜像克隆出的 Identity/Credential、原设备与第二硬件并行使用同一 Credential，以及日志根/proof 不一致均稳定拒绝且不创建 Ghost Device；不得把 split-view 当作最终一致性延迟静默接受，也不得把备份恢复静默视为已获权 Device。

**证据：** 首台与后续 Device 两套批准链、OIDC/Device Contract 负向报告、恶意 Control Plane/普通管理员/伪造链拒绝记录、追加式 Device/Key 日志根、inclusion/consistency proof 与 split-view 检测报告、Epoch 摘要、撤销前后访问矩阵、五平台安全存储 Attestation、备份排除清单、第二硬件恢复/并行使用拒绝记录、新 Enrollment 或获批同 Device 恢复的平台证明，以及 C3/C4 扫描。

### AC-002 Agent 与模型离线时的纯 IM

**覆盖 Gate：** M2、M6

**前置条件：** Alice 与 Bob 是 `channel-a` 成员；停止 Runtime Gateway、所有 `agentd`、Model Control 和模型 Endpoint，保留 IM Core、Sync 与 Ciphertext 存储。

**步骤：**

1. Alice 和 Bob 发送、接收、回复并在 Thread 中继续一组 E2EE 消息。
2. 上传一个经客户端扫描并加密的合成附件；Bob 下载、校验并在本地预览。
3. 两端执行本地消息和文件搜索并同步 Read Cursor。
4. 发送普通消息，确认没有创建 Task、Run 或模型调用。

**预期结果：** 普通 IM、文件、Thread、搜索与同步可用；Agent/模型仅显示独立能力不可用。Server 和对象存储只有密文与必要 Metadata，普通消息不触发模型。

**失败路径：** Runtime/Model 恢复前显式创建 Task 可以排队，但不能阻塞消息、伪造 Run 成功或把消息发送给服务端模型；任何故障不得引发明文降级。

**证据：** 停机时间线、IM Event/Cursor 状态、Task/模型零调用计数、Blob/DB/日志 Canary 扫描、本地搜索与附件校验摘要。

### AC-003 断网 Outbox、重复、顺序与 Gap Repair

**覆盖 Gate：** M2、M3、M6

**前置条件：** Alice Desktop/iOS/Android 与 Bob Desktop 在同一 Channel；记录各设备 Cursor，准备断网、进程终止和重复注入。

**步骤：**

1. 断开 Alice Desktop 网络，连续发送三条消息；确认先写 Durable Local Outbox 并显示 Pending。
2. 第二条写入后终止客户端或 `locald`，恢复进程和网络，以相同 Idempotency Key 重试每条至少 100 次。
3. 在接收端制造中间 `channel_seq` 缺口、重复 Event、乱序通知和 Realtime 重启，然后恢复连接。
4. iOS/Android 分别在飞行模式和进程回收后恢复；Air-gapped 模式关闭 Push 后重新打开 App。

**预期结果：** 每个逻辑 Event 只提交一次；Durable ACK 前 Pending 不删除，ACK 只在 Event、`channel_seq` 和 Transactional Outbox 同事务达到持久条件后返回。Cursor 不越过 Gap；最终设备按相同 Sequence 收敛且无丢失。

**失败路径：** PostgreSQL 未满足同步提交时不得假 ACK；NATS、Redis、Realtime 或 Push 丢失只造成提示延迟；损坏或跨 Tenant Event 被拒绝且 Cursor 不前进。

**证据：** Outbox 崩溃前后快照、100 次重试唯一 Event/ACK 统计、事务故障记录、各设备 Cursor、最终状态 Digest 和 Canary 扫描。

### AC-004 E2EE 成员变更、密钥轮换与版本安全失败

**覆盖 Gate：** M0、M2、M6

**前置条件：** 使用版本锁定的 `tl-mls-1` Protocol Harness；Alice、Bob 为当前 Device Leaf，Carol 离线；准备重复、损坏、未来 Epoch、跨 Group、分叉 Commit、未知 Crypto Profile 和旧状态快照；准备 library-independent 的 History Sharing、Device History Sharing 与 Recovery Envelope 语义向量及其损坏变体。

**步骤：**

1. 运行 RFC 9420 Known Answer、Threadline Golden Vector 和至少一个独立 RFC 实现的 Transcript 互操作。
2. 添加 Carol、执行自更新、撤销 Bob Device，并按服务端排序应用 Membership Change/Commit。
3. 撤销后立即发送消息，确认 Group 进入 `rekey_required`，消息只留 Local Outbox；由获权 Committer 建立 successor Epoch 后再提交。
4. 向各端注入重复、乱序、跨 Tenant/Group、分叉和损坏 Commit，回滚本地状态，并尝试未知 Profile、Cipher Suite 降级和 N-1 客户端同步。
5. 轮换 Recovery Key Version，确认只影响新 Epoch；旧 Retention 范围历史仍绑定原 Key Version，系统不批量重写既有消息。
6. 在不依赖候选密码库内部类型的 Harness 中，验证 History Sharing、Device History Sharing 与 Recovery Envelope 的外部语义和绑定；分别注入未获权或没有旧设备证明的历史请求、跨 Tenant/Group/Epoch/Profile/Key Version/Recipient 包装、未知版本、缺失或损坏 Wrapper，以及安全服务不可用。
7. 确认上述负向向量全部 fail closed，不释放历史密钥或恢复输出、不改变成员与 Epoch 状态，也不触发明文或服务端持钥降级。本场景只验 M0 协议语义与绑定；成功恢复、Retention 截止和完整隔离流程仍由 AC-010 验收。

**预期结果：** 五平台外部 Group/Epoch/Envelope 状态一致；Bob 不能读取 successor Epoch。History/Device History/Recovery 包装只在 Tenant、Group、Epoch、Profile、Key Version、Recipient 和授权证明全部匹配时可继续处理。错误输入以稳定错误安全失败且不形成长期分叉。未知 Profile 或 Envelope 版本不降级；仍支持 Group Profile 的 N-1 客户端可同步，不支持者只读或明确不兼容。

**失败路径：** 候选库出现 Panic、未豁免漏洞、独立互操作失败、持久状态回滚、History/Recovery 绑定负向向量失败、安全服务不可用时不安全失败或真机不通过时，Crypto 生产准入保持关闭；不得以 `catch_unwind`、服务端密钥或服务端明文替代修复。

**证据：** 精确依赖锁与 SBOM、漏洞和许可证审计、Golden Vector、独立实现 Transcript、五平台 Host Harness、Commit/Fork/Replay/回滚负向报告、library-independent 的 History/Device History/Recovery 语义向量与跨绑定/损坏/不可用负向报告、真机内存与 Crash/Resume 结果、Security Review。

### AC-005 E2EE 文件、断点续传、本地搜索与撤权

**覆盖 Gate：** M3、M5、M6

**前置条件：** Alice 有一个合成文件；本地 Scanner Adapter 已批准；Bob 有父 Channel 权限，Carol 无权限；准备中断上传、Scanner 超时或崩溃、Blob 损坏、缓存或索引损坏和 ACL 并发撤销。

**步骤：**

1. Scanner 在加密前扫描文件并把结论绑定 Content Hash 与 Policy Version；随后分块加密并开始上传。
2. 中断上传并从已确认分片恢复；Bob 下载、验证 Checksum、预览并建立本地文件索引。
3. Carol 尝试用替换的 Tenant/Channel/Object ID 获取 Metadata、Blob、Content Key 和搜索结果。
4. 在 Bob 查询和下载期间撤销其权限；清除 Key、索引、预览和可解密缓存后重放旧 URL/Grant。
5. 分别注入 Scanner 超时或崩溃、扫描后文件替换、Blob 损坏、磁盘配额和索引损坏。

**预期结果：** 对象存储只收到加密块，续传不重复逻辑对象；Search 是本地可重建缓存且每次按当前 ACL 复检。撤权后立即不可搜索或解密；索引可从仍有权的 Ciphertext 重建。Scanner 失败不上传明文，也不无限阻塞独立文本消息。

**失败路径：** 若 Group 内更窄 ACL 的密码受众 Envelope 未经评审或验证，则该能力必须显式禁用，不能仅靠服务端 ACL 宣称 E2EE。损坏或 Hash 不一致的文件不进入可预览或可交付状态。

**证据：** Scanner/Content Hash 绑定、Multipart 状态、Blob Ciphertext/Checksum、ACL 负向矩阵、撤权时序、缓存与索引清理和重建报告、对象存储/临时目录/日志扫描。

### AC-006 首个垂直切片：E2EE Message → Local Agent → Approval → Artifact

**覆盖 Gate：** M2（G2 必选）、M4、M6

**前置条件：** Alice 与 Bob 已登录私有部署并加入 E2EE Channel；Alice Desktop Runtime 在线且获权；`workspace-a` 只映射一个含合成文件的本地目录；Agent 为 `manual`；测试模型 Endpoint 符合数据策略。

**步骤：**

1. Alice 在短时断网下发送需求消息；恢复后按 AC-003 验证 Durable ACK、幂等和顺序。
2. Alice 选择该消息创建 Task，显式选择有限 Message Ref、逻辑 Workspace、Alice Desktop、预期 Artifact 和 Agent；确认 Channel 历史未被整体复制进 Task/Run。
3. Model Control 仅收到能力、区域、工具、输出格式和数据策略，返回绑定 Endpoint、模型、参数和 TTL 的 Route Grant；该 Run 固定模型和版本。
4. `agentd` 取得唯一 Execution Owner Lease；凭 Capability 从 `locald` 请求选定 Message Ref 的有界 Context，并凭 Workspace Grant 从 `connectord` 读取一个授权文件。
5. Runtime 在本机组装 Prompt 并直连批准的模型 Endpoint，产生 Patch/Artifact 草案和结构化 Activity，不把原始 stdout/stderr 刷进 Channel。
6. Runtime 在受保护写动作前暂停。Bob 在 Task Thread 看到规范化目标、内容 Digest、Capability、设备、数据目的地、有效期与影响后批准；执行端复检批准绑定并完成写入。
7. 执行设备对 Artifact 加密后上传；`locald` 将带 Agent/Task/Run/Grant/Device/Hash Provenance 的结果加密发布回父 Channel。Bob 解密、核对 Diff、测试和 Artifact 后接受交付。
8. 用同一输入创建第二个 Run，Bob 选择拒绝；确认受保护动作未执行且拒绝状态可审计。

**预期结果：** 从消息到 Artifact 的可观察闭环完成；一个 Run 只有一个 Owner、模型版本和 Workspace Lease。Agent Attribution 可由接收端验证，Artifact 可追溯到 Run/Step/Actor/Hash。IM Server、Runtime Gateway 和 Model Control 不接触消息、文件、Artifact 明文或 Prompt；对象存储只有加密 Artifact。

**失败路径：** 普通消息不触发 Agent；未授权 Context 或路径、过期 Grant、错误 Endpoint、修改后的批准目标、重复批准和拒绝后重放均被拒绝。Runtime、Connector 或 Model 故障使 Run 等待、阻塞或失败，但不影响 Channel 消息；Retry 创建新 Run，不覆盖旧历史。

**证据：** Task/Run/Lease 时间线、Context Ref 与窗口大小、Grant/Policy/Approval Digest、Model Control 请求字段 Allowlist、模型 Endpoint 接收记录摘要、Connector 路径范围、Artifact Ciphertext/Hash/Provenance、批准和拒绝审计、全链路 Canary 扫描。

### AC-007 Desktop 离线、执行所有权、崩溃与显式转交

**覆盖 Gate：** M2、M4、M6

**前置条件：** Alice Desktop 是 Task 指定设备；Alice iOS、Android 与 Bob Desktop 在线；准备停止 `agentd`、`connectord`、`locald` 和 Runtime Gateway，并准备第二个逻辑 Workspace 映射。

**步骤：**

1. 关闭 Alice Desktop，分别从 Alice iOS 和 Android 发起 Task。
2. 观察 Task 等待状态，同时在 Mobile 上评论、审批尝试和取消其中一个排队 Task。
3. 恢复 Alice Desktop，确认未取消 Task 只派发给该设备并取得唯一 Lease/Fencing Token。
4. 运行中终止 `agentd`，让 Lease 过期后恢复旧进程；确认旧 Fencing Token 不能提交新状态。
5. 显式把同一个 `run_id` 的 Execution Owner 转交 Bob Desktop，选择 Bob 自己的 Workspace 映射并签发新 Grant；记录 Owner 变更以及 Lease Generation/Fencing Token 的单调递增，并确认 Alice 的旧 Lease/Grant 立即失效。
6. 对原 Task 执行 Retry，确认 Retry 创建新的 `run_id`，并与上一步“同一 Run 的 Owner 转交”在状态、审计和幂等键上可区分。

**预期结果：** Desktop 离线期间 Task 不漂移到 Mobile、Bob、Server 或隐藏 Runner；Mobile 不获得物理路径或 Execution Owner。Crash/Resume 不产生双 Writer；转交保持同一 `run_id`，可见、可审计并使旧 Lease/Grant 失效；只有 Retry 创建新的 `run_id`。

**失败路径：** Runtime Gateway 故障时派发暂停而 IM 继续；旧 Writer、重复 Dispatch、错误 Device、未重新授权 Workspace 和过期 Fencing Token 均不能提交状态或 Artifact。

**证据：** Mobile 与 Desktop 状态投影、同一 `run_id` 的设备/Owner/Lease Generation/Fencing 时间线、Retry 新 `run_id` 对照、进程故障注入、旧 Writer 拒绝记录、转交审批链和 Server 无物理路径/无远程执行扫描。

### AC-008 运行中撤权与高影响审批绑定

**覆盖 Gate：** M2、M4、M5、M6

**前置条件：** Run 已获得有限 Context、Workspace 与 Route Grant，并停在受保护动作前；准备管理员撤销、Grant 到期、策略版本更新和 Approval 重放或目标替换。

**步骤：**

1. 管理员在 Run 中撤销 Workspace 写权限；Runtime 发起下一次写动作。
2. 恢复权限并创建新 Run，让 Capability、Route 和 Approval 分别在使用前到期。
3. 批准一个文件写入后，替换规范化路径、内容、参数、Actor、Run 或 Content Hash 再执行。
4. 在另一个 Device/Run 重放批准；并发提交相同批准；批准后再更新组织 Policy。

**预期结果：** 每次 Context、Route、Connector 和受保护动作都按当前 Policy 复检。撤权或到期后的下一动作立即拒绝，Run 进入 `blocked` 或明确失败；恢复不会自动续用旧 Grant。批准只适用于绑定目标、内容、动作、Actor、Run、Policy 和有效期，并且最多产生一个逻辑效果。

**失败路径：** 拒绝、部分执行、执行进程崩溃和审计暂时不可用时均不得扩大权限或伪造成功；修改后的动作产生新的 Approval 请求。普通管理员不能借 Workspace/Approval Grant 调用 Recovery。

**证据：** 撤权与下一动作的有序时间线、Grant/Approval Digest、重放与 TOCTOU 负向结果、Run 最终状态、不可变审计 Event ID、文件效果计数和日志/Trace 内容扫描。

### AC-009 三端消息、Task 控制与生命周期一致性

**覆盖 Gate：** M1（Bridge/IPC 基线）、M3、M4、M6

**前置条件：** iOS/Android 真机和三个 Desktop OS 目标使用同一 Wire/Crypto Profile 与 Core Contract；准备进程回收、屏幕锁定、生物识别拒绝、Push 丢失、Bridge 取消和慢消费者。

**步骤：**

1. 五个平台互发 E2EE 消息、Thread、Reaction、编辑、撤回、附件和 Read Cursor，比较最终物化状态。
2. 在 iOS/Android 执行锁屏、进程回收、飞行模式、Push 丢失和重启恢复；在 Desktop 分别重启 UI、`locald`、`agentd` 与 `connectord`。
3. 从两种 Mobile 发起、观察、中断和审批 Task；目标 Desktop 离线与在线各执行一次。
4. 对 FFI/IPC 执行错误、取消、可靠流、背压、未知字段、无效 Handle 和内存压力 Contract。
5. 用键盘、触控、读屏、系统返回与 200% 缩放复核 Message/Task/Approval/Artifact 事件语义。

**预期结果：** 协议、Cursor、错误、取消和状态机语义一致；平台 UI 符合各自生命周期与无障碍规则。Mobile 只发送控制命令，不运行长任务、不访问 Workspace；可靠事件不因慢消费者丢失，而是按 Cursor 重新订阅。

**失败路径：** Bridge Panic 不穿越 FFI；未知错误映射为安全 `unknown`；Migration、安全存储或生物识别失败进入可诊断安全状态，不清库、不丢 Outbox、不暴露密钥。Agent Sidecar 故障不阻塞 Desktop IM。

**证据：** 五平台 Golden Fixture Digest、iOS/Android 真机运行与内存结果、三个 Desktop Sidecar/签名结果、最终状态比较、生命周期故障矩阵、无障碍记录和 Crash/诊断内容扫描。

### AC-010 企业恢复成功、失败与恢复密钥轮换

**覆盖 Gate：** M5、M6、M7

**前置条件：** 仅在 Recovery 满足启用 Gate 后执行成功路径；Recovery Control 使用独立网络、IAM、ServiceAccount、DB Role 和 Audit Sink；KMS/HSM 私钥不可导出；准备两名不同审批者和指定 Recipient Device。

**步骤：**

1. 创建绑定 Tenant、Group、Epoch/时间范围、对象、原因、Expiry、Policy Version 和 Recipient Device 的 Recovery Case；请求者不能成为自己的审批者。
2. 两名不同审批者批准后，由 Recovery Control 请求 KMS/HSM 做范围绑定操作，把结果端到端交付给指定 Recipient Device；验证该设备只恢复获批 Retention 范围。
3. 依次执行单人或自批、过期、重复审批、替换对象/Recipient/原因、跨 Tenant/Group/Epoch/Key Version、损坏 Envelope、审计不可用、KMS 不可用和错误设备接收。
4. 使用 Core、Realtime、Worker、Runtime Gateway、Agent、Model Control、普通管理员、集群操作员和伪造 ServiceAccount 调用 KMS/HSM 或恢复出口。
5. 轮换 Recovery Key；从 PostgreSQL PITR、Blob Backup 与 HSM 灾备组合恢复，覆盖旧 Key Version、轮换中断、部分材料丢失和错误销毁。

**预期结果：** 成功输出仅交给获批 Device 且不能复用为 Channel 主密钥；所有越权或失败输入都安全失败、不可抵赖且不交付部分结果。非恢复域不能调用 KMS/HSM。Key 轮换不批量重写旧消息，Retention 内旧 Envelope 与正确版本仍可按新 Case 恢复。

**失败路径：** KMS/HSM、审批、审计、独立密码评审或密钥/Envelope 灾备任一条件不满足时，Recovery 保持关闭并明确不可用；不能降级到普通管理员导出、服务端明文，或把恢复输出提供给 Agent、Search、DLP、摘要或模型。

**证据：** Network/IAM/KMS Policy、双人审批链、Case/输出绑定 Digest、所有 Workload 身份拒绝矩阵、失败和重放审计、Recipient Device 解封范围、轮换与灾备演练、HSM 操作审计和私钥未导出证明。

### AC-011 私有部署、备份、升级与回滚

**覆盖 Gate：** M1、M5、M6

**前置条件：** 一套 Standard Private 与一套 Air-gapped 测试环境；签名 OCI/Helm/离线 Bundle、SBOM、私有 CA、OIDC、对象存储、数据库迁移、N-1 客户端和上一可用版本回滚包。

**步骤：**

1. 在无公共 Registry、CDN、遥测和 License 服务的环境验签并安装七个工作负载；验证默认无公网入站，Air-gapped 无公网出站，Standard Private 仅允许白名单出站。
2. 创建消息、Outbox、Cursor、Task、Artifact、Recovery Envelope 和审计合成状态，完成加密备份与 PITR。
3. 滚动升级 Server、Client、Sidecar、Schema 和 Crypto 兼容层；升级期间让 N-1 Client 收发固定 Profile 消息并执行 Epoch 变化。
4. 注入 Pod、节点、PG、NATS、Redis、对象存储、KMS、网络故障、Migration 中断和未知字段/Profile。
5. 回滚可逆发布；对不可逆 Migration 验证预设门禁和前滚恢复。再从备份恢复并核对 RPO/RTO、Event、Cursor、Blob、审计和 Recovery Key Version 关系。

**预期结果：** 安装和运行不依赖公网；签名、SBOM 和制品 Digest 可追溯。升级或回滚不丢已 ACK 消息、Pending Outbox、E2EE 状态、Task 历史或 Artifact；N-1/未知字段按兼容规则处理，未知 Profile 安全失败。应用备份不含恢复私钥，HSM 灾备独立。

**失败路径：** 签名、迁移、兼容或 KMS 条件不满足时停止在安全版本；不得仅回滚 UI 而留下不兼容 Sidecar/Schema。NATS、Redis 或 Realtime 故障可恢复，PostgreSQL 不满足持久条件时不 ACK。

**证据：** 离线安装网络捕获、制品签名/SBOM/Digest、滚动升级与 N-1 矩阵、Migration/回滚报告、故障注入时间线、备份恢复 RPO/RTO 和数据状态 Digest、应用备份/HSM 权限扫描。

### AC-012 Retention、故障隔离与可观测性负证据

**覆盖 Gate：** M2、M5、M6、M7

**前置条件：** 所有数据类别均有唯一合成 Canary；配置短 Retention；能独立停止七个工作负载和基础组件；准备策略、时钟和本地快照回滚、永久离线设备模拟和诊断包导出。

**步骤：**

1. 分别停止 Model Control、Runtime Gateway、Recovery Control、Worker、Realtime、NATS、Redis 和 OTel，在每次故障中验证本地历史、Pending 发送和恢复补洞。
2. 让消息、文件、Context、Session、Artifact、Grant、索引与缓存到期；设备在线和离线各执行一次，再通过时钟、策略和数据库快照尝试回滚。
3. 执行成员或设备撤权、Tombstone、对象版本清理和备份到期；重建本地索引并复检当前 ACL。
4. 导出管理员预览过的诊断包；扫描 Server、队列、对象存储、日志、Trace、Crash Dump、共享卷、临时目录、诊断包和备份。
5. 尝试删除、重排、覆盖和跨 Tenant 注入 Audit Event，并尝试把正文、Prompt、Token 或 Key 写入审计。

**预期结果：** 非关键故障不改变消息事实和 E2EE；恢复后按 Cursor/Outbox 收敛。Retention 或撤权首先让 Key 与索引不可用，再完成缓存和密文生命周期；回滚不能恢复解密资格。诊断默认关闭且导出受控；普通可观测面不出现 C3/C4。Audit 追加、Tenant-scoped、最小化且可验证完整性。

**失败路径：** 发现任一 C3/C4 Canary、共享卷中的 IM DB/Prompt Cache、可复用 Grant/Key 或可篡改审计即为 Release Blocker。对被完全攻陷或永久离线设备只记录无法保证物理删除的已知限制，不虚假宣称远程擦除。

**证据：** 逐组件故障矩阵、Retention/撤权/回滚时间线、Key/索引/缓存清理报告、Cursor/Outbox 最终状态、全表面 Canary 扫描、诊断包审批与内容清单、Audit 完整性和跨 Tenant 负向测试。

## 7. M0–M7 签字清单

每项填写 `PASS / FAIL / NOT RUN / N/A（附批准理由）`、证据包 Digest、签字人和日期。任何“必须”项不是 `PASS`，Gate 即保持未通过。

### M0 / G0 架构、Crypto 与 Native Bridge Gate

- [ ] Scope、非目标、本文核心旅程和信任边界由 Product、Architecture、Security 一致确认。
- [ ] Client、Server/Protocol/Storage ADR 已接受；Crypto ADR 的候选状态与实际证据一致，没有把 Spike 交付误报为生产批准。
- [ ] Threat Model、数据分类、信任边界和 Risk Register 完整；M0 Due 的 Critical/High 有关闭证据。
- [ ] AC-001 的 M0 子集通过：首台 Device 只能由独立 Device Authority 或硬件保护的引导管理员建立；后续 Device 只接受现有获权 Device 的端到端批准或独立高影响流程；普通 OIDC 管理员、恶意 Control Plane、伪造/截断/跨 Tenant 批准链均不能创建 Cryptographic Member 或 Ghost Device；追加式 Device/Key 日志根与 split-view 负向证据可由独立方复核。
- [ ] AC-004 的 RFC/Golden Vector、Commit/Fork/Replay/未知 Profile、History/Recovery 负向向量通过。
- [ ] Rust、Desktop IPC、Swift 真机、Kotlin 真机与独立 RFC 实现对同一外部 Contract 给出一致结果。
- [ ] FFI 的调用、错误、取消、可靠流、背压、Crash/Resume 和内存所有权在 iOS/Android 真机通过。
- [ ] 候选 Crypto 依赖精确锁定，SBOM、许可证、漏洞和安全响应审查通过，无未批准生产阻断项。
- [ ] Proto Skeleton 不暴露明文、Prompt、恢复私钥或底层密码库类型。

**必须签字：** Product Owner、Architecture Owner、Security Owner、Identity/Device Authority Owner、Client-core Owner、Desktop/Native Bridge Owner、Crypto Owner、Contracts Owner、iOS Owner、Android Owner。

### M1 / G1 工程基础

- [ ] AC-001 OIDC/Device Credential 分离、Enrollment、撤销、安全存储及五平台备份/镜像到第二硬件的 anti-clone 通过。
- [ ] AC-009 的 Bridge/IPC Golden Fixture 与错误/取消基础通过。
- [ ] AC-011 可重复构建、签名、SBOM、离线开发栈和最小私有部署通过。
- [ ] Proto Lint/Breaking/Generate、未知字段和 N-1 Contract 通过；生成物与 Lockfile 由指定 Owner 维护。
- [ ] Secret Scan、制品证明、最小 CI 权限和依赖来源/完整性检查成为合并门。
- [ ] Server、Runtime、Model、Recovery、数据库、队列和可观测身份、网络与 Schema 权限默认拒绝。

**必须签字：** Architecture、Security、Integration/Release、Contracts、Client Platform、Identity、Platform。

### M2 / G2 首个产品垂直切片

- [ ] AC-002 证明 Agent/模型离线时普通 IM 独立可用，普通消息不触发模型。
- [ ] AC-003 证明 Offline Outbox、Durable ACK、重复 100 次、顺序、Gap Repair 和 Crash/Resume 收敛。
- [ ] AC-004 的 M2 子集在产品垂直切片上通过：当前 Epoch 加/撤成员、`rekey_required`、successor Epoch、重复/乱序/回滚与未知 Profile 均安全收敛，不依赖候选密码库内部类型。
- [ ] AC-006 完整通过 Message → Local Agent → Approval → Artifact 成功与拒绝闭环。
- [ ] AC-007 证明 Desktop 离线等待、不漂移、单 Owner、Fencing、Crash 和显式转交。
- [ ] AC-008 运行中撤权与下一动作拒绝通过。
- [ ] Agent Attribution/Provenance、跨 Tenant/Channel IDOR 和 Runtime 不读 IM DB 的负向证据通过。
- [ ] Server、Model Control、Runtime Gateway、对象存储和日志扫描未发现明文、Prompt、Token 或 Key。

**必须签字：** Product、Architecture、Security、Core/Sync、Client-core、Desktop、Runtime、Model Control、Quality。

### M3 / G3 Collaboration Alpha

- [ ] AC-005 文件加密、断点续传、Scanner、ACL、预览、本地搜索、撤权和重建通过。
- [ ] AC-009 中 DM、Channel、Thread、Reply、Reaction、编辑、撤回、附件与 Read Cursor 在五平台语义一致。
- [ ] iOS/Android 真机 E2EE 消息、Offline Outbox、进程恢复、文件选择与权限拒绝通过。
- [ ] APNs/FCM 仅作提示；Air-gapped 或丢 Push 后 Cursor 补齐通过。
- [ ] Desktop、iOS、Android 的键盘、触控、系统返回、读屏和输入法基本路径通过。

**必须签字：** Product/Design、Security、Desktop、iOS、Android、File/Search、Quality。

### M4 / G4 Agent Team Beta

- [ ] AC-006、AC-007、AC-008 在完整 Runtime/Capability/Approval/Artifact/Model Route 实现上复跑通过。
- [ ] Capability/Route/Workspace Grant 绑定 Tenant、Actor、Device、Task、Run、Resource、Action、TTL、Nonce 和当前 Policy，跨主体重放全部拒绝。
- [ ] Connector 路径规范化、符号链接、挂载、大小写、Unicode、TOCTOU 和任意执行参数负向测试通过。
- [ ] Approval 的目标、内容 Hash、动作和参数绑定与修改后重新批准通过。
- [ ] 模型 Endpoint 身份、数据位置、Retention、Fallback 和 Route Grant 重放负向测试通过。
- [ ] iOS/Android 发起、观察、中断和审批通过，且没有 Runtime、Connector 或物理路径能力。
- [ ] Prompt、Tool Secret、stdout/stderr 和临时 Context 的结束或撤权清理与 Canary 扫描通过。

**必须签字：** Product、Security、Runtime、Connector、Approval/Core、Model Control、Desktop、iOS、Android、Quality。

### M5 / G5 Private Deployment Beta

- [ ] AC-010 Recovery Control 隔离、双人审批、KMS/HSM Policy、范围绑定交付、失败和轮换/灾备通过。
- [ ] AC-011 Helm、七工作负载、私有 CA、Standard Private、Air-gapped、备份/PITR、升级/回滚通过。
- [ ] AC-012 Retention、Tombstone、备份生命周期、诊断包与不可变 Audit 通过。
- [ ] Recovery 私钥不在应用 DB、Secret、Pod、环境变量、备份、日志或诊断包；所有非恢复身份调用被拒绝。
- [ ] Organization/Member/Role/Device/Session/Runtime/Key 撤销和管理员高影响二次确认通过。
- [ ] 运维人员仅凭 Runbook 可完成安装、故障定位、恢复与回滚，且不获得消息明文。

**必须签字：** Product、Security、Recovery Security、Platform/SRE、Audit/Compliance、Core/Admin、Release、企业运维代表。

### M6 / G6 Release Candidate

- [ ] AC-001 至 AC-012 在候选制品上全部通过，无依赖未提交本地状态。
- [ ] macOS、Windows、Linux、iOS、Android 的关键 E2E、签名、安装、升级、失败迁移和回滚分别留证。
- [ ] 消息、同步、热点 Channel、文件、Runtime、模型和 Recovery 的 Load/Soak/Backpressure 达到批准的 SLO、内存、耗电、磁盘和容量预算。
- [ ] PostgreSQL、NATS、Redis、Realtime、Runtime、Model、KMS/HSM、网络和客户端 Crash/Resume Chaos 与 Runbook 一致；ACK 后零丢失。
- [ ] 五平台无障碍、IME、时区、语言、通知、后台和 200% 缩放矩阵通过。
- [ ] 日志、Trace、Crash Dump、诊断包、共享卷、对象存储和备份的 C3/C4 Canary 为零命中。
- [ ] Feature Complete；所有阻断缺陷有关闭 Commit 和复跑证据。

**必须签字：** Product、Architecture、Security、五平台 Owner、SDET/Quality、SRE、Release。

### M7 / G7 Security / Pilot Candidate

- [ ] 独立 Crypto Review 覆盖上游库、Threadline Adapter、FFI、Message/History/Recovery Envelope 和 KMS/HSM 流程；所有发现完成修复和复测。
- [ ] Penetration Test 覆盖身份、跨 Tenant、Ghost Device、E2EE/重放、Runtime/Connector、Model Egress、Recovery、供应链、管理面和私有部署；High/Critical 全部关闭。
- [ ] Risk Register 中全部 Critical/High 满足关闭标准并记录证据、Reviewer、残余分值和重开条件。
- [ ] 首轮企业 UAT 按本场景集执行，阻断问题关闭；试点数据处理、支持和升级/恢复演练获企业代表确认。
- [ ] 发布候选的签名、SBOM、依赖、离线 Bundle、Runbook、用户说明和审计证据一致可追溯。
- [ ] 没有以风险接受、临时管理员解密、明文回退或测试环境特例替代安全控制。

**必须签字：** Product Owner、Architecture Owner、Security Owner、独立 Crypto Reviewer、Pentest Owner、Release Owner、企业 Pilot Owner。

## 8. 签字记录模板

每个 Gate 使用一份独立记录，不在本文直接填入易变执行状态：

- Gate / target version：
- Candidate commit and artifact digests：
- Scenario results（`PASS / FAIL / NOT RUN / approved N/A`）：
- Evidence package location and digest：
- Open defects and Risk Register entries：
- Explicitly disabled capabilities：
- Product sign-off / date：
- Architecture sign-off / date：
- Security sign-off / date：
- Engineering and platform sign-offs / date：
- Enterprise reviewer sign-off / date（when required）：
- Decision（`PASS / HOLD / REJECT`）：
- Re-run or reopen conditions：

签字只批准该候选 Commit、制品和 Policy 组合。协议、Crypto Provider、Device Authority、Recovery、模型数据边界、平台 Bridge、KMS/HSM、签名根或 Scope 变化时，按 ADR/Threat Model 的重开条件确定需要复跑的场景和 Gate，不能沿用旧签字。

## 9. 需求与场景追踪

| 必要旅程或失败类型 | 场景 |
| --- | --- |
| OIDC、设备授权与撤销 | AC-001 |
| 模型或 Runtime 离线时 IM 可用 | AC-002、AC-012 |
| 断网、Outbox、重复、顺序、Gap | AC-003 |
| 成员变更、密钥轮换、未知版本、回滚 | AC-004 |
| 文件、断点续传、扫描、本地搜索 | AC-005 |
| E2EE Message → Local Agent → Approval → Artifact | AC-006 |
| Desktop 离线等待、不漂移、单 Owner、转交 | AC-007 |
| 运行中撤权、过期、批准重放与 TOCTOU | AC-008 |
| Desktop / iOS / Android 职责与证据差异 | AC-009 |
| 企业恢复成功、失败、隔离与密钥轮换 | AC-010 |
| 私有部署、备份、升级、失败迁移与回滚 | AC-011 |
| Retention、故障隔离、日志、诊断与审计 | AC-012 |

## 10. 依据

- [v1 Scope Freeze](./scope.md)
- [产品需求文档](../product-requirements.md)
- [交付计划](../delivery-plan.md)
- [Client Platform ADR](../adr/0001-client-platform.md)
- [Server / Protocol / Storage ADR](../adr/0002-server-protocol-storage.md)
- [Group E2EE / Recovery ADR](../adr/0003-group-e2ee-recovery.md)
- [数据分类](../security/data-classification.md)
- [信任边界](../security/trust-boundaries.md)
- [威胁模型](../security/threat-model.md)
- [风险台账](../security/risk-register.md)
- [T011 E2EE 互操作 Spike](../spikes/e2ee-interop.md)
