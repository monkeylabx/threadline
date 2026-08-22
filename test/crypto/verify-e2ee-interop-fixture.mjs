import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const fixtureRoot = dirname(fileURLToPath(import.meta.url));
const manifest = JSON.parse(readFileSync(join(fixtureRoot, "e2ee-interop-v1.manifest.json"), "utf8"));
const vectorBytes = readFileSync(join(fixtureRoot, "e2ee-interop-v1.vector"));

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
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
assert(manifest.schema_version === 1, "manifest schema_version must be 1");
assert(manifest.contract_version === "tl-mls-1/semantic-spike-v1", "contract_version changed");
assert(manifest.owner === "Crypto", "Crypto must remain the fixture owner");
assert(JSON.stringify(manifest.reviewers) === JSON.stringify(["Crypto", "Security"]), "fixture reviewers changed");
assert(manifest.classification === "public_metadata", "fixture classification must remain public_metadata");
assert(manifest.provenance.production_export === false && manifest.provenance.license === "Apache-2.0", "fixture provenance changed");
assert(manifest.generator.version === "none-existing-vector-is-immutable", "immutable-vector generator boundary changed");
assert(manifest.files.length === 1 && manifest.files[0].path === "e2ee-interop-v1.vector", "manifest must bind exactly the T011 vector");
assert(sha256(vectorBytes) === manifest.files[0].sha256, "T011 vector SHA-256 mismatch");

const records = parseVector(vectorBytes);
assert(JSON.stringify([...records.keys()]) === JSON.stringify(manifest.required_keys), "vector key set or order changed");
assert(records.get("format_version") === "1", "vector format_version changed");
assert(records.get("profile") === manifest.expected.profile, "Crypto Profile changed");
assert(records.get("protocol") === "MLS-1.0-RFC9420", "MLS protocol changed");
assert(records.get("ciphersuite") === "MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519", "cipher suite changed");
assert(records.get("wire_format.handshake") === manifest.expected.handshake_wire_format, "handshake wire format changed");
assert(records.get("key_package.expected") === manifest.expected.key_package, "KeyPackage expectation changed");
assert(records.get("history.authorized") === manifest.expected.history_authorized, "History success expectation changed");
assert(records.get("history.unauthorized") === manifest.expected.history_unauthorized, "History denial expectation changed");
assert(records.get("recovery.success") === manifest.expected.recovery_success, "Recovery success expectation changed");
assert(records.get("recovery.no_recipient") === manifest.expected.recovery_unavailable, "Recovery unavailable expectation changed");
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
  && manifest.n_minus_one.reader_writer_pairs.length === 0,
"semantic vector must not claim persisted-message N-1 evidence");

console.log("Threadline T011 Crypto semantic fixture manifest is valid; production admission and physical evidence remain unproven.");
