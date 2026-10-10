# Worker stop coordination

`WorkerStop` coordinates shutdown of one already owned worker boundary. Call
`request_stop` to latch the local dispatch fence, then `poll_stop` for one attempt
at revocation, ACP interruption, termination, and independent cleanup observation.
The future execution owner must check the latch before every new dispatch.

Revocation failure does not skip termination. Interruption runs once, including
when it fails; a cancel acknowledgement proves no cleanup. Pending containment
is explicitly polled again. Successful revocation and sealed/empty containment
are retained, so a repeated confirmed stop performs no effects. Termination
failure remains visible even if independent proof subsequently confirms cleanup.
`StopReport` describes local observations, never durable Core `CANCELLED`.

`StopDriver` is a seam for bounded, exclusively owned containment operations.
The only implementation is a deterministic test fake. A real adapter must prove
that the entire container is sealed, empty, and reaped, and cannot admit new
members. Parent exit, a bare PID, an ACP response, and an empty but unsealed
process group cannot supply that proof. Errors are coarse enums with no raw
worker output, paths, prompts, or credentials.

The binary remains inert and exits unavailable. This R1-A slice implements no
process launch, OS sandbox, ACP transport, local IPC, model call, or tool effect.
Subsequent tasks must implement authenticated IPC/Run authorization, protected
input and activity lifetime, bounded ACP transport, and OS process ownership
with deadlines, descendant cleanup, and supervisor-crash recovery. Pinned Goose
admission still requires all gates in ADR-0006. Dropping this coordinator does
not perform I/O or prove cleanup; the eventual process owner must handle exit.

```text
cargo test -p threadline-agentd --locked
cargo clippy -p threadline-agentd --all-targets --locked -- -D warnings
cargo fmt --all --check
```

Tests use synthetic results only: no subprocesses, network, timers, or user data.
See issue #227 for the bounded scope and acceptance criteria.
