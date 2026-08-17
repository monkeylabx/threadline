# T011 Group E2EE 与 Recovery 互操作 Spike

- Issue: `#25`
- 日期: 2026-08-10
- ADR: `docs/adr/0003-group-e2ee-recovery.md`
- Profile: `tl-mls-1`
- 候选库: OpenMLS `0.8.1`
- 密码套件: `MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519`

## 结论

**Spike 交付通过；OpenMLS 0.8.1 生产准入不通过。**

OpenMLS 能完成本次 Rust 运行时场景，但当前证据不能批准生产接入。阻断项是：

1. OpenMLS `0.8.1` 的损坏密文路径在 debug 构建中触发 `debug_assert!` panic；release 构建返回错误。Threadline adapter 可以在 spike 中隔离 panic，但生产密码边界不应依赖 `catch_unwind`。
2. 2026-08-10 的 RustSec 扫描报告 6 个漏洞，其中 `libcrux-secrets 0.0.5` 与 `libcrux-sha3 0.0.8` 通过 `hpke-rs -> openmls_rust_crypto` 位于实际依赖路径，包含 4 个 High；另有 2 个 Medium 与 1 个 High 位于锁文件但未出现在当前目标的 Cargo feature graph。两个 `libcrux-aesgcm` Medium 暂无修复版本。
3. Swift 与 Kotlin harness 验证了同一版本化语义向量，但尚未通过真实 FFI 驱动 OpenMLS，也没有用独立 MLS 实现消费同一 RFC transcript。因此它不是 ADR 所要求的独立实现互操作证据。
4. Recovery Envelope 只验证版本、绑定、授权和稳定拒绝语义；没有实现 Recovery Recipient 密钥托管或恢复解密。这是刻意限制，避免在 spike 中自创密码协议。

因此 ADR-0003 不应转为 Accepted。后续必须重新打开候选库/版本比较；服务端明文回退继续被禁止。

## 交付物

- 公共 Golden Vector: `test/crypto/e2ee-interop-v1.vector`
- Rust/OpenMLS harness: `spikes/e2ee-interop/rust/`
- Swift harness: `spikes/e2ee-interop/swift/`
- Kotlin/JVM harness: `spikes/e2ee-interop/kotlin/`
- CycloneDX SBOM: `spikes/e2ee-interop/sbom/cargo.cdx.json`
- 一键入口: `spikes/e2ee-interop/verify.sh`

Golden Vector 是 library-independent 的外部语义契约，不暴露 OpenMLS 类型。它包含 Epoch 状态、预期动作、稳定错误码和 Recovery Envelope 公共绑定字段；不包含消息内容、私钥、恢复私钥或真实密文。

## 验证结果

| 场景 | 结果 | 证据与限制 |
| --- | --- | --- |
| KeyPackage 与 Welcome | PASS | Rust 使用 OpenMLS 0.8.1 生成、存储并消费 KeyPackage；Bob 从 Welcome 加入 Epoch 1。 |
| Epoch 连续推进 | PASS | 创建 0、加人 1、两次更新 2/3、离线设备加入 4、撤销设备 5。 |
| 乱序 Commit | PASS（应用层需排队） | Bob 在 Epoch 1 收到 Epoch 3 Commit 时被拒；处理前序 Commit 后可顺序合并。adapter 对外映射 `TL_E2EE_FUTURE_EPOCH`。 |
| replay / old epoch | PASS | 已合并 Commit 重放与旧 Epoch application frame 均被 OpenMLS 拒绝；对外语义固定为 `TL_E2EE_REPLAY` / `TL_E2EE_OLD_EPOCH`。 |
| offline / new device | PASS | Charlie 的 KeyPackage 在群组更新前生成，在 Epoch 4 使用 Welcome 加入当前群组。 |
| device revocation | PASS | 移除 Bob 后 Epoch 进入 5，Bob group inactive，无法处理后续 application frame。 |
| corrupt frame | FAIL（生产阻断） | release 返回 AEAD error；debug 在 `private_message_in.rs` 的失败路径 panic。harness 仅证明 adapter 可隔离，不代表库满足生产错误边界。 |
| History Sharing | PASS（语义层） | 授权、未授权、跨租户结果由三端共同向量固定；尚未实现历史密钥导出/加密。 |
| Recovery 成功/拒绝/失败 | PASS（语义层） | 可选 wrapper、tenant/group/epoch/recipient/payload digest 绑定与确定性错误已验证；未接 KMS/HSM，不持有 Recovery 私钥。 |
| Rust host | PASS | `cargo test --locked --offline`: 5 passed。 |
| Kotlin host | PASS | Kotlin 2.4.10 / Gradle 9.5.1: Golden Vector 与 SHA-256 binding 验证通过。 |
| Swift host | PARTIAL | `swiftc -frontend -parse` 通过；恢复后的本机 Swift 6.3.3 与 macOS SDK 6.3.2 不匹配，无法完成 build/run。 |
| 独立 RFC 实现互操作 | FAIL（生产阻断） | 尚未让 mls-rs 或 IETF interop client 消费同一 transcript。 |

## Recovery Envelope 边界

本 spike 固定以下应用层规则，不修改 MLS：

- `Recovery Recipient` wrapper 是可选的；不存在 recipient 时返回 `TL_E2EE_RECOVERY_UNAVAILABLE`，绝不降级到服务端明文。
- Recovery 私钥位置固定为 `external-kms-hsm-only`，不会进入客户端或应用服务。
- binding digest 覆盖 wrapper version、tenant、group、epoch、recipient key id 和 payload digest。
- 未知版本、跨租户、跨群组、旧 Epoch、损坏数据、未授权请求都产生稳定 `TL_E2EE_*` 错误。
- 成功结果只表示策略与绑定接受，不表示本 spike 已实现恢复解密或批准 KMS/HSM 方案。

## 日志与文件安全

- Golden Vector 只含公共 metadata、SHA-256 digest 和合成状态值。
- harness 不把 MLS frame、解密后的 application content、签名私钥或 Recovery 私钥写入普通文件、日志或诊断包。
- 未启用 OpenMLS 的 `crypto-debug` 或 `content-debug` features。
- 测试 payload 是内存中的合成固定字节；测试输出只报告 PASS/FAIL，不输出 payload 或 ciphertext。

## 供应链证据

Rust 依赖全部由独立 `Cargo.lock` 固定；CycloneDX SBOM 收录 152 个 registry components 及 crates.io SHA-256。关键 pin：

| crate | version | crates.io SHA-256 |
| --- | --- | --- |
| `openmls` | `0.8.1` | `dcb512bfe6a55777518853ea535c6241f069cb0e8984678c117151d2a1e7e903` |
| `openmls_basic_credential` | `0.5.0` | `983e8be1457dd6f316f409292cec334af3b57b49a19deadc925c83c3c35e15b6` |
| `openmls_rust_crypto` | `0.5.1` | `fafcc8a3552b10fbb3ab757cccaf1a34081e826ca819f49aa7e6645b1d95c00f` |

RustSec `cargo-audit 0.22.2`（1199 advisories）结果：6 vulnerabilities、2 allowed warnings。

| advisory | package | severity | 当前判断 |
| --- | --- | --- | --- |
| RUSTSEC-2026-0212 | `libcrux-secrets 0.0.5` | High 8.2 | 位于 active HPKE graph；阻断。 |
| RUSTSEC-2026-0207 | `libcrux-sha3 0.0.8` | High 8.2 | 位于 active HPKE graph；阻断。 |
| RUSTSEC-2026-0208 | `libcrux-sha3 0.0.8` | High 8.2 | 位于 active HPKE graph；阻断。 |
| RUSTSEC-2026-0124 | `libcrux-chacha20poly1305 0.0.7` | High 8.2 | 锁文件命中；当前 target graph 未显示，仍需清除或证明不可达。 |
| RUSTSEC-2026-0209 | `libcrux-aesgcm 0.0.7` | Medium 6.3 | 锁文件命中，无修复版本。 |
| RUSTSEC-2026-0211 | `libcrux-aesgcm 0.0.7` | Medium 6.3 | 锁文件命中，无修复版本。 |

OpenMLS `0.8.1` 是带 verified signature 的正式 release，且包含 0.8.0 安全修复后的依赖更新；这不足以抵消当前扫描结果。作为比较候选，mls-rs 声明符合 RFC 9420 并提供 interop vectors，但也明确尚未接受完整第三方安全审计；其 RustCrypto provider 仍标为 experimental。

## 复现命令

```sh
cargo test --manifest-path spikes/e2ee-interop/rust/Cargo.toml --locked
swiftc -frontend -parse \
  spikes/e2ee-interop/swift/Sources/T011SwiftHarness/main.swift
./gradlew -p spikes/e2ee-interop/kotlin run \
  --args="$PWD/test/crypto/e2ee-interop-v1.vector"
cargo audit --file spikes/e2ee-interop/rust/Cargo.lock
node spikes/e2ee-interop/generate-sbom.mjs
```

本机 Swift 完整运行失败的可复现原因：Command Line Tools 提供 Swift `6.3.3`，SDK module 由 Swift `6.3.2` 构建；需要恢复仓库固定的 Xcode `26.6` 后重跑。

## 必须的后续动作

1. 由 Security Owner 重新打开 ADR-0003 候选库/版本决策；不得接受当前 0.8.1 组合。
2. 评估更新后的 OpenMLS/provider 组合或 mls-rs，并要求 `cargo audit` 无未豁免漏洞；任何豁免必须有 reachability 与 Security Owner 批准。
3. 消除损坏密文 debug panic；不以 `catch_unwind` 作为生产修复。
4. 建立 library-independent adapter/FFI，让 Rust、Swift、Kotlin host 驱动相同真实 MLS transcript。
5. 引入独立 RFC 9420 实现进行 byte-level transcript 互操作，并恢复 Xcode 26.6 后完成 Swift runtime 证据。
6. Recovery 继续保持关闭，直到 KMS/HSM、审批、审计、双人控制和恢复密钥轮换风险项完成。
