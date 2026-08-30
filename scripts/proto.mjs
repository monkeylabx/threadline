import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { delimiter, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const protoDir = join(root, "proto");
const localBin = join(root, "node_modules", ".bin");

// Generated code is never hand-edited, so every language is described here
// rather than in a per-language script. A language whose plugins are absent is
// skipped with a named reason instead of failing the whole run: no single
// workstation is expected to hold five toolchains, but CI installs them all.
const languages = [
  {
    name: "go",
    template: "buf.gen.go.yaml",
    plugins: ["protoc-gen-go", "protoc-gen-connect-go"],
    out: "services/gen",
  },
  {
    name: "ts",
    template: "buf.gen.ts.yaml",
    plugins: ["protoc-gen-es"],
    out: "packages/generated-ts/src",
  },
  {
    name: "rust",
    template: "buf.gen.rust.yaml",
    plugins: ["protoc-gen-prost", "protoc-gen-prost-crate"],
    out: "crates/client-proto/src/generated",
  },
  {
    name: "swift",
    template: "buf.gen.swift.yaml",
    plugins: ["protoc-gen-swift", "protoc-gen-connect-swift"],
    out: "packages/generated-swift/Sources/ThreadlineProto",
  },
  {
    name: "kotlin",
    template: "buf.gen.kotlin.yaml",
    plugins: ["protoc-gen-java", "protoc-gen-kotlin", "protoc-gen-connect-kotlin"],
    out: "packages/generated-kotlin/src/main/kotlin",
  },
];

// Buf plugins are resolved as plain executables, so node_modules/.bin has to be
// ahead of the inherited PATH for the npm-provided ones to win.
const env = {
  ...process.env,
  PATH: `${localBin}${delimiter}${process.env.PATH ?? ""}`,
};

function resolveBuf() {
  const vendored = join(localBin, "buf");
  if (existsSync(vendored)) return vendored;
  const probe = spawnSync("buf", ["--version"], { encoding: "utf8", env });
  if (probe.status === 0) return "buf";
  return null;
}

const buf = resolveBuf();

function requireBuf() {
  if (buf) return true;
  console.error(
    "[proto] buf is not installed. Add the workspace dev dependencies " +
      "(@bufbuild/buf) or install the Buf CLI on PATH.",
  );
  return false;
}

function hasPlugin(name) {
  if (existsSync(join(localBin, name))) return true;
  const probe = spawnSync(name, ["--version"], { encoding: "utf8", env });
  return probe.error?.code !== "ENOENT";
}

function run(label, args, cwd = protoDir) {
  console.log(`\n[${label}] buf ${args.join(" ")}`);
  const result = spawnSync(buf, args, { cwd, stdio: "inherit", env });
  if (result.status !== 0) {
    console.error(`[${label}] failed: exit ${result.status}`);
    return false;
  }
  return true;
}

// The compile gate. `build` is the language-independent proof that every file
// parses and every cross-file reference resolves; lint and format keep the
// contract readable for the five teams that consume it.
function check() {
  if (!requireBuf()) return false;
  return [
    run("build", ["build"]),
    run("formal-build", ["build", "--config", join(protoDir, "buf.formal.yaml")], root),
    run("lint", ["lint"]),
    run("format", ["format", "--diff", "--exit-code"]),
  ].every(Boolean);
}

// Compatibility gate. Public fields may only be added; a removed field must be
// `reserved`. Checked against `main` so a PR that breaks the wire fails before
// five platforms have generated against it.
function breaking() {
  if (!requireBuf()) return false;

  // On a CI pull request checkout there is no local `main` branch, only the
  // remote-tracking ref, so try that first.
  const baseline = ["origin/main", "main"].find((ref) => {
    const probe = spawnSync("git", ["ls-tree", ref, "--", "proto/buf.yaml"], {
      cwd: root,
      encoding: "utf8",
    });
    return probe.status === 0 && probe.stdout.trim() !== "";
  });

  if (!baseline) {
    console.log(
      "\n[breaking] skipped: no proto module on main yet. " +
        "The first landing establishes the baseline.",
    );
    return true;
  }

  return run("breaking", ["breaking", "--against", `../.git#ref=${baseline},subdir=proto`]);
}

function generate(requested) {
  if (!requireBuf()) return false;

  const selected =
    requested.length > 0
      ? languages.filter((language) => requested.includes(language.name))
      : languages;

  const unknown = requested.filter(
    (name) => !languages.some((language) => language.name === name),
  );
  for (const name of unknown) console.error(`[generate] unknown language: ${name}`);
  if (unknown.length > 0) return false;

  let ok = true;
  for (const language of selected) {
    const missing = language.plugins.filter((plugin) => !hasPlugin(plugin));
    if (missing.length > 0) {
      // Naming a language explicitly is a request, not a preference: failing
      // loudly beats reporting success for output that was never written.
      if (requested.length > 0) {
        console.error(`[generate:${language.name}] missing plugins: ${missing.join(", ")}`);
        ok = false;
      } else {
        console.log(`\n[generate:${language.name}] skipped, missing: ${missing.join(", ")}`);
      }
      continue;
    }
    ok =
      run(`generate:${language.name}`, [
        "generate",
        "--template",
        language.template,
      ]) && ok;
    if (ok) console.log(`[generate:${language.name}] wrote ${language.out}`);
  }
  return ok;
}

const action = process.argv[2] ?? "check";
const args = process.argv.slice(3);

switch (action) {
  case "check":
    if (!check()) process.exitCode = 1;
    break;
  case "breaking":
    if (!breaking()) process.exitCode = 1;
    break;
  case "generate":
    if (!generate(args)) process.exitCode = 1;
    break;
  case "verify":
    if (![check(), breaking(), generate([])].every(Boolean)) process.exitCode = 1;
    break;
  default:
    console.error(`Unknown proto action: ${action}`);
    console.error("Usage: node scripts/proto.mjs <check|breaking|generate|verify> [languages...]");
    process.exitCode = 2;
}
