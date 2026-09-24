# Agent runtime alternatives and the build boundary

Research date: 2026-09-17. This note distinguishes reusable execution harnesses from orchestration libraries and OS isolation backends. Candidate facts below come from official repositories and documentation inspected on that date; they are not results of runtime testing or a production security audit. Links to `main` and live documentation can change.

## Additional candidates

### Deep Agents: reusable general-purpose harness

[Deep Agents](https://github.com/langchain-ai/deepagents) is MIT-licensed and provides a Python harness, with a separate JavaScript/TypeScript implementation. It includes planning, context compaction, subagents, skills, file tools, and model-provider integration. It is built on LangChain's agent loop and LangGraph execution; it is substantially more assembled than a bare model client. The vendor describes model-agnostic tool-calling support, but this does not demonstrate equal task quality across models. [Repository and architecture](https://github.com/langchain-ai/deepagents/blob/main/libs/ARCHITECTURE.md).

The filesystem backend is an explicit seam. Default files live in thread-scoped graph state; persistent stores and local filesystems are different options. `CompositeBackend` routes paths to different backends. `LocalShellBackend` explicitly executes on the host without isolation. A configured sandbox backend can supply file operations and shell execution inside an isolated environment, but its guarantees depend on that backend. [Backend documentation](https://docs.langchain.com/oss/python/deepagents/backends), [sandbox contract](https://docs.langchain.com/oss/python/deepagents/sandboxes).

Human approval uses `interrupt_on` and requires a checkpointer; resume uses the same thread ID. Approval, rejection, and argument editing are available. Choosing an in-memory checkpointer does not make approval state survive process death. Threadline must bind approval to its own principal, grant and exact requested action; framework thread IDs are not authorization identities. [Human-in-the-loop documentation](https://docs.langchain.com/oss/python/deepagents/human-in-the-loop).

**Threadline implication (inference):** credible alternative for general enterprise work when packaged as a Python or JS worker behind a Rust supervisor. A custom filesystem/tool backend offers a useful Connector integration point. It still needs desktop packaging, credential isolation, sandbox selection, resource limits, durable storage configuration and end-to-end cancellation validation. Its own repository instructs developers to enforce limits at the tool/sandbox boundary. It is not evidence that installing the library protects the desktop.

### Rig: Rust components for a deliberately owned runtime

[Rig](https://github.com/0xPlaygrounds/rig) is an MIT Rust library with provider-neutral model/tool contracts and a classic Agent runtime. Current repository documentation separates `rig-core` provider/backend contracts from `rig-agent` orchestration. The latter includes streaming, hooks, a live tool registry and a serializable `AgentRun` state machine. The README explicitly warns of breaking changes. This is a richer starting point than hand-writing HTTP requests, but it is not a turnkey desktop Agent service. [Official README](https://github.com/0xPlaygrounds/rig/blob/main/README.md).

Its open [interactive coding-agent roadmap](https://github.com/0xPlaygrounds/rig/issues/2118) distinguishes existing loop/hooks/memory/tool primitives from work needed to connect steering, cancellation, durable sessions and host execution contracts. An open umbrella issue does not prove every child feature is missing; an implementation must check the selected release and relevant merged changes. Do not convert roadmap intentions into completed safety guarantees.

**Threadline implication (inference):** a plausible library if Threadline chooses to own a small Rust runtime, particularly for typed enterprise tools. It does not eliminate ownership of the durable run log, side-effect idempotency, approval recovery, process supervision or OS isolation. Rust's memory-safety properties do not constrain a permitted shell, network client or filesystem API. The evidence reviewed here does not establish a built-in cross-platform desktop sandbox.

### Microsoft Agent Framework: workflow/service architecture candidate

[Microsoft Agent Framework](https://github.com/microsoft/agent-framework) is MIT-licensed. Its main repository covers Python and .NET and points to a separate Go SDK. It supplies provider integration, middleware, graph workflows, streaming, human-in-the-loop and observability. It is appropriate to compare when the requirement is enterprise orchestration or service-hosted agents; it is not a Rust-native packaged local harness. [Official feature list](https://github.com/microsoft/agent-framework#key-features).

Workflow checkpoints can restore execution state, using configured storage. The documentation differentiates in-memory, local-file and Cosmos-backed implementations. This is useful durability infrastructure; it does not itself undo external side effects or prove that child processes terminate when a workflow stops. [Checkpoint documentation](https://learn.microsoft.com/en-us/agent-framework/workflows/checkpoints).

**Threadline implication (inference):** useful if existing .NET/Microsoft service infrastructure drives the decision. These workflow capabilities alone are not evidence of workstation filesystem/network isolation. For the Rust desktop goal, wrapping another runtime and supplying a separate execution boundary still creates integration work; adopting it solely for its enterprise branding would not resolve the local execution problem.

## Pi plus a separate execution boundary

Pi offers an embeddable session API and a JSONL RPC interface, including streaming, cancellation and session management. Its lower-level agent package is also reusable. This is a credible option when Threadline wants an existing multi-provider loop rather than owning one. The SDK's default resource discovery includes local configuration and extensions; a managed integration must explicitly control these inputs. [SDK](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/sdk.md), [RPC](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/rpc.md).

Pi explicitly does not impose a built-in sandbox: extensions run with the process's privileges. That is an integration responsibility, not a reason to assume a new sandbox must be written. [Security policy](https://github.com/earendil-works/pi/blob/main/SECURITY.md).

Its official containerization guide documents whole-process Docker/OpenShell isolation and a Gondolin extension that delegates built-in tools to a Linux microVM. In the latter arrangement, Pi extensions remain on the host unless separately delegated. A mounted workspace can still be changed by sandboxed tools. These patterns have materially different trust boundaries. [Integration guide](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/containerization.md).

[NVIDIA OpenShell](https://github.com/NVIDIA/OpenShell) is Apache-2.0 execution infrastructure, not another model loop. It provides policy-controlled filesystem, process, network and provider access. The documented prerequisites include Linux, Apple Silicon macOS, or experimental Windows/WSL2, plus a container or virtualization runtime. It is a relevant reuse candidate for arbitrary-code execution, but increases desktop installation and support requirements. Threadline must validate the chosen release, compute driver and credential path; the existence of policy controls is not an end-to-end security certification.

## Build boundary: a small Rust runtime versus a coding harness

**Recommendation:** own Threadline's permission, approval and run-lifecycle contracts. For an initial product limited to typed enterprise tools, prototype a small Rust runtime using reusable provider/tool components such as Rig. If arbitrary shell, package installation and coding are launch requirements, compare an existing harness plus isolation backend instead of building that entire stack. These are conditional product choices, not a claim that the latest library inspected is the universal winner.

The existing [Codex evaluation](2026-09-agent-runtime-selection.md) remains relevant to the coding path. This note extends the candidate set; it neither changes the approved architecture nor demonstrates production parity among candidates.

### Proposed local architecture

```text
Threadline UI / remote control plane
                |
       authenticated Run API
                |
     Rust local supervisor (agentd)
       |                  |
       |           bounded Agent worker
       |           (own loop OR adapter)
       |                  |
       +-- policy / approval broker
                |
       authorized local Connector
                |
       scoped files and product tools

Optional coding worker -> sandbox / VM -> scoped workspace export
                         (no direct IM database or host credentials)
```

This is a proposed process boundary, not an implementation description. Rust code can be shared between components without putting untrusted tool execution inside the privileged desktop UI process. Web and mobile can control runs, but local file operations require a running, authorized local Connector. Model unavailability must not prevent IM operation.

For sandbox workspaces, the Connector exports only authorized data and mediates write-back. Do not bypass the product's Connector rule by granting a harness the user's entire filesystem. A writable host mount also requires explicit authorization; a sandbox is not a substitute for it.

### What Threadline would implement

| Component | Bounded responsibility |
| --- | --- |
| Provider adapter | Reuse streaming/model/tool protocols; resolve models through routing policy. |
| Agent loop | Validate typed tool requests; enforce step, token, time and output budgets; support interruption. |
| Run journal | Persist transitions and operation IDs; recover pending approvals; distinguish uncertain external outcomes. |
| Policy broker | Authorize principal, device, run, tool and exact arguments; bind approvals to scope and expiry. |
| Connector | Recheck grants at execution, mediate local data, revoke access, reject duplicate side effects. |
| Supervisor | Launch workers, limit resources, revoke capabilities on stop, terminate descendants and verify cleanup. |

Only explicitly registered tools are exposed. No default arbitrary shell, dynamic host extensions, package installation or automatic loading of workspace instructions as trusted policy. Tool output remains untrusted model input. Even this narrow runtime requires secure parsers, bounded network access and isolation for risky helpers; using Rust alone does not supply those protections.

Approval must survive a crash without silently re-authorizing changed arguments. Recovery must query or reconcile an operation with an uncertain outcome instead of blindly replaying it. Cancellation must revoke access before worker shutdown, and must report incomplete cleanup rather than prematurely promising that every action stopped.

### What not to recreate initially

Do not write a new cross-platform OS sandbox, cryptographic scheme or full provider protocol stack. A full coding harness adds terminal lifecycle, patch application, context compaction, durable sessions, streaming recovery, subprocess cleanup, tool discovery and compatibility testing. This is a separate product-sized commitment, not a small extension to a tool-calling loop.

## Decision and validation gates

| Product need | First option to validate | Reason / remaining cost |
| --- | --- | --- |
| Typed business tools and authorized local documents | Small Rust runtime with reusable components | Closely fits Threadline capabilities; Threadline owns durability and lifecycle. |
| Multi-model coding and shell execution | Goose ACP or Pi worker, each behind a validated execution boundary | Reuses an existing loop; default tool exposure, installation and child-process cleanup need proof. |
| General-purpose agent with context management and subagents | Deep Agents worker with custom Connector backend | More assembled orchestration; adds Python/JS packaging and backend validation. |
| Existing service workflows in Microsoft stack | Microsoft Agent Framework | Useful workflow integration; does not establish desktop isolation. |

Codex remains a comparison baseline for the coding path, subject to the provider and lifecycle limitations already recorded in the earlier report. Choose one initial engine after a spike; do not implement every adapter in advance.

A bounded evaluation should use the same product scenario: read an authorized document, propose an edit, request approval and commit it through the Connector. Add an unauthorized read, network exfiltration attempt, crash during approval, ambiguous external write, cancellation during a tool call and restart recovery. For coding, add symlink escapes, child processes and background-process cleanup. An unavailable sandbox must fail closed rather than fall back to host execution.

Record release/commit pins, host OS, policy, event traces and observed results. Compare startup time, memory, packaging dependencies and cleanup alongside task success. Allocate one or two days per focused spike as an evaluation budget, not a delivery estimate. Repeat OS-specific security checks on every supported desktop platform before making cross-platform claims.

## Goose ACP macOS verification (2026-09-24)

**Decision from this bounded test:** Goose ACP is technically integrable as a separate Rust-built process, but its default configuration fails Threadline's local-tool boundary. A preconfigured, isolated process can omit the tested built-in tools. ACP cancellation did not stop one deliberately backgrounded shell child. Goose is therefore a candidate for a supervised coding worker, not an approved production runtime or a substitute for the Connector.

The host was macOS Darwin 25.4.0, arm64. The official [v1.52.0 release](https://github.com/aaif-goose/goose/releases/tag/v1.52.0) macOS arm64 CLI archive had SHA-256 **7674b0124aab685c71f8782fb7e65bac100c736ce3de0c9d3bf46ba07910e412**, matching the release asset digest. The extracted binary reported version 1.52.0. Its Mach-O signature was ad hoc; this test did not assess distribution signing or notarization. No real model credentials or user documents were supplied.

The child process received only synthetic environment values and an isolated absolute GOOSE_PATH_ROOT. A child macOS Seatbelt profile denied reads and writes under the user's home and denied non-loopback networking; it permitted a temporary localhost OpenAI-compatible fixture server. Controls showed localhost HTTP succeeded and a non-loopback IP connection failed under that profile. This is **external test isolation**, not a Goose-provided sandbox. The source's [path resolver](https://github.com/aaif-goose/goose/blob/v1.52.0/crates/goose/src/config/paths.rs) explains why GOOSE_PATH_ROOT was required: an initial attempt using GOOSE_CONFIG_DIR alone tried to create a config directory in the user's home and failed under the child profile.

The test sent ACP initialize, session/new and session/prompt over stdio. The synthetic model returned the release's [basic text fixture](https://github.com/aaif-goose/goose/blob/v1.52.0/crates/goose/tests/acp_test_data/openai_basic.txt) or a [shell tool-call fixture](https://github.com/aaif-goose/goose/blob/v1.52.0/crates/goose/tests/acp_test_data/openai_shell_tool_call.txt); it did not call a real model endpoint. The [ACP new-session implementation](https://github.com/aaif-goose/goose/blob/v1.52.0/crates/goose/src/acp/server/new_session.rs) and [extension selection](https://github.com/aaif-goose/goose/blob/v1.52.0/crates/goose/src/acp/server.rs) were read alongside the observed events.

| Probe | Observed result | Limit |
| --- | --- | --- |
| ACP initialize and synthetic text turn | Protocol v1 initialized; session/new succeeded with synthetic provider settings; session/prompt streamed text and ended normally. | Only the macOS CLI and a local fixture were exercised. |
| Fresh configuration | New session mode was auto. Eight extensions loaded, including developer. Model request advertised shell, write and edit; a synthetic shell call ran echo and returned its output. | This demonstrates the tested default, not every configuration or tool. |
| Preconfigured restricted session | GOOSE_MODE was approve and all observed default extensions, including developer, were disabled before startup. New session mode was approve, extensionResults was empty, and model requests had no tools. A forced shell call failed: “Tool 'shell' was not advertised for this model turn.” | The test did not attach a Connector tool or attempt to re-enable tools through recipes, client metadata or later configuration changes. |
| Foreground command cancellation | A command created a start marker, slept five seconds, then would create a completion marker. After the start marker appeared, session/cancel returned stopReason cancelled; the completion marker was absent six seconds later. | One attached command under one Seatbelt policy. |
| Background child cancellation | A command launched a background child that would write a marker after five seconds, then kept the foreground shell busy. After the start marker appeared, session/cancel returned cancelled, **but the background marker appeared within nine seconds**. | One synthetic detached-child pattern; other launch forms, shutdown and OS platforms remain untested. |

The default auto mode and developer extension match Goose's [permission-mode definition](https://github.com/aaif-goose/goose/blob/v1.52.0/crates/goose-provider-types/src/goose_mode.rs) and [developer-tool documentation](https://github.com/aaif-goose/goose/blob/v1.52.0/documentation/docs/mcp/developer-mcp.md). The successful restricted configuration shows that the tools can be absent at session creation in this setup. It does not prove that a managed Threadline build has permanently removed all host-capable paths.

The restricted config set GOOSE_MODE to approve and marked analyze, apps, extensionmanager, scheduler, summon, tom, skills and developer disabled before process startup. The test process used an empty environment except for those isolated paths, a fake provider key, the localhost fixture endpoint and required execution variables. A session without any provider setting failed explicitly with “Configuration value not found: GOOSE_PROVIDER”.

**Integration gate:** Threadline must create the configuration before process launch, restrict extension and MCP loading, expose local files only through its authorized Connector, and run the worker inside an OS-level containment boundary. A security Stop must revoke Connector capabilities, cancel inference, terminate the sandbox/process group including descendants, and verify cleanup. A cancelled ACP prompt alone cannot be treated as evidence that host execution has stopped. Repeat the same scenario with a real Connector adapter, real endpoint policy and on every supported desktop OS before choosing Goose for production.

No real-model task quality, approval-to-exact-arguments binding, crash recovery, session retention, third-party MCP behavior, Windows/Linux isolation, or signed desktop packaging was validated. The temporary probe scripts and synthetic files were not product code.

## Handoff and evidence limits

- Issue: #210; branch: `codex/210-runtime-alternatives`.
- Changed surface: this research note only; no protocol, runtime, dependency or security policy changes.
- Verification: primary-source reading, document checks and the bounded Goose macOS ACP experiment above. The earlier Codex report separately records its bounded experiments.
- Open decision: whether arbitrary code execution is required for the first release. The recommendation above covers both answers.
- Risks: moving upstream documentation, untested packaging and lifecycle semantics, and integration work at authorization boundaries. Goose cancellation left a tested background child running.
- Next owner action: choose the initial product scope; validate Connector-only tools and process-tree containment before considering Goose for integration. No merge or production readiness is implied by this note.
