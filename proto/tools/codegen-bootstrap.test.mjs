import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { verifiedBootstrapNode } from "./codegen-bootstrap.mjs";

const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");

test("bootstrap authenticates Node bytes before execution and denies path/digest substitution", (context) => {
  const root = mkdtempSync(join(tmpdir(), "threadline-bootstrap-"));
  context.after(() => rmSync(root, { recursive: true, force: true }));
  const node = join(root, "closures/node/bin/node");
  mkdirSync(join(root, "closures/node/bin"), { recursive: true });
  writeFileSync(node, "synthetic unexecuted fixture");
  const manifest = { tools: { node: { path: "closures/node/bin/node", invocation: "native", sha256: sha256(readFileSync(node)) } } };
  const record = (value) => {
    const bytes = JSON.stringify(value);
    writeFileSync(join(root, "manifest.json"), bytes);
    return sha256(bytes);
  };
  const digest = record(manifest);
  assert.equal(verifiedBootstrapNode(root, digest), realpathSync(node));
  assert.throws(() => verifiedBootstrapNode(root, "0".repeat(64)), /manifest digest/u);
  for (const change of [{ path: "/usr/bin/node" }, { invocation: "verified-node" }]) {
    assert.throws(() => verifiedBootstrapNode(root, record({ tools: { node: { ...manifest.tools.node, ...change } } })), /fixed native Node path/u);
  }
  record(manifest);
  writeFileSync(node, "changed");
  assert.throws(() => verifiedBootstrapNode(root, digest), /executable digest/u);
  rmSync(node);
  mkdirSync(node);
  assert.throws(() => verifiedBootstrapNode(root, digest), /digest\/type mismatch/u);
  if (process.platform !== "win32") {
    renameSync(join(root, "closures"), join(root, "original"));
    symlinkSync(join(root, "original"), join(root, "closures"));
    assert.throws(() => verifiedBootstrapNode(root, digest), /must not contain symlinks/u);
  }
});
