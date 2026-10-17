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
`RpcMessage` validates envelope shapes only. Parsed JSON is internal data, never
Run or tool authority. Full Session lifecycle controls remain pending.

Every transport error and cancellation of a polled I/O future permanently closes
both halves to new operations. An already in-flight operation can remain pending
until its deadline and can have a partial/uncertain I/O outcome; its result is
discarded after a concurrent failure. Never replay it blindly. This latch neither
terminates a worker nor proves cleanup. Errors and Debug omit worker content;
stderr is outside this transport. See issue #229 for the R1-B contract and the
[ACP transport specification](https://agentclientprotocol.com/protocol/v1/transports).

Remaining gates include authenticated IPC/Run authorization, protected input and
activity lifetime, trusted Run/Session binding, OS containment with descendant cleanup
and crash recovery, and pinned Goose admission under ADR-0006.

## ACP initialization

`acp::Initialization` privately owns the transport halves for exactly one attempt.
It sends a fixed client-owned request ID, protocol version 1 and empty client
capabilities, then requires a matching v1 result within one total send/receive
deadline. Errors, unsolicited requests/notices, timeout and cancellation of a
polled attempt permanently prevent reinitialization; success also forbids replay.

The opaque `InitializedV1` receipt proves protocol agreement only. Optional
`agentCapabilities` and `authMethods` may be omitted; supplied containers must
be an object and array respectively. No capability flags, authentication entries,
implementation information or extension metadata are consumed, exposed or granted.
There is no tool dispatch or transport extraction API. This restrictive
pre-initialization profile does not support callbacks. Failure does not kill/reap
a worker. See issue #231 and the
[ACP v1 initialization schema](https://agentclientprotocol.com/protocol/v1/schema#initialize).

## Internal Session creation

After successful initialization, the same `Initialization` owner permits exactly
one `create_session` exchange with a distinct client-owned request ID and empty
MCP list. The trusted caller supplies a native absolute UTF-8 workspace path of
at most 4,096 bytes without NUL; validation grants no file consent or containment.
Invalid local arguments preserve the initialized owner and emit no request.

One total deadline bounds send/receive. Once the exchange is polled, failure,
cancellation or success prevents another Session creation. The owner retains the
exact opaque ID and returns a redacted `CreatedSession` receipt: 1–256 UTF-8 bytes,
no control characters, no normalization. Other response metadata is discarded.
These bounds and the refusal of pre-result callbacks/notices are Threadline
profile constraints. No Session load/resume, tools or product dispatch
is enabled. Process cleanup remains separate. See issue #233 and
[ACP v1 Session setup](https://agentclientprotocol.com/protocol/v1/session-setup).

## Bounded internal text turn

The owner permits one `prompt` using its retained Session, a distinct request ID,
1–65,536 UTF-8 input bytes and one total send/receive deadline. Only same-Session
text chunks are accepted: at most 8,192 bytes each, 65,536 bytes total and 64 updates
(including empty chunks). At most 65 frames are read; JSON escaping still obeys the
transport frame cap. Callbacks, reasoning/tools, other updates and malformed data
fail closed. Once polled, success/failure/cancellation all prevent another turn.
Local argument errors preserve the ready Session and emit no prompt. No partial
text escapes on failure; result Debug omits text. Known typed stop reasons are
engine evidence, never business Run completion/cancellation or cleanup proof.
This synthetic profile enables no real worker, tool grants or observer stream.
See #235 and [ACP v1 prompt turns](https://agentclientprotocol.com/protocol/v1/prompt-turn).

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

## Windows Job resource probe (W0)

The private `windows_job` integration target owns empty unnamed Job references
with non-inheritable handles, kill-on-last-close and an eight-process limit;
both breakaway flags remain disabled. Native queries verify those settings.
Explicit close is terminal/idempotent; failed close retains the reference for an
explicit retry. Synthetic setup/query/close faults are labelled in the tests.
The harness's Drop fallback fails visibly on release error and supplies no proof.

This test-only probe is compiled from `test_support/`; it changes no production
library/binary API and cannot assign or launch a process. It emits no
`SealedAndEmpty`, StopReport or Run transition. The receipt's `windowsJobResources`
field records empty-object testing only; `windowsApi` still describes the earlier
symbol fixture, while containment and Goose remain NOT_RUN.

Creation-time JOB_LIST with suspended launch, descendant cleanup, supervisor
crashes, leaked-handle negative experiments and StopDriver integration remain
separate follow-ups under the [containment profile](../../docs/architecture/worker-containment-profile.md).
