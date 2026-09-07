# Trusted Session and Device verification

Status: Draft for #192, 2026-09-07. Baseline:
`66a1e75325f05f8ef6c53c2e3f88f185567ebbab`. This specifies proposed production
adapters around merged interfaces; it does not implement login or change Proto.
M0/G0 remains HOLD. The words “must” below describe the proposed contract to
review, not evidence that a production service enforces it today.

## Existing boundary and gaps

[#109](https://github.com/monkeylabx/threadline/issues/109) supplies
[`SessionVerifier`](../../services/internal/rpcmiddleware/auth.go), returning
`VerifiedSession{TenantID, ActorType, ActorID, DeviceID, SessionID}` from
`VerifySession(context.Context, string)`. The argument is a raw bearer
credential. The interceptor constructs an immutable `Principal`, removes the
Authorization header before calling the handler, validates identifiers and
maps failures. It does not implement an OIDC verifier, session database or
cryptographic device proof. It authenticates a stream at entry, not every
subsequent frame.

[`DeviceService` and `Session`](../../proto/threadline/identity/v1/identity_service.proto)
and [device lifecycle](../../proto/threadline/identity/v1/device.proto) are the
wire authority. [Current authorization](../../services/core/internal/authorization/current/current.go)
resolves organization/member/channel/ACL inside a caller-owned READ COMMITTED
transaction; it does **not** resolve Session, Device or Epoch state and does not
support DM resource references today. The
[Capability signature contract](../architecture/capability-grant-signature-contract.md)
requires independent trusted execution-device context, not a signed claim alone.

## Login completion and identity mapping

An administrator provisions the trusted tenant/issuer/client/redirect settings.
An unverified token, request header, email domain or caller-supplied tenant never
selects an unrestricted issuer, JWKS URL or database tenant. An organization
selection before login is only a lookup into that trusted configuration.

The native client uses authorization code flow in the system browser, S256 PKCE,
and a one-use login transaction containing unpredictable state and nonce.
Completion binds the redirect, issuer, client and verifier to that same
transaction; mismatches or replay fail without creating a Session. Validate
signature with approved algorithms and trusted issuer keys, exact issuer,
audience and authorized-party rules, expiry and nonce. A kid is only a key
selector inside the configured issuer. Unknown key lookup is bounded and fails
closed on untrusted or unavailable key material.
These requirements follow [OIDC Core, ID Token validation](https://openid.net/specs/openid-connect-core-1_0.html#IDTokenValidation)
and [OAuth security BCP](https://www.rfc-editor.org/rfc/rfc9700.html).

The stable identity mapping is `(configured tenant, issuer, subject) → existing
active Human Member`, subject to enterprise provisioning policy. Email/display
name is not a stable key and cannot silently link accounts. OIDC completion does
not create an Agent/Service actor, authorize a Device, add a cryptographic leaf
or mint a Capability Grant. Those require their own authority decisions.

Store only the server-selected identity and token/session metadata required for
authorization. API handlers consume a Threadline session credential, never an
arbitrary external ID token. Credential entropy, at-rest verifier construction,
expiry limits, key management and upstream refresh custody need a reviewed
credential/storage task before implementation; do not invent a hash/key scheme
in a handler. Client credentials live in platform secure storage.

## Device binding and enrollment bootstrap

An ordinary Session is bound to one Device ID and cannot be moved to another
Device by editing request fields. Before returning VerifiedSession the adapter
must validate the session credential, current active tenant/member/session,
expiry, authorized non-revoked device, actor/tenant equality and the independently
verified presenting-device binding. Copying `device_id` out of a bearer token is
not proof of which Device is presenting it. A stolen bearer can replay the same
string. The current interface contains no standard wire proof-of-possession
format; the trusted transport/device authenticator and its context contract
must be defined in successor S1. Without that evidence, fail closed for ordinary
device-bound protected operations; do not advertise the existing interceptor as
cryptographic device authentication.

There is a real bootstrap gap: existing DeviceService comments require a
Device-bound session, but the first Device has not yet been authorized. Propose
a separate one-use, short-lived enrollment-only authentication route following
OIDC, bound to a pending enrollment and device public key. It must not create an
ordinary Principal or permit Message/Sync/Capability/Artifact calls. Its endpoint,
proof, allowed methods, expiry and credential schema need an additive Contracts
change reviewed before any implementation. Do not exempt EnrollDevice in the
current interceptor globally or allow pending devices into ordinary APIs.

The first-device endorsement comes from Device Authority; later devices require
an existing authorized Device or the reviewed administrator exception. OIDC
success is insufficient. Only accepted endorsement/log state changes a pending
Device to AUTHORIZED. REVOKED and REJECTED are terminal; new hardware/restored
identity needs new enrollment. Revoking a Session stops API use, not MLS
membership. Revoking a Device invalidates its sessions/credential/unused
KeyPackages and orders group removal/rekey; production storage and atomicity for
those operations remain separate work.

## Failure mapping and cancellation

Preserve merged public categories in [auth.go](../../services/internal/rpcmiddleware/auth.go)
and the [shared error model](error-model.md). Do not add an enum here.

| Verifier observation | Existing return/category | Interceptor outcome |
| --- | --- | --- |
| Unknown credential, bad device binding, identity mismatch | `VerificationRejected` | unauthenticated, stable `session rejected` |
| Session expired | `VerificationExpired` | unauthenticated, stable `session expired` |
| Revoked session/device or disabled member | `VerificationRevoked` | unauthenticated, stable `session revoked` |
| Missing/malformed bearer | handled before verifier | unauthenticated, `authentication required` |
| Dependency unavailable/untrusted response | non-sensitive ordinary error | unavailable, `session verification unavailable` |
| Caller canceled/deadline | preserve standard context error | canceled/deadline-exceeded |
| Invalid verifier output or panic | existing guard | internal, stable redacted message |

When failure causes overlap, use a deterministic precedence: cancellation,
invalid credential/binding, revoked/disabled, expiry, success. Operational
failures with no trustworthy facts cannot be converted into a success or a
cached authorization. A malformed credential must not cause an issuer probe.

The verifier uses the caller's context for bounded I/O, checks cancellation
before work and after dependencies, never retains/logs the credential and
returns no raw upstream/database error. Context-dependent retry/timeout policy
belongs to caller/integration configuration, not hidden loops. Nil internal
inputs are programmer errors to reject safely, never an authentication bypass.

## Current facts, cache and transaction races

Authentication establishes identity for a request; a Principal is not permanent
authority. No positive session/device authorization cache with a stale grace
period is proposed for v1. Public issuer keys may be cached by issuer/kid and
reviewed rotation policy; that does not cache membership or revocation. Loss of
the authoritative store fails protected server operations closed. IM independence
from models remains intact; network outage is handled by durable local pending
work, not by bypassing authorization on the server.

Protected reads recheck before each page/frame is disclosed. Long streams
recheck before each protected action/delivery and are closed when revocation or
expiry is observed; connection presence cannot keep old privileges alive.

Mutations must re-resolve current Session/Device together with member/channel/ACL
and Epoch facts in the protected database transaction. The existing resolver
locks organization → member → resource → channel-membership → current ACL.
Successor S2 must define where Session/Device locks fit and update all relevant
revocation writers to the same order; adding a second query after authorization
without coordinated locks is insufficient. A draft ordering is organization →
member → session → device → conversation/membership → ACL → epoch. It is not
implemented by the current resolver and cannot be claimed ready until lock
compatibility with each mutation is reviewed. Multi-row locks use stable ID order.

Whichever conflicting transaction acquires the agreed lock and commits first
defines the authorization linearization point: revocation-first denies the
mutation; mutation-first may commit and then revocation prevents later work.
Do not promise retroactive removal of content already disclosed. Auth failure
must prevent domain writes, outbox inserts and success ACKs.

## Refresh rotation and replay

Propose one-use Threadline refresh credentials bound to the same session family
and authenticated device. Rotation atomically consumes the old generation and
creates exactly one successor. Concurrent refreshes cannot create two valid
children. A consumed credential replay revokes that family under the reviewed
policy; the losing legitimate concurrent client must reauthenticate. No grace
window or replayable raw-token response cache is proposed. If a response is lost
after rotation, reauthentication is the safe recovery path. This is a project
policy draft; [RFC 9700 §4.14](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.14)
provides the refresh-token protection rationale.

Logout revokes the session/family; device revoke or offboarding invalidates all
applicable sessions/families. Upstream IdP tokens and Threadline refresh tokens
are separate credentials with separate revocation responsibilities. All rotation
and revocation state changes need durable transactions. Audit only the allowed
metadata; never record bearer/refresh values, OIDC code, PKCE verifier, nonce,
private keys, issuer response bodies or confidential user content.

## Successor drafts and acceptance

Each row is a separate 0.5–2 day issue after contract review, not a bulk ready list.

| Draft | Owner / single owned area | Input and output | Required evidence before completion |
| --- | --- | --- | --- |
| S1 Device proof + bootstrap protocol, 1–2d | Contracts: `docs/contracts/` | Existing DeviceService and this draft → exact transport proof, bootstrap allowlist and additive Proto proposal | `git diff --check`; stolen bearer/device substitution, enrollment replay and ordinary API denial fixture table; separate Proto/SDK integration task |
| S2 Session/device lock and schema contract, 1d | Contracts: `docs/contracts/` | S1 + current resolver → row identities, lock order, revocation transaction and refresh uniqueness specification | `git diff --check`; concurrent revoke/send/refresh schedules with explicit winners; Integration reserves migration IDs |
| S3 Session schema, 1d | Integration: `db/` | Accepted S2 → one migration with keys, expiry, revocation/generation constraints | `make -C db migration-test migration-ledger-test` (pinned Atlas/PostgreSQL required); forward/rollback and concurrent unique-child checks in PostgreSQL; generated queries separate if ownership requires |
| S4 Current Session verifier, 1–2d | Core: `services/core/internal/session/` | S1/S2/S3 + authenticated transport context → SessionVerifier adapter | `cd services && go test -race ./core/internal/session ./internal/rpcmiddleware`; fake clock, unavailable store, cancel, mismatch, revoked and expired cases |
| S5 OIDC completion adapter, 1–2d | Core: `services/core/internal/oidclogin/` | Trusted issuer config + S1 bootstrap → validated identity mapping only | `cd services && go test -race ./core/internal/oidclogin`; local deterministic issuer fixture; wrong iss/aud/azp/nonce/state/PKCE/key and repeat completion fail; dependencies integrated separately |
| S6 Refresh state transition, 1–2d | Core: `services/core/internal/session/` after S4 | S2/S3 → consume/replace/family revoke | `cd services && go test -race ./core/internal/session`; two concurrent consumers, replay, response loss, rollback and expired device |
| S7 Device endorsement transition, 1–2d | Core: `services/core/internal/device/` | S1 + separately integrated Device schema/key-log contract → pending/authorized/rejected transition only | `cd services && go test -race ./core/internal/device`; wrong approver/tenant, duplicate endorsement, invalid chain and revoked-device reuse rejected |

S7 does not implement all-device revocation fan-out; that is a later bounded
transaction/outbox slice. Native key custody, UI, server endpoint assembly,
OIDC library manifests and Proto/codegen are separate ownership tasks.

For this Draft, run `git diff --check`, local-link checks and
`cd services && go test ./internal/rpcmiddleware`. Those tests establish the
existing seam still works, not successful login/device authentication. Acceptance
review must cover every matrix row and each explicit unimplemented dependency.
