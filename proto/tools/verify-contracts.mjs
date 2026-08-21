import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { readFileSync, readdirSync, statSync } from "node:fs";
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

function decodeVarint(bytes, start) {
  let value = 0;
  let shift = 0;
  for (let index = start; index < bytes.length && shift <= 49; index += 1) {
    const byte = bytes[index];
    value += (byte & 0x7f) * 2 ** shift;
    if ((byte & 0x80) === 0) return { value, next: index + 1 };
    shift += 7;
  }
  throw new Error("invalid or oversized varint");
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
assert(toolchain.tools.javaVendor === "Eclipse Adoptium" && workspaceToolchain.android.javaVendor === "Temurin", "Contract codegen JDK vendor must match the workspace Temurin pin");
assert(toolchain.scope.startsWith("Contract-specific"), "Proto lock must state its contract-only boundary");
assert(Object.keys(toolchain.outputs).length === 5, "exactly five language outputs must be pinned");
const expectedGenerationPlan = {
  version: "v2",
  plugins: [
    { local: "protoc-gen-go", out: toolchain.outputs.go, opt: ["paths=source_relative"] },
    { local: "protoc-gen-es", out: toolchain.outputs.typescript, opt: ["target=ts"] },
    { local: "protoc-gen-prost", out: toolchain.outputs.rust, opt: ["bytes=."] },
    { local: "protoc-gen-swift", out: toolchain.outputs.swift },
    { protoc_builtin: "java", protoc_path: "protoc", out: toolchain.outputs.kotlin.javaMessages },
    { protoc_builtin: "kotlin", protoc_path: "protoc", out: toolchain.outputs.kotlin.kotlinDsl },
  ],
  inputs: [{ directory: "proto" }],
};
assert(JSON.stringify(generationPlan) === JSON.stringify(expectedGenerationPlan), "buf.gen.yaml must exactly match the pinned five-language generation plan");
for (const plugin of ["protoc-gen-go", "protoc-gen-es", "protoc-gen-prost", "protoc-gen-swift"]) {
  assert(typeof toolchain.tools[plugin] === "string", `${plugin} version must be pinned`);
}
assert(typeof toolchain.tools.git === "string", "repository-mode Git version must be pinned");
for (const runtime of ["kotlin-compiler", "protobuf-java", "protobuf-kotlin", "kotlin-stdlib"]) {
  assert(typeof toolchain.tools[runtime] === "string", `${runtime} version must be pinned`);
}
assert(toolchain.integrity.toolManifestSchemaVersion === 4, "codegen tool manifest schema must be pinned to v4");
assert(JSON.stringify(toolchain.integrity.toolManifestSchema) === JSON.stringify({
  manifestKeys: ["schemaVersion", "platform", "profile", "sources", "closures", "tools"],
  sourceKeys: ["kind", "file", "url", "sha256"],
  builderSourceKeys: ["kind", "file", "url", "sha256", "authentication"],
  rustChecksumAuthenticationKeys: ["kind", "file", "url", "sha256"],
  appleInstallerAuthenticationKeys: ["kind", "signer"],
  closureKeys: ["root", "sources", "treeSha256", "files"],
  toolKeys: ["path", "sha256", "closure", "provenance", "invocation"],
  officialProvenanceKeys: ["kind", "sources"],
  sourceBuiltProvenanceKeys: ["kind", "source", "builders", "buildCommand", "reproducibility"],
  protocolStubProvenanceKeys: ["kind", "sources"],
  sourceKinds: ["builder-toolchain", "official-binary", "official-package", "protocol-fixture", "source-archive"],
  provenanceKinds: ["official-binary", "official-package", "protocol-stub", "source-built"],
  sourceBuiltReproducibility: "single-build-output-verified",
}), "codegen tool manifest v4 vocabulary must remain exact");
for (const collection of [toolchain.integrity.runtimeJars, toolchain.integrity.kotlinCompilerClasspath]) {
  for (const [artifact, integrity] of Object.entries(collection)) {
    assert(/^[0-9a-f]{64}$/u.test(integrity.sha256), `${artifact} must have a canonical SHA-256 pin`);
    assert(/^https:\/\/repo1\.maven\.org\/maven2\//u.test(integrity.source), `${artifact} must record its Maven Central source`);
    assert(/^[0-9a-f]{40}$/u.test(integrity.sourceSha1), `${artifact} must record the published source SHA-1`);
  }
}
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
  assert(Array.isArray(checks.expectedRelativePaths) && checks.expectedRelativePaths.length > 0, `${surface} must pin the exact generated file set`);
  assert(new Set(checks.expectedRelativePaths).size === checks.expectedRelativePaths.length, `${surface} expected generated paths must be unique`);
  assert(JSON.stringify(checks.expectedRelativePaths) === JSON.stringify([...checks.expectedRelativePaths].sort()), `${surface} expected generated paths must be sorted`);
  assert(checks.expectedRelativePaths.every((path) => !path.startsWith("/") && !path.includes("\\") && !path.split("/").includes("..")), `${surface} expected generated paths must be canonical relative paths`);
  assert(Array.isArray(checks.signatureRegex) && checks.signatureRegex.length > 0, `${surface} must pin real-generator signatures`);
  assert(Array.isArray(checks.structureRegex) && checks.structureRegex.length > 0, `${surface} must pin ErrorEnvelope output structure`);
}
assert(statSync(join(protoRoot, "tools", "verify-codegen.mjs")).isFile(), "the verified codegen command must exist");
const installTestPath = join(protoRoot, "tools", "verify-codegen-install.test.mjs");
assert(statSync(installTestPath).isFile(), "repository codegen failure-injection tests must exist");

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
assert(manifest.schemaVersion === 1, "Golden Frame manifest schemaVersion must be 1");
assert(manifest.canaryFieldNumber === 50000, "Golden Frame unknown-field canary must remain field 50000");
assert(manifest.acceptanceBoundary.issue === 28, "Golden Frame acceptance boundary must identify T014");
assert(manifest.acceptanceBoundary.issueMayClose === false, "T014 must remain open until its concrete Golden Frame requirement is resolved");
assert(manifest.acceptanceBoundary.status === "blocked-on-representative-frames-and-cross-language-evidence", "T014 Golden Frame blocker must remain explicit");
assert(manifest.acceptanceBoundary.notSatisfiedHere.includes("representative-channel-event-envelope-frame"), "representative ChannelEventEnvelope frame must remain an explicit blocker");
assert(manifest.acceptanceBoundary.notSatisfiedHere.includes("representative-recovery-envelope-frame"), "representative RecoveryEnvelope frame must remain an explicit blocker");
assert(manifest.acceptanceBoundary.notSatisfiedHere.includes("cross-language-unknown-field-roundtrip"), "cross-language unknown-field evidence must remain an explicit blocker");
assert(manifest.acceptanceBoundary.notSatisfiedHere.includes("n-minus-one-compatibility"), "N-1 evidence must remain an explicit blocker");
assert(manifest.acceptanceBoundary.splitRequiresExplicitApprovalFrom.includes("Contracts") && manifest.acceptanceBoundary.splitRequiresExplicitApprovalFrom.includes("Product"), "moving the concrete frames out of T014 requires Contracts and Product approval");
assert(manifest.fixtures.length === 2, "Ciphertext and Crypto Envelope canaries are both required");

for (const fixture of manifest.fixtures) {
  const fixturePath = join(dirname(manifestPath), fixture.file);
  assert(statSync(fixturePath).isFile(), `${fixture.contract} fixture is missing`);
  const hex = read(fixturePath).trim();
  assert(/^(?:[0-9a-f]{2})+$/u.test(hex), `${fixture.contract} fixture must be canonical lowercase hex`);
  const bytes = Buffer.from(hex, "hex");
  const digest = createHash("sha256").update(bytes).digest("hex");
  assert(digest === fixture.sha256, `${fixture.contract} SHA-256 mismatch`);
  try {
    const tag = decodeVarint(bytes, 0);
    const length = decodeVarint(bytes, tag.next);
    const payload = bytes.subarray(length.next, length.next + length.value);
    assert(Math.floor(tag.value / 8) === manifest.canaryFieldNumber, `${fixture.contract} must use field 50000`);
    assert(tag.value % 8 === 2, `${fixture.contract} canary must be length-delimited`);
    assert(length.next + length.value === bytes.length, `${fixture.contract} must contain one complete field`);
    assert(payload.toString("utf8") === fixture.payloadUtf8, `${fixture.contract} payload mismatch`);
  } catch (error) {
    errors.push(`${fixture.contract} is not a valid Golden Frame canary: ${error.message}`);
  }
}

if (errors.length > 0) {
  console.error(errors.map((error) => `- ${error}`).join("\n"));
  process.exit(1);
}

const installTests = spawnSync(process.execPath, [installTestPath], { cwd: repositoryRoot, encoding: "utf8", stdio: "pipe" });
if (installTests.status !== 0) {
  console.error(`${installTests.stdout ?? ""}${installTests.stderr ?? ""}`);
  process.exit(installTests.status ?? 1);
}

console.log("Threadline contract structure and Golden Frame canaries are valid.");
console.log("Threadline codegen repository failure-injection tests are valid.");
