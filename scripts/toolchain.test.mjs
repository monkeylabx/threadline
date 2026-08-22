import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  pins,
  resolveProbeInvocation,
  validateDatabasePins,
  validateWorkflowPins,
} from "./toolchain.mjs";

const root = fileURLToPath(new URL("../", import.meta.url));
const workflow = readFileSync(join(root, ".github", "workflows", "build.yml"), "utf8");
const databaseSources = {
  goModule: readFileSync(join(root, "services", "go.mod"), "utf8"),
  serviceCatalog: readFileSync(
    join(root, "docs", "architecture", "service-catalog.md"),
    "utf8",
  ),
  reproducibleBuilds: readFileSync(join(root, "docs", "build", "reproducible-builds.md"), "utf8"),
};

test("the checked-in workflow has no toolchain pin drift", () => {
  assert.deepEqual(validateWorkflowPins(workflow), []);
});

test("database generator, driver, and schema pins are internally consistent", () => {
  assert.deepEqual(validateDatabasePins(pins.database, databaseSources), []);
});

test("database pin verification rejects a drifted pgx dependency", () => {
  const drifted = {
    ...databaseSources,
    goModule: databaseSources.goModule.replace(`v${pins.database.pgx}`, "v0.0.0"),
  };
  assert.match(
    validateDatabasePins(pins.database, drifted).join("\n"),
    /services pgx dependency/,
  );
});

test("database pin verification rejects pgx text that appears only in a comment", () => {
  const commented = {
    ...databaseSources,
    goModule: databaseSources.goModule.replace(
      `require github.com/jackc/pgx/v5 v${pins.database.pgx}`,
      `// github.com/jackc/pgx/v5 v${pins.database.pgx}`,
    ),
  };
  assert.match(
    validateDatabasePins(pins.database, commented).join("\n"),
    /missing direct require/,
  );
});

test("database pin verification rejects a well-formed but drifted sqlc digest", () => {
  const database = structuredClone(pins.database);
  database.sqlc.archives["linux-amd64"].sha256 = "0".repeat(64);
  assert.match(validateDatabasePins(database, databaseSources).join("\n"), /linux-amd64 SHA-256/);
});

test("database pin verification rejects an incomplete sqlc archive set", () => {
  const database = structuredClone(pins.database);
  delete database.sqlc.archives["linux-arm64"];
  assert.match(
    validateDatabasePins(database, databaseSources).join("\n"),
    /sqlc archive platform set/,
  );
});

const gradleLock = readFileSync(join(root, "apps", "android", "gradle.lockfile"), "utf8");

test("Android strict lock contains the SDK API configuration", () => {
  const emptyLock = gradleLock.split("\n").find((line) => line.startsWith("empty="));
  assert.ok(emptyLock, "missing empty Gradle lock state");
  assert.ok(
    emptyLock.slice("empty=".length).split(",").includes("androidApis"),
    "missing androidApis Gradle lock state",
  );
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
