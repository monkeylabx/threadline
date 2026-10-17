# Device proof and enrollment bootstrap

Status: **Draft for #200**. Baseline `b045d402ebf45537b1ac24bfbe4413465114d235`.
This document proposes a Human native-client profile, not an implemented login
service, approved security design or change to existing wire authority. G0/M0
remains HOLD. Production use is blocked by the decision register below.

## Authority and proposed approach

The [Session draft](session-device-verification.md),
[identity service](../../proto/threadline/identity/v1/identity_service.proto),
[device lifecycle](../../proto/threadline/identity/v1/device.proto) and
[authentication interceptor](../../services/internal/rpcmiddleware/auth.go)
are inputs. The interceptor accepts `Bearer` and calls
`VerifySession(context.Context, string)`; it has no presenting-key input and
checks streams only at entry. `DeviceService.EnrollDevice` presently assumes a
device-bound session. Neither interface can implement the bootstrap below unchanged.

Propose DPoP for ordinary HTTP RPC sender constraint, with a separate device
proof key registered to the authorized Device. DPoP is not device endorsement,
hardware attestation, message signing or MLS membership. A copied token retaining
the original device ID is still insufficient. The proof key must not silently
reuse a Crypto Profile's identity/MLS key; their algorithms and lifecycles differ.

Standards baseline: [RFC 9449 §§4,6–9](https://www.rfc-editor.org/rfc/rfc9449.html)
defines the DPoP header, signed JWT proof, token/key binding, request validation
and nonce challenges. Required proof members are `jti`, `htm`, `htu`, `iat`,
`ath` for resource requests, and the issued `nonce`. JOSE uses `typ=dpop+jwt`,
an approved asymmetric `alg`, and public `jwk`; never `none` or a symmetric key.
`ath` binds the exact token using base64url SHA-256. `htu` excludes query/fragment.
DPoP does not sign request bodies and requires HTTPS. The proposal imports the
normative validation rules rather than defining another signature format.

[RFC 7638 §3](https://www.rfc-editor.org/rfc/rfc7638.html#section-3) defines JWK
thumbprints: required key members, lexicographic ordering, UTF-8 JSON and hashing.
Use its SHA-256 thumbprint for `proof_jkt`; hashing arbitrary JSON is not equivalent.
Native OIDC continues to use the system browser and PKCE as specified in
[RFC 8252](https://www.rfc-editor.org/rfc/rfc8252.html), with the existing Session
draft's issuer/state/nonce rules. No external ID token is an ordinary API token.

Everything below is a **Threadline proposal**, including budgets, endpoint names
and data records. It needs Contracts/Security acceptance and interoperability
fixtures. It is not a claim that the RFC mandates these choices.

## Transport and credential boundary

- Client trusts only configured enterprise HTTPS origins and validated server
  certificates. No insecure redirect or token forwarding to another origin.
- Proposed first slice: unary POST Connect RPCs only, no query parameters,
  fragments, path aliases or redirects. Configure canonical external origin and
  procedure paths; compare `htu` using RFC 9449 §4.3 normalization. Reject ambiguous
  encoded paths, dot segments and proxy rewrites not covered by test vectors.
- TLS terminator is inside the trusted computing boundary. Prefer verification
  there; otherwise use an authenticated private hop and configured canonical
  route mapping. Ignore caller `Forwarded`/`X-Forwarded-*` for proof identity.
  An untrusted TLS terminator could alter a body; DPoP does not remedy that.
- Ordinary requests use one `Authorization: DPoP <opaque-access-token>` and one
  `DPoP` proof header. Reject duplicate/case-variant duplicates and malformed
  headers. Do not rewrite to `Bearer` to trick the current interceptor.
- Server-owned access record: token verifier, purpose=`ordinary`, issuer,
  resource audience (configured Core origin), tenant, Human actor, session ID,
  authorized device ID, `proof_jkt`, binding generation, expiry, revocation.
  Unknown token never selects issuer, tenant database, audience or key server.
  Token generation/verifier-at-rest and refresh custody are S2/S3 decisions.
- Authoritative Device proof binding: `(tenant, actor, device, proof_jkt,
  generation, endorsement/log reference, status)`. Public JWK possession alone
  cannot create this binding. Ordinary access record must match it exactly.
  Key replacement requires a new endorsed binding and invalidates old sessions;
  no first-use key pinning on an existing Session.

## Validation order and trusted context

1. Check caller cancellation, trusted route/transport and bounded headers before
   lookup or expensive crypto. Proposal: proof <=8 KiB, token <=4 KiB, `jti`
   <=128 ASCII characters; invalid encoding, duplicate JSON members, private JWK
   material, remote key URLs and unsupported JOSE critical extensions fail closed.
2. Resolve the opaque credential through the fixed issuer's trusted storage.
   Check purpose, configured audience, tenant/member state, session lifetime and
   authorized current Device/binding. Missing or mismatched facts reject.
3. Run RFC 9449 proof validation against the actual method/canonical URI,
   credential and public key. Restrict algorithms to a server-owned approved
   profile; never accept the client's `alg` as policy. Compare `proof_jkt` with
   both access record and endorsed Device binding.
4. Check server nonce, clock window and atomically reserve replay identity.
   Proposal: accept `now-60s <= iat <= now+5s`; integer timestamps only. Server
   uses a trusted clock; drift or unavailable replay storage denies access.
5. Recheck cancellation and current authorization, then construct a private,
   immutable `VerifiedDeviceRequest` containing tenant/actor/session/device,
   proof thumbprint and binding generation, canonical procedure, validation
   time, access expiry and replay reservation identity. No raw token/JWT/header.
6. Session/command adapters require this context and matching credential record;
   ordinary callers cannot fabricate it through public struct fields or headers.
   Business handlers see only authorized identity, not proof internals. Existing
   `Principal` construction stays private. Missing proof context rejects.

A request-scoped context is not a durable authorization fact. S2 must define the
Session/Device/binding lock and revoke ordering before a message transaction can
commit. Cancellation before reservation performs no write; after reservation it
may burn that replay identity but must not invoke a business handler. Cancellation
after an eventual domain commit cannot undo it; retry uses command idempotency.

## Nonces, replay, budgets and errors

Proposed nonce record: random 256-bit opaque value, fixed issuer/resource,
credential-record identity, `proof_jkt`, issued/expiry times. Keep server custody,
60-second lifetime, at most two live nonces per binding; reject on capacity rather
than evict unexpired proof reservations. A nonce may serve concurrent requests;
the unique proof `jti` is consumed atomically across all replicas for that binding.
Reserve `(issuer, proof_jkt, jti)` through at least `iat+65s` (maximum accepted
expiry plus 5 seconds cleanup margin); no accepted proof becomes replayable after
cache restart or failover. A reservation store outage is `unavailable`.

Issue a nonce only after token record and cryptographic binding checks succeed.
A missing/expired nonce yields a redacted unauthenticated response with the RFC
resource challenge `WWW-Authenticate: DPoP error="use_dpop_nonce"` and
`DPoP-Nonce`; no ordinary handler runs. Fresh valid nonce plus fresh proof may
retry the same logical command. Clients allow one automatic challenge retry per
RPC; further challenge loops stop. Apply configured per-credential and global
challenge/replay capacity limits before allocating records. Security/Platform
must approve actual deployment capacities and distributed atomicity (D3).

Proof rejection/replay maps to `VerificationRejected`; credential expiry to
`VerificationExpired`; revoked Session/Device to `VerificationRevoked`.
The existing [error model](error-model.md) and interceptor map these to stable
unauthenticated messages. Storage failures map to unavailable; preserve canceled
and deadline-exceeded priority. The nonce challenge is new transport behavior,
not an existing ErrorCode. Never return raw signature/store errors, IDs, public
keys, tokens, nonce values or claims in logs/errors/telemetry. The dedicated nonce
response header is the only challenge-value output and must be excluded from logs.

## Enrollment-only route and state

Propose a separate service, **EnrollmentBootstrapService**, after OIDC completion.
It does not use an ordinary Principal or exempt existing DeviceService calls.
Proposed login broker boundary (separate from the bootstrap allowlist):
`POST /auth/native/start` accepts a candidate public proof JWK, identity-key digest,
local S256 PKCE challenge and configured client/redirect identifier. Core freezes
these in a random one-use login transaction before opening the system browser.
Core's enterprise OIDC transaction separately stores upstream state, nonce and
PKCE verifier. The upstream callback validates that transaction and gives the
native app only a short-lived one-use result code bound to the frozen record;
no bootstrap or ordinary access token appears in a redirect URL.
`POST /auth/native/complete` takes that result code and local PKCE verifier plus
a token-endpoint DPoP proof (no access token exists yet, so no `ath`). Verify the
frozen key, local challenge, trusted OIDC result and proof before consuming the
result code atomically and issuing bootstrap access. Never accept a replacement
JWK or identity digest at completion. These routes mint no ordinary Principal.

Proposal: start/result records expire in 5 minutes; fixed configured redirects,
rate-limited starts, no arbitrary issuer/redirect discovery. Nonce challenges on
completion bind to the login record and frozen proof key; no access-record lookup
is possible there. Start itself carries no identity authority. Expired/consumed
result code or failed proof cannot create bootstrap state. D2 must review this
broker choice, public error mapping and native-link hijack fixtures before it is
implemented. The existing callback alone is not presenting-device proof.

Server-owned bootstrap access record: issuer, purpose=`enrollment-only`, audience
exact bootstrap service, tenant/member from trusted OIDC mapping, login transaction
ID, enrollment ID, candidate proof thumbprint, candidate identity key digest,
expiry and consumed state. Proposal: 5-minute non-refreshable access, only one
candidate enrollment per login transaction. Identity key material is public but
must not be logged. `EnrollmentContext` is a private distinct type with no
conversion into Principal; validate proof, nonce, cancellation and replay using
the preceding rules against bootstrap access rather than an ordinary access record.
Pending Device state is permitted only at this dedicated boundary.

Proposed exact allowlist, all POST, no caller tenant/actor fields:

| Procedure suffix after `/threadline.identity.v1.EnrollmentBootstrapService/` | Request / result | Permitted effect |
| --- | --- | --- |
| `BeginEnrollment` | Existing enrollment ID; display name/platform/protection/profile list; identity public key; proof key from verified header | Bind immutable candidate once; return own pending Device/enrollment only |
| `GetEnrollmentStatus` | Own enrollment ID | Read pending/authorized/rejected status; no directory/key-log listing |
| `CompleteEnrollment` | Own enrollment ID, no new keys | Once endorsed and log-verified, atomically consume bootstrap and mint ordinary access bound to the approved Device/proof key |

D2 must specify issuance before BeginEnrollment: login completion freezes candidate
identity digest and proof thumbprint in the server record. Begin rejects any key
substitution. Identical Begin with fresh proof returns the same pending result;
changed immutable fields reject without creating another Device. Proof replay is
rejected even for identical body. Neither Begin nor Complete may endorse itself.

First Device requires Device Authority. Later Devices require a currently
AUTHORIZED Device of the same member or the existing hardware-backed administrator
exception, under the [device lifecycle](../../proto/threadline/identity/v1/device.proto).
Endorsement must cover tenant/member, enrollment/device IDs, both identity and
proof-key binding, profiles and generation in the signed authorization/log evidence.
Existing DeviceEnrollment fields do not encode this full signed binding; D2/D4
must supply a reviewed format. OIDC success or unsigned Core metadata cannot
replace it. Broken log continuity, missing endorsement, rejected/revoked Device
or wrong approver prevents Complete. No MLS leaf is added by bootstrap alone.

Complete is one-use, unlike status reads. Commit consumes bootstrap and creates
one session atomically; a lost response requires fresh OIDC login and proof of the
already endorsed key, not replaying Complete or reviving consumed bootstrap.
Expiry before Complete also requires fresh login; resume only the same verified
pending binding, never a caller-selected member. Cleanup/retention is S2 work.
All other procedures, including existing EnrollDevice, Message, Sync, Capability,
Artifact, directory and ordinary session calls, reject enrollment-only access
before domain reads/writes. Existing authorized-device EnrollDevice behavior must
be reviewed alongside the additive bootstrap service, not silently repurposed.

## Native clients, streams and reconnect

Desktop locald/native host and iOS/Android native adapters own proof keys through
platform secure storage and a bounded signer API. UI, Agent runtime and connector
never receive token/key handles or a general signing oracle. Signing input comes
from an allowlisted transport request, not model/tool-supplied bytes. Hardware
protection claims must match actual capability; DPoP itself is not attestation.
Platform algorithm/key-store compatibility remains D1, including Linux software
storage policy; an unsupported platform cannot silently fall back to Bearer.

Offline compose may keep local encrypted pending commands; it cannot pre-generate
proofs or assume old authorization remains valid. Reconnect refreshes credentials
as needed and obtains a fresh proof/nonce for every network request. Keep logical
command IDs stable while changing proof `jti`; transport replay and message dedup
are distinct. Key loss requires new enrollment, not copying another device's key.

This first profile authorizes unary RPC only. Do not admit protected WSS or
streaming RPC with an entry proof and then trust all later frames. A separate
stream contract must bind handshake to connection identity, enforce expiry/revocation
and per-command authorization, and define reconnect. Until implemented, deny those
transports under this profile; it cannot yet deliver realtime product readiness.
Browser UI cannot inherit native key custody; browser support needs its own reviewed
credential/key boundary. Agent/Service authentication is outside this Human profile.

## Acceptance fixtures (proposed; execution NOT RUN)

Every row uses synthetic credentials, a deterministic clock and an isolated store.
`Reject` means redacted unauthenticated/VerificationRejected; `Unavailable` and
`Canceled` use the mapping above. Except where stated, no domain handler, Device,
Session, event, Outbox write or credential mint is permitted on failure.

| Case | Trusted fixture input | Expected result / forbidden effects | Successor owner |
| --- | --- | --- | --- |
| Valid ordinary request | Active member/session/device, endorsed K1, matching audience, fresh proof/nonce | Verified identity only; handler still performs domain authorization | Core proof adapter |
| Stolen bearer retaining device ID | Token bound K1; attacker signs K2 or omits proof | Reject; no Principal | Core proof adapter |
| Substitute device or key | Same token, different Device record/JWK or binding generation | Reject; no implicit registration | Core + S2 |
| Replay/concurrent replicas | Same accepted `(issuer,K1,jti)` on two replicas | One reservation succeeds; other Reject; no second handler | S2 + Core |
| Expired proof | Clock at iat+61 seconds | Reject; no handler; fresh proof needed | Core |
| Expired Session | Fresh proof, expired access | Expired; no challenge that extends access | Core |
| Wrong tenant | Record belongs T1, authority resolves T2 | Reject; no cross-tenant domain lookup | Core + S2 |
| Wrong audience | Core token presented to bootstrap, or reverse | Reject before endpoint handler | Transport/Core |
| Wrong method/path | Proof for one POST method, another procedure or GET | Reject; no redirect/rewrite workaround | Transport |
| Cancellation | Canceled before lookup / after replay reservation | Exact canceled category; zero lookup initially / reservation may remain, no handler | Core |
| Missing nonce | Valid token/key proof, no nonce | Challenge only; no business effects | Transport/Core |
| Store outage/restart | Replay state unavailable or unrecoverable | Unavailable; no accept-on-empty-cache | S2/Platform |
| Stolen login result | Frozen K1/PKCE challenge; stolen result code with K2 or wrong verifier | Reject completion; no bootstrap token | Login broker/Core |
| First enrollment | Trusted login-bound K1/identity, no Device yet | Begin pending only; Complete denied until Device Authority evidence | Bootstrap + Authority |
| Duplicate enrollment | Fresh proofs, same enrollment and immutable inputs | One pending Device; changed identity/key rejects | Bootstrap + S2 |
| Pending ordinary access | Enrollment token at Message/Sync/Capability/Artifact/ListDevices | Reject; no ordinary reads or Principal | Transport |
| Wrong endorsement/log | Different member approver or log gap | Reject Complete; no Session or authorization transition | Authority/Core |
| Complete replay/lost reply | Already-consumed bootstrap | Reject; no second mint; fresh login recovery | Bootstrap + S2 |
| Revoked Device | Fresh proof but revoked Device/current binding | Revoked; no Session resurrection or successor Epoch access | Core/S2/Crypto |
| Reconnect | Same command ID, fresh valid proof versus reused proof | Fresh request may enter command dedup; reused proof rejects | Client/Core |
| Stream attempt | Valid ordinary proof on WSS/stream route | Deny until stream contract exists; no long-lived authenticated stream | Transport |
| Body change | Proof valid but altered candidate key/body | TLS protects hop; bootstrap immutable-record check rejects key substitution; DPoP alone claims no body integrity | Platform/Bootstrap |

Boundary fixtures also cover exact time edges (-60,+5 seconds), size limit and
limit+1, duplicate headers/JSON keys, algorithm downgrade, arbitrary forwarding
headers and challenge retry exhaustion. Capacity failure must never evict live
replay evidence to admit another request.

## Decisions and small successor tasks

| Decision | Responsible owner / evidence to accept | Blocks |
| --- | --- | --- |
| D1 Proof profile and algorithms | Security + Client: approved JOSE library, cross-platform key custody and URI/signature vectors; DPoP versus mTLS alternative reviewed | Production proof validation and client signer |
| D2 Login/endorsement binding | Contracts + Security/Device Authority: login-to-key binding, signed endorsement format including both keys, authority trust roots and lost-response flow | Bootstrap issuance/Complete and Device binding acceptance |
| D3 Budgets and replay consistency | Security + Platform/S2: clock/capacity values, atomic shared replay store, failover, nonce expiry and load evidence | Production proof acceptance |
| D4 Wire/transport integration | Contracts/Integration: additive service/messages, privacy/error mapping, existing service comment reconciliation, DPoP auth/challenge, SDK fixtures | Any bootstrap or DPoP deployment |
| D5 Credential and revoke transactions | Core/S2: token verifier format, Session/Device/binding lock ordering, refresh/key rotation and Complete atomicity | Production Session verifier and message authorization |

Next bounded tasks (each requires a claimed Issue; none created implicitly here):

- S2, Contracts, 1–2 days: session/device/binding/nonce/replay storage and lock
  schedule document, exact concurrent revoke/send/Complete outcomes. No migration.
- S1-I, Integration, 1–2 days after D1/D2/D4 review: additive Proto/SDK and wire
  fixtures, reserve names/field numbers then. Run `node proto/tools/verify-contracts.mjs`
  and the pinned repository codegen/compatibility checks in its Issue.
- S1-T, Core transport, 1–2 days after D1/D3/D4: proof verifier and private context
  only; fixtures above, planned `cd services && go test -race ./internal/deviceproof`.
- S1-B, Core, 1–2 days after D2/D4/D5: pending-enrollment Begin/status only;
  planned `cd services && go test -race ./core/internal/enrollmentbootstrap`.
  Complete/mint and Authority verification require separate bounded tasks.
- S1-N, one client platform at a time, 1–2 days after D1/D4: native signer/transport
  adapter with fake transport; acceptance uses that platform's pinned test command
  defined before claim. No cross-platform claim from one implementation.

The proposed Go packages and implementation tests do not exist at this baseline;
no pass is claimed for them. This task verifies only `git diff --check`, local
Markdown links and `node proto/tools/verify-contracts.mjs`. Contract review permits
further drafts; it cannot approve security decisions or unlock a production verifier.
