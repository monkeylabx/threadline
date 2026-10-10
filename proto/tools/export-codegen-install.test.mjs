import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { outputDefinitions } from "./codegen-output.mjs";
import { exportCodegenInstallation, verifyInstallArtifact } from "./export-codegen-install.mjs";

const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");
const lock = JSON.parse(readFileSync(new URL("../toolchain.lock.json", import.meta.url)));

function fixture(context) {
  const root = mkdtempSync(join(tmpdir(), "threadline-sdk-export-"));
  context.after(() => rmSync(root, { recursive: true, force: true }));
  const repositoryRoot = join(root, "repo");
  const bundle = join(root, "bundle");
  mkdirSync(repositoryRoot);
  mkdirSync(join(bundle, "bin"), { recursive: true });
  const git = join(bundle, "bin", "git");
  // A synthetic tool fixture; formal release evidence comes from the protected runner.
  writeFileSync(git, '#!/bin/sh\nexec /usr/bin/git "$@"\n');
  chmodSync(git, 0o755);
  const env = {
    PATH: "/usr/bin:/bin:/usr/sbin:/sbin", HOME: root, GIT_CONFIG_NOSYSTEM: "1", GIT_CONFIG_GLOBAL: "/dev/null",
    GIT_AUTHOR_DATE: "2000-01-01T00:00:00Z", GIT_COMMITTER_DATE: "2000-01-01T00:00:00Z",
  };
  const init = spawnSync(git, ["-C", repositoryRoot, "init", "--quiet", "--object-format=sha1"], { env, encoding: "utf8" });
  assert.equal(init.status, 0, init.stderr);
  const commit = spawnSync(git, ["-C", repositoryRoot, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "--quiet", "--allow-empty", "-m", "baseline"], { env, encoding: "utf8" });
  assert.equal(commit.status, 0, commit.stderr);
  const targetSha = spawnSync(git, ["-C", repositoryRoot, "rev-parse", "HEAD"], { env, encoding: "utf8" }).stdout.trim();
  const manifest = JSON.stringify({ profile: "release", tools: { git: { path: "bin/git", sha256: digest(readFileSync(git)) } } });
  writeFileSync(join(bundle, "manifest.json"), manifest);
  const toolchain = structuredClone(lock);
  const content = "// GENERATED\nStructure\n";
  for (const [name, output] of outputDefinitions(toolchain)) {
    mkdirSync(join(repositoryRoot, output), { recursive: true });
    writeFileSync(join(repositoryRoot, output, "fixture.source"), content);
    toolchain.generationChecks[name] = { fileCount: 1, treeSha256: digest(`file\0fixture.source\0${digest(content)}\n`), extensions: [".source"], signatureRegexAny: ["GENERATED"], structureRegex: ["Structure"] };
  }
  const identity = { targetPr: "226", targetSha, preparedRunId: "1", installRunId: "2", manifestSha256: digest(manifest) };
  return { repositoryRoot, bundle, outputRoot: join(root, "export"), identity, toolchain };
}

test("repository export contains only the six locked trees and a bound patch", { skip: process.platform === "win32" }, (context) => {
  const options = fixture(context);
  writeFileSync(join(options.repositoryRoot, "unrelated.txt"), "not part of this SDK task\n");
  const evidence = exportCodegenInstallation(options);
  const patch = readFileSync(join(options.outputRoot, "codegen-sdk.patch"));
  verifyInstallArtifact(evidence, patch, options);
  assert.equal(evidence.installed, true);
  assert.equal(evidence.status, "passed");
  for (const [, output] of outputDefinitions(options.toolchain)) assert.ok(patch.toString().includes(`${output}/fixture.source`));
  assert.equal(patch.toString().includes("unrelated.txt"), false);
  for (const [name, value] of Object.entries(options.identity)) {
    assert.throws(() => verifyInstallArtifact({ ...evidence, [name]: `${value}-wrong` }, patch, options), /identity mismatch/u);
  }
  assert.throws(() => verifyInstallArtifact(evidence, Buffer.from("tampered"), options), /patch digest/u);
  for (const change of [{ status: "candidate-awaiting-review" }, { installed: false }, { schemaVersion: 2 }]) {
    assert.throws(() => verifyInstallArtifact({ ...evidence, ...change }, patch, options), /completed repository installation/u);
  }
  assert.throws(() => verifyInstallArtifact({ ...evidence, generationTrees: {} }, patch, options), /tree inventory/u);
});

test("export refuses changed identity, Git bytes, stale trees, and caller-owned output", { skip: process.platform === "win32" }, (context) => {
  const options = fixture(context);
  assert.throws(() => exportCodegenInstallation({ ...options, identity: { ...options.identity, manifestSha256: "0".repeat(64) } }), /manifest digest/u);
  assert.throws(() => exportCodegenInstallation({ ...options, identity: { ...options.identity, targetSha: "0".repeat(40) } }), /target commit/u);
  assert.equal(existsSync(options.outputRoot), false);
  const source = join(options.repositoryRoot, options.toolchain.outputs.go, "fixture.source");
  writeFileSync(source, "// GENERATED\nStructure\nchanged\n");
  assert.throws(() => exportCodegenInstallation(options), /exact tree mismatch/u);
  writeFileSync(join(options.bundle, "bin", "git"), "not the reviewed executable");
  assert.throws(() => exportCodegenInstallation(options), /executable digest/u);
  mkdirSync(options.outputRoot);
  writeFileSync(join(options.outputRoot, "marker"), "preserve");
  assert.throws(() => exportCodegenInstallation(options), /new directory/u);
  assert.equal(readFileSync(join(options.outputRoot, "marker"), "utf8"), "preserve");
  assert.throws(() => exportCodegenInstallation({ ...options, outputRoot: join(options.repositoryRoot, "export") }), /outside the repository/u);
  const alias = join(options.repositoryRoot, "..", "repo-alias");
  symlinkSync(options.repositoryRoot, alias);
  assert.throws(() => exportCodegenInstallation({ ...options, outputRoot: join(alias, "export") }), /outside the repository/u);
});

test("export rejects unauthenticated, escaped, and linked Git tools", { skip: process.platform === "win32" }, (context) => {
  const options = fixture(context);
  const manifestPath = join(options.bundle, "manifest.json");
  const original = JSON.parse(readFileSync(manifestPath));
  const reject = (manifest, pattern) => {
    const bytes = JSON.stringify(manifest);
    writeFileSync(manifestPath, bytes);
    assert.throws(() => exportCodegenInstallation({ ...options, identity: { ...options.identity, manifestSha256: digest(bytes) } }), pattern);
  };
  reject({ ...original, profile: "fixture" }, /reviewed release manifest/u);
  reject({ ...original, tools: {} }, /reviewed release manifest/u);
  reject({ ...original, tools: { git: { ...original.tools.git, path: "/usr/bin/git" } } }, /escapes its bundle/u);
  symlinkSync(join(options.bundle, "bin", "git"), join(options.bundle, "bin", "git-link"));
  reject({ ...original, tools: { git: { ...original.tools.git, path: "bin/git-link" } } }, /digest\/type mismatch/u);
  const git = join(options.bundle, "bin", "git");
  writeFileSync(git, "#!/bin/sh\nexit 1\n");
  reject({ ...original, tools: { git: { ...original.tools.git, sha256: digest(readFileSync(git)) } } }, /Git failed/u);
  chmodSync(git, 0o600);
  reject({ ...original, tools: { git: { ...original.tools.git, sha256: digest(readFileSync(git)) } } }, /EACCES/u);
});
