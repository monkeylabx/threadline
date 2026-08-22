import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const fixtureRoot = dirname(fileURLToPath(import.meta.url));
const repositoryRoot = resolve(fixtureRoot, "../..");
const manifest = JSON.parse(readFileSync(join(fixtureRoot, "e2ee-interop-v1.manifest.json"), "utf8"));
const vectorBytes = readFileSync(join(fixtureRoot, "e2ee-interop-v1.vector"));
const pinnedSourceCommit = "2b39382a1b0a050e466aa3418c468cb3cccc8939";
const pinnedVectorSha256 = "b9ce8eec6ccab6e5e200159c4a5bac73527ad8ebd929b21cb1f0447ab332ff40";
const pinnedKeys = [
  "format_version", "profile", "protocol", "ciphersuite", "mls_library", "mls_library_version", "vector_class",
  "key_package.expected", "epoch.initial", "epoch.after_add", "epoch.after_update_1", "epoch.after_update_2",
  "epoch.after_offline_join", "epoch.after_revoke", "wire_format.handshake", "leaf_lifetime.not_before_skew_seconds",
  "leaf_lifetime.max_range_seconds", "interop.independent_implementation", "interop.independent_implementation_version",
  "offline_new_device.expected", "out_of_order_commit.expected", "device_revocation.expected", "history.authorized",
  "history.unauthorized", "history.cross_tenant", "recovery.wrapper", "recovery.private_key_location", "recovery.version",
  "recovery.tenant_id", "recovery.group_id", "recovery.epoch", "recovery.recipient_key_id", "recovery.payload_sha256",
  "recovery.binding_sha256", "recovery.success", "recovery.no_recipient", "recovery.refused", "recovery.corrupt",
  "recovery.old_epoch", "recovery.cross_group", "recovery.cross_tenant", "error.replay", "error.corrupt",
  "error.old_epoch", "error.future_epoch", "error.unknown_version", "output.classification",
];
const fixturePaths = [
  "test/crypto/e2ee-interop-v1.vector",
  "test/crypto/e2ee-interop-v1.manifest.json",
  "test/crypto/README.md",
  "test/crypto/verify-e2ee-interop-fixture.mjs",
];

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function scanPublicMetadata(bytes, label) {
  const source = bytes.toString("utf8");
  const prohibited = [
    [/-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/u, "private-key block"],
    [/\bBearer\s+[A-Za-z0-9._~+/-]{16,}={0,2}\b/u, "bearer token"],
    [/\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/u, "JWT"],
    [/\b(?:github_pat_|gh[pousr]_|xox[baprs]-|sk_live_|pk_live_|AKIA|ASIA|AIza)[A-Za-z0-9_-]{8,}\b/u, "provider token"],
    [/(?:password|passwd|access[_-]?token|client[_-]?secret)\s*[:=]\s*["']?[A-Za-z0-9._~+/-]{8,}/iu, "credential-shaped assignment"],
    [/(?:^|[\s"'(])\/(?:Users|home|tmp|private|var|Volumes|opt|srv|mnt)\/[^\s"')]+/mu, "absolute local path"],
    [/[A-Za-z]:\\(?:Users|Temp|Windows|ProgramData)\\[^\s"']+/u, "absolute Windows path"],
    [/https?:\/\/[^\s/@:]+:[^\s/@]+@/u, "credential-bearing URL"],
    [/\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b/iu, "e-mail address"],
    [/(?:^|\D)\+?[1-9]\d{0,2}[ .-]?(?:\(\d{2,4}\)|\d{2,4})[ .-]?\d{3,4}[ .-]?\d{4}(?:\D|$)/u, "phone number"],
    [/\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b/iu, "UUID or device identifier"],
    [/\b(?:10|127)\.\d{1,3}\.\d{1,3}\.\d{1,3}\b|\b192\.168\.\d{1,3}\.\d{1,3}\b|\b172\.(?:1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3}\b/u, "private network address"],
  ];
  for (const [pattern, description] of prohibited) assert(!pattern.test(source), `${label}: prohibited ${description}`);
}

function git(args) {
  const result = spawnSync("git", args, {
    cwd: repositoryRoot,
    encoding: null,
    env: { PATH: process.env.PATH ?? "", LANG: "C", LC_ALL: "C", TZ: "UTC" },
  });
  assert(result.status === 0, `git ${args[0]} failed while scanning fixture history`);
  return result.stdout;
}

function scanReachableFixtureHistory() {
  for (const path of fixturePaths) {
    const commits = git(["log", "--all", "--format=%H", "--", path]).toString("utf8").trim().split("\n").filter(Boolean);
    for (const commit of commits) scanPublicMetadata(git(["show", `${commit}:${path}`]), `${path}@${commit}`);
  }
}

function parseVector(bytes) {
  const records = new Map();
  const source = bytes.toString("utf8");
  assert(!source.includes("\r"), "vector must use canonical LF line endings");
  for (const [index, line] of source.split("\n").entries()) {
    if (line === "" || line.startsWith("#")) continue;
    const separator = line.indexOf("=");
    assert(separator > 0 && separator < line.length - 1, `line ${index + 1}: expected non-empty key=value`);
    const key = line.slice(0, separator);
    const value = line.slice(separator + 1);
    assert(/^[a-z][a-z0-9_.]*$/u.test(key), `line ${index + 1}: invalid key ${key}`);
    assert(!records.has(key), `line ${index + 1}: duplicate key ${key}`);
    records.set(key, value);
  }
  return records;
}

assert(manifest.fixture_set === "crypto.e2ee-interop-semantic.v1", "fixture_set changed");
assert(JSON.stringify(Object.keys(manifest)) === JSON.stringify([
  "fixture_set", "schema_version", "contract_version", "owner", "reviewers", "classification", "provenance", "generator",
  "seed_policy", "allowed_surfaces", "forbidden_surfaces", "cleanup", "files", "required_keys", "expected", "n_minus_one",
  "acceptance_boundary", "reopen_triggers",
]), "manifest top-level schema changed");
assert(manifest.schema_version === 1, "manifest schema_version must be 1");
assert(manifest.contract_version === "tl-mls-1/semantic-spike-v1", "contract_version changed");
assert(manifest.owner === "Crypto", "Crypto must remain the fixture owner");
assert(JSON.stringify(manifest.reviewers) === JSON.stringify(["Crypto", "Security"]), "fixture reviewers changed");
assert(manifest.classification === "public_metadata", "fixture classification must remain public_metadata");
assert(JSON.stringify(manifest.provenance) === JSON.stringify({
  source: "Threadline T011 Issue #25 / PR #56 semantic spike",
  license: "Apache-2.0",
  production_export: false,
}), "fixture provenance changed");
assert(JSON.stringify(manifest.generator) === JSON.stringify({
  path: "git-object:test/crypto/e2ee-interop-v1.vector",
  version: pinnedSourceCommit,
  reproduction_command: `git show ${pinnedSourceCommit}:test/crypto/e2ee-interop-v1.vector`,
}), "immutable-vector source anchor changed");
assert(typeof manifest.seed_policy === "string" && manifest.seed_policy.includes("No seed"), "seed policy is missing");
assert(JSON.stringify(manifest.allowed_surfaces) === JSON.stringify([
  "repository contract verification", "Crypto semantic regression tests", "independent contract-input review",
]), "allowed fixture surfaces changed");
assert(JSON.stringify(manifest.forbidden_surfaces) === JSON.stringify([
  "production import", "credential or key provisioning", "logs and telemetry payloads", "production-readiness evidence",
]), "forbidden fixture surfaces changed");
assert(typeof manifest.cleanup === "string" && manifest.cleanup.includes("No ephemeral secret state"), "cleanup policy is missing");
assert(manifest.files.length === 1 && manifest.files[0].path === "e2ee-interop-v1.vector", "manifest must bind exactly the T011 vector");
assert(manifest.files[0].sha256 === pinnedVectorSha256, "manifest vector SHA-256 differs from independent pin");
assert(sha256(vectorBytes) === pinnedVectorSha256, "T011 vector SHA-256 mismatch");
const reproducedVectorBytes = git(["show", `${pinnedSourceCommit}:test/crypto/e2ee-interop-v1.vector`]);
assert(reproducedVectorBytes.equals(vectorBytes), "declared historical source does not reproduce the current vector byte-for-byte");

const records = parseVector(vectorBytes);
assert(JSON.stringify(manifest.required_keys) === JSON.stringify(pinnedKeys), "manifest key schema differs from independent pin");
assert(JSON.stringify([...records.keys()]) === JSON.stringify(pinnedKeys), "vector key set or order changed");
assert(records.get("format_version") === "1", "vector format_version changed");
assert(records.get("profile") === manifest.expected.profile, "Crypto Profile changed");
assert(records.get("protocol") === "MLS-1.0-RFC9420", "MLS protocol changed");
assert(records.get("ciphersuite") === "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519", "cipher suite changed");
assert(records.get("wire_format.handshake") === manifest.expected.handshake_wire_format, "handshake wire format changed");
assert(records.get("key_package.expected") === manifest.expected.key_package, "KeyPackage expectation changed");
assert(records.get("offline_new_device.expected") === manifest.expected.offline_new_device, "offline-device expectation changed");
assert(records.get("out_of_order_commit.expected") === manifest.expected.out_of_order_commit, "out-of-order Commit expectation changed");
assert(records.get("device_revocation.expected") === manifest.expected.device_revocation, "revocation expectation changed");
assert(records.get("history.authorized") === manifest.expected.history_authorized, "History success expectation changed");
assert(records.get("history.unauthorized") === manifest.expected.history_unauthorized, "History denial expectation changed");
assert(records.get("history.cross_tenant") === manifest.expected.history_cross_tenant, "History tenant expectation changed");
assert(records.get("recovery.success") === manifest.expected.recovery_success, "Recovery success expectation changed");
assert(records.get("recovery.no_recipient") === manifest.expected.recovery_unavailable, "Recovery unavailable expectation changed");
for (const [key, expectedName] of [
  ["recovery.refused", "recovery_refused"], ["recovery.corrupt", "recovery_corrupt"],
  ["recovery.old_epoch", "recovery_old_epoch"], ["recovery.cross_group", "recovery_cross_group"],
  ["recovery.cross_tenant", "recovery_cross_tenant"], ["error.replay", "replay"], ["error.corrupt", "corrupt"],
  ["error.old_epoch", "old_epoch"], ["error.future_epoch", "future_epoch"],
]) assert(records.get(key) === manifest.expected[expectedName], `${key} expectation changed`);
assert(records.get("error.unknown_version") === manifest.expected.unknown_version, "unknown-version expectation changed");
assert(records.get("output.classification") === "public-metadata-and-digests-only", "vector classification changed");
assert(/^[0-9a-f]{64}$/u.test(records.get("recovery.payload_sha256")), "recovery payload digest is not canonical SHA-256");
assert(/^[0-9a-f]{64}$/u.test(records.get("recovery.binding_sha256")), "recovery binding digest is not canonical SHA-256");
assert(manifest.acceptance_boundary.openmls_0_8_1_production_admission === "FAIL", "OpenMLS 0.8.1 spike must remain production FAIL");
for (const gate of ["production_crypto_provider", "independent_production_interoperability"]) {
  assert(manifest.acceptance_boundary[gate] === "NOT ESTABLISHED", `${gate} must remain NOT ESTABLISHED`);
}
for (const gate of ["real_kms_hsm", "physical_devices"]) {
  assert(manifest.acceptance_boundary[gate] === "NOT RUN", `${gate} must remain NOT RUN`);
}
assert(manifest.n_minus_one.status === "not_applicable_to_semantic-spike-vector"
  && manifest.n_minus_one.reader_writer_pairs.length === 0
  && JSON.stringify(manifest.n_minus_one.retained_historical_files) === JSON.stringify(["e2ee-interop-v1.vector"])
  && manifest.n_minus_one.note.includes("not a persisted Protobuf frame"),
"semantic vector must not claim persisted-message N-1 evidence");
assert(JSON.stringify(manifest.reopen_triggers) === JSON.stringify([
  "Crypto Profile tuple changes", "MLS provider or provider version changes", "leaf lifetime policy changes",
  "History or Recovery semantic/error changes", "classification or fixture digest changes", "production admission decision changes",
]), "fixture reopen triggers changed");

for (const path of fixturePaths) scanPublicMetadata(readFileSync(join(repositoryRoot, path)), `${path}@working-tree`);
scanReachableFixtureHistory();

console.log("Threadline T011 Crypto semantic fixture manifest is valid; production admission and physical evidence remain unproven.");
