import { spawnSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import { doctor as toolchainDoctor, verifyPins } from "./toolchain.mjs";

const root = fileURLToPath(new URL("../", import.meta.url));
const action = process.argv[2] ?? "doctor";
const swiftScratch = join(tmpdir(), "threadline-swift-skeleton");
const androidRoot = join(root, "apps", "android");
const gradleCommand = process.platform === "win32" ? "gradlew.bat" : "./gradlew";

const desktopSource = "apps/desktop/src/main.ts";
const desktopTest = "apps/desktop/test/smoke.test.ts";
const goSources = [
  "agentd/main.go",
  "core/main.go",
  "model-control/main.go",
  "realtime/main.go",
  "recovery-control/main.go",
  "runtime-gateway/main.go",
  "worker/main.go",
];

function run(label, command, args, options = {}) {
  console.log(`\n[${label}] ${command} ${args.join(" ")}`);
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? root,
    env: { ...process.env, ...options.env },
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
    env: { ...process.env, ...options.env },
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
    ".html",
    ".json",
    ".kt",
    ".kts",
    ".md",
    ".mjs",
    ".properties",
    ".proto",
    ".rs",
    ".swift",
    ".toml",
    ".ts",
    ".xml",
    ".yaml",
    ".yml",
  ]);
  const ignored = new Set([".git", ".gradle", ".build", "build", "gen", "target"]);
  const files = [
    "Cargo.toml",
    "Cargo.lock",
    "CONTRIBUTING.md",
    "Makefile",
    ".java-version",
    ".node-version",
    ".xcode-version",
    "apps/android/gradlew",
    "apps/android/gradlew.bat",
    "package.json",
    "pnpm-lock.yaml",
    "pnpm-workspace.yaml",
    "rust-toolchain.toml",
    "toolchains.json",
  ];

  function walk(relative) {
    for (const entry of readdirSync(join(root, relative), { withFileTypes: true })) {
      if (ignored.has(entry.name)) continue;
      const path = join(relative, entry.name);
      if (entry.isDirectory()) walk(path);
      else if (
        extensions.has(entry.name.slice(entry.name.lastIndexOf("."))) ||
        entry.name === "Makefile" ||
        entry.name === "gradle.properties"
      ) {
        files.push(path);
      }
    }
  }

  for (const directory of [
    ".github",
    "apps",
    "crates",
    "deploy",
    "docs/build",
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
  return [
    run("typescript", "node", ["--experimental-strip-types", "--check", desktopSource]),
    run("rust", "cargo", ["build", "--workspace", "--locked"]),
    run("go", "go", ["build", "./..."], {
      cwd: join(root, "services"),
      env: { GOTOOLCHAIN: "local" },
    }),
    run("swift", "swift", [
      "build",
      "--package-path",
      "apps/ios",
      "--scratch-path",
      swiftScratch,
    ]),
    run("android", gradleCommand, ["assembleDebug", "--no-daemon"], { cwd: androidRoot }),
  ].every(Boolean);
}

function test() {
  return [
    run("typescript", "node", ["--experimental-strip-types", "--test", desktopTest]),
    run("rust", "cargo", ["test", "--workspace", "--locked"]),
    run("go", "go", ["test", "./..."], {
      cwd: join(root, "services"),
      env: { GOTOOLCHAIN: "local" },
    }),
    run("swift", "swift", [
      "test",
      "--package-path",
      "apps/ios",
      "--scratch-path",
      swiftScratch,
    ]),
    run("android", gradleCommand, ["testDebugUnitTest", "--no-daemon"], { cwd: androidRoot }),
  ].every(Boolean);
}

function lint() {
  return [
    textLint(),
    verifyPins(),
    run("node", "node", ["--check", "scripts/workspace.mjs"]),
    run("typescript", "node", ["--experimental-strip-types", "--check", desktopSource]),
    run("rust", "cargo", ["fmt", "--all", "--check"]),
    runWithEmptyOutput("go", "gofmt", ["-d", ...goSources], {
      cwd: join(root, "services"),
      env: { GOTOOLCHAIN: "local" },
    }),
    run("swift", "swift", ["package", "--package-path", "apps/ios", "dump-package"]),
    run("android", gradleCommand, ["lintDebug", "--no-daemon"], { cwd: androidRoot }),
  ].every(Boolean);
}

function verifyStructure() {
  const required = [
    "Cargo.toml",
    "Cargo.lock",
    "toolchains.json",
    "rust-toolchain.toml",
    "pnpm-lock.yaml",
    "apps/android/gradlew",
    "apps/android/gradlew.bat",
    "apps/android/gradle/wrapper/gradle-wrapper.jar",
    "apps/android/gradle/wrapper/gradle-wrapper.properties",
    "apps/android/settings.gradle.kts",
    "apps/android/gradle.properties",
    "apps/android/gradle.lockfile",
    "package.json",
    "pnpm-workspace.yaml",
    "apps/desktop/package.json",
    "apps/desktop/src-tauri/Cargo.toml",
    "apps/desktop/src-tauri/tauri.conf.json",
    "apps/desktop/src-tauri/icons/icon.png",
    "apps/desktop/src-tauri/icons/icon.ico",
    "apps/ios/Package.swift",
    "apps/android/build.gradle.kts",
    "apps/android/src/main/AndroidManifest.xml",
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
    "proto/buf.yaml",
    "proto/threadline/message/v1/envelope.proto",
    "proto/threadline/task/v1/task.proto",
    "deploy/README.md",
    "test/README.md",
    ".github/workflows/build.yml",
    "docs/build/reproducible-builds.md",
    "docs/build/toolchain-research.md",
  ];
  const missing = required.filter((path) => !existsSync(join(root, path)));
  if (missing.length > 0) {
    for (const path of missing) console.error(`[structure] missing: ${path}`);
    return false;
  }
  if (!verifyPins()) return false;
  if (!textLint()) return false;
  console.log(`workspace structure ok (${required.length} required surfaces)`);
  return true;
}

switch (action) {
  case "doctor":
    if (!toolchainDoctor()) process.exitCode = 1;
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
