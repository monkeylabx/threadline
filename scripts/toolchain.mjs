import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
export const pins = JSON.parse(readFileSync(join(root, "toolchains.json"), "utf8"));

function read(relative) {
  return readFileSync(join(root, relative), "utf8").replaceAll("\r\n", "\n");
}

function readJson(relative) {
  return JSON.parse(read(relative));
}

function assertEqual(errors, label, actual, expected) {
  if (`${actual}` !== `${expected}`) {
    errors.push(`${label}: expected ${expected}, got ${actual}`);
  }
}

function assertIncludes(errors, label, text, expected) {
  if (!text.includes(expected)) errors.push(`${label}: missing ${expected}`);
}

export function verifyPins() {
  const errors = [];
  const rootPackage = readJson("package.json");
  const desktopPackage = readJson("apps/desktop/package.json");
  const rust = read("rust-toolchain.toml");
  const goWork = read("go.work");
  const goModule = read("services/go.mod");
  const tauriCargo = read("apps/desktop/src-tauri/Cargo.toml");
  const cargoLock = read("Cargo.lock");
  const pnpmLock = read("pnpm-lock.yaml");
  const gradleRoot = read("build.gradle.kts");
  const android = read("apps/android/build.gradle.kts");
  const wrapper = read("gradle/wrapper/gradle-wrapper.properties");
  const workflow = read(".github/workflows/build.yml");

  assertEqual(errors, ".node-version", read(".node-version").trim(), pins.node);
  assertEqual(errors, ".nvmrc", read(".nvmrc").trim(), pins.node);
  assertEqual(errors, "engines.node", rootPackage.engines.node, pins.node);
  assertEqual(errors, "engines.pnpm", rootPackage.engines.pnpm, pins.pnpm);
  assertEqual(errors, "packageManager", rootPackage.packageManager, `pnpm@${pins.pnpm}`);
  assertIncludes(errors, "rust channel", rust, `channel = "${pins.rust}"`);
  for (const goFile of [["go.work", goWork], ["services/go.mod", goModule]]) {
    assertIncludes(errors, `${goFile[0]} language`, goFile[1], `go ${pins.goLanguage}`);
    assertIncludes(errors, `${goFile[0]} toolchain`, goFile[1], `toolchain go${pins.goToolchain}`);
  }
  assertEqual(errors, "Tauri CLI", desktopPackage.devDependencies["@tauri-apps/cli"], pins.tauri.cli);
  assertEqual(errors, "Tauri API", desktopPackage.dependencies["@tauri-apps/api"], pins.tauri.api);
  assertIncludes(errors, "Tauri locked build", desktopPackage.scripts["build:native"], "--locked");
  assertIncludes(errors, "Tauri core", tauriCargo, `version = "=${pins.tauri.core}"`);
  assertIncludes(errors, "Tauri build", tauriCargo, `version = "=${pins.tauri.build}"`);
  assertIncludes(errors, "Cargo.lock Tauri core", cargoLock, `name = "tauri"\nversion = "${pins.tauri.core}"`);
  assertIncludes(errors, "Cargo.lock Tauri build", cargoLock, `name = "tauri-build"\nversion = "${pins.tauri.build}"`);
  assertIncludes(errors, "pnpm-lock Tauri CLI", pnpmLock, `'@tauri-apps/cli@${pins.tauri.cli}'`);
  assertIncludes(errors, "pnpm-lock Tauri API", pnpmLock, `'@tauri-apps/api@${pins.tauri.api}'`);
  assertEqual(
    errors,
    "Tauri core/API minor",
    pins.tauri.core.split(".").slice(0, 2).join("."),
    pins.tauri.api.split(".").slice(0, 2).join("."),
  );
  assertEqual(errors, ".xcode-version", read(".xcode-version").trim(), pins.apple.xcode);
  assertIncludes(
    errors,
    "Xcode developer directory",
    workflow,
    `/Applications/Xcode_${pins.apple.xcode}.app/Contents/Developer`,
  );
  assertEqual(errors, ".java-version", read(".java-version").trim(), pins.android.java.split("+")[0]);
  assertIncludes(errors, "AGP", gradleRoot, `version "${pins.android.agp}"`);
  assertIncludes(errors, "compileSdk", android, `compileSdk = ${pins.android.compileSdk}`);
  assertIncludes(errors, "buildTools", android, `buildToolsVersion = "${pins.android.buildTools}"`);
  assertIncludes(errors, "NDK", android, `ndkVersion = "${pins.android.ndk}"`);
  assertIncludes(
    errors,
    "Android SDK platform install",
    workflow,
    `'platforms;android-${pins.android.compileSdk}'`,
  );
  assertIncludes(
    errors,
    "Android Build Tools install",
    workflow,
    `'build-tools;${pins.android.buildTools}'`,
  );
  assertIncludes(errors, "Android NDK install", workflow, `'ndk;${pins.android.ndk}'`);
  assertIncludes(errors, "CI Corepack", workflow, `corepack@${pins.corepack}`);
  assertIncludes(errors, "CI pnpm", workflow, `pnpm@${pins.pnpm}`);
  assertIncludes(errors, "CI Go", workflow, `go-version: ${pins.goToolchain}`);
  assertIncludes(errors, "CI Java", workflow, `java-version: ${pins.android.java}`);
  assertIncludes(errors, "CI Go auto-download guard", workflow, "GOTOOLCHAIN: local");
  assertIncludes(errors, "Gradle distribution", wrapper, `gradle-${pins.android.gradle}-bin.zip`);
  assertIncludes(
    errors,
    "Gradle distribution checksum",
    wrapper,
    `distributionSha256Sum=${pins.checksums.gradleDistributionSha256}`,
  );

  const wrapperJar = readFileSync(join(root, "gradle/wrapper/gradle-wrapper.jar"));
  assertEqual(
    errors,
    "Gradle wrapper JAR checksum",
    createHash("sha256").update(wrapperJar).digest("hex"),
    pins.checksums.gradleWrapperJarSha256,
  );
  for (const runner of Object.values(pins.runners)) {
    if (!workflow.includes(`runs-on: ${runner}`) && !workflow.includes(`os: ${runner}`)) {
      errors.push(`CI runner: missing ${runner}`);
    }
  }
  for (const [action, sha] of Object.entries(pins.actions)) {
    assertIncludes(errors, `CI action ${action}`, workflow, `uses: ${action}@${sha}`);
  }
  for (const match of workflow.matchAll(/^\s*uses:\s*([^\s#]+).*$/gm)) {
    const reference = match[1];
    if (!/@[0-9a-f]{40}$/.test(reference)) {
      errors.push(`CI action is not pinned to a full SHA: ${reference}`);
    }
  }

  if (errors.length > 0) {
    for (const error of errors) console.error(`[pin] ${error}`);
    return false;
  }
  console.log("toolchain pins are internally consistent");
  return true;
}

function probe(label, command, args, expected, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? root,
    encoding: "utf8",
    env: { ...process.env, ...options.env },
    shell: process.platform === "win32",
    stdio: ["ignore", "pipe", "pipe"],
  });
  const output = `${result.stdout ?? ""}${result.stderr ?? ""}`.trim();
  const available = result.status === 0;
  const matches = available && expected.every((value) => output.includes(value));
  const status = matches ? "ok" : available ? "mismatch" : "missing";
  console.log(`${status.padEnd(9)} ${label.padEnd(10)} ${output.split("\n")[0] ?? ""}`.trimEnd());
  if (available && !matches) console.error(`  expected: ${expected.join(", ")}`);
  return matches;
}

export function doctor(scopes = ["workspace", "desktop", "android", "apple"]) {
  const selected = new Set(scopes);
  let valid = verifyPins();
  valid = probe("Node", "node", ["--version"], [`v${pins.node}`]) && valid;
  valid = probe("Corepack", "corepack", ["--version"], [pins.corepack]) && valid;
  valid = probe("pnpm", "corepack", ["pnpm", "--version"], [pins.pnpm]) && valid;

  if (selected.has("workspace") || selected.has("desktop")) {
    valid = probe("Rust", "rustc", ["--version"], [`rustc ${pins.rust}`]) && valid;
  }
  if (selected.has("workspace")) {
    valid = probe("Go", "go", ["version"], [`go${pins.goToolchain}`], {
      env: { GOTOOLCHAIN: "local" },
    }) && valid;
  }
  if (selected.has("desktop")) {
    valid =
      probe(
        "Tauri",
        "corepack",
        ["pnpm", "--filter", "@threadline/desktop", "exec", "tauri", "--version"],
        [pins.tauri.cli],
      ) &&
      valid;
  }
  if (selected.has("android")) {
    valid = probe("Java", "java", ["-version"], [pins.android.java, pins.android.javaVendor]) && valid;
    const gradle = process.platform === "win32" ? "gradlew.bat" : "./gradlew";
    const sdkmanager = process.env.ANDROID_SDK_ROOT
      ? join(
          process.env.ANDROID_SDK_ROOT,
          "cmdline-tools",
          "latest",
          "bin",
          process.platform === "win32" ? "sdkmanager.bat" : "sdkmanager",
        )
      : process.platform === "win32"
        ? "sdkmanager.bat"
        : "sdkmanager";
    valid = probe("Gradle", gradle, ["--version", "--no-daemon"], [`Gradle ${pins.android.gradle}`]) && valid;
    valid =
      probe("Android SDK", sdkmanager, ["--list_installed"], [
        `platforms;android-${pins.android.compileSdk}`,
        `build-tools;${pins.android.buildTools}`,
        `ndk;${pins.android.ndk}`,
      ]) && valid;
  }
  if (selected.has("apple")) {
    valid = probe("Xcode", "xcodebuild", ["-version"], [
      `Xcode ${pins.apple.xcode}`,
      `Build version ${pins.apple.xcodeBuild}`,
    ]) && valid;
    valid = probe("Swift", "swift", ["--version"], [`Swift version ${pins.apple.swift}`]) && valid;
    valid =
      probe("Apple SDK", "xcrun", ["--sdk", "iphoneos", "--show-sdk-version"], [pins.apple.sdk]) &&
      valid;
  }
  return valid;
}

function parseScopes(argv) {
  const value = argv.find((argument) => argument.startsWith("--scope="));
  return value ? value.slice("--scope=".length).split(",") : undefined;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const action = process.argv[2] ?? "doctor";
  const valid = action === "verify" ? verifyPins() : doctor(parseScopes(process.argv));
  if (!valid) process.exitCode = 1;
}
