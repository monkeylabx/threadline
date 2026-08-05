import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { pins, resolveProbeInvocation, validateWorkflowPins } from "./toolchain.mjs";

const root = fileURLToPath(new URL("../", import.meta.url));
const workflow = readFileSync(join(root, ".github", "workflows", "build.yml"), "utf8");

test("the checked-in workflow has no toolchain pin drift", () => {
  assert.deepEqual(validateWorkflowPins(workflow), []);
});

test("one drifted repeated Corepack pin fails verification", () => {
  const drifted = workflow.replace(`corepack@${pins.corepack}`, "corepack@0.0.0");
  assert.match(validateWorkflowPins(drifted).join("\n"), /CI Corepack: expected .* got 0\.0\.0/);
});

test("one drifted repeated pnpm pin fails verification", () => {
  const drifted = workflow.replace(`pnpm@${pins.pnpm}`, "pnpm@0.0.0");
  assert.match(validateWorkflowPins(drifted).join("\n"), /CI pnpm: expected .* got 0\.0\.0/);
});

test("a missing repeated Corepack setup fails verification", () => {
  const missing = workflow.replace(`npm install --global corepack@${pins.corepack}`, "");
  assert.match(
    validateWorkflowPins(missing).join("\n"),
    /CI Corepack occurrence count: expected \d+, got \d+/,
  );
});

test("one drifted repeated action pin fails verification", () => {
  const expected = pins.actions["actions/setup-node"];
  const drifted = workflow.replace(
    `actions/setup-node@${expected}`,
    `actions/setup-node@${"0".repeat(40)}`,
  );
  assert.match(validateWorkflowPins(drifted).join("\n"), /CI action actions\/setup-node/);
});

test("Windows Corepack runs its JavaScript entrypoint without a shell", (context) => {
  const prefix = mkdtempSync(join(tmpdir(), "threadline-corepack-"));
  context.after(() => rmSync(prefix, { recursive: true, force: true }));
  const entrypoint = join(prefix, "node_modules", "corepack", "dist", "corepack.js");
  mkdirSync(join(prefix, "node_modules", "corepack", "dist"), { recursive: true });
  writeFileSync(entrypoint, "");

  assert.deepEqual(
    resolveProbeInvocation("corepack", ["--version"], {
      platform: "win32",
      pathValue: prefix,
      nodeExecutable: "C:\\node\\node.exe",
    }),
    {
      command: "C:\\node\\node.exe",
      args: [entrypoint, "--version"],
    },
  );
});
