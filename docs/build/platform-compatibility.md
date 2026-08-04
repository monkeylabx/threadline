# Platform compatibility baseline

Status: proposed T009 compatibility baseline
Checked: 2026-08-04

This document separates the SDK used to build Threadline from the oldest operating system that may run it. A current compiler or SDK does not imply a current-only runtime requirement.

## Terms

- **Build SDK/toolchain** controls which APIs the compiler can see and which store requirements the artifact can satisfy.
- **Minimum runtime** is the oldest OS allowed to install and run the product.
- **Target runtime** opts the app into a platform's runtime behavior contract; on Android it is independent from `compileSdk` and `minSdk`.
- **Release gate** is a version on which every candidate must pass install, launch, upgrade, storage, notification, network, and critical IM-flow checks.
- **Compatibility smoke** proves that the artifact starts and completes a small contract check; it is not equivalent to full product QA.

## Supported runtime matrix

| Platform | Build baseline | Tier 1 runtime support | Transitional / Tier 2 support | Initial architecture coverage |
| --- | --- | --- | --- | --- |
| Android | JDK 17; `compileSdk = 37`; SDK package `platforms;android-37.0`; production `targetSdk = 36` until Android 17 leaves preview | Android 9 / API 28 through the latest stable Android release | Android 17 preview is compile and forward-compatibility coverage, not the production behavior target | `arm64-v8a`; `x86_64` emulator |
| iOS | Xcode 26.6 and iOS 26.5 SDK | iOS 17 through iOS 26 | New major iOS previews are forward-compatibility lanes only | `arm64` device; `arm64` and `x86_64` simulator where available |
| macOS | Xcode 26.6 and macOS 26.5 SDK | macOS 14, 15, and 26 | No unsupported macOS release is a production target | Universal artifact: `aarch64-apple-darwin` and `x86_64-apple-darwin` |
| Windows | MSVC plus a supported Windows SDK; Tauri 2; WebView2 Evergreen | Supported Windows 11 releases, with 24H2 as the oldest general baseline | Windows 10 22H2 only for customers enrolled in Microsoft ESU; no support for unpatched Windows 10 | `x86_64-pc-windows-msvc`; ARM64 is a follow-up release lane |
| Linux Desktop | Build release artifacts on Ubuntu 22.04 while it remains the oldest supported glibc/WebKitGTK 4.1 baseline | Ubuntu 22.04, 24.04, and 26.04 LTS; Debian 13 | Debian 12 LTS through 2028; other glibc distributions are compatibility-smoke / best-effort until added to the release gate | `x86_64-unknown-linux-gnu`; ARM64 is a follow-up release lane |

The minimums are security support boundaries as well as API boundaries. A technically launchable but vendor-unsupported operating system is not Tier 1.

## Android policy

The current Android module already has `minSdk = 28`. This means Android 9 and later devices can install it. `compileSdk = 37` only selects the APIs available during compilation; it does not raise the install floor. Android's build documentation defines `minSdk` as the install floor and `targetSdk` as the runtime behavior and tested-version declaration.

The current module is a library skeleton, so it does not yet establish the final application's `targetSdk`. The application module must set:

```kotlin
defaultConfig {
    minSdk = 28
    targetSdk = 36
}
```

API 36 satisfies the Google Play requirement taking effect on 2026-08-31. Permanently private organization apps may be exempt from that store policy, but Threadline should use the same secure behavior baseline unless a customer-specific compatibility review says otherwise. Raise `targetSdk` to 37 only after Android 17 reaches the required stability level and its behavior-change suite passes.

The Rust/JNI build must also preserve API 28 as its NDK floor. Native symbols newer than `minSdk` can prevent a shared library from loading even when Java/Kotlin code guards the call, so the API-28 link target and device smoke test are both required.

Release gates:

- API 28 emulator/device: install, launch, FFI load, database open/migrate, enrollment, offline outbox.
- API 36 stable: full Android suite and store-target behavior.
- API 37 preview/current: compile, lint, and forward behavior smoke until promoted to stable.

## Apple policy

The Swift package already declares iOS 17 and macOS 14 deployment targets. Xcode 26.6 supports deployment targets older than those values, so compiling with the iOS/macOS 26.5 SDK does not force users onto OS 26. Apple requires App Store submissions to use Xcode 26 and the iOS 26 SDK as of 2026-04-28; that is a build requirement, not a minimum user OS.

Use availability checks for APIs introduced after iOS 17 or macOS 14. Every release must exercise both the minimum deployment version and the newest stable version; simulator coverage does not replace the real-device FFI, background, Keychain, notification, and memory-pressure checks owned by T010/P13.

The desktop Tauri bundle must explicitly set macOS `minimumSystemVersion` to `14.0`, and release builds must set `MACOSX_DEPLOYMENT_TARGET=14.0`. The Swift package setting alone does not constrain the separate Tauri desktop executable. macOS release artifacts should be universal rather than Apple-Silicon-only.

## Windows policy

Tauri uses WebView2 on Windows. Windows 11 contains WebView2, and Microsoft also deployed it broadly to Windows 10, but the installer must still handle machines where the runtime is missing. Use Evergreen WebView2 for ordinary connected deployments and bundle the Evergreen standalone installer for air-gapped enterprise installation. New WebView2 APIs require feature detection; a managed customer can pin a fixed runtime only with an explicit security-update process.

Windows 10 reached end of normal support on 2025-10-14. Threadline may support Windows 10 22H2 only when the customer is enrolled in ESU, which runs through 2028; this exception must be visible in deployment inventory and support diagnostics. The normal release gate starts at Windows 11 24H2 and follows Microsoft's supported Windows 11 servicing releases.

The hosted `windows-2025` build proves MSVC buildability, not Windows client runtime compatibility. Runtime checks need Windows 11 client VMs and, while the exception exists, a Windows 10 22H2 ESU VM.

## Linux policy

Linux compatibility is primarily an ABI and system-library problem, not a single version number. Tauri warns that glibc can break backward compatibility and requires building on the oldest base system intended for support. Ubuntu 22.04 and Debian 12 are suitable WebKitGTK 4.1 baselines. Therefore an Ubuntu 24.04 release build may accidentally require glibc 2.39 and fail on otherwise supported Ubuntu 22.04.

Keep modern Ubuntu lanes for workspace and forward checks, but build distributable Linux binaries/AppImages on Ubuntu 22.04 until that minimum is retired. Smoke-test the same artifact on Ubuntu 22.04, 24.04, 26.04 and Debian 12/13. Ubuntu 22.04 remains in standard maintenance until May 2027 and can move to a customer-specific Ubuntu Pro lane afterward; Debian 12 is LTS-supported until June 2028.

The first supported Linux architecture is x86_64. ARM64 needs its own native build and package validation; claiming it based on Rust cross-compilation alone is insufficient.

## CI and release evidence

The existing five-platform workflow is a build matrix. It should evolve into two layers:

1. **Per-PR build gate**: exact toolchains, lint/unit tests, cross-compile checks, unsigned platform builds, API-availability checks, and the Ubuntu 22.04 Linux release baseline.
2. **Release/runtime gate**: minimum and newest supported OS VMs/devices, installation and upgrade from the previous release, WebView/runtime presence, FFI load, encrypted database migration, crash/resume, offline outbox, notifications/deep links, signing, and rollback evidence.

Required gaps from the current skeleton:

- Add `targetSdk = 36` to the future Android application module while retaining `minSdk = 28`.
- Add Android API 28 and API 36 runtime lanes; keep API 37 as forward coverage until stable.
- Test iOS 17 and current iOS on real devices, not only a generic simulator build.
- Set and assert Tauri macOS 14 deployment metadata; produce both Intel and Apple Silicon slices.
- Build Linux release artifacts on Ubuntu 22.04 and run them on the supported Ubuntu/Debian matrix.
- Add Windows 11 client runtime tests and a separately identified Windows 10 ESU exception lane.
- Revisit this matrix at least every six months and before raising any minimum deployment target.

## Primary sources

- [Android: configure your build (`compileSdk`, `minSdk`, `targetSdk`)](https://developer.android.com/build)
- [Android NDK: SDK version properties and native `minSdk` behavior](https://developer.android.com/ndk/guides/sdk-versions)
- [Android 17 SDK setup](https://developer.android.com/about/versions/17/setup-sdk)
- [Google Play target API requirements](https://support.google.com/googleplay/android-developer/answer/11926878?hl=en-GB_ALL)
- [Apple: Xcode SDK and deployment target matrix](https://developer.apple.com/xcode/system-requirements)
- [Apple: minimum App Store SDK requirements](https://developer.apple.com/news/upcoming-requirements/)
- [Apple: runtime availability and deployment targets](https://developer.apple.com/documentation/xcode/running-code-on-a-specific-version/)
- [Tauri: macOS minimum system version](https://v2.tauri.app/distribute/macos-application-bundle/)
- [Tauri: AppImage/glibc compatibility baseline](https://v2.tauri.app/distribute/appimage/)
- [Tauri: platform prerequisites and WebView2](https://v2.tauri.app/start/prerequisites/)
- [Microsoft: supported Windows 11 releases](https://learn.microsoft.com/en-us/windows/release-health/windows11-release-information)
- [Microsoft: Windows lifecycle and Windows 10 ESU](https://learn.microsoft.com/en-us/lifecycle/faq/windows)
- [Microsoft: WebView2 Evergreen and fixed runtime](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/evergreen-vs-fixed-version)
- [Ubuntu release cycle](https://ubuntu.com/about/release-cycle)
- [Debian 12 LTS handover and end date](https://www.debian.org/News/2026/20260712)
