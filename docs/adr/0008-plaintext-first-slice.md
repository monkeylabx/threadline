---
status: proposed
date: 2026-10-18
---

# ADR-0008: Deliver a plaintext Desktop loop as the first integrated slice

The first integrated vertical slice of Threadline Private Enterprise v1.0 will be a **plaintext Desktop loop**: two Desktop clients exchange messages through the single-region dev stack with offline Outbox, reconnect replay and idempotent retry; one message becomes a Task; local `agentd` runs one turn against a synthetic model endpoint; the approval sheet returns to the conversation and the Artifact returns to the Task Thread. E2EE, enterprise recovery, real-device Mobile, Helm/offline deployment, Model Control routing and production identity move to later slices. The v1 product promise in [Scope §2](../acceptance/scope.md) is unchanged; this ADR changes delivery order only. Decision record and work breakdown: [#252](https://github.com/monkeylabx/threadline/issues/252).

## Context

As of main `1f20a80` (2026-10-18) the repository holds roughly 14k Go lines of Core persistence, 11k of Worker relay modules, an ACP client and Windows containment tests in `agentd`, SQLCipher storage and a simulator-verified FFI facade, and an HTML prototype. None of the seven steps in Scope §7 has a runnable product path: `services/core`, `services/worker`, `services/realtime` and `crates/locald` are empty processes and `apps/desktop` is a 27-line shell. M0/G0 has been HOLD since 2026-08-21 on real-device evidence, ADR-0003/0004 acceptance and a production Crypto Provider. The original M2 date (2026-11-13) cannot be met, and the 2937 person-day plan assumed twelve engineers rather than one maintainer driving agents.

Three options were weighed with the Product Owner:

- **A. Plaintext-first slice** (chosen): prove the product loop end to end on Desktop without encryption, then re-enter E2EE on a working path.
- **B. Keep full scope, re-plan the calendar**: at observed throughput of about one 1–2 day task per day, the calendar multiplies three to four times, and the human gates (real device, independent crypto review, pentest, UAT) remain.
- **C. Continue the Runtime containment line**: roughly ten Windows Job Object cases remain NOT RUN; immediately claimable, but the product path stays empty.

## Decision

1. The first integrated slice is Slice A as defined in [Scope §7.1](../acceptance/scope.md). It is complete when the end-to-end script in A13 (#260) passes against the Compose dev stack in CI.
2. Slice A stores the message Envelope payload as opaque client-submitted bytes. The server gains no plaintext-aware feature (search, DLP, summaries). This keeps the E2EE boundary of [ADR-0003](./0003-group-e2ee-recovery.md) re-enterable without a schema change.
3. Identity in Slice A is a dev-only static session verifier implementing the existing `rpcmiddleware.SessionVerifier` seam, marked NOT PRODUCTION in code, configuration and documentation. The production path remains the #192/#200 contracts.
4. Model access in Slice A uses a discoverable routing policy file pointing at a fixed dev endpoint. Workflow code contains no model names, matching the existing routing rule.
5. ADR-0003 and ADR-0004 stay `proposed`; their acceptance is re-timed, not withdrawn. The milestone table in `docs/delivery-plan.md` marks M2 as superseded and leaves later dates pending re-plan.

## Consequences

- Scope §10 is triggered by the change in E2EE delivery order. This ADR, Scope §7.1 and the delivery-plan addendum are that explicit review; no silent downgrade to a server-plaintext product occurs because the server never interprets payload bytes.
- Work proceeds as fifteen 0.5–2 day tasks (A0–A14) across architecture, core, realtime-worker, client-core, desktop, contracts, runtime and quality, with six tasks able to start in parallel. Observed throughput is a measurement, not a schedule commitment.
- The Windows containment evidence in `docs/architecture/worker-containment-profile.md` remains NOT RUN and is not required for Slice A; Slice A's `agentd` work (A9–A11) runs on macOS first, following the #210 probe.
- Anything demonstrated in Slice A is a dev-stack demonstration. It does not change M0/G0 HOLD, #24/#41 NOT RUN, #148 needs-info, or any production admission.

## Closing condition

Move this ADR to `accepted` when A13 passes in CI. Move it to `superseded` when the E2EE slice that follows Slice A is planned and its ADR records how the opaque-payload path becomes the encrypted path.
