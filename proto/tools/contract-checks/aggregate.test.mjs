import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { runAggregate } from "./aggregate.mjs";
import { runRegisteredVerifiers } from "./verifier-runner.mjs";

// Deterministic stand-in for spawnSync keyed by script path.
function fakeSpawn(outcomes) {
  const calls = [];
  const spawn = (command, [path], options) => {
    calls.push({ command, path, cwd: options.cwd });
    return outcomes[path] ?? { status: 0, stdout: "", stderr: "" };
  };
  return { spawn, calls };
}

const passingPlan = {
  checks: [(check) => check(true, "unused")],
  verifiers: [{ path: "a.mjs" }, { path: "b.mjs" }],
  installTestPath: "install.mjs",
  successLines: ["first ok", "second ok"],
};

test("a fully passing plan exits 0 and prints the success lines in order", () => {
  const { spawn, calls } = fakeSpawn({});
  const result = runAggregate(passingPlan, { spawn, cwd: "/repo" });
  assert.deepEqual(result, { exitCode: 0, stdout: ["first ok", "second ok"], stderr: [] });
  assert.deepEqual(calls.map((call) => call.path), ["a.mjs", "b.mjs", "install.mjs"]);
  assert.ok(calls.every((call) => call.command === process.execPath && call.cwd === "/repo"));
});

test("one failing registered verifier fails the aggregate before the install test runs", () => {
  const { spawn, calls } = fakeSpawn({ "b.mjs": { status: 1, stdout: "child stdout\n", stderr: "child stderr\n" } });
  const result = runAggregate(passingPlan, { spawn });
  assert.equal(result.exitCode, 1);
  assert.deepEqual(result.stdout, []);
  assert.deepEqual(result.stderr, ["- child stdout\nchild stderr"]);
  assert.ok(!calls.some((call) => call.path === "install.mjs"), "install test must not run after a failure");
});

test("policy failures are reported first and in observation order, followed by verifier failures", () => {
  const plan = {
    ...passingPlan,
    checks: [(check) => { check(false, "policy one"); check(true, "fine"); }, (check) => check(false, "policy two")],
  };
  const { spawn } = fakeSpawn({ "a.mjs": { status: 2, stdout: "", stderr: "a failed" } });
  const result = runAggregate(plan, { spawn });
  assert.equal(result.exitCode, 1);
  assert.deepEqual(result.stderr, ["- policy one\n- policy two\n- a failed"]);
});

test("a failing policy check still runs every registered verifier so one run reports everything", () => {
  const plan = { ...passingPlan, checks: [(check) => check(false, "policy broke")] };
  const { spawn, calls } = fakeSpawn({});
  runAggregate(plan, { spawn });
  assert.deepEqual(calls.map((call) => call.path), ["a.mjs", "b.mjs"]);
});

test("the install test's own exit status and raw output propagate unchanged", () => {
  const { spawn } = fakeSpawn({ "install.mjs": { status: 3, stdout: "out\n", stderr: "err\n" } });
  const result = runAggregate(passingPlan, { spawn });
  assert.deepEqual(result, { exitCode: 3, stdout: [], stderr: ["out\nerr\n"] });
});

test("an install test killed by a signal exits 1 rather than 0", () => {
  const { spawn } = fakeSpawn({ "install.mjs": { status: null, signal: "SIGKILL", stdout: "", stderr: "" } });
  const result = runAggregate(passingPlan, { spawn });
  assert.equal(result.exitCode, 1);
});

test("the real spawn seam propagates a child process failure", () => {
  const root = mkdtempSync(join(tmpdir(), "threadline-verifier-runner-"));
  try {
    const passing = join(root, "pass.mjs");
    const failing = join(root, "fail.mjs");
    writeFileSync(passing, "console.log('pass');\n");
    writeFileSync(failing, "console.error('broken invariant'); process.exit(1);\n");
    assert.deepEqual(runRegisteredVerifiers([{ path: passing }], { cwd: root }), []);
    assert.deepEqual(runRegisteredVerifiers([{ path: passing }, { path: failing }], { cwd: root }), ["broken invariant"]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
