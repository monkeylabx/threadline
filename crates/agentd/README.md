# Local worker primitives

The binary remains inert and exits unavailable. Library components below do not
launch a production worker or expose a local IPC listener.

## Worker stop coordination

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

Dropping this coordinator does not perform I/O or prove cleanup; the eventual
process owner must handle exit. See issue #227 for the R1-A scope.

## ACP pipe framing

`acp::stdio` binds already owned async streams to independent read/write halves
with a trusted nonzero deadline. Each UTF-8 JSON-RPC 2.0 frame is bounded to
256 KiB including its terminating LF, on input and during output serialization.
`RpcMessage` validates envelope shapes only. Method payloads, initialization,
protocol version, capabilities, request correlation and Session lifecycle need
a typed ACP client. Parsed JSON is internal data, never Run or tool authority.

Every transport error and cancellation of a polled I/O future permanently closes
both halves to new operations. An already in-flight operation can remain pending
until its deadline and can have a partial/uncertain I/O outcome; its result is
discarded after a concurrent failure. Never replay it blindly. This latch neither
terminates a worker nor proves cleanup. Errors and Debug omit worker content;
stderr is outside this transport. See issue #229 for the R1-B contract and the
[ACP transport specification](https://agentclientprotocol.com/protocol/v1/transports).

Remaining gates include authenticated IPC/Run authorization, protected input and
activity lifetime, the typed ACP client, OS containment with descendant cleanup
and crash recovery, and pinned Goose admission under ADR-0006.

## Verification

```text
cargo test -p threadline-agentd --locked
cargo clippy -p threadline-agentd --all-targets --locked -- -D warnings
cargo fmt --all --check
```

Unit tests use in-memory streams and Tokio's paused clock. The pipe integration
test launches only the checked-in synthetic fixture with pinned Node, clears its
environment, and kills/reaps the direct child. It proves pipe exchange and EOF,
not descendant containment. No tests call models or access user files/network.
