import { statSync } from "node:fs";
import { join } from "node:path";

import { protoRoot, read, repositoryRoot } from "./support.mjs";

// Pins the contract toolchain: Buf configuration, the five-language generation
// plan, codegen tool/manifest vocabulary and the canonical command set. Every
// value here is a frozen policy; a change is a reviewed contract decision.

const trustedLaunchPrefix = "<approved-clean-env-launcher> <bundle-absolute-node> proto/tools/verify-codegen.mjs";

const toolManifestSchema = {
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
};

const pinnedPlugins = [
  "protoc-gen-go",
  "protoc-gen-connect-go",
  "protoc-gen-es",
  "protoc-gen-prost",
  "protoc-gen-prost-crate",
  "protoc-gen-swift",
  "protoc-gen-connect-swift",
  "protoc-gen-connect-kotlin",
];

const pinnedRuntimes = ["kotlin-compiler", "protobuf-java", "protobuf-kotlin", "kotlin-stdlib"];

function expectedGenerationPlan(outputs) {
  return {
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
      { local: "protoc-gen-go", out: outputs.go, opt: ["paths=source_relative"] },
      { local: "protoc-gen-connect-go", out: outputs.go, opt: ["paths=source_relative"] },
      { local: "protoc-gen-es", out: outputs.typescript, opt: ["target=ts", "import_extension=.js"] },
      { local: "protoc-gen-prost", out: outputs.rust, opt: ["bytes=.", "compile_well_known_types=false"] },
      { local: "protoc-gen-prost-crate", out: outputs.rust, opt: ["no_features"], strategy: "all" },
      { local: "protoc-gen-swift", out: outputs.swift, opt: ["Visibility=Public", "FileNaming=PathToUnderscores"] },
      { local: "protoc-gen-connect-swift", out: outputs.swift, opt: ["Visibility=Public", "FileNaming=PathToUnderscores"] },
      { protoc_builtin: "java", protoc_path: "protoc", out: outputs.kotlin.javaMessages },
      { protoc_builtin: "kotlin", protoc_path: "protoc", out: outputs.kotlin.kotlinDsl },
      { local: "protoc-gen-connect-kotlin", out: outputs.kotlin.kotlinDsl },
    ],
    inputs: [{ directory: "proto" }],
  };
}

function checkBufConfiguration(assert, bufYaml, generationYaml) {
  const combinedConfig = `${bufYaml}\n${generationYaml}`;
  assert(!/buf\.build|\bremote:|\bdeps:|\bmodule:/u.test(combinedConfig), "Buf configuration must not use the public BSR");
  assert(
    /breaking:\s*\n\s+use:\s*\n\s+- FILE/u.test(bufYaml),
    "buf.formal.yaml must enforce FILE breaking rules",
  );
  assert(/disallow_comment_ignores:\s*true/u.test(bufYaml), "Buf lint comment ignores must remain disabled");
}

function checkWorkspacePins(assert, toolchain, workspaceToolchain) {
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
}

function checkGenerationPlan(assert, toolchain, generationPlan) {
  assert(JSON.stringify(generationPlan) === JSON.stringify(expectedGenerationPlan(toolchain.outputs)), "buf.gen.yaml must exactly match the pinned five-language generation plan");
  for (const plugin of pinnedPlugins) {
    assert(typeof toolchain.tools[plugin] === "string", `${plugin} version must be pinned`);
  }
  assert(typeof toolchain.tools.git === "string", "repository-mode Git version must be pinned");
  for (const runtime of pinnedRuntimes) {
    assert(typeof toolchain.tools[runtime] === "string", `${runtime} version must be pinned`);
  }
}

function checkCodegenIntegrity(assert, toolchain) {
  assert(toolchain.integrity.toolManifestSchemaVersion === 5, "codegen tool manifest schema must be pinned to v5");
  assert(JSON.stringify(toolchain.integrity.toolManifestSchema) === JSON.stringify(toolManifestSchema), "codegen tool manifest v5 vocabulary must remain exact");
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
}

function checkContractCommands(assert, toolchain) {
  assert(JSON.stringify(Object.keys(toolchain.commands).sort()) === JSON.stringify(["breaking", "generate", "lint", "protocolSmoke", "verify", "verifyCodegen"]), "contract command set must remain exact");
  assert(toolchain.commands.lint === "buf lint", "contract lint command must remain canonical");
  assert(toolchain.commands.breaking === "buf breaking --against '.git#branch=main'", "contract breaking command must remain canonical");
  assert(toolchain.commands.verify === "node proto/tools/verify-contracts.mjs", "repository structural verification command must remain canonical");
  assert(toolchain.commands.generate === `${trustedLaunchPrefix} --mode=repository`, "verified repository generation must use the trusted bootstrap contract");
  assert(toolchain.commands.verifyCodegen === `${trustedLaunchPrefix} --mode=verify-only`, "release codegen verification must use the trusted bootstrap contract");
  assert(toolchain.commands.protocolSmoke === `${trustedLaunchPrefix} --mode=protocol-smoke`, "protocol smoke must use the same trusted bootstrap contract while remaining non-release");
}

function checkGenerationChecks(assert, toolchain) {
  assert(Object.keys(toolchain.generationChecks).length === 6, "all six generated source surfaces must have formal output checks");
  for (const [surface, checks] of Object.entries(toolchain.generationChecks)) {
    assert(Array.isArray(checks.extensions) && checks.extensions.length > 0, `${surface} must pin generated source extensions`);
    assert(Number.isSafeInteger(checks.fileCount) && checks.fileCount > 0, `${surface} must pin the generated file count`);
    assert(/^[0-9a-f]{64}$/u.test(checks.treeSha256), `${surface} must pin the exact generated tree SHA-256`);
    assert(Array.isArray(checks.signatureRegexAny) && checks.signatureRegexAny.length > 0, `${surface} must pin accepted real-generator signatures`);
    assert(Array.isArray(checks.structureRegex) && checks.structureRegex.length > 0, `${surface} must pin authoritative contract output structure`);
  }
  assert(statSync(join(protoRoot, "tools", "verify-codegen.mjs")).isFile(), "the verified codegen command must exist");
  assert(statSync(join(protoRoot, "tools", "verify-codegen-install.test.mjs")).isFile(), "repository codegen failure-injection tests must exist");
}

export function checkToolchainPolicy(assert) {
  const bufYaml = read(join(protoRoot, "buf.formal.yaml"));
  const generationYaml = read(join(protoRoot, "buf.gen.yaml"));
  const toolchain = JSON.parse(read(join(protoRoot, "toolchain.lock.json")));
  const workspaceToolchain = JSON.parse(read(join(repositoryRoot, "toolchains.json")));
  checkBufConfiguration(assert, bufYaml, generationYaml);
  checkWorkspacePins(assert, toolchain, workspaceToolchain);
  checkGenerationPlan(assert, toolchain, JSON.parse(generationYaml));
  checkCodegenIntegrity(assert, toolchain);
  checkContractCommands(assert, toolchain);
  checkGenerationChecks(assert, toolchain);
}
