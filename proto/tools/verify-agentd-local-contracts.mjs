import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { isDeepStrictEqual } from "node:util";

import { decisions } from "./agentd-local-reference.mjs";

const root = fileURLToPath(new URL("../../", import.meta.url));
const fixtureRoot = resolve(process.argv[2] ?? join(root, "test/fixtures/proto/agentd-local"));
const raw = readFileSync(join(fixtureRoot, "scenarios.json"));
const wireRaw = readFileSync(join(fixtureRoot, "wire.json"));
const fixture = JSON.parse(raw);
const wire = JSON.parse(wireRaw);
const manifest = JSON.parse(readFileSync(join(fixtureRoot, "manifest.json"), "utf8"));
const errors = [];

function check(condition, message) {
  if (!condition) errors.push(message);
}

function materialize(base, overrides = {}) {
  const facts = structuredClone(base);
  for (const [section, changes] of Object.entries(overrides)) {
    check(Object.hasOwn(facts, section), `unknown fixture section: ${section}`);
    if (!Object.hasOwn(facts, section)) continue;
    for (const key of Object.keys(changes)) {
      const generatedText = (section === "request" || section === "activity") && (key === "textRepeat" || key === "textUnit");
      check(Object.hasOwn(facts[section], key) || generatedText, `unknown fixture fact: ${section}.${key}`);
    }
    Object.assign(facts[section], changes);
  }
  for (const section of ["request", "activity"]) {
    const factsSection = facts[section];
    if (factsSection.textRepeat === undefined) continue;
    factsSection.text = (factsSection.textUnit ?? "x").repeat(factsSection.textRepeat);
  }
  return facts;
}

const digest = createHash("sha256").update(raw).digest("hex");
const wireDigest = createHash("sha256").update(wireRaw).digest("hex");
check(manifest.schemaVersion === 1 && fixture.schemaVersion === 1, "fixture schema must be v1");
check(wire.schemaVersion === 1, "wire fixture schema must be v1");
check(manifest.scenariosSha256 === digest, "fixture SHA-256 differs from manifest");
check(manifest.wireSha256 === wireDigest, "wire fixture SHA-256 differs from manifest");
check(manifest.classification === "synthetic-no-secrets", "fixture classification must be synthetic-no-secrets");
check(manifest.runtimeEvidence === "NOT RUN", "fixture must not claim runtime evidence");
check(Array.isArray(fixture.cases) && fixture.cases.length >= 40, "expected the full local contract case matrix");
check(isDeepStrictEqual(wire.frames.map((frame) => frame.type), [
  "SubmitRunInputRequest", "SubmitRunInputResponse", "WatchRunActivityRequest",
  "WatchRunActivityResponse", "WatchRunActivityResponse",
  "RequestRunStopRequest", "RequestRunStopResponse",
]), "wire fixture must cover all local request/response types and both watch variants");
const seen = new Set();
for (const scenario of fixture.cases) {
  check(typeof scenario.id === "string" && !seen.has(scenario.id), `duplicate or missing case ID: ${scenario.id}`);
  seen.add(scenario.id);
  const decide = decisions[scenario.operation];
  check(typeof decide === "function", `${scenario.id}: unknown operation`);
  if (decide) check(decide(materialize(fixture.base, scenario.overrides)) === scenario.expected, `${scenario.id}: expected ${scenario.expected}`);
}
for (const id of manifest.requiredCases) check(seen.has(id), `required case missing: ${id}`);

const proto = readFileSync(join(root, "proto/threadline/local_agent/v1/agent_service.proto"), "utf8");
const sharedErrors = readFileSync(join(root, "proto/threadline/type/v1/error.proto"), "utf8");
for (const rpc of ["SubmitRunInput", "WatchRunActivity", "RequestRunStop"]) {
  check(new RegExp(`\\brpc ${rpc}\\(`, "u").test(proto), `local service lacks ${rpc}`);
}
check((proto.match(/\brpc /gu) ?? []).length === 3, "local service must contain exactly three RPCs");
check(/rpc WatchRunActivity\([^)]*\) returns \(stream WatchRunActivityResponse\)/u.test(proto), "watch must server-stream");
check(/message SubmitRunInputRequest \{[^}]*string run_id = 1;[^}]*string input_id = 2;[^}]*string text = 3;/su.test(proto), "input request field surface changed");
check(/message WatchRunActivityRequest \{[^}]*string run_id = 1;[^}]*string stream_id = 2;[^}]*uint64 after_sequence = 3;/su.test(proto), "watch request field surface changed");
check(/message RequestRunStopRequest \{[^}]*string run_id = 1;/su.test(proto), "stop request field surface changed");
check(/message LocalToolActivity \{[^}]*string label = 1;[^}]*LocalToolPhase phase = 2;[^}]*\}/su.test(proto), "tool view must remain label and phase only");
check(/message WatchRunActivityResponse \{[^}]*string run_id = 1;[^}]*string stream_id = 2;[^}]*uint64 sequence = 3;[^}]*string input_id = 4;[^}]*oneof view \{[^}]*ActivityStreamReady ready = 5;[^}]*string agent_text = 6;[^}]*LocalToolActivity tool = 7;/su.test(proto), "activity view surface changed");
check(/ERROR_CODE_RUN_INPUT_OUTCOME_UNCERTAIN = 45;/u.test(sharedErrors), "uncertain input error must have additive code 45");

function convert(type, from, to, payload) {
  const result = spawnSync(join(root, "node_modules/.bin/buf"), [
    "convert", "proto", `--type=threadline.local_agent.v1.${type}`,
    `--from=-#format=${from}`, `--to=-#format=${to}`,
  ], { cwd: root, input: payload, maxBuffer: 1024 * 1024 });
  const failure = result.stderr?.toString("utf8").trim() || result.error?.message || `exit ${result.status}`;
  check(result.status === 0, `${type} ${from}->${to}: ${failure}`);
  return result.status === 0 ? result.stdout : null;
}

for (const frame of wire.frames) {
  const encoded = convert(frame.type, "json", "binpb", JSON.stringify(frame.json));
  if (!encoded) continue;
  check(encoded.toString("hex") === frame.hex, `${frame.type} binary encoding differs from fixture`);
  const decoded = convert(frame.type, "binpb", "json", Buffer.from(frame.hex, "hex"));
  if (decoded) check(isDeepStrictEqual(JSON.parse(decoded.toString("utf8")), frame.json), `${frame.type} JSON roundtrip differs from fixture`);
}

if (errors.length) {
  console.error(errors.map((error) => `- ${error}`).join("\n"));
  process.exit(1);
}
console.log(`Local agentd contract: ${fixture.cases.length} synthetic cases, ${wire.frames.length} wire frames and Proto surface valid.`);
