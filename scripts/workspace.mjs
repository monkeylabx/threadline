import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const action = process.argv[2] ?? "doctor";
const strictDoctor = process.argv.includes("--strict");
const swiftScratch = join(tmpdir(), "threadline-swift-skeleton");
const kotlinBuild = join(root, "build", "android-kotlin");

const tools = [
  ["Node", "node", ["--version"]],
  ["Corepack", "corepack", ["--version"]],
  ["pnpm", "pnpm", ["--version"]],
  ["Rust", "cargo", ["--version"]],
  ["Go", "go", ["version"]],
  ["Swift", "swift", ["--version"]],
  ["Java", "java", ["-version"]],
  ["Gradle", "gradle", ["--version"]],
  ["Kotlin", "kotlinc", ["-version"]],
];

const desktopSource = "apps/desktop/src/main.ts";
const desktopTest = "apps/desktop/test/smoke.test.ts";
const kotlinSource =
  "apps/android/src/main/kotlin/com/threadline/android/ThreadlineAndroidSkeleton.kt";
const kotlinTest =
  "apps/android/src/test/kotlin/com/threadline/android/ThreadlineAndroidSkeletonTest.kt";
const goSources = [
  "agentd/main.go",
  "core/main.go",
  "model-control/main.go",
  "realtime/main.go",
  "recovery-control/main.go",
  "runtime-gateway/main.go",
  "worker/main.go",
];

function probe(command, args) {
  const result = spawnSync(command, args, {
    cwd: root,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
  const output = `${result.stdout ?? ""}${result.stderr ?? ""}`
    .trim()
    .split("\n")[0];
  return { available: result.status === 0, output };
}

function doctor() {
  let missing = false;
  for (const [label, command, args] of tools) {
    const result = probe(command, args);
    missing ||= !result.available;
    const status = result.available ? "ok" : "missing";
    console.log(`${status.padEnd(8)} ${label.padEnd(10)} ${result.output}`.trimEnd());
  }
  if (missing) {
    console.log("\nT009 owns exact versions, wrappers, and five-platform CI installation.");
  }
  if (strictDoctor && missing) process.exitCode = 1;
}

function run(label, command, args, options = {}) {
  console.log(`\n[${label}] ${command} ${args.join(" ")}`);
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? root,
    stdio: "inherit",
  });
  if (result.status !== 0) {
    const reason = result.error?.code === "ENOENT" ? "tool missing" : `exit ${result.status}`;
    console.error(`[${label}] failed: ${reason}`);
    return false;
  }
  return true;
}

function runWithEmptyOutput(label, command, args, options = {}) {
  console.log(`\n[${label}] ${command} ${args.join(" ")}`);
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? root,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "inherit"],
  });
  if (result.status !== 0) {
    const reason = result.error?.code === "ENOENT" ? "tool missing" : `exit ${result.status}`;
    console.error(`[${label}] failed: ${reason}`);
    return false;
  }
  if (result.stdout.length > 0) {
    process.stdout.write(result.stdout);
    console.error(`[${label}] failed: formatter reported changes`);
    return false;
  }
  return true;
}

function textLint() {
  const extensions = new Set([
    ".go",
    ".json",
    ".kt",
    ".kts",
    ".md",
    ".mjs",
    ".rs",
    ".swift",
    ".toml",
    ".ts",
    ".yaml",
    ".yml",
  ]);
  const ignored = new Set([".git", ".gradle", ".build", "build", "target"]);
  const files = [
    "Cargo.toml",
    "CONTRIBUTING.md",
    "Makefile",
    "build.gradle.kts",
    "go.work",
    "gradle.properties",
    "package.json",
    "pnpm-workspace.yaml",
    "settings.gradle.kts",
  ];

  function walk(relative) {
    for (const entry of readdirSync(join(root, relative), { withFileTypes: true })) {
      if (ignored.has(entry.name)) continue;
      const path = join(relative, entry.name);
      if (entry.isDirectory()) walk(path);
      else if (
        extensions.has(entry.name.slice(entry.name.lastIndexOf("."))) ||
        entry.name === "Makefile" ||
        entry.name === "go.work" ||
        entry.name === "gradle.properties"
      ) {
        files.push(path);
      }
    }
  }

  for (const directory of [
    "apps",
    "crates",
    "deploy",
    "packages",
    "proto",
    "scripts",
    "services",
    "test",
  ]) {
    walk(directory);
  }
  let valid = true;
  for (const file of files) {
    const text = readFileSync(join(root, file), "utf8");
    if (!text.endsWith("\n") || /[ \t]+$/m.test(text)) {
      console.error(`[text] invalid whitespace: ${file}`);
      valid = false;
    }
  }
  return valid;
}

function build() {
  mkdirSync(kotlinBuild, { recursive: true });
  return [
    run("typescript", "node", ["--experimental-strip-types", "--check", desktopSource]),
    run("rust", "cargo", ["build", "--workspace"]),
    run("go", "go", ["build", "./..."], { cwd: join(root, "services") }),
    run("swift", "swift", [
      "build",
      "--package-path",
      "apps/ios",
      "--scratch-path",
      swiftScratch,
    ]),
    run("kotlin", "kotlinc", [kotlinSource, "-d", join(kotlinBuild, "skeleton.jar")]),
  ].every(Boolean);
}

function test() {
  mkdirSync(kotlinBuild, { recursive: true });
  return [
    run("typescript", "node", ["--experimental-strip-types", "--test", desktopTest]),
    run("rust", "cargo", ["test", "--workspace"]),
    run("go", "go", ["test", "./..."], { cwd: join(root, "services") }),
    run("swift", "swift", [
      "test",
      "--package-path",
      "apps/ios",
      "--scratch-path",
      swiftScratch,
    ]),
    run("kotlin-compile-test", "kotlinc", [
      kotlinSource,
      kotlinTest,
      "-include-runtime",
      "-d",
      join(kotlinBuild, "skeleton-test.jar"),
    ]),
    run("kotlin-test", "java", ["-jar", join(kotlinBuild, "skeleton-test.jar")]),
  ].every(Boolean);
}

function lint() {
  mkdirSync(kotlinBuild, { recursive: true });
  return [
    textLint(),
    run("node", "node", ["--check", "scripts/workspace.mjs"]),
    run("typescript", "node", ["--experimental-strip-types", "--check", desktopSource]),
    run("rust", "cargo", ["fmt", "--all", "--check"]),
    runWithEmptyOutput("go", "gofmt", ["-d", ...goSources], {
      cwd: join(root, "services"),
    }),
    run("swift", "swift", ["package", "--package-path", "apps/ios", "dump-package"]),
    run("kotlin", "kotlinc", [kotlinSource, "-d", join(kotlinBuild, "lint.jar")]),
  ].every(Boolean);
}

function verifyStructure() {
  const required = [
    "Cargo.toml",
    "go.work",
    "package.json",
    "pnpm-workspace.yaml",
    "settings.gradle.kts",
    "apps/desktop/package.json",
    "apps/ios/Package.swift",
    "apps/android/build.gradle.kts",
    "crates/client-core/Cargo.toml",
    "crates/client-crypto/Cargo.toml",
    "crates/client-ffi/Cargo.toml",
    "crates/locald/Cargo.toml",
    "crates/connectord/Cargo.toml",
    "services/go.mod",
    "services/core/main.go",
    "services/realtime/main.go",
    "services/runtime-gateway/main.go",
    "services/worker/main.go",
    "services/model-control/main.go",
    "services/recovery-control/main.go",
    "services/agentd/main.go",
    "proto/README.md",
    "deploy/README.md",
    "test/README.md",
  ];
  const missing = required.filter((path) => !existsSync(join(root, path)));
  if (missing.length > 0) {
    for (const path of missing) console.error(`[structure] missing: ${path}`);
    return false;
  }
  if (!textLint()) return false;
  console.log(`workspace structure ok (${required.length} required surfaces)`);
  return true;
}

switch (action) {
  case "doctor":
    doctor();
    break;
  case "build":
    if (!build()) process.exitCode = 1;
    break;
  case "test":
    if (!test()) process.exitCode = 1;
    break;
  case "lint":
    if (!lint()) process.exitCode = 1;
    break;
  case "verify":
    if (!verifyStructure()) process.exitCode = 1;
    break;
  default:
    console.error(`Unknown workspace action: ${action}`);
    process.exitCode = 2;
}
