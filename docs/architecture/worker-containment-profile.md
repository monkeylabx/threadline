# Worker containment profile

Status: Draft research and Runtime/Security interface input; no implementation or admission.
Evidence reviewed: 2026-10-17. Origin: [#239](https://github.com/monkeylabx/threadline/issues/239),
following [#235](https://github.com/monkeylabx/threadline/issues/235).

This profile concerns one worker's process lifetime and its descendants. It does
not approve a filesystem, network or IPC sandbox, a real Goose binary, tool
execution, protected input, or a business Run transition. Ordinary IM remains
independent. All recommendations below are Threadline design proposals; OS
mechanisms are separately attributed to their primary sources.

No OS adapter, crash experiment, model, Goose worker or user-file test was run
for this document. Online `latest`/`main` sources are moving references; archived
Apple material is historical evidence. A successor must record exact OS build,
SDK, dependency and fixture versions and verify the mechanisms on that build.
API introduction dates below do not establish Threadline's supported OS policy.

## Required meaning of cleanup

The existing [StopDriver](../../crates/agentd/src/stop_driver.rs) and
[worker stop coordination](../../crates/agentd/README.md#worker-stop-coordination)
remain authoritative. `SealedAndEmpty` requires independent evidence about the
same exclusively owned boundary: admission is irreversibly fenced, no descendant
can escape it, no live member remains, and OS wait/resource-release obligations
have completed. A caller-supplied `sealed` boolean is not evidence.

An ACP cancellation acknowledgement, root exit, a successful termination call,
an empty PID listing, or an empty container still open to later assignment does
not meet this contract. OS ownership must begin before worker code can run.
Unknown ownership, timeout, loss of observation or a leaked control capability
keeps cleanup unconfirmed. No recovery operation may target a reused bare PID,
process name or caller-selected container.

## Mechanisms and limits

### Windows: Job Objects are the first synthetic candidate

- A job groups associated processes; ordinary `CreateProcess` descendants join
  by default. Both breakaway flags change this behavior. The documentation also
  explicitly excludes children created through `Win32_Process.Create`; a job is
  therefore not a proof that every broker-created effect belongs to it.
  [Microsoft: Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects).
- With `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, closing the **last** job handle
  terminates associated processes. Any surviving handle, including a read-only
  observer's handle, changes the crash-cleanup premise. This is a lifetime
  mechanism, not proof of completed cleanup at the instant of close.
  [Microsoft: limit flags](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-jobobject_basic_limit_information).
- `CREATE_SUSPENDED` prevents the primary thread from running until resume.
  `PROC_THREAD_ATTRIBUTE_JOB_LIST` supplies jobs during process creation and is
  documented from Windows 10 / Server 2016. Threadline's candidate must use both,
  verify membership before resume, and reject unsupported or failed creation;
  creating suspended and assigning afterward leaves an unowned crash window.
  No such fallback is allowed.
  [Microsoft: creation flags](https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags),
  [creation attributes](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-updateprocthreadattribute).
- Nested jobs have been available since Windows 8 / Server 2012. Association
  depends on a valid hierarchy and existing restrictions; assignment can fail.
  A descendant in a nested job remains within its ancestor jobs unless permitted
  breakaway applies. The candidate denies both breakaway flags, probes the
  actual host/CI job topology and fails closed rather than escaping a host job.
  [Microsoft: Nested Jobs](https://learn.microsoft.com/en-us/windows/win32/procthread/nested-jobs),
  [assignment requirements](https://learn.microsoft.com/en-us/windows/win32/api/jobapi2/nf-jobapi2-assignprocesstojobobject).
- `CreateJobObjectW` permits an unnamed object and a non-inheritable handle.
  Threadline proposes a fresh unnamed job with no worker-visible job handle.
  Explicit `HANDLE_LIST` inheritance must include only approved fixture pipes,
  never an owner-process, job or broad control handle. No raw handle or job name
  is accepted from an ACP message or application caller.
  [Microsoft: job creation](https://learn.microsoft.com/en-us/windows/win32/api/jobapi2/nf-jobapi2-createjobobjectw),
  [handle-list semantics](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-updateprocthreadattribute).
- `PROCESS_DUP_HANDLE` permits cross-process handle duplication; a duplicate
  refers to the same object and in some cases can request greater access.
  Merely making handles non-inheritable, unnamed or query-only does not prove
  protection from a hostile worker able to open the owner process. Preventing
  that access and broker/alternate-parent creation requires separately reviewed
  process/token/DACL/IPC policy, with negative tests. AppContainer provides
  separate resource/process isolation capabilities; selecting and configuring
  those capabilities remains pending, not implied by use of a job.
  [Microsoft: DuplicateHandle](https://learn.microsoft.com/en-us/windows/win32/api/handleapi/nf-handleapi-duplicatehandle),
  [process access rights](https://learn.microsoft.com/en-us/windows/win32/procthread/process-security-and-access-rights),
  [AppContainer isolation](https://learn.microsoft.com/en-us/windows/win32/secauthz/appcontainer-isolation).
- `TerminateJobObject` targets members and descendant jobs; termination is an
  attempted effect, followed by observation. Process termination can remain
  pending on I/O; bounded waits on retained process handles establish exit.
  `ActiveProcesses` is independently queryable job accounting, with documented
  reference-release effects. Completion-port messages are generally not
  guaranteed, so they cannot be the only source of empty evidence.
  [Microsoft: job termination](https://learn.microsoft.com/en-us/windows/win32/api/jobapi2/nf-jobapi2-terminatejobobject),
  [asynchronous process termination](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-terminateprocess),
  [accounting fields](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-jobobject_basic_accounting_information),
  [completion notifications](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-jobobject_associate_completion_port).

Threadline design inference: normal shutdown may combine an irreversible owner
launch/assign fence, exclusive handle provenance, no permitted escape, observed
`ActiveProcesses == 0`, and discharged wait/handle obligations. Zero accounting
alone is insufficient while the owner or any external actor can assign again.
The sealed owner must retain no callable launch, assign, duplication, reopening
or retry path. A query-only handle is defense in depth, not an irreversible OS
seal by itself. The future implementation must derive this terminal condition
from its private ownership state and reviewed capability graph.

On agentd death, Windows closes that process's handles; other holders can keep
objects alive. Thus kill-on-close needs every job handle confined to agentd.
An external crash-test observer must hold only bounded process wait/query
handles, not a job handle. Its observations validate a synthetic experiment;
they cannot become a posthumous agentd `SealedAndEmpty` or Core `CANCELLED` report.
[Microsoft: process termination and handle lifetime](https://learn.microsoft.com/en-us/windows/win32/procthread/terminating-a-process),
[process wait API](https://learn.microsoft.com/en-us/windows/win32/api/synchapi/nf-synchapi-waitforsingleobject).

### Linux: cgroup v2 needs a manager and a reaping plan

Kernel cgroup v2 documentation specifies inherited membership at fork,
permission-controlled migration, and `cgroup.kill` covering a subtree while
handling concurrent fork/migration. `cgroup.events:populated=0` means no live
processes in the subtree; zombies are absent from `cgroup.procs`, and an empty
cgroup can be removed while zombies remain. Freezing permits migration and is
not an admission seal. A dying removed cgroup cannot accept new members.
[Linux kernel: cgroup v2](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html).

Threadline inference: a future Linux adapter needs a stable owned cgroup
reference, worker placement before execution, no worker access to migration
authority, terminal manager admission fencing, subtree kill, independent
population observation and identity-safe removal. Directory/PID-list absence
alone proves neither ownership nor reaping; a recreated path is a new object.
The reaper must discharge descendant wait obligations separately because
terminated children require waits to release zombie resources.
[Linux man-pages: wait](https://man7.org/linux/man-pages/man2/waitid.2.html).

systemd's delegation guidance requires one manager per cgroup and defines the
delegation boundary. A managed service launches its own processes; a scope
collects processes launched elsewhere. `KillMode=control-group` applies to all
remaining unit members on stop, with configured stop timeout and final signal.
Delegation or placing a worker in a scope does not itself establish what happens
when a separate agentd client crashes.
[systemd: delegation and unit ownership](https://github.com/systemd/systemd/blob/main/docs/CGROUP_DELEGATION.md),
[kill policy source](https://github.com/systemd/systemd/blob/main/man/systemd.kill.xml).

A candidate independent manager could bind worker service lifetime to an
agentd service using `BindsTo=` plus `After=`; this needs a separate deployment
and identity contract. `ExitType=cgroup` keeps a service active while any member
remains, so it is not itself supervisor-death termination policy. Disable
unintended restart/activation and test agentd and manager failure independently.
[systemd: dependency semantics](https://github.com/systemd/systemd/blob/main/man/systemd.unit.xml),
[service exit semantics](https://github.com/systemd/systemd/blob/main/man/systemd.service.xml).

`PR_SET_PDEATHSIG` applies to the calling process, is cleared in forked children
and has setup/parent-thread caveats. A process group can change through
`setsid()`. Neither mechanism alone proves a sealed descendant boundary.
[Linux man-pages: parent-death signal](https://man7.org/linux/man-pages/man2/PR_SET_PDEATHSIG.2const.html),
[setsid](https://man7.org/linux/man-pages/man2/setsid.2.html).

### macOS: launchd and App Sandbox address different obligations

Apple's published launchd manual source describes cleanup of processes with
the job's process-group ID unless `AbandonProcessGroup` is enabled. Apple's
archived `setsid(2)` documents creation of a new session/process group. These
sources do not establish containment of a descendant that changes groups.
The launchd guide also expects cooperative daemon behavior, including avoiding
daemonization and `setsid`. Threadline cannot treat those expectations as
enforcement against arbitrary worker code.
[Apple: launchd.plist source](https://github.com/apple-oss-distributions/launchd/blob/main/man/launchd.plist.5),
[setsid manual](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/setsid.2.html),
[launchd behavior guide](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html).

App Sandbox restricts resource access through entitlements. Apple's inheritance
guidance distinguishes child-process inheritance from XPC privilege separation.
Neither cited document supplies the seal/descendant-empty/reaping receipt
required here. A macOS successor must investigate a concrete signed helper/XPC
and sandbox profile, including changed-group descendants and supervisor death,
before admission; this is an evidence gap, not a claim that macOS containment
is impossible.
[Apple: App Sandbox](https://developer.apple.com/documentation/security/app-sandbox),
[sandbox inheritance guidance](https://developer.apple.com/library/archive/documentation/Miscellaneous/Reference/EntitlementKeyReference/Chapters/EnablingAppSandbox.html).

## Reversible first implementation recommendation

Choose one **synthetic Windows Job Object ownership experiment** first because
the cited APIs jointly offer descendant membership, creation-time assignment,
last-handle crash termination and independent accounting. This research
recommendation does not select a product OS minimum or supersede accepted ADRs.
Linux and macOS remain unavailable until separately evidenced.

The successor is one Runtime-owned, internal adapter and checked-in native
fixture; it accepts no executable/path/handle/PID/job selector from a caller.
It permits no Goose, model, credentials, user files, external broker, network or
general tool invocation. The fixture and trusted harness are cooperative except
for explicitly scripted adversarial behaviors. This narrows experimental trust;
it does not establish resistance to hostile same-user worker code.

Before implementation, Integration must authorize any direct Windows API
dependency and required manifest/lockfile changes. A transitive `windows-sys`
lock entry is not permission to depend on it. No dependency is added here.
Integration must also add explicit native Windows adapter tests/evidence:
the current [build workflow](../../.github/workflows/build.yml) runs agentd
tests on Ubuntu; its Windows SQLCipher/Tauri checks do not exercise this owner.
Existing green platform checks cannot admit the future Windows adapter.

The executable experiment must gate launch, configure the unnamed job before
creation, provide `JOB_LIST` at creation, verify membership before resume, and
test each crash checkpoint. Normal stop must preserve a terminal ownership
fence, invoke job termination, complete bounded waits and close owned process
references before the final independent accounting query. It must
prove why no later assignment/duplication can occur before claiming cleanup.
The crash harness waits on pre-established fixture process handles only and
never retains a job handle or infers exit from missing heartbeat/output.

A leaked job handle is a required negative experiment: kill-on-last-close must
not be claimed when that premise is intentionally broken. Restart starts a new
owner generation with prior effects uncertain; it never opens/adopts an old job
by name, kills by PID, resumes the old Session or replays an uncertain effect.
Without an independently reviewed recovery receipt, prior local cleanup stays
unconfirmed and durable Run/effect reconciliation belongs to Core.

Passing this experiment supplies process-lifetime evidence only. Production
still requires separate denial of handle theft and broker/alternate-parent
escape, filesystem/network/IPC sandbox policy, trusted Run/Lease/Grant and
approval authority, protected input/output lifetime and effect reconciliation,
plus pinned Goose admission. Failure of any required gate leaves the actual
runtime unavailable; the adapter cannot approve its own security admission.

## Process owner and stop-driver contract

The producer is one private OS process owner, created by a trusted supervisor
for one boundary and incarnation. The consumer is a future `StopDriver` adapter;
the UI and ACP peer receive no OS selector or ownership constructor. The owner
supplies termination and observation only. Revoking Grants/queued tools/pending
approval execution and issuing ACP interruption retain their existing owners;
the synthetic driver may script those steps but cannot admit them in production.

| Owner condition | Permitted behavior | Observation supplied to the consumer |
| --- | --- | --- |
| Preparing | Configure the fresh boundary, inheritance and creation attributes before starting the fixture. Failure closes owned resources; never resume outside the boundary. | No cleanup proof until failed-launch resources are reconciled. |
| Running | One internal fixture launch; descendants stay in the same ancestor job. No public relaunch/assignment/duplication API. | `LiveMembers` when independently observed; missing evidence is an error. |
| Stopping | Irreversibly fence launch/assignment before effects; terminate, wait/release process references and query the retained job. Explicit retries can advance cleanup only. | `LiveMembers` or coarse error until every proof obligation holds. |
| Confirmed | No route back to Preparing/Running and no remaining OS effects or owner resources to release. | `SealedAndEmpty` only after stable exclusive ownership, no escape/admission, zero accounting and discharged wait/resource obligations are established together. |
| Owner lost | No callbacks can return on behalf of the dead incarnation. Restart creates a distinct owner and leaves earlier effects uncertain. | No inferred proof or durable cancellation. |

This is an ownership lifecycle, not another Run state machine or a new public
type/API. The launch fence and capability provenance must be implementation
invariants established before observation, never a request field or a supplied
"trusted" flag. `WorkerStop` retains successful cleanup evidence, so a later
assignment or external mutator would invalidate its contract permanently.

Preserve the existing order: latch dispatch stop, attempt revocation, interrupt
once, terminate and observe. Revocation/interruption errors never bypass
termination. Successful termination is a step result, not cleanup proof;
independent proof can succeed after a termination error while that error remains
visible. Repeated confirmed stop releases no new effects. `StopReport` stays a
local observation; only the authorized owner/Core path reconciles durable state.

For the Windows candidate, wait for retained process handles to signal, close
them and any test observer's process references, then query job accounting while
retaining the owner's job handle. Microsoft's documented reference-release
condition means querying zero while retaining those references can deadlock the
proof procedure. If a process reference cannot be discharged, report uncertainty;
do not reinterpret the accounting field or close the job early to fabricate proof.
After the final zero query, successfully close the last owned job handle before
returning `SealedAndEmpty`. A close failure leaves cleanup unconfirmed; no resource
release is deferred until after confirmation.
[Accounting source](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-jobobject_basic_accounting_information).

Each explicit synthetic stop attempt has one trusted monotonic budget of ten
seconds for OS waits/observation; all operations consume the remaining budget.
This is a test-profile choice, not an OS termination guarantee or a production
SLO. No infinite wait, busy loop or hidden retry is permitted. Exhaustion or
unavailable proof returns `StopError::Unavailable` (mapped to Unknown cleanup);
an unsupported/rejected policy returns `StopError::Rejected`. Neither error
restores launch. Further cleanup requires an explicit bounded attempt.

Diagnostics contain only fixture-case ID, phase, coarse outcome, durations and
OS/SDK/dependency versions. No raw OS error string, worker stdout/stderr, prompt,
credential, user path or protocol payload is retained or published. An owned
resource-release failure remains visible; it cannot be silently ignored.

## Planned synthetic acceptance and reproduction

Every row below is **NOT RUN**. Integration first pins the direct Windows API
dependency/features and adds an explicit native Windows test job. Runtime then
implements one internal owner plus the checked-in synthetic fixture, with at
most eight fixture processes enforced by a job active-process limit. Eight is
an experimental resource bound, not a Goose or production allowance. The test
observer uses readiness pipes/events and identity-bound handles, never a fixed
sleep or process-name/PID kill. Each case has an outer 30-second harness deadline
and a separate owned-fixture cleanup path; no test may leave a live fixture.

| Case | Required observable result |
| --- | --- |
| Root exits while child/grandchild remains | Parent exit cannot confirm cleanup; stopping terminates and independently verifies all contained members. |
| Concurrent spawning, nested job and process-limit flood | Descendants remain in the ancestor boundary; excess creation fails without escape or unbounded allocation. |
| Breakaway or alternate launch attempt | Both job breakaway settings remain disabled; tested breakaway is denied. Broker/alternate-parent hostility remains a separate production admission test, never assumed covered by this fixture. |
| Missing JOB_LIST, rejected nesting, membership or resume failure | No fallback launch and no fixture work marker; reconcile every created owned resource before any proof. |
| Agentd killed before creation, after creation/before resume, and with descendants running | Last-close behavior ends the pre-established fixture processes; external observer holds no job handle. No dead-owner report or Core cancellation is invented. |
| Deliberately retained external job handle | Negative experiment demonstrates the last-close premise is broken; cleanup cannot be claimed. Harness later closes the exact leaked handle and verifies fixture cleanup. |
| Delayed process exit or retained process reference | No zero-count/proof shortcut; complete waits and reference release before final accounting. |
| Missing completion notification | Independent accounting still determines the result; notification receipt is never necessary or sufficient proof. |
| Revocation/interruption/termination/query/wait/resource-release failure or deadline | Termination is still attempted as appropriate; coarse pending/error remains visible and no dispatch reopens. Fault injection is labelled synthetic. |
| Repeated stop and racing launch versus stop | Fence precedes effects; no new launch/assignment after seal and no effects after confirmed stop. |
| Restart after crash or uncertain tool outcome | Distinct incarnation; no adoption by name/PID, old Session reuse, inferred cleanup receipt or input/effect replay. |
| Unrelated harness process outside the boundary | Remains untouched; cleanup never broadens to process names, a user session or the whole machine. |

After those prerequisites, the successor must provide a native Windows
`windows_containment` integration-test target. Planned commands (the target does
not exist in this documentation PR and these commands are **NOT RUN**):

```text
cargo test -p threadline-agentd --locked --test windows_containment -- --list
cargo test -p threadline-agentd --locked --test windows_containment -- --test-threads=1
cargo clippy -p threadline-agentd --all-targets --locked -- -D warnings
cargo fmt --all --check
```

The Windows CI owner must assert that the required cases actually executed,
with no zero-test/cfg-skipped success, and retain bounded redacted result evidence
including the OS build, target architecture, fixture hash, dependency/SDK pins,
deadlines and per-case outcomes. Linux/macOS compilation or documentation-only
result jobs cannot substitute. A later security review must separately test
hostile process-handle access, external brokers and filesystem/network/IPC denial
before a real worker probe. Neither this Draft nor a passing synthetic fixture
is that review, and no successor is automatically production-ready.
