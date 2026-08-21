# T010-A Rust Native Bridge — 独立验证记录

日期：2026-08-18 · 验证对象：`agent/t010a-simulator-emulator-ffi` @ `df08663`
关联：[#40](https://github.com/monkeylabx/threadline/issues/40)、ADR-0001、交付计划 P00-07

本文件记录对 T010-A spike 的一次**独立复核**结果：哪些结论有实测证据、哪些没有、
以及 M0 门（2026-08-21）能否据此关闭。它不替代
[`rust-native-bridge.md`](rust-native-bridge.md)，只补充证据状态。

## 1. 验证环境

| 项 | 值 |
| --- | --- |
| 主机 | macOS 26.4 (25E246)，arm64 |
| Rust | 1.97.1 (8bab26f4f)，与 `rust-toolchain.toml` 一致 |
| Swift | 6.3.3 (swiftlang-6.3.3.1.3)，随 Command Line Tools |
| Xcode | **未安装**（仅 Command Line Tools，无 iPhoneOS/iPhoneSimulator SDK，无 XCTest） |
| Android SDK / NDK / JDK | **未安装** |

环境限制直接决定了下面第 3 节的空白项。这不是 spike 的缺陷。

## 2. 已通过实测验证的部分

### 2.1 Rust workspace

```bash
cargo fmt --all --check
cargo clippy --workspace --exclude threadline-desktop-host --all-targets --locked -- -D warnings
cargo test --workspace --exclude threadline-desktop-host --locked
```

三条命令全部通过，clippy 在 `-D warnings` 下无告警。`threadline-client-ffi` 15 个
单元测试全绿，覆盖句柄代次、缓冲区 exactly-once 释放、panic/unknown 稳定错误、
有界流与游标续传、1000 次生命周期循环后资源计数归零。

### 2.2 进程崩溃与游标续传

```bash
cargo run --package threadline-client-ffi --example crash_resume --locked -- write "$CURSOR"   # 退出码 86
cargo run --package threadline-client-ffi --example crash_resume --locked -- resume "$CURSOR"
```

写进程按约定以退出码 **86** 终止且不释放原生资源；新进程读取持久化游标后输出
`resumed cursor 3 with sequences 4 and 5`，即从 `cursor + 1` 续传。符合 ADR-0001
对"断流恢复依赖持久化 Cursor 而非内存队列"的要求。

### 2.3 Swift → Rust 边界(macOS 宿主,33/33 通过)

XCTest 随 Xcode 分发,本机不可用,因此另写了一个不依赖 XCTest 的可执行 harness,
复用同一套 `ThreadlineIOSHost` Facade 源码驱动契约。源码保存在
[`docs/spikes/ffi-harness/main.swift`](ffi-harness/main.swift)。

> **它刻意不放进 `apps/ios` 包,也不进 CI。** 见 §4.2 —— 往该包里增加任何 target 都会
> 让 `xcodebuild test -scheme ThreadlineIOSHost` 构建失败。使用时把它复制到包外的独立
> SwiftPM 包,并把 `apps/ios/Sources/ThreadlineIOSHost` 软链进去即可。

结果 `checks: 33   failures: 0`,覆盖:

| 契约点 | 观测结果 |
| --- | --- |
| Contract version | `1` |
| 请求提交与结果缓冲 | `committed = true`,payload `threadline-ok` |
| 提交前取消 | `cancel()` 返回 `ok`,完成事件为 `canceled` |
| 提交窗口内取消 | 返回 `alreadyCommitted`,操作继续至完成 |
| 终态后取消 | 返回 `ok`(幂等空操作,见 4.1) |
| Panic 隔离 | 映射为 `panic`,未穿越 FFI 进入 Swift |
| 未知错误 | 映射为 `unknown` (255) |
| 有界流 | 64 事件全达,序号严格单调 `1..64` |
| 背压 | `capacity 4`,`maxDepth 4` 未越界,`backpressureCount 60` |
| 游标续传 | `cursor 41` → 交付 `[42, 43, 44]` |
| 重复事件 | 终止于 `protocolViolation`,重复序号未暴露给宿主 |
| 回调栅栏 | `close()` 返回后 0 次投递 |
| 内存所有权 | 1000 次 create/start/close/release 后四类资源计数均回到基线 0 |
| 陈旧句柄 | 释放后调用返回稳定 `closed`,无 use-after-free |

### 2.4 移动端目标交叉编译

五个目标全部 `cargo check` 通过：

```text
aarch64-apple-ios          CHECK OK
aarch64-apple-ios-sim      CHECK OK
x86_64-apple-ios           CHECK OK
aarch64-linux-android      CHECK OK
x86_64-linux-android       CHECK OK
```

iOS **静态库**（iOS 实际链接的产物）在无 Xcode 的情况下即可产出：
`target/aarch64-apple-ios/release/libthreadline_client_ffi.a`，`lipo -info` 确认为
`arm64`，导出 20 个 `threadline_*` C 符号。同 crate 的 `cdylib` 因缺少 iphoneos SDK
链接失败，但 iOS 集成路径不使用 cdylib，**不构成阻塞**。

`threadline-client-ffi` **无任何外部 crate 依赖**（纯 std），这显著降低了 ADR-0001
"退出条件"里"发现无法通过 Facade 隔离的 ABI/链接/许可证限制"的概率。

### 2.5 三个宿主表面的符号一致性

| 比对 | 结果 |
| --- | --- |
| C 头文件声明 ↔ 导出符号 | 20 / 20 完全一致 |
| Swift `@_silgen_name` ↔ 导出符号 | 19 / 20（`threadline_request_state` 仅 Kotlin 侧使用） |
| Kotlin `external fun` ↔ Rust JNI 导出 | 23 / 23 完全一致 |

Kotlin 侧无悬空 `external` 声明——这是 JNI 最常见的运行期崩溃来源，静态比对已排除。

### 2.6 CI 实跑证据（GitHub Actions）

| Run | Commit | 结果 |
| --- | --- | --- |
| #16 | `df08663`（T010-A 分支） | **五个 job 全绿**，含 `apple` 的 iOS Simulator 与 `android` 的 Emulator |
| #29 / #30 | 本验证分支 | `workspace-linux` / `desktop-linux` / `desktop-windows` / **`android` 全绿**；`apple` 失败，原因见 §4.2（已修） |

run #16 的 `native-bridge-ios-simulator` 工件为 **51,510,358 字节**的完整 `.xcresult`，
是 Simulator 实跑并产出测试结果的直接证据；#29/#30 失败时该工件仅 ~8.5 KB（构建阶段
即失败，无测试结果），两者对比也是定位 §4.2 那个缺陷的关键依据。

**结论：Simulator 与 Emulator 层面的执行证据已经取得**，缺口收窄为真机（T010-B）。

## 3. **未**验证的部分（M0 门的实质缺口）

| 缺口 | 原因 | 状态 |
| --- | --- | --- |
| iOS Simulator 实跑 | 本机无完整 Xcode | ✅ **已由 CI run #16 覆盖**（51 MB `.xcresult`） |
| Android Emulator 实跑 | 本机无 JDK/SDK/NDK | ✅ **已由 CI run #16、#29、#30 覆盖** |
| Android JVM 单元测试 | 同上 | ✅ 已由 CI `android` job 覆盖 |
| **iOS / Android 真机** | 无设备、无签名证书 | ❌ **仍缺 —— P00-07 与 ADR-0001 明文要求的就是真机** |

关键点：`rust-native-bridge.md` 自述"validates the host boundary only… is not
evidence for the real-device gate in T010-B"。而 ADR-0001 的验证要求与退出条件
写的是"**真机**验证 FFI 的调用、流、取消、错误和内存压力"，交付计划 P00-07 的
验收也是"真机通过"。**Simulator/Emulator 证据不满足 P00-07 的字面验收标准。**

## 4. 观察项（非阻塞）

### 4.1 `cancel()` 在终态返回 `ok`，语义有歧义

`runtime.rs` 的 `cancel()` 仅在 `Committed` 相位返回 `AlreadyCommitted`；一旦请求
进入 `Succeeded`/`Canceled` 等终态即返回 `Ok`。实现是有意的（`cancellation_has_a_
deterministic_commit_point` 用 `wait_until_committed` 专门命中该窗口），但宿主
拿到 `Ok` 时无法区分"取消已受理"与"无事可取消"。`rust-native-bridge.md` 的状态图
也未说明终态下的取消返回值。建议在 Contract v1 定稿前明确该语义或增加独立状态码。

### 4.2 向 `apps/ios` 包增加任何 target 都会打断 Simulator job

本次验证过程中把 harness 作为 `executableTarget` 加进 `apps/ios/Package.swift`，
导致 `scripts/ci/run-ios-simulator-ffi.sh` 的 `xcodebuild test -scheme ThreadlineIOSHost`
以退出码 66 失败。特征是：步骤仍耗时数分钟（模拟器正常启动并进入构建），但产出的
`.xcresult` 从 51 MB 塌缩到 ~8.5 KB —— 即**构建阶段失败，一个测试都没跑**。

先只删掉 `products` 里的 `.executable` 条目并不能修复（run #30 仍失败）：xcodebuild
构建的是包内**所有 target**，不只是 products。最终把 harness 整个移出 `apps/ios`
才恢复。

因此:**`apps/ios` 的 `Package.swift` 应视为 Simulator 门禁的一部分**。任何新增
target/product 都必须在装有 Xcode 的机器或 CI 上验证过 `xcodebuild test`，本地
`swift build` / `swift test` 通过**不构成**证据 —— 后者根本不会走 xcodebuild 的
scheme 与构建图。

## 5. 对 M0 门（2026-08-21）的结论

**技术风险层面：低。** 就已验证的范围看，ADR-0001 第 4 节要求的版本化 Facade、
稳定错误封套、panic 不穿越 FFI、取消语义、有界流与背压、游标续传、句柄代次与
exactly-once 释放，均有可复现的实测证据；零依赖 + 五目标交叉编译通过 + 三宿主
符号一致，说明"更换 Binding 生成器/放弃 Shared Core"这类重排期风险目前**没有**
被触发的迹象。

**门禁层面：P00-07 仅差真机证据。** Simulator/Emulator 已由 CI 覆盖（§2.6），剩余：

1. 补 T010-B 的 iOS/Android **真机** 验证（加载、签名、后台/进程回收、内存压力）。
2. 将 T010-A 合入 `main` —— 当前 `main` 上 `client-ffi` 仍是 25 行空壳，任何下游
   工作都拿不到这些成果。

建议 M0 对 P00-07 记为**条件通过（真机证据待补）**，并把上述两项作为门后立即项；
若必须严格按"真机通过"判定，则 P00-07 判为未过，但风险等级可从"不可逆风险"
下调——目前没有任何证据指向需要重开 ADR 的方向。

## 6. 复现命令

```bash
# Rust 全量
cargo fmt --all --check
cargo clippy --workspace --exclude threadline-desktop-host --all-targets --locked -- -D warnings
cargo test --workspace --exclude threadline-desktop-host --locked

# 崩溃/续传
cargo run --package threadline-client-ffi --example crash_resume --locked -- write /tmp/cursor   # 期望退出码 86
cargo run --package threadline-client-ffi --example crash_resume --locked -- resume /tmp/cursor

# Swift 边界（无需 Xcode）：见 §2.3，harness 需在 apps/ios 包外运行
cargo build --package threadline-client-ffi --locked
mkdir -p /tmp/h/Sources/Harness && cd /tmp/h
ln -sfn "$OLDPWD/apps/ios/Sources/ThreadlineIOSHost" Sources/ThreadlineIOSHost
cp "$OLDPWD/docs/spikes/ffi-harness/main.swift" Sources/Harness/
# 再写一个 macOS-only 的 Package.swift，声明 ThreadlineIOSHost + Harness 两个 target
THREADLINE_FFI_LIBRARY_DIR="$OLDPWD/target/debug" swift run Harness

# 移动端目标
rustup target add aarch64-apple-ios aarch64-apple-ios-sim x86_64-apple-ios \
  aarch64-linux-android x86_64-linux-android
cargo check --package threadline-client-ffi --target aarch64-apple-ios --locked
```

harness 输出以 `checks: N   failures: 0` 与 `RESULT: PASS` 结尾，退出码 0；任一
契约点失败即非零退出。**不要**为了接进 CI 而把它加回 `apps/ios` 包，原因见 §4.2。
