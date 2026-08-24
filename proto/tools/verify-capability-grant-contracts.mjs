import { createHash, createPublicKey, verify } from "node:crypto";
import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const fixtureRoot = join(repositoryRoot, "test", "fixtures", "proto", "capability-grant");
const manifest = JSON.parse(readFileSync(join(fixtureRoot, "manifest.json"), "utf8"));
const sourceBytes = readFileSync(join(fixtureRoot, manifest.source.file));
const vectors = JSON.parse(sourceBytes.toString("utf8"));
const transcriptPrefix = "threadline.capability.grant/v1\n";
const profileWireNumbers = new Map([
  ["CAPABILITY_GRANT_SIGNATURE_PROFILE_ED25519_JCS_V1", 1],
]);
const scopeKeys = [
  "channelIds",
  "dmIds",
  "eventIds",
  "threadIds",
  "fileIds",
  "workspaceBindingIds",
  "workspacePathPrefixes",
  "toolIds",
];

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function canonicalize(value) {
  if (value === null || typeof value === "boolean" || typeof value === "string") {
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) return `[${value.map(canonicalize).join(",")}]`;
  assert(typeof value === "object", "unsupported canonical transcript value");
  return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${canonicalize(value[key])}`).join(",")}}`;
}

function isCanonicalIdentifier(value) {
  return typeof value === "string" && value !== "" && value.trim() === value &&
    !value.includes("*") && !value.includes("?") && !/[\p{Cc}\p{Cs}]/u.test(value);
}

function utf8Compare(left, right) {
  return Buffer.compare(Buffer.from(left, "utf8"), Buffer.from(right, "utf8"));
}

function isUniqueSorted(values, compare) {
  return values.every((value, index) => index === 0 || compare(values[index - 1], value) < 0);
}

function timestampNanos(value) {
  const match = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})\.(\d{9})Z$/u.exec(value);
  assert(match, "timestamp must use the canonical UTC nanosecond form");
  const millisecondForm = `${match[1]}.${match[2].slice(0, 3)}Z`;
  const milliseconds = Date.parse(millisecondForm);
  assert(Number.isFinite(milliseconds) && new Date(milliseconds).toISOString() === millisecondForm,
    "timestamp must name a real UTC instant");
  return BigInt(milliseconds) * 1_000_000n + BigInt(match[2].slice(3));
}

function validateActor(actor) {
  assert(actor && isCanonicalIdentifier(actor.actorId), "actor identifier is invalid");
  assert(Number.isInteger(actor.actorType) && actor.actorType >= 1 && actor.actorType <= 3,
    "actor type is invalid");
}

function validateScope(scope) {
  assert(scope && JSON.stringify(Object.keys(scope).sort()) === JSON.stringify([...scopeKeys].sort()),
    "resource scope fields are incomplete");
  for (const key of scopeKeys) {
    const values = scope[key];
    assert(Array.isArray(values) && values.every(isCanonicalIdentifier), "resource scope value is invalid");
    assert(isUniqueSorted(values, utf8Compare), "resource scope values must be unique and UTF-8 sorted");
  }
}

function validateGrant(grant) {
  for (const key of [
    "capabilityGrantId", "tenantId", "taskId", "runId", "policyVersion",
    "executionDeviceId", "signingKeyId",
  ]) {
    assert(isCanonicalIdentifier(grant[key]), "signed identifier is invalid");
  }
  validateActor(grant.grantee);
  validateActor(grant.initiator);
  assert(Array.isArray(grant.capabilities) && grant.capabilities.length > 0,
    "capability list must be non-empty");
  assert(grant.capabilities.every((value) => Number.isInteger(value) && value >= 1 && value <= 14),
    "capability value is invalid");
  assert(isUniqueSorted(grant.capabilities, (left, right) => left - right),
    "capabilities must be unique and numerically sorted");
  validateScope(grant.resourceScope);
  assert(timestampNanos(grant.issuedAt) < timestampNanos(grant.expiresAt),
    "Grant expiry must follow issuance");
  assert(/^[0-9a-f]{64}$/u.test(grant.nonceHex), "Grant nonce must be exactly 32 bytes");
  assert(profileWireNumbers.get(grant.signatureProfile) === 1, "signature profile is unsupported");
  assert(grant.signedProjectionVersion === 1, "signed projection version is unsupported");
  assert(/^[0-9a-f]{128}$/u.test(grant.signatureHex), "Grant signature must be exactly 64 bytes");
}

function signedProjection(grant) {
  validateGrant(grant);
  return {
    capabilityGrantId: grant.capabilityGrantId,
    tenantId: grant.tenantId,
    taskId: grant.taskId,
    runId: grant.runId,
    grantee: {
      actorId: grant.grantee.actorId,
      actorType: String(grant.grantee.actorType),
    },
    initiator: {
      actorId: grant.initiator.actorId,
      actorType: String(grant.initiator.actorType),
    },
    capabilities: grant.capabilities.map(String),
    resourceScope: Object.fromEntries(scopeKeys.map((key) => [key, grant.resourceScope[key]])),
    issuedAt: grant.issuedAt,
    expiresAt: grant.expiresAt,
    nonceHex: grant.nonceHex,
    policyVersion: grant.policyVersion,
    executionDeviceId: grant.executionDeviceId,
    signatureProfile: String(profileWireNumbers.get(grant.signatureProfile)),
    signingKeyId: grant.signingKeyId,
    signedProjectionVersion: String(grant.signedProjectionVersion),
  };
}

function canonicalTranscript(grant) {
  return Buffer.from(`${transcriptPrefix}${canonicalize(signedProjection(grant))}`, "utf8");
}

function publicKeyFromRawHex(rawHex) {
  assert(/^[0-9a-f]{64}$/u.test(rawHex), "fixture public key must be exactly 32 bytes");
  const spkiPrefix = Buffer.from("302a300506032b6570032100", "hex");
  return createPublicKey({ key: Buffer.concat([spkiPrefix, Buffer.from(rawHex, "hex")]), format: "der", type: "spki" });
}

function actorsEqual(left, right) {
  return left?.actorId === right?.actorId && left?.actorType === right?.actorType;
}

function verifies(grant, context) {
  try {
    if (grant.tenantId !== context.tenantId || grant.taskId !== context.taskId ||
        grant.runId !== context.runId || !actorsEqual(grant.grantee, context.grantee) ||
        grant.executionDeviceId !== context.executionDeviceId) return false;
    const publicKey = publicKeyFromRawHex(context.trustedPublicKeys[grant.signingKeyId]);
    const transcript = canonicalTranscript(grant);
    return verify(null, transcript, publicKey, Buffer.from(grant.signatureHex, "hex"));
  } catch {
    return false;
  }
}

function mutate(source, path, value) {
  const result = structuredClone(source);
  const segments = path.split(".");
  let target = result;
  for (const segment of segments.slice(0, -1)) target = target[segment];
  target[segments.at(-1)] = value;
  return result;
}

function rejectsCanonicalTranscript(grant) {
  try {
    canonicalTranscript(grant);
    return false;
  } catch {
    return true;
  }
}

assert(manifest.schemaVersion === 1, "Capability Grant manifest schema must be v1");
assert(manifest.classification === "synthetic-public-signature-vector-no-secrets",
  "Capability Grant fixtures must remain explicitly secret-free");
assert(manifest.transcriptDomain === transcriptPrefix.trimEnd(), "Capability Grant transcript domain changed");
assert(manifest.signatureProfile === "CAPABILITY_GRANT_SIGNATURE_PROFILE_ED25519_JCS_V1",
  "Capability Grant signature profile changed");
assert(manifest.signedProjectionVersion === 1, "Capability Grant projection version changed");
assert(sha256(sourceBytes) === manifest.source.sha256, "Capability Grant fixture source hash mismatch");
assert(vectors.schemaVersion === 1, "Capability Grant vector schema must be v1");
assert(vectors.valid.requiresOnlineLifecycleRecheck === true,
  "Capability Grant fixture must require online lifecycle recheck");

const rejectedIds = vectors.rejectedMutations.map(({ id }) => id);
assert(JSON.stringify(rejectedIds) === JSON.stringify(manifest.requiredRejectedMutationIds),
  "Capability Grant rejected mutation coverage changed");
const rejectedContextIds = vectors.rejectedContextMutations.map(({ id }) => id);
assert(JSON.stringify(rejectedContextIds) === JSON.stringify(manifest.requiredRejectedContextMutationIds),
  "Capability Grant authenticated context mutation coverage changed");
const lifecycleIds = vectors.unsignedLifecycleMutations.map(({ id }) => id);
assert(JSON.stringify(lifecycleIds) === JSON.stringify(manifest.requiredUnsignedLifecycleMutationIds),
  "Capability Grant lifecycle mutation coverage changed");
const malformedIds = new Set(manifest.requiredMalformedMutationIds);
assert([...malformedIds].every((id) => rejectedIds.includes(id)),
  "Capability Grant malformed mutation coverage must be a rejected subset");

const grant = vectors.valid.grant;
const context = {
  ...vectors.valid.expectedAudience,
  trustedPublicKeys: { [grant.signingKeyId]: vectors.valid.publicKeyRawHex },
};
const transcript = canonicalTranscript(grant);
assert(sha256(transcript) === vectors.valid.expectedTranscriptSha256,
  "Capability Grant canonical transcript hash changed");
assert(verifies(grant, context), "valid Capability Grant signature and authenticated context must verify");

for (const testCase of vectors.rejectedMutations) {
  const mutated = mutate(grant, testCase.path, testCase.value);
  if (malformedIds.has(testCase.id)) {
    assert(rejectsCanonicalTranscript(mutated),
      `${testCase.id}: malformed input must fail before transcript construction`);
  } else if (testCase.id === "signature") {
    assert(canonicalTranscript(mutated).equals(transcript),
      "signature bytes must remain outside the signed projection");
  } else {
    assert(!canonicalTranscript(mutated).equals(transcript),
      `${testCase.id}: valid signed-field mutation must change transcript bytes`);
  }
  assert(!verifies(mutated, context),
    `${testCase.id}: signed mutation must fail closed`);
}

for (const testCase of vectors.rejectedContextMutations) {
  assert(!verifies(grant, mutate(context, testCase.path, testCase.value)),
    `${testCase.id}: authenticated context mismatch must fail closed`);
}

for (const testCase of vectors.unsignedLifecycleMutations) {
  const mutated = mutate(grant, testCase.path, testCase.value);
  assert(canonicalTranscript(mutated).equals(transcript),
    `${testCase.id}: lifecycle projection must not change the immutable transcript`);
  assert(verifies(mutated, context),
    `${testCase.id}: signature validity must remain distinct from lifecycle validity`);
}

const proto = readFileSync(join(repositoryRoot, "proto", "threadline", "capability", "v1", "capability.proto"), "utf8");
assert(/CAPABILITY_GRANT_SIGNATURE_PROFILE_ED25519_JCS_V1\s*=\s*1;/u.test(proto),
  "Capability Grant Protocol must pin the Ed25519/JCS v1 profile");
assert(/string\s+execution_device_id\s*=\s*16;/u.test(proto),
  "Capability Grant Protocol must bind the execution Device");
assert(/CapabilityGrantSignatureProfile\s+signature_profile\s*=\s*17;/u.test(proto),
  "Capability Grant Protocol must carry the signature profile");
assert(/string\s+signing_key_id\s*=\s*18;/u.test(proto),
  "Capability Grant Protocol must carry the signing key identifier");
assert(/uint32\s+signed_projection_version\s*=\s*19;/u.test(proto),
  "Capability Grant Protocol must carry the signed projection version");

console.log("Threadline P03-06B Capability Grant signature contract is valid.");
