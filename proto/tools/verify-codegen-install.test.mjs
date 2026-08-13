import assert from "node:assert/strict";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

for (const name of Object.keys(process.env)) {
  if (name === "NODE_OPTIONS" || name === "NODE_PATH" || name === "LD_PRELOAD" || name === "JAVA_TOOL_OPTIONS" || name === "JDK_JAVA_OPTIONS" || name === "_JAVA_OPTIONS" || name.startsWith("DYLD_") || name.startsWith("GIT_")) {
    delete process.env[name];
  }
}
const outputs = [
  "services/gen/proto",
  "packages/generated-ts/src",
  "crates/generated-proto/src/generated",
  "packages/generated-swift/Sources/ThreadlineGenerated",
  "packages/generated-kotlin/src/main/java",
  "packages/generated-kotlin/src/main/kotlin",
];

function writeTree(root, marker) {
  for (const output of outputs) {
    const directory = join(root, output);
    mkdirSync(directory, { recursive: true });
    writeFileSync(join(directory, "fixture.txt"), `${marker}:${output}\n`, "utf8");
  }
}

async function fixture() {
  const root = mkdtempSync(join(tmpdir(), "threadline-codegen-install-test-"));
  mkdirSync(join(root, ".git"));
  mkdirSync(join(root, "proto", "tools"), { recursive: true });
  cpSync(new URL("./verify-codegen.mjs", import.meta.url), join(root, "proto", "tools", "verify-codegen.mjs"));
  cpSync(new URL("../toolchain.lock.json", import.meta.url), join(root, "proto", "toolchain.lock.json"));
  cpSync(new URL("../../buf.gen.yaml", import.meta.url), join(root, "buf.gen.yaml"));
  cpSync(new URL("../../toolchains.json", import.meta.url), join(root, "toolchains.json"));
  const fakeGit = join(root, "fake-git.mjs");
  writeFileSync(fakeGit, "// A clean git-status double intentionally writes no output.\n", "utf8");
  const module = await import(`${pathToFileURL(join(root, "proto", "tools", "verify-codegen.mjs")).href}?fixture=${Date.now()}-${Math.random()}`);
  return { root, fakeGit, ...module };
}

async function testExclusiveLock() {
  const context = await fixture();
  try {
    const release = context.acquireRepositoryLock();
    assert.throws(() => context.acquireRepositoryLock(), /another cooperative Threadline codegen installation holds/u);
    assert.throws(() => context.acquireRepositoryLock(), /another cooperative Threadline codegen installation holds/u, "a failed contender must not delete the owner's lock");
    release();
    const releaseAfter = context.acquireRepositoryLock();
    releaseAfter();
  } finally {
    rmSync(context.root, { recursive: true, force: true });
  }
}

async function testRenameFailureRollback() {
  const context = await fixture();
  try {
    writeTree(context.root, "old");
    const generated = join(context.root, "generated");
    writeTree(generated, "new");
    assert.throws(() => context.synchronizeRepositoryOutputs(
      generated,
      [process.execPath, context.fakeGit],
      {},
      { afterBackup: () => { throw new Error("injected rename failure"); } },
    ), /injected rename failure/u);
    for (const output of outputs) {
      assert.equal(readFileSync(join(context.root, output, "fixture.txt"), "utf8"), `old:${output}\n`);
    }
  } finally {
    rmSync(context.root, { recursive: true, force: true });
  }
}

async function testSymlinkEscapeRejected() {
  const context = await fixture();
  try {
    const outside = mkdtempSync(join(tmpdir(), "threadline-codegen-outside-"));
    const generated = join(context.root, "generated");
    writeTree(generated, "new");
    mkdirSync(join(context.root, "services"));
    symlinkSync(outside, join(context.root, "services", "gen"));
    assert.throws(() => context.synchronizeRepositoryOutputs(generated, [process.execPath, context.fakeGit], {}), /contains a symlink/u);
    assert.equal(existsSync(join(outside, "proto")), false, "rejected installation must not write through the symlink");
    rmSync(outside, { recursive: true, force: true });
  } finally {
    rmSync(context.root, { recursive: true, force: true });
  }
}

await testExclusiveLock();
await testRenameFailureRollback();
await testSymlinkEscapeRejected();
console.log("Threadline codegen repository lock, rollback, and symlink tests passed.");
