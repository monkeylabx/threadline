import { statSync } from "node:fs";
import { join } from "node:path";

import { loadScopedGeneratedCompatManifest } from "../generated-envelope-compat/scoped-manifest.mjs";
import { protoRoot, read, repositoryRoot } from "./support.mjs";

// The canonical registry of child verifiers the aggregate command executes.
// Registering a verifier here is the only way it joins `verify-contracts`;
// `existence` names the verifiers whose absence is itself a contract failure.

const tool = (name) => join(protoRoot, "tools", name);

export const registeredVerifiers = [
  { path: tool("codegen-output.test.mjs") },
  { path: tool("codegen-bundle-tools.test.mjs") },
  { path: tool("export-codegen-install.test.mjs") },
  { path: tool("create-codegen-bundle-spec.test.mjs") },
  { path: tool("codegen-bootstrap.test.mjs") },
  { path: tool(join("contract-checks", "support.test.mjs")) },
  { path: tool(join("contract-checks", "aggregate.test.mjs")) },
  { path: tool("verify-domain-event-contract.mjs") },
  { path: tool("verify-golden-frames.mjs"), existence: "representative Golden Frame verifier must exist" },
  { path: tool("verify-message-sync-contracts.mjs"), existence: "T015 message/sync behavior verifier must exist" },
  { path: tool("verify-authorization-contracts.mjs"), existence: "P03-05B authorization behavior verifier must exist" },
  { path: tool("verify-capability-grant-contracts.mjs"), existence: "P03-06B Capability Grant signature verifier must exist" },
  { path: tool("verify-audit-retention-contracts.mjs"), existence: "P03-07A Audit/Retention metadata verifier must exist" },
  { path: tool("verify-crypto-contracts.mjs"), existence: "T019 crypto/recovery behavior verifier must exist" },
  { path: tool("verify-agentd-local-contracts.mjs"), existence: "agentd local interface verifier must exist" },
  { path: join(repositoryRoot, "test", "crypto", "verify-e2ee-interop-fixture.mjs"), existence: "T011 Crypto semantic fixture verifier must exist" },
];

// Runs after every registered verifier passed; its own exit status becomes
// the aggregate exit status.
export const installTestPath = tool("verify-codegen-install.test.mjs");

const t019VerificationCommand = "node proto/tools/verify-t019-generated-compat.mjs --baseline=b997fd70f1b4a115c9046df4db15fea4849df487 --target=19e64a8d727a496c3526057ea690f3495012ad90 --frame-manifest=test/fixtures/proto/crypto/generated-compat-manifest.json --languages=go,typescript,rust,kotlin,swift";

function checkScopedGeneratedCompat(assert) {
  const cryptoContractManifest = JSON.parse(read(join(repositoryRoot, "test", "fixtures", "proto", "crypto", "manifest.json")));
  assert(statSync(tool("verify-t019-generated-compat.mjs")).isFile(), "T019 scoped generated compatibility verifier must exist");
  assert(statSync(join(repositoryRoot, "test", "fixtures", "proto", "crypto", "generated-compat-manifest.json")).isFile(), "T019 scoped generated compatibility manifest must exist");
  assert(statSync(tool(join("generated-envelope-compat", "scoped-manifest.mjs"))).isFile(), "T019 scoped manifest validator must exist");
  const { manifest } = loadScopedGeneratedCompatManifest("test/fixtures/proto/crypto/generated-compat-manifest.json");
  assert(manifest.baselineCommit === "b997fd70f1b4a115c9046df4db15fea4849df487", "T019 scoped compatibility baseline must remain pinned to T015 main");
  assert(manifest.targetSchemaCommit === "19e64a8d727a496c3526057ea690f3495012ad90", "T019 scoped compatibility target must remain pinned to the reviewed parent schema commit");
  assert(manifest.verificationCommand === t019VerificationCommand, "T019 scoped compatibility command must document every pinned input");
  assert(manifest.failClosedEvidence.verifier === "proto/tools/verify-crypto-contracts.mjs", "T019 scoped fail-closed evidence must use the canonical crypto verifier");
  for (const scenarioId of manifest.failClosedEvidence.scenarioIds) {
    assert(cryptoContractManifest.requiredCases.includes(scenarioId), `T019 scoped fail-closed scenario is not aggregated: ${scenarioId}`);
  }
}

function checkEnvelopeHarnesses(assert) {
  const rustEnvelopeHarnessRoot = tool("rust-envelope-compat");
  assert(statSync(tool("verify-rust-envelope-preservation.mjs")).isFile(), "Rust persisted-envelope verifier must exist");
  assert(statSync(tool("verify-generated-envelope-compat.mjs")).isFile(), "five-language generated-envelope verifier must exist");
  assert(statSync(tool(join("generated-envelope-compat", "typescript.mjs"))).isFile(), "TypeScript generated-envelope adapter must exist");
  assert(statSync(tool(join("generated-envelope-compat", "go", "main.go"))).isFile(), "Go generated-envelope adapter must exist");
  assert(statSync(tool(join("generated-envelope-compat", "kotlin", "Main.kt"))).isFile(), "Kotlin generated-envelope adapter must exist");
  assert(statSync(tool(join("generated-envelope-compat", "swift", "main.swift"))).isFile(), "Swift generated-envelope adapter must exist");
  assert(statSync(join(rustEnvelopeHarnessRoot, "Cargo.toml")).isFile(), "isolated Rust persisted-envelope harness manifest must exist");
  assert(statSync(join(rustEnvelopeHarnessRoot, "Cargo.lock")).isFile(), "isolated Rust persisted-envelope harness lock must exist");
  assert(statSync(join(rustEnvelopeHarnessRoot, "src", "main.rs")).isFile(), "isolated Rust persisted-envelope harness source must exist");
  const rustHarnessManifest = read(join(rustEnvelopeHarnessRoot, "Cargo.toml"));
  const rustHarnessLock = read(join(rustEnvelopeHarnessRoot, "Cargo.lock"));
  assert(rustHarnessManifest.includes('prost = "=0.14.1"'), "Rust harness must exactly pin prost 0.14.1");
  assert(rustHarnessManifest.includes('prost-reflect = "=0.16.5"'), "Rust harness must exactly pin prost-reflect 0.16.5");
  assert(rustHarnessLock.includes('name = "prost-reflect"\nversion = "0.16.5"'), "Rust harness lock must retain prost-reflect 0.16.5");
}

// Every named verifier must exist before anything runs; the T019 scoped
// manifest and the envelope harnesses are checked alongside them because the
// registered crypto verifiers depend on those inputs.
export function checkRegisteredVerifiers(assert) {
  for (const entry of registeredVerifiers.filter((candidate) => candidate.existence)) {
    assert(statSync(entry.path).isFile(), entry.existence);
  }
  checkScopedGeneratedCompat(assert);
  checkEnvelopeHarnesses(assert);
}
