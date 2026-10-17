# Worker NATS connection custody and permission proof

Status: Draft for #203, 2026-09-30. Base
`0d0c77bfd25149916771034182bd53cfb6a57a20`, which is issue baseline `b045d402`
plus documentation/CI-only commits. Of those, #206 makes `workspace-linux` skip
the NATS steps for docs-only PRs, so this PR's CI does not run the live probe;
no other cited input changed. This task changes documentation only: no Worker
code, NATS config, probe, CI, credential or deployment. It does not unlock
[#148](https://github.com/monkeylabx/threadline/issues/148), which stays
`needs-info` until the successors in Section 9 are implemented and independently
reviewed. Every "must" is a proposal; accepting #203 does not approve D1-D10.

## Inputs and current facts

- #148 security review: in NATS Server 2.10.20 the USER.INFO service import is an
  ordinary subscription path. A matching subscriber can observe the request and
  race a forged reply, and nats.go accepts the first reply. A constructor
  `New(*nats.Conn, Mapping)` therefore proves neither account isolation nor
  responder authenticity.
- #175 fixture ([config](../../deploy/nats/nats-server.conf),
  [validator](../../deploy/nats/contract.py), [probe](../../deploy/nats/probe.py)):
  - Accounts: `SYS` (no principal) and `THREADLINE_WORKER` (one bcrypt user).
  - Publish allow is exactly `threadline.domain.events.v1` and
    `$SYS.REQ.USER.INFO`; subscribe allow is only `_INBOX.threadline.worker.>`.
  - Denied: publishing that inbox, and subscribing `$SYS.REQ.USER.INFO` and
    `$SYS.REQ.USER.*.INFO`.
  - Absent: anonymous user, auth callout, leaf, gateway, and any configured
    import or export.
  - The probe checks exact USER.INFO identity and permissions, rejects anonymous
    and wrong credentials, checks that those subscriptions and inbox publication
    are denied, and checks that JetStream state is unchanged.
  - The development password is committed by design in the fixture controls and
    the dev-stack document.
- #179 runs `nats-server -t`, both unit suites and the live probe against the
  pinned `nats:2.10.20-alpine` in `workspace-linux`
  ([build.yml](../../.github/workflows/build.yml)).
- The [dev stack](../development/private-dev-stack.md) and
  [Kind manifests](../../deploy/kind/stack.yaml) allow only the Worker-labeled
  edge to NATS. The dev Secret is unmounted from dependency workloads, and the
  host probe reads it as cluster admin. The claim is limited to the fixture.
- Worker code: [main.go](../../services/worker/main.go) is empty and nothing
  opens a connection.
  - [outboxinspect](../../services/worker/internal/outboxinspect/inspect.go)
    takes a caller's `*nats.Conn` plus `jetstream.JetStream`. It issues
    `AccountInfo`, `StreamNameBySubject` and `Stream` info requests: publishes to
    `$JS.API.INFO`, `$JS.API.STREAM.NAMES` and `$JS.API.STREAM.INFO.<stream>`, or
    to `$JS.<domain>.API.*` when a domain is set.
  - The fixture allows none of these subjects, so it cannot host today's
    inspector.
  - [outboxpublish](../../services/worker/internal/outboxpublish/types.go)
    binds a validated `Mapping`.
- nats.go v1.53.1 client:
  - `Conn` exports a mutable `Opts` field and embeds writable `Statistics`
    (including `Reconnects`). Any holder can subscribe, publish or request, and
    `jetstream.JetStream.Conn()` returns the connection.
  - `UserCredentials` and `RootCAs(file)` re-read their files on every
    (re)connect. `UserJWTAndSeed`, `Nkey` and `Secure(*tls.Config)` take
    in-memory material.
  - Without `TLSHandshakeFirst` the server INFO, including the server ID, is
    read before TLS.
  - Permission violations arrive only through the `ErrorHandler` callback.
- nats-server v2.10.20:
  - The reply is `{server{name, id, tags, ...}, data{user, account, permissions,
    expires}, error}`.
  - The request is imported into `SYS` as `$SYS.REQ.USER.<account>.INFO`. Every
    server subscribes `$SYS.REQ.USER.*.INFO`, and the server source notes that a
    remote server can still answer.
  - Config reload disconnects clients no longer authorized, removes unauthorized
    subscriptions and closes clients whose account changed. A still-authorized
    connection stays connected.

| Proven for the fixture only | Production requirement | Status |
| --- | --- | --- |
| One enumerated principal, validated by exact config comparison | Every rendered production config passes an equivalent gate before any apply, reload or JWT update (P5) | Not built |
| That principal is denied USER.INFO subscriptions and inbox publication | Same for every non-server principal and every server-originated path into the inbox (P1-P4) | Not built |
| Anonymous and wrong credentials rejected | Plus TLS-first with a verified server identity and no plaintext listener (P7) | Not built |
| Dev credential in a Secret plus a NetworkPolicy edge | Immutable mount, in-memory loading and rotation (P6) | Not built |
| None | A single trusted in-process owner of the connection (Section 3) | Not built |

## 1. Trust model and principals

**Trusted:**
- the NATS server binaries and the reviewed rendered config Platform applies;
- the single Worker Go process and everything linked into it (Go has no
  in-process isolation);
- the deployment PKI;
- Platform tooling and operators.

An out-of-band broker or cluster administrator is outside this contract, as the
dev-stack document already states.

**Untrusted:** every other workload (Core, Realtime, future consumers), every
principal those workloads hold, and network peers.

| Principal | Account | Holder | May do |
| --- | --- | --- | --- |
| Server internal clients | `SYS` | NATS server | Serve USER.INFO and the JetStream API |
| Worker runtime principal (count per D7) | `THREADLINE_WORKER` | Worker custody module only | The exact set in P1 |
| Provisioning principal | `THREADLINE_WORKER` | Platform provisioning job only | Create and update the reviewed Stream and consumers; carries the P1 denies |
| Probe principal | Each probed account | Platform pipeline only | Negative probes; carries the P1 denies |
| Consumer principals (D9) | `THREADLINE_WORKER` | Consumer workloads | Pull from Platform-provisioned consumers only; carry the P1 denies |

## 2. What a constructor receiving `*nats.Conn` cannot prove

| Property | Observable from a received connection? | Who establishes it |
| --- | --- | --- |
| Authenticated user and account | Only via a USER.INFO reply, which is authentic only under P1-P4 | Platform (P); Worker matches at runtime |
| Exclusive ownership | No: any holder can subscribe, publish, or mutate `Opts` and `Statistics` | Worker assembly (Section 3) |
| TLS and server identity | Partly (`TLSConnectionState`); the dial options and a pre-TLS INFO are unknown | Custody module dials |
| Inbox prefix, server pool, credential source | Readable from `Opts`, but mutable and not authority | Custody module sets them and never exposes them |
| Other principals, imports, mappings, callouts, listeners, routes | No, by least privilege | Platform gate (P5) |
| Future reconnect, reload or rotation | No | Evidence binding (Section 4) and lifecycle (Section 6) |

A caller's connection, flag, or statement that the broker is configured is an
assertion, never evidence. Custody is separate from reply parsing: #148's
bounded-response, single-document, API-error and pattern-matching rules stand
unchanged. This document adds only the binding and the preconditions.

## 3. Proposed assembly contract (Worker)

A new package, proposed as `services/worker/internal/natscustody`, is the only
code that dials NATS or holds the `*nats.Conn` (D1).

**Inputs.** One trusted startup-only deployment file, parsed strictly like the
Outbox policy file, supplies:
- `tls://` server URLs (1..N) and the JetStream domain;
- CA bundle and credential file paths (form per D2);
- the expected account, the expected user identity and the exact expected
  permission set;
- the inbox prefix `_INBOX.threadline.worker` and the `Mapping`;
- the attestation record (Section 5).

The module reads the credential and CA bytes once at startup, then dials only
with in-memory options (`UserJWTAndSeed` or `Nkey`, and `Secure` with a fixed
`RootCAs` pool), so a changed file never takes effect mid-process. For NKey or
JWT it derives the public identity locally and requires it to equal the expected
user before dialing; the bcrypt form cannot do this. Credential bytes never
appear in environment values, logs, errors or panics.

**Fixed options:**
- `TLSHandshakeFirst`, server-name verification, no `InsecureSkipVerify`, and no
  custom dialer;
- `CustomInboxPrefix`, and `IgnoreDiscoveredServers` with the fixed URLs;
- a fixed client name;
- `ErrorHandler`, `ReconnectHandler` and `DisconnectErrHandler` that only
  invalidate evidence.

**Surface.** The module returns a `*Custody`. Its exported surface exposes no
`*nats.Conn`, `jetstream.JetStream`, `Options`, subscription or raw publish. It
constructs `outboxinspect`, `outboxpublish` and the #148 checker internally and
hands out only those narrow capabilities plus a readiness view. There is one
connection per process, no plugin loading, and no debug endpoint exposing
connection or heap state.

**Enforcement.** A `go/types` analyzer test fails if any package other than
`natscustody` calls `nats.Connect`, `Options.Connect`, `nats.NewEncodedConn`,
`jetstream.New*`, `(jetstream.JetStream).Conn`, `nats.SetCustomDialer` or the
`CustomDialer` option. It also fails if any package outside the allowlist
(`natscustody`, `outboxinspect`, `outboxpublish`, `outboxcredential`) names
`*nats.Conn` or `jetstream.JetStream`, or if `reflect`/`unsafe` is applied to
those types.

## 4. Evidence binding

`Check(ctx)` issues exactly one USER.INFO request on the custody-owned
connection with #148's fixed subject, empty body and caller context, and parses
it with #148's bounds. It then requires:

1. Before the request, `S0` read in the order `Reconnects`, `ConnectedServerId`,
   `Reconnects`, with equal counts, plus a completed TLS handshake with verified
   chains.
2. `server.id` equals `S0`'s server ID. A reply from another server is
   `unavailable`, not `incompatible`, because remote servers can answer. In the
   reply, `server.name` must be in the attested server names and `server.tags`
   must contain the attested config digest tag.
3. `data.account` equals the expected account, and `data.user` equals the
   expected identity.
4. `data.permissions` is exactly the expected set (D3). Each allow and deny list
   matches as a set, `responses` is absent, and there are no queue-scoped
   entries. `expires`, when present, exceeds the evidence lifetime.
5. `S1`, read the same way after the reply, equals `S0`.

Evidence is `{S0, expected-set digest, attestation digest, monotonic time}`. It
remains valid only while all three hold:
- the current snapshot equals `S0`;
- its age is at most `T_recheck` (D4);
- `ErrorHandler` has reported no permission violation since it was recorded.

Otherwise the result fails closed: `unavailable` for transport, timeout,
responder or remote-server replies, and `incompatible` for mismatches.
Readiness then denies new Claims, and existing Claims follow their persisted
deadlines ([Outbox policy](../architecture/transactional-outbox-policy.md) §5).
Core's Durable ACK never depends on this.

The binding shows the Worker's effective permissions on that server, and that
the server claims the attested config revision, **only if P1-P4 hold**. A forged
reply could copy `name` and `tags`. This does not prove data-plane publish,
PubAck, replication, other servers' config or universal negative least
privilege.

## 5. Deployment invariants (Platform)

The Worker cannot observe other principals without breaking least privilege, so
P1-P7 are an explicitly justified trusted precondition. They are enforced by a
Platform admission gate and post-apply probes.

The result is an **attestation record** per rendered-config revision, carrying:
- the config digest;
- the server names;
- the expected-set digest;
- the pass result.

Platform stamps the digest into each server's `server_tags`, and the Worker's
trusted file carries the record. Section 4 matches it against the reply, which
binds the attestation to the connected server's claimed revision but is no more
authentic than P1-P4. Whether the record is signed is D5.

- **P1 — Worker account principals.** Every principal in the Worker account is
  enumerated. Each denies subscribing `$SYS.REQ.USER.INFO` and
  `$SYS.REQ.USER.*.INFO`, and denies publishing `_INBOX.threadline.worker.>`.
  - Each subscribes only its own inbox prefix.
  - The Worker runtime publish allow is exactly the business subject(s),
    `$SYS.REQ.USER.INFO`, and the read-only JetStream API subjects above
    (domain-prefixed when a domain is set).
  - No runtime principal may create or modify Streams or consumers.
- **P2 — Server-originated paths.**
  - No Stream in the Worker account captures `_INBOX.>`, `$SYS.>` or `$JS.>`.
  - No consumer delivers into `_INBOX.threadline.worker.>`.
  - No account `mappings` or subject transform targets those subjects.
  - Consumers read only through Platform-provisioned pull consumers (D9).
  - The account has no configured import or export; the only imports are the
    server-provided USER.INFO and JetStream API imports, which P4 and P5 cover.
- **P3 — Entry points.** No `no_auth_user`, anonymous user or auth callout (a
  callout can mint users into any account).
  - No leaf, WebSocket or MQTT listener binds to the Worker account or `SYS`.
  - Routes and gateways require mutual TLS from the deployment CA, with
    Platform-held route credentials, and every gateway cluster runs the same
    reviewed account config.
  - In operator/JWT mode (D2), resolver pushes of account JWTs pass the same P5
    gate, and signing keys stay in Platform custody.
- **P4 — `SYS` account.** `SYS` has no externally authenticatable principal
  (D8). A `SYS` subscriber on `$SYS.REQ.USER.*.INFO`, `$SYS.REQ.USER.>`,
  `$SYS.>` or `>` would receive the imported
  `$SYS.REQ.USER.THREADLINE_WORKER.INFO` request and could answer it; so could a
  subscriber on the JetStream API.
- **P5 — Admission gate and probes.** A production-shape validator, the analogue
  of `contract.py`, runs over the rendered config before every apply, reload and
  JWT update. It rejects any added principal, import, export, mapping, callout,
  listener, Stream capture, consumer deliver subject or widened allow.
  - After apply, the probe asserts the exact per-account import list through
    `/accountz`.
  - It proves each P1-P4 denial with dedicated probe principals held by Platform
    (never the Worker credential), including `>`, `$SYS.>`, `$SYS.REQ.USER.>`
    and `_INBOX.>` attempts.
  - The dev fixture keeps its committed dev credential by design.
- **P6 — Credential custody.** The Worker credential is an immutable Secret, or
  a fixed `subPath` mount, readable only by the Worker pod (mode `0400`). It
  never appears in images, environment values or logs, and no other runtime
  workload reuses it.
- **P7 — Transport.** The client port requires `handshake_first` TLS with a
  deployment-CA certificate for the configured names, and there is no plaintext
  listener. NetworkPolicy allows only Worker-to-NATS for the Worker, and the
  Worker cannot reach the monitoring port.

## 6. Lifecycle

- **Reconnect** (same or another server) changes `Reconnects`, so evidence is
  invalid until `Check` passes again. Servers outside the fixed list, or without
  a CA-verified name, are never dialed (A09).
- **Account change on reload** closes the connection; after reconnect, the
  exact-account match fails and the Worker stays not ready.
- **Policy reload without reconnect** is invisible to the counter.
  - The primary control is the P5 gate before reload.
  - Secondary: a changed config digest tag fails the next periodic `Check`
    within `T_recheck`, and any reported permission violation invalidates the
    evidence at once.
- **Credential rotation** in v1 is restart-only (D6). Because material is read
  once, a changed Secret has no effect until pods roll. Platform deploys the new
  credential and rolls the pods (new custody, new evidence), then revokes the
  old credential. A stale connection is then disconnected, and its reconnect
  fails with the Worker not ready.
- **Unavailable evidence** (timeout, SYS down, malformed or remote reply, or a
  missing or mismatched attestation) leaves the Worker not ready, with no new
  Claims. IM stays usable.

## 7. Adversarial matrix

Owner W is the Worker (`services/worker/`), P is Platform (`deploy/`), and I is
Integration (`.github/workflows/`).

| ID | Attack | Preventing control | Evidence source | Fail-closed outcome | Owner |
| --- | --- | --- | --- | --- | --- |
| A01 | Another package obtains the connection (directly, via `JetStream.Conn()`, reflection, or a second dial), then subscribes the inbox, mutates `Opts` or `Statistics`, or publishes | Custody is the sole owner; opaque surface; `go/types` ownership analyzer | Analyzer and reflection tests; review | Test fails before merge | W |
| A02 | Another workload, or a probe, uses the Worker credential | P6 mount scope; dedicated P5 probe principals; NetworkPolicy | Manifest test that the Secret is mounted only in the Worker Deployment | Deployment gate fails | P |
| A03 | Wrong credential or account configured | Local identity check (NKey/JWT); exact `account`/`user` match | Unit test; runtime `Check` | Startup `invalid-input`, or `incompatible` | W |
| A04 | Same-account principal forges the first reply | P1-P3 | P5 validator and probe | Missing attestation: not ready | P |
| A05 | `SYS` principal, JetStream publisher or mapping answers or injects | P2 and P4 | Validator; `/accountz` import check; probe | Gate fails | P |
| A06 | Wildcard observer (`>`, `$SYS.>`, `$SYS.REQ.USER.>`, `_INBOX.>`) | P1 exact subscribe allow; P4 | Probe attempts each pattern | Gate fails | P |
| A07 | Publisher into `_INBOX.threadline.worker.*`, including via a consumer deliver subject | P1 publish deny; P2 | Probe per principal; Stream and consumer config check | Gate fails | P |
| A08 | Reload or JWT update widens permissions or adds a principal, import, mapping or callout | P5 before the change; digest tag and exact set on re-check | Attestation per revision; periodic `Check` driven by W3 | Gate fails, or not ready within `T_recheck` | P, W |
| A09 | Reconnect to another or rogue server, or a discovered URL | Fixed URLs; `IgnoreDiscoveredServers`; TLS-first with CA and name check | Options test; `S0`/`S1` snapshots | Not ready until re-check | W |
| A10 | Rotated or revoked credential still in use | Read-once material; restart rotation; reload disconnects revoked clients | Rollout test | Not ready; never ready on stale evidence | P, W |
| A11 | Evidence unavailable: timeout, API error, malformed or remote reply, missing attestation | `Check` taxonomy | Unit tables (#148) | `unavailable` or `incompatible`; no Claims | W |
| A12 | TLS downgrade or MITM, including tampered pre-TLS INFO | `TLSHandshakeFirst` and `handshake_first`; pinned CA; no plaintext listener | Options test; verified-chain check; plaintext-refused probe | Dial fails; not ready | W, P |
| A13 | Caller supplies a connection or an "attested" flag | The API accepts neither; the record comes only from the trusted file and is matched against the reply | Reflection test of the exported surface | Not constructible | W |
| A14 | Auth callout, resolver push, route, leaf, WebSocket or MQTT injects a principal or messages | P3 and P5 | Validator; listener and route checks; probe | Gate fails | P |

## 8. Automated entry points and environments

All are NOT RUN: none exists at this base. Live tests follow the repository's
environment-gated skip pattern, as in `outboxinspect`.

| Entry point | Environment | Must prove |
| --- | --- | --- |
| `cd services && go test -race ./worker/internal/natscustody/...` | Hermetic; fake dialer seam | Options, read-once loading, identity check, snapshot binding, invalidation, redaction, opaque surface (A01, A03, A09-A13) |
| `cd services && go test ./worker/internal/natscustody -run TestConnectionOwnership` | Hermetic `go/types` analysis of `services/worker` | A01 banned forms |
| `cd services && go test -race ./worker/internal/natscustody/... ./worker/internal/outboxcredential/...` with the live TLS-fixture variables set | Pinned NATS 2.10.20 TLS fixture from P1; test CA generated at run time; Docker | TLS-first dial, exact-set `Check`, digest tag, invalidation on forced reconnect, JetStream unchanged |
| `python3 deploy/nats/contract_test.py` (production-shape cases) | Python 3 | P1-P4 and P7 rejections for each forbidden addition |
| `python3 deploy/nats/probe.py` (extended) | Pinned image, Docker, `workspace-linux` | Per-principal denials with probe principals, `/accountz` imports, refused plaintext, unchanged JetStream |

## 9. Successor scopes

Each is at most 2 days and a separate issue after review. #148 stays blocked
until W1, P1, P2 and I1 are integrated and independently security-reviewed; only
then may W2 resume it.

| Task | Owner and path | Deliverable |
| --- | --- | --- |
| W1 Custody module, 2d | Worker: `services/worker/internal/natscustody/` | Section 3 surface, file parser, read-once material, options, evidence state, hermetic and analyzer tests |
| W2 Permission evidence check (#148), 1-2d | Worker: `services/worker/internal/outboxcredential/` | Section 4 `Check` with #148 parser rules; live test on the P1 fixture |
| W3 Readiness driver, 1-2d | Worker: `services/worker/internal/` (new module) | Periodic re-check, error-handler invalidation and the Claim gate; after W1 and W2 |
| P1 Production-shape gate and TLS fixture, 2d | Platform: `deploy/nats/` | P1-P5 and P7 validator, TLS-first fixture with JetStream API allows, probe principals, `/accountz` check, attestation record |
| P2 Credential distribution and rotation, 1-2d | Platform: `deploy/kind/`, `deploy/compose/`, and the future production chart | P6 immutable mount, rollout-then-revoke procedure and test, A02 manifest checks |
| I1 CI wiring, 0.5-1d | Integration: `.github/workflows/` | Run the P1 fixture probe and the W live tests in `workspace-linux` |

None of these rows may publish the business subject, claim PubAck or
replication, change a live deployment, or access production credentials without
its own reviewed task.

## 10. Decisions requiring review

| ID | Proposed decision | Owner |
| --- | --- | --- |
| D1 | Custody dials and solely holds the connection, replacing #148's `New(*nats.Conn, Mapping)` input | Worker, Security |
| D2 | Credential form: static NKey, operator-mode user JWT, or bcrypt (the bcrypt form loses the local identity check) | Platform, Security |
| D3 | Exact equality of the full effective set (including JetStream API subjects, no `responses`, no queue scoping) instead of Allow-present plus Deny check | Worker, Security |
| D4 | `T_recheck` and maximum evidence age | Worker, Platform |
| D5 | Attestation record format, digest tag, and whether it is signed | Platform, Security |
| D6 | Restart-only credential rotation in v1 | Platform |
| D7 | One shared principal for all Worker replicas (mutual inbox observation accepted) or per-replica principals | Platform, Security |
| D8 | No external principal in `SYS`; operators use the monitoring endpoint | Platform |
| D9 | Consumers are same-account restricted pull principals rather than a cross-account path | Platform, Realtime-worker, Security |
| D10 | Remote-server USER.INFO replies are treated as `unavailable` (possible readiness flapping in clusters) | Worker, Platform |

## This task's verification

Run `git diff --check`, resolve every local Markdown link in this file, and run
`node proto/tools/verify-contracts.mjs`, `python3 deploy/nats/contract_test.py`
and `python3 deploy/nats/probe_test.py`. They show that no wire, generated or
fixture surface changed; they do not test this proposal.
