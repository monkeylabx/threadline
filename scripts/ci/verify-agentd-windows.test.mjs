import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { verifyExecution, readLog } from "./verify-agentd-windows.mjs";

const required = JSON.parse(readFileSync(new URL("./agentd-windows-cases.json", import.meta.url), "utf8"));
const summary = (count) => `test result: ok. ${count} passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.01s`;
const output = (names = required) => [...names.map((name) => `test ${name} ... ok`), summary(names.length)].join("\n");

test("native baseline includes every existing case and the compile/link contract", () => {
  assert.equal(required.length, 50);
  assert.ok(required.includes("pinned_windows_job_api_surface_is_available"));
  assert.ok(required.includes("real_synthetic_process_exchanges_frames_and_reports_abnormal_eof"));
  assert.deepEqual(verifyExecution(output(), required), { passed: 50, required: 50, cases: [...required].sort() });
});

test("additional executed cases, empty unit suites and CRLF remain valid", () => {
  const log = `${summary(0)}\n${output([...required, "future::case"])}`.replaceAll("\n", "\r\n");
  assert.equal(verifyExecution(log, required).passed, 51);
});

test("missing, duplicate, failed, ignored and malformed cases cannot pass", () => {
  for (const log of [output(required.slice(1)), output([...required, required[0]]),
    output().replace(" ... ok", " ... FAILED"), output().replace(" ... ok", " ... ignored"),
    output().replace(" ... ok", " ... unknown"), output().replace(" ... ok", " ... ok unexpected")]) {
    assert.throws(() => verifyExecution(log, required));
  }
});

test("zero, absent, malformed, filtered, ignored and inconsistent summaries fail", () => {
  for (const log of [summary(0), "", "running 0 tests", output().replace(summary(50), ""),
    output().replace(summary(50), "test result: FAILED"), output().replace("0 ignored", "1 ignored"),
    output().replace("0 filtered out", "1 filtered out"), output().replace("0 failed", "1 failed"),
    output().replace("0 measured", "1 measured"), output().replace("50 passed", "49 passed")]) {
    assert.throws(() => verifyExecution(log, required));
  }
});

test("invalid baselines and output byte overflow fail closed", () => {
  for (const baseline of [null, [], [required[0], required[0]], ["missing"]]) {
    assert.throws(() => verifyExecution(output(), baseline));
  }
  const prefix = "x".repeat(1024 * 1024 - Buffer.byteLength(output()) - 1);
  assert.equal(Buffer.byteLength(`${prefix}\n${output()}`), 1024 * 1024);
  assert.equal(verifyExecution(`${prefix}\n${output()}`, required).passed, 50);
  assert.throws(() => verifyExecution(`${"é".repeat(524289)}\n${output()}`, required), /Oversized/);
});

test("file intake is bounded, complete and closes resources on read failures", (t) => {
  const directory = mkdtempSync(join(tmpdir(), "agentd-receipt-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const path = join(directory, "output");
  writeFileSync(path, output());
  assert.equal(readLog(path), output());
  writeFileSync(path, "x".repeat(1024 * 1024));
  assert.equal(readLog(path).length, 1024 * 1024);
  writeFileSync(path, "é".repeat(524289));
  assert.throws(() => readLog(path), /Oversized/);
  assert.throws(() => readLog(directory));
  assert.throws(() => readLog(join(directory, "missing")));
});

test("CLI refuses missing input or non-Windows evidence without a receipt", () => {
  const args = process.platform === "win32" ? [] : ["absent"];
  const result = spawnSync(process.execPath, [fileURLToPath(new URL("./verify-agentd-windows.mjs", import.meta.url)), ...args], { encoding: "utf8" });
  assert.notEqual(result.status, 0);
  assert.equal(result.stdout, "");
  assert.match(result.stderr, process.platform === "win32" ? /Expected one test log/ : /Native Windows execution required/);
});
