# SQLCipher Windows crash and cross-platform alternatives

Status: decision research for PR #106 / issue #103  
Checked: 2026-08-30  
Scope: Windows `cipher_memory_security` crash mechanism, five-platform consistency, and encrypted-SQLite alternatives. This note does not change product code, dependencies, or CI.

## Recommendation

Do **not** replace SQLCipher at this point. The evidence identifies a version-specific SQLCipher 4.14.0 Windows logging recursion, not an absence of Windows support or a failure of the database encryption itself. SQLCipher 4.18.0's own change log explicitly fixes a Windows crash caused by allocating memory while logging when `cipher_memory_security=ON`; current source comments describe the same allocation/logging recursion that Threadline's experiments expose ([4.18.0 change log](https://github.com/sqlcipher/sqlcipher/blob/master/CHANGELOG.md#4180---august-2026---4180-changes), [current Windows logging source](https://github.com/sqlcipher/sqlcipher/blob/master/src/sqlcipher.c#L2214-L2287)).

Recommended order:

1. Upgrade the bundled SQLCipher core to 4.18.0, using an audited source override if necessary until `libsqlite3-sys` publishes that core. Pin the exact upstream commit, source archive checksum, generated-amalgamation checksums, and retained license.
2. Run the existing wrong-key/live-WAL reproducer repeatedly on Windows and the full five-platform matrix with `cipher_memory_security=ON`. This validates the exact upstream fix while preserving one encryption format and one security policy.
3. Only if 4.18.0 still crashes, capture a native Windows stack and use a narrow `SQLCIPHER_OMIT_LOG` A/B build to determine whether another logging path remains. The flag is diagnostic evidence, not a release fix.
4. If the upgrade cannot pass, the only low-risk uniform fallback is to stop explicitly enabling enhanced memory security on **all** platforms and use SQLCipher's official default `OFF`. This requires explicit security-owner approval. It does not disable database encryption, key derivation, page HMACs, or SQLCipher's locking/sanitization of internal cryptographic allocations; it only stops extending wiping/locking to every allocation made by SQLite ([SQLCipher API](https://www.zetetic.net/sqlcipher/sqlcipher-api/#cipher_memory_security)).

`SQLCIPHER_OMIT_LOG` is not recommended as the permanent release policy. It removes the recursion path in 4.14.0, but it also removes SQLCipher's own error diagnostics and masks an already-fixed upstream defect. A 4.18.0 upgrade fixes the allocation behavior while retaining logging.

## Root-cause assessment

Confidence is **high**. SQLCipher 4.18.0's release note and source name the exact Windows crash and recursion mechanism; a native debugger stack is necessary only if the original reproducer still fails after the upgrade.

The observed stack overflow is not caused by Threadline's code size. It is consistent with an unbounded recursive error-reporting path:

1. Threadline's locked `libsqlite3-sys` 0.38.2 bundle identifies itself as SQLCipher `4.14.0 community` and enables `PRAGMA cipher_memory_security=ON`.
2. With enhanced memory security enabled, SQLCipher replaces SQLite's allocation methods. Every SQLite allocation calls `sqlcipher_mem_malloc()`, which calls `sqlcipher_mlock()` ([4.14 allocator source](https://raw.githubusercontent.com/sqlcipher/sqlcipher/v4.14.0/src/sqlcipher.c), especially lines 720-800).
3. On Windows, `sqlcipher_mlock()` calls `VirtualLock()`. If it fails, SQLCipher 4.14.0 emits a `WARN` message; the default log level is also `WARN`, and the default non-Apple/non-Android target is stderr ([4.14 lock source](https://raw.githubusercontent.com/sqlcipher/sqlcipher/v4.14.0/src/sqlcipher.c), lines 663-689; [default logging source](https://raw.githubusercontent.com/sqlcipher/sqlcipher/v4.14.0/src/sqlcipher.c), lines 237-257 and 405-424).
4. SQLCipher 4.14.0's Windows stderr formatter creates its UTF-8 and UTF-16 buffers with `sqlite3_vmprintf()` and `sqlite3_malloc()` ([4.14 Windows formatter](https://raw.githubusercontent.com/sqlcipher/sqlcipher/v4.14.0/src/sqlcipher.c), lines 2063-2094). Those allocations re-enter the enhanced allocator, which calls `VirtualLock()` again. Another lock failure emits another warning, which allocates again, and so on.
5. Each `sqlcipher_log()` frame in 4.14.0 also contains an 8,192-byte automatic buffer (`MAX_LOG_LEN`), so recursion consumes stack quickly ([4.14 logger](https://raw.githubusercontent.com/sqlcipher/sqlcipher/v4.14.0/src/sqlcipher.c), lines 2199-2276). Enlarging Threadline's worker stack delays but cannot terminate an unbounded recursion.

Windows makes the initial `VirtualLock()` failure plausible by design. Microsoft documents that every Windows version intentionally limits how many pages a process can lock; the maximum is the minimum working-set size minus overhead, and applications need to adjust the working set to lock more pages ([Microsoft `VirtualLock`](https://learn.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-virtuallock)). Raising the working set is not a robust product fix: Microsoft also warns that it removes physical memory from the rest of the system, may cause other operations to fail, and does not guarantee the requested reservation ([Microsoft `SetProcessWorkingSetSize`](https://learn.microsoft.com/en-us/windows/win32/api/memoryapi/nf-memoryapi-setprocessworkingsetsize)).

The upstream history independently matches this mechanism:

- SQLCipher 4.16.0 removed the default-visible `WARN` on `mlock` / `VirtualLock` failure because lock quotas are expected and not practically avoidable ([upstream commit `1fda83f`](https://github.com/sqlcipher/sqlcipher/commit/1fda83fa7cad3fcf2c97f57b97c6efd594bcaffe)). This prevents the 4.14 recursion under the default `WARN` threshold, but non-default logging could still expose it.
- SQLCipher 4.18.0 changed Windows log writes to use bounded stack buffers specifically because allocation failures and logging in allocation code could cause recursion. Its change log names the resulting Windows crash under `cipher_memory_security=ON` ([4.18 source](https://github.com/sqlcipher/sqlcipher/blob/master/src/sqlcipher.c#L2214-L2287), [4.18 change log](https://github.com/sqlcipher/sqlcipher/blob/master/CHANGELOG.md#4180---august-2026---4180-changes)).

Threadline's controlled evidence is consistent with the source analysis:

- PR #181 removed only the optional `cipher_memory_security` block in the ephemeral Windows checkout; the same executable then passed all four tests three times (`0, 0, 0`). The corresponding keyed build with the setting enabled overflowed three times ([experiment record](https://github.com/monkeylabx/threadline/pull/181#issuecomment-5404880158)).
- PR #106 subsequently showed that one-time initialization and a 64 MiB worker stack do not resolve the Windows failure ([PR #106](https://github.com/monkeylabx/threadline/pull/106)). This is expected for recursive logging and argues against ordinary deep but finite call depth.

### What `SQLCIPHER_OMIT_LOG` can prove

In SQLCipher source, `SQLCIPHER_OMIT_LOG` compiles `sqlcipher_log(...)` calls to an empty macro and makes runtime log configuration unavailable. The official Android integration also documents the flag as the way to exclude SQLCipher/JNI log output ([SQLCipher Android logging](https://github.com/sqlcipher/sqlcipher-android#logging)).

A controlled A/B must change only this compile definition, build one Windows executable, and run the original live-WAL/wrong-key scenario at least three times. Interpretation:

- enabled flag passes, baseline overflows: strongly confirms the logging recursion;
- both overflow: obtain a debugger stack and revisit allocator/mutex hypotheses;
- any other exit pattern: inconclusive, preserve artifacts and repeat.

The diagnostic should also record the exact SQLCipher version, runner image, executable hash, compile options, exit status, and native stack without retaining database keys or fixture data.

## SQLCipher platform and consistency assessment

SQLCipher is a maintained SQLite fork with on-the-fly AES-256 database encryption, tamper detection, memory sanitization, and strong key derivation. It preserves database-format compatibility within a SQLCipher major version across platforms, which is the important interoperability boundary for Threadline ([SQLCipher repository and compatibility statement](https://github.com/sqlcipher/sqlcipher#compatibility)). Community Edition is BSD-3-Clause licensed ([license](https://github.com/sqlcipher/sqlcipher/blob/master/LICENSE.md)).

Windows is an officially supported target, not an accidental port. The core source includes MSVC/Windows build support and Zetetic ships native Windows packages for x86, x64, and arm64 ([Windows integration](https://www.zetetic.net/sqlcipher/sqlcipher-windows/)). Official Community integrations also exist for Android and Apple; the Android library supports API 23+ and four common ABIs, and the Apple project is an official Swift package ([SQLCipher Android](https://github.com/sqlcipher/sqlcipher-android#compatibility), [SQLCipher.swift](https://github.com/sqlcipher/SQLCipher.swift)). Linux builds directly from the Community source ([documentation index](https://www.zetetic.net/sqlcipher/documentation/)).

For Threadline's Rust core, `rusqlite` / `libsqlite3-sys` is the strongest existing integration: it provides explicit `bundled-sqlcipher` and `bundled-sqlcipher-vendored-openssl` Cargo features that compile embedded SQLCipher source, instead of requiring a platform DLL or system package ([rusqlite build documentation](https://github.com/rusqlite/rusqlite#notes-on-building-rusqlite-and-libsqlite3-sys)). That gives desktop platforms the same C core and Rust API. iOS and Android still require simulator/device evidence, linker-symbol checks, packaging, and OS secure-key integration; desktop success alone is not mobile admission.

There are two different meanings of “platform consistency”:

- **Database consistency:** same SQLCipher major version, page format, KDF/HMAC parameters, migration behavior, and key lifecycle. SQLCipher already offers this.
- **Hardening consistency:** same optional `cipher_memory_security` policy. The clean choices are `OFF` everywhere now, or `ON` everywhere after 4.18.0 passes. A Windows-only exception is workable but is not the user's desired uniform policy.

## Alternatives

| Candidate | Five-platform and Rust fit | Encryption / SQLite compatibility | License and maturity | Migration cost | Assessment |
| --- | --- | --- | --- | --- | --- |
| **SQLCipher 4.18+** | Official Windows, Linux, Android, and Apple paths; direct `rusqlite` Cargo features | Full-database encryption; same SQLCipher 4 format across platforms | BSD-3-Clause; long-running maintained project and official mobile wrappers | Low: dependency/source upgrade plus regression evidence; no logical schema rewrite expected within major v4 | **Recommended** |
| **SQLite3MultipleCiphers 2.5.1** | Build files for Windows/Linux/macOS and Android binaries; no official Rust binding is listed, so Threadline would need a custom `libsqlite3-sys` amalgamation/build integration; iOS has linker/VFS caveats | VFS-based encrypted SQLite; supports its own ChaCha20-Poly1305 default and SQLCipher-compatible modes | MIT; active, but its own README says development was mainly on Windows and tested on Linux; security response is best-effort | Medium/high: new native build owner, compile-option parity, mobile packaging, key/error behavior, and DB/WAL migration/interoperability suite | Best open-source fallback, not a cheaper immediate fix ([project](https://github.com/utelle/SQLite3MultipleCiphers), [installation](https://utelle.github.io/SQLite3MultipleCiphers/docs/installation/install_overview/), [license](https://github.com/utelle/SQLite3MultipleCiphers/blob/main/LICENSE)) |
| **SQLite SEE** | ANSI-C drop-in source can be compiled per platform, but requires Threadline-owned Rust/mobile packaging and private source distribution controls | Full database and rollback-journal encryption; SQLite-compatible API | Proprietary perpetual source license, currently US$2,000; not acceptable if the requirement is fully open source | Medium: provider swap and data export/re-encryption; legal/procurement and private source handling | Technically credible, but violates the open-source/no-cost preference ([SQLite SEE](https://sqlite.org/com/see.html)) |
| **Turso / libSQL** | libSQL has an official Rust driver and MIT license; Turso is a Rust rewrite, but the official repository describes Turso as beta and libSQL as production-ready/legacy-feature focused | SQLite-compatible direction, replication/remote features; Turso at-rest encryption is explicitly experimental and uses a different implementation/format boundary | MIT and active, but local encrypted mobile storage is not the same mature target as SQLCipher | High: storage engine/API, packaging, behavior, file format, offline semantics, and migration all need revalidation | Interesting future architecture option, not a focused replacement for this fixed upstream bug ([libSQL](https://github.com/tursodatabase/libsql), [Turso](https://github.com/tursodatabase/turso), [experimental encryption](https://github.com/tursodatabase/turso/blob/main/docs/sql-reference/pragmas.mdx#encryption)) |
| **sqleet** | Simple cross-compile story, but no out-of-box Android/iOS integration | ChaCha20-Poly1305 encrypted SQLite | Public domain/Unlicense, but the maintainer explicitly marks it unmaintained and stuck at SQLite 3.31.1 | High and unsafe: stale SQLite baseline plus new format/integration | Reject ([project warning](https://github.com/resilar/sqleet#readme)) |
| **Application-level field encryption over plain SQLite** | Cryptographic libraries are portable, but every query/model must decide which fields to encrypt | Does not transparently protect schema, indexes, metadata, temporary files, or all WAL content; changes query semantics | Depends on chosen crypto library | Very high: schema redesign, indexing/search compromises, migration, and a larger authorization surface | Not an equivalent full-database replacement |

`wxSQLite3` itself is not a good Threadline candidate: it is a C++ wrapper designed for wxWidgets, uses SQLite3MultipleCiphers for current encryption, and carries `LGPL-3.0-or-later WITH WxWindows-exception-3.1`; Threadline would take a C++/wx integration it does not otherwise need ([wxSQLite3](https://github.com/utelle/wxsqlite3), [license](https://github.com/utelle/wxsqlite3/blob/master/LICENCE.txt)). Evaluate SQLite3MultipleCiphers directly instead if a fallback is required.

## Decision gates

Keep SQLCipher if all of the following pass:

- SQLCipher 4.18.0 eliminates the original Windows crash with enhanced memory security enabled;
- one reviewed core build/configuration is used for all intended platforms;
- existing wrong-key, corrupted-ciphertext, no-rewrite, WAL/SHM secrecy, migration, and secret-safe error tests pass;
- iOS and Android simulator/device packaging and runtime tests pass before product admission.

Reopen the replacement decision only if 4.18.0 still reproduces the crash with a native stack outside the fixed logger, if a supported target cannot link/run the same SQLCipher major-format policy, or if product requirements demand a capability Community SQLCipher cannot provide. In that event, spike SQLite3MultipleCiphers first; do not migrate to sqleet, and do not adopt SEE unless the open-source/no-cost constraint changes.
