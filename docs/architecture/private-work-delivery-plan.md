# Private Work and Artifact Publication delivery boundaries

Status: Draft for #195, 2026-09-07. Base:
`66a1e75325f05f8ef6c53c2e3f88f185567ebbab` (includes PR #194 / V33).
This is an implementation-planning and scope-review packet. It does not accept
ADR-0005, amend Frozen Scope, introduce wire fields or implement Private Work.
All PW production scenarios remain NOT RUN. M0/G0 remains HOLD.

## Product meaning and existing contracts

Use [CONTEXT.md](../../CONTEXT.md)'s existing language:

| Concept | Responsibility / visibility |
| --- | --- |
| Private Work | A member's personal work with an Agent. Its conversation, drafts and making process are not shared with a team by default; a source conversation is optional |
| Shared Task | A collaboration unit with a parent Channel/DM; its Task Thread inherits that parent's visibility |
| Artifact version | A selected immutable result revision proposed for delivery; version identity and lineage still require a contract |
| Artifact Publication | The author's explicit delivery of a selected version and allowed accompanying information to a destination conversation; not session sharing |

[ADR-0005](../adr/0005-private-work-publication-boundary.md) preserves these
boundaries. [Task Proto](../../proto/threadline/task/v1/task.proto) requires a
parent conversation; [Run and Artifact](../../proto/threadline/task/v1/run.proto)
bind existing artifacts to runs and those runs to tasks. There is no standalone
Private Work entity or Publication command/version/recipient contract today.
[Capability](../../proto/threadline/capability/v1/capability.proto) is scoped
authority, not a generic private-session sharing permission. Do not fill in a
fake Channel, hidden Task, empty task_id or UI-only private flag to reuse these
interfaces. A new separate entity or an explicitly versioned extension needs
Contracts/Security review and compatibility fixtures.

The existing shared Task path remains: authorized conversation context → local
Run → necessary approvals → Artifact return. A member can deliberately choose
that path. Private Work must never silently become shared because the Runtime
is offline or because a channel was referenced. Mobile can control work under
its permitted scope but does not execute long-running local tasks.

## Scope decisions required before implementation

The [frozen scope](../acceptance/scope.md) and existing AC-001–AC-012 remain
unchanged. V33 confirms desktop interaction; it does not settle the following
storage/security choices. Owner entries name roles, not signatures.

| Decision | Options to evaluate / working recommendation | Cost and scope effect | Decision owner |
| --- | --- | --- | --- |
| Private history storage | Device-local encrypted state; or encrypted service synchronization. Evaluate a local-only first slice, visibly device-bound, without promising it as the release scope | Synced option adds per-member multi-device key distribution, private metadata and server sync services; local-only loses history with device loss unless separately recoverable | Product + Client-core + Security |
| Device recovery and backup | Same-device protected restore; new-device explicitly authorized import; separately reviewed enterprise recovery; or documented no-recovery first slice | No option may clone Device Identity, silently export keys or borrow a team's Channel key. Recovery availability changes user commitments | Product + Security + Client-core |
| Retention and clearing | Define policy for transcripts, drafts, artifacts, indexes/cache and backups separately; choose enforceable TTL and deletion acknowledgement semantics | Offline devices and immutable backups limit deletion claims; do not promise immediate remote erasure | Security + Product + Platform |
| Multi-device history | Explicitly out of first slice, or end-to-end authorized history sharing | Adds conflicts, append ordering, cancellation ownership and revocation tests; never share SQLite files | Product + Client-core + Runtime |
| Publication version and lineage | Immutable published version with new explicit Publication on revision; do not mutate earlier published bytes in place | Requires version identity, idempotency and discoverable supersession/withdrawal semantics | Contracts + Core |
| Audience and source disclosure | Source read authority AND source sharing policy AND target publish authority AND valid cryptographic recipient set | Cross-conversation rewrapping requires independent crypto design; target ACL alone cannot fix overbroad key distribution | Contracts + Core + Crypto + Security |
| Private work administration | Administrative metadata and approved recovery only; no standing private-content browser | Audit/search/support endpoints need separate negative tests; privacy is not a paid feature switch | Security + Product |
| Delivery milestone | Add a separately estimated PW slice after scope approval; retain M2 shared-Task definition | Do not absorb PW into the old 2937-person-day estimate or preserve old dates as a promise | Product + Architecture + Integration |

For each decision, the approver records selected option, rejected alternatives,
user-visible limitations, affected AC/PW rows, owner and revised work packages.
No decision is approved by merging this planning document. Changing retention or
recovery assumptions must be reviewed against ADR-0003 rather than silently
weakening enterprise requirements.

## Proposed publication boundary

The workflow below is an input to the publication contract task, not a new API.

1. The author selects one immutable version, delivery description, destination
   and allowed source disclosures. Build a reviewable manifest; do not serialize
   the complete Private Work or Run as a publication payload.
2. Recheck ownership of the selected result, current source-read and source-share
   policy, target publish authority and valid cryptographic recipient scope.
   Sharing policy applies to derived content as well as copied source links.
   If the policy for a source cannot be established, block publication of material
   derived from it; hiding the link is not a sharing-policy bypass.
3. Produce only the reviewed result bytes and allowed provenance, encrypted for
   the approved audience with a reviewed construction. Copying a private object
   URL or widening its ACL cannot grant correct E2EE access. Key wrapping and
   object scope require a separate Crypto contract, not ad hoc encryption here.
4. Persist the publication's version/target/manifest binding and message handoff
   through a reviewed idempotent commit path. Private creation may precede it,
   but no team-visible projection is produced before authorized commit.
5. The recipient previews and responds to the published version. A revision
   request authorizes no private transcript or workspace access. The author
   revises privately and explicitly publishes a new selected version.

Cancel before commit has no shared side effect. If cancel or network failure
races commit, the UI displays unknown/pending until the same idempotency key
resolves the result; it must not promise “nothing was shared.” A retry binds the
same selected version, target, manifest and operation identity. Changing any of
those is a new reviewed action, not a reuse of a previous approval. Membership,
source expiry or permission change before commit requires a fresh decision.
Already disclosed information cannot be remotely unlearned; future access and
new disclosure are the revocation boundary.

Private offline work can remain local if the chosen storage profile allows it.
Publication while disconnected waits for authoritative permission and audience
checks; it is not shown to recipients speculatively. Runtime/model failure must
not block ordinary IM or transfer Private Work to another member/runtime without
new authority. High-impact tools remain subject to approvals inside private
work, without automatically disclosing the whole private history to approvers.
The exact minimal approval payload and permitted approver set need a contract.

## Data minimization responsibilities

| Surface | Permitted design input | Prohibited automatic disclosure | Owner |
| --- | --- | --- | --- |
| Team timeline and Activity | Committed Publication and explicitly shared Task facts | Private Work existence, title, draft, tool progress, private Run state | Core projection + Runtime |
| Notifications/push/unread | Authorized destination event references; privacy-safe notification text | Private prompts, local directories, private task title or status | Core/Worker + native clients |
| Search | Published content under destination permission; private local scope only if separately authorized | Shared index joins into author's private corpus or inferred private hits/counts | Client-core + Core |
| Audit/diagnostics | Approved minimal actor/action/outcome/policy metadata and non-content correlation | Prompt, code/file contents, path, private transcript, raw credential or reversible sensitive payload | Security + Audit owner |
| Artifact preview/provenance | Selected bytes, version and explicitly permitted source references | Hidden source text, draft variants, private run IDs that become lookup capabilities, source secrets in preview metadata | Core + Client-core + Crypto |
| Model egress | Task-scoped context and approved route on the executing device | Server-side Prompt proxy or automatic entire-channel context | Runtime + Model-control |

Even identifiers/hashes can reveal private activity. A private ID is not a public
capability, and a digest is not permission to index private data. Freeze an exact
field allowlist and access policy per projection before implementing it. Preview
renderers must use sanitized synthetic negative controls so metadata, archive
entries and embedded source references cannot leak unselected material.

## PW acceptance mapping

Source: [PW-01–PW-08](../acceptance/private-work-publication.md). For every row,
use synthetic Alice/Bob, isolated test workspaces and canaries; retain only
redacted outcomes and approved non-content evidence. “Pass” requires executing
the real integrated path, not inspecting HTML or checking only a mocked call.

| PW | Contract / implementation ownership | Independent observation and negative control | Production status |
| --- | --- | --- | --- |
| PW-01 | Private identity/storage + context grants; Client-core/Runtime/Core | Alice creates with no source and with allowed source; Bob lists/searches/subscribes and sees no work, existence count or private activity; remove source access and resolution fails | NOT RUN |
| PW-02 | Publication manifest/commit + audience envelope; Contracts/Core/Crypto | Cancel before commit yields zero shared events/objects/notifications; publish selected revision reveals that revision only; canary Prompt/path/draft absent in every recipient surface | NOT RUN |
| PW-03 | Immutable version/feedback linkage; Core/Desktop | Bob previews v1 and requests change, cannot fetch private run/history; Alice's v2 is not shared until explicit publish; v1 remains distinguishable | NOT RUN |
| PW-04 | Source sharing and target authority/crypto audience; Core/Crypto | Readable-but-not-shareable source, revoked target member, expired source and cross-channel replay all deny new disclosure; existing source history is not unlocked by a publication | NOT RUN |
| PW-05 | Offline/pending/idempotency/cancel; Client-core/Runtime/Core | Stop model/runtime while sending IM; stop network during publication, restart then retry; one logical result or explicit denial, no private-to-shared conversion | NOT RUN |
| PW-06 | Existing Task parent visibility; Contracts/Core/Desktop | Deliberately shared Task remains visible to authorized parent members; it is never labeled private; private route cannot invoke shared Task create as a workaround | NOT RUN |
| PW-07 | V33 desktop layout, keyboard/window/a11y; Desktop/Quality | 104/180 defaults, first rail 48 collapsed, second 112–360, restore, orange dot/end-of-row count and fixed composer; keyboard/200%/screen reader independent from visual approval | NOT RUN for production; historical partial browser checks only |
| PW-08 | Selected retention/recovery/device policy; Client-core/Security/Platform | Delete/expire and restore tests against selected policy, no cloned device credential; admin audit/search/export cannot become private content access | NOT RUN |

## Small successor drafts and sequencing

IDs here are local planning references, not newly claimed Issues. Each row has
one primary owned area; any public schema, migration or manifest changes get a
separate Integration task. Exact new harness commands must be recorded in the
Issue before it becomes ready; missing runners are dependencies, not test passes.

| Draft / effort | Owner and owned path | Input → committed output | Verification / dependencies |
| --- | --- | --- | --- |
| W1 Private identity/lifecycle contract, 1d | Contracts: `docs/contracts/` | Approved storage/scope option → private IDs, ownership, state, retention/recovery interface and failure fixtures | `git diff --check`; PW-01/08 tables; independent of publication implementation |
| W2 Publication manifest/receipt contract, 1–2d | Contracts: `docs/contracts/` | W1 + #192/#193 accepted boundaries → version/target/key identity, cancel/unknown outcome, source/target policy and redacted projection fields | `git diff --check`; PW-02/03/04/05 fixtures; no reuse of shared Task fields |
| W3 Publication crypto audience design, 1–2d | Crypto architecture: `docs/architecture/` | W2 + #191 candidate constraints → recipient/key-wrap design, validation and independent-review packet | `git diff --check`; wrong recipient/profile/source policy matrix; design only, Security review required |
| W4 Private state schema, 1d | Integration: `crates/client-core/` migration surface | W1 approved local-state option → one migration and rollback fixtures | `cargo test -p threadline-client-core --locked`; encrypted reopen, wrong device/key, no private rows in team projection; other storage choice needs re-split |
| W5 Private create/load slice, 1–2d | Client-core: `crates/client-core/` | W4 + existing keyed DB + secure-key input contract → single-writer owner-scoped create/load API | `cargo test -p threadline-client-core --locked`; cross-owner/tenant and crash/reopen; delete/retention separate slice |
| W6 Private execution context slice, 1–2d | Runtime: `services/agentd/` | W1 + existing future Runtime/grant execution foundation → private context resolution without shared projections | `cd services && go test -race ./agentd/...`; explicit source grants, revocation and no team event on tool progress; blocked until Runtime seam exists |
| W7 Publication commit slice, 1–2d | Core: `services/core/internal/publication/` | W2 + reviewed W3 implemented by separate crypto task + #193 message path + separate DB migration → one authorized idempotent handoff | `cd services && go test -race ./core/internal/publication`; cancel/retry/revoke/changed-manifest matrix against real PostgreSQL |
| W8 Private work UI slice, 1–2d | Desktop: `apps/desktop/` | W1/W5 API and contract fake → one create/reopen work view using V33 | Root `npm run test` and a focused desktop runner recorded in the issue; PW-01/07, keyboard and offline states |
| W9 Publish review UI slice, 1–2d | Desktop: `apps/desktop/` after W8 | W2/W7/fake → select version/target/disclosure, pending/unknown/confirmed outcomes | Root `npm run test`; focused UI tests specified at claim; PW-02/03/05, no auto-publish on edit |
| W10 Projection privacy contract, 1d | Contracts: `docs/contracts/` | W1/W2 → exact team search/notification/activity/audit allowlists and denial fixtures | `git diff --check`; cross-surface canary matrix; implementation split by actual projection owner before W7 integration |
| W11 Dual-member negative E2E, 1–2d | Quality: `test/e2e/` | W5/W6/W7/W9 plus W10 owner implementations → PW-01–06/08 executable suite | Pin new runner command before claim; deny Bob/admin private fetch/search/export, inject source/target revocation, verify one committed publication after retry |
| W12 Native parity planning, 0.5–1d | Architecture: `docs/architecture/` | W1/W2 + V33 + #41 limitations → separate iOS/Android controls, offline and key-protection tasks | `git diff --check`; no long-task Runtime on Mobile, no mobile PASS inferred from Desktop |

W4 and W5 are conditional on local-state selection, not a storage decision made
here. W7 depends on encrypted object handling, trusted identity, source policy,
message commit and projection implementations; a two-day commit slice does not
include all of those foundations. W11 executes after them, while unit/contract
privacy tests accompany each earlier slice. W8/W9 do not broaden their ownership
to client-core or invent transport endpoints.

## Scope review and delivery stages

1. Product/Architecture/Security decide storage, backup/retention, source sharing,
   recipient semantics and private approval visibility; Integration records an
   approved change against Frozen Scope and re-estimates work packages. Record
   explicit rejection/defer decisions too. Until then W1–W3/W10/W12 are drafts.
2. Keep the existing minimal IM and shared Task M2 acceptance path. #191 establishes
   evidence needs, #192 trusted identity and #193 reliable message boundaries;
   their drafts do not imply these implementations are already delivered.
3. After the PW scope decision, implement the smallest selected private-state and
   explicit-publication slice with synthetic two-member E2E. Production acceptance
   requires both positive workflow and non-disclosure evidence from the PW table.
4. Extend to mobile, retention/recovery, operational tooling and Pilot only under
   the approved scope. A usable UI, successful functional test or this plan does
   not establish customer adoption or willingness to pay.

## Verification for #195

Run `git diff --check` and local-link checks. Review each PW row against the
source proposal and each concept against Task/Run/Artifact Proto. Ensure all
undecided items remain Proposed/NOT RUN and every successor declares a single
owner, prerequisite, artifact and test method. No runtime, key, payload or
customer data is used by this documentation task. Handoff is recorded in its PR;
no product permission, migration, generated code or lockfile changes are required.
