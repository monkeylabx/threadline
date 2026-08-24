import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const fixtureRoot = join(repositoryRoot, "test", "fixtures", "audit");
const transcriptPrefix = "threadline.audit.event/v1\n";
const replayEvidencePrefix = "threadline.audit.outbox-replay-evidence/v1\n";
const zeroHash = "0".repeat(64);

class ContractError extends Error {
  constructor(code) {
    super(`audit contract: ${code}`);
    this.code = code;
  }
}

function fail(code) {
  throw new ContractError(code);
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function decodeUTF8(bytes) {
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    fail("invalid-shape");
  }
}

function parseStrictJSON(source) {
  let offset = 0;

  function skipWhitespace() {
    while (offset < source.length && /[\t\n\r ]/u.test(source[offset])) offset += 1;
  }

  function parseString() {
    if (source[offset] !== '"') fail("invalid-shape");
    const start = offset;
    offset += 1;
    while (offset < source.length) {
      const character = source[offset];
      if (character === '"') {
        offset += 1;
        try {
          return JSON.parse(source.slice(start, offset));
        } catch {
          fail("invalid-shape");
        }
      }
      if (character === "\\") {
        offset += 1;
        if (source[offset] === "u") {
          if (!/^[0-9a-fA-F]{4}$/u.test(source.slice(offset + 1, offset + 5))) fail("invalid-shape");
          offset += 5;
        } else {
          if (!/["\\/bfnrt]/u.test(source[offset] ?? "")) fail("invalid-shape");
          offset += 1;
        }
      } else {
        if (character.charCodeAt(0) < 0x20) fail("invalid-shape");
        offset += 1;
      }
    }
    fail("invalid-shape");
  }

  function parseArray() {
    const result = [];
    offset += 1;
    skipWhitespace();
    if (source[offset] === "]") {
      offset += 1;
      return result;
    }
    while (true) {
      result.push(parseValue());
      skipWhitespace();
      if (source[offset] === "]") {
        offset += 1;
        return result;
      }
      if (source[offset] !== ",") fail("invalid-shape");
      offset += 1;
      skipWhitespace();
    }
  }

  function parseObject() {
    const result = Object.create(null);
    const keys = new Set();
    offset += 1;
    skipWhitespace();
    if (source[offset] === "}") {
      offset += 1;
      return result;
    }
    while (true) {
      const key = parseString();
      if (keys.has(key)) fail("invalid-shape");
      keys.add(key);
      skipWhitespace();
      if (source[offset] !== ":") fail("invalid-shape");
      offset += 1;
      Object.defineProperty(result, key, {
        value: parseValue(), enumerable: true, writable: true, configurable: true,
      });
      skipWhitespace();
      if (source[offset] === "}") {
        offset += 1;
        return result;
      }
      if (source[offset] !== ",") fail("invalid-shape");
      offset += 1;
      skipWhitespace();
    }
  }

  function parseValue() {
    skipWhitespace();
    const character = source[offset];
    if (character === '"') return parseString();
    if (character === "{") return parseObject();
    if (character === "[") return parseArray();
    for (const [literal, value] of [["true", true], ["false", false], ["null", null]]) {
      if (source.startsWith(literal, offset)) {
        offset += literal.length;
        return value;
      }
    }
    const number = /^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/u.exec(source.slice(offset));
    if (!number) fail("invalid-shape");
    offset += number[0].length;
    const value = Number(number[0]);
    if (!Number.isFinite(value)) fail("invalid-shape");
    return value;
  }

  const value = parseValue();
  skipWhitespace();
  if (offset !== source.length) fail("invalid-shape");
  return value;
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function exactKeys(value, expected) {
  if (!value || Array.isArray(value) || typeof value !== "object" ||
      JSON.stringify(Object.keys(value).sort()) !== JSON.stringify([...expected].sort())) {
    fail("invalid-shape");
  }
}

function canonicalize(value) {
  if (value === null || typeof value === "boolean" || typeof value === "string") return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(canonicalize).join(",")}]`;
  if (!value || typeof value !== "object") fail("invalid-value");
  return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${canonicalize(value[key])}`).join(",")}}`;
}

function isIdentifier(value) {
  return typeof value === "string" && value !== "" && value.trim() === value &&
    !value.includes("*") && !value.includes("?") && !/[\p{Cc}\p{Cs}]/u.test(value);
}

function isPositiveBigIntString(value) {
  return typeof value === "string" && /^[1-9]\d*$/u.test(value) && BigInt(value) <= 9223372036854775807n;
}

function timestampNanos(value) {
  const match = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})\.(\d{9})Z$/u.exec(value);
  if (!match) fail("invalid-value");
  const millisecondsText = `${match[1]}.${match[2].slice(0, 3)}Z`;
  const milliseconds = Date.parse(millisecondsText);
  if (!Number.isFinite(milliseconds) || new Date(milliseconds).toISOString() !== millisecondsText) fail("invalid-value");
  return BigInt(milliseconds) * 1_000_000n + BigInt(match[2].slice(3));
}

const actions = new Set([
  "channel.archive", "capability_grant.issue", "capability_grant.revoke",
  "retention.expire", "retention.legal_hold.apply", "retention.legal_hold.release",
  "recovery.request", "recovery.decision", "recovery.commit", "outbox.replay.request",
]);
const outcomes = new Set(["succeeded", "denied", "failed"]);
const reasons = new Set([
  "authorized", "authorization_denied", "evidence_invalid", "policy_denied",
  "retention_expired", "state_conflict", "invalid_input", "internal_failure",
]);
const targetTypes = new Set(["channel", "capability_grant", "retention_subject", "recovery_case", "outbox_entry"]);
const forbiddenFieldNames = new Set([
  "messagePlaintext", "filePlaintext", "prompt", "modelResponse", "token",
  "capabilityGrant", "claimToken", "nonce", "signature", "privateKey",
  "publicKey", "recoveryEnvelope", "encryptedFieldCiphertext", "diagnosticText",
  "workspacePath", "filePath",
]);
const eventKeys = [
  "contractVersion", "auditEventId", "tenantId", "tenantSequence", "recordedAt",
  "principal", "action", "outcome", "reason", "target", "policyVersion", "requestId",
  "approvalId", "recoveryCaseId", "evidenceDigestHex", "previousEventHashHex", "eventHashHex",
];
const requiredMutationIds = [
  "contract-version", "event-id", "duplicate-event-id", "tenant", "invalid-unicode", "sequence-gap", "sequence-overflow",
  "recorded-at", "principal-id", "principal-type", "action", "unknown-action", "unknown-outcome", "unknown-reason",
  "unknown-target-type", "outcome", "reason",
  "target-type", "target-id", "target-version", "policy", "request", "approval", "recovery-case",
  "evidence-digest", "previous-hash", "event-hash", "delete-middle", "reorder", "cross-tenant",
  "wrong-genesis", "head-hash", "unknown-field", "disallowed-prompt-field",
];
const requiredReplayMutationIds = [
  "replay-tenant", "replay-principal", "replay-principal-type", "replay-entry", "replay-generation",
  "replay-policy", "approval-id", "approval-tenant", "approval-principal", "approval-principal-type",
  "approval-state", "approval-action", "approval-target-type", "approval-target", "approval-target-version",
  "approval-policy",
];

function rejectForbiddenFields(value) {
  if (Array.isArray(value)) {
    for (const entry of value) rejectForbiddenFields(entry);
    return;
  }
  if (!value || typeof value !== "object") return;
  for (const [key, child] of Object.entries(value)) {
    if (forbiddenFieldNames.has(key)) fail("minimized-data-violation");
    rejectForbiddenFields(child);
  }
}

function validateActor(actor) {
  exactKeys(actor, ["actorId", "actorType"]);
  if (!isIdentifier(actor.actorId) || !["1", "2", "3"].includes(actor.actorType)) fail("invalid-value");
}

function validateTarget(target) {
  exactKeys(target, ["targetType", "targetId", "targetVersion"]);
  if (!targetTypes.has(target.targetType)) fail("registry-unknown");
  if (!isIdentifier(target.targetId) ||
      (target.targetVersion !== null && !isPositiveBigIntString(target.targetVersion))) fail("invalid-value");
}

function validateEvent(event) {
  rejectForbiddenFields(event);
  exactKeys(event, eventKeys);
  for (const value of [event.auditEventId, event.tenantId, event.policyVersion, event.requestId]) {
    if (!isIdentifier(value)) fail("invalid-value");
  }
  if (event.contractVersion !== "1" || !isPositiveBigIntString(event.tenantSequence)) fail("invalid-value");
  timestampNanos(event.recordedAt);
  validateActor(event.principal);
  if (!actions.has(event.action) || !outcomes.has(event.outcome) || !reasons.has(event.reason)) fail("registry-unknown");
  validateTarget(event.target);
  for (const reference of [event.approvalId, event.recoveryCaseId]) {
    if (reference !== null && !isIdentifier(reference)) fail("invalid-value");
  }
  if (event.evidenceDigestHex !== null && !/^[0-9a-f]{64}$/u.test(event.evidenceDigestHex)) fail("invalid-value");
  if (!/^[0-9a-f]{64}$/u.test(event.previousEventHashHex) || !/^[0-9a-f]{64}$/u.test(event.eventHashHex)) fail("invalid-value");
  if (event.action === "channel.archive" && event.target.targetType !== "channel") fail("invalid-value");
  if (event.action.startsWith("capability_grant.") && event.target.targetType !== "capability_grant") fail("invalid-value");
  if (event.action.startsWith("retention.") && event.target.targetType !== "retention_subject") fail("invalid-value");
  if (event.action.startsWith("recovery.") &&
      (event.target.targetType !== "recovery_case" || event.recoveryCaseId !== event.target.targetId)) fail("invalid-value");
  if (event.action === "outbox.replay.request" &&
      (event.target.targetType !== "outbox_entry" || event.target.targetVersion === null)) fail("invalid-value");
  if (event.action === "outbox.replay.request" && event.outcome === "succeeded" &&
      (event.approvalId === null || event.evidenceDigestHex === null)) fail("invalid-value");
}

function integrityProjection(event) {
  validateEvent(event);
  const projection = structuredClone(event);
  delete projection.eventHashHex;
  return projection;
}

function eventHash(event) {
  return sha256(Buffer.from(`${transcriptPrefix}${canonicalize(integrityProjection(event))}`, "utf8"));
}

function replayEvidenceDigest(event, context) {
  const projection = {
    auditEventId: event.auditEventId,
    tenantId: context.tenantId,
    principal: context.principal,
    outboxEntryId: context.outboxEntryId,
    oldReplayGeneration: context.oldReplayGeneration,
    policyVersion: context.policyVersion,
    approval: context.approval,
  };
  return sha256(Buffer.from(`${replayEvidencePrefix}${canonicalize(projection)}`, "utf8"));
}

function verifyChain(chain) {
  rejectForbiddenFields(chain);
  exactKeys(chain, ["tenantId", "events", "head", "expectedReplayContext"]);
  if (!isIdentifier(chain.tenantId) || !Array.isArray(chain.events) || chain.events.length === 0) fail("invalid-shape");
  exactKeys(chain.head, ["tenantId", "lastSequence", "lastAuditEventId", "lastEventHashHex"]);
  let previousHash = zeroHash;
  let previousTime = null;
  const eventIDs = new Set();
  for (const [index, event] of chain.events.entries()) {
    validateEvent(event);
    if (event.tenantId !== chain.tenantId) fail("tenant-mismatch");
    if (eventIDs.has(event.auditEventId)) fail("chain-mismatch");
    eventIDs.add(event.auditEventId);
    if (event.tenantSequence !== String(index + 1) || event.previousEventHashHex !== previousHash) fail("chain-mismatch");
    const recordedAt = timestampNanos(event.recordedAt);
    if (previousTime !== null && recordedAt < previousTime) fail("chain-mismatch");
    if (eventHash(event) !== event.eventHashHex) fail("hash-mismatch");
    previousHash = event.eventHashHex;
    previousTime = recordedAt;
  }
  const last = chain.events.at(-1);
  if (chain.head.tenantId !== chain.tenantId) fail("tenant-mismatch");
  if (chain.head.lastSequence !== last.tenantSequence || chain.head.lastAuditEventId !== last.auditEventId ||
      chain.head.lastEventHashHex !== last.eventHashHex) fail("chain-mismatch");
}

function replayEvidenceResult(event, context) {
  try {
    validateEvent(event);
    if (eventHash(event) !== event.eventHashHex) fail("replay-evidence-invalid");
    exactKeys(context, [
      "tenantId", "principal", "outboxEntryId", "oldReplayGeneration", "policyVersion", "approval",
    ]);
    validateActor(context.principal);
    exactKeys(context.approval, [
      "approvalId", "tenantId", "authorizedPrincipal", "state", "action", "target", "policyVersion",
    ]);
    validateActor(context.approval.authorizedPrincipal);
    validateTarget(context.approval.target);
    for (const value of [
      context.tenantId, context.outboxEntryId, context.policyVersion,
      context.approval.approvalId, context.approval.tenantId, context.approval.policyVersion,
    ]) {
      if (!isIdentifier(value)) fail("replay-evidence-invalid");
    }
    if (!isPositiveBigIntString(context.oldReplayGeneration) || context.approval.state !== "approved" ||
        context.approval.action !== "outbox.replay.request" || event.action !== "outbox.replay.request" ||
        event.outcome !== "succeeded" || event.tenantId !== context.tenantId ||
        event.principal.actorId !== context.principal.actorId || event.principal.actorType !== context.principal.actorType ||
        event.target.targetType !== "outbox_entry" || event.target.targetId !== context.outboxEntryId ||
        event.target.targetVersion !== context.oldReplayGeneration || event.policyVersion !== context.policyVersion ||
        context.approval.tenantId !== context.tenantId ||
        context.approval.authorizedPrincipal.actorId !== context.principal.actorId ||
        context.approval.authorizedPrincipal.actorType !== context.principal.actorType ||
        context.approval.target.targetType !== "outbox_entry" ||
        context.approval.target.targetId !== context.outboxEntryId ||
        context.approval.target.targetVersion !== context.oldReplayGeneration ||
        context.approval.policyVersion !== context.policyVersion ||
        event.approvalId !== context.approval.approvalId ||
        event.evidenceDigestHex !== replayEvidenceDigest(event, context)) fail("replay-evidence-invalid");
    return { ok: true, code: null };
  } catch {
    return { ok: false, code: "replay-evidence-invalid" };
  }
}

function mutate(source, testCase) {
  const result = structuredClone(source);
  if (testCase.operation === "delete-event") {
    result.events.splice(testCase.index, 1);
    return result;
  }
  if (testCase.operation === "swap-events") {
    [result.events[testCase.left], result.events[testCase.right]] = [result.events[testCase.right], result.events[testCase.left]];
    return result;
  }
  const segments = testCase.path.split(".");
  let target = result;
  for (const segment of segments) target = target[segment];
  if (testCase.operation === "add-field") target[testCase.field] = testCase.value;
  else if (testCase.operation === "set") {
    target = result;
    for (const segment of segments.slice(0, -1)) target = target[segment];
    target[segments.at(-1)] = testCase.value;
  } else fail("invalid-shape");
  return result;
}

function setPath(source, path, value) {
  const result = structuredClone(source);
  const segments = path.split(".");
  let target = result;
  for (const segment of segments.slice(0, -1)) target = target[segment];
  target[segments.at(-1)] = value;
  return result;
}

function assertChainError(chain, expectedCode, id) {
  try {
    verifyChain(chain);
    throw new Error(`${id}: invalid Audit chain was accepted`);
  } catch (error) {
    if (!(error instanceof ContractError) || error.code !== expectedCode) {
      throw new Error(`${id}: got ${error.code ?? error.message}, want ${expectedCode}`);
    }
  }
}

const manifestBytes = readFileSync(join(fixtureRoot, "manifest.json"));
const sourceBytes = readFileSync(join(fixtureRoot, "scenarios.json"));
const manifest = parseStrictJSON(decodeUTF8(manifestBytes));
const vectors = parseStrictJSON(decodeUTF8(sourceBytes));

exactKeys(manifest, [
  "schemaVersion", "classification", "contract", "transcriptDomain", "registries", "source",
  "requiredRejectedMutationIds", "requiredRejectedReplayContextMutationIds", "requiredDisallowedFieldNames",
]);
exactKeys(manifest.source, ["file", "sha256"]);
exactKeys(manifest.registries, ["actions", "outcomes", "reasons", "targetTypes"]);
exactKeys(vectors, [
  "schemaVersion", "valid", "rejectedMutations", "rejectedReplayContextMutations", "disallowedFieldNames",
]);
assert(manifest.schemaVersion === 1, "Audit manifest schema must be v1");
assert(manifest.classification === "synthetic-public-audit-integrity-fixture-no-secrets",
  "Audit fixture classification changed");
assert(manifest.contract === "docs/architecture/audit-retention-contract.md", "Audit contract path changed");
assert(manifest.transcriptDomain === transcriptPrefix.trimEnd(), "Audit transcript domain changed");
assert(JSON.stringify(manifest.registries) === JSON.stringify({
  actions: [...actions], outcomes: [...outcomes], reasons: [...reasons], targetTypes: [...targetTypes],
}), "Audit registries changed");
assert(manifest.source.file === "scenarios.json" && /^[0-9a-f]{64}$/u.test(manifest.source.sha256),
  "Audit fixture source identity is invalid");
assert(sha256(sourceBytes) === manifest.source.sha256, "Audit fixture source hash mismatch");
assert(vectors.schemaVersion === 1, "Audit scenarios schema must be v1");
assert(JSON.stringify(vectors.disallowedFieldNames) === JSON.stringify(manifest.requiredDisallowedFieldNames),
  "Audit disallowed-field registry changed");
assert(JSON.stringify(vectors.disallowedFieldNames) === JSON.stringify([...forbiddenFieldNames]),
  "Audit verifier disallowed-field registry changed");

const mutationIds = vectors.rejectedMutations.map(({ id }) => id);
assert(new Set(mutationIds).size === mutationIds.length, "Audit mutation IDs must be unique");
assert(JSON.stringify(manifest.requiredRejectedMutationIds) === JSON.stringify(requiredMutationIds),
  "Audit manifest must retain every verifier-required mutation");
assert(JSON.stringify(mutationIds) === JSON.stringify(manifest.requiredRejectedMutationIds),
  "Audit rejected mutation coverage changed");
const replayMutationIds = vectors.rejectedReplayContextMutations.map(({ id }) => id);
assert(new Set(replayMutationIds).size === replayMutationIds.length, "Audit replay mutation IDs must be unique");
assert(JSON.stringify(manifest.requiredRejectedReplayContextMutationIds) === JSON.stringify(requiredReplayMutationIds),
  "Audit manifest must retain every verifier-required replay mutation");
assert(JSON.stringify(replayMutationIds) === JSON.stringify(manifest.requiredRejectedReplayContextMutationIds),
  "Audit replay mutation coverage changed");

verifyChain(vectors.valid);
const replayEvent = vectors.valid.events.find(({ action }) => action === "outbox.replay.request");
assert(replayEvidenceResult(replayEvent, vectors.valid.expectedReplayContext).ok,
  "valid tenant/Principal/Approval/Outbox replay evidence must be accepted");
const staleDigestEvent = structuredClone(replayEvent);
staleDigestEvent.evidenceDigestHex = "f".repeat(64);
staleDigestEvent.eventHashHex = eventHash(staleDigestEvent);
assert(replayEvidenceResult(staleDigestEvent, vectors.valid.expectedReplayContext).code === "replay-evidence-invalid",
  "rehashed Event with a stale replay evidence digest must fail closed");

for (const testCase of vectors.rejectedMutations) {
  if (testCase.operation === "set") exactKeys(testCase, ["id", "operation", "path", "value", "expectedCode"]);
  else if (testCase.operation === "delete-event") exactKeys(testCase, ["id", "operation", "index", "expectedCode"]);
  else if (testCase.operation === "swap-events") exactKeys(testCase, ["id", "operation", "left", "right", "expectedCode"]);
  else if (testCase.operation === "add-field") exactKeys(testCase, ["id", "operation", "path", "field", "value", "expectedCode"]);
  else fail("invalid-shape");
  assertChainError(mutate(vectors.valid, testCase), testCase.expectedCode, testCase.id);
}
for (const testCase of vectors.rejectedReplayContextMutations) {
  exactKeys(testCase, ["id", "path", "value"]);
  const result = replayEvidenceResult(replayEvent, setPath(vectors.valid.expectedReplayContext, testCase.path, testCase.value));
  assert(!result.ok && result.code === "replay-evidence-invalid",
    `${testCase.id}: replay evidence mismatch must fail with one stable category`);
}

try {
  parseStrictJSON('{"schemaVersion":1,"schemaVersion":2}');
  throw new Error("duplicate JSON key was accepted");
} catch (error) {
  assert(error instanceof ContractError && error.code === "invalid-shape", "duplicate JSON keys must fail closed");
}

for (const testCase of [
  { id: "raw-root-proto", source: '{"allowed":1,"__proto__":{"prompt":"canary"}}', nested: false },
  { id: "escaped-root-proto", source: '{"allowed":1,"\\u005f\\u005fproto__":{"prompt":"canary"}}', nested: false },
  { id: "raw-nested-proto", source: '{"allowed":{"__proto__":"canary"}}', nested: true },
  { id: "escaped-nested-proto", source: '{"allowed":{"\\u005f\\u005fproto__":"canary"}}', nested: true },
]) {
  try {
    const parsed = parseStrictJSON(testCase.source);
    exactKeys(parsed, ["allowed"]);
    if (testCase.nested) exactKeys(parsed.allowed, []);
    throw new Error(`${testCase.id}: prototype key was accepted`);
  } catch (error) {
    assert(error instanceof ContractError && error.code === "invalid-shape",
      `${testCase.id}: prototype keys must remain visible and fail closed`);
  }
}

for (const fieldName of forbiddenFieldNames) {
  const mutated = structuredClone(vectors.valid);
  mutated.events[0][fieldName] = "synthetic-protected-canary";
  assertChainError(mutated, "minimized-data-violation", `disallowed-${fieldName}`);
}

console.log("Threadline P03-07A Audit/Retention metadata contract is valid.");
