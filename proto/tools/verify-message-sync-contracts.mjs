import { createHash } from "node:crypto";
import { readFileSync, statSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const fixtureRoot = join(repositoryRoot, "test", "fixtures", "proto", "message");
const manifest = JSON.parse(readFileSync(join(fixtureRoot, "manifest.json"), "utf8"));
const sourceBytes = readFileSync(join(fixtureRoot, manifest.source.file));
const scenarios = JSON.parse(sourceBytes.toString("utf8"));

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function read(relativePath) {
  return readFileSync(join(repositoryRoot, relativePath), "utf8").replaceAll("\r\n", "\n");
}

function merge(base, overlay) {
  if (Array.isArray(overlay) || overlay === null || typeof overlay !== "object") return overlay;
  const result = { ...(base ?? {}) };
  for (const [key, value] of Object.entries(overlay)) {
    if (key === "extends") continue;
    result[key] = merge(base?.[key], value);
  }
  return result;
}

function reject(errorCode) {
  return { status: "rejected", errorCode, durableAck: false };
}

function evaluateSend(testCase) {
  const { session, conversation, envelope, transaction, priorCommit } = testCase;
  assert(!Object.hasOwn(envelope, "serverCommit"), `${testCase.id}: a sender must not supply ServerCommit`);

  if (!session.authorized) return reject("ERROR_CODE_PERMISSION_DENIED");
  if (envelope.tenantId !== session.tenantId || conversation.tenantId !== session.tenantId) {
    return reject("ERROR_CODE_TENANT_MISMATCH");
  }
  if (envelope.e2eeGroupId !== conversation.e2eeGroupId) return reject("ERROR_CODE_GROUP_MISMATCH");
  if (conversation.rekeyRequired && envelope.category === "EVENT_CATEGORY_APPLICATION") return reject("ERROR_CODE_REKEY_REQUIRED");
  if (envelope.epoch !== conversation.currentEpoch) return reject("ERROR_CODE_EPOCH_STALE");
  if (envelope.senderDeviceId !== session.deviceId || envelope.senderActorId !== session.actorId) {
    return reject("ERROR_CODE_PERMISSION_DENIED");
  }
  if (!envelope.signatureMetadataValid || !envelope.contentHashValid || !envelope.categoryBodyConsistent) {
    return reject("ERROR_CODE_CIPHERTEXT_CORRUPT");
  }
  if (priorCommit) {
    assert(priorCommit.idempotencyKey === envelope.idempotencyKey, `${testCase.id}: prior commit must exercise the same idempotency key`);
    if (priorCommit.contentHash !== envelope.contentHash) return reject("ERROR_CODE_IDEMPOTENCY_CONFLICT");
    return {
      status: "durable",
      eventId: priorCommit.eventId,
      idempotencyKey: priorCommit.idempotencyKey,
      channelSeq: priorCommit.channelSeq,
      deduplicated: true,
    };
  }
  if (!transaction.eventPersisted || !transaction.outboxPersisted || !transaction.synchronousCommit) {
    return reject("ERROR_CODE_NOT_DURABLE");
  }
  return {
    status: "durable",
    eventId: envelope.eventId,
    idempotencyKey: envelope.idempotencyKey,
    channelSeq: transaction.channelSeq,
    deduplicated: false,
  };
}

function evaluateResume(testCase) {
  const appliedSequences = [];
  let nextSeq = testCase.cursor.lastAppliedSeq;
  let gap = null;
  for (const sequence of testCase.deliveredSequences) {
    if (sequence !== nextSeq + 1) {
      gap = {
        firstMissingSeq: nextSeq + 1,
        lastKnownSeq: Math.max(...testCase.deliveredSequences),
      };
      break;
    }
    appliedSequences.push(sequence);
    nextSeq = sequence;
  }
  return { appliedSequences, nextSeq, gap };
}

function evaluateGapRepair(testCase) {
  assert(testCase.maxEventsPerGap > 0, `${testCase.id}: fixture must request an explicit positive repair bound`);
  const contiguous = [];
  let expected = testCase.gap.firstMissingSeq;
  for (const sequence of testCase.availableSequences) {
    if (sequence !== expected || contiguous.length >= testCase.maxEventsPerGap) break;
    contiguous.push(sequence);
    expected += 1;
  }
  assert(contiguous.length > 0, `${testCase.id}: recoverable fixture must return at least one event`);
  return {
    returnedSequences: contiguous,
    nextSeq: contiguous.at(-1),
    hasMore: contiguous.at(-1) < testCase.gap.lastKnownSeq,
    unrecoverable: false,
  };
}

function evaluateCursorRejection(testCase) {
  assert(testCase.cursor.channelId === testCase.serverCursor.channelId, `${testCase.id}: server cursor must name the same conversation`);
  if (testCase.invalidReason === "ahead") {
    assert(testCase.cursor.lastAppliedSeq > testCase.serverHeadSeq, `${testCase.id}: fixture must exercise an ahead cursor`);
  } else {
    throw new Error(`${testCase.id}: unsupported cursor rejection reason`);
  }
  return {
    errorCode: "ERROR_CODE_CURSOR_INVALID",
    resumeFromSeq: testCase.serverCursor.lastAppliedSeq,
  };
}

function evaluatePastRetention(testCase) {
  assert(testCase.cursor.channelId === testCase.serverCursor.channelId, `${testCase.id}: server cursor must name the same conversation`);
  assert(testCase.serverCursor.lastAppliedSeq === testCase.cursor.lastAppliedSeq, `${testCase.id}: server must not offer a forward cursor jump`);
  assert(testCase.cursor.lastAppliedSeq < testCase.retentionFloorSeq, `${testCase.id}: fixture must exercise a past-retention cursor`);
  const expectedGap = {
    firstMissingSeq: testCase.cursor.lastAppliedSeq + 1,
    lastKnownSeq: testCase.retentionFloorSeq - 1,
  };
  assert(JSON.stringify(testCase.repairRequest) === JSON.stringify(expectedGap), `${testCase.id}: repair must name the expired range exactly`);
  assert(JSON.stringify(testCase.repairResponse.unrecoverableGap) === JSON.stringify(expectedGap), `${testCase.id}: server must return the expired range as unrecoverable`);
  return {
    errorCode: "ERROR_CODE_CURSOR_INVALID",
    repairStartsAtSeq: testCase.retentionFloorSeq,
    cursorAdvances: false,
  };
}

function evaluateCheckpoint(testCase) {
  const matches = testCase.checkpoint.signatureValid
    && testCase.checkpoint.channelId === testCase.applied.channelId
    && testCase.checkpoint.channelSeq === testCase.applied.channelSeq
    && testCase.checkpoint.chainHash === testCase.applied.chainHash;
  return matches
    ? { status: "verified", cursorAdvances: true }
    : { status: "rejected", errorCode: "ERROR_CODE_CIPHERTEXT_CORRUPT", cursorAdvances: false };
}

function decodeVarint(bytes, start) {
  let value = 0n;
  let shift = 0n;
  let offset = start;
  while (offset < bytes.length && shift <= 63n) {
    const byte = bytes[offset];
    value |= BigInt(byte & 0x7f) << shift;
    offset += 1;
    if ((byte & 0x80) === 0) return { value, next: offset };
    shift += 7n;
  }
  throw new Error(`invalid varint at byte ${start}`);
}

function parseWire(bytes) {
  const fields = [];
  let offset = 0;
  while (offset < bytes.length) {
    const start = offset;
    const tag = decodeVarint(bytes, offset);
    offset = tag.next;
    const number = Number(tag.value >> 3n);
    const wireType = Number(tag.value & 7n);
    assert(number > 0, `invalid field number at byte ${start}`);
    if (wireType === 0) {
      offset = decodeVarint(bytes, offset).next;
    } else if (wireType === 1) {
      offset += 8;
    } else if (wireType === 2) {
      const length = decodeVarint(bytes, offset);
      offset = length.next + Number(length.value);
    } else if (wireType === 5) {
      offset += 4;
    } else {
      throw new Error(`unsupported wire type ${wireType} for field ${number}`);
    }
    assert(offset <= bytes.length, `truncated field ${number}`);
    fields.push({ number, wireType, raw: bytes.subarray(start, offset) });
  }
  return fields;
}

function evaluateUnknownExtension(testCase) {
  const fixturePath = resolve(fixtureRoot, testCase.fixture);
  const fixtureRelative = relative(repositoryRoot, fixturePath);
  assert(fixtureRelative !== "" && !fixtureRelative.startsWith("..") && !isAbsolute(fixtureRelative), `${testCase.id}: fixture escaped the repository`);
  const hex = readFileSync(fixturePath, "utf8").trim();
  assert(/^(?:[0-9a-f]{2})+$/u.test(hex), `${testCase.id}: Golden Frame must be canonical lowercase hex`);
  const bytes = Buffer.from(hex, "hex");
  const fields = parseWire(bytes);
  const unknown = fields.filter((field) => field.number === testCase.fieldNumber);
  assert(unknown.length === 1, `${testCase.id}: expected exactly one unknown extension`);
  assert(unknown[0].wireType === 2, `${testCase.id}: unknown extension must be length-delimited`);
  const tag = decodeVarint(unknown[0].raw, 0);
  const length = decodeVarint(unknown[0].raw, tag.next);
  const payload = unknown[0].raw.subarray(length.next, length.next + Number(length.value));
  assert(payload.toString("utf8") === testCase.payloadUtf8, `${testCase.id}: unknown extension payload mismatch`);
  assert(Buffer.concat(fields.map((field) => field.raw)).equals(bytes), `${testCase.id}: decode/re-encode lost wire bytes`);
  return { byteExactRoundTrip: true };
}

function evaluate(testCase) {
  if (testCase.kind === "send") return evaluateSend(testCase);
  if (testCase.kind === "resume") return evaluateResume(testCase);
  if (testCase.kind === "gap-repair") return evaluateGapRepair(testCase);
  if (testCase.kind === "cursor-rejection") return evaluateCursorRejection(testCase);
  if (testCase.kind === "past-retention") return evaluatePastRetention(testCase);
  if (testCase.kind === "checkpoint") return evaluateCheckpoint(testCase);
  if (testCase.kind === "unknown-extension") return evaluateUnknownExtension(testCase);
  throw new Error(`${testCase.id}: unknown case kind ${testCase.kind}`);
}

assert(manifest.schemaVersion === 1, "fixture manifest schemaVersion must be 1");
assert(manifest.classification === "synthetic-protocol-behavior-no-secrets", "fixture classification must remain synthetic and secret-free");
assert(JSON.stringify(manifest.reviewers) === JSON.stringify(["Contracts", "Security"]), "fixture review ownership must remain explicit");
assert(manifest.provenance.issue === 29 && manifest.provenance.generator === "none", "fixture provenance must remain bound to T015 and hand-authored inputs");
assert(manifest.allowedData.length > 0 && manifest.forbiddenData.length > 0, "fixture data policy must remain explicit");
assert(manifest.acceptance.physicalDevices === "NOT RUN", "T015 must not claim physical-device evidence");
for (const [name, value] of Object.entries(manifest.acceptance)) {
  if (name !== "physicalDevices") assert(value === true, `${name} acceptance must remain true`);
}
assert(sha256(sourceBytes) === manifest.source.sha256, "scenario source SHA-256 mismatch");
assert(scenarios.schemaVersion === manifest.schemaVersion, "scenario and manifest schema versions differ");
assert(scenarios.classification === manifest.classification, "scenario and manifest classifications differ");

const expectedContracts = [
  "threadline/identity/v1/identity.proto",
  "threadline/identity/v1/identity_service.proto",
  "threadline/channel/v1/channel.proto",
  "threadline/channel/v1/channel_service.proto",
  "threadline/message/v1/envelope.proto",
  "threadline/message/v1/message_service.proto",
  "threadline/sync/v1/sync.proto",
  "threadline/sync/v1/sync_service.proto",
];
assert(JSON.stringify(manifest.contracts) === JSON.stringify(expectedContracts), "fixture contract set must remain exact");
for (const contract of manifest.contracts) assert(statSync(join(repositoryRoot, "proto", contract)).isFile(), `missing contract ${contract}`);

const identityService = read("proto/threadline/identity/v1/identity_service.proto");
const channelService = read("proto/threadline/channel/v1/channel_service.proto");
const envelope = read("proto/threadline/message/v1/envelope.proto");
const messageService = read("proto/threadline/message/v1/message_service.proto");
const sync = read("proto/threadline/sync/v1/sync.proto");
const syncService = read("proto/threadline/sync/v1/sync_service.proto");
const errors = read("proto/threadline/type/v1/error.proto");

assert(identityService.includes("authenticated,\n// Device-bound session"), "IdentityService must bind tenant and Actor to the Device session");
assert(channelService.includes("ERROR_CODE_TENANT_MISMATCH"), "ChannelService must state its tenant mismatch boundary");
assert(/message\s+ServerCommit\s*\{/u.test(envelope), "ServerCommit contract is missing");
assert(/bytes\s+application_ciphertext\s*=\s*16;/u.test(envelope), "opaque application ciphertext field is missing");
assert(/bytes\s+sender_signature\s*=\s*24;/u.test(envelope), "sender signature field is missing");
assert(/string\s+idempotency_key\s*=\s*4;/u.test(messageService), "Durable ACK response must echo the idempotency key");
assert(/message\s+CursorRejection\s*\{/u.test(sync), "detailed cursor rejection contract is missing");
assert(/message\s+SyncEventsResponse\s*\{[^}]*repeated\s+CursorRejection\s+cursor_rejections\s*=\s*2;/su.test(syncService), "unary sync cursor rejections are missing");
assert(/message\s+StreamEventsResponse\s*\{[^}]*repeated\s+CursorRejection\s+cursor_rejections\s*=\s*2;/su.test(syncService), "streaming sync cursor rejections are missing");
assert(/uint32\s+max_events_per_gap\s*=\s*2;/u.test(syncService), "bounded gap repair field is missing");
assert(/repeated\s+CursorRejection\s+cursor_rejections\s*=\s*2;/u.test(syncService), "cursor rejection results are missing");
for (const code of [
  "ERROR_CODE_TENANT_MISMATCH",
  "ERROR_CODE_GROUP_MISMATCH",
  "ERROR_CODE_REKEY_REQUIRED",
  "ERROR_CODE_EPOCH_STALE",
  "ERROR_CODE_CIPHERTEXT_CORRUPT",
  "ERROR_CODE_IDEMPOTENCY_CONFLICT",
  "ERROR_CODE_CURSOR_INVALID",
  "ERROR_CODE_NOT_DURABLE",
]) {
  assert(errors.includes(code), `stable error vocabulary is missing ${code}`);
}

const rawCases = new Map(scenarios.cases.map((testCase) => [testCase.id, testCase]));
assert(rawCases.size === scenarios.cases.length, "scenario IDs must be unique");
assert(JSON.stringify([...rawCases.keys()].sort()) === JSON.stringify([...manifest.requiredCases].sort()), "scenario set must exactly match requiredCases");

const resolvedCases = new Map();
function resolveCase(id, resolving = new Set()) {
  if (resolvedCases.has(id)) return resolvedCases.get(id);
  assert(!resolving.has(id), `${id}: cyclic fixture inheritance`);
  const raw = rawCases.get(id);
  assert(raw, `${id}: missing fixture base`);
  resolving.add(id);
  const base = raw.extends ? resolveCase(raw.extends, resolving) : {};
  const resolved = merge(base, raw);
  // Expected results describe the complete observable outcome, so a derived
  // case replaces rather than inherits its base expectation.
  if (raw.expected) resolved.expected = raw.expected;
  resolving.delete(id);
  resolvedCases.set(id, resolved);
  return resolved;
}

for (const id of manifest.requiredCases) {
  const testCase = resolveCase(id);
  const actual = evaluate(testCase);
  assert(JSON.stringify(actual) === JSON.stringify(testCase.expected), `${id}: expected ${JSON.stringify(testCase.expected)}, got ${JSON.stringify(actual)}`);
  assert(!JSON.stringify(actual).includes(scenarios.forbiddenPlaintextCanary), `${id}: server-visible result leaked plaintext canary`);
  if (testCase.envelope) {
    assert(!JSON.stringify(testCase.envelope).includes(scenarios.forbiddenPlaintextCanary), `${id}: plaintext canary entered Ciphertext Envelope`);
  }
}

const goldenHex = readFileSync(join(repositoryRoot, manifest.goldenReference.file), "utf8").trim();
assert(sha256(Buffer.from(goldenHex, "hex")) === manifest.goldenReference.decodedSha256, "referenced T014 Golden Frame digest changed");
assert(manifest.goldenReference.unknownFieldNumber === 50000, "unknown extension must remain the T014 field 50000 canary");

console.log(`Verified ${manifest.requiredCases.length} synthetic T015 message/sync contract scenarios; physical devices NOT RUN.`);
