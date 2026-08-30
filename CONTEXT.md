# Threadline

Threadline is an enterprise instant-messaging system where people and permissioned Agents collaborate while execution context and authority remain explicit and scoped.

## Language

**Channel**:
A long-lived shared conversation whose membership defines who may participate. Human access to encrypted content is realized through separately authorized Devices; Agent or Service participation does not imply possession of Channel keys. A Channel is not an Agent session or a unit of execution.
_Avoid_: Agent session, task room

**DM**:
A private conversation among explicitly selected members. A DM has its own cryptographic membership boundary; changing its participants creates a new DM rather than inheriting the original DM's history.
_Avoid_: Private Channel

**E2EE Group**:
The cryptographic membership boundary corresponding to exactly one Channel or DM. Threads and Task Threads inherit their parent conversation's E2EE Group.
_Avoid_: Thread group, Task group, Run group

**Epoch**:
A numbered version of an E2EE Group's membership and cryptographic state. A membership, device-key, or recovery-key change advances the Group to a new Epoch so removed participants cannot access future messages and updated participants can regain post-compromise protection.
_Avoid_: Time window, message batch, key version

**Thread**:
A focused conversation within a Channel or DM. It inherits the parent conversation's membership and cryptographic visibility.
_Avoid_: Sub-channel

**Task Thread**:
The shared conversation surface for a Task. It inherits its parent Channel or DM's visibility and does not create a private subgroup.
_Avoid_: Agent session

**Capability Grant**:
A short-lived authorization that permits a specific actor to access defined resources or perform defined operations. It limits Agent access without changing E2EE Group membership.
_Avoid_: Channel membership, blanket permission

**Actor**:
An identified Human, Agent, or Service that can participate in Threadline under explicit membership, responsibility, and audit rules. Actor identity alone does not confer cryptographic access.
_Avoid_: User, process

**Authenticated Principal**:
The Tenant, Actor, Device, and Session identity produced by a trusted authentication boundary for one request. Request fields cannot construct or replace it, and it never contains the bearer credential.
_Avoid_: Caller identity, request user

**Member**:
The current participation fact for one Actor in one Organization, including lifecycle state and Organization Role. An Actor is not automatically a Member of every Organization.
_Avoid_: Actor, user account

**Organization Role**:
The Tenant-level RBAC ceiling attached to a Member. It cannot create Channel Membership or content access by itself.
_Avoid_: Channel Role, global content access

**Channel Role**:
The permission ceiling attached to one current Channel Membership interval. Leaving ends that interval; rejoining creates a new interval without restoring historical access.
_Avoid_: Organization Role, permanent membership

**Authorization Action**:
A stable, auditable operation evaluated against one resource. It is not an RPC procedure name, Capability, or blanket permission.
_Avoid_: Endpoint name, Capability Grant

**Resource ACL**:
A versioned set of Actor-specific allow or deny constraints for one resource. It can only narrow permission already allowed by current Membership and Roles; matching deny takes precedence.
_Avoid_: Channel Membership, effective permission cache

**Authorization Decision**:
A one-time allow or deny result for the current Authenticated Principal, Authorization Action, resource, and versioned facts. It is re-evaluated for each protected action and is never persisted as effective permission.
_Avoid_: Session claim, cached permission

**Domain Event**:
An immutable, Tenant-scoped fact that a domain change committed successfully. Delivery state, broker routing, retries, and projections cannot rewrite the fact.
_Avoid_: Command, broker message, Outbox Entry

**Transactional Outbox Entry**:
A durable delivery record created atomically with one Domain Event for one destination. Its delivery state may advance, but the referenced event facts never change.
_Avoid_: Domain Event, queue message, Job

**Delivery Claim**:
Short-lived, fenced authority for one Worker attempt to advance one Transactional Outbox Entry. After expiry, an atomic replacement claim supersedes it without transferring ownership of the Domain Event.
_Avoid_: Domain ownership, permanent lock, consumer ACK

**Channel Membership**:
The application-level right of an Actor to participate in a Channel. For a Human, content access is exercised through authorized Devices; Agent and Service membership does not create MLS membership.
_Avoid_: E2EE Group membership, Capability Grant

**Cryptographic Member**:
An authorized Device represented in an E2EE Group and capable of participating in its current Epoch. Human, Agent, and Service Actors are not themselves cryptographic members.
_Avoid_: Actor, Channel Member

**Membership Change**:
An ordered, Control-Plane-authorized change to the Devices represented in an E2EE Group. Its cryptographic effect is completed by an accepted MLS Commit rather than by the server handling group secrets.
_Avoid_: Server rekey, local membership edit

**Committer Device**:
A currently authorized Cryptographic Member that converts an approved Membership Change into an MLS Commit. It is a temporary protocol role, not the owner of the Channel or its keys.
_Avoid_: Key server, group owner

**Rekey Required**:
The safety state after a member removal or Device revocation in which new application messages remain pending until an accepted Commit establishes the successor Epoch.
_Avoid_: Offline, sync pending

**Agent Actor**:
An auditable non-human participant that can be assigned Tasks and publish attributed results. It receives only Capability-scoped Context and never holds standing Channel or Epoch keys.
_Avoid_: Bot account, MLS member, Agent session

**History Sharing**:
An explicit, auditable grant of retained pre-join Channel history to a newly authorized member. It is bounded by current access and retention policy and is not ordinary server-side key retrieval or enterprise recovery.
_Avoid_: Automatic key backfill, recovery

**Content Key**:
A random key protecting one message, Attachment, or Artifact. It is independently disposable and is not an MLS Ratchet Secret or a reusable Channel key.
_Avoid_: Message password, Epoch key

**History Key**:
Retained, Epoch-scoped material that can unwrap Content Keys still covered by access and retention policy. It enables History Sharing and approved recovery while intentionally limiting forward secrecy to history whose keys have already been destroyed.
_Avoid_: MLS Ratchet Secret, Channel master key

**Retention**:
The policy that bounds how long content and its decryption capability remain available across server storage and authorized Devices. On a normally operating Device, expiry requires local cryptographic erasure even while offline; it is not a promise of remote physical deletion from a compromised Device.
_Avoid_: Server TTL, remote wipe guarantee

**Device**:
An independently authorized endpoint belonging to a Member, with its own identity, lifecycle, and access to cryptographic state. Authorizing a Member does not automatically authorize every device they operate.
_Avoid_: Member, session

**Device Credential**:
A time-bounded enterprise binding between a Tenant, Actor, Device, and Device Identity Key. It establishes cryptographic device identity independently of an OIDC login session.
_Avoid_: OIDC token, user credential, KeyPackage

**Device Authority**:
The enterprise-controlled trust authority that can approve the first Device or an exceptional replacement Device. The ordinary application Control Plane cannot establish a new Cryptographic Member by itself.
_Avoid_: OIDC provider, IM server

**Device Enrollment**:
The high-impact approval process that turns a Device into an authorized cryptographic endpoint. It requires endorsement by an existing authorized Device or the Device Authority and produces an auditable approval chain.
_Avoid_: Login, session creation

**KeyPackage**:
A short-lived, normally single-use public package through which a Device can be added to an MLS Group. It is bound to that Device's credential, supported protocol version, cipher suite, and capabilities.
_Avoid_: Device Credential, reusable invitation

**Device History Sharing**:
An end-to-end transfer of retained history access from an existing authorized device to a newly authorized device belonging to the same Member. Without an eligible source device, the new device starts from its current authorization point unless a separate enterprise recovery case is approved.
_Avoid_: Device sync, automatic recovery

**Recovery Case**:
A time-bounded, auditable request to recover a defined set of retained E2EE Group Epochs for one designated recipient device. A Recovery Case is not standing access to a Tenant, Channel, or DM.
_Avoid_: Admin decrypt, recovery session

**Recovery Envelope**:
Versioned cryptographic material that binds recoverability to one E2EE Group, Epoch, enterprise recovery-key version, and protocol version. It does not grant general access to a group master key.
_Avoid_: Escrowed master key, server key

**Crypto Profile**:
The immutable, named combination of MLS protocol version, cipher suite, and Threadline message, history, and recovery envelope versions used by an E2EE Group. A library release is not a Crypto Profile, and unknown Profiles are never silently downgraded.
_Avoid_: OpenMLS version, algorithm preference

**Recovery Recipient Device**:
The explicitly approved, trusted device to which the result of a Recovery Case is delivered end to end. Recovery Control and administrator interfaces are not recovery recipients.
_Avoid_: Recovery Control, administrator account
