import { createHash } from "node:crypto";
import { readFileSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const protoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const goldenRoot = join(protoRoot, "golden", "v1");
const manifest = JSON.parse(readFileSync(join(goldenRoot, "manifest.json"), "utf8"));

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function readCanonicalHex(path) {
  const text = readFileSync(path, "utf8").trim();
  assert(/^(?:[0-9a-f]{2})+$/u.test(text), `${path} must contain canonical lowercase hex`);
  return { text, bytes: Buffer.from(text, "hex") };
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
    let value;
    if (wireType === 0) {
      const decoded = decodeVarint(bytes, offset);
      value = decoded.value;
      offset = decoded.next;
    } else if (wireType === 1) {
      assert(offset + 8 <= bytes.length, `truncated fixed64 field ${number}`);
      value = bytes.subarray(offset, offset + 8);
      offset += 8;
    } else if (wireType === 2) {
      const decodedLength = decodeVarint(bytes, offset);
      const length = Number(decodedLength.value);
      assert(Number.isSafeInteger(length), `unsafe length for field ${number}`);
      offset = decodedLength.next;
      assert(offset + length <= bytes.length, `truncated length-delimited field ${number}`);
      value = bytes.subarray(offset, offset + length);
      offset += length;
    } else if (wireType === 5) {
      assert(offset + 4 <= bytes.length, `truncated fixed32 field ${number}`);
      value = bytes.subarray(offset, offset + 4);
      offset += 4;
    } else {
      throw new Error(`unsupported wire type ${wireType} for field ${number}`);
    }
    fields.push({ number, wireType, value, raw: bytes.subarray(start, offset) });
  }
  return fields;
}

function values(fields, number, wireType) {
  const matches = fields.filter((field) => field.number === number);
  assert(matches.length > 0, `field ${number} is missing`);
  for (const field of matches) assert(field.wireType === wireType, `field ${number} has the wrong wire type`);
  return matches.map((field) => field.value);
}

function one(fields, number, wireType) {
  const matches = values(fields, number, wireType);
  assert(matches.length === 1, `field ${number} must occur exactly once`);
  return matches[0];
}

function assertUtf8(fields, number, expected) {
  assert(one(fields, number, 2).toString("utf8") === expected, `field ${number} UTF-8 value mismatch`);
}

function assertVarint(fields, number, expected) {
  assert(one(fields, number, 0) === BigInt(expected), `field ${number} varint value mismatch`);
}

function assertBytes(fields, number, expectedBase64) {
  assert(one(fields, number, 2).equals(Buffer.from(expectedBase64, "base64")), `field ${number} byte value mismatch`);
}

function assertTimestamp(bytes, expected) {
  const fields = parseWire(bytes);
  const seconds = BigInt(Math.floor(Date.parse(expected) / 1000));
  assertVarint(fields, 1, seconds);
  assert(!fields.some((field) => field.number !== 1 && field.number !== 2), "timestamp contains an unexpected field");
}

function assertCryptoProfile(bytes, expected) {
  const fields = parseWire(bytes);
  assertUtf8(fields, 1, expected.name);
  assertUtf8(fields, 2, expected.mlsProtocolVersion);
  assertVarint(fields, 3, 1);
  assertVarint(fields, 4, expected.messageEnvelopeVersion);
  assertVarint(fields, 5, expected.historyEnvelopeVersion);
  assertVarint(fields, 6, expected.recoveryEnvelopeVersion);
}

function assertRecoveryEnvelope(bytes, source, { canary } = {}) {
  const fields = parseWire(bytes);
  assertUtf8(fields, 1, source.tenantId);
  assertUtf8(fields, 2, source.e2eeGroupId);
  assertVarint(fields, 3, source.epoch);
  assertCryptoProfile(one(fields, 4, 2), source.cryptoProfile);
  assertVarint(fields, 5, source.recoveryKeyVersion);
  assertBytes(fields, 6, source.wrappedMaterial);
  assertVarint(fields, 7, source.envelopeVersion);
  assertVarint(fields, 8, source.recoveryRecipientPresent ? 1 : 0);
  assertUtf8(fields, 9, source.recoveryKeyId);
  assertBytes(fields, 10, source.bindingHash);
  if (canary) assertCanary(fields, canary);
  return fields;
}

function assertServerCommit(bytes, source) {
  const fields = parseWire(bytes);
  assertVarint(fields, 1, source.channelSeq);
  assertTimestamp(one(fields, 2, 2), source.committedAt);
  assertBytes(fields, 3, source.chainHash);
  assertBytes(fields, 4, source.previousChainHash);
}

function assertAgentAttribution(bytes, source) {
  const fields = parseWire(bytes);
  assertUtf8(fields, 1, source.agentActorId);
  assertUtf8(fields, 2, source.taskId);
  assertUtf8(fields, 3, source.runId);
  assertUtf8(fields, 4, source.capabilityGrantId);
}

function assertCanary(fields, canary) {
  const matches = fields.filter((field) => field.number === manifest.canaryFieldNumber);
  assert(matches.length === 1, `field ${manifest.canaryFieldNumber} must occur exactly once`);
  const field = matches[0];
  assert(field.wireType === 2, "Golden Frame canary must be length-delimited");
  assert(field.value.toString("utf8") === canary.payloadUtf8, "Golden Frame canary payload mismatch");
  const expectedRaw = readCanonicalHex(join(goldenRoot, canary.file)).bytes;
  assert(field.raw.equals(expectedRaw), "Golden Frame must preserve the exact canary bytes");
  assert(fields.at(-1) === field, "Golden Frame canary must be appended after known fields");
}

function assertFieldSet(fields, requiredKnownFields) {
  const actual = [...new Set(fields.filter((field) => field.number !== manifest.canaryFieldNumber).map((field) => field.number))].sort(
    (left, right) => left - right,
  );
  assert(JSON.stringify(actual) === JSON.stringify(requiredKnownFields), `known field set mismatch: ${actual.join(",")}`);
}

function assertChannelEventEnvelope(bytes, source, canary) {
  const fields = parseWire(bytes);
  assertUtf8(fields, 1, source.eventId);
  assertUtf8(fields, 2, source.tenantId);
  assertUtf8(fields, 3, source.channelId);
  assertVarint(fields, 6, source.epoch);
  assertUtf8(fields, 5, source.e2eeGroupId);
  assertServerCommit(one(fields, 7, 2), source.serverCommit);
  assertVarint(fields, 8, 1);
  assertUtf8(fields, 9, source.senderDeviceId);
  assertUtf8(fields, 10, source.senderActorId);
  assertUtf8(fields, 11, source.idempotencyKey);
  assertTimestamp(one(fields, 12, 2), source.clientSentAt);
  assertUtf8(fields, 14, source.cryptoProfile);
  assertVarint(fields, 15, source.envelopeVersion);
  assertBytes(fields, 16, source.applicationCiphertext);
  assertBytes(fields, 18, source.contentHash);
  assertRecoveryEnvelope(one(fields, 20, 2), source.recoveryEnvelope);
  const attachments = values(fields, 22, 2).map((value) => value.toString("utf8"));
  assert(JSON.stringify(attachments) === JSON.stringify(source.attachmentBlobIds), "attachment field mismatch");
  assertAgentAttribution(one(fields, 23, 2), source.agentAttribution);
  assertBytes(fields, 24, source.senderSignature);
  assertCanary(fields, canary);
  return fields;
}

assert(manifest.schemaVersion === 2, "Golden Frame manifest schemaVersion must be 2");
assert(manifest.canaryFieldNumber === 50000, "Golden Frame canary must remain field 50000");
assert(manifest.classification === "synthetic-protocol-compatibility-no-secrets", "fixture classification must remain synthetic and secret-free");
assert(manifest.canaries.length === 2, "exactly two contract-family canaries are required");
assert(manifest.frames.length === 2, "exactly two representative persisted-envelope frames are required");

const canaries = new Map();
for (const canary of manifest.canaries) {
  const fixture = readCanonicalHex(join(goldenRoot, canary.file));
  assert(sha256(fixture.bytes) === canary.sha256, `${canary.contract} canary SHA-256 mismatch`);
  const fields = parseWire(fixture.bytes);
  assertCanary(fields, canary);
  canaries.set(canary.contract, canary);
}

for (const frame of manifest.frames) {
  assert(statSync(join(protoRoot, frame.schemaFile)).isFile(), `${frame.contract} schema file is missing`);
  const schema = readFileSync(join(protoRoot, frame.schemaFile), "utf8");
  assert(new RegExp(`message\\s+${frame.contract.split(".").at(-1)}\\s*\\{`, "u").test(schema), `${frame.contract} message is missing`);
  assert(new RegExp(`reserved\\s+${manifest.canaryFieldNumber}\\s*;`, "u").test(schema), `${frame.contract} must reserve the canary field`);

  const sourceBytes = readFileSync(join(goldenRoot, frame.sourceJson));
  assert(sha256(sourceBytes) === frame.sourceSha256, `${frame.contract} source JSON SHA-256 mismatch`);
  const source = JSON.parse(sourceBytes.toString("utf8"));
  const fixture = readCanonicalHex(join(goldenRoot, frame.file));
  assert(sha256(fixture.bytes) === frame.sha256, `${frame.contract} frame SHA-256 mismatch`);
  const canary = canaries.get(frame.canary);
  assert(canary, `${frame.contract} references an unknown canary`);

  const fields =
    frame.contract === "threadline.message.v1.ChannelEventEnvelope"
      ? assertChannelEventEnvelope(fixture.bytes, source, canary)
      : assertRecoveryEnvelope(fixture.bytes, source, { canary });
  assertFieldSet(fields, frame.requiredKnownFields);
  assert(Buffer.concat(fields.map((field) => field.raw)).equals(fixture.bytes), `${frame.contract} wire parser did not retain every field byte`);
}

console.log("Representative ChannelEventEnvelope and RecoveryEnvelope Golden Frames are valid.");
