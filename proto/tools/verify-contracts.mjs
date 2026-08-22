import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const protoRoot = fileURLToPath(new URL("../", import.meta.url));
const repositoryRoot = fileURLToPath(new URL("../../", import.meta.url));
const errors = [];

function read(path) {
  return readFileSync(path, "utf8").replaceAll("\r\n", "\n");
}

function assert(condition, message) {
  if (!condition) errors.push(message);
}

function filesBelow(directory, suffix) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return filesBelow(path, suffix);
    return path.endsWith(suffix) ? [path] : [];
  });
}

const bufYaml = read(join(repositoryRoot, "buf.yaml"));
const generationYaml = read(join(repositoryRoot, "buf.gen.yaml"));
const generationPlan = JSON.parse(generationYaml);
const combinedConfig = `${bufYaml}\n${generationYaml}`;
assert(!/buf\.build|\bremote:|\bdeps:|\bmodule:/u.test(combinedConfig), "Buf configuration must not use the public BSR");
assert(/breaking:\s*\n\s+use:\s*\n\s+- FILE/u.test(bufYaml), "buf.yaml must enforce FILE breaking rules");
assert(/disallow_comment_ignores:\s*true/u.test(bufYaml), "Buf lint comment ignores must remain disabled");

const toolchain = JSON.parse(read(join(protoRoot, "toolchain.lock.json")));
const workspaceToolchain = JSON.parse(read(join(repositoryRoot, "toolchains.json")));
assert(toolchain.schemaVersion === 1, "toolchain lock schemaVersion must be 1");
assert(toolchain.tools.javaRuntime === workspaceToolchain.android.java, "Contract codegen JDK must match the workspace JDK pin");
assert(toolchain.tools.go === workspaceToolchain.goToolchain, "Contract source-built generators must match the workspace Go toolchain pin");
assert(toolchain.tools.javaVendor === "Eclipse Adoptium" && workspaceToolchain.android.javaVendor === "Temurin", "Contract codegen JDK vendor must match the workspace Temurin pin");
assert(toolchain.scope.startsWith("Contract-specific"), "Proto lock must state its contract-only boundary");
assert(JSON.stringify(toolchain.rustPersistence) === JSON.stringify({
  cargo: workspaceToolchain.rust,
  prost: "0.14.1",
  prostReflect: "0.16.5",
  harnessManifest: "proto/tools/rust-envelope-compat/Cargo.toml",
  connectedCommand: "THREADLINE_BUF=<absolute-pinned-buf> THREADLINE_CARGO=<absolute-pinned-cargo> node proto/tools/verify-rust-envelope-preservation.mjs --connected",
  offlineCommand: "THREADLINE_BUF=<absolute-pinned-buf> THREADLINE_CARGO=<absolute-pinned-cargo> node proto/tools/verify-rust-envelope-preservation.mjs --offline",
}), "Rust persisted-envelope seam and exact runtime pins must remain fixed");
assert(Object.keys(toolchain.outputs).length === 5, "exactly five language outputs must be pinned");
const expectedGenerationPlan = {
  version: "v2",
  clean: true,
  managed: {
    enabled: true,
    override: [
      { file_option: "go_package_prefix", value: "github.com/monkeylabx/threadline/services/gen" },
      { file_option: "java_package_prefix", value: "com.threadline.proto" },
    ],
  },
  plugins: [
    { local: "protoc-gen-go", out: toolchain.outputs.go, opt: ["paths=source_relative"] },
    { local: "protoc-gen-connect-go", out: toolchain.outputs.go, opt: ["paths=source_relative"] },
    { local: "protoc-gen-es", out: toolchain.outputs.typescript, opt: ["target=ts", "import_extension=.js"] },
    { local: "protoc-gen-prost", out: toolchain.outputs.rust, opt: ["bytes=.", "compile_well_known_types=false"] },
    { local: "protoc-gen-prost-crate", out: toolchain.outputs.rust, opt: ["no_features"], strategy: "all" },
    { local: "protoc-gen-swift", out: toolchain.outputs.swift, opt: ["Visibility=Public", "FileNaming=PathToUnderscores"] },
    { local: "protoc-gen-connect-swift", out: toolchain.outputs.swift, opt: ["Visibility=Public", "FileNaming=PathToUnderscores"] },
    { protoc_builtin: "java", protoc_path: "protoc", out: toolchain.outputs.kotlin.javaMessages },
    { protoc_builtin: "kotlin", protoc_path: "protoc", out: toolchain.outputs.kotlin.kotlinDsl },
    { local: "protoc-gen-connect-kotlin", out: toolchain.outputs.kotlin.kotlinDsl },
  ],
  inputs: [{ directory: "proto" }],
};
assert(JSON.stringify(generationPlan) === JSON.stringify(expectedGenerationPlan), "buf.gen.yaml must exactly match the pinned five-language generation plan");
for (const plugin of [
  "protoc-gen-go",
  "protoc-gen-connect-go",
  "protoc-gen-es",
  "protoc-gen-prost",
  "protoc-gen-prost-crate",
  "protoc-gen-swift",
  "protoc-gen-connect-swift",
  "protoc-gen-connect-kotlin",
]) {
  assert(typeof toolchain.tools[plugin] === "string", `${plugin} version must be pinned`);
}
assert(typeof toolchain.tools.git === "string", "repository-mode Git version must be pinned");
for (const runtime of ["kotlin-compiler", "protobuf-java", "protobuf-kotlin", "kotlin-stdlib"]) {
  assert(typeof toolchain.tools[runtime] === "string", `${runtime} version must be pinned`);
}
assert(toolchain.integrity.toolManifestSchemaVersion === 5, "codegen tool manifest schema must be pinned to v5");
assert(JSON.stringify(toolchain.integrity.toolManifestSchema) === JSON.stringify({
  manifestKeys: ["schemaVersion", "platform", "profile", "sources", "closures", "tools"],
  sourceKeys: ["kind", "file", "url", "sha256"],
  builderSourceKeys: ["kind", "file", "url", "sha256", "authentication"],
  hostBuilderSourceKeys: ["kind", "path", "url", "authentication"],
  rustChecksumAuthenticationKeys: ["kind", "file", "url", "sha256"],
  goDistributionAuthenticationKeys: ["kind", "file", "url", "sha256"],
  appleInstallerAuthenticationKeys: ["kind", "signer"],
  appleXcodeAuthenticationKeys: ["kind", "bundleIdentifier", "version", "build", "swiftVersion", "sdkVersion"],
  invocationKinds: ["native", "verified-node", "verified-java"],
  closureKeys: ["root", "sources", "treeSha256", "files"],
  toolKeys: ["path", "sha256", "closure", "provenance", "invocation"],
  officialProvenanceKeys: ["kind", "sources"],
  sourceBuiltProvenanceKeys: ["kind", "source", "builders", "buildCommand", "reproducibility"],
  protocolStubProvenanceKeys: ["kind", "sources"],
  sourceKinds: ["builder-toolchain", "host-builder-toolchain", "official-binary", "official-package", "protocol-fixture", "source-archive"],
  provenanceKinds: ["official-binary", "official-package", "protocol-stub", "source-built"],
  sourceBuiltReproducibility: "single-build-output-verified",
}), "codegen tool manifest v5 vocabulary must remain exact");
for (const collection of [toolchain.integrity.runtimeJars, toolchain.integrity.kotlinCompilerClasspath]) {
  for (const [artifact, integrity] of Object.entries(collection)) {
    assert(/^[0-9a-f]{64}$/u.test(integrity.sha256), `${artifact} must have a canonical SHA-256 pin`);
    assert(/^https:\/\/repo1\.maven\.org\/maven2\//u.test(integrity.source), `${artifact} must record its Maven Central source`);
    assert(/^[0-9a-f]{40}$/u.test(integrity.sourceSha1), `${artifact} must record the published source SHA-1`);
  }
}
assert(JSON.stringify(toolchain.integrity.swiftProtobufSourceArchive) === JSON.stringify({
  version: toolchain.tools["protoc-gen-swift"],
  sha256: "7e35c119afe8f16fe4de45c2143b0f50a205db83738092336562d610469283ac",
  source: "https://github.com/apple/swift-protobuf/archive/refs/tags/1.38.1.tar.gz",
  tagCommit: "55d7a1cc5666b85c13464aea1c4b4a90feccb4c8",
}), "Swift compatibility source must remain bound to the exact official 1.38.1 tag archive");
assert(toolchain.tools["protoc-gen-es"] === "2.14.0", "TypeScript generator must match the workspace package pin");
const trustedLaunchPrefix = "<approved-clean-env-launcher> <bundle-absolute-node> proto/tools/verify-codegen.mjs";
assert(JSON.stringify(Object.keys(toolchain.commands).sort()) === JSON.stringify(["breaking", "generate", "lint", "protocolSmoke", "verify", "verifyCodegen"]), "contract command set must remain exact");
assert(toolchain.commands.lint === "buf lint", "contract lint command must remain canonical");
assert(toolchain.commands.breaking === "buf breaking --against '.git#branch=main'", "contract breaking command must remain canonical");
assert(toolchain.commands.verify === "node proto/tools/verify-contracts.mjs", "repository structural verification command must remain canonical");
assert(toolchain.commands.generate === `${trustedLaunchPrefix} --mode=repository`, "verified repository generation must use the trusted bootstrap contract");
assert(toolchain.commands.verifyCodegen === `${trustedLaunchPrefix} --mode=verify-only`, "release codegen verification must use the trusted bootstrap contract");
assert(toolchain.commands.protocolSmoke === `${trustedLaunchPrefix} --mode=protocol-smoke`, "protocol smoke must use the same trusted bootstrap contract while remaining non-release");
assert(Object.keys(toolchain.generationChecks).length === 6, "all six generated source surfaces must have formal output checks");
for (const [surface, checks] of Object.entries(toolchain.generationChecks)) {
  assert(Array.isArray(checks.extensions) && checks.extensions.length > 0, `${surface} must pin generated source extensions`);
  assert(Number.isSafeInteger(checks.fileCount) && checks.fileCount > 0, `${surface} must pin the generated file count`);
  assert(/^[0-9a-f]{64}$/u.test(checks.treeSha256), `${surface} must pin the exact generated tree SHA-256`);
  assert(Array.isArray(checks.signatureRegexAny) && checks.signatureRegexAny.length > 0, `${surface} must pin accepted real-generator signatures`);
  assert(Array.isArray(checks.structureRegex) && checks.structureRegex.length > 0, `${surface} must pin authoritative contract output structure`);
}
assert(statSync(join(protoRoot, "tools", "verify-codegen.mjs")).isFile(), "the verified codegen command must exist");
const installTestPath = join(protoRoot, "tools", "verify-codegen-install.test.mjs");
assert(statSync(installTestPath).isFile(), "repository codegen failure-injection tests must exist");
const bundleBuilderPath = join(protoRoot, "tools", "create-codegen-bundle.mjs");
assert(statSync(bundleBuilderPath).isFile(), "formal codegen bundle builder must exist");
const bundleBuilderSource = read(bundleBuilderPath);
assert(!bundleBuilderSource.includes("rmSync(outputRoot"), "formal bundle builder must never recursively replace a caller-selected output directory");
assert(bundleBuilderSource.includes("bundle output must be a new, non-existing directory"), "formal bundle builder must refuse every pre-existing output directory");
const bundleRefusalRoot = mkdtempSync(join(tmpdir(), "threadline-bundle-refusal-"));
try {
  const existingOutput = join(bundleRefusalRoot, "existing-output");
  const marker = join(existingOutput, "owned-by-caller.txt");
  const spec = join(bundleRefusalRoot, "spec.json");
  mkdirSync(existingOutput);
  writeFileSync(marker, "must survive\n");
  writeFileSync(spec, `${JSON.stringify({
    schemaVersion: 1,
    platform: "darwin-arm64",
    profile: "release",
    sources: {},
    closures: {},
    tools: Object.fromEntries([
      "buf", "protoc", "protoc-gen-go", "protoc-gen-connect-go", "protoc-gen-es",
      "protoc-gen-prost", "protoc-gen-prost-crate", "protoc-gen-swift",
      "protoc-gen-connect-swift", "protoc-gen-connect-kotlin", "java", "javac", "node",
    ].map((name) => [name, null])),
  })}\n`);
  const refusal = spawnSync(process.execPath, [bundleBuilderPath], {
    cwd: repositoryRoot,
    encoding: "utf8",
    env: { ...process.env, THREADLINE_CODEGEN_BUNDLE_SPEC: spec, THREADLINE_CODEGEN_BUNDLE_DIR: existingOutput },
  });
  assert(refusal.status !== 0, "formal bundle builder must reject a pre-existing output directory");
  assert(read(marker) === "must survive\n", "formal bundle builder rejection must preserve caller-owned output contents");
} finally {
  rmSync(bundleRefusalRoot, { recursive: true, force: true });
}
const bundleSpecBuilderPath = join(protoRoot, "tools", "create-codegen-bundle-spec.mjs");
assert(statSync(bundleSpecBuilderPath).isFile(), "formal codegen bundle spec builder must exist");

for (const path of filesBelow(join(protoRoot, "threadline"), ".proto")) {
  const source = read(path);
  const packageMatch = source.match(/^package\s+([a-z0-9_.]+);/mu);
  const expectedPackage = relative(protoRoot, dirname(path)).split(/[\\/]/u).join(".");
  assert(source.startsWith('syntax = "proto3";'), `${relative(repositoryRoot, path)} must declare proto3 syntax first`);
  assert(packageMatch?.[1] === expectedPackage, `${relative(repositoryRoot, path)} package must be ${expectedPackage}`);
  assert(/\.v[1-9][0-9]*$/u.test(expectedPackage), `${relative(repositoryRoot, path)} must use a stable version suffix`);
}

const manifestPath = join(protoRoot, "golden", "v1", "manifest.json");
const manifest = JSON.parse(read(manifestPath));
assert(manifest.schemaVersion === 2, "Golden Frame manifest schemaVersion must be 2");
assert(manifest.canaryFieldNumber === 50000, "Golden Frame unknown-field canary must remain field 50000");
assert(manifest.classification === "synthetic-protocol-compatibility-no-secrets", "Golden Frame fixtures must remain synthetic and secret-free");
assert(manifest.acceptanceBoundary.issue === 28, "Golden Frame acceptance boundary must identify T014");
assert(manifest.acceptanceBoundary.issueMayClose === true, "T014 may close only after current protected-runner evidence passes");
assert(manifest.acceptanceBoundary.status === "passed", "T014 acceptance boundary must record the completed evidence state");
assert(manifest.acceptanceBoundary.satisfiedHere.includes("representative-channel-event-envelope-frame"), "representative ChannelEventEnvelope frame must be recorded");
assert(manifest.acceptanceBoundary.satisfiedHere.includes("representative-recovery-envelope-frame"), "representative RecoveryEnvelope frame must be recorded");
assert(manifest.acceptanceBoundary.satisfiedHere.includes("rust-dynamic-message-persistence-seam"), "the selected Rust DynamicMessage seam must be recorded");
assert(!manifest.acceptanceBoundary.notSatisfiedHere.includes("rust-persisted-envelope-unknown-field-preservation-decision"), "the verified Rust persistence decision must not remain listed as unsatisfied");
assert(manifest.acceptanceBoundary.satisfiedHere.includes("five-language-generated-envelope-unknown-field-roundtrip"), "five-language unknown-field evidence must be recorded as satisfied");
assert(manifest.acceptanceBoundary.satisfiedHere.includes("bidirectional-n-minus-one-compatibility"), "N-1 evidence must be recorded as satisfied");
assert(!manifest.acceptanceBoundary.notSatisfiedHere.includes("cross-language-unknown-field-roundtrip"), "verified cross-language evidence must not remain a blocker");
assert(!manifest.acceptanceBoundary.notSatisfiedHere.includes("n-minus-one-compatibility"), "verified N-1 evidence must not remain a blocker");
assert(manifest.acceptanceBoundary.satisfiedHere.includes("formal-plan-reconciled-with-merged-templates"), "the merged formal generation plan must be recorded as satisfied");
assert(!manifest.acceptanceBoundary.notSatisfiedHere.includes("formal-plan-reconciliation-with-merged-templates"), "completed formal-plan reconciliation must not remain a blocker");
assert(manifest.acceptanceBoundary.satisfiedHere.includes("protected-runner-formal-codegen-evidence"), "formal protected-runner evidence must be recorded as satisfied");
assert(manifest.acceptanceBoundary.notSatisfiedHere.length === 0, "T014 must not close with an unsatisfied evidence item");
assert(manifest.acceptanceBoundary.splitRequiresExplicitApprovalFrom.includes("Contracts") && manifest.acceptanceBoundary.splitRequiresExplicitApprovalFrom.includes("Product"), "moving the concrete frames out of T014 requires Contracts and Product approval");
assert(manifest.compatibilityEvidence.nMinusOneCommit === "b6c797c45d90fbb8b0465f7d7407ee1536e322e3", "generated-adapter N-1 evidence must remain pinned to the pre-T014 main commit");
assert(manifest.compatibilityEvidence.status === "passed", "the generated-adapter matrix must be recorded as passed");
assert(manifest.compatibilityEvidence.protocol === "current-write-n-minus-one-read-mutate-write-current-read-and-reverse", "N-1 evidence must require bidirectional read/mutate/write handoff");
assert(JSON.stringify(manifest.compatibilityEvidence.frames) === JSON.stringify([
  "threadline.message.v1.ChannelEventEnvelope",
  "threadline.crypto.v1.RecoveryEnvelope",
]), "generated-adapter evidence must cover both representative persisted envelopes");
assert(manifest.compatibilityEvidence.canaryFieldNumber === 50000, "generated-adapter evidence must preserve field 50000");
assert(JSON.stringify(manifest.compatibilityEvidence.requiredAdapters) === JSON.stringify(["go", "typescript", "rust", "swift", "kotlin"]), "generated-adapter evidence must cover the exact five-language set");
assert(JSON.stringify(manifest.compatibilityEvidence.verifiedAdapters) === JSON.stringify(manifest.compatibilityEvidence.requiredAdapters), "every required generated adapter must have verified evidence");
assert(manifest.compatibilityEvidence.verificationCommand === "node proto/tools/verify-generated-envelope-compat.mjs --languages=go,typescript,rust,swift,kotlin", "the canonical five-language verification command must remain fixed");
assert(JSON.stringify(manifest.formalCodegenEvidence) === JSON.stringify({
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
}), "formal codegen evidence must remain bound to the reviewed protected-runner artifact");
assert(JSON.stringify(manifest.compatibilityEvidence.requiredEnvironment) === JSON.stringify([
  "THREADLINE_GO",
  "THREADLINE_PROTOC_GEN_GO",
  "THREADLINE_CARGO",
  "THREADLINE_JAVA_HOME",
  "THREADLINE_PROTOC",
  "THREADLINE_KOTLIN_JARS",
  "THREADLINE_SWIFT",
  "THREADLINE_SWIFT_SDK",
  "THREADLINE_SWIFT_PROTOBUF_ARCHIVE",
]), "five-language verification must name every externally supplied pinned input");
assert(manifest.canaries.length === 2, "Ciphertext and Crypto Envelope canaries are both required");
assert(manifest.frames.length === 2, "ChannelEventEnvelope and RecoveryEnvelope frames are both required");

const goldenTestPath = join(protoRoot, "tools", "verify-golden-frames.mjs");
assert(statSync(goldenTestPath).isFile(), "representative Golden Frame verifier must exist");
const messageSyncTestPath = join(protoRoot, "tools", "verify-message-sync-contracts.mjs");
assert(statSync(messageSyncTestPath).isFile(), "T015 message/sync behavior verifier must exist");
const cryptoSemanticFixtureTestPath = join(repositoryRoot, "test", "crypto", "verify-e2ee-interop-fixture.mjs");
assert(statSync(cryptoSemanticFixtureTestPath).isFile(), "T011 Crypto semantic fixture verifier must exist");
const rustEnvelopeTestPath = join(protoRoot, "tools", "verify-rust-envelope-preservation.mjs");
const generatedEnvelopeTestPath = join(protoRoot, "tools", "verify-generated-envelope-compat.mjs");
const rustEnvelopeHarnessRoot = join(protoRoot, "tools", "rust-envelope-compat");
assert(statSync(rustEnvelopeTestPath).isFile(), "Rust persisted-envelope verifier must exist");
assert(statSync(generatedEnvelopeTestPath).isFile(), "five-language generated-envelope verifier must exist");
assert(statSync(join(protoRoot, "tools", "generated-envelope-compat", "typescript.mjs")).isFile(), "TypeScript generated-envelope adapter must exist");
assert(statSync(join(protoRoot, "tools", "generated-envelope-compat", "go", "main.go")).isFile(), "Go generated-envelope adapter must exist");
assert(statSync(join(protoRoot, "tools", "generated-envelope-compat", "kotlin", "Main.kt")).isFile(), "Kotlin generated-envelope adapter must exist");
assert(statSync(join(protoRoot, "tools", "generated-envelope-compat", "swift", "main.swift")).isFile(), "Swift generated-envelope adapter must exist");
assert(statSync(join(rustEnvelopeHarnessRoot, "Cargo.toml")).isFile(), "isolated Rust persisted-envelope harness manifest must exist");
assert(statSync(join(rustEnvelopeHarnessRoot, "Cargo.lock")).isFile(), "isolated Rust persisted-envelope harness lock must exist");
assert(statSync(join(rustEnvelopeHarnessRoot, "src", "main.rs")).isFile(), "isolated Rust persisted-envelope harness source must exist");
const rustHarnessManifest = read(join(rustEnvelopeHarnessRoot, "Cargo.toml"));
const rustHarnessLock = read(join(rustEnvelopeHarnessRoot, "Cargo.lock"));
assert(rustHarnessManifest.includes('prost = "=0.14.1"'), "Rust harness must exactly pin prost 0.14.1");
assert(rustHarnessManifest.includes('prost-reflect = "=0.16.5"'), "Rust harness must exactly pin prost-reflect 0.16.5");
assert(rustHarnessLock.includes('name = "prost-reflect"\nversion = "0.16.5"'), "Rust harness lock must retain prost-reflect 0.16.5");
const goldenTests = spawnSync(process.execPath, [goldenTestPath], { cwd: repositoryRoot, encoding: "utf8", stdio: "pipe" });
if (goldenTests.status !== 0) errors.push(`${goldenTests.stdout ?? ""}${goldenTests.stderr ?? ""}`.trim());
const messageSyncTests = spawnSync(process.execPath, [messageSyncTestPath], { cwd: repositoryRoot, encoding: "utf8", stdio: "pipe" });
if (messageSyncTests.status !== 0) errors.push(`${messageSyncTests.stdout ?? ""}${messageSyncTests.stderr ?? ""}`.trim());
const cryptoSemanticFixtureTests = spawnSync(process.execPath, [cryptoSemanticFixtureTestPath], { cwd: repositoryRoot, encoding: "utf8", stdio: "pipe" });
if (cryptoSemanticFixtureTests.status !== 0) errors.push(`${cryptoSemanticFixtureTests.stdout ?? ""}${cryptoSemanticFixtureTests.stderr ?? ""}`.trim());

if (errors.length > 0) {
  console.error(errors.map((error) => `- ${error}`).join("\n"));
  process.exit(1);
}

const installTests = spawnSync(process.execPath, [installTestPath], { cwd: repositoryRoot, encoding: "utf8", stdio: "pipe" });
if (installTests.status !== 0) {
  console.error(`${installTests.stdout ?? ""}${installTests.stderr ?? ""}`);
  process.exit(installTests.status ?? 1);
}

console.log("Threadline contract structure and representative Golden Frames are valid.");
console.log("Threadline T015 message/sync behavior fixtures are valid.");
console.log("Threadline T011 Crypto semantic fixture manifest is valid.");
console.log("Threadline codegen repository failure-injection tests are valid.");
