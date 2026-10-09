import assert from "node:assert/strict";
import { test } from "node:test";
import { bundleToolNames, generationToolNames } from "./codegen-bundle-tools.mjs";

test("read-only bundles retain the exact generation tools", () => {
  const tools = Object.fromEntries(generationToolNames.map((name) => [name, {}]));
  assert.deepEqual(bundleToolNames(tools), generationToolNames);
  assert.deepEqual(bundleToolNames({ ...tools, git: {} }), [...generationToolNames, "git"]);
  assert.throws(() => bundleToolNames({ ...tools, shell: {} }), /exactly/u);
  assert.throws(() => bundleToolNames({ ...tools, git: {}, shell: {} }), /exactly/u);
  delete tools.node;
  assert.throws(() => bundleToolNames(tools), /exactly/u);
  assert.throws(() => bundleToolNames({ ...tools, git: {} }), /exactly/u);
});
