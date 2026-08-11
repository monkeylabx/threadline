# Error model

`threadline.common.v1.ErrorEnvelope` is the transport-independent failure interface. Connect, gRPC, WSS, FFI, and local connector adapters map their native failures to this shape at the boundary.

| Field | Contract |
| --- | --- |
| `domain` | Stable bounded namespace owned by a domain, for example `identity` or `sync` |
| `code` | Stable machine-readable code within the domain; unknown codes map to a controlled fallback |
| `retryable` | True only when repeating the operation can be safe; it is not a UI instruction by itself |
| `user_message_key` | Safe localization key, never raw upstream text or user-visible prose |
| `trace_id` | Optional opaque correlation identifier |

The envelope never carries message/file content, Prompt, Token, credentials, key material, SQL, stack traces, or local paths. Detailed operational diagnostics stay in authorized telemetry keyed by `trace_id`.

Domain tasks own their code registry and retry semantics. New codes are additive. A code cannot later change meaning, authorization implications, or retry safety inside `v1`; use a new code instead.
