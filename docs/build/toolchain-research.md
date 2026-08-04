# Threadline 五平台工具链调研

状态：T009 实施输入

调研快照：2026-08-04

范围：P00-05 Monorepo、工具链锁定和开发环境；为 P00-06 CI/签名基线提供边界，不在本任务接入生产签名材料。

## 结论

Threadline 应冻结下列生产构建基线。版本号必须精确匹配，不接受 `latest`、`stable`、`^`、`~` 或只写主/次版本。

| 领域 | 冻结版本 | 仓库内权威位置 | 兼容性结论 |
| --- | --- | --- | --- |
| Node.js | `24.19.0` LTS | `.node-version`、根 `package.json#engines.node` | Node 24 是生产 LTS；Node 26 仍是 Current，不作为基线 |
| pnpm | `11.20.0` | 根 `package.json#packageManager` | pnpm 11 支持 Node 24；锁文件只由该精确版本生成 |
| Rust | `1.97.1` | `rust-toolchain.toml` | 包含该发行版配套的 Cargo，并显式安装 `rustfmt`、`clippy` 和所需 targets |
| Go | `1.26.5` | `go.work`、`services/go.mod` 的 `toolchain go1.26.5` | `go 1.26.0` 表达语言/模块语义，`toolchain` 表达补丁版本 |
| Tauri core | `tauri = =2.11.5` | `apps/desktop/src-tauri/Cargo.toml` | Rust crate 与 JS API 保持同一 `2.11` minor |
| Tauri build | `tauri-build = =2.6.3` | `apps/desktop/src-tauri/Cargo.toml` | Cargo 中必须带 `=`；普通 `2.6.3` 仍允许 caret 漂移 |
| Tauri CLI | `@tauri-apps/cli = 2.11.4` | `apps/desktop/package.json` | 只采用 Node CLI，不再并行安装 Cargo CLI |
| Tauri JS API | `@tauri-apps/api = 2.11.1` | `apps/desktop/package.json` | 与 core 同一 minor；Tauri 插件的 npm/crate 两侧还须精确同 patch |
| Xcode | `26.6` build `17F113` | CI 的 `DEVELOPER_DIR` 和 doctor 断言 | 当前稳定版，内置 Swift 6.3、iOS/macOS 26.5 SDK；不采用 Xcode 27 beta |
| Swift（Apple） | Xcode 26.6 内置 `6.3` | 由 Xcode pin 间接冻结 | Apple 平台构建不混用 swift.org 独立 toolchain |
| Swift（非 Apple，可选） | `6.3.3` | 独立 Linux Swift job 的镜像/安装清单 | 仅在确实增加 Linux Swift job 时使用，不替代 Xcode toolchain |
| JDK | Eclipse Temurin `17.0.19+10` | CI setup 和本地 doctor | AGP 9.3 要求并默认 JDK 17；Gradle 9.5.x 可运行在 Java 17 |
| Android Gradle Plugin | `9.3.1` | version catalog 或根 plugin declaration | 官方 9.3 线最大支持 API 37，要求 Gradle 至少 9.5.0 |
| Gradle wrapper | `9.5.1` | committed wrapper | 9.5.0 的官方补丁升级；仍在 AGP 9.3 的 9.5 兼容线内 |
| Kotlin for Android | AGP 9.3.1 built-in Kotlin | 不应用 `org.jetbrains.kotlin.android` | 避免显式 KGP 2.4.10 与 AGP 9.3 超出 JetBrains 已声明完整支持矩阵 |
| 独立 Kotlin/JVM（仅需要时） | `org.jetbrains.kotlin.jvm = 2.4.10` | 非 Android module 的 plugin pin | 不得把该 plugin 加回 Android module |
| Android SDK | API/compile SDK `37` | Android build config + sdk install manifest | AGP 9.3 支持的最高 API；`minSdk`/`targetSdk` 是产品决策，不在本文冻结 |
| SDK Build Tools | `36.0.0` | SDK package manifest | AGP 9.3 官方默认/最低组合 |
| Android NDK | `28.2.13676358` | `ndkVersion` + SDK package manifest | AGP 9.3 官方默认版本；用于 Rust/Android FFI 时精确冻结 |

这些版本是“已验证组合”，不是“自动跟随最新”。安全补丁或平台提交要求变化时，通过独立升级 PR 更新所有 pin、校验和、锁文件和验证证据。

## 选择依据与兼容边界

### Node.js 和 pnpm

- Node 官方发布索引在调研日列出 `24.19.0`，且 Node 24（Krypton）处于 LTS；Node 26 是 Current。生产构建采用 LTS 线。
- pnpm 官方 registry 的 `latest` 为 `11.20.0`，其引擎要求与 Node 24 相容。pnpm 的兼容表也列出 pnpm 11 支持 Node 22、24、26。
- 根 `package.json` 使用 `"packageManager": "pnpm@11.20.0"`，`.node-version` 和 `engines.node` 同时为 `24.19.0`。CI 安装依赖使用 `pnpm install --frozen-lockfile`。
- 不缓存 `node_modules`；缓存 pnpm content-addressable store，并由 `pnpm-lock.yaml`、Node 和 pnpm 精确版本共同决定 cache key。
- Node 24 仍带 Corepack，但不能把“runner 中恰好存在的 Corepack 版本”当成 pin。CI 先选择 Node 24.19.0，再启用项目声明的 pnpm；doctor 必须验证最终 `pnpm --version`。

### Rust 和 Tauri 2

- `rust-toolchain.toml` 冻结 `channel = "1.97.1"`、`profile = "minimal"`、`components = ["rustfmt", "clippy"]`。Rust 1.97.1 是 1.97 的修复发行版，修复了 LLVM 误编译风险。
- Desktop targets 至少为：Linux `x86_64-unknown-linux-gnu`，Windows `x86_64-pc-windows-msvc`，macOS runner 原生 `aarch64-apple-darwin`。如增加 Intel macOS 制品，再显式加入 `x86_64-apple-darwin`。
- Rust/Android FFI 启用时，再加入 `aarch64-linux-android`、`armv7-linux-androideabi`、`i686-linux-android`、`x86_64-linux-android`；不应让 T009 空壳下载暂时不使用的 targets。
- Tauri 官方发布面板显示 core、build helper、CLI 和 JS API 并不共用同一个 patch 号，因此不能臆造一个统一版本。官方只要求 core crate 与 `@tauri-apps/api` 对齐 minor；单个插件的 Rust/npm 两侧则必须精确对齐 patch。
- Cargo 的普通 `version = "2.11.5"` 语义允许兼容更新。为了可重复构建，直接依赖写成 `version = "=2.11.5"`，同时提交 `Cargo.lock`。
- Rust 1.97.1 高于 Tauri 2.11.5 的 MSRV 1.77.2，满足编译下限。

### Go

- Go 官方发布历史列出 `1.26.5` 为 1.26 的安全和 bug 修复版本；Go 官方只维护最近两个 major release，故不选择旧线。
- `go 1.26.0` 是语言与模块语义版本；`toolchain go1.26.5` 才是构建器补丁版本。两者同时存在不是冲突。
- CI 将 `GOTOOLCHAIN=local`，并显式安装 1.26.5，防止 Go 根据 `toolchain` 指令静默联网切换。`go version` 不精确匹配时立即失败。
- Go cache key 至少包含 runner OS/arch、`go1.26.5` 和所有 `go.sum` 的 hash；不得缓存工作区输出作为发布制品。

### Apple：Xcode 和 Swift

- Apple 当前稳定版为 Xcode 26.6 build 17F113，内置 Swift 6.3 和 iOS/macOS 26.5 SDK；Xcode 27 beta 不进入合并门禁。
- GitHub 当前 `macos-26` arm64 image 提供 `/Applications/Xcode_26.6.app`。CI 必须设置：

  ```text
  DEVELOPER_DIR=/Applications/Xcode_26.6.app/Contents/Developer
  ```

  随后断言 `xcodebuild -version` 同时包含 `Xcode 26.6` 和 `Build version 17F113`，不能依赖会随镜像变动的默认 `/Applications/Xcode.app`。
- `Package.swift` 的 `// swift-tools-version` 是 manifest 能力下限，不是编译器 pin。现有 manifest 若未用 Swift 6.3-only 语法，可保持 6.0；真正的 Apple 编译器基线由 Xcode 26.6 冻结。
- swift.org 当前独立稳定版为 6.3.3，但官方明确 Apple App Store 构建应使用 Xcode 自带 Swift。只有未来增加 Linux SwiftPM 验证时，才另行 pin 6.3.3。
- 当前 iOS target 可跑 SwiftPM unit test，无需签名。设备、Archive 和发布签名属于 P00-06/后续 release workflow。

### Android：JDK、Gradle、AGP 和 Kotlin

- AGP 9.3.1 是官方 9.3 稳定补丁。9.3 兼容表规定：Gradle 最低/默认 9.5.0、JDK 最低/默认 17、SDK Build Tools 36.0.0、NDK 默认 28.2.13676358、最大 API 37。
- Threadline 选择 Gradle 9.5.1，因为 Gradle 官方建议从 9.5.0 升级到修复回归的 9.5.1；这是同一 9.5 兼容线内的 patch 选择。若实际 Android 验证发现 AGP 回归，唯一回退是经记录切回 9.5.0，不允许漂到 `9.6.x`。
- Wrapper 使用 `gradle-9.5.1-bin.zip`，并写入官方 SHA-256：

  ```text
  distributionSha256Sum=bafc141b619ad6350fd975fc903156dd5c151998cc8b058e8c1044ab5f7b031f
  ```

  committed `gradle-wrapper.jar` 的官方 SHA-256 为：

  ```text
  497c8c2a7e5031f6aa847f88104aa80a93532ec32ee17bdb8d1d2f67a194a9c7
  ```

  若兼容性验证要求回退到 AGP 文档的默认 Gradle 9.5.0，官方 `-bin.zip` SHA-256 为：

  ```text
  553c78f50dafcd54d65b9a444649057857469edf836431389695608536d6b746
  ```

  9.5.0 与 9.5.1 的 wrapper JAR checksum 相同；回退 PR 仍须同时验证并更新 distribution URL/checksum，不允许只改 URL。

- Java runtime 固定 Temurin `17.0.19+10`，CI 同时断言 vendor、feature、patch 和 build number；只检查 `17` 不足以复现。
- Kotlin 2.4.10 是 JetBrains 当前稳定版，但 JetBrains 的完整支持矩阵仅声明到 AGP 9.1.0。AGP 9 起默认支持 built-in Kotlin，因此 Android module 使用 AGP 9.3.1 built-in Kotlin，并移除 `org.jetbrains.kotlin.android`。这既避免不受支持的组合，也避免重复 Kotlin 配置。
- 若仓库以后出现纯 JVM/Kotlin tooling module，可在该非 Android module 精确应用 Kotlin JVM plugin 2.4.10；不得因此改变 Android module 的选择。
- 当前 `apps/android/build.gradle.kts` 直接调用宿主机 `kotlinc` 的空壳做法不具备版本封闭性。完成 Android 工程化时应改为 committed wrapper + AGP built-in Kotlin；在此之前 doctor 必须把本机 Kotlin 视为缺口而不是已冻结工具。

## GitHub Actions 构建矩阵

GitHub-hosted runner 的 OS label 只固定 OS major，不固定每周更新的镜像 revision。以下矩阵通过显式安装/选择工具和运行时断言收紧漂移；每个 job 仍应把 runner image version、OS build 和实际工具版本上传为 provenance 文本。

| Job | `runs-on` | 覆盖 | 必须执行的门禁 | 签名 |
| --- | --- | --- | --- | --- |
| `workspace-linux` | `ubuntu-24.04` | Node/pnpm、Rust workspace、Go services、协议/通用检查 | strict doctor；frozen install；lint/test/build | 无 |
| `desktop-linux` | `ubuntu-24.04` | Tauri Linux host | 官方 Tauri Linux prerequisites；`cargo check/test`；`tauri build --no-bundle` | 无 |
| `desktop-windows` | `windows-2025` | Tauri Windows/MSVC host | Rust MSVC target；`cargo check/test`；`tauri build --no-bundle`；记录 MSVC/Windows SDK | 无 |
| `apple` | `macos-26` | SwiftPM iOS host、Tauri macOS host | 选择 Xcode 26.6；`swift test`；Rust/Tauri no-bundle build | 无，模拟器/测试构建 |
| `android` | `ubuntu-24.04` | 原生 Android host、后续 Rust Android FFI | Temurin 17.0.19+10；wrapper 校验；API 37/Build Tools/NDK exact；`test/lint/assembleDebug` | debug-only |

额外约束：

- 禁止 `ubuntu-latest`、`windows-latest`、`macos-latest`。
- `macos-26` 当前对应 arm64 image；若仓库额度/可用性不支持该 label，先把 Apple job 标为受阻并选择有同一 Xcode build 的明确 runner，而不是静默回落到不同 Xcode。
- PR workflow 顶层默认 `permissions: contents: read`。只有确有需要的 job 才局部增加权限。
- 默认 fail-fast 可开启；若需要一次收集五平台全部失败证据，可对 matrix 关闭 fail-fast，但不能把失败 job 设为允许失败。
- Windows、macOS SDK 和 Linux 系统库仍来自每周变化的 runner image。P00-05 的可重复含义是“受支持 runner 上可由精确语言工具链和 lockfile 重建”；若需要 bit-for-bit OS SDK 复现，应另立任务使用自托管不可变镜像。
- PR 只生成测试/unsigned artifacts。安装包、商店 archive、notarization、SBOM 签名进入 P00-06 的受保护 release jobs。

### Action 版本与供应链约束

所有 action，包括 `actions/*`，在 workflow 中都引用完整 commit SHA，并在同行注释审核过的 tag。调研时可用的不可变 pin 为：

| Action | 完整 SHA | 调研时 tag |
| --- | --- | --- |
| `actions/checkout` | `de0fac2e4500dabe0009e67214ff5f5447ce83dd` | `v6.0.2` |
| `actions/setup-node` | `820762786026740c76f36085b0efc47a31fe5020` | `v7.0.0` |
| `actions/setup-go` | `4a3601121dd01d1626a1e23e37211e3254c1c06c` | `v6.4.0` |
| `actions/setup-java` | `03ad4de0992f5dab5e18fcb136590ce7c4a0ac95` | `v5.6.0` |
| `gradle/actions/setup-gradle` | `16f3e46a58d2b926c34615132d7969a96bccb22b` | `v6.0.0` |
| `actions/cache`（仅手工 cache） | `9255dc7a253b0ccc959486e2bca901246202afeb` | `v5.0.1` |

相同规则适用于后来新增的任何 action；实施 PR 要从官方 tag 解析、复核并记录 SHA，不把可移动的 `@vN` 写进 main。

## Cache 设计

Cache 是可丢弃的性能优化，不是依赖来源、制品仓库或秘密存储。cache miss 必须仍能成功构建。

| 生态 | 缓存 | Key 输入 | 明确不缓存 |
| --- | --- | --- | --- |
| pnpm | pnpm store | OS/arch、Node 24.19.0、pnpm 11.20.0、`pnpm-lock.yaml` | `node_modules`、构建输出、npm token |
| Cargo | registry、git db、各 job 独立 `target` | OS/arch、Rust 1.97.1、target triple、features、`Cargo.lock` | signing keys、跨 target 的 `target`、最终 bundle |
| Go | module cache、build cache | OS/arch、Go 1.26.5、所有 `go.sum` | workspace 输出、凭据 |
| Gradle | `setup-gradle` 管理的 dependency/config cache | OS/arch、JDK exact、Gradle exact、wrapper、Gradle dependency files | 同时开启 setup-java Gradle cache、keystore、APK/AAB release |
| Swift | 初期不缓存；出现外部依赖后再评估 `.build` | OS/arch、Xcode 26.6 build、Swift、`Package.resolved` | DerivedData 跨 Xcode 复用、profiles/certificates |

Gradle 使用 `gradle/actions/setup-gradle` 的单一 cache owner，配置 `cache-provider: basic`；不要再给 `actions/setup-java` 开 `cache: gradle`。来自 PR 的 job 只读/受 GitHub cache scope 限制，main 写入 canonical cache。

## 版本漂移检测和升级流程

### 合并门禁：strict doctor

每个 job 在编译前输出并解析以下信息；任何精确版本不匹配均失败：

```text
node --version
pnpm --version
rustc -Vv
cargo -V
go version
java -version
./gradlew --version
swift --version
xcodebuild -version
xcrun --sdk iphoneos --show-sdk-version
```

Android job 还应校验安装清单中存在 API 37 的精确 SDK 包 `platforms;android-37.0`、`build-tools;36.0.0` 和 `ndk;28.2.13676358`；`compileSdk = 37` 与可安装包名分开锁定，以兼容 Android 17 引入的 minor SDK package 命名。Apple job记录 runner image version；Windows job记录 `cl` 和 Windows SDK 版本。doctor 的人类可读提示可以给安装建议，`--strict` 在 CI 中必须以非零状态拒绝漂移。

仓库内 pin 的单一真相关系为：

- Node：`.node-version` 与 `package.json#engines.node` 必须相等。
- pnpm：`packageManager` 与生成 `pnpm-lock.yaml` 的版本必须相等。
- Rust：只读 `rust-toolchain.toml`，不再从 workflow 复制另一个可漂移版本。
- Go：`go.work` 与各 `go.mod` 的 `toolchain` 必须一致。
- Tauri：检查 Cargo/JS minor 规则和所有精确版本；插件检查 npm/crate patch 相等。
- Gradle：wrapper URL、distribution checksum、wrapper JAR checksum、AGP 和 JDK 构成一组验证。
- Apple：workflow Xcode path、build number 和 doctor expectation 构成一组验证。

### 定时漂移报告

每周运行非阻塞 `toolchain-drift` workflow，从本文末尾的官方端点检查：

1. 当前稳定/LTS 是否出现安全或补丁发行版；
2. GitHub runner 是否准备迁移/弃用，指定 Xcode path 是否仍存在；
3. Tauri core/API minor 和插件双端 patch 是否仍满足约束；
4. AGP/Gradle/JDK/Kotlin 的官方兼容矩阵是否改变；
5. committed action SHA 对应 tag 是否出现安全公告或新 patch。

该 job 只产出报告或 issue，不直接改 main、version files、lockfiles 或 wrapper。升级由单独 PR 完成，PR 必须：附官方 release/兼容来源和校验和；更新全部重复 pin；重新生成 lockfile；通过五平台矩阵；记录回退版本和风险。

Dependabot 可管理 npm、Cargo、Go modules、Gradle 和 GitHub Actions，但官方支持表在调研日对 pnpm lockfile 只明确列到 pnpm 10，因此 pnpm 11 的漂移不能只依赖 Dependabot。定时 job 应直接读取 pnpm 官方 registry/发布信息，仍由人工审核升级 PR。

## 签名占位与 Secret 边界

T009 和普通 PR 不需要任何生产秘密。P00-06 应把签名拆成独立 `release-sign` jobs，仅允许受保护 tag 或手动发布触发，并绑定设置 required reviewers 的 GitHub Environment；审批完成前 secrets 不可进入 runner。

建议占位名：

| 平台 | 占位 secret/config | 处理要求 |
| --- | --- | --- |
| Apple distribution | `APPLE_DISTRIBUTION_CERT_P12_B64`、`APPLE_DISTRIBUTION_CERT_PASSWORD`、`APPLE_PROVISIONING_PROFILE_B64`、`APPLE_TEAM_ID` | 只解码到 `$RUNNER_TEMP` 和临时 keychain，job 结束销毁 |
| App Store Connect/notarization | `APP_STORE_CONNECT_KEY_ID`、`APP_STORE_CONNECT_ISSUER_ID`、`APP_STORE_CONNECT_PRIVATE_KEY_P8` | 优先短期/最小权限 API key；不得进 cache/artifact/log |
| Android upload key | `ANDROID_RELEASE_KEYSTORE_B64`、`ANDROID_RELEASE_KEY_ALIAS`、`ANDROID_RELEASE_STORE_PASSWORD`、`ANDROID_RELEASE_KEY_PASSWORD` | 配合 Play App Signing，仅把 upload key 注入临时目录；不提交 `.jks`/`keystore.properties` |
| Windows signing | `WINDOWS_SIGNING_ENDPOINT`、`WINDOWS_SIGNING_ACCOUNT`、`WINDOWS_SIGNING_PROFILE` | 优先 OIDC + Azure Artifact Signing/Key Vault，避免可导出的长期 PFX |
| Tauri updater | `TAURI_SIGNING_PRIVATE_KEY`、`TAURI_SIGNING_PRIVATE_KEY_PASSWORD` | 与 OS code signing 是两套密钥；不得复用或进入 repository/cache |

Release workflow 继续遵守：顶层最小权限；fork PR 永不接触 environment；秘密用 environment/stdin 或临时文件传递而不是命令行；禁止输出 base64 内容；签名制品与未签名测试制品使用不同 artifact 名称和 retention policy。

## 实施验收清单

- [ ] 所有表中版本都在仓库内有唯一、精确、可机器读取的 pin。
- [ ] Gradle wrapper distribution 和 wrapper JAR 均按官方 SHA-256 验证。
- [ ] Tauri Cargo 直接依赖使用 `=`，JS dependencies 不使用范围；core/API minor 和 plugin patch 规则通过 doctor。
- [ ] 五个平台 job 使用明确 runner OS label，且实际版本断言先于 build。
- [ ] 干净 runner 上只靠仓库 pin/lockfile 可执行 build/test；cache miss 不改变结果。
- [ ] PR jobs 全部 unsigned，且无 release environment/secrets。
- [ ] 定时 drift 只报告，不自动改 pin；升级 PR 通过完整矩阵。
- [ ] CI job log 记录 runner image、OS、Xcode/SDK、JDK、Gradle、Node/pnpm、Rust/Cargo、Go 和 Tauri 实际版本；独立 provenance artifact 属于 P00-06。

## 官方来源

### Node、pnpm、Rust、Go

- [Node.js 官方发行索引](https://nodejs.org/dist/index.json)
- [Node.js release status](https://nodejs.org/en/about/previous-releases)
- [pnpm 官方 registry metadata](https://registry.npmjs.org/pnpm/latest)
- [pnpm installation/Node compatibility](https://pnpm.io/installation)
- [Rust 1.97.1 announcement](https://blog.rust-lang.org/2026/07/16/Rust-1.97.1/)
- [rustup toolchain file](https://rust-lang.github.io/rustup/overrides.html#the-toolchain-file)
- [Go release history](https://go.dev/doc/devel/release)
- [Go toolchain selection](https://go.dev/doc/toolchain)

### Tauri

- [Tauri official release dashboard](https://v2.tauri.app/release/)
- [Updating Tauri dependencies](https://v2.tauri.app/develop/updating-dependencies/)
- [Tauri prerequisites](https://v2.tauri.app/start/prerequisites/)
- [Tauri 2.11.5 Cargo manifest/MSRV](https://docs.rs/crate/tauri/2.11.5/source/Cargo.toml)
- [Tauri updater signing](https://v2.tauri.app/plugin/updater/)
- [Tauri macOS signing/notarization](https://v2.tauri.app/distribute/sign/macos/)
- [Tauri Android signing](https://v2.tauri.app/distribute/sign/android/)
- [Tauri Windows signing](https://v2.tauri.app/distribute/sign/windows/)

### Apple、Android 和 JVM

- [Apple Xcode SDK and system requirements](https://developer.apple.com/xcode/system-requirements)
- [GitHub macOS 26 arm64 runner image contents](https://github.com/actions/runner-images/blob/main/images/macos/macos-26-arm64-Readme.md)
- [Swift macOS installer and Apple platform constraint](https://www.swift.org/install/macos/package_installer/)
- [Swift 6.3 release](https://www.swift.org/blog/swift-6.3-released/)
- [AGP 9.3 release notes and compatibility](https://developer.android.com/build/releases/agp-9-3-0-release-notes)
- [AGP official Maven metadata](https://dl.google.com/dl/android/maven2/com/android/tools/build/gradle/maven-metadata.xml)
- [Gradle 9.5.1 release notes](https://docs.gradle.org/9.5.1/release-notes.html)
- [Gradle distribution/wrapper checksums](https://gradle.org/release-checksums/)
- [Gradle Java compatibility](https://docs.gradle.org/current/userguide/compatibility.html)
- [Temurin 17.0.19+10 release](https://github.com/adoptium/temurin17-binaries/releases/tag/jdk-17.0.19%2B10)
- [Kotlin releases](https://kotlinlang.org/docs/releases.html)
- [Kotlin Gradle/AGP compatibility](https://kotlinlang.org/docs/gradle-configure-project.html)
- [Android built-in Kotlin migration](https://developer.android.com/build/migrate-to-built-in-kotlin)

### GitHub Actions、cache 和 signing

- [GitHub-hosted runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
- [GitHub runner image update policy](https://github.com/actions/runner-images)
- [GitHub secure use: full commit SHA](https://docs.github.com/en/actions/reference/security/secure-use)
- [Dependency caching](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching)
- [setup-node cache behavior](https://github.com/actions/setup-node)
- [setup-go toolchain/cache behavior](https://github.com/actions/setup-go)
- [setup-java distributions/cache behavior](https://github.com/actions/setup-java)
- [setup-gradle cache and wrapper validation](https://github.com/gradle/actions/blob/main/docs/setup-gradle.md)
- [Cargo CI cache guidance](https://doc.rust-lang.org/stable/cargo/guide/continuous-integration.html)
- [Dependabot supported ecosystems](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories)
- [Deployments and protected environments](https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments)
- [Using secrets in workflows](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets)
- [GitHub OIDC](https://docs.github.com/en/actions/reference/security/oidc)
- [Apple distribution signing](https://developer.apple.com/documentation/xcode/distributing-your-app-for-beta-testing-and-releases)
- [Android Play App Signing](https://developer.android.com/studio/publish/app-signing)
