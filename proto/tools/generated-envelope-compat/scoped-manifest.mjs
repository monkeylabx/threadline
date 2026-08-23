import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { dirname, isAbsolute, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
const completeLanguages = ["go", "typescript", "rust", "kotlin", "swift"];

export function repositoryFile(path) {
  const resolved = resolve(repositoryRoot, path);
  const repositoryRelative = relative(repositoryRoot, resolved);
  if (repositoryRelative.startsWith("..") || isAbsolute(repositoryRelative)) {
    throw new Error(`fixture path escapes repository: ${path}`);
  }
  if (!existsSync(resolved)) throw new Error(`fixture path is missing: ${path}`);
  return resolved;
}

function sha256(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
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

export function parseWire(bytes) {
  const fields = [];
  let offset = 0;
  while (offset < bytes.length) {
    const start = offset;
    const tag = decodeVarint(bytes, offset);
    offset = tag.next;
    const number = Number(tag.value >> 3n);
    const wireType = Number(tag.value & 7n);
    if (number < 1) throw new Error(`invalid field number at byte ${start}`);
    let value;
    if (wireType === 0) {
      const decoded = decodeVarint(bytes, offset);
      value = decoded.value;
      offset = decoded.next;
    } else if (wireType === 1) {
      value = bytes.subarray(offset, offset + 8);
      offset += 8;
    } else if (wireType === 2) {
      const length = decodeVarint(bytes, offset);
      offset = length.next;
      value = bytes.subarray(offset, offset + Number(length.value));
      offset += Number(length.value);
    } else if (wireType === 5) {
      value = bytes.subarray(offset, offset + 4);
      offset += 4;
    } else {
      throw new Error(`unsupported wire type ${wireType} for field ${number}`);
    }
    if (offset > bytes.length) throw new Error(`truncated field ${number}`);
    fields.push({ number, wireType, value, raw: bytes.subarray(start, offset) });
  }
  return fields;
}

export function oneField(fields, number) {
  const matches = fields.filter((field) => field.number === number);
  if (matches.length !== 1) throw new Error(`field ${number} must occur exactly once`);
  return matches[0];
}

export function loadScopedGeneratedCompatManifest(manifestArgument, expected = {}) {
  const manifestPath = repositoryFile(manifestArgument);
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  if (manifest.schemaVersion !== 1 || manifest.classification !== "synthetic-protocol-compatibility-no-secrets") {
    throw new Error("scoped frame manifest metadata is invalid");
  }
  if (expected.baseline && manifest.baselineCommit !== expected.baseline) {
    throw new Error("baseline does not match the scoped manifest");
  }
  if (expected.target && manifest.targetSchemaCommit !== expected.target) {
    throw new Error("target does not match the scoped manifest");
  }
  if (JSON.stringify(manifest.adapterInputs.languages) !== JSON.stringify(completeLanguages)) {
    throw new Error("scoped manifest must retain the complete five-language adapter set");
  }
  if (manifest.acceptance.generatedFiveLanguageNMinusOne !== "PENDING_INTEGRATION") {
    throw new Error("scoped task must leave generatedFiveLanguageNMinusOne PENDING_INTEGRATION");
  }
  if (manifest.acceptance.physicalDevices !== "NOT RUN") {
    throw new Error("scoped task must leave physical devices NOT RUN");
  }
  if (JSON.stringify(manifest.failClosedEvidence.legacyWire.generatedAdapters) !== JSON.stringify(completeLanguages)
    || JSON.stringify(manifest.failClosedEvidence.legacyWire.absentCurrentFields) !== JSON.stringify([11, 12, 13, 14, 15, 16, 17, 18])) {
    throw new Error("legacy wire evidence must cover all adapters and absent current fields 11-18");
  }
  for (const [path, expectedSha256] of [
    [manifest.frame.sourceJson, manifest.frame.sourceSha256],
    [manifest.frame.file, manifest.frame.fileSha256],
    [manifest.frame.canary.file, manifest.frame.canary.fileSha256],
    [manifest.historicalT014RecoveryFrame.file, manifest.historicalT014RecoveryFrame.sha256],
    [manifest.targetSchema.file, manifest.targetSchema.sha256],
  ]) {
    if (sha256(repositoryFile(path)) !== expectedSha256) {
      throw new Error(`scoped fixture SHA-256 mismatch: ${path}`);
    }
  }
  const encoded = Buffer.from(readFileSync(repositoryFile(manifest.frame.file), "utf8").trim(), "hex");
  if (createHash("sha256").update(encoded).digest("hex") !== manifest.frame.decodedSha256) {
    throw new Error("scoped RecoveryEnvelope decoded SHA-256 mismatch");
  }
  const fields = parseWire(encoded);
  const actualKnown = fields.filter((field) => field.number !== manifest.frame.unknownFieldNumber).map((field) => field.number);
  if (JSON.stringify(actualKnown) !== JSON.stringify(manifest.frame.requiredKnownFields)) {
    throw new Error(`scoped RecoveryEnvelope known fields mismatch: ${actualKnown.join(",")}`);
  }
  if (oneField(fields, manifest.frame.unknownFieldNumber).raw.toString("hex") !== manifest.frame.canary.hex) {
    throw new Error("scoped RecoveryEnvelope canary bytes mismatch");
  }
  if (oneField(fields, 1).value.toString("utf8") !== manifest.frame.expectedTenantId) {
    throw new Error("scoped RecoveryEnvelope Tenant mismatch");
  }
  if (oneField(fields, 2).value.toString("utf8") !== manifest.frame.expectedGroupId) {
    throw new Error("scoped RecoveryEnvelope Group mismatch");
  }
  if (oneField(parseWire(oneField(fields, 4).value), 1).value.toString("utf8") !== manifest.frame.expectedProfile) {
    throw new Error("scoped RecoveryEnvelope Crypto Profile mismatch");
  }
  return { encoded, fields, manifest, manifestPath };
}
