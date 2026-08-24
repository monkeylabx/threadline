# T010-A Rust Native Bridge Simulator/Emulator Spike

Status: implementation under verification

Issue: [#40](https://github.com/monkeylabx/threadline/issues/40)

This spike validates the host boundary only. It contains no message, sync,
SQLite, file, or cryptographic behavior and is not evidence for the real-device
gate in T010-B.

## Contract shape

Contract version 1 exposes four generation-checked opaque resource types:

- `Client` owns the lifetime boundary for child requests and streams.
- `Request` has a deterministic pending/committed/terminal state machine.
- `Stream` is a bounded, monotonic event queue resumed from a durable cursor.
- `Buffer` has an explicit length and an idempotent release operation.

The C declaration is
[`crates/client-ffi/include/threadline_client_ffi.h`](../../crates/client-ffi/include/threadline_client_ffi.h).
Swift links the C ABI directly. Kotlin calls primitive-only JNI entry points
that delegate to the same Rust registry and state machines.

JNI operations that create handles return a positive opaque handle on success
and the negated stable status code on failure. This preserves error identity
without passing JVM objects or output pointers through the boundary.

Rust never stores or calls a Swift/Kotlin object. Each host owns its worker and
delivery executor, pulls a result/event from Rust, and passes it through a
callback gate. Closing a Request, Stream, or Client closes the gate and waits
for already-running deliveries; queued or later deliveries are suppressed.
This avoids cross-runtime object retention and makes “no callback after close
returns” an enforceable host invariant.

Native delay waits are interruptible. Closing a resource wakes its Rust worker
instead of leaving a detached thread asleep for the caller-provided delay, and
the Client prunes released weak child registrations as new children are added.

## Request state and cancellation

```text
pending --cancel--> canceled
   |
   +--commit--> committed --complete--> succeeded
                    |
                    +--cancel--> already_committed (operation continues)

any non-closed state --close--> closed
```

The fault harness can delay before commit, panic behind a Rust unwind boundary,
or return the reserved unknown error. Panic diagnostics contain only the fixed
text `injected native bridge fault`; no payload is interpolated.

## Stream, backpressure, and resume

A stream accepts `cursor`, `event_count`, and `capacity`. The producer cannot
grow its queue beyond `capacity`; it waits on the consumer and increments a
backpressure counter. Sequence numbers must be strictly monotonic. The
duplicate-event fault terminates with `protocol_violation` before exposing the
duplicate. A late-event attempt after close is counted and suppressed.

The crash fixture writes and synchronizes the last accepted cursor, terminates
the process with exit code 86 without releasing native resources, then starts a
fresh process and verifies that the next sequences are `cursor + 1` and
`cursor + 2`.

## Stable status codes

| Code | Meaning |
| ---: | --- |
| 0 | ok |
| 1 | invalid argument |
| 2 | invalid or stale handle |
| 3 | closed |
| 4 | canceled before commit |
| 5 | isolated panic |
| 6 | cancellation arrived after commit |
| 7 | wait timed out |
| 8 | reserved push-adapter backpressure status |
| 9 | protocol violation |
| 10 | end of stream |
| 255 | stable wrapper for an unknown native failure |

The shared fault cases live in
[`crates/client-ffi/fixtures/fault-cases.tsv`](../../crates/client-ffi/fixtures/fault-cases.tsv).
A Rust contract test checks that the C header and both host enums cover every
fixture entry.

## Verification matrix

```text
cargo fmt --all --check
cargo clippy --workspace --exclude threadline-desktop-host --all-targets --locked -- -D warnings
cargo test --workspace --exclude threadline-desktop-host --locked

THREADLINE_FFI_LIBRARY_DIR=<target/debug> \
  swift test --package-path apps/ios

THREADLINE_FFI_LIBRARY_DIR=<target/debug> \
  ./apps/android/gradlew -p apps/android testDebugUnitTest lintDebug --no-daemon

# CI-only on the pinned Xcode/Android images
bash scripts/ci/run-ios-simulator-ffi.sh
bash scripts/ci/run-android-emulator-ffi.sh
```

The Simulator and Emulator tests execute 1,000 create/start/close/release
cycles and assert that registered resource counts return to baseline. The host
tests also cover dispatcher delivery, cancellation, panic/unknown errors,
bounded streams, duplicate suppression, cursor resume, and callback fencing.

## Security and evidence boundary

- Fixture payloads are fixed ASCII identifiers, never message or file content.
- Logs contain status codes, fixed fault names, tool versions, and test results
  only; they contain no message plaintext, prompt, token, key, or user path.
- CI records XCTest/Instrumentation results and prints failure-only Emulator
  diagnostics. Crash/Resume text, the iOS `.xcresult`, and Android JVM and
  Instrumentation reports are retained as workflow artifacts for 14 days.
  These are Simulator/Emulator results, not real-device evidence.
- T010-B must still validate iOS/Android arm64 loading, signing, Keychain or
  Keystore adapters, background/process eviction, memory tools, and real-device
  vendor behavior.
