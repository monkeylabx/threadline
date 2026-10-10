import { createHash } from "node:crypto";
import { existsSync, lstatSync, mkdirSync, readFileSync, realpathSync, writeFileSync } from "node:fs";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { inspectGeneratedOutput, outputDefinitions } from "./codegen-output.mjs";

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function verifiedGit(bundle, manifestDigest) {
  const bytes = readFileSync(join(bundle, "manifest.json"));
  if (sha256(bytes) !== manifestDigest) throw new Error("export manifest digest mismatch");
  const manifest = JSON.parse(bytes);
  if (manifest.profile !== "release" || !manifest.tools.git) throw new Error("export requires the reviewed release manifest with Git");
  const git = resolve(bundle, manifest.tools.git.path);
  const within = relative(realpathSync(bundle), realpathSync(git));
  if (isAbsolute(within) || within === ".." || within.startsWith(`..${sep}`)) throw new Error("export Git escapes its bundle");
  if (lstatSync(git).isSymbolicLink() || !lstatSync(git).isFile() || sha256(readFileSync(git)) !== manifest.tools.git.sha256) {
    throw new Error("export Git executable digest/type mismatch");
  }
  return git;
}

function runGit(git, repositoryRoot, args) {
  const result = spawnSync(git, [
    "-C", repositoryRoot, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "core.autocrlf=false", ...args,
  ], { encoding: "utf8", maxBuffer: 128 * 1024 * 1024, env: {
    PATH: process.platform === "win32" ? process.env.PATH : "/usr/bin:/bin:/usr/sbin:/sbin",
    SystemRoot: process.env.SystemRoot, HOME: repositoryRoot,
    GIT_CONFIG_NOSYSTEM: "1", GIT_CONFIG_GLOBAL: process.platform === "win32" ? "NUL" : "/dev/null", GIT_OPTIONAL_LOCKS: "0",
  } });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`export Git failed: ${result.stderr}`);
  return result.stdout;
}

export function verifyInstallArtifact(evidence, patch, expected) {
  if (evidence.schemaVersion !== 1 || evidence.status !== "passed" || evidence.installed !== true) throw new Error("artifact is not completed repository installation evidence");
  for (const [name, value] of Object.entries(expected.identity)) {
    if (evidence[name] !== value) throw new Error(`artifact identity mismatch: ${name}`);
  }
  if (sha256(patch) !== evidence.patchSha256) throw new Error("SDK patch digest mismatch");
  const trees = Object.fromEntries(outputDefinitions(expected.toolchain).map(([name]) => {
    const { fileCount, treeSha256 } = expected.toolchain.generationChecks[name];
    return [name, { fileCount, treeSha256 }];
  }));
  if (JSON.stringify(evidence.generationTrees) !== JSON.stringify(trees)) throw new Error("artifact tree inventory differs from the reviewed lock");
}

export function exportCodegenInstallation({ repositoryRoot, bundle, outputRoot, identity, toolchain }) {
  if (existsSync(outputRoot)) throw new Error("export output must be a new directory");
  const canonicalOutput = join(realpathSync(dirname(resolve(outputRoot))), basename(outputRoot));
  const fromRepository = relative(realpathSync(repositoryRoot), canonicalOutput);
  if (fromRepository === "" || (!isAbsolute(fromRepository) && !fromRepository.startsWith(`..${sep}`) && fromRepository !== "..")) {
    throw new Error("export output must be outside the repository");
  }
  const git = verifiedGit(bundle, identity.manifestSha256);
  if (runGit(git, repositoryRoot, ["rev-parse", "HEAD"]).trim() !== identity.targetSha) throw new Error("export target commit mismatch");
  const generationTrees = inspectGeneratedOutput(repositoryRoot, toolchain, "verify-only");
  const outputs = outputDefinitions(toolchain).map(([, output]) => output);
  runGit(git, repositoryRoot, ["add", "--", ...outputs]);
  const patch = runGit(git, repositoryRoot, ["diff", "--cached", "--binary", "--no-ext-diff", "--no-textconv", "--no-color", "--", ...outputs]);
  const evidence = { schemaVersion: 1, ...identity, status: "passed", installed: true, patchSha256: sha256(patch), generationTrees, physicalDevices: "NOT RUN" };
  verifyInstallArtifact(evidence, patch, { identity, toolchain });
  mkdirSync(outputRoot, { mode: 0o700 });
  writeFileSync(join(outputRoot, "codegen-sdk.patch"), patch);
  writeFileSync(join(outputRoot, "install-evidence.json"), `${JSON.stringify(evidence, null, 2)}\n`);
  return evidence;
}

function main() {
  const repositoryRoot = fileURLToPath(new URL("../../", import.meta.url));
  const required = ["BUNDLE", "SDK_EXPORT", "TARGET_PR", "TARGET_SHA", "PREPARED_RUN_ID", "MANIFEST_SHA256", "GITHUB_RUN_ID"];
  for (const name of required) if (!process.env[name]) throw new Error(`${name} is required`);
  const identity = {
    targetPr: process.env.TARGET_PR, targetSha: process.env.TARGET_SHA,
    preparedRunId: process.env.PREPARED_RUN_ID, installRunId: process.env.GITHUB_RUN_ID,
    manifestSha256: process.env.MANIFEST_SHA256,
  };
  console.log(JSON.stringify(exportCodegenInstallation({
    repositoryRoot, bundle: resolve(process.env.BUNDLE), outputRoot: resolve(process.env.SDK_EXPORT), identity,
    toolchain: JSON.parse(readFileSync(join(repositoryRoot, "proto/toolchain.lock.json"))),
  })));
}

if (process.argv[1] && realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url))) main();
