import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { createCollector, filesBelow, protoRoot, repositoryRoot } from "./support.mjs";

test("the collector records only failed assertions, in order", () => {
  const { assert: check, failures } = createCollector();
  check(true, "not recorded");
  check(false, "first");
  check(0, "second");
  check("non-empty", "not recorded either");
  assert.deepEqual(failures, ["first", "second"]);
});

test("filesBelow walks nested directories and filters by suffix", () => {
  const root = mkdtempSync(join(tmpdir(), "threadline-files-below-"));
  try {
    mkdirSync(join(root, "nested", "deeper"), { recursive: true });
    writeFileSync(join(root, "top.proto"), "");
    writeFileSync(join(root, "nested", "skip.txt"), "");
    writeFileSync(join(root, "nested", "deeper", "leaf.proto"), "");
    assert.deepEqual(filesBelow(root, ".proto").sort(), [join(root, "nested", "deeper", "leaf.proto"), join(root, "top.proto")]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("roots resolve to the proto module and the repository", () => {
  assert.ok(protoRoot.endsWith("/proto/"));
  assert.equal(repositoryRoot, protoRoot.replace(/proto\/$/u, ""));
});
