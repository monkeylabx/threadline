# Slice A dev Task authored-field profile

Status: proposed contract for [#353](https://github.com/monkeylabx/threadline/issues/353), G4 of [Core Task control](./slice-a-task-control.md), under [#350](https://github.com/monkeylabx/threadline/issues/350). **NOT PRODUCTION.** Documentation and synthetic vectors only; no wire fields, codecs, negotiation, storage or admission are implemented here.

## 1. Responsibility and compatibility decision

This profile defines the representation of Task title/goal, Approval justification and optional decision note. Authorization, profile admission, persistence, transport and client decoding remain separate modules. Inputs are a trusted deployment policy, independently authenticated caller, supported-profile offer and typed authored bytes; outputs are a selected profile binding or explicit failure, then bounded opaque fields. No profile selection creates permission, Runtime identity, a Grant or approval authority. The future test seams are the actual profile-admission interface and pure client codec, not renderer flags or fixture booleans.

| Option | Decision |
| --- | --- |
| Retain genuine `EncryptedField` only | Valid encrypted route, requiring real Group/Epoch/key/provider support. It does not supply the missing dev representation. Production keeps this route and its existing admission gates. |
| Put plaintext in `ciphertext`, invent Group/Epoch, or reuse the Message dev Crypto Profile | Rejected: changes the public cryptographic meaning and permits accidental downgrade. |
| Retype existing fields to an envelope, or move them into a new `oneof` | Rejected: changes existing public shape/presence behavior. A length-delimited wire type alone does not establish compatibility. |
| Add explicit dev-only sibling fields | Selected: a new `DevAuthoredField` type, with mutually exclusive application admission and mandatory profile negotiation. Existing encrypted fields retain names, numbers, types and meaning. |

[ADR-0008](../adr/0008-plaintext-first-slice.md) concerns opaque **Message Envelope** payload delivery. It does not authorize plaintext `EncryptedField`, Task authored-field admission or production fallback. This contract proposes a separate dev representation; it neither supersedes the ADR nor changes its production gates. The profile is independently reversible before wire/persistence implementation; those successors need their own compatibility/rollback evidence. No new domain term or ADR amendment is required for this implementation profile.

The following inventory is exhaustive for this subset. Future Contracts/Integration work allocates unused field numbers and adds the proposed siblings; this document reserves none. Do not move the original fields into a `oneof` or change their JSON names. Original `EncryptedField` members `ciphertext=1`, `e2ee_group_id=2`, `epoch=3`, `crypto_profile=4`, `envelope_version=5` remain unchanged.

| Existing public field(s), unchanged | Proposed additive sibling(s) |
| --- | --- |
| `CreateTaskRequest.title=3`, `goal=4` | `dev_title`, `dev_goal` |
| `Task.title=6`, `goal=7` | `dev_title`, `dev_goal` |
| Gateway `RequestApprovalRequest.justification=5` | `dev_justification` |
| `Approval.justification=7`, `decision_note=13` | `dev_justification`, `dev_decision_note` |
| `DecideApprovalRequest.decision_note=6` | `dev_decision_note` |
| `UpdateTaskRequest.title=2`, `goal=3` | None; Task editing stays `UNIMPLEMENTED`. |

All other `EncryptedField` uses, Message envelopes and signed CapabilityGrant bytes are outside scope. Original fields remain absent in dev requests/responses, including empty-message placeholders. Encrypted peers never receive synthesized empty ciphertext or dev siblings. Old binaries can ignore added fields, so binary compatibility alone is insufficient: an unnegotiated peer must fail before a Task/Approval response or decision, rather than interpret missing old fields as empty text.

## 2. Selected representation and byte limits

`DevAuthoredField` has exactly these semantic members; the wire task defines their tags and explicit scalar presence. All four are required, including an explicitly present empty `opaque_payload` when allowed. No defaulting, duplicate member, unknown member, encryption metadata or alternate body is accepted at the negotiated boundary.

| Member | v1 value / meaning |
| --- | --- |
| `profile_id` | Exact ASCII `tl-slice-a-dev-authored`; an authored encoding identifier, **not a Crypto Profile**. |
| `format_version` | Unsigned 32-bit integer `1`; zero and unknown versions are incompatible. |
| `kind` | `1` Task title, `2` Task goal, `3` Approval justification, `4` Approval decision note; must match the containing field. Zero/unknown kinds are invalid. |
| `opaque_payload` | Exact authored text encoded as strict UTF-8 bytes, without JSON, compression, BOM insertion, crypto framing or a trailing terminator. |

Presence of the outer message distinguishes an omitted note from a present empty note. Title, goal and justification are required. Only the note may be absent; empty title/goal/justification is invalid. No trimming, Unicode normalization, newline conversion, case folding or replacement decoding occurs. A caller that wants different text creates different bytes before first submission. A received BOM or valid control code point is preserved in identity; a view may show it visibly without mutating storage or retry bytes.

| Kind | UTF-8 payload bytes, inclusive | Additional limit |
| --- | --- | --- |
| Task title | 1–256 | Every binary Protobuf wrapper is at most 65,536 bytes; a ProtoJSON wrapper is at most 131,072 received bytes. |
| Task goal | 1–61,440 | [Runtime control §6](./runtime-control-delivery.md) still limits decoded goal + one trigger message to 65,536 combined UTF-8 bytes. |
| Approval justification | 1–8,192 | Same wrapper limit. |
| Decision note | 0–8,192 when present | Absence remains distinct from empty. |

These are project dev budgets, narrower than #347's per-`EncryptedField` ceiling; they do not alter that type's limit. Measure payload octets, not characters, UTF-16 code units, base64 length or compressed length. Enforce the transport-specific wrapper bound before decoding/unbounded allocation, then the same payload/metadata limits for both transports; bounded overall request limits remain in force. The JSON allowance covers base64 overhead, not a larger payload. No truncation or splitting to bypass a limit.

The native authoring codec rejects unpaired UTF-16 surrogates and invalid UTF-8, including overlong forms, surrogate encodings, truncation and values above U+10FFFF. The receiving codec and locald resolver repeat strict decoding. Core/Gateway inspect only required metadata, representation, presence and byte counts; they do **not** decode or normalize the payload. Consequently, a malicious invalid UTF-8 payload can pass Core's opaque checks: the consuming client must reject it atomically, show an incompatible-content state and supply no goal to a model. It must not return an empty string, replacement characters or a partial prompt. Render valid content as inert text, never HTML, a command or an approval-policy instruction.

## 3. Trusted enablement and negotiation

Core's operator-controlled configuration declares `deployment_id`, class (`production` or `dedicated-dev`), authored policy generation and exactly one mode: `encrypted-only` or this dev profile/version. Missing/unknown configuration disables dev. `production` plus any dev enablement is invalid: authored Task operations fail closed until repaired, without disabling ordinary IM. A request header, body flag, support offer, URL query, renderer state or Message Crypto Profile cannot change these facts. A dev server uses isolated synthetic data and a visibly **NOT PRODUCTION** endpoint; its native UI shows that marker whenever these Task/Approval fields are used.

The planned authenticated `NegotiateAuthoredProfile` seam receives 0–8 distinct supported `(profile_id, format_version)` pairs and returns one server-selected binding. An identifier is 1–64 printable non-space ASCII bytes; versions are explicit unsigned 32-bit values. No intersection (including an empty offer) means incompatibility, not a guessed common denominator. An encrypted-only selection must come from the existing trusted allowed Crypto Profile policy, never an arbitrary offered string. The client connects over authenticated TLS to its trusted configured endpoint and checks deployment identity/class against its own native configuration. A production-configured client rejects a dev advertisement, even if it implements the codec. Support offers are compatibility claims, not client attestation or authority.

A binding consists of an opaque server-issued handle (1–128 printable non-space ASCII bytes) and selected deployment, tenant, authenticated Actor/Device plus Human Session ID/generation **or** authenticated Runtime identity/generation, policy generation, profile/version and expiry. Its lifetime is at most 600 seconds and never exceeds the authenticated session/credential lifetime; at Core time `now >= expires_at` it is invalid. It is server-owned session state, not a bearer credential or signed Grant. The native caller carries the handle as transport metadata on subsequent calls; Core validates its ownership and all current facts under the same fact/transaction discipline as the business operation, never as a cached allow. Missing, expired, foreign, wrong-endpoint, stale-Session/Runtime, stale-policy or post-Core-restart handles cannot be used. Handles are volatile; renegotiation after restart/relogin is required. The response exposes no authored content. Gateway relays an authenticated Runtime's binding without substituting a Human Session.

Keep at most one live binding per authenticated deployment/tenant/Actor/Device/Session-or-Runtime-generation tuple. Repeated compatible negotiation returns that binding without extending expiry or invalidating in-flight calls. An incompatible offer leaves it unchanged; expiry or invalidation permits a fresh binding after current checks. Existing authentication/rate limits bound total sessions; binding-store exhaustion fails without evicting another caller's live binding. Never send dev authored bytes before successful negotiation. A new client seeing an old server's absent/`UNIMPLEMENTED` negotiation stops with incompatibility, without trying legacy Task fields.

Negotiation applies before creating/reading a dev Task or Approval, starting/admitting its Run, requesting/deciding Approval, and every cached/list/stream response containing these objects. A decision with an omitted note still requires the Task's profile binding. A Runtime needs the same profile readiness before dispatch admission; a Message-profile offer does not imply this capability. An old client that never negotiates receives an explicit incompatible response. Unrelated IM and authorized metadata-only Run reads/CancelRun remain usable; negotiation cannot disable #348's separately authenticated local stop, revocation delivery or restricted cleanup ports. Those ports return no authored content and confer no new execution authority.

Core pins the Task's representation/profile/version at creation; all its Approvals and present decision notes must match. Required siblings must all be dev or all genuinely encrypted: no per-field mixture, dual representation (even an empty legacy wrapper), automatic transcoding or live mode migration. Dev policy accepts only the selected dev profile, never genuine encrypted fields opportunistically; encrypted-only policy never admits dev. A policy generation change invalidates bindings. Rebinding to the **same** profile permits same-key, byte-identical retry after current authorization; switching encoding does not transform an old command into a new one. If configuration no longer supports the stored profile, return incompatibility without content; an explicit future migration is required. Content-free stop/cleanup remains available as above.

Authenticate and establish current tenant/resource disclosure permission before returning object-specific profile facts; negotiation itself uses only the authenticated caller's deployment/tenant. Profile gates never replace #347/#348 authorization, exact action, owner/lease/fence, approval, retention or reconciliation rules. Ordinary encrypted admission still requires real allowed Group/Epoch/Crypto Profile validation. Fake groups/epochs, a dev marker in `EncryptedField.crypto_profile` or a forged ciphertext body never select this profile. Core's opaque handling cannot by itself prove that arbitrary ciphertext is genuine; existing cryptographic producers/consumers must authenticate it, and no participant may decode it as dev plaintext.

## 4. Exact retry projection

The authored-field subprojection is versioned and independent of Protobuf/JSON serialization. `U32(n)` is four unsigned big-endian octets. `B(x)` is `U32(byte_length(x)) || x`. Domain bytes are ASCII `threadline.authored-field/1` followed by one NUL. `slot` is one octet, values 1–4 from §2.

```text
absent:  domain || slot || 0x00
present: domain || slot || 0x01 || B(profile_id ASCII)
         || U32(format_version) || U32(kind) || B(opaque_payload)
```

Only slot 4 may be absent. All metadata is validated before projecting; unknown fields/versions/kinds are rejected, not canonicalized away. The slot and inner kind must agree. Wrapper scalar presence is required even for an empty payload; an absent required member is not its zero value. Duplicate fields are invalid, including duplicate equal values; future transport adapters must detect them before a generated parser discards presence, duplicates or unknowns. No last-wins parsing defines command identity.

[#355](https://github.com/monkeylabx/threadline/issues/355) must incorporate this subprojection, the paired encrypted/dev representation tag and every other semantic command field into its typed command projection and existing tenant/Actor/Device/RPC or Runtime/Run/owner namespace. This subprojection alone is not a command ID, signature, content hash or external-effect deduplication key. It allocates no encrypted-field canonical format. Bearer tokens, profile-binding handles, transport/correlation IDs and login generations are excluded from command bytes; the **selected profile/version inside each authored wrapper**, exact payload and outer presence are included. Renegotiation cannot silently rewrite them.

Identical same-key bytes recover the existing receipt after current disclosure checks. Same-key changes to even one byte, CRLF/LF, NFC/NFD, kind, profile, version, presence or another command field conflict with a committed command; malformed/incompatible requests are rejected before receipt comparison. Present-empty and absent notes differ. No re-encode-on-retry, regenerated note, fresh operation ID after a timeout or plaintext SHA-256 shortcut. Protected receipts store exact identity or a separately reviewed integrity commitment under #347 retention rules; they expose no low-entropy authored hash.

For future ProtoJSON adapters, `opaque_payload` is the official `bytes` base64 mapping. Accepted standard/URL-safe and padded/unpadded spellings decode to the same exact bytes before this projection; the canonical response spelling is standard padded base64. Never UTF-8-encode the JSON property as raw text: unparsable base64, extra members and duplicate logical keys (including snake/camel aliases) are invalid. This profile rejects explicit JSON `null` for authored outer fields and wrapper members; only omission represents an absent note. An ASCII string that is valid base64 always means its decoded bytes, regardless of author intent. JSON and deterministic Protobuf serialization are never used directly as retry identity. Golden values in [encoding-vectors.json](../../test/fixtures/proto/slice-a-authored-fields/encoding-vectors.json) are synthetic subprojection bytes, not allocated wire frames.

## 5. Errors and data handling

All denials occur before new business writes, events, Grant issuance or model input. Authentication/current disclosure checks precede incompatible-object details; unavailable policy never becomes a permissive default. Mixed representations are invalid shape; a single valid representation rejected by deployment/Task policy is incompatibility.

| Failure | Status / existing detail code |
| --- | --- |
| Missing/invalid authentication | `UNAUTHENTICATED`; no object detail. |
| Foreign binding or current permission denied | `PERMISSION_DENIED / PERMISSION_DENIED`; no foreign object/profile disclosure. |
| No negotiation; expired/stale/missing binding; dev disabled; wrong deployment/mode/profile; genuine-encrypted/dev mismatch | `FAILED_PRECONDITION / CRYPTO_PROFILE_UNSUPPORTED`; renegotiate only when trusted configuration permits. |
| Unknown/zero format version under the known dev profile | `FAILED_PRECONDITION / ENVELOPE_VERSION_UNSUPPORTED`; no fallback. |
| Missing members/required field, empty required text, explicit authored JSON null, wrong/unknown kind, mixed/dual/duplicate/unknown member, forbidden encryption metadata | `INVALID_ARGUMENT`; no invented domain code. |
| Payload/wrapper/combined context over limit | `RESOURCE_EXHAUSTED / PAYLOAD_TOO_LARGE`; no partial result. |
| Local strict UTF-8 decode failure | Local `INVALID_ARGUMENT` result; no ciphertext-integrity error or replacement text. |
| Valid same-key command differs from committed identity | `ALREADY_EXISTS / IDEMPOTENCY_CONFLICT`. |
| Current policy/fact source unavailable | `UNAVAILABLE`; no cached allow. |
| Binding capacity/rate limit | `RESOURCE_EXHAUSTED / RATE_LIMITED`; no live-binding eviction. |
| Unsupported Task editing | `UNIMPLEMENTED`, after method authentication. |

Dev payloads are plaintext C3 **even while treated opaquely by Core/Gateway**. They transit and persist in the dev Core resource/receipt stores; TLS and opaque APIs do not provide E2EE or confidentiality from that server/operator. Use only the dedicated dev profile and existing C3 access/retention controls. Gateway is a bounded relay, never a content cache. No payload, snippet, base64/hex copy, low-entropy digest or decoded text belongs in logs, error details, metrics, traces, audit, indexes, search/filter keys, notifications or control outbox. Authorized resource reads relay unchanged opaque fields; control notifications carry only identities/versions. Decoding occurs in the native client/locald for the bounded Task view or authorized initial goal; no file/history widening or direct agentd IM database access.

## 6. Follow-on ownership and verification

[#388](https://github.com/monkeylabx/threadline/issues/388) adds the wrapper/siblings and negotiation wire, transport binding, strict presence/unknown/duplicate validation requirements and generated-code provenance, serialized before #355 touches the same messages. It depends on #353 and #335, not #355; #355 then composes the full command projection. [#389](https://github.com/monkeylabx/threadline/issues/389) owns Core trusted configuration, volatile binding lifecycle and opaque checks through fact/store ports, without Task business writes or process wiring. [#390](https://github.com/monkeylabx/threadline/issues/390) owns the Client-core pure codec and this subprojection; #370 consumes it for initial context. These bounded tasks belong under #350; downstream Task/Approval handlers, storage and wiring consume them through their existing map reservations. Do not duplicate Desktop work or reuse another worktree's uncommitted adapters.

[scenarios.json](../../test/fixtures/proto/slice-a-authored-fields/scenarios.json) is an expectation table, **not an executable profile gate, codec, wire verifier or security proof**. Each case overlays its `given` on declared synthetic defaults; `when` names the responsible future seam. Size descriptors repeat exact hex octets, not a runtime shortcut. Denial expectations require no new writes/events/Grants/model input; local decode rejection may follow a prior successful opaque store and cannot claim that stored content was rolled back.

Run `node scripts/proto.mjs check`, parse both JSON files and `git diff --check`. Independently inspect the reference octets, UTF-8 lengths, unique case IDs, boundaries and registered error vocabulary; manually trace both compatibility directions, rolling old-client/server failure, opaque handling and retry identity. Parsing/checking reference data does not execute the proposed policy. Later wire/Core/client tests must exercise real modules and mutation/failure cases; no legacy aggregate-verifier growth while #335 is open. Existing #347/#348 tests do not certify this new profile. #350/#259 remain blocked on implementation and real-boundary acceptance; this document grants no production admission.

Primary format sources: [Proto3 field evolution](https://protobuf.dev/programming-guides/proto3/), [ProtoJSON bytes mapping](https://protobuf.dev/programming-guides/json/), [non-canonical Proto serialization](https://protobuf.dev/programming-guides/serialization-not-canonical/), and [RFC 3629 UTF-8 §3–4](https://www.rfc-editor.org/rfc/rfc3629.html#section-3). They support the compatibility/format constraints; deployment policy, representation choice, caps and retry projection above are project decisions.
