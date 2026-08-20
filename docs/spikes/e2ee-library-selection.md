# P00-08 加密库选型验证证据

- Issue: `#25`(T011 续做)
- 日期: 2026-08-18
- ADR: `docs/adr/0003-group-e2ee-recovery.md`、`docs/adr/0004-e2ee-crypto-library-selection.md`
- Profile: `tl-mls-1`
- 被验证候选: OpenMLS `0.8.1`(ADR-0003 指定起点)、OpenMLS `0.9.0-rc.2`、mls-rs `0.55.3`(ADR-0003 指定对照/退路)

本文接续 `docs/spikes/e2ee-interop.md`。上一轮 spike 通过了交付，但明确列出四条不能批准生产接入的
阻断项。本轮针对其中三条补证据，并给出选型结论。第四条(Recovery Envelope 只验证语义、未实现恢复
解密)保持不变，属于 P05-08 与独立密码评审范围。

## 结论

**候选协议与库不变(MLS 1.0 / OpenMLS)；被验证的版本线要变。**

ADR-0003 把 OpenMLS `0.8.x` 稳定线作为评估起点。本轮证据显示上一轮记录的两条生产阻断项都不是
OpenMLS 的设计问题，而是 `0.8.1` 这条版本线的问题，且都已在 `0.9.0-rc.2` 上游修复：

1. 损坏密文的 `debug_assert!` panic —— `0.8.1` 复现，`0.9.0-rc.2` 返回普通错误。
2. 依赖链 6 个 RustSec 漏洞(4 High)—— 全部来自 `openmls_rust_crypto 0.5.1 -> hpke-rs 0.6.1 ->
   libcrux-*`；`0.9.0-rc.2` 的 `hpke-rs 0.7.0 -> libcrux-* 0.0.9` 依赖图零漏洞。

第三条(缺独立 RFC 9420 实现互操作证据)本轮已补齐：OpenMLS 与 mls-rs 在同一 `tl-mls-1` 群组上双向
互通。互通过程中发现两处**默认配置不互操作**的问题，必须写进 `tl-mls-1` 而不是依赖任一库的默认值。

因此 ADR-0003 仍不能升为 `Accepted`(五平台、真机、恢复解密和独立密码评审仍缺)，但选型本身不需要
重开：不必更换协议，也不必更换库，只需把版本线从 `0.8.1` 改为跟踪 `0.9.0` 正式版。

## 一、RFC 9420 Known Answer Test

`spikes/e2ee-interop/rust/tests/rfc9420_kat.rs`。向量来自 IETF `mlswg/mls-implementations`，按 cipher
suite 1 过滤，出处与校验和见 `spikes/e2ee-interop/vectors/rfc9420/README.md`。

harness 直接按 RFC 9420 重写标签构造与 TLS presentation language 编码，只调用 provider 的
`OpenMlsCrypto` 原语，因此验证的是候选 crypto provider 与 RFC 的一致性，而不是 OpenMLS 与自己的一致性。

| 向量 | 覆盖内容 | 结果 |
| --- | --- | --- |
| `crypto-basics` | `RefHash`、`ExpandWithLabel`、`DeriveSecret`、`DeriveTreeSecret` | PASS |
| `crypto-basics` | `SignWithLabel`:验签通过，且 Ed25519 逐字节复现向量签名 | PASS |
| `crypto-basics` | `EncryptWithLabel`:解开向量密文、重新封装往返、换标签必须失败 | PASS |
| `key-schedule` | 5 个 epoch 的 `GroupContext` 编码 | PASS |
| `key-schedule` | `joiner`/`welcome`/`epoch` 与 9 个派生 secret 全链 | PASS |
| `key-schedule` | `external_pub = KEM.DeriveKeyPair(external_secret)` | PASS |
| `key-schedule` | `MLS-Exporter(label, context, length)` | PASS |
| `psk_secret` | 0 至 10 个 external PSK 的 `PSKLabel` 与链式 Extract | PASS(11 例) |

harness 自带负向控制(`kat_harness_detects_a_wrong_label`):换标签必须得到不同派生结果，篡改 AEAD
密文必须被拒绝。没有这条，前面的 PASS 不能证明 harness 有分辨能力。

尚未覆盖:`transcript-hashes`、`welcome`、`messages`、`tree-*`、`treekem`、`passive-client-*`。这些需要
向群组状态注入私钥或驱动库内部结构，OpenMLS 只在 `test-utils` feature 下开放。补齐路径见
`spikes/e2ee-interop/vectors/rfc9420/README.md`。

## 二、独立实现互操作:OpenMLS ↔ mls-rs

`spikes/e2ee-interop/interop-mls-rs/tests/cross_implementation.rs`。两个库跨进程边界只交换序列化的
`MLSMessage`，不共享任何库类型。

| 场景 | 结果 |
| --- | --- |
| mls-rs 建群 → OpenMLS 用 KeyPackage 加入 → 双向应用消息 → OpenMLS commit 推进 epoch | PASS |
| OpenMLS 建群 → mls-rs 用 KeyPackage 加入 → 双向应用消息 → mls-rs commit 推进 epoch | PASS |
| 每次 epoch 变化后双方 `export_secret` 逐字节一致 | PASS |
| OpenMLS 移除 mls-rs 设备:被移除方自认离开，且无法解密后继 epoch | PASS |

这同时是对 ADR-0003 退路的验证:`mls-rs 0.55.3` 能与 `tl-mls-1` 线上语义互通，退路是真实可用的，
不是纸面选项。

### 发现 1:LeafNode Lifetime 默认值不互操作

- OpenMLS `0.8.1` 要求 `not_before < now < not_after`(严格小于)，并把 `not_after - not_before` 上限
  设为 1 小时 + 84 天(`DEFAULT_KEY_PACKAGE_LIFETIME_MARGIN + DEFAULT_KEY_PACKAGE_LIFETIME`)。
- mls-rs `0.55.3` 默认 `not_before = now`(无时钟偏移余量)，范围 365 天。
- 结果:mls-rs 默认生成的 KeyPackage 与 LeafNode 被 OpenMLS 拒绝(`Lifetime(NotCurrent)`)。同一秒内
  创建的 leaf 即使把范围改短仍会被拒，因为 `not_before` 不严格早于 `now`。

`tl-mls-1` 因此必须固定 lifetime 策略，不能继承任一库默认值:`not_before` 回拨 3600 秒时钟偏移余量，
`not_after - not_before` 不超过 7261200 秒。该策略已写入 `test/crypto/e2ee-interop-v1.vector`
(`leaf_lifetime.*`),并由 `default_lifetime_policies_are_not_interoperable` 锁定为回归测试。

### 发现 2:Handshake Wire Format 默认值不互操作

- OpenMLS 默认 `PURE_CIPHERTEXT_WIRE_FORMAT_POLICY`,拒绝 PublicMessage handshake。
- mls-rs 默认 `encrypt_control_messages = false`,以 PublicMessage 发送 Commit。
- 结果:OpenMLS 以 `IncompatibleWireFormat` 拒绝 mls-rs 的 Commit。

ADR-0003 第 1 节把「Commit、Welcome、Proposal 用公开还是私密 Wire Format」留给 T019 与 P00-08 证据
冻结。本轮据此给出结论:`tl-mls-1` 固定 handshake 使用 **PrivateMessage**。除了互操作，这也让投递服务
看不到 Proposal 内容与发送方 leaf，与 ADR-0003「服务端只见必要 C2 元数据」的方向一致。已写入向量
(`wire_format.handshake=private-message`),并由
`plaintext_control_messages_are_rejected_by_openmls_defaults` 锁定。

两条发现都不是库缺陷，而是**默认值不能作为跨端契约**的证据。它们直接影响 P02-04 的 Device / KeyPackage
/ Epoch proto 字段与校验规则。

## 三、Crash/Resume 与密钥清除

`spikes/e2ee-interop/rust/tests/persistence_resume.rs`。上一轮 spike 全程在内存中运行，重启从未被建模。

| 场景 | 结果 |
| --- | --- |
| 设备在 epoch 1 快照后「崩溃」，从序列化存储重启,`MlsGroup::load` 恢复到同一 epoch | PASS |
| 重启后的设备仍能解密对端实时消息 | PASS |
| 重启后的设备仍能自行 commit(说明私有 tree 状态而非仅公开状态被恢复),对端接受 | PASS |
| `MlsGroup::delete` 后群组状态不可再加载 | PASS |
| 随库提供的持久化输出是明文 | PASS(记录为差距,见下) |

harness 自建 `SpikeProvider`(`RustCrypto` + `MemoryStorage`),正是 ADR-0003 给 `client-crypto` 划的边界:
存储归 Threadline，协议归库。没有这个拆分，重启根本无法表达。

**差距**:`openmls_memory_storage` 的 `persistence` feature 把整个 key store 以 base64 JSON 明文落盘，
没有包装密钥、没有 AEAD、没有 KDF。它适合 spike，不能上线。ADR-0003 第 4/5 节要求本地密钥材料位于
加密本地存储、包装密钥由 OS Secure Storage 持有，因此 P05 必须自己实现加密 `StorageProvider`,这不是
可选优化。`shipped_persistence_is_unencrypted_and_cannot_be_shipped` 以结构检查(不打印任何值)锁定该
事实,上游若加密即失败提醒复核。

## 四、性能剖面

`spikes/e2ee-interop/rust/tests/perf_profile.rs`,`--release`,Apple Silicon,单进程。默认 `#[ignore]`。

```
cargo test --manifest-path spikes/e2ee-interop/rust/Cargo.toml --locked --release -- --ignored --nocapture
```

| leaves | add commit (ms) | self-update (ms) | encrypt (ms) | decrypt (ms) | welcome (B) | commit (B) |
| --- | --- | --- | --- | --- | --- | --- |
| 2 | 1.009 | 0.363 | 0.066 | 0.077 | 378 | 708 |
| 8 | 0.836 | 0.674 | 0.061 | 0.072 | 378 | 1272 |
| 32 | 0.932 | 0.898 | 0.042 | 0.050 | 378 | 3310 |
| 128 | 1.653 | 1.676 | 0.036 | 0.041 | 378 | 11252 |
| 256 | 2.718 | 2.721 | 0.042 | 0.053 | 378 | 21787 |

KeyPackage:100 个生成耗时 13.0 ms(每个 0.130 ms),序列化后每个 290 字节。

读法:

- 计算量不是瓶颈。256 leaf 群组的 commit 仍在 3 ms 内，应用消息加解密与群组规模基本无关。
- **Commit 体积随 leaf 数线性增长**,256 leaf 时约 21.8 KB，且每次成员变更或自更新都要广播给所有设备。
  这是移动端流量与电量的主要成本，也是 ADR-0003 留给 M0 测量的「自更新策略阈值」的真实约束:阈值应
  按 leaf 数分档，而不是全局固定。
- Welcome 恒为 378 B，因为默认配置不携带 ratchet tree extension，树走带外。若为跨实现加入便利而打开
  该 extension(互操作 harness 就是这么做的),Welcome 会随群组规模增长，需要在 T019 中明确取舍。
- 本剖面在桌面 CPU 上跑，**不能替代移动端内存压力测试**。iOS/Android 真机数字属于 P00-07/P00-08 剩余
  部分。

## 五、供应链

`cargo-audit 0.22.2` 于 2026-08-18 重新扫描。

| 依赖图 | 结果 |
| --- | --- |
| OpenMLS `0.8.1`(155 crates) | 6 vulnerabilities(4 High、2 Medium)+ 2 warnings |
| OpenMLS `0.9.0-rc.2`(197 crates) | **0 vulnerabilities**,1 unmaintained warning(`proc-macro-error2`) |

`0.8.1` 的全部 6 条都来自同一条路径:

```
openmls_rust_crypto 0.5.1 -> hpke-rs 0.6.1 -> libcrux-sha3 0.0.8 / libcrux-secrets 0.0.5
                                            / libcrux-chacha20poly1305 0.0.7 / libcrux-aesgcm 0.0.7
```

`0.9.0-rc.2` 升到 `hpke-rs 0.7.0 -> libcrux-* 0.0.9`,该路径全部清零。mls-rs 自身依赖图不涉及这些
libcrux 版本(互操作 crate 的 lock 里出现 libcrux 是因为它同时依赖 OpenMLS,已用 `cargo tree -i` 确认)。

## 六、跨宿主契约的覆盖范围

新增的 `wire_format.handshake` 与 `leaf_lifetime.*` 已写入 `test/crypto/e2ee-interop-v1.vector`,目前
只有 Rust harness 校验它们。Swift 与 Kotlin harness 采用「按 key 断言」方式，新增 key 不会导致失败，
但也不会被校验。本机缺 JRE/Gradle、Swift 6.3.3 与 SDK 6.3.2 不匹配，因此本轮不修改这两个 harness,
避免提交未经运行的代码。补齐属于 P00-07/T010a 的宿主工作。

## 七、仍然缺失的证据

本轮不改变以下结论:它们仍然阻断 ADR-0003 升为 `Accepted`。

1. **Recovery Envelope 仍只有语义层验证**。没有实现恢复解密,没有接 KMS/HSM,没有 Recovery Recipient
   密钥托管。属于 P05-08 与独立密码评审。
2. **History Sharing 同样只有语义层**。历史密钥导出与重新封装未实现。
3. **五平台真机证据缺失**。Swift harness 因本机 Swift 6.3.3 与 macOS SDK 6.3.2 不匹配仍无法 build/run;
   iOS/Android 真机与 FFI 驱动属于 P00-07/T010a。本轮所有证据都在 macOS 主机 Rust 上取得。
4. **并发 Commit 与 Group Fork 未覆盖**。上一轮覆盖了乱序、重放与旧 epoch，但没有两个设备同时 commit
   后按服务端顺序收敛的测试。
5. **独立密码与安全评审未开始**。
6. **`0.9.0` 尚未正式发布**。本轮针对 rc 的证据不能替代对正式版重跑全套。

## 八、复现

```sh
cargo test --manifest-path spikes/e2ee-interop/rust/Cargo.toml --locked
cargo test --manifest-path spikes/e2ee-interop/interop-mls-rs/Cargo.toml --locked
cargo test --manifest-path spikes/e2ee-interop/rust/Cargo.toml --locked --release -- --ignored --nocapture
cargo audit --file spikes/e2ee-interop/rust/Cargo.lock
```

版本线对照(`0.8.1` panic、`0.9.0-rc.2` 不 panic)的复现方式:把 `spikes/e2ee-interop/rust/Cargo.toml` 的
OpenMLS 三个 pin 临时改为 `=0.9.0-rc.2` / `=0.6.0-rc.2` / `=0.6.0-rc.2`,并把
`corrupted_ciphertext_is_contained_at_the_adapter_boundary` 的断言收紧为 `matches!(outcome, Ok(Err(_)))`
(即禁止 panic),在 debug 下运行。仓库内保持稳定版 pin,不引入 rc 依赖。

## 日志与文件安全

- 新增 harness 不写入 MLS frame、解密后内容、签名私钥或恢复私钥。
- KAT 向量是公开 IETF 数据。断言失败时打印的是向量期望值与实际派生值——这是公开测试数据，不是
  Threadline 密钥。
- 明文持久化检查只做结构判定(base64/JSON 形状),不打印、不断言任何取值。
- 未启用 OpenMLS 的 `crypto-debug` 或 `content-debug` feature。
