---
status: proposed
date: 2026-08-18
---

# ADR-0004: E2EE 加密库选型验证与 `tl-mls-1` 线上策略固定

M0 决策；由 Threadline Architecture 和 Security Workstream 评审。关联 Issue #25、P00-03、P00-08、
[ADR-0003](./0003-group-e2ee-recovery.md) 和 `docs/spikes/e2ee-library-selection.md`。

本 ADR 不重新决定协议或库，也不取代 ADR-0003。它记录 P00-08 spike 的可执行证据，据此收窄 ADR-0003
留下的三个开放点：候选库的**版本线**、`tl-mls-1` 的 **handshake wire format**、`tl-mls-1` 的
**LeafNode lifetime 策略**。

## 背景

ADR-0003 选定 MLS 1.0 / RFC 9420，首选 OpenMLS 作为 Rust spike 候选，并以 OpenMLS `0.8.x` 稳定线作为
评估起点。它把三件事显式留给 P00-08 证据：Commit/Welcome/Proposal 使用公开还是私密 Wire Format、
自更新策略阈值、以及库能否通过互操作与安全 Gate。

T011 spike(`docs/spikes/e2ee-interop.md`)完成了第一轮验证，结论是「spike 交付通过，OpenMLS `0.8.1`
生产准入不通过」，列出四条阻断项。第二轮验证(`docs/spikes/e2ee-library-selection.md`)针对其中三条补
证据，并首次让一个独立 RFC 9420 实现消费同一线上语义。

关键问题是:这些阻断项是否意味着要更换协议或更换库——即是否触发 ADR-0003 的退出条件与 8~18 周重排。

## 决策

### 1. 协议与库不变

MLS 1.0 / RFC 9420 与 OpenMLS 保持为候选，不触发 ADR-0003 的退出条件。证据是:上一轮记录的两条生产
阻断项都不是设计缺陷，而是所选版本线的缺陷，且都已在上游修复。

### 2. 版本线从 `0.8.x` 改为跟踪 `0.9.0`

ADR-0003 第 1 节「以 OpenMLS `0.8.x` 稳定发布线作为评估起点」在本 ADR 中被取代：

- 实现任务不得以 `0.8.1` 为基线。该版本在损坏密文路径上有 `debug_assert!(false)`
  (`framing/private_message_in.rs:136`),任何 debug profile 构建都会被一帧攻击者可控的损坏密文打成
  panic。生产密码边界不应依赖 `catch_unwind` 兜底。
- `0.9.0-rc.2` 在同一路径返回普通错误(A/B 实测,见 spike 报告第一节与复现步骤),并把依赖链从
  `hpke-rs 0.6.1 -> libcrux-* 0.0.7/0.0.8` 升到 `hpke-rs 0.7.0 -> libcrux-* 0.0.9`,使
  `cargo-audit` 结果从 6 个漏洞(4 High)变为 0。
- 因此:`client-crypto` 的实现任务锁定 OpenMLS `0.9.0` **正式版**;在正式版发布前，实现任务可以基于
  `0.9.0-rc` 开发，但不得进入任何交付制品。`0.9.0` 正式发布后必须重跑本 ADR 引用的全部证据，rc 上的
  结果不自动继承。
- 若 `0.9.0` 正式版在实现窗口内未发布，退路是携带 `0.8.1` 的最小上游补丁(移除该 `debug_assert`)并
  单独评估 libcrux 漏洞的实际可利用性，而不是接受现状。这条退路必须经 Security Owner 批准。

### 3. `tl-mls-1` 固定 handshake wire format 为 PrivateMessage

ADR-0003 把这一项留给 T019 与 P00-08 证据冻结。现在冻结为：

```text
tl-mls-1
`- Handshake wire format: PrivateMessage (Commit / Proposal)
```

理由有二。互操作:OpenMLS 默认 `PURE_CIPHERTEXT_WIRE_FORMAT_POLICY`,直接以
`IncompatibleWireFormat` 拒绝 mls-rs 默认发出的 PublicMessage Commit;不固定就没有跨实现互通。
元数据:PrivateMessage 让投递服务看不到 Proposal 内容与发送方 leaf，与 ADR-0003「服务端只见必要 C2
元数据」一致。

Welcome 不受此项约束(RFC 9420 中 Welcome 本身没有 Public/Private 形式)。

### 4. `tl-mls-1` 固定 LeafNode Lifetime 策略

```text
tl-mls-1
|- not_before  = now - 3600s        (时钟偏移余量)
`- not_after - not_before <= 7261200s  (1h + 84d)
```

两个 RFC 9420 实现的默认值互不接受:mls-rs `0.55.3` 默认 `not_before = now`、范围 365 天;OpenMLS
`0.8.1` 要求 `not_before` 严格早于 `now`,且范围不超过 1h + 84d。默认值不能作为跨端契约。

该策略是**发送侧与校验侧的共同规则**:Threadline 客户端按此签发 KeyPackage 与 LeafNode，并按此校验
收到的对象。服务端 Key Directory 在不读取密码状态的前提下拒收超出范围的 KeyPackage。

### 5. 本地持久化必须由 Threadline 实现

候选库随附的持久化(`openmls_memory_storage` 的 `persistence` feature)把整个 key store 以 base64
JSON 明文落盘。它在 spike 中可用，不构成任何生产选项。`client-crypto` 必须实现自己的加密
`StorageProvider`,包装密钥由 OS Secure Storage 持有，符合 ADR-0003 第 4 节与第 5 节。这不是可选优化，
而是 P05-05 的前置条件。

### 6. mls-rs 确认为可用退路

ADR-0003 把 `mls-rs` 列为互操作对照而非首选生产依赖。本轮证据表明 `mls-rs 0.55.3` 能与 `tl-mls-1` 双向
互通并正确执行成员移除，因此它是**真实可执行的退路**而不是纸面选项。它仍不是首选:替换会改变
`client-crypto` Adapter 内部实现与依赖审计范围，且 mls-rs 自述尚未完成第三方安全审计。

### 7. 自更新策略阈值按 leaf 数分档

ADR-0003 要求自更新阈值由 M0 测量后确定、不写死在工作流代码。测量结果:commit 体积随 leaf 数线性
增长(2 leaf 708 B，256 leaf 21787 B),而 CPU 成本在 256 leaf 时仍低于 3 ms。因此约束是**广播体积**而
非计算量，阈值必须按 leaf 数分档并可配置。具体档位由 P05-06 在移动端真机数据到位后确定，本 ADR 只
固定「按 leaf 数分档、不使用全局固定值」这一形状。

## 明确不包含

- 把 ADR-0003 升级为 `Accepted`。五平台真机、Recovery 解密、History Sharing 实现、并发 Commit/Group
  Fork 覆盖和独立密码评审仍然缺失。
- OpenMLS `0.9.0` 或任何 Crypto Provider 的生产安全背书。
- Recovery Envelope 与 History Envelope 的密码构造批准。
- 移动端内存与电量结论。本轮全部数据取自 macOS 主机。
- Protobuf 字段定义。本 ADR 只固定语义策略，字段由 T019 / P02-04 定义。

## P00-08 剩余验收项

本 ADR 覆盖后，ADR-0003 的 P00-08 Gate 清单状态如下：

| Gate 项 | 状态 |
| --- | --- |
| RFC 9420 Known Answer Test | 部分通过(crypto-basics / key-schedule / psk_secret;tree、welcome、passive-client 未覆盖) |
| OpenMLS 互操作测试 | 通过(与 mls-rs 双向) |
| Threadline Golden Vector | 通过 |
| 五平台构建与最小 Host Harness | **未通过**(仅 macOS 主机 Rust;Swift/Kotlin/iOS/Android 属 P00-07) |
| Device add/remove/revoke、离线乱序、未知 Profile、回滚 | 通过 |
| 并发 Commit、Group Fork | **未覆盖** |
| History Sharing、Recovery Envelope 协议说明与负向测试 | 语义层通过，密码实现未开始 |
| Crash/Resume、加密持久化、Key 清除 | Crash/Resume 与 Key 清除通过;加密持久化**未实现**(见决策 5) |
| Retention 到期、备份恢复 | **未覆盖** |
| 独立密码与安全评审 | **未开始** |
| 性能:群组规模、Epoch 频率、KeyPackage 消耗 | 桌面端通过;移动端内存压力**未覆盖** |

## 退出与回退条件

本 ADR 在以下任一情况下必须重开：

- OpenMLS `0.9.0` 正式版重跑证据时，损坏密文路径或依赖漏洞回归。
- `0.9.0` 正式版在实现窗口内未发布，且 Security Owner 不批准补丁 `0.8.1` 的退路。
- 五平台构建证据显示 `0.9.0` 在 iOS 或 Android 上无法稳定构建或持久化。
- 固定 handshake wire format 或 lifetime 策略后仍无法与独立实现互通。
- 加密 `StorageProvider` 无法在某个平台满足 ADR-0003 第 4 节的密钥保护要求。

任何情况下都不得以「退化为服务端明文」「服务端保存 MLS Ratchet Secret」或「放宽 Envelope 校验」来
绕过上述条件。

## 考虑过的替代方案

### 继续以 `0.8.1` 为基线，靠 Adapter 兜住 panic

拒绝。`catch_unwind` 只能在 spike 里证明可隔离。把攻击者可控的输入路径上的 panic 当作正常错误处理，
等于把密码边界的失败模式交给 unwind 语义，而 `client-crypto` 未来要通过 FFI 暴露给 Swift/Kotlin 宿主，
跨语言 unwind 不是可依赖的行为。libcrux 的 4 个 High 也不会因此消失。

### 直接改用 mls-rs 作为首选

拒绝。互操作证据表明它可用，但这会把 ADR-0003 已经完成的库评估、依赖审计范围和 Adapter 设计全部重置，
换来的只是同一批问题的另一组默认值。mls-rs 自述未完成第三方安全审计,不构成升级理由。保留为退路。

### 把 handshake wire format 留给实现期决定

拒绝。它决定服务端能看到多少元数据，也决定跨实现能否互通，属于线上契约而非实现细节。留到实现期意味着
两端各自采用库默认值，而实测证明两个库的默认值互斥。

### 依赖库默认 lifetime，仅在校验侧放宽

拒绝。放宽校验意味着接受 365 天的 LeafNode 有效期，与设备撤销和 Retention 的时间约束冲突。策略必须在
签发侧和校验侧同时固定。

## 后果

### 正面

- ADR-0003 的选型不必重开，M0 不因加密库触发 8~18 周重排。
- 两条上一轮的生产阻断项有了明确、可验证的关闭路径,而不是长期挂账。
- `tl-mls-1` 多了两条可执行、可回归的线上策略,P02-04 的 proto 校验规则有了确定输入。
- 退路(mls-rs)从纸面选项变成已验证选项。

### 负面

- 依赖一个尚未正式发布的上游版本。`0.9.0` 的发布时间不在 Threadline 控制范围内，正式版发布后必须重跑
  全套证据。
- 固定 PrivateMessage handshake 会让服务端更难在不解密的情况下做排序辅助校验,排序与幂等只能依赖外层
  Envelope 字段。
- 固定 84 天 lifetime 上限意味着 KeyPackage 需要更频繁地补货,Key Directory 的容量与补货策略要相应设计。
- 加密 `StorageProvider` 是 P05 必须自建的额外工作量,不能靠上游。
