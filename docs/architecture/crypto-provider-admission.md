# Production Crypto Provider admission evidence

Status: Draft evidence plan for #191; **M0/G0 HOLD, production admission NOT RUN**.
Reviewed: 2026-09-07. Repository baseline: `66a1e75325f05f8ef6c53c2e3f88f185567ebbab`.
This document inventories evidence; it does not accept an ADR, approve a library,
change the frozen scope, or implement cryptography.

## Authority and evidence rules

[ADR-0003](../adr/0003-group-e2ee-recovery.md) and
[ADR-0004](../adr/0004-e2ee-crypto-library-selection.md) remain proposed.
The protocol candidate remains MLS 1.0 / `tl-mls-1`, with OpenMLS behind the
[client-crypto boundary](../../crates/client-crypto/src/lib.rs). That boundary
currently contains a version constant, not a production provider.
[SQLCipher's source override](../../third_party/libsqlite3-sys/UPSTREAM.md)
fixes a database dependency issue; it supplies neither an MLS StorageProvider
nor OS key custody or a complete encrypted messaging implementation.

In the matrix, PASS means only the named historical test/report passed at its
recorded scope. FAIL means observed failure in that version or an explicit
admission failure. NOT RUN means required production evidence is absent. A report
commit identifies the source of a claim, not a new signed test attestation.
No runtime, mobile, KMS, vulnerability audit or independent crypto review was
rerun for this documentation task. A later version must not inherit a PASS.

Evidence sources:

- **E1**: [T011 initial report](../spikes/e2ee-interop.md), dated 2026-08-10;
  historical OpenMLS 0.8.1 results, superseded only where E2 supplies evidence.
  The reviewed copy is at the baseline above; its old error and audit claims
  are historical observations, not today's vulnerability inventory.
- **E2**: [library-selection report](../spikes/e2ee-library-selection.md) and
  [handoff](../spikes/handoff-t011-library-selection.md), report/code commit
  `2b39382a1b0a050e466aa3418c468cb3cccc8939`, dated 2026-08-18.
  Checked-in OpenMLS 0.8.1 / mls-rs 0.55.3 tests and an ephemeral 0.9.0-rc.2
  comparison have different dependency graphs. The rc report is not a retained
  production lockfile or evidence for the stable release.
- **E3**: [semantic fixture manifest](../../test/crypto/e2ee-interop-v1.manifest.json),
  generator `2b39382a1b0a050e466aa3418c468cb3cccc8939`, vector SHA-256
  `b9ce8eec6ccab6e5e200159c4a5bac73527ad8ebd929b21cb1f0447ab332ff40`.
  The manifest explicitly forbids production-readiness use.
- **E4**: [FFI verification](../spikes/rust-native-bridge-verification.md), reviewed
  document commit `42f1c36efacbacb5e3221465f5c1caaa83f90d1d`, target `df08663`;
  [#40](https://github.com/monkeylabx/threadline/issues/40) simulator/emulator stage
  is closed. [#41](https://github.com/monkeylabx/threadline/issues/41) remains
  physical-device NOT RUN; [#24](https://github.com/monkeylabx/threadline/issues/24)
  remains open. Generic FFI lifecycle tests are not real MLS-through-FFI tests.

## Admission matrix

Owner entries are responsible roles, not invented assignees or approvals.
All references E1–E4 resolve to the source commits and version limits above.

| Required evidence | Historical result and source | Production result / missing proof | Owner |
| --- | --- | --- | --- |
| Exact library, features, checksum, SBOM and advisory database revision | E1/E2: 0.8.1 audit FAIL; rc.2 report says zero vulnerabilities at 2026-08-18, with warning | NOT RUN on the final stable production target/feature graph; retain reachability and dated Security decisions | Crypto + Integration + Security |
| Malformed ciphertext returns ordinary error in debug/release | E1: FAIL on 0.8.1 debug; E2 rc.2 comparison PASS | NOT RUN for stable provider and all FFI boundaries; no panic-based normal error path | Crypto |
| RFC known-answer vectors | E2: crypto-basics, key-schedule, psk_secret PASS, including wrong-label negative control | NOT RUN for remaining transcript/tree/welcome/passive-client coverage and final build | Crypto + independent reviewer |
| Independent implementation interoperability | E2: OpenMLS ↔ mls-rs 0.55.3 bidirectional join/message/commit/exporter/remove PASS | NOT RUN for stable provider; exchanging serialized MLSMessage is not proof that Threadline's application envelopes are reviewed | Crypto |
| Epoch/add/remove/revoke, replay, old/future epoch and offline ordering | E1/E2 historical scenarios PASS with application queue responsibility | NOT RUN through real client/server persistence; server ordering and authenticated membership changes must converge | Crypto + Core + Client-core |
| Concurrent Commit and Group Fork | E2 explicitly missing | NOT RUN: two writers, accepted order, losing writer resync/recommit, divergent transcript refusal | Crypto + Quality |
| Profile, PrivateMessage handshake and LeafNode lifetime | E2/E3 PASS in Rust; new keys not checked by all semantic hosts | NOT RUN through actual Swift/Kotlin provider; unknown profile must fail without downgrade | Crypto + native hosts |
| Crash/resume and deletion | E2 memory-store snapshot/load/delete PASS | NOT RUN for atomic encrypted provider state, crash mid-write, rollback detection and key erasure | Client-core + Crypto |
| Encrypted MLS storage and OS key custody | E2 detects cleartext persistence; production admission FAIL for that path | NOT RUN: SQLCipher adapter integration, separate key purposes, lock/unlock/revocation, no key bytes in UI/diagnostics | Client-core + platform adapters + Security |
| History Sharing | E1/E3 policy/binding semantics PASS | NOT RUN: independent construction review, encrypted rewrapping, retention and revoked-device negative tests | Crypto + Security |
| Enterprise Recovery isolation | E1/E3 semantic wrapper binding PASS | NOT RUN: real recipient re-encryption, KMS/HSM integration, two distinct approvers, key rotation and expired-case refusal | Recovery + Security + Platform |
| Retention, backup and anti-rollback | E2 explicitly missing | NOT RUN: expiry offline, tombstones, index/cache deletion, identity not cloned on new hardware | Client-core + Security |
| macOS/Windows/Linux production crypto host | E2 macOS Rust subset PASS; general CI build is a separate fact | NOT RUN for the same admitted provider/lock/features and encrypted state on all three hosts | Crypto + Integration |
| Swift/Kotlin and iOS/Android | E4 generic FFI stage PASS at its scope | NOT RUN: MLS-through-FFI plus #41 signed physical installs, secure storage, eviction, bounded memory | iOS + Android + human device owner |
| Performance and self-update policy | E2 desktop measurements PASS for tested sizes | NOT RUN mobile memory/power; choose leaf-count policy only from measured limits, not fixed workflow values | Crypto + native hosts |
| Independent security review | E2 says not begun | NOT RUN for provider adapter, application envelopes, persistence, history/recovery, authorization integration; independent interop is not security review | Independent reviewer + Security |

## Evidence invalidation and acceptance

Every execution packet must name repository SHA, exact crate/checksum and lockfile
hash, Rust/compiler/OS/CPU, target/features/profile, harness and vector digest,
commands/exit codes, retained redacted artifact digest, limitations and reviewer.
The evidence SHA must be reachable by the integration owner. Do not retain real
keys, prompts, paths, decrypted messages or credentials in that packet.

A library/provider/feature/toolchain change requires the relevant malformed-input,
interop and full target matrix to be rerun. A storage schema change requires
crash/rollback/migration evidence; a profile/envelope change requires golden and
N-1 compatibility plus independent construction review. Recovery-policy and
key-custody changes require renewed recovery review. A new advisory invalidates
old clean-audit claims until the exact graph is rescanned. Simulator results
never graduate into physical-device evidence.

Production admission requires all ADR gate items to have current evidence, no
unaccepted security findings, and recorded Architecture and Security decisions
on the exact candidate and scope. Product signs scope limitations; the human
owner supplies #41 devices/signing/evidence; the independent reviewer signs
review conclusions. This document supplies none of those signatures. A minimal
message experiment must not be presented as v1 delivery while v1 recovery/history
requirements remain unimplemented. M0/G0 and ADR statuses remain unchanged.

Reopen the applicable ADR if the stable candidate regresses on malformed input,
cannot meet supported-platform storage/interop requirements, or cannot clear
security findings. Evaluate the documented alternative only with Security and
Architecture approval. Do not adopt unreviewed patches, invent encryption,
relax checks, preserve MLS ratchet secrets on the server or fall back to plaintext.

## Bounded successor task drafts

These are specifications to split/claim after review, not ready production work.
Each uses one primary ownership area; integration-owned dependencies are separate.

| Draft / effort | Owned surface and output | Prerequisite | Acceptance |
| --- | --- | --- | --- |
| C1 Provider public seam, 1 day | Crypto: `crates/client-crypto/`; library-independent operation/error/state-transaction interface and deterministic fake; no real crypto | Reviewed version/profile contract and storage transaction requirements | `cargo test -p threadline-client-crypto --locked`; external caller can use interface without OpenMLS types; missing/unknown capabilities fail closed |
| C2 Dependency admission packet, 0.5–1 day | Integration: separate workspace/lock/SBOM change for approved exact dependency set | Security/Architecture candidate decision, upstream source integrity | `cargo tree --locked`; `cargo audit --file Cargo.lock`; license/SBOM/checksum and target-feature evidence, exceptions explicitly approved |
| C3 Storage transaction interface, 1 day | Client-core contract task in `docs/contracts/`; typed opaque state, atomic commit/version CAS, deletion and crash behavior | C1 review and existing keyed database API | `git diff --check`; state-transition fixtures cover missing key, stale generation, partial write, cancel-before/after commit; no SQLCipher connection crosses API |
| C4 First encrypted state adapter slice, 2 days | Client-core: `crates/client-core/`; one group snapshot atomic save/load/delete via existing keyed DB | C3 accepted; schema/manifest changes in separate Integration task; OS key input contract | `cargo test -p threadline-client-core --locked`; wrong key/reopen/crash-in-transaction tests, no plaintext file; this slice alone does not finish retention/recovery |
| C5 First stable interop regression, 1–2 days | Crypto: `test/crypto/`; a harness using C1 real adapter, external implementation transcript join/message/update | C2 integrated; real adapter minimal operations separately implemented after C1 | Existing fixture check plus new harness's pinned command in issue before claim; assert plaintext equality only on synthetic bytes, mismatched exporter/old epoch/tampered frame rejected |
| C6 Concurrent commit evidence, 1–2 days | Crypto: existing isolated `spikes/e2ee-interop/` harness | Reviewed deterministic accepted-order fixture and pinned library | `cargo test --manifest-path spikes/e2ee-interop/rust/Cargo.toml --locked`; two proposals, winning commit, resync loser, no persistent fork; explicitly spike-only |

C4 deliberately excludes OS key generation, History/Recovery and multi-device
backup. Those need their own role-specific tasks. C5 does not claim independent
review, all-platform support or completion of the whole provider implementation.

## Current upstream verification

### OpenMLS 上游核对（2026-09-07）

只做资料与现有锁文件检查；没有改依赖，没有运行 cargo audit 或正式版测试。

### 版本事实（高置信度）

- 官方 [Releases](https://github.com/openmls/openmls/releases) 当前 Latest 是 **0.9.0 正式版**，GitHub 发布日期 2026-08-25。其正文标题保留 `0.9.0 (2026-08-03)`，不得把标题日期混作 GitHub 正式发布日期。
- Tag `openmls-v0.9.0` 指向 [3a3e35de3feeca8f6605143c464d5452ae584d43](https://github.com/openmls/openmls/commit/3a3e35de3feeca8f6605143c464d5452ae584d43)。
- [官方 tags](https://github.com/openmls/openmls/tags) 列出历史 rc.2（2026-08-06，831ea9e）、rc.3（2026-08-20，80b2163）、rc.4（2026-08-24，730b3be）；rc.4 是列表中最后的 0.9.0 prerelease，但已由正式版取代。rc.2/rc.4 的完整 commit 页面抓取失败，只保留 tags 可验证短 SHA。
- 0.9.0 release notes 声明 MSRV 为 Rust 1.91，并改变 storage 支持及提供迁移入口；准入应显式检查工具链和存储迁移，不能继承 rc 结果。[正式版说明](https://github.com/openmls/openmls/releases/tag/openmls-v0.9.0)
- 正式 tag 的 [openmls_rust_crypto/Cargo.toml](https://github.com/openmls/openmls/blob/openmls-v0.9.0/openmls_rust_crypto/Cargo.toml) 是 0.6.0，hpke-rs / hpke-rs-crypto / hpke-rs-rust-crypto 要求 0.7；这不是 Threadline 已锁定的实际完整依赖图。

### 历史证据与当前事实必须区分

仓库 `docs/spikes/e2ee-library-selection.md`（2026-08-18）报告 0.8.1 图 6 vulnerabilities / 4 High，以及临时 rc.2 图 0 vulnerabilities。ADR-0004 仍为 proposed，要求正式版重跑证据。这些是历史仓库报告，不是本次重新执行的结果。

当前 `spikes/e2ee-interop/rust/Cargo.lock` 静态读取仍包含 OpenMLS 0.8.1、provider 0.5.1、hpke-rs 0.6.1、libcrux-sha3 0.0.8、secrets 0.0.5、chacha20poly1305 0.0.7、aesgcm 0.0.7。

### 官方 RustSec 现有公告（高置信度；不是完整新扫描）

| 公告 | 包与当前公告修复边界 |
| --- | --- |
| [RUSTSEC-2026-0124](https://rustsec.org/advisories/RUSTSEC-2026-0124.html) | libcrux-chacha20poly1305 <0.0.8；修复 >=0.0.8；High，过长输出缓冲区 panic |
| [RUSTSEC-2026-0207](https://rustsec.org/advisories/RUSTSEC-2026-0207.html) | libcrux-sha3 <=0.0.9；修复 >=0.0.10；High，多次 squeeze 输出错误 |
| [RUSTSEC-2026-0208](https://rustsec.org/advisories/RUSTSEC-2026-0208.html) | libcrux-sha3 <=0.0.9；修复 >=0.0.10；High，特定 AVX2 SHAKE-256 输出尺寸 panic |
| [RUSTSEC-2026-0212](https://rustsec.org/advisories/RUSTSEC-2026-0212.html) | libcrux-secrets <=0.0.5；修复 >=0.0.6；High，aarch64 swap/select 结果错误 |
| [RUSTSEC-2026-0209](https://rustsec.org/advisories/RUSTSEC-2026-0209.html) | libcrux-aesgcm <=0.0.8；原包无 patched version，迁移到 libcrux-aes 0.0.9；Medium，AAD 上限未检查 |
| [RUSTSEC-2026-0211](https://rustsec.org/advisories/RUSTSEC-2026-0211.html) | libcrux-aesgcm <=0.0.8；原包无 patched version；Medium，tag 检查非恒定时间 |
| [RUSTSEC-2026-0210](https://rustsec.org/advisories/RUSTSEC-2026-0210.html) | libcrux-aesgcm 已改名 libcrux-aes；unmaintained 信息性公告，不计为漏洞 |

这些公告版本范围与现有旧锁文件包版本匹配。此静态匹配不证明 Threadline 的可利用路径，也不等价于完整当前审计。sha3 公告明确说明上述两条不会影响公告所述的 ML-KEM/ML-DSA 用法；仍应记录功能、架构和可达性分析，不能自动豁免。

仓库泛称 `libcrux-* 0.0.9` 全部清零不够精确：当前 RustSec 对 sha3 要求 >=0.0.10。不要把历史 rc.2 零漏洞推导成当前正式版零漏洞。上游 [issue 2126](https://github.com/openmls/openmls/issues/2126) 现已关闭，仅证明维护者已处理 issue，不是 Threadline 的新审计证据。

### 对准入清单的建议（推论）

将“等待 0.9.0 发布”更新为“正式版已发布；等待在精确正式版依赖图上重跑全部证据”。后续记录 release/tag SHA、实际 Cargo.lock hash、features/target、rustc/cargo-audit 版本、RustSec DB commit、日期及 raw audit artifact。分别记录漏洞、notice、unmaintained warning 与安全负责人处理结果。不得在没有这些证据时标 PASS 或安全生产准入。

### 限制

crates.io API 网页抓取失败；shell gh API 因网络限制失败。因此“Latest”依据官方 GitHub 发布页，不声称已核验 crates.io published/yanked 状态。本次没有重跑任何构建、互操作、panic 回归、供应链审计或下载依赖。
