# Authorization contract fixtures

These synthetic fixtures freeze Threadline's standing Space and Channel
authorization vocabulary. They contain only invented identifiers, roles, and
policy versions; they contain no credentials, message or file content, Prompt,
key material, production tenant data, or telemetry.

The role matrix is the complete Organization Role × Channel Role × Action
cross-product. It is only one layer of the decision: Tenant equality, current
Member state, current Channel Membership, resource state, Resource ACL,
Runtime Capability/delegation, and required Approval are intersected around it.
No allow at a later layer can revive an earlier deny.

Run the deterministic verifier from the repository root:

```bash
node proto/tools/verify-authorization-contracts.mjs
```

The fixture deliberately does not model ACL inheritance or group subjects. It
also keeps application authorization separate from E2EE Group membership,
decryption capability, keys, and pre-join history access. Physical devices are
not exercised by this contract task.
