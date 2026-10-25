import { spawnSync } from "node:child_process";

import { repositoryRoot } from "./support.mjs";

// Executes registered verifier scripts as child Node processes and reports
// their combined output as failures; `spawn` is injectable for hermetic tests.

function runScript(path, spawn, cwd) {
  const result = spawn(process.execPath, [path], { cwd, encoding: "utf8", stdio: "pipe" });
  return { status: result.status, output: `${result.stdout ?? ""}${result.stderr ?? ""}` };
}

// Returns one trimmed combined-output entry per failing verifier, in registry
// order; a passing verifier contributes nothing.
export function runRegisteredVerifiers(verifiers, { spawn = spawnSync, cwd = repositoryRoot } = {}) {
  const failures = [];
  for (const { path } of verifiers) {
    const result = runScript(path, spawn, cwd);
    if (result.status !== 0) failures.push(result.output.trim());
  }
  return failures;
}

// Runs a verifier whose exit status must propagate unchanged; `null` status
// (killed by signal) is reported as the generic failure code 1.
export function runPropagatingVerifier(path, { spawn = spawnSync, cwd = repositoryRoot } = {}) {
  const result = runScript(path, spawn, cwd);
  return { ok: result.status === 0, status: result.status ?? 1, output: result.output };
}
