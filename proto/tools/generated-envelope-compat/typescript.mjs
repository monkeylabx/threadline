import { readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
import { join, resolve } from "node:path";
import { fromBinary, toBinary } from "@bufbuild/protobuf";

const [mode, generatedRoot, inputRoot, outputRoot, expectedLabel, outputLabel] = process.argv.slice(2);
if (!new Set(["produce", "relay", "consume"]).has(mode) || !generatedRoot || !inputRoot) {
  throw new Error("usage: typescript.mjs produce|relay|consume GENERATED_ROOT INPUT_ROOT [OUTPUT_ROOT EXPECTED_LABEL OUTPUT_LABEL]");
}

const messageModule = await import(pathToFileURL(resolve(generatedRoot, "threadline/message/v1/envelope_pb.js")));
const recoveryModule = await import(pathToFileURL(resolve(generatedRoot, "threadline/crypto/v1/recovery_pb.js")));

function hexFile(path) {
  return Uint8Array.from(Buffer.from(readFileSync(path, "utf8").trim(), "hex"));
}

function wireFile(path) {
  return Uint8Array.from(readFileSync(path));
}

function contains(haystack, needle) {
  return Buffer.from(haystack).includes(Buffer.from(needle));
}

const repositoryRoot = resolve(import.meta.dirname, "../../..");
const goldenRoot = join(repositoryRoot, "proto/golden/v1");
const channelCanary = hexFile(join(goldenRoot, "ciphertext-envelope.canary.hex"));
const recoveryCanary = hexFile(join(goldenRoot, "crypto-envelope.canary.hex"));
const sourceIsGolden = mode === "produce";
const channelInput = sourceIsGolden
  ? hexFile(join(inputRoot, "channel-event-envelope.golden.hex"))
  : wireFile(join(inputRoot, "channel.bin"));
const recoveryInput = sourceIsGolden
  ? hexFile(join(inputRoot, "recovery-envelope.golden.hex"))
  : wireFile(join(inputRoot, "recovery.bin"));

function transform(schema, input, canary, field, expected, replacement) {
  const decoded = fromBinary(schema, input);
  if (!contains(input, canary)) throw new Error(`${field}: input dropped the exact field-50000 canary`);
  if (expected && decoded[field] !== expected) {
    throw new Error(`${field}: expected ${expected}; got ${decoded[field]}`);
  }
  if (mode === "consume") return;
  decoded[field] = replacement;
  const encoded = toBinary(schema, decoded);
  if (!contains(encoded, canary)) throw new Error(`${field}: generated adapter dropped the exact field-50000 canary`);
  const checked = fromBinary(schema, encoded);
  if (checked[field] !== replacement) throw new Error(`${field}: known-field mutation was not preserved`);
  return encoded;
}

const channelOutput = transform(
  messageModule.ChannelEventEnvelopeSchema,
  channelInput,
  channelCanary,
  "eventId",
  sourceIsGolden ? "evt-golden-0001" : expectedLabel,
  outputLabel,
);
const recoveryOutput = transform(
  recoveryModule.RecoveryEnvelopeSchema,
  recoveryInput,
  recoveryCanary,
  "recoveryKeyId",
  sourceIsGolden ? "recovery-key-golden-v7" : expectedLabel,
  outputLabel,
);

if (mode !== "consume") {
  if (!outputRoot || !outputLabel) throw new Error(`${mode} requires OUTPUT_ROOT and OUTPUT_LABEL`);
  mkdirSync(outputRoot, { recursive: true });
  writeFileSync(join(outputRoot, "channel.bin"), channelOutput);
  writeFileSync(join(outputRoot, "recovery.bin"), recoveryOutput);
}

console.log(`TypeScript ${mode} passed for ${expectedLabel || "Golden Frames"}${outputLabel ? ` -> ${outputLabel}` : ""}.`);
