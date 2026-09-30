import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { isDeepStrictEqual } from "node:util";

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

function bytes(value) {
  return Buffer.byteLength(value, "utf8");
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

function authorization(facts, role) {
  const { principal, binding, request, run } = facts;
  if (!principal.present || !principal.active) return "UNAUTHENTICATED";
  if (principal.tenant !== run.tenant) return "ERROR_CODE_TENANT_MISMATCH";
  if (principal.actor !== run.authorizedActor || principal.device !== run.authorizedDevice) return "PERMISSION_DENIED";
  if (principal.window !== binding.window || request.run !== run.id) return "PERMISSION_DENIED";
  if (!run[role]) return "PERMISSION_DENIED";
  return null;
}

function input(facts) {
  const denied = authorization(facts, "control");
  if (denied) return denied;
  const { request, run, intake } = facts;
  if (!run.owner) return "ERROR_CODE_NOT_EXECUTION_OWNER";
  if (!run.lease) return "ERROR_CODE_LEASE_LOST";
  if (!run.fencing) return "ERROR_CODE_FENCING_TOKEN_STALE";
  if (!run.grant) return "ERROR_CODE_GRANT_REVOKED";
  if (!run.live) return "ERROR_CODE_INVALID_STATE_TRANSITION";
  if (!/^[\x21-\x7e]{1,128}$/u.test(request.inputId)) return "INVALID_ARGUMENT";
  if (bytes(request.text) === 0) return "INVALID_ARGUMENT";
  if (bytes(request.text) > 65536) return "ERROR_CODE_PAYLOAD_TOO_LARGE";
  if (intake.uncertain) return "ERROR_CODE_RUN_INPUT_OUTCOME_UNCERTAIN";
  if (intake.prior !== null) return intake.prior === request.text ? "DUPLICATE" : "ERROR_CODE_IDEMPOTENCY_CONFLICT";
  if (intake.activeTurn) return "ERROR_CODE_INVALID_STATE_TRANSITION";
  return "ACCEPTED";
}

function watch(facts) {
  const denied = authorization(facts, "observe");
  if (denied) return denied;
  const { run, request, activity } = facts;
  if (!run.grant) return "ERROR_CODE_GRANT_REVOKED";
  if (!request.stream) return request.after === 0 ? "WATCH" : "ERROR_CODE_CURSOR_INVALID";
  if (request.stream !== activity.stream || request.after > activity.latest) return "ERROR_CODE_CURSOR_INVALID";
  if (request.after + 1 < activity.first) return "ERROR_CODE_SEQUENCE_GAP";
  return "WATCH";
}

function stop(facts) {
  const denied = authorization(facts, "control");
  if (denied) return denied;
  if (!facts.run.owner) return "ERROR_CODE_NOT_EXECUTION_OWNER";
  return "STOP_RECEIPT";
}

function output(facts) {
  const { activity } = facts;
  if (authorization(facts, "observe") || !facts.run.grant) return "DROP";
  if (!activity.authorized) return "DROP";
  if (activity.kind !== "agent_text" && activity.kind !== "tool_label") return "DROP";
  const limit = activity.kind === "agent_text" ? 8192 : 128;
  if (bytes(activity.text) > limit) return "ERROR_CODE_PAYLOAD_TOO_LARGE";
  return activity.kind === "agent_text" ? "TEXT" : "TOOL_LABEL";
}

function approval(facts) {
  const { approval: decision, run } = facts;
  const trusted = decision.source === "core" && decision.actionMatches && decision.active;
  return trusted && !authorization(facts, "control") && !decision.runCancelled && run.live && run.owner && run.grant && run.lease && run.fencing
    ? "ALLOW_PROTECTED_EFFECT" : "DENY_PROTECTED_EFFECT";
}

const decisions = { input, watch, stop, output, approval };
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
