import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";

const root = new URL("../", import.meta.url);
const read = (path) => readFileSync(new URL(path, root), "utf8");
const fixtureRoot = "golden/domain-events-v1/";
const schema = read("threadline/outbox/v1/domain_event.proto");
const source = JSON.parse(read(`${fixtureRoot}envelope.json`));
const manifest = JSON.parse(read(`${fixtureRoot}manifest.json`));
const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");

// Test-only wire inspection: no runtime parser or authorization decisions.
function varint(bytes, start) {
  let value = 0n;
  for (let index = 0; index < 10; index += 1) {
    const byte = bytes[start + index];
    assert(byte !== undefined, "truncated varint");
    assert(index < 9 || byte <= 1, "overflow varint");
    value |= BigInt(byte & 127) << BigInt(index * 7);
    if (byte < 128) return { value, next: start + index + 1 };
  }
  throw new Error("overflow varint");
}

function fields(bytes) {
  const result = [];
  let offset = 0;
  while (offset < bytes.length) {
    const start = offset;
    const tag = varint(bytes, offset);
    const number = Number(tag.value >> 3n);
    const wire = Number(tag.value & 7n);
    assert(number > 0 && number <= 536870911, "invalid tag");
    const decoded = varint(bytes, tag.next);
    offset = decoded.next;
    let value = decoded.value;
    if (wire === 2) {
      assert(value <= BigInt(bytes.length - offset), "truncated bytes");
      offset += Number(value);
      value = bytes.subarray(decoded.next, offset);
    } else {
      assert.equal(wire, 0, "unexpected fixture wire type");
    }
    result.push({ number, wire, value, raw: bytes.subarray(start, offset) });
  }
  return result;
}

function one(values, number, wire) {
  const matches = values.filter((field) => field.number === number);
  assert.equal(matches.length, 1, `field ${number} count`);
  assert.equal(matches[0].wire, wire, `field ${number} wire`);
  return matches[0].value;
}

function verify(bytes) {
  const values = fields(bytes);
  assert.deepEqual(values.map((field) => field.number), [1, 2, 3, 4, 5, 6, 7, 8, 50000]);
  assert.equal(one(values, 1, 0), 1n);
  for (const [tag, name] of [[2, "tenantId"], [3, "eventId"], [4, "eventType"]]) {
    assert.equal(one(values, tag, 2).toString("utf8"), source[name]);
  }
  assert.equal(one(values, 5, 0), BigInt(source.schemaVersion));
  const timestamp = fields(one(values, 6, 2));
  assert.equal(one(timestamp, 1, 0), BigInt(Date.parse(source.occurredAt) / 1000));
  assert.deepEqual(one(values, 7, 2), Buffer.from(source.payload, "base64"));
  assert.equal(one(values, 8, 0), BigInt(source.outboxEntryId));
  assert.equal(one(values, 50000, 2).toString("utf8"), manifest.canaryUtf8);
  // An N-1 reader edits a known field while carrying every unknown raw field.
  const rewritten = values.map((field) => field.number === 8 ? Buffer.from([64, 2]) : field.raw);
  assert.equal(one(fields(Buffer.concat(rewritten)), 50000, 2).toString(), manifest.canaryUtf8);
  assert.equal(one(fields(Buffer.concat(rewritten)), 8, 0), 2n);
  assert.deepEqual(Buffer.concat(values.map((field) => field.raw)), bytes);
}

const expected = [
  ["uint32", "envelope_version", 1], ["string", "tenant_id", 2],
  ["string", "event_id", 3], ["string", "event_type", 4],
  ["uint32", "schema_version", 5], ["google.protobuf.Timestamp", "occurred_at", 6],
  ["bytes", "payload", 7], ["int64", "outbox_entry_id", 8],
];
const declared = [...schema.matchAll(/^\s*(\S+)\s+(\w+)\s*=\s*(\d+);/gmu)]
  .map((match) => [match[1], match[2], Number(match[3])]);
assert.deepEqual(declared, expected);
assert.match(schema, /reserved 50000;/u);
assert.equal(manifest.classification, "synthetic-protocol-compatibility-no-secrets");
const hex = read(`${fixtureRoot}envelope.hex`).trim();
assert.match(hex, /^(?:[0-9a-f]{2})+$/u);
const bytes = Buffer.from(hex, "hex");
assert.equal(sha256(bytes), manifest.sha256);
assert.equal(sha256(Buffer.from(read(`${fixtureRoot}envelope.json`))), manifest.sourceSha256);
verify(bytes);
assert.throws(() => verify(Buffer.concat([bytes, Buffer.from([8, 1])])));
assert.throws(() => verify(Buffer.from([8, 0])));
assert.throws(() => fields(Buffer.from([0, 0])));
assert.throws(() => fields(Buffer.from([10, 3, 1])));
assert.throws(() => varint(Buffer.alloc(10, 255), 0));
assert.throws(() => varint(Buffer.from([128]), 0));
console.log("Domain Event v1 schema, Golden bytes, unknown-field seam and malformed fixtures pass.");
