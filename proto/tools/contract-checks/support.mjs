import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

// Shared, side-effect-free helpers for the contract check modules. Paths are
// anchored on this file so every module agrees on the same repository layout.
export const protoRoot = fileURLToPath(new URL("../../", import.meta.url));
export const repositoryRoot = fileURLToPath(new URL("../../../", import.meta.url));

export function read(path) {
  return readFileSync(path, "utf8").replaceAll("\r\n", "\n");
}

export function filesBelow(directory, suffix) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return filesBelow(path, suffix);
    return path.endsWith(suffix) ? [path] : [];
  });
}

// Collects failures instead of throwing so one run reports every broken
// invariant; the order of `failures` is the order the checks observed them.
export function createCollector() {
  const failures = [];
  return {
    failures,
    assert(condition, message) {
      if (!condition) failures.push(message);
    },
  };
}
