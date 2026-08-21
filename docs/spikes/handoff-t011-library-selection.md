# Agent Task Handoff

## Identity

- Issue: `#25`(T011 续做:P00-08 加密库选型验证)
- Workstream: `crypto-recovery`
- Branch: `agent/t011-crypto-spike-evidence`
- Base commit: `eec8746`(`agent/t011-e2ee-recovery-interop`)
- Head commit: 见本次提交

## Outcome

- Completed:
  - RFC 9420 Known Answer Test(crypto-basics / key-schedule / psk_secret,含负向控制)。
  - OpenMLS ↔ mls-rs 双向跨实现互操作,含跨实现设备移除。
  - Crash/Resume、`MlsGroup::delete` 密钥清除、随库持久化明文差距。
  - 群组规模性能剖面(2/8/32/128/256 leaf)与 KeyPackage 生成成本。
  - 2026-08-18 供应链重扫,并定位 6 条 RustSec 漏洞的唯一来源路径。
  - `tl-mls-1` 新增两条线上策略(handshake wire format、LeafNode lifetime),写入 golden vector 并锁为回归测试。
  - ADR-0004 与选型证据报告。
- Not completed(仍阻断 ADR-0003 转 `Accepted`):
  - 五平台真机与 FFI 驱动证据(属 P00-07/T010a)。
  - Recovery / History 的密码实现(只有语义层),属 P05-08。
  - 并发 Commit 与 Group Fork 收敛测试。
  - Retention 到期与备份恢复测试。
  - `transcript-hashes`/`welcome`/`tree-*`/`passive-client-*` 向量。
  - 独立密码与安全评审。
- Acceptance status: 本任务的验收命令全部通过;P00-08 Gate 整体仍未通过,清单见 ADR-0004。

## Changed Surfaces

- Owned paths: `spikes/e2ee-interop/`、`test/crypto/`、`docs/spikes/`、`docs/adr/`
- Contract changes: `test/crypto/e2ee-interop-v1.vector` 新增 5 个 key
  (`wire_format.handshake`、`leaf_lifetime.not_before_skew_seconds`、
  `leaf_lifetime.max_range_seconds`、`interop.independent_implementation`、
  `interop.independent_implementation_version`)。新增为追加,Swift/Kotlin harness 按 key 断言,不受影响。
- Migration changes: 无
- Generated or lockfile changes required from Integration Owner: 无。
  `spikes/e2ee-interop/interop-mls-rs/` 是独立 cargo workspace,带自己的 `Cargo.lock`,
  **未**加入根 `Cargo.toml` workspace,也未创建根 `Cargo.lock`。

## Verification

```text
cargo test --manifest-path spikes/e2ee-interop/rust/Cargo.toml --locked
  -> lib 3 passed / openmls_runtime 2 passed / persistence_resume 3 passed
     / rfc9420_kat 4 passed / perf_profile 2 ignored

cargo test --manifest-path spikes/e2ee-interop/interop-mls-rs/Cargo.toml --locked
  -> cross_implementation 5 passed

cargo test --manifest-path spikes/e2ee-interop/rust/Cargo.toml --locked --release -- --ignored --nocapture
  -> group_size_performance_profile / key_package_generation_profile passed,数据见报告第四节

cargo fmt --check / cargo clippy --all-targets   -> 两个 crate 均无 warning

cargo audit (spikes/e2ee-interop/rust)           -> 6 vulnerabilities, 2 warnings (0.8.1 依赖图)
cargo audit (openmls 0.9.0-rc.2 scratch graph)   -> 0 vulnerabilities, 1 warning

node spikes/e2ee-interop/generate-sbom.mjs
  -> sbom/cargo.cdx.json 154 components, sbom/interop-mls-rs.cdx.json 182 components
```

未运行:`swift run`(本机 Swift 6.3.3 与 macOS SDK 6.3.2 不匹配)、`./gradlew`(本机无 JRE/Gradle)。
`verify.sh` 已加入新 crate,但整脚本仍需要这两个工具链才能跑完。

## Security And Data

- Permissions or trust-boundary impact: 无运行时代码变更。ADR-0004 收紧了 `tl-mls-1` 的线上策略,
  handshake 改为 PrivateMessage 会减少投递服务可见的元数据。
- Message, file, prompt, token, or key handling impact:
  harness 不写出 MLS frame、解密内容、签名私钥或恢复私钥。KAT 向量是公开 IETF 数据。
  明文持久化检查只做结构判定,不打印任何取值。未启用 `crypto-debug` / `content-debug` feature。
- Logging and telemetry review: 新增测试只输出 PASS/FAIL 与性能数字;性能剖面输出的是耗时与字节数。

## Risks And Decisions

- Known risks:
  - ADR-0004 依赖尚未正式发布的 OpenMLS `0.9.0`。发布时间不受控,正式版必须重跑全套证据。
  - 84 天 LeafNode 上限意味着 KeyPackage 补货更频繁,Key Directory 容量策略需相应设计。
  - 性能数据来自 macOS 桌面 CPU,不能外推到移动端。
- Failed approaches worth preserving:
  - 先用两库默认配置做互操作,失败于 `Lifetime(NotCurrent)` 与 `IncompatibleWireFormat`。
    这两次失败本身是本轮最有价值的产出,已固化为两条回归测试,不要「修好就删」。
  - 用 `catch_unwind` 兜住 `0.8.1` 的 `debug_assert` panic 可以让 spike 通过,但不能作为生产方案,
    理由见 ADR-0004「考虑过的替代方案」。
- ADR or follow-up required:
  - ADR-0004 需 Architecture 与 Security Owner 评审签字。
  - ADR-0003 第 1 节的 `0.8.x` 起点被 ADR-0004 取代,合入后应在 ADR-0003 加注指向。

## Next

- Issues unblocked:
  - P02-04(Device / KeyPackage / Epoch / Recovery Envelope proto):lifetime 校验规则与 handshake
    wire format 已确定,可直接落字段与校验。
  - P05-05:加密 `StorageProvider` 确认为必须自建,可以开工设计。
  - P05-06:自更新阈值形状已确定(按 leaf 数分档)。
- Blocking condition and owner, if any:
  - ADR-0003 转 `Accepted` 仍被五平台真机(P00-07/T010a)与 Recovery 密码实现(P05-08)阻塞。
- Recommended next task:
  1. 并发 Commit / Group Fork 收敛测试(可在现有 rust harness 内完成,成本低)。
  2. `0.9.0` 正式版发布监控与证据重跑。
  3. Swift/Kotlin harness 补校验新增的两条 `tl-mls-1` 策略(需宿主工具链)。
