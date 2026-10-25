import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { protoRoot, read, repositoryRoot } from "./support.mjs";

// Fault injection against the formal codegen bundle builder: it must refuse a
// pre-existing output directory and leave the caller's files untouched.

const bundleBuilderPath = join(protoRoot, "tools", "create-codegen-bundle.mjs");
const bundleSpecBuilderPath = join(protoRoot, "tools", "create-codegen-bundle-spec.mjs");

const refusalSpec = {
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
};

function checkBuilderSource(assert) {
  assert(statSync(bundleBuilderPath).isFile(), "formal codegen bundle builder must exist");
  const bundleBuilderSource = read(bundleBuilderPath);
  assert(!bundleBuilderSource.includes("rmSync(outputRoot"), "formal bundle builder must never recursively replace a caller-selected output directory");
  assert(bundleBuilderSource.includes("bundle output must be a new, non-existing directory"), "formal bundle builder must refuse every pre-existing output directory");
}

function probeExistingOutputRefusal(assert) {
  const bundleRefusalRoot = mkdtempSync(join(tmpdir(), "threadline-bundle-refusal-"));
  try {
    const existingOutput = join(bundleRefusalRoot, "existing-output");
    const marker = join(existingOutput, "owned-by-caller.txt");
    const spec = join(bundleRefusalRoot, "spec.json");
    mkdirSync(existingOutput);
    writeFileSync(marker, "must survive\n");
    writeFileSync(spec, `${JSON.stringify(refusalSpec)}\n`);
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
}

export function checkBundleBuilderRefusal(assert) {
  checkBuilderSource(assert);
  probeExistingOutputRefusal(assert);
  assert(statSync(bundleSpecBuilderPath).isFile(), "formal codegen bundle spec builder must exist");
}
