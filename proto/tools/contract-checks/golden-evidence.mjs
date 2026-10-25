import { join } from "node:path";

import { protoRoot, read } from "./support.mjs";

// Freezes the T014 Golden Frame manifest: the recorded acceptance evidence,
// the generated-adapter N-1 matrix and the protected-runner codegen artifact.

const satisfiedEvidence = [
  ["representative-channel-event-envelope-frame", "representative ChannelEventEnvelope frame must be recorded"],
  ["representative-recovery-envelope-frame", "representative RecoveryEnvelope frame must be recorded"],
  ["rust-dynamic-message-persistence-seam", "the selected Rust DynamicMessage seam must be recorded"],
];

const laterSatisfiedEvidence = [
  ["five-language-generated-envelope-unknown-field-roundtrip", "five-language unknown-field evidence must be recorded as satisfied"],
  ["bidirectional-n-minus-one-compatibility", "N-1 evidence must be recorded as satisfied"],
];

const retiredBlockers = [
  ["cross-language-unknown-field-roundtrip", "verified cross-language evidence must not remain a blocker"],
  ["n-minus-one-compatibility", "verified N-1 evidence must not remain a blocker"],
];

const formalCodegenEvidence = {
  status: "passed",
  targetSha: "6d4183410793e9946c86ec6c55397a31bc018da4",
  prepareRunId: "32569817612",
  prepareWorkflowSha: "1f1e53244328ed3001cb0ec83f10071e3ae3dff9",
  verifyRunId: "32570070634",
  verifyWorkflowSha: "1f1e53244328ed3001cb0ec83f10071e3ae3dff9",
  runnerImageVersion: "20260728.0273.1",
  runnerImagesInventorySha: "8d3ea005fa2d87f3cbc9255c27fdfed9e901a043",
  manifestSha256: "36f0052f7594c0b8e03fd87b832481053de23dd9f143837b19da7f018b068678",
  mode: "verify-only",
  physicalDevices: "NOT RUN",
};

const requiredEnvironment = [
  "THREADLINE_GO",
  "THREADLINE_PROTOC_GEN_GO",
  "THREADLINE_CARGO",
  "THREADLINE_JAVA_HOME",
  "THREADLINE_PROTOC",
  "THREADLINE_KOTLIN_JARS",
  "THREADLINE_SWIFT",
  "THREADLINE_SWIFT_SDK",
  "THREADLINE_SWIFT_PROTOBUF_ARCHIVE",
];

function checkAcceptanceBoundary(assert, boundary) {
  assert(boundary.issue === 28, "Golden Frame acceptance boundary must identify T014");
  assert(boundary.issueMayClose === true, "T014 may close only after current protected-runner evidence passes");
  assert(boundary.status === "passed", "T014 acceptance boundary must record the completed evidence state");
  for (const [item, message] of satisfiedEvidence) assert(boundary.satisfiedHere.includes(item), message);
  assert(!boundary.notSatisfiedHere.includes("rust-persisted-envelope-unknown-field-preservation-decision"), "the verified Rust persistence decision must not remain listed as unsatisfied");
  for (const [item, message] of laterSatisfiedEvidence) assert(boundary.satisfiedHere.includes(item), message);
  for (const [item, message] of retiredBlockers) assert(!boundary.notSatisfiedHere.includes(item), message);
  assert(boundary.satisfiedHere.includes("formal-plan-reconciled-with-merged-templates"), "the merged formal generation plan must be recorded as satisfied");
  assert(!boundary.notSatisfiedHere.includes("formal-plan-reconciliation-with-merged-templates"), "completed formal-plan reconciliation must not remain a blocker");
  assert(boundary.satisfiedHere.includes("protected-runner-formal-codegen-evidence"), "formal protected-runner evidence must be recorded as satisfied");
  assert(boundary.notSatisfiedHere.length === 0, "T014 must not close with an unsatisfied evidence item");
  assert(boundary.splitRequiresExplicitApprovalFrom.includes("Contracts") && boundary.splitRequiresExplicitApprovalFrom.includes("Product"), "moving the concrete frames out of T014 requires Contracts and Product approval");
}

function checkCompatibilityEvidence(assert, evidence) {
  assert(evidence.nMinusOneCommit === "b6c797c45d90fbb8b0465f7d7407ee1536e322e3", "generated-adapter N-1 evidence must remain pinned to the pre-T014 main commit");
  assert(evidence.status === "passed", "the generated-adapter matrix must be recorded as passed");
  assert(evidence.protocol === "current-write-n-minus-one-read-mutate-write-current-read-and-reverse", "N-1 evidence must require bidirectional read/mutate/write handoff");
  assert(JSON.stringify(evidence.frames) === JSON.stringify([
    "threadline.message.v1.ChannelEventEnvelope",
    "threadline.crypto.v1.RecoveryEnvelope",
  ]), "generated-adapter evidence must cover both representative persisted envelopes");
  assert(evidence.canaryFieldNumber === 50000, "generated-adapter evidence must preserve field 50000");
  assert(JSON.stringify(evidence.requiredAdapters) === JSON.stringify(["go", "typescript", "rust", "swift", "kotlin"]), "generated-adapter evidence must cover the exact five-language set");
  assert(JSON.stringify(evidence.verifiedAdapters) === JSON.stringify(evidence.requiredAdapters), "every required generated adapter must have verified evidence");
  assert(evidence.verificationCommand === "node proto/tools/verify-generated-envelope-compat.mjs --languages=go,typescript,rust,swift,kotlin", "the canonical five-language verification command must remain fixed");
}

export function checkGoldenEvidence(assert) {
  const manifest = JSON.parse(read(join(protoRoot, "golden", "v1", "manifest.json")));
  assert(manifest.schemaVersion === 2, "Golden Frame manifest schemaVersion must be 2");
  assert(manifest.canaryFieldNumber === 50000, "Golden Frame unknown-field canary must remain field 50000");
  assert(manifest.classification === "synthetic-protocol-compatibility-no-secrets", "Golden Frame fixtures must remain synthetic and secret-free");
  checkAcceptanceBoundary(assert, manifest.acceptanceBoundary);
  checkCompatibilityEvidence(assert, manifest.compatibilityEvidence);
  assert(JSON.stringify(manifest.formalCodegenEvidence) === JSON.stringify(formalCodegenEvidence), "formal codegen evidence must remain bound to the reviewed protected-runner artifact");
  assert(JSON.stringify(manifest.compatibilityEvidence.requiredEnvironment) === JSON.stringify(requiredEnvironment), "five-language verification must name every externally supplied pinned input");
  assert(manifest.canaries.length === 2, "Ciphertext and Crypto Envelope canaries are both required");
  assert(manifest.frames.length === 2, "ChannelEventEnvelope and RecoveryEnvelope frames are both required");
}
