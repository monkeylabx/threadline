import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { verifyExecution } from "./verify-agentd-windows.mjs";

const required = JSON.parse(readFileSync(new URL("./agentd-windows-cases.json", import.meta.url), "utf8"));
const summary = (count) => `test result: ok. ${count} passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.01s`;
const output = (names = required) => [...names.map((name) => `test ${name} ... ok`), summary(names.length)].join("\n");

test("native baseline includes existing cases, API surface, Job resources and suspended launch", () => {
  assert.equal(required.length, 71);
  assert.ok(required.includes("pinned_windows_job_api_surface_is_available"));
  assert.ok(required.includes("probe::cases::windows_job_profile_is_bounded_and_non_inheritable"));
  assert.ok(required.includes("probe::launch::cases::windows_launch_is_suspended_and_in_job_before_resume"));
  assert.ok(required.includes("probe::launch::tree::cases::windows_tree_root_exit_leaves_a_live_owned_descendant"));
  assert.ok(required.includes("real_synthetic_process_exchanges_frames_and_reports_abnormal_eof"));
  assert.deepEqual(verifyExecution(output(), required), { passed: 71, required: 71, cases: [...required].sort() });
});

test("additional executed cases, empty unit suites and CRLF remain valid", () => {
  const log = `${summary(0)}\n${output([...required, "future::case"])}`.replaceAll("\n", "\r\n");
  assert.equal(verifyExecution(log, required).passed, 72);
});

test("missing, duplicate, failed, ignored and malformed cases cannot pass", () => {
  for (const log of [output(required.slice(1)), output([...required, required[0]]),
    output().replace(" ... ok", " ... FAILED"), output().replace(" ... ok", " ... ignored"),
    output().replace(" ... ok", " ... unknown"), output().replace(" ... ok", " ... ok unexpected")]) {
    assert.throws(() => verifyExecution(log, required));
  }
});

test("zero, absent, malformed, filtered, ignored and inconsistent summaries fail", () => {
  for (const log of [summary(0), "", "running 0 tests", output().replace(summary(71), ""),
    output().replace(summary(71), "test result: FAILED"), output().replace("0 ignored", "1 ignored"),
    output().replace("0 filtered out", "1 filtered out"), output().replace("0 failed", "1 failed"),
    output().replace("0 measured", "1 measured"), output().replace("71 passed", "70 passed")]) {
    assert.throws(() => verifyExecution(log, required));
  }
});

test("invalid baselines and output byte overflow fail closed", () => {
  for (const baseline of [null, [], [required[0], required[0]], ["missing"]]) {
    assert.throws(() => verifyExecution(output(), baseline));
  }
  const prefix = "x".repeat(1024 * 1024 - Buffer.byteLength(output()) - 1);
  assert.equal(Buffer.byteLength(`${prefix}\n${output()}`), 1024 * 1024);
  assert.equal(verifyExecution(`${prefix}\n${output()}`, required).passed, 71);
  assert.throws(() => verifyExecution(`${"é".repeat(524289)}\n${output()}`, required), /Oversized/);
});
