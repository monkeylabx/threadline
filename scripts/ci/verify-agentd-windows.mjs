// Execution receipt for a pinned libtest format, not worker containment evidence.
import { readFileSync, openSync, readSync, closeSync } from "node:fs";
import { release } from "node:os";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const limit = 1024 * 1024;

export function readLog(path) {
  const fd = openSync(path, "r");
  try {
    const bytes = Buffer.alloc(limit + 1);
    let count = 0;
    while (count < bytes.length) {
      const size = readSync(fd, bytes, count, bytes.length - count, null);
      if (size === 0) break;
      count += size;
    }
    if (count > limit) throw new Error("Oversized test output");
    return bytes.subarray(0, count).toString("utf8");
  } finally {
    closeSync(fd);
  }
}

function passedCases(lines) {
  const cases = new Set();
  for (const line of lines.filter((line) => line.startsWith("test ") && !line.startsWith("test result:"))) {
    const match = /^test ([A-Za-z0-9_:]+) \.\.\. (ok|FAILED|ignored)$/.exec(line);
    if (!match || match[2] !== "ok" || cases.has(match[1])) throw new Error("Invalid or repeated test outcome");
    cases.add(match[1]);
  }
  return cases;
}

function passedTotal(lines) {
  const summaries = lines.filter((line) => line.startsWith("test result:"));
  if (summaries.length === 0) throw new Error("Missing test summaries");
  return summaries.reduce((total, line) => {
    const match = /^test result: ok\. (\d+) passed; (\d+) failed; (\d+) ignored; (\d+) measured; (\d+) filtered out; finished in \S+$/.exec(line);
    if (!match || match.slice(2).some((count) => Number(count) !== 0)) throw new Error("Incomplete test execution");
    return total + Number(match[1]);
  }, 0);
}

export function verifyExecution(log, required) {
  if (Buffer.byteLength(log) > limit) throw new Error("Oversized test output");
  if (!Array.isArray(required) || required.length === 0 || new Set(required).size !== required.length) throw new Error("Invalid test baseline");
  const lines = log.replaceAll("\r\n", "\n").split("\n");
  const cases = passedCases(lines);
  if (cases.size === 0 || passedTotal(lines) !== cases.size || required.some((name) => !cases.has(name))) {
    throw new Error("Missing or inconsistent test execution");
  }
  return { passed: cases.size, required: required.length, cases: [...cases].sort() };
}

function main() {
  if (process.platform !== "win32") throw new Error("Native Windows execution required");
  if (process.argv.length !== 3) throw new Error("Expected one test log");
  const required = JSON.parse(readFileSync(new URL("./agentd-windows-cases.json", import.meta.url), "utf8"));
  const result = verifyExecution(readLog(process.argv[2]), required);
  console.log(JSON.stringify({ schema: 1, platform: process.platform, architecture: process.arch,
    osRelease: release(), node: process.version, commit: process.env.GITHUB_SHA ?? null,
    windowsApi: "compile-and-link-only", containment: "NOT_RUN", goose: "NOT_RUN", ...result }));
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
