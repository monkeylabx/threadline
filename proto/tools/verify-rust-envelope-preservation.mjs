import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const protoRoot = join(repositoryRoot, "proto");
const goldenRoot = join(protoRoot, "golden", "v1");
const harnessManifest = join(protoRoot, "tools", "rust-envelope-compat", "Cargo.toml");
const goldenManifest = JSON.parse(readFileSync(join(goldenRoot, "manifest.json"), "utf8"));
const buf = process.env.THREADLINE_BUF;
const cargo = process.env.THREADLINE_CARGO;
const mode = process.argv[2];
const optionArguments = process.argv.slice(3);
const baselineArgument = optionArguments.find((value) => value.startsWith("--baseline="));
const frameRootArgument = optionArguments.find((value) => value.startsWith("--frame-root="));
const legacyRecoveryFrameArgument = optionArguments.find((value) => value.startsWith("--legacy-recovery-frame="));
const requireCurrentBindings = optionArguments.includes("--require-current-recovery-bindings");
const allowedArguments = new Set([baselineArgument, frameRootArgument, legacyRecoveryFrameArgument, ...(requireCurrentBindings ? ["--require-current-recovery-bindings"] : [])].filter(Boolean));
if (optionArguments.some((value) => !allowedArguments.has(value))) throw new Error(`unknown argument: ${optionArguments.find((value) => !allowedArguments.has(value))}`);
const baseline = baselineArgument?.slice("--baseline=".length) ?? goldenManifest.compatibilityEvidence.nMinusOneCommit;
const frameRoot = resolve(frameRootArgument?.slice("--frame-root=".length) ?? goldenRoot);
const legacyRecoveryFrame = legacyRecoveryFrameArgument
  ? resolve(legacyRecoveryFrameArgument.slice("--legacy-recovery-frame=".length))
  : undefined;
const inheritedNames = [
  "ALL_PROXY",
  "CARGO_HOME",
  "CARGO_HTTP_CAINFO",
  "HOME",
  "HTTPS_PROXY",
  "HTTP_PROXY",
  "NO_PROXY",
  "RUSTUP_HOME",
  "RUSTUP_TOOLCHAIN",
  "SSL_CERT_FILE",
];

if (!buf || !isAbsolute(buf)) throw new Error("THREADLINE_BUF must name the absolute pinned Buf executable");
if (!cargo || !isAbsolute(cargo)) throw new Error("THREADLINE_CARGO must name the absolute pinned Cargo executable");
if (mode !== "--connected" && mode !== "--offline") {
  throw new Error("usage: verify-rust-envelope-preservation.mjs <--connected|--offline> [--baseline=<full-sha>] [--frame-root=<absolute-directory>] [--require-current-recovery-bindings --legacy-recovery-frame=<absolute-file>]");
}
if (!/^[0-9a-f]{40}$/u.test(baseline)) throw new Error("--baseline must be a full Git object ID");
if (!isAbsolute(frameRoot) || !existsSync(frameRoot)) throw new Error("--frame-root must be an existing absolute directory");
if (requireCurrentBindings && (!legacyRecoveryFrame || !isAbsolute(legacyRecoveryFrame) || !existsSync(legacyRecoveryFrame))) {
  throw new Error("--require-current-recovery-bindings requires an existing absolute --legacy-recovery-frame");
}

function run(executable, commandArguments, options = {}) {
  const result = spawnSync(executable, commandArguments, { encoding: "utf8", ...options });
  if (result.status !== 0) {
    throw new Error(
      `${executable} ${commandArguments.join(" ")} failed:\n${result.stdout ?? ""}${result.stderr ?? ""}`,
    );
  }
  return result;
}

function extractProtoSnapshot(commit, destination) {
  if (!/^[0-9a-f]{40}$/u.test(commit)) throw new Error("N-1 commit must be a full Git object ID");
  const listing = run("git", ["ls-tree", "-r", "--name-only", commit, "--", "proto"], {
    cwd: repositoryRoot,
    encoding: "utf8",
    env: process.env,
  }).stdout;
  const paths = listing.split("\n").filter((path) => path === "proto/buf.yaml" || path.endsWith(".proto"));
  if (!paths.includes("proto/buf.yaml") || paths.filter((path) => path.endsWith(".proto")).length === 0) {
    throw new Error(`N-1 commit ${commit} does not contain the expected proto module`);
  }
  for (const path of paths) {
    const output = join(destination, path);
    mkdirSync(dirname(output), { recursive: true });
    const content = spawnSync("git", ["show", `${commit}:${path}`], {
      cwd: repositoryRoot,
      encoding: null,
      env: process.env,
      stdio: "pipe",
    });
    if (content.error || content.status !== 0) throw new Error(`cannot extract ${commit}:${path}`);
    writeFileSync(output, content.stdout, { mode: 0o600 });
  }
}

const bufPath = `${dirname(buf)}${delimiter}${dirname(process.execPath)}${delimiter}/usr/bin${delimiter}/bin`;
const bufVersion = run(buf, ["--version"], { env: { PATH: bufPath } }).stdout.trim();
if (bufVersion !== "1.72.0") throw new Error(`Buf 1.72.0 is required; received ${bufVersion}`);

const inheritedEnvironment = Object.fromEntries(
  inheritedNames.filter((name) => process.env[name] !== undefined).map((name) => [name, process.env[name]]),
);
const cargoPath = `${dirname(cargo)}${delimiter}/usr/bin${delimiter}/bin`;
const cargoVersion = run(cargo, ["--version"], {
  env: { ...inheritedEnvironment, PATH: cargoPath },
}).stdout.trim();
if (!cargoVersion.startsWith("cargo 1.97.1 ")) throw new Error(`Cargo 1.97.1 is required; received ${cargoVersion}`);

const temporaryRoot = mkdtempSync(join(tmpdir(), "threadline-rust-envelope-"));
const environment = {
  ...inheritedEnvironment,
  CARGO_TARGET_DIR: join(temporaryRoot, "target"),
  CARGO_TERM_COLOR: "never",
  LANG: "C",
  LC_ALL: "C",
  PATH: `${dirname(cargo)}${delimiter}${bufPath}`,
  TMPDIR: temporaryRoot,
  TZ: "UTC",
};

try {
  const currentDescriptor = join(temporaryRoot, "threadline-current-descriptor-set.binpb");
  const previousDescriptor = join(temporaryRoot, "threadline-n-minus-one-descriptor-set.binpb");
  const previousRoot = join(temporaryRoot, "n-minus-one");
  extractProtoSnapshot(baseline, previousRoot);
  run(
    buf,
    ["build", "proto", "--as-file-descriptor-set", "--exclude-source-info", "-o", currentDescriptor],
    { cwd: repositoryRoot, env: { PATH: bufPath, TMPDIR: temporaryRoot } },
  );
  run(
    buf,
    ["build", join(previousRoot, "proto"), "--as-file-descriptor-set", "--exclude-source-info", "-o", previousDescriptor],
    { cwd: repositoryRoot, env: { PATH: bufPath, TMPDIR: temporaryRoot } },
  );

  if (mode === "--connected") {
    run(cargo, ["fetch", "--locked", "--manifest-path", harnessManifest], { cwd: repositoryRoot, env: environment });
  }

  const cargoArguments = ["run", "--locked"];
  if (mode === "--offline") cargoArguments.push("--offline");
  cargoArguments.push("--quiet", "--manifest-path", harnessManifest, "--", currentDescriptor, previousDescriptor, frameRoot);
  if (requireCurrentBindings) cargoArguments.push("--require-current-recovery-bindings", legacyRecoveryFrame);
  const verification = run(cargo, cargoArguments, { cwd: repositoryRoot, env: environment });
  process.stdout.write(verification.stdout);
} finally {
  rmSync(temporaryRoot, { recursive: true, force: true });
}
