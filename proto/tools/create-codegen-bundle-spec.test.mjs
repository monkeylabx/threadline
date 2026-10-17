import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { generationToolNames } from "./codegen-bundle-tools.mjs";

// Empty synthetic inputs exercise spec selection, never release authentication.
const inputs = [
  "buf-Darwin-arm64.tar.gz", "protoc-35.1-osx-aarch_64.zip", "node-v24.19.0-darwin-arm64.tar.gz",
  "OpenJDK17U-jdk_aarch64_mac_hotspot_17.0.19_10.tar.gz", "protoc-gen-go.v1.36.11.darwin.arm64.tar.gz",
  "connect-go-1.20.0.tar.gz", "go1.26.5.darwin-arm64.tar.gz", "go-release.json",
  "protoc-gen-es-2.14.0.tgz", "protobuf-2.14.0.tgz", "protoplugin-2.14.0.tgz", "vfs-1.6.4.tgz",
  "typescript-5.4.5.tgz", "debug-4.4.3.tgz", "ms-2.1.3.tgz", "protoc-gen-prost-0.5.0.tar.gz",
  "rust-1.97.1-aarch64-apple-darwin.tar.xz", "rust-1.97.1-aarch64-apple-darwin.tar.xz.sha256",
  "swift-protobuf-1.38.1.tar.gz", "protoc-gen-connect-swift.tar.gz", "protoc-gen-connect-kotlin-0.9.0.jar",
];

function fixture(context) {
  const root = mkdtempSync(join(tmpdir(), "threadline-bundle-spec-"));
  context.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(join(root, "sources"));
  for (const input of inputs) writeFileSync(join(root, "sources", input), "synthetic\n");
  const output = join(root, "spec.json");
  const invoke = (includeGit) => spawnSync(process.execPath, [fileURLToPath(new URL("./create-codegen-bundle-spec.mjs", import.meta.url))], {
    encoding: "utf8", env: {
      THREADLINE_FORMAL_WORK_ROOT: root, THREADLINE_CODEGEN_BUNDLE_SPEC: output,
      THREADLINE_RUNNER_IMAGES_SHA: "8d3ea005fa2d87f3cbc9255c27fdfed9e901a043", THREADLINE_CODEGEN_INCLUDE_GIT: includeGit,
      NODE_V8_COVERAGE: process.env.NODE_V8_COVERAGE,
    },
  });
  return { root, output, invoke };
}

test("repository specs add only pinned source-built Git with the Xcode builder", (context) => {
  const fixtureState = fixture(context);
  assert.equal(fixtureState.invoke("false").status, 0);
  const generation = JSON.parse(readFileSync(fixtureState.output));
  assert.deepEqual(Object.keys(generation.tools).sort(), [...generationToolNames].sort());
  assert.equal(Object.hasOwn(generation.sources, "git-source"), false);
  writeFileSync(join(fixtureState.root, "sources/git-2.50.1.tar.gz"), "synthetic Git\n");
  assert.equal(fixtureState.invoke("true").status, 0);
  const repository = JSON.parse(readFileSync(fixtureState.output));
  assert.deepEqual(Object.keys(repository.tools).sort(), [...generationToolNames, "git"].sort());
  assert.equal(repository.tools.git.path, join(fixtureState.root, "closures/git/git"));
  assert.equal(repository.tools.git.provenance.kind, "source-built");
  assert.deepEqual(repository.tools.git.provenance.builders, ["xcode-builder"]);
  assert.equal(repository.sources["git-source"].url, "https://github.com/git/git/archive/refs/tags/v2.50.1.tar.gz");
  assert.deepEqual(repository.closures.git.sources, ["git-source", "xcode-builder"]);
  assert.equal(repository.sources["xcode-builder"].authentication.sdkVersion, "26.5");
});

test("repository specs reject invalid selection or missing/nonregular Git source", (context) => {
  const fixtureState = fixture(context);
  const reject = (selection, pattern) => {
    const result = fixtureState.invoke(selection);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, pattern);
  };
  reject("yes", /must be true or false/u);
  reject("true", /formal source input is missing/u);
  const gitSource = join(fixtureState.root, "sources/git-2.50.1.tar.gz");
  mkdirSync(gitSource);
  reject("true", /formal source input is missing/u);
  rmSync(gitSource, { recursive: true });
  if (process.platform !== "win32") {
    symlinkSync(join(fixtureState.root, "sources", inputs[0]), gitSource);
    reject("true", /formal source input is missing/u);
  }
});
