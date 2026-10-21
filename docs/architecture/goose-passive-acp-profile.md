# Bounded passive Goose ACP profile

Issue: #326. Runtime-only profile, pinned to Goose 1.52.0, source commit
`302b60806639ea9f0ae8f053f49f8bf0e88b26f4`, executable SHA-256
`b7a38f3ff1e2d48b34b6550f84cf4104f3f7d797429e1231d1255d0501d47702`.
This profile is specified before implementation. It extends the existing single-turn
Rust client; it creates no public interface or production admission.

## Responsibility and boundary

The decoder accepts bounded text with three inert notification shapes for exactly
the session returned by `session/new`. Every other callback, method, session or
update kind fails closed. Only the completed text and typed stop reason escape.
All passive data is validated then discarded: command descriptions, provider or
runtime metadata never become permissions, routing, approvals, tools or Run identity.

Inputs are worker JSON-RPC frames; outputs are the existing `TurnResult`/`TurnError`.
There are no new side effects. Invalid shapes, exhausted budgets, transport errors,
remote errors, timeout and dropped futures poison the attempt; no partial text or
blind retry is permitted. The test seam is hermetic stdio with paused Tokio time.
The change is independently revertible; there is no persisted state or migration.

## Observed shapes and admitted subset

A 2026-10-21 synthetic macOS arm64 run of the hash-checked binary observed:
active-run info, setup usage, available commands, text, usage, cleared active-run
info, usage, then `end_turn`. Setup notifications may be queued after session
creation and consumed during the prompt. Upstream emission sites are
`acp/response_builder.rs::send_session_setup_notifications`,
`acp/server.rs::send_active_run_update` and `send_session_usage_updates` at the pin.

All notifications require `method: "session/update"`, no `id`, object `params`,
exact `sessionId`, and object `update`. Passive update objects permit only the keys
below, plus optional object `_meta` (except info, whose metadata is required):

| `sessionUpdate` | Required fields | Optional fields |
| --- | --- | --- |
| `usage_update` | `used`, `size`: unsigned 64-bit integers | `cost`: object with nonnegative finite numeric `amount`, nonempty `currency` string ≤16 UTF-8 bytes; object `_meta` |
| `available_commands_update` | `availableCommands`: array ≤32 entries | object `_meta` |
| `session_info_update` | `_meta: {"goose":{"activeRunId": string or null}}` | none |

Command entries admit only nonempty string `name` ≤256 bytes and string
`description` ≤4096 bytes, optional `input: {"hint": string ≤1024 bytes}`,
and optional object `_meta`. Unknown fields or malformed nested data are refused.
Info metadata admits only the `goose` object and its `activeRunId`; a string must
be nonempty, ≤256 bytes and contain no control characters. Title/steering updates,
custom Goose methods, thought/tool/plan/mode/config updates remain unsupported.
Usage cost objects admit only `amount`, `currency` and optional object `_meta`;
input objects admit only `hint` and optional object `_meta`.

## Budgets

The existing frame limit remains 262,144 wire bytes including newline. A prompt
accepts at most 64 notifications total (text and passive combined), plus one terminal
response, at most 131,072 aggregate incoming wire bytes including whitespace and
newlines, and at most 16,384 wire bytes per passive notification. Existing text
limits remain 8,192 UTF-8 bytes per chunk and 65,536 in aggregate. Terminal frames
also consume the aggregate byte budget. One caller-supplied nonzero total deadline
covers prompt write and every receive; passive updates never reset it. Transport
per-operation deadlines also remain in force. Limits are local policy, not worker
claims; no accepted usage report can enlarge them.

## Admission limits

Text interoperability does not establish Core completion or owner cleanup.
#258 observed a background child surviving ACP cancel; ordinary group kill is
not `SealedAndEmpty`. macOS containment remains unproven. No real worker may
enter a protected Run on the strength of this profile or its synthetic text result.
Normal CI runs hermetic fixtures only and never downloads or executes Goose.

## Real strict-client evidence and reproduction

The [receipt](goose-passive-acp-receipt.json) records the 2026-10-21 macOS run:
initialize, session creation and the strict text turn succeeded; the exact expected
24-byte synthetic text and `EndTurn` were asserted, with zero advertised model tools.
The [opt-in example](../../crates/agentd/examples/goose_passive_probe.rs) fails the
process if either assertion fails, kills/waits the direct child, and prints only
synthetic match/length evidence. It is a test fixture, not a production launcher.

Reproduction requires #258's **committed** probe support at `a81e835` (PR #325),
which was not yet integrated at this task's base `3fdae33`. Build this branch's
example with `cargo build -p threadline-agentd --example goose_passive_probe --locked`.
Use #258's verified archive, routing-policy and restrictive sandbox instructions.
In a separate Python driver, import `Endpoint`, `load_policy` and `strict_agentd`
from that commit's `crates/agentd/test_support`; call `strict_agentd` inside
`with Endpoint(route) as endpoint`, passing a fresh output root, verified binary,
route, endpoint and the absolute path to this branch's built example. Assert
`result["text_matches"]`, `result["text_bytes"] == 24`,
`result["stop_reason"] == "end_turn"`, a nonempty `endpoint.requests`, and that every
request has an empty `tools` list. No uncommitted #258 state is needed. Full #258
probe validation and containment/admission remain separate integration work.
