# Trusted Desktop window and agentd IPC binding

Status: proposed binding profile `desktop-agentd-binding/1` for [#349](https://github.com/monkeylabx/threadline/issues/349), under [#350](https://github.com/monkeylabx/threadline/issues/350). Baseline: `0054bd4cecdd7b6868234c174af585f024f8f2e8` (#348/#371 integrated). This delivers design and reference expectations; no listener, real-window test or production admission is implemented here.

## 1. Responsibility, threat model and existing gap

The caller-binding module proves the provenance and current lifetime of one native Desktop caller before any Run lookup. Its interface is a checked immutable binding, not renderer identity fields or an `authenticated` flag. OS transport, host window registry and current Core identity are separate adapters at this seam. Run permissions, execution ownership and effects remain the [Core control-delivery profile](runtime-control-delivery.md) and [C1 local interface](agentd-local-interface.md).

Inputs: framework-injected caller handle, native window/document/Session generations, private launch-channel state, authenticated local transport and current Core Session facts. Output: a short-lived `TrustedBinding` scoped to exactly one request/frame. Side effects: bounded registration, fencing and teardown of IPC bindings/streams, never Task/Run transitions. Missing, expired, mismatched, revoked or unavailable evidence denies access before Run lookup. The test seam uses deterministic peer, registry, clock and Core-fact adapters; real OS/window tests must later cross the same interface. The document is independently reversible before implementation.

Attackers may control renderer payloads, another window with the same Actor/Device, stale callbacks, an unrelated process under the same OS account, socket names and captured public IDs/certificates. Native host, agentd, Core identity verifier and OS credential/cryptographic implementations are trusted. Native code injection, arbitrary memory/key extraction, administrator/kernel compromise and a compromised renderer's misuse of its own currently authorized interface are not solved by this profile. Those limitations do not permit cross-window identity substitution. Signed/hardened distribution, enterprise Device proof and worker containment require their existing admission reviews; dev evidence cannot claim them.

At this baseline `HostState` is one managed process state, despite its window-scoped comment; commands do not take an injected caller handle. `Session::connect` opens local storage without network verification. `SessionInfo` exposes configured Tenant/Actor/Device but has no verified Session ID. The dev server's `SessionVerifier` supplies a real fixture Session ID; environment IDs and a successful local store open do not establish that identity. C1's injected `AuthoritySource` is a test seam, not a bootstrap implementation.

## 2. Chosen mechanism and trusted fact sources

Use a native-host-owned agentd child, one exclusive anonymous duplex launch/control channel, and per-window mutually authenticated TLS 1.3 gRPC over local UDS. Windows keeps Named Pipe transport. There is no local TCP listener, raw ACP endpoint, extra local business RPC or shared application-wide window credential. TLS protects possession/replay on a connection; Core/current binding checks still decide authorization. A path/UID/PID/certificate by itself cannot name a Threadline Session or window.

| C1 fact | Real source and validation | Invalidation / observable denial |
| --- | --- | --- |
| Tenant/Actor/Device/Session | Core verifies the host-held session credential and current Device binding; native-only caller reference below. Agentd independently checks it through Core before Run lookup. | Logout, expiry, revoke, offboarding, reference loss or verifier outage: no Run lookup/emission. |
| Window identity | Framework-injected native `WebviewWindow` plus host-generated random window-instance ID; registry uses the actual handle/generation, not its reusable label. | Close/destroy/recreate fences old requests, even with the same label. |
| Document generation | Fresh host document tag bound to the single initially loaded document; compare against registry and captured command generation. | Full navigation/reload or unexpected load fences the old instance before replacement. |
| Session generation | Host registry epoch after verified login/reauthentication; exact Session ID/reference must match. | Same-window reauthentication never adopts old streams or input callbacks. |
| `peer_authenticated` | OS peer evidence plus full TLS client/server proof against launch-registered exact public keys; server attaches the resulting connection identity internally. | Failed peer query, unknown key, old incarnation, credential expiry or control-channel loss: no binding. |
| `active` / `unexpired` | Current Core Session/Device/member facts plus host and agentd generation/expiry checks. | No stale grace period or positive-authorization cache. |
| Observer/controller/owner/Grant | Separate current #348 Core authority read for the verified caller and Run. | Missing/changed facts deny that operation; IPC identity never grants execution ownership. |

### Core identity seam, explicitly missing today

Two planned native-only semantic operations require a bounded additive wire/adapter task: `IssueLocalCallerReference` and `CheckLocalCallerReference`. They are not new `LocalAgentService` RPCs or already implemented endpoints. Do not substitute `ListSessions`, `SessionInfo` or a claimed Session ID.

The first authenticates the native host with its Human session credential and independent Device evidence under [#192](session-device-verification.md)/[#200](device-proof-bootstrap.md); the dev substitute is the explicitly enabled server-owned static Session map. Both operations use the trusted configured Core TLS endpoint; a loopback address or existing plaintext dev IM endpoint cannot silently establish Agent identity. It binds a random opaque reference to the verified principal, exact target Runtime/generation, host/agentd incarnations, native window/document/Session epochs, purpose `agentd-local/1`, issuance/version and expiry. Core checks that target Runtime belongs to the same current operator/Device/Tenant. Host-supplied IDs can only narrow this linkage; a renderer cannot call this seam or choose identity.

The second authenticates agentd through #348's Runtime identity, resolves that exact reference/linkage and rechecks current Human Session/Device/member and Runtime facts with Core time. It returns bounded principal/version/expiry facts, no credential or Run metadata. Agentd compares them with the native registration before Run lookup. A reference alone is not a bearer credential, permission, observer role or Device proof; unrelated Runtime, generation or purpose cannot query it. Unknown/retired references fail closed. Core may discard these bounded ephemeral reference records on restart; loss requires a fresh verified binding, not restored authority or Run replay. Session credential bytes never leave the native host for agentd.

Issuance binds a stable native binding-operation ID and exact linkage; an authorized identical retry returns the same reference/expiry, never an extension, while changed linkage conflicts. Expired/retired identity is not recreated by retry. Reference loss forces the host to fence the old binding and register a fresh generation/key under current identity; changing the reference on an existing registration conflicts. Limit active references to eight per host and issuance/retired receipts to 64 per Session/host launch namespace; at capacity refuse issuance before mutation. Namespace expiry/retirement cannot reopen old registrations; private launch tombstones and current Core checks remain mandatory.

Dev policy is visibly **NOT PRODUCTION**, synthetic content/no user files/protected tools, no production proof fallback. An operator-enabled static map supplies exact Session IDs, expiry and revocation; configured host IDs must equal the verified mapping. Missing Session ID is an authentication failure, never a locally invented ID. Production identity and stream admission remain subject to #192/#200's unresolved reviews.

## 3. Native launch and per-window bootstrap

1. The host verifies the intended bundled agentd artifact against trusted deployment/build policy, creates a new unpredictable host incarnation and retains the actual child lifetime handle. It creates an anonymous duplex control channel and passes only the child endpoint at spawn, with all other handles closed. Expected process identity is tied to that retained launch lifetime; path/name or a reusable bare PID is insufficient. No secret travels in argv, environment, files, renderer state or general process discovery.
2. Agentd creates a fresh incarnation and ephemeral server TLS key in its own memory, binds an exclusively owned local endpoint, and sends its public certificate/pin plus endpoint and fresh launch challenge over that private channel. Host verifies the expected child lifetime/channel and replies with that challenge and protocol/host identity. Neither participant accepts an attach to an independently discovered existing agentd. Lost launch-channel custody, unknown version, startup failure or peer-identity ambiguity aborts Agent IPC without blocking ordinary IM.
3. Host creates a registry entry for the actual native caller, a fresh random window-instance ID/document tag and Session epoch, initially disabled for Agent commands. It obtains a Core caller reference, generates a new per-binding TLS client key locally and registers its public key plus exact binding/linkage/expiry over the private channel. Private keys never cross the channel. Agentd validates the reference through Core, then ACKs the exact registration. Only that ACK enables the binding. A partial/lost registration response leaves it disabled; identical registration may recover the same receipt, not another capability.
4. Host connects to the endpoint, checks OS peer evidence and the exact launch-pinned server key; agentd checks the expected host OS peer and exact registered client key. Full TLS proof is required on both sides. The transport attaches an immutable `(connection, host, agentd, window, document, Session epoch, binding generation, caller reference, purpose, expiry)` record. No caller-supplied gRPC metadata may replace it. Use one physical connection per active binding; HTTP/2 multiplexing cannot mix windows or Sessions.
5. A typed Tauri call uses the framework-injected caller, matching document tag and registry entry. The host strips/rejects caller identity/credential/endpoint fields and supplies only the existing C1 request over that entry's channel. Agentd validates identity/currentness before invoking its Run authority adapter. Both host and agentd recheck captured generations after asynchronous work and immediately before response/frame delivery.

The private control channel has versioned typed `Hello`, `Ready`, `RegisterBinding`, `RevokeBinding`, `Heartbeat` and `Receipt` records, not a general command tunnel or business service. Each direction uses positive contiguous sequence numbers, host/agentd incarnation and operation identity; `RegisterBinding` carries only the public certificate/key, Core caller reference and immutable linkage. Same-ID/same-record repeats recover one receipt; changed repeats, gaps, unknown fields/kinds or incarnation changes fence all bindings. Duplicated `RevokeBinding` is harmless and cannot enable anything. No control record contains Human/Runtime credentials, TLS private keys, Run content or an engine method. The wire task must encode these records as length-prefixed Protobuf, with `u32be` length ≤16 KiB and exact canonical comparison vectors before implementation; no ad-hoc JSON protocol or hand-edited SDKs.

Unneeded bootstrap descriptors are closed after spawn and marked close-on-exec thereafter. Agentd must close the control channel and TLS/IPC handles in every worker child; a leaked control endpoint invalidates the trust premise and blocks admission. Control EOF, child/host lifetime loss or a missing heartbeat fences transport and prompts the supervisor's own safe stop/reconciliation. It never confirms Core cancellation.

### macOS first; compatible OS adapters

| Platform | Required local endpoint / peer evidence | Additional proof and remaining gate |
| --- | --- | --- |
| macOS | Pathname AF_UNIX SOCK_STREAM inside a fresh private user directory (0700), socket 0600; check owner/type/no-symlink/exclusive creation and reject an existing endpoint. Both sides check `getpeereid`; use `LOCAL_PEERPID` against retained launch lifetime evidence. | Exact registered TLS keys and private launch custody are mandatory. Missing/unsupported process-lifetime verification denies admission. The OS adapter must prove its handle/start-identity behavior on the supported macOS version; code signing/hardening and containment remain separate. |
| Linux | Private pathname UDS, same ownership/path rules; connected-socket `SO_PEERCRED` plus retained process lifetime evidence. No abstract namespace or world/group-accessible listener. | Same TLS/binding rules; credential snapshot alone cannot attest current Session or safely survive PID reuse. Actual Linux adapter tests remain separate. |
| Windows | Local Named Pipe with explicit logon-SID DACL/minimal rights, remote clients rejected and exclusive first instance; query both peer process identities and compare retained process handles. No default Everyone/anonymous ACL. | Same TLS/binding rules and strictly allowlisted inherited handles. No TCP fallback. Actual Named Pipe adapter tests remain separate. |

The OS evidence identifies a process/account, not a window. The chosen composition of exclusive launch custody, native registry and key possession is the binding mechanism; same UID, a PID or socket possession alone fails. Replaced endpoint: pin mismatch fails before gRPC. A hostile same-user process may still deny service by removing a pathname; availability does not justify attaching to an unverified endpoint or deleting an unrelated stale path. Unlink only the socket owned by this retained launch instance after its teardown.

TLS follows RFC 8446 with ALPN `h2`, mutual certificate proof and exact registered leaf public-key pins, client/server EKU and validity checks. No system-CA/TOFU fallback, early data, session tickets/resumption, post-handshake identity switch or retrying a command as TLS recovery. Self-signed local leaves are accepted only by this explicit pin policy; failed signature/algorithm/expiry checks are not ignored. The TLS/X.509 provider and algorithm configuration must be separately pinned/reviewed and tested; this document installs no library or custom signature scheme. Production native hardening is not established by local TLS.

## 4. Window/document/Session isolation and lifetime

Native registry keys include actual webview instance, random window ID, document generation, host Session epoch and agentd incarnation. A display label, selected Channel, React store, JS `session_info` result or `run_id` cannot select an entry. Use separate native connections/keys/cancellation tokens for separate windows, even when they share the Human/Device or the same backing IM store. Registry changes serialize with call admission; no network I/O holds that lock. Recheck captured generations after I/O before releasing data.

Tauri application commands must have explicit allowlisted command permissions and webview scope, no wildcard remote capability, and exact trusted bundled origin. Development origin, if enabled, is an exact operator-configured loopback origin and port, never `localhost:*` or a request override. No untrusted iframe/subwebview or remote content may share the Agent-capable webview. Deny window creation/navigation/download features outside their separate native allowlists; DOM/CSS hiding is not authorization.

One native window instance admits one initial document. Full navigation/reload is denied in place; an intentional reload first revokes the binding, destroys the old webview/window instance, then creates a fresh instance/tag/key. This includes same-origin reload and label reuse. SPA navigation within the same document does not change identity. Any unexpected page-load/navigation event disables Agent commands and fences the entry; it cannot reuse initialization state. Native hooks must enforce this before replacement; if the platform cannot prove that ordering, Agent IPC stays disabled for that mode. Existing IM rendering/refresh behavior is not changed by this document.

A per-instance random document tag may be placed in the initial native initialization script only for its exact origin. It is a request-generation selector, not a credential or identity source; the host still checks injected webview and registry. There is no renderer command to mint/register a binding. An old queued request carries the retired tag; no lookup by label can retarget it to the new document. Do not reuse a static initialization tag across reloads. Agent activity is returned only through the originating invocation/channel after a final native generation check; no application-wide event broadcast containing text.

| Event | Required native/agentd invalidation |
| --- | --- |
| Close/destroy, full navigation/reload | Disable local entry first; cancel pending callbacks/watches, send RevokeBinding, erase key/reference and retire generation. New instance gets no old receipt/stream/content authority. |
| Reauthentication/account/tenant/Device switch | Fence old Session epoch before issuing new credentials, then reverify and register anew. Old same-window watch is invalid even when Actor/Device are unchanged. |
| Core Session/Device/member revoke or expiry | Current identity check denies before Run lookup/frame emission; fence affected bindings, clear unauthorized per-binding data. Notification only accelerates this check. |
| agentd restart or lost control channel | All old registrations/connections/streams invalid; new launch/incarnations and current Core checks required. Old local intake/outcome is not replayed; #348's uncertain-effect barrier persists. |
| Host exit, leaked bootstrap handle, peer identity loss | Fence all local bindings. Supervisor stops its own already-owned workers as needed and reconciles through #348; no PID-targeted recovery or fabricated terminal state. |
| Rotation/expiry/time uncertainty | Old binding is fenced before replacement. No overlapping credential grace, no expiry extension from heartbeat or certificate-only validation. |

Revoking a binding is local transport state, not Core CancelRun. Closing a window alone does not cancel a collaborative Run; a still-authorized owner may continue independently. Loss of actual execution authority stops new effects through #348. If current authority is unavailable, no RPC fabricates controller permission; the supervisor can always stop its own contained worker independently. UI displays pending/unconfirmed until Core's verified terminal path commits.

## 5. Current authority, request ordering and replay

Agentd applies this order for every unary call and before every activity emission: bounded shape/transport checks → current registered key/connection/incarnations → Core current caller reference/Session/Device → compare exact principal/window/Session epochs → tenant-scoped Run lookup/current C1 observe/control facts → operation-specific owner/lease/fence/Grant/route/budget checks. Do not query a Run, consult its receipt, emit a ready frame or allocate intake before identity validation. C1's `binding.session`/`binding.window` equal the separately verified principal fields; two empty values never match as evidence.

`AuthoritySource.binding()` projects only those current facts; `run()` consumes #348's verified caller and current Core authority. `observe` and `control` belong to the current Human caller, not the Runtime's assigned Agent or execution owner. A caller may observe without owning execution, and controller permission never implies Approval authority. Locald context uses a separately scoped service purpose/key/linkage under #370; the agentd-window credential cannot be replayed as a context/Connector/Runtime/cleanup credential.

Disconnect/reconnect requires a new full TLS handshake and the same still-current registered linkage. Captured TLS bytes/public certificate are insufficient without the private key and live registration. Rotation retires old connection generations. C1 input duplicates retain the same Run/input ID/bytes and undergo current authorization before returning a receipt; transport failure cannot justify a fresh input ID or turn. A new native document/Session/agentd instance cannot resume old C1 stream IDs/cursors. Authorized clients start a fresh watch; missing plaintext history remains an explicit C1 gap/loss, not reconstructed from Core or another window.

Cancel/revoke/reauthentication races are linearized at the authoritative current check and the local generation fence. Invalidation-first drops the result/frame and prevents new admission; already admitted work may have happened and follows #348 reconciliation. Do not promise retroactive removal of data already delivered. No background task may capture a binding indefinitely or hold a stale allow across another effect/frame.

## 6. Credential custody and fixed dev limits

| Resource | Bound / failure behavior |
| --- | --- |
| Random values | Incarnations, reference/document/binding IDs and challenges: 32 CSPRNG bytes (opaque encoded identifiers ≤128 bytes); uniqueness checked within their issuer. RNG failure aborts, no timestamp/PID substitute. |
| Native key/reference custody | Private keys remain only in their owning native process memory, no renderer/argv/env/temp file/log/crash export. Public certificates and Core references travel only over the exclusive control channel. Wipe/drop on fence/exit; OS memory hardening is separately verified. |
| Binding lifetime | Maximum 5 minutes, also bounded by current Core Session/reference expiry; check both Core time and local monotonic deadline. Rotate by 4 minutes with fresh key/reference after fencing old generation. Unknown time basis denies. |
| Server key lifetime | Maximum 30 minutes; rotate via the same private channel with all old connections fenced before new pin acceptance. New incarnation never inherits a key/pin. |
| Launch/handshake/Core check | Five-second total deadlines per operation, ≤8 pending unauthenticated connections and rate limit before expensive work. Deadline/cancel releases resources and yields no allow. |
| Registry | ≤8 live windows per host/agentd pair, one connection per binding, ≤4 watches/window. Keep ≤64 registration/tombstone identities per launch; at capacity refuse new binding/rotation before mutation, require explicit safe relaunch; never evict to reopen an old key. |
| Control channel | ≤16 KiB/frame, 32 unacknowledged frames/256 KiB total; 1-second heartbeat, 3-second loss fence. Revoke ACK deadline 1 second; loss disables the whole Agent IPC pair instead of enabling replacement. |
| Content / local intake | Existing C1 per-Run limits/lifetime apply unchanged: 16 accepted later inputs, 64 KiB each; bounded 128 updates/256 KiB activity. No new durable plaintext or copied cross-window transcript. |

Counters/generations are checked positive signed-bigint values; overflow fences instead of wrapping. Receipt retention and current authorization are distinct. Loss of any native binding does not remove durable Core receipts/barriers or permit unsafe Run retry. All failure paths leave ordinary IM independent of Agent startup; an unavailable server still uses the existing IM offline/pending rules.

## 7. Error and disclosure contract

Use existing C1/shared ErrorDetail vocabulary; no new public error shape or secret diagnostic. Tauri's host maps these into bounded typed UI failures through a separately owned bridge task. Authentication failures have no Run subject or existence hint. Errors/logs exclude keys, pins/certificates, caller references/tags, credentials, text, paths and raw worker/Core responses; use a separate opaque diagnostic correlation ID.

| Failure | Connect/gRPC status; existing code | Observable forbidden work |
| --- | --- | --- |
| Missing principal/peer, unregistered/expired key, invalid incarnation | UNAUTHENTICATED; no object detail | Run lookups, ready frames, intake and effects all zero. |
| Injected caller/tag/window/Session mismatch | PERMISSION_DENIED; PERMISSION_DENIED | No replacement entry or foreign-window callback. |
| Current identity dependency unavailable/time unknown | UNAVAILABLE; no invented allow | No Run lookup/emission; no cached grace. |
| Current Session revoked/expired | UNAUTHENTICATED, matching #192 category | Old stream closes, pending data dropped. |
| Unknown Run / denied observe/control | PERMISSION_DENIED; PERMISSION_DENIED | No foreign resource disclosure. |
| Trusted provider returns inconsistent Tenant | PERMISSION_DENIED; TENANT_MISMATCH, no foreign subject | Defensive C1 check; never a caller-selected tenant lookup. |
| Old C1 stream / lost retained segment | FAILED_PRECONDITION; CURSOR_INVALID / SEQUENCE_GAP | No stream adoption or invented transcript. |
| Reused control identity with changed registration | ALREADY_EXISTS; IDEMPOTENCY_CONFLICT | No key replacement or extra registration. |
| Unknown private version/role/field, malformed length | UNIMPLEMENTED or INVALID_ARGUMENT as applicable | Fence bootstrap; no legacy/unprotected fallback. |
| Bounds/queue capacity | RESOURCE_EXHAUSTED; RATE_LIMITED / PAYLOAD_TOO_LARGE when applicable | No partial registration or unbounded allocation. |
| Current Run owner/lease/fence/Grant invalid | Existing C1 NOT_EXECUTION_OWNER / LEASE_LOST / FENCING_TOKEN_STALE / GRANT_REVOKED / GRANT_EXPIRED | Identity remains distinct from effect authority; no protected effect. |

## 8. Sequence traces, successors and evidence

1. Two windows: native A/B receive distinct instance/tag/key/reference; A's typed call can use only A's native channel; B presenting A's tag/stream is denied. Matching Actor/Device and shared storage do not merge registries.
2. Reauthentication: fence old epoch and native callbacks → revoke old binding/ACK → verify new Session → register new key/reference → fresh watch. A delayed old ready/text/input response is dropped.
3. Restart: private channel/lifetime loss fences all entries → owned supervisor cleanup remains separate → new host/agentd handshake → fresh verified windows. Old intake/result uncertainty remains behind Core's barrier; a new TLS key cannot clear it.

Named bounded successors belong under #350; all remain unclaimed and gated on their actual contracts. No platform code, shared verifier, Proto/SDK, manifest, lockfile or migration is changed here.

| Gap / owner | Required separate task |
| --- | --- |
| B1 — Contracts/Integration, #374 | Private launch/control record schema, canonical comparison/replay vectors and local transport metadata contract; no extra C1 business method. |
| B2 — Contracts/Integration, #375 | Core native caller-reference issuance/check wire; preserve #192/#200 auth, current identity/disclosure semantics and #361's separate Run authority. |
| B3 — Core/Security, #376 | Current dev Session/reference verifier adapter and restricted Runtime lookup, no production OIDC/device-proof shortcut. |
| B4 — Desktop/Client, #377 | Native per-window/document/Session registry, typed caller extraction and lifecycle fencing; keep changes in a new owned host module before separate wiring. |
| B5 — Runtime/Security, #378 | agentd private bootstrap, macOS UDS/TLS peer validation and current TrustedBinding adapter; server registration separate from C1 behavior. |
| B6 — Integration/Security, #379 | Review/pin local TLS/X.509 and OS peer/lifetime dependency/configuration profile; negative pin/expiry/replay/descriptor tests, isolated manifests/lockfiles. |
| B7 — Integration, #380 | Minimal Desktop/agentd native command/launch wiring after B3–B6 and #368; no Task UI or IM transport redesign. |
| B8 — Quality, #381 | Actual macOS two-window/reauthentication/peer/restart evidence against the wired fake/no-effect runtime; real native evidence separate from fixture success. |
| B9/B10 — Runtime/Security, #382/#383 | Independently scoped Linux UDS / Windows Named Pipe adapters and real OS evidence after B5/B6; no cross-platform result inferred from macOS. |

#370 must consume the independently scoped locald context role and current caller/Run authority; it cannot reuse a renderer/window certificate as a file or Context Grant. #259/#350 remain blocked on business implementations and real-boundary acceptance. #328/#369 retain native supervisor/containment evidence; do not rerun Goose probes or count IPC authentication as worker cleanup.

[scenarios.json](../../test/fixtures/proto/desktop-agentd-binding/scenarios.json) is a fixed-time synthetic expectation table. Each `given` overlays trusted test defaults, never runtime fallback flags; `runLookups`/`emissions`/`newEffects` name forbidden observable work. Manual source/threat/trace review and JSON parsing are not an executable bootstrap or platform test. Run `node scripts/proto.mjs check`, `node proto/tools/verify-agentd-local-contracts.mjs`, JSON parsing and `git diff --check`; new executable conformance/registration waits for its bounded task and #335.

## 9. Primary-source checks and limits (2026-10-23)

The mechanism above is a project design choice. Platform primitives were checked against these primary sources, not experimentally verified here:

- [Tauri capabilities](https://tauri.app/security/capabilities/) and [runtime authority](https://tauri.app/security/runtime-authority/): explicit application-command/webview scope is required; overlapping capabilities combine permissions. Capabilities are only the first caller check.
- [Pinned Tauri 2.11.5 builder](https://docs.rs/tauri/2.11.5/tauri/webview/struct.WebviewWindowBuilder.html): navigation can be canceled; initialization scripts run on top-level navigations. These hooks motivate fail-closed document recreation, not a claim of complete native race evidence. Local pinned source confirms injected `WebviewWindow` comes from `CommandItem.message.webview()`, not a deserialized label.
- [Apple getpeereid](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man3/getpeereid.3.html) identifies effective peer UID/GID at connect/listen; [Apple XNU local socket declarations](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/un.h) expose peer PID/audit-token options. Neither proves a Threadline Session/window.
- [Linux unix(7)](https://man7.org/linux/man-pages/man7/unix.7.html) documents connected peer credential snapshots; [Microsoft pipe security](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights) and [CreateNamedPipe](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-createnamedpipea) motivate explicit ACL/local-client configuration. OS adapter support and lifetime races must be tested per platform.
- [RFC 8446](https://www.rfc-editor.org/rfc/rfc8446.html) supplies TLS 1.3 certificate proof and warns about 0-RTT replay. Local pin registration, window linkage and expiry/revocation policy are Threadline obligations, not automatic TLS properties.
