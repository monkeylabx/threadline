# Capability Grant signature contract

Status: P03-06B v1 contract

This contract defines the only signed projection accepted for a v1
`threadline.capability.v1.CapabilityGrant`. It is a credential-integrity
interface, not a revocation or authorization cache. Every protected use still
asks Core to re-evaluate current Grant state, policy, membership, ACL, Task,
Run, Device, lease, and fencing facts.

## Profile

`CAPABILITY_GRANT_SIGNATURE_PROFILE_ED25519_JCS_V1` means Ed25519 over these
exact bytes:

```text
UTF8("threadline.capability.grant/v1\n" + JCS(signed_projection))
```

`JCS` is RFC 8785 JSON Canonicalization Scheme. All property names below are
ASCII. Implementations must reject unknown profiles or projection versions;
they must not guess, downgrade, or retry another algorithm. v1 requires
`signedProjectionVersion` to encode Protocol value `1`.

The Ed25519 signature is exactly 64 bytes. Verification keys are resolved from
a trusted, tenant-scoped key ring by `signingKeyId`; a key shipped inside or
alongside an untrusted Grant is never trusted. The key ring is selected from
the independently authenticated expected tenant, never from the untrusted
Grant's `tenantId`. Key resolution and rotation are implementation work outside
this contract.

## Verification interface

Verification receives the untrusted Grant plus an independently authenticated
expected tenant, Task, Run, grantee Actor, and execution Device. It first
requires exact equality between those expected facts and the corresponding
signed claims, resolves `signingKeyId` only inside the expected tenant's trusted
key ring, and then verifies the canonical transcript. The current Device must
come from a Device-authenticated local channel or session, not from a value
supplied by agentd or copied from the Grant. If that authenticated Device
context is unavailable, verification fails closed unless a later contract
defines and verifies Device proof of possession.

Signing `executionDeviceId` preserves the issuer's claim; it does not by itself
prove which Device presented the credential. The external equality check is
what prevents an unchanged captured Grant from being accepted for another
Device, Task, Run, tenant, or grantee.

## Signed projection

The JCS object contains exactly these properties. Proto field numbers are shown
only to make inclusion auditable; field numbers are not JSON property names.

| JCS property | Proto field | Canonical value |
| --- | ---: | --- |
| `capabilityGrantId` | 1 | string |
| `tenantId` | 2 | string |
| `taskId` | 3 | string |
| `runId` | 4 | string |
| `grantee` | 5 | Actor object |
| `initiator` | 6 | Actor object |
| `capabilities` | 7 | array of enum wire numbers as decimal strings |
| `resourceScope` | 8 | Resource Scope object |
| `issuedAt` | 10 | normalized timestamp string |
| `expiresAt` | 11 | normalized timestamp string |
| `nonceHex` | 13 | lowercase hexadecimal string |
| `policyVersion` | 14 | string |
| `executionDeviceId` | 16 | string |
| `signatureProfile` | 17 | enum wire number as a decimal string |
| `signingKeyId` | 18 | string |
| `signedProjectionVersion` | 19 | unsigned decimal string |

An Actor object contains exactly `actorId` and `actorType`; `actorType` is its
enum wire number as a decimal string. A Resource Scope object contains exactly
`channelIds`, `dmIds`, `eventIds`, `threadIds`, `fileIds`,
`workspaceBindingIds`, `workspacePathPrefixes`, and `toolIds`, each an array of
strings.

Capabilities are non-empty, unique, and sorted by ascending wire number. Every
Resource Scope array is unique and sorted by unsigned lexicographic UTF-8 byte
order. An empty array grants nothing. Producers canonicalize before signing;
verifiers reject unsorted or duplicate inputs instead of silently repairing
them. This preserves one byte representation for one logical Grant.

Timestamps are UTC RFC 3339 strings with exactly nine fractional digits:
`YYYY-MM-DDTHH:mm:ss.sssssssssZ`. `expiresAt` must be later than `issuedAt`.
The nonce is exactly 32 bytes and is encoded as 64 lowercase hexadecimal
characters. Identifiers and path entries are non-empty, already trimmed,
contain no Unicode control character, and contain neither `*` nor `?`. They
must also be valid Unicode scalar-value strings: lone UTF-16 surrogates and
invalid UTF-8 fail closed rather than being escaped or replaced differently by
language runtimes.

## Deliberately unsigned lifecycle projection

`state` (field 9), `revokedAt` (field 12), and `signature` (field 15) are not in
the signed projection. State and revocation time are mutable control-plane
views; re-signing a credential during revocation would create another
credential and would not make an offline copy current. Therefore:

- signature verification proves only issuer authenticity and immutable claim
  integrity;
- local expiry is an additional deny condition, never an allow decision;
- `ACTIVE` on the wire is never accepted as revocation evidence; and
- every protected action fails closed unless the current Core recheck also
  confirms the Grant remains active and all authorization facts still hold.

Grant bytes, nonce, signature, resource paths, and verification diagnostics are
data class C4. Logs retain only a Grant identifier or hash and a stable outcome
category.
