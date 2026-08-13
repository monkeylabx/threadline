# Threadline HTML 原型页面与状态矩阵

状态：T012 基线清点
基线：`5525eac8cd65e0a011d1f854e4a7a9ddf3b53055`

## 1. 范围与结论

本文清点 [`docs/prototype/README.md`](../prototype/README.md) 中的全部 Route，并定义 Desktop、iOS、Android 在 Loading、Empty、Error、Offline、Permission、E2EE 和 Recovery 下应保持的产品语义。

`docs/prototype/index.html` 仍是唯一产品设计入口。窄于 720px 的视口由根入口路由到 `docs/prototype/mobile/` 内部渲染器；内部页面不是第二个交付入口，也不应直接分发。

当前结论：

- 12 个 Desktop Route 均有正常、有数据的演示态。
- Loading、Empty 和 Error 几乎都没有形成可评审状态。
- Desktop 已演示一次性审批、同步缺口修复、权限范围说明和部分加密标识，但 Runtime Offline 原演示会自动切换到超出 v1 Scope 的企业 Runner，不能计为有效实现。
- Mobile 实际实现了消息、活动、搜索、频道、任务 Sheet、审批 Sheet 和“我的”容器。
- Mobile 对 `files`、`runtime`、`sync`、`organization` 和独立 `task-result` 的映射只是兼容重定向，不能记为页面已经实现；`agents`、`admin` 的完整页面不属于 Frozen Scope 的 Mobile v1 必需目的地。
- IM 网络离线、Agent Runtime 离线和设备撤销是三个不同故障域。Runtime 离线不得阻断普通 IM。
- 解密失败应是对象级状态，不能把整个页面变成通用 Error，也不能暴露 Ciphertext、Key、Prompt 或无权限元数据。

## 2. 读表与拆 Issue 规则

状态标记：

- `●`：当前原型已有可见、可操作的主要状态。
- `△`：已有部分表达，但缺少平台覆盖、失败分支或恢复动作。
- `○`：当前原型缺失。
- `—`：不需要独立整页状态，但对象级处理仍须遵守全局契约。

每个 `△` 或 `○` 单元格都有唯一 Gap ID，例如 `G-CH-OFF`。每个 Gap ID 都可直接拆成一个后续 Issue。

Issue 至少应包含：

- Route、状态和受影响平台。
- 从唯一入口进入的复现方式。
- 主动作、重试/退出动作、禁用动作和恢复后的焦点位置。
- 缓存、Outbox、权限复检、审计和敏感数据处理要求。
- Desktop、iOS、Android 的截图或真机证据；不要求三端复用同一布局。

建议后续统一使用：

`?screen=<route>&state=<fixture>`

窄视口仍必须从根入口路由，不直接分发 `mobile/index.html`。

## 3. 全局状态呈现契约

| 状态 | 必须表达 | 不允许 |
| --- | --- | --- |
| Loading | 保留全局导航和页面骨架；标出正在加载的区域；长等待提供取消或重试；辅助技术可获知忙碌状态 | 用空白页冒充加载；Skeleton 引起布局跳动；加载时误开放高影响动作 |
| Empty | 区分从未有数据、筛选无结果、离线且无缓存、权限过滤后无结果；给出与原因一致的下一步 | 用“暂无”掩盖 Error、Permission 或解密失败 |
| Error | 说明失败域、影响范围、草稿/缓存是否保留及恢复动作；关联 ID 不得包含敏感内容 | 展示原始堆栈、Ciphertext、Token、Key、Prompt 或本地绝对路径 |
| Offline | 区分 IM 网络离线与 Runtime Offline；显示最后同步时间和待发送数量；本地草稿与 Durable Outbox 继续工作 | 因 Runtime Offline 禁用 IM；把“已进入 Outbox”标成“已送达” |
| Permission | 在读取和动作执行前复检；只显示最少必要的资源描述；撤权立即生效 | 通过搜索片段、缩略图、文件名或 Agent Context 泄露无权限内容 |
| E2EE | 对象级展示等待 Key、Epoch 落后、损坏或验证失败；允许安全重试或请求 Key | 显示 Ciphertext/Key Material；静默回退为服务端明文 |
| Recovery | 明确恢复范围、审批人、阶段、审计和失败后的数据可见性；企业恢复与日常消息链路隔离 | 让 IM Server、Model Control、Agent 或模型 Endpoint 调用恢复私钥 |

状态优先级：

1. 设备撤销或安全存储不可用时，先锁定受保护内容和写操作。
2. 页面权限决定能否进入资源；对象权限可以形成最小化占位符。
3. E2EE/Recovery 在获准访问后按对象呈现，不能被通用 Error 吞掉。
4. Offline 保留缓存和草稿；区域 Error 只替换失败区域。
5. Loading 和 Empty 只能在确认没有更高优先级状态后展示。

## 4. Route 状态矩阵：Loading、Empty、Error、Offline

表格文字既是目标行为，也是对应 Gap 的最小验收范围。

| Route | Loading | Empty | Error | Offline |
| --- | --- | --- | --- | --- |
| `channel` | `○ G-CH-LOAD` Timeline 骨架；保留 Header/Composer 结构并禁止误发 | `○ G-CH-EMPTY` 新频道欢迎态和可执行的发消息动作 | `○ G-CH-ERR` 区域重试；保留草稿和缓存 Timeline | `○ G-CH-OFF` 显示最后同步和 Outbox；可排队发送，Runtime 状态单独表达 |
| `inbox` | `○ G-IN-LOAD` 列表骨架，不预设未读数准确 | `○ G-IN-EMPTY` 区分全部已处理与筛选无结果 | `○ G-IN-ERR` 聚合失败，可回到来源页面 | `○ G-IN-OFF` 展示缓存结果和数据时间；处理动作进入待同步 |
| `search` | `○ G-SE-LOAD` 区分本地索引和远端查询 | `△ G-SE-EMPTY` Mobile 有无结果文案；补 Desktop、筛选原因和索引未就绪 | `○ G-SE-ERR` 区分索引损坏、查询失败和权限复检失败 | `○ G-SE-OFF` 搜索本地已解密索引；远端范围标为不可用 |
| `tasks` | `○ G-TA-LOAD` 任务列表和所选 Run 分区加载 | `○ G-TA-EMPTY` 无任务、筛选无任务和未选 Run 分开 | `○ G-TA-ERR` Run Event 读取失败不伪装成 Run 失败 | `○ G-TA-OFF` 原自动切企业 Runner 违反 Scope；实现 `waiting_for_runtime`、取消、重连和显式 Owner 转交 |
| `approvals` | `○ G-AP-LOAD` 待办计数和详情独立加载；决定按钮保持禁用 | `○ G-AP-EMPTY` 无待办，并可查看已处理记录 | `○ G-AP-ERR` 决定提交失败时保持待处理且防止重复决策 | `○ G-AP-OFF` 可读缓存收据；排队决定明确尚未生效和过期风险 |
| `task-result` | `○ G-TR-LOAD` 元数据、Diff、Artifact 分区加载 | `○ G-TR-EMPTY` Run 完成但无变更或 Artifact 的合法结果 | `○ G-TR-ERR` Diff/Artifact 单区失败；不改变 Run 完成事实 | `○ G-TR-OFF` 可审阅缓存；创建 PR 等远端动作禁用或排队 |
| `files` | `○ G-FI-LOAD` 列表、缩略图、下载和解密进度分开 | `○ G-FI-EMPTY` 无文件、筛选无结果和缓存已清理分开 | `○ G-FI-ERR` 上传、下载、校验、预览失败分别恢复 | `○ G-FI-OFF` 缓存文件可打开；未缓存文件说明原因；上传保持可恢复会话 |
| `agents` | `○ G-AG-LOAD` 目录和所选 Agent 详情分区加载 | `○ G-AG-EMPTY` 组织无 Agent 或筛选无结果 | `○ G-AG-ERR` 目录、Policy、Runtime 健康失败分别表达 | `○ G-AG-OFF` 展示缓存职责和上次健康；配置变更不可假装已生效 |
| `runtime` | `○ G-RU-LOAD` 设备目录和心跳详情分区加载 | `○ G-RU-EMPTY` 无 Runtime，提供 Desktop Enrollment 指引 | `○ G-RU-ERR` 心跳未知、健康查询失败和执行失败分开 | `○ G-RU-OFF` 原自动漂移演示无效；实现等待本设备、取消或显式转交另一台获权 Desktop |
| `sync` | `○ G-SY-LOAD` DB、Cursor、Outbox 检查分阶段 | `○ G-SY-EMPTY` 新设备尚无历史，不显示“同步完成” | `○ G-SY-ERR` Gap Repair、签名、物化、索引重建分别失败 | `△ G-SY-OFF` 已有过期 Cursor 和成功修复；补网络离线、重连和未收敛状态 |
| `organization` | `○ G-OR-LOAD` 验证身份、打开隔离 DB 和 Key 状态 | `○ G-OR-EMPTY` 未加入组织，提供加入或创建入口 | `○ G-OR-ERR` 切换失败回到原组织，且不混合缓存和授权 | `○ G-OR-OFF` 仅进入有本地凭据和缓存的组织；其他组织说明原因 |
| `admin` | `○ G-AD-LOAD` 指标、提醒和审计区域独立加载 | `○ G-AD-EMPTY` 无提醒或无审计结果是健康态 | `○ G-AD-ERR` 单个治理模块失败不污染其他模块 | `○ G-AD-OFF` 只读缓存；策略、高风险操作和导出不得离线提交 |

## 5. Route 状态矩阵：Permission、E2EE、Recovery

| Route | Permission | E2EE | Recovery |
| --- | --- | --- | --- |
| `channel` | `△ G-CH-PERM` 已有 Agent Context 范围；补成员移除、只读频道、发送被拒和撤权 | `○ G-CH-E2EE` 消息级解密失败、等待 Epoch/Key、验证失败 | `○ G-CH-REC` 部分历史恢复、等待/失败和不可恢复区间 |
| `inbox` | `○ G-IN-PERM` 来源撤权后移除片段，只保留最小提示 | `○ G-IN-E2EE` 暂不可解密时不得泄露摘要 | `○ G-IN-REC` 恢复中的聚合项显示可用范围 |
| `search` | `△ G-SE-PERM` 已有“结果前复检”；补撤权 Tombstone 和过滤计数 | `○ G-SE-E2EE` 只索引已解密且当前有权对象 | `○ G-SE-REC` 恢复或重建时显示索引覆盖范围和进度 |
| `tasks` | `△ G-TA-PERM` 已展示 Grant；补过期、撤销、Scope 变化和重新申请 | `○ G-TA-E2EE` Context 无法解密时阻塞 Run，不发送到 Runtime 或模型 | `○ G-TA-REC` 恢复后重新授权；旧 Run 不自动扩权 |
| `approvals` | `△ G-AP-PERM` 已有 Capability Scope；补无资格、双人职责分离和决定冲突 | `○ G-AP-E2EE` 收据只含必要元数据；内容仅在有权设备本地展示 | `○ G-AP-REC` 双人恢复审批、过期、拒绝和不可变审计 |
| `task-result` | `○ G-TR-PERM` Artifact/Diff 撤权、来源撤权和接受资格 | `△ G-TR-E2EE` 已有 Hash/来源；补 Key 缺失、校验失败和轮换 | `○ G-TR-REC` 恢复 Artifact 标明范围，不自动进入 Agent Context |
| `files` | `△ G-FI-PERM` 已显示继承频道；补拒绝、撤权、重新申请和缩略图清理 | `△ G-FI-E2EE` 已显示加密缓存；补上传加密、下载解密/校验和 Key 缺失 | `○ G-FI-REC` 企业恢复文件的审批、范围、失败和缓存策略 |
| `agents` | `△ G-AG-PERM` 已有能力控件；补不可编辑原因、Owner 缺失和撤权 | `○ G-AG-E2EE` 标示合格 Runtime，禁止不合格 Runtime 处理 Context | `○ G-AG-REC` Agent 不是恢复接收者；恢复内容需要新 Grant |
| `runtime` | `△ G-RU-PERM` 已列目录授权；补路径撤销、Grant 过期和受管设备限制 | `△ G-RU-E2EE` 已显示设备密钥；补 Key 无效、Epoch 落后和不合格 Runtime | `○ G-RU-REC` Runtime 不持有恢复私钥；恢复期间暂停相关 Run |
| `sync` | `○ G-SY-PERM` Membership 变化后停止拉取/物化并清理索引 | `△ G-SY-E2EE` 已显示加密和验证；补 Epoch、签名和损坏帧失败 | `△ G-SY-REC` 已有成功流程；补新设备、History Sharing、企业恢复和失败 |
| `organization` | `○ G-OR-PERM` 停用、访客范围、切换资格和重新登录 | `△ G-OR-E2EE` 已有 E2EE 标识；补每组织独立 Device/Key 状态 | `○ G-OR-REC` 恢复绑定组织，禁止跨组织复用审批、Key 或索引 |
| `admin` | `○ G-AD-PERM` 无管理员权限、细分角色、二次确认和双人审批 | `△ G-AD-E2EE` 已改为单一 E2EE 与可审计恢复概览；补未知、不合规和不可见边界 | `○ G-AD-REC` 恢复队列、双人审批、HSM/KMS 隔离、失败和审计 |

## 6. 关键跨页面状态

### 6.1 解密失败

解密失败以对象占位符保留排序和上下文位置，不替换整个页面。

| 触发 | `channel` 表达 | 其他消费者 | 可执行动作 |
| --- | --- | --- | --- |
| Key Package 或 Epoch 尚未到达 | 原位置显示“正在获取此消息的密钥”，仅保留有权的发送者和时间元数据 | `inbox` 不生成正文摘要；`search` 不产生命中；`tasks` 不加入 Context | 重试同步、查看设备或同步状态 |
| 当前设备无历史访问权 | 显示“此设备无权查看这段历史”，不制造无效重试循环 | 文件预览、Artifact 和搜索片段同样隐藏 | 在策略允许时发起 History Sharing；否则解释策略 |
| 签名、Ciphertext 或附件校验失败 | 显示“内容验证失败”，隔离对象并保留序列位置 | `sync` 记录失败阶段；诊断不含正文或 Key | 重新拉取密文帧、提交脱敏诊断；禁止忽略验证 |
| 设备已撤销 | 进入全局安全锁定，不再尝试解密未来 Epoch | 清理敏感内存，停止索引、预览和 Agent Context | 重新注册设备；不得用企业恢复绕过撤销 |

直接后续 Gap：

- `G-CH-E2EE`
- `G-IN-E2EE`
- `G-SE-E2EE`
- `G-TA-E2EE`
- `G-FI-E2EE`
- `G-TR-E2EE`
- `G-SY-E2EE`

### 6.2 设备撤销

设备撤销是身份与密钥安全状态，不是普通网络 Error。

Desktop：

- 停止 `locald` 对受保护数据的读取。
- 停止 Agent Context 和 Connector Grant。
- 锁定 Composer、文件和任务写操作。
- 仍可进入重新注册和脱敏诊断。

iOS：

- 清理进程内 Key。
- Keychain 项进入不可用或删除流程。
- Push 只能引导重新认证，不能显示消息预览。

Android：

- 清理进程内 Key。
- Keystore Alias 进入不可用或删除流程。
- 通知和后台工作不得继续物化受保护内容。

三端共同要求：

- 显示撤销时间、设备名称、管理员联系或重新注册动作。
- 不得读取未来 Epoch。
- 不得用缓存搜索索引绕过撤权。
- 已打开的文件预览和 Agent Context 必须立即失效。

Issue-ready Gap：`G-DEV-REVOKED`。

验收范围包括前台撤销、离线期间被撤销后重连、应用进程重启、已打开预览、待发 Outbox 和运行中的 Agent Context。

### 6.3 Runtime Offline

Runtime Offline 与 IM 网络离线正交：

- `channel`、`inbox`、本地 `search`、缓存 `files` 和普通 Composer 保持可用。
- 新任务可以排队等待指定 Runtime、显式选择另一个合格 Runtime，或取消。
- 不得静默切换 Execution Owner。
- 运行中任务进入“等待 Runtime”或“Owner 失联”，而不是业务失败。
- 恢复写入必须经过 Lease/Fencing 校验。
- 等待审批的收据继续可见。
- 若 Grant 在 Runtime 恢复前过期，任务必须重新申请。
- Mobile 只观察、发起、暂停/中断和审批；不会把手机变成 Runtime，也不会取得 Desktop Workspace 路径权限。

现有 Desktop Runtime 页面已移除企业 Runner 正常态，并把模拟失联改为“当前 Run 等待本设备恢复，不会自动转移”；任务、频道和 Mobile 尚未联动，所以两个 Route 仍记为 `○`。

直接后续 Gap：

- `G-RU-OFF`
- `G-TA-OFF`
- `G-CH-OFF`
- `G-AP-OFF`

### 6.4 审批等待

当前原型已有：

- Desktop `approvals` 页面。
- Mobile `approval` Sheet。
- 一次性 Grant 的能力、目标、有效期和执行位置。
- 批准、拒绝入口。
- Task Activity 中的“等待你的决定”。

仍需补齐：

- Task/Run 停在确切 Step，并显示申请时间和过期时间。
- 无审批资格、请求撤回、请求过期、他人已决定、提交失败分别呈现。
- 批准、拒绝和过期都形成不可变审计事件。
- 重复点击或重放只能产生一个逻辑决定。
- Runtime Offline 时可阅读收据，但本地排队决定必须标为“尚未生效”。
- Recovery Approval 使用独立双人仪式，不能复用普通单人 Workspace Grant。

直接后续 Gap：

- `G-AP-ERR`
- `G-AP-OFF`
- `G-AP-PERM`
- `G-AP-REC`

### 6.5 Recovery 生命周期

Recovery 至少包含：

`not-started -> approval-pending -> approved -> key-unwrapping -> history-rebuilding -> complete`

失败或中止分支：

- `rejected`
- `expired`
- `key-unavailable`
- `verification-failed`
- `partial-history`

产品要求：

- 显示恢复组织、频道或文件范围、目标设备和审批人。
- 不显示恢复私钥或消息正文。
- 恢复不会给现有 Agent Run 或旧 Capability Grant 自动扩权。
- 部分恢复时，Timeline、Search、Files、Inbox 和 Task Context 都声明覆盖范围。
- 用户可退出进度页；IM 继续工作。
- 服务端工作由隔离的 Recovery Control 承担。

主入口 Gap 为 `G-SY-REC` 和 `G-AP-REC`，消费端由各 Route 的 `*-REC` Gap 验收。

## 7. 当前移动端实际路由

根入口的当前映射：

| README Route | 当前 Mobile View | 当前实际覆盖 |
| --- | --- | --- |
| `channel` | `channel` | 已有单列 Timeline 和 Composer |
| `inbox` | `activity` | 已有活动聚合正常态 |
| `search` | `search` | 已有搜索 Overlay 和无结果文案 |
| `tasks` | `task` | 已有 Task Activity Sheet |
| `approvals` | `approval` | 已有 Approval Sheet |
| `task-result` | `task` | 仅回到运行中任务，不是独立交付结果页 |
| `files` | `messages` | 未实现文件目的地 |
| `agents` | `messages` | 未实现 Agent 目录目的地 |
| `runtime` | `profile` | 仅“我的”中的摘要，不是 Runtime 详情 |
| `sync` | `profile` | 未实现同步与恢复详情 |
| `organization` | `profile` | 仅显示当前组织，未实现组织切换流程 |
| `admin` | `profile` | 未实现管理目的地 |

兼容重定向不能作为对应 Route 的验收证据；同样，Desktop Route 的存在不自动把完整 Mobile 页面升级为 v1 要求。

## 7.1 已修正的原型范围冲突

本次清点发现并直接修正三处不能作为后续 Issue 基线的 Frozen Scope 冲突：

- Runtime Offline 曾显示自动切换企业 Runner，Runtime 正常态还展示 Kubernetes Runner；现改为获权 Desktop Runtime 离线时等待，只有用户可显式转交另一台获权 Desktop。
- Agent 目录曾把 Agent 标为企业/远程 Runtime；现改为本地或获权 Desktop 执行位置。
- Admin 曾同时展示 Managed Encryption 与 E2EE，并提供 SSO/SCIM 导航；现改为 v1 单一 E2EE + 隔离、多人审批的可审计恢复，以及 OIDC/成员管理。

这些修正只移除越界正常态，不代表对应 Loading、Empty、Error、Offline、Permission、E2EE、Recovery Gap 已经实现。

## 8. Desktop、iOS、Android 导航与职责

### 8.1 导航层级

| 平台 | 根导航 | 内容推进 | Sheet 或高影响动作 | 返回语义 |
| --- | --- | --- | --- | --- |
| Desktop/Web | 组织 Rail、Channel Sidebar、主 Workspace；宽屏可有 Context/Inspector | Route 在 Workspace 切换；任务和文件可多栏检查 | 创建 Task 使用 Modal；Task/Approval 可用独立页面或活动层 | 保留组织、频道、所选对象和滚动位置 |
| iOS | `消息 / 活动 / 我的` Tab；频道从消息列表推进 | Channel、Thread、File 与本机 Runtime/Sync 自助使用 Navigation Stack | Task/Approval 优先使用可扩展 Sheet；Recovery/撤销使用全屏安全流程 | 支持边缘返回；处理未提交意见；遵守 Safe Area 和键盘 |
| Android | `消息 / 活动 / 我的` Bottom Navigation；频道从消息列表推进 | Channel、Thread、File 与本机 Runtime/Sync 自助使用 Navigation Compose | Task/Approval 使用 Modal Bottom Sheet 或全屏目的地；Recovery/撤销使用全屏流程 | 支持系统和 Predictive Back；恢复目的地及滚动状态 |

Mobile 不复用 Desktop Rail、三栏 Task Workspace 或侧边 Inspector。语义一致通过相同状态、动作和审计收据达成，布局由各平台原生导航表达。

### 8.2 Route 到移动目的地

| Route | Mobile v1 适用性 | 目标 iOS/Android 目的地与职责 | 导航 Gap |
| --- | --- | --- | --- |
| `channel` | Required | 从消息列表推进到单列 Timeline；Composer、Thread、对象级 E2EE 状态 | 状态 Gap 见 `G-CH-*` |
| `inbox` | Required | 活动 Tab；提及、回复、Agent 结果和审批入口 | 状态 Gap 见 `G-IN-*` |
| `search` | Required | 根级 Search；本地索引优先，结果推进到对象 | 状态 Gap 见 `G-SE-*` |
| `tasks` | Required | Task Sheet 或全屏详情；发起、观察、暂停/中断，不执行长任务 | 状态 Gap 见 `G-TA-*` |
| `approvals` | Required | Approval Sheet 或全屏收据；批准、拒绝、冲突和过期 | 状态 Gap 见 `G-AP-*` |
| `task-result` | Required | Result 详情；摘要、Diff/Artifact、要求修改和接受 | `G-NAV-MOB-RESULT` |
| `files` | Required | 消息附件列表和 Preview；系统 File Picker/Share；不能访问 Desktop Workspace 路径 | `G-NAV-MOB-FILES` |
| `agents` | Read-only handoff | Task/消息内仅展示必要 Agent 身份、职责与健康；完整目录和 Policy 交给 Desktop/Admin Web | 无独立 Mobile Route Gap |
| `runtime` | Required self-service | “我的”下展示本机和 Execution Owner 状态；可诊断、取消/中断，并在发起新 Task 时选择获权 Desktop；既有 Run 的 Owner 转交仅在 Desktop/Admin Web 重新授权 | `G-NAV-MOB-RUNTIME`（仅状态与自助诊断，不含 Owner 转交） |
| `sync` | Required self-service | “我的”下的本机同步和恢复详情；Cursor、Outbox、Key/Recovery | `G-NAV-MOB-SYNC` |
| `organization` | Required basic | 组织选择器；每个组织使用隔离身份、数据库、索引和 Grant | `G-NAV-MOB-ORG` |
| `admin` | N/A | 完整治理属于 Desktop/Admin Web；Mobile 只消费已有审批与设备自助，不建立 Admin Route | 无独立 Mobile Route Gap |

### 8.3 平台职责差异

| 能力 | Desktop/Web | iOS | Android |
| --- | --- | --- | --- |
| 长任务 Runtime | Desktop 可托管授权的 `agentd`/`connectord` 并绑定 Workspace；Web 仅控制 | 只发起、观察、中断、审批 | 只发起、观察、中断、审批 |
| 本地文件 | 通过用户授权 Connector 和 Path Grant；显示精确范围 | 系统 File Picker/Share；复制到应用沙箱后加密 | Storage Access Framework/Share；复制到应用沙箱后加密 |
| 安全存储 | OS Keychain/Credential Store；Desktop Runtime 设备密钥 | Keychain/Secure Enclave 能力可用时使用；锁屏和重装状态显式 | Keystore/StrongBox 能力可用时使用；锁屏和重装状态显式 |
| 后台与通知 | 长连接、系统托盘和 Runtime 健康 | APNs 仅携带最小元数据；后台时间受限 | FCM 仅携带最小元数据；后台恢复由受限调度承担 |
| 管理 | 完整 Admin、审计和导出；高影响动作二次确认 | 关键提醒、审批和设备自助；完整策略管理转授权端 | 关键提醒、审批和设备自助；完整策略管理转授权端 |

## 9. 当前原型覆盖

Desktop 当前已有：

- README 中全部 12 个 Route 的正常态。
- 从频道创建 Agent Task。
- 发送频道消息。
- 一次性 Capability Grant 审批。
- 接受 Task Result 并创建 PR。
- Runtime Offline 模拟（等待本设备恢复，不自动漂移）。
- 同步 Gap Repair 模拟。
- Agent Participation Mode 和筛选切换。

Mobile 当前已有：

- 消息、活动、“我的”根导航。
- 搜索和无结果状态。
- 频道单列 Timeline。
- Task Activity Sheet。
- Approval Sheet。
- 发送消息和批准动作。

本任务只建立清点与状态契约，不要求修改正常态 HTML。

## 10. 后续 Gap 验收基线

每个 Gap Issue 至少验证：

1. Desktop 入口 `docs/prototype/index.html?screen=<route>`。
2. 同一根入口在 iOS 和 Android 代表宽度下的内部移动路由。
3. 不直接分发 `mobile/index.html`。
4. 状态出现、用户动作、状态恢复和返回后的 UI 连续性。
5. 键盘或触摸、焦点、读屏播报、200% 缩放或系统字体放大。
6. Runtime Offline 未被当作 IM Offline。
7. Permission、E2EE 和 Recovery 未被当作 Empty 或通用 Error。
8. 日志、诊断、搜索片段、通知和审计不包含正文、Prompt、Token、Key 或无权限文件内容。

## 11. 建议实施顺序

1. `G-DEV-REVOKED`、`G-CH-E2EE`、`G-SY-E2EE`、`G-AP-REC`：先固定不可绕过的安全状态。
2. `G-CH-OFF`、`G-TA-OFF`、`G-RU-OFF`、`G-AP-OFF`：固定 IM、Runtime、审批三个故障域。
3. `G-NAV-MOB-RESULT`、`G-NAV-MOB-FILES`、`G-NAV-MOB-RUNTIME`、`G-NAV-MOB-SYNC`、`G-NAV-MOB-ORG`：补齐 Frozen Scope 内核心 Mobile 目的地；不创建完整 Agent/Admin Mobile Route。
4. 各 Route 的 Loading、Empty、Error。
5. 各 Route 的 Permission、E2EE、Recovery 消费态。
