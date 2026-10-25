import assert from "node:assert/strict";
import { cpSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { runAggregate } from "../contract-checks/aggregate.mjs";
import { registeredVerifiers } from "../contract-checks/verifier-registry.mjs";
import { evaluate, rules } from "./model.mjs";
import { loadFixtures, verify } from "./verify.mjs";

test("unknown output cannot leak data and unknown seams cannot fall back", () => {
  const unsafe = rules.map((rule) => rule.id === "availability" ? { ...rule, result: { ...rule.result, rawGrant: "synthetic" } } : rule);
  assert.throws(() => verify(loadFixtures(), unsafe), /unexpected output key/);
  assert.throws(() => evaluate({ when: "synthetic-unknown", facts: {} }), /Unknown reference seam/);
  assert.equal(evaluate({ when: "ReportCleanup.restricted-assignment", facts: {} }).status, "UNAUTHENTICATED");
});

test("both children are registered and an actual failing verifier fails the aggregate", () => {
  const tools = new URL("../", import.meta.url);
  for (const name of ["verify-task-control-authorization.mjs", "task-control-authorization/verify.test.mjs"]) {
    assert.ok(registeredVerifiers.some((row) => row.path === fileURLToPath(new URL(name, tools)) && row.existence));
  }
  const sandbox = mkdtempSync(join(tmpdir(), "threadline-task-contract-"));
  try {
    const modelRoot = join(sandbox, "proto/tools/task-control-authorization");
    mkdirSync(modelRoot, { recursive: true });
    cpSync(new URL("./", import.meta.url), modelRoot, { recursive: true });
    const entry = join(sandbox, "proto/tools/verify-task-control-authorization.mjs");
    cpSync(new URL("verify-task-control-authorization.mjs", tools), entry);
    const dataRoot = join(sandbox, "test/fixtures/proto/task-control-authorization");
    mkdirSync(dataRoot, { recursive: true });
    const invalid = loadFixtures();
    invalid.scenarios.cases[0].then.decision = "deny";
    for (const name of ["decisions", "scenarios"]) writeFileSync(join(dataRoot, `${name}.json`), JSON.stringify(invalid[name]));
    mkdirSync(join(sandbox, "proto/threadline/type/v1"), { recursive: true });
    cpSync(new URL("../threadline/type/v1/error.proto", tools), join(sandbox, "proto/threadline/type/v1/error.proto"));
    const result = runAggregate({ checks: [], verifiers: [{ path: entry }], installTestPath: "must-not-run", successLines: [] }, { cwd: sandbox });
    assert.equal(result.exitCode, 1);
    assert.match(result.stderr.join("\n"), /allow-task-create/);
    assert.deepEqual(result.stdout, []);
  } finally {
    rmSync(sandbox, { recursive: true, force: true });
  }
});
