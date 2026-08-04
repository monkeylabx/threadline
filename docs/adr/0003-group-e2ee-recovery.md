---
status: proposed
date: 2026-08-04
---

# ADR-0003: MLS Group E2EE、设备密钥、历史共享与企业恢复

M0 候选决策；由 Threadline Product、Architecture 和 Security Workstream 评审。关联 Issue #19、P00-03、P00-08、`docs/acceptance/scope.md`、`docs/security/data-classification.md` 和 `docs/security/trust-boundaries.md`。

## 背景

Threadline Private Enterprise v1.0 要求消息正文和附件默认端到端加密，应用服务只保存密文；同时又要支持多设备、离线消息、成员变更、长期 Channel 历史和经多人审批的企业恢复。Agent、Service、IM Server、Model Control 和 Recovery Control 都不能因为参与协作或执行管理职责而获得持续的 Channel 解密能力。

本 ADR 选择用于 M0 验证的协议与库候选，并冻结 E2EE Group、Device、Epoch、历史共享、恢复、Retention 和版本边界。它不批准未经验证的密码构造，也不把候选库视为生产安全背书。

## 决策

### 1. 协议与实现边界

- Group E2EE 基线采用 [MLS 1.0 / RFC 9420](https://www.rfc-editor.org/rfc/rfc9420)。MLS 提供异步群组密钥建立、设备级成员、Epoch、成员增删、Forward Secrecy 和 Post-Compromise Security。
- M0 首选 [OpenMLS](https://github.com/openmls/openmls) 作为 Rust Spike 候选。
- OpenMLS 只能由 `crates/client-crypto/` 内的 Threadline Adapter 使用。UI、`client-core` 上层、Swift/Kotlin Binding、服务端协议和持久化 Envelope 不得暴露 OpenMLS 类型或序列化格式之外的库内部状态。
- `mls-rs` 可用于互操作对照，不作为首选生产依赖。自研 Group E2EE 协议不属于候选方案。
- History Sharing 和 Enterprise Recovery 是 Threadline 应用层协议，不修改 MLS 核心算法，不以未标准化 MLS 草案作为 v1 必需条件。

#### MLS 服务角色与元数据边界

- 按 [RFC 9750 MLS Architecture](https://www.rfc-editor.org/rfc/rfc9750.html)，Threadline 的 Device Authority、Device Credential 和公开 Key Directory 共同承担身份认证职责；Control Plane、Realtime 和 Sync 共同承担投递、排序与补洞职责。这些应用角色都不是 Cryptographic Member，也不因此获得群组密钥。
- 服务端可以看到并必须保护 C2 元数据，包括 Tenant/Channel/DM 与 E2EE Group 的绑定、Device/Leaf 引用、Epoch 编号、事件顺序、Ciphertext 长度和时间。v1 不承诺隐藏通信关系、频率或消息大小。
- Group ID 必须是 Tenant-scoped、不可与其他 Tenant 混淆的随机标识，并通过版本化 Envelope 与 Channel/DM 绑定；客户端拒绝跨 Tenant、跨 Group 或父会话不一致的对象。
- 服务端只根据外层 Envelope、Device 授权、已排序的 Membership Change、预期前序 Epoch、幂等键和大小限制接受或排序 Commit；MLS Commit、Welcome 和 Group State 的密码有效性由客户端验证。
- 恶意或故障服务端仍可延迟、丢弃、重放或向不同 Device 投递不同 Commit。客户端必须通过 MLS Transcript/Group State、严格 Epoch 连续性、Device/Key 追加式日志和 Gap Repair 检测并安全失败；E2EE 不承诺抵抗拒绝服务。
- Commit、Welcome 和 Proposal 使用公开还是私密 MLS Wire Format，以及外层 Envelope 中最小必要字段，由 T019 Contract 和 P00-08 互操作证据冻结；无论选择哪种形式，都不得让服务端根据内部明文重新解释密码状态。

#### 候选库供应链门

- M0 以 OpenMLS `0.8.x` 稳定发布线作为评估起点，但实现任务必须锁定精确版本、校验来源与完整性，并生成 SBOM；不得依赖浮动 Git 分支。
- OpenMLS 的移动目标目前属于构建覆盖而非完整真机测试覆盖，因此 iOS/Android 可用性必须由 Threadline Host Harness 自行证明。
- 禁止在生产构建启用会输出敏感内容或密钥的 debug feature。测试工具也只能使用合成 Fixture。
- 候选依赖必须经过许可证、维护活跃度、已知漏洞、RustSec/依赖审计、Fuzz/Interop 证据和安全响应流程检查。
- 上游发布出现密码或状态机安全修复时，必须重新运行 Golden Vector、持久化兼容和 N-1 测试，不能只依赖 SemVer 判断安全兼容。
- 独立 Crypto Review 的范围包括 Threadline Adapter 和应用层 Envelope，而不只审查上游 MLS 库。

### 2. E2EE Group 与密码成员

- 一个 `Channel` 或 `DM` 对应一个 E2EE Group。
- 普通 Thread 和 Task Thread 继承所属 Channel/DM 的 E2EE Group，不创建独立 Group。
- DM 的参与者发生变化时创建新的 DM 和 E2EE Group，不自动继承原 DM 历史。
- 只有获权 Device 是 MLS Cryptographic Member；同一 Human Member 的每台 Device 对应独立 Leaf。
- Agent Actor、Service Actor、IM Server、Model Control、Runtime Gateway 和 Recovery Control 都不是普通 MLS Member，也不持有 Channel/Epoch Key。
- Agent 通过短期 Capability Grant 从本机 `locald` 获取有限 Context。Agent 结果由执行设备上的 `locald` 加密回 Channel；应用 Envelope 记录 Agent Actor、Task、Run、Capability Grant 和 Provenance，密码层发送者仍是该 Device。
- Agent 署名不能只是客户端声明的 `agent_actor_id`。`locald` 必须用 Device Identity 绑定应用 Envelope，服务端在不读取正文的前提下复检 Agent/Task/Run/Grant 关系；接收客户端同时验证 Device、Agent Attribution 和 Ciphertext 的绑定。具体 AAD/签名字段由 T019 冻结。
- v1 不支持 Channel 内只对部分成员可见的私密 Task。需要不同可见范围时，用户必须在独立私密 Channel 或 DM 中创建 Task。
- Attachment 和 Artifact 使用独立 Content Key 与对象 ACL；其存在和引用仍遵循 Task/Channel 可见性。服务端 ACL 复检不能单独被宣称为 E2EE 的受众隔离。

### 3. Membership Change 与 Epoch

- IM Control Plane 是 Channel Membership、Device 状态和成员变更顺序的事实源，但不持有群组密钥。
- Control Plane 先持久化并排序 Membership Change，再签发不含密钥的 Membership Change Authorization。
- 一个当前获权、在线的 Committer Device 根据授权记录和目标 Device KeyPackage 生成 MLS Commit。服务端不能替客户端生成 MLS 群组密钥状态。
- 添加成员在 Commit 和 Welcome 被接受前保持 Pending；现有成员可以继续使用旧 Epoch。
- 移除成员或撤销 Device 后，Group 进入 `rekey_required`。在 successor Epoch 被接受前，新应用消息保留在 Durable Local Outbox，不能继续使用旧 Epoch 发送。
- 并发 Commit 由服务端成员变更序列决定。失败的 Device 同步已接受的 Commit 后重新生成，不创建长期分叉。
- 成员/Device 增删、Device 密钥更新、Recovery Key Version 变化和 Crypto Profile 重初始化必须推进 Epoch。
- 即使成员不变，也要通过可配置的自更新策略获得 Post-Compromise Security；具体时间或消息阈值由 M0 测量后确定，不写死在工作流代码中。

### 4. Device Credential 与 KeyPackage

- 每台 Device 拥有独立 Device Identity Key；私钥保存在 OS Secure Storage 中，并优先使用不可导出能力。
- Device Credential 绑定 Tenant、Actor、Device、Device Identity Key、Credential Version 和 Expiry。OIDC Session 只证明登录，不能直接成为密码身份。
- 第一台 Device 必须由企业 Device Authority 或具备硬件保护凭据的管理员批准。
- 同一 Member 的后续 Device 必须由现有获权 Device 端到端确认，或走独立的企业设备恢复/管理员审批流程。普通应用 Control Plane 不能单独创建 Cryptographic Member。
- 每个 KeyPackage 绑定 Device Credential、协议版本、Cipher Suite 和能力声明；默认一次性使用、短期有效。
- 服务端 Key Directory 只保存公开 KeyPackage，并原子标记消费状态。
- 撤销 Device 时同时撤销 Credential、作废未使用 KeyPackage，并通过 Membership Change 从所有相关 E2EE Group 移除其 Leaf。
- Device Enrollment 与 Leaf 添加携带可验证的批准链。M0 至少验证组织级追加式 Device/Key 日志和客户端一致性检查，防止服务端静默插入“幽灵设备”。

#### 五平台 Secure Storage

- Apple 平台使用 Keychain，并在算法和生命周期允许时使用 Secure Enclave；Android 使用 Keystore，并优先硬件支持能力。
- Windows 使用 CNG/DPAPI 或企业可用的硬件 Key Provider；Linux 使用 Secret Service，并允许企业 Profile 要求 TPM-backed Provider。
- MLS Leaf Signature Key、Device Identity Key、数据库包装密钥和 Recovery Recipient Key 是不同用途，必须独立派生或生成、独立标记、独立轮换，不能因为都存放在 OS Secure Storage 就复用。
- 当平台硬件不支持 `tl-mls-1` 所需算法时，允许使用 OS Secure Storage 中的硬件保护包装密钥封装软件密钥；文档和 Attestation 必须准确说明保护等级，不能声称该软件密钥本身不可导出。
- Swift/Kotlin/TypeScript UI 只持有不透明 Handle 或公开标识；私钥字节不得进入语言层 DTO、异常、日志、Crash Dump 或诊断包。

### 5. Content Key、History Key 与 Forward Secrecy

- 每条消息、Attachment 或 Artifact 使用独立随机 Content Key 加密。
- 当对象 ACL 与父 E2EE Group 一致时，Content Key 由当前 Epoch 的 History Key 封装。History Key 是 Threadline 应用层、Epoch-scoped 的保留材料，不是 MLS Ratchet Secret 或 Channel Master Key。
- 当对象 ACL 比父 E2EE Group 更窄时，Content Key 必须使用经评审、版本化且只面向有效密码受众的对象 Envelope；如果 P00-08 无法证明该路径，v1 必须禁止在 Group 内收窄密码受众，而不能退化为仅依赖服务端 ACL。
- MLS Ratchet Secret 按 RFC 9420 的删除计划及时清除，不为历史同步长期保存。
- 获权 Device 只在加密本地存储中保留仍处于 Retention 范围内的 History Key。
- 精确的 KDF、Domain Separation Label、HPKE/AEAD 构造、Envelope 编码和抗回滚规则必须在 M0 协议说明、Golden Vector 和独立密码评审中确定；不得在业务实现中临时发明。
- 本设计明确接受以下取舍：完全攻陷一台获权 Device 可能暴露该设备仍在 Retention 范围内的历史。Forward Secrecy 不能同时保证已经销毁密钥的历史安全与无限历史可恢复。

### 6. History Sharing

- 新加入 Channel 的 Member 默认可以读取仍在 Retention 范围内的历史，但只能通过显式、可审计的 History Sharing 获得相应 History Key。
- History Sharing 受当前 Membership、ACL、Retention 和企业策略限制；服务端不能仅凭当前 Membership 直接返回历史密钥。
- 新 Device 完成 Device Enrollment 后先加入当前 Epoch。历史优先由同一 Member 的现有获权 Device 端到端重新封装并交付。
- 如果没有合格的旧 Device，新 Device 可以收发当前及之后的消息，但不能自动获取历史。企业恢复是独立的高影响流程，不能作为静默设备同步后门。
- 被移除 Member 或撤销 Device 不能获得 successor Epoch，也不能继续请求新的 History Sharing。

### 7. Enterprise Recovery

- Private Enterprise v1.0 的 Channel/DM 从创建开始遵循组织恢复策略；用户不能在 v1 创建严格不可恢复 Channel。协议仍须为未来省略企业恢复接收者保留版本边界。
- Recovery Envelope 绑定 Tenant、E2EE Group、Epoch、Crypto Profile、Recovery Key Version 和被保护对象/History Key 范围，不包含可复用的 Channel Master Key。
- Recovery Case 必须绑定请求者、原因、对象、Epoch/时间范围、Expiry 和一个明确的 Recovery Recipient Device，并满足组织定义的多人审批。
- v1 默认至少需要两名不同审批者，请求者不能批准自己的 Recovery Case；企业可以提高阈值，但不能降为单人自批。
- KMS/HSM 只在获批 Recovery Case 中执行范围绑定的解封，并把结果重新加密给 Recovery Recipient Device。
- Recovery Control、管理员页面和普通应用服务不能获得恢复私钥字节、可复用群组主密钥或持续浏览能力。
- Recovery Key 轮换后，新 Epoch 使用新版本；旧版本只为仍在 Retention 范围内的历史保留。系统不批量重写既有消息。
- Recovery Case 到期后，授权和临时材料立即失效；再次恢复必须重新审批。
- 恢复输出不能成为 Agent Tool、服务端搜索、DLP、摘要或模型输入的通用接口。

### 8. Retention 与密码删除

- Retention 是服务端和 Device 都必须执行的密码生命周期策略，不只是对象存储 TTL。
- Content/History Key 携带可验证的到期策略。正常运行的客户端即使离线，也要在到期后停止解密并清除 Key、本地索引、预览和缓存。
- 重连后客户端同步 Tombstone、撤权和策略变更；服务端删除 Ciphertext、Recovery Envelope 和派生投影。
- 备份通过密钥销毁与备份生命周期共同完成清理，不能声称立即改写所有离线备份。
- 设备备份不得克隆 Device Identity、Device Credential 或 KeyPackage 私钥。恢复到新硬件视为新的 Device Enrollment；只有同一受保护设备身份可恢复其本地加密状态。
- 对被完全攻陷、越狱或永久离线的 Device，Threadline 不承诺远程物理删除。系统撤销 Device、阻止其进入后续 Epoch，并把未确认清理记录为合规风险。

### 9. Crypto Profile 与兼容性

v1 定义一个不可静默降级的 Profile：

```text
tl-mls-1
|- MLS protocol: 1.0
|- Cipher suite: MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519
|- Message envelope version: 1
|- History envelope version: 1
`- Recovery envelope version: 1
```

- 每个 E2EE Group 创建时固定 Crypto Profile；Device 通过 KeyPackage 声明支持范围。
- 新 Profile 不能重新解释旧 Ciphertext 或 Envelope。
- Profile 升级通过 MLS Group Reinitialization 或明确的新 Group 世代完成，并保留可审计的新旧关系。
- 客户端遇到未知 Profile 时进入安全的不兼容状态，不猜测、不降级。
- OpenMLS crate 版本不是 Wire Protocol Version。替换底层库不能改变 `tl-mls-1` 的线上语义。
- N-1 客户端只要仍支持 Group 固定的 Crypto Profile 就可继续同步；服务端不能以库版本号拒绝兼容客户端。
- 不支持目标 Profile 的 Device 不能加入 Group。Profile 升级期间，旧客户端保持只读或明确的不兼容状态，不得通过降级 Cipher Suite 绕过。
- 持久化 Envelope 必须保留未知字段并通过 Golden Frame 验证；具体 Protobuf Contract 由 T019 定义，本 ADR 不抢占 Contracts Workstream。
- v1 不宣称 FIPS 140-3 validated。`client-crypto` 保留 Crypto Provider 抽象；未来 FIPS Profile 必须单独排期、选择适用 Cipher Suite 和已验证密码模块，并建立独立验收 Gate。

## 明确不包含

- 最终 History/Recovery Envelope 密码构造的生产批准。
- OpenMLS 或任意 Crypto Provider 的生产安全背书。
- FIPS 140-3 产品验证。
- 严格不可恢复 Channel。
- Channel 内私密 Task 或每 Thread/Task/Run 一个 MLS Group。
- Agent、Service 或服务端持有 Channel Key。
- 服务端明文搜索、DLP、摘要或 Prompt 代理。
- P00-08 的五平台实现、性能与安全验证。

## P00-03 验收

P00-03 在以下文档条件全部满足时完成：

- 本 ADR 记录协议/库候选、E2EE Group/Device/Epoch 边界、History/Recovery/Retention 语义和版本策略。
- `CONTEXT.md` 使用同一套领域语言，不把 Actor、Channel Membership、Cryptographic Member、Capability Grant 或 Agent Session 混为一谈。
- 本 ADR 与 Scope Freeze、数据分类、信任边界、Client ADR 和 Server/Protocol/Storage ADR 无未解释冲突。
- 候选方案包含替代方案、明确退出条件和 P00-08 验证清单。
- Product、Architecture 和 Security Owner 同意它可以作为 M0 Spike 输入，但不把 `Proposed` 误报为生产批准。

代码实现、五平台互操作通过和生产密码批准不属于 P00-03 的完成条件。

## P00-08 接受 Gate

本 ADR 只有在以下证据完成后才能从 `Proposed / M0 Candidate` 升级为 `Accepted`：

- RFC 9420 Known Answer Test、OpenMLS 互操作测试和 Threadline Golden Vector 通过。
- Rust Core 在 macOS、Windows、Linux、iOS 和 Android 的构建与最小 Host Harness 通过。
- Device add/remove/revoke、离线乱序、并发 Commit、Group Fork、未知 Profile 和回滚攻击测试通过。
- History Sharing、Device History Sharing 和 Recovery Envelope 有独立协议说明、测试向量与负向测试。
- Crash/Resume、加密持久化、Key 清除、Retention 到期和备份恢复测试通过。
- 对 OpenMLS、Crypto Provider、FFI、应用层 Envelope 和 KMS/HSM 流程完成独立密码与安全评审。
- 性能测试覆盖目标 Group Size、Epoch 频率、KeyPackage 消耗、历史恢复范围和移动端内存压力。
- 任何高风险发现均有修复、替代候选或明确的 Scope 回退结论。

最高测试 Seam 是 library-independent 的 `client-crypto` Protocol Harness：同一组版本化输入 Transcript 和 Golden Vector 必须在 Rust、Swift Host、Kotlin Host 以及至少一个独立 RFC 9420 实现中得到相同的外部结果。测试断言 Group/Epoch/Envelope 状态、错误和可恢复性，不断言 OpenMLS 内部类型或调用序列。

## 退出与回退条件

- OpenMLS 无法在五平台稳定构建、持久化或通过互操作/安全 Gate 时，替换 Adapter 后的实现候选；不得改变 Threadline 上层授权和 Envelope 契约来掩盖库缺陷。
- 应用层 History/Recovery 方案无法通过独立密码评审时，企业恢复和历史回填保持关闭；不得退化为服务端保存 MLS Ratchet Secret、Channel Master Key 或消息明文。
- 无法防止服务端静默插入 Device 时，不得宣称抵抗恶意 Control Plane；必须先完成 Device Authority 与可验证批准链。
- Retention 与历史可恢复性的目标无法同时满足时，必须重新打开产品 Scope 并明确选择，不得同时作出互相矛盾的安全承诺。
- 任何让 Agent、Model Control、Runtime Gateway、Recovery Control 或 IM Server 获得持续 Channel 解密能力的需求都必须重开 Scope、Threat Model 和本 ADR。

## 考虑过的替代方案

### 自研群组密码协议或 Sender Key 变体

拒绝。Threadline 需要多设备异步成员变更、Forward Secrecy、Post-Compromise Security、互操作测试和外部评审基础；自研协议会扩大不可接受的密码风险和审计范围。

### 服务端作为 MLS Member 或 Group Key Service

拒绝。它会让应用服务具备持续解密能力，直接违反 E2EE 与信任边界。

### Agent Actor 作为 MLS Member

拒绝。Agent 会因此持续获得 Channel 历史，绕过 Capability Grant 的对象、用途和期限限制。

### 长期保存 MLS Ratchet Secret 以支持历史

拒绝。历史与恢复使用独立、可版本化、可删除的 Content/History Key 层；MLS Ratchet Secret 继续按协议清除。

### 新 Device 自动调用企业恢复

拒绝。企业恢复必须是可见、多人审批、范围绑定的高影响流程，不能成为普通设备同步路径。

### v1 直接承诺 FIPS 140-3 validated

拒绝。当前 Scope 和排期没有跨五平台认证工作。未来以独立 Crypto Profile、Provider 和验证 Gate 交付。

## 后果

### 正面

- Group 密钥只存在于获权 Device，服务端仍可维护企业 Membership、排序和审计。
- Agent 权限与 Channel 密码成员身份保持分离。
- Device 可独立加入和撤销；被撤销 Device 不能进入后续 Epoch。
- History Sharing 和 Enterprise Recovery 具有明确、可审计且可版本化的边界。
- OpenMLS 被 Adapter 隔离，库替换不会直接污染跨端 API 或 Wire Contract。

### 负面

- History Key 的保留会缩小 Retention 范围内历史的 Forward Secrecy 保证。
- 移除或撤销后的 `rekey_required` 会暂时阻止新消息提交，直到在线 Device 完成 Commit。
- Device Authority、批准链和一致性日志增加 Enrollment 与运维复杂度。
- History/Recovery Envelope 是高风险应用层密码设计，必须投入独立评审和跨平台测试。
- 多设备、离线乱序、并发 Commit 和长期历史会显著扩大本地持久化及恢复测试矩阵。
