import { spawnSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const protoRoot = join(repositoryRoot, "proto");
const goldenRoot = join(protoRoot, "golden", "v1");
const harnessManifest = join(protoRoot, "tools", "rust-envelope-compat", "Cargo.toml");
const buf = process.env.THREADLINE_BUF;
const cargo = process.env.THREADLINE_CARGO;
const mode = process.argv[2];
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
  throw new Error("usage: verify-rust-envelope-preservation.mjs <--connected|--offline>");
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
  const descriptor = join(temporaryRoot, "threadline-descriptor-set.binpb");
  run(
    buf,
    ["build", "proto", "--as-file-descriptor-set", "--exclude-source-info", "-o", descriptor],
    { cwd: repositoryRoot, env: { PATH: bufPath, TMPDIR: temporaryRoot } },
  );

  if (mode === "--connected") {
    run(cargo, ["fetch", "--locked", "--manifest-path", harnessManifest], { cwd: repositoryRoot, env: environment });
  }

  const cargoArguments = ["run", "--locked"];
  if (mode === "--offline") cargoArguments.push("--offline");
  cargoArguments.push("--quiet", "--manifest-path", harnessManifest, "--", descriptor, goldenRoot);
  const verification = run(cargo, cargoArguments, { cwd: repositoryRoot, env: environment });
  process.stdout.write(verification.stdout);
} finally {
  rmSync(temporaryRoot, { recursive: true, force: true });
}
