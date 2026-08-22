# Error model

`threadline.type.v1.ErrorDetail` and `threadline.type.v1.ErrorCode` are the
merged transport-independent failure interface. Connect/gRPC transports carry
`ErrorDetail` as structured error details; WSS, FFI, and local connector
adapters map their native failures to the same vocabulary at the boundary.

| Field | Contract |
| --- | --- |
| `code` | Stable append-only `ErrorCode`; unknown values map to a controlled fallback |
| `reason` | Stable non-localized diagnostic reason; clients never parse it for behavior |
| `subject_id` | Optional opaque object identifier, omitted when disclosure is unauthorized |
| `policy_version` | Policy or Crypto Profile version that produced the decision |
| `retry_after` | Present only when retrying the same operation can be safe |

The detail never carries message/file content, Prompt, Token, credentials, key
material, SQL, stack traces, or local paths. Detailed operational diagnostics
stay in authorized telemetry and are correlated outside the public wire
detail.

The contracts workstream owns the shared code registry; domain tasks own their
mapping and retry semantics. New codes are additive. A code cannot later change
meaning, authorization implications, or retry safety inside `v1`; use a new
code instead.

T014 removed the earlier branch-local duplicate error message and rebound the
formal Kotlin compilation smoke to `ErrorDetail`. New transports must consume
this authoritative type instead of adding a parallel envelope.
