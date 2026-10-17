import test from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { readLog } from "./verify-agentd-windows.mjs";

test("file intake is bounded, complete and closes resources on read failures", (t) => {
  const directory = mkdtempSync(join(tmpdir(), "agentd-receipt-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const path = join(directory, "output");
  writeFileSync(path, "synthetic\noutput");
  assert.equal(readLog(path), "synthetic\noutput");
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
