import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { basename, resolve } from "node:path";

import {
  loadScopedGeneratedCompatManifest,
  repositoryFile,
  repositoryRoot,
} from "./generated-envelope-compat/scoped-manifest.mjs";

const supportedLanguages = new Set(["go", "typescript", "rust", "kotlin", "swift"]);

function option(name) {
  const prefix = `--${name}=`;
  const matches = process.argv.slice(2).filter((value) => value.startsWith(prefix));
  if (matches.length !== 1) throw new Error(`exactly one ${prefix}<value> argument is required`);
  return matches[0].slice(prefix.length);
}

function run(command, args) {
  const result = spawnSync(command, args, { cwd: repositoryRoot, encoding: "utf8", stdio: "inherit", env: process.env });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${basename(command)} ${args.join(" ")} exited ${result.status}`);
}

function gitFile(commit, path) {
  const result = spawnSync("git", ["show", `${commit}:${path}`], { cwd: repositoryRoot, encoding: null, stdio: "pipe" });
  if (result.error || result.status !== 0) throw new Error(`cannot read ${commit}:${path}`);
  return result.stdout;
}

function gitOutput(args) {
  const result = spawnSync("git", args, { cwd: repositoryRoot, encoding: "utf8", stdio: "pipe" });
  if (result.error || result.status !== 0) throw new Error(`git ${args.join(" ")} failed`);
  return result.stdout.trim();
}

const baseline = option("baseline");
const target = option("target");
const manifestArgument = option("frame-manifest");
const languagesArgument = option("languages");
if (process.argv.length !== 6) {
  throw new Error("usage: node proto/tools/verify-t019-generated-compat.mjs --baseline=<full-sha> --target=<full-sha> --frame-manifest=<repository-relative-json> --languages=go,typescript,rust,kotlin,swift");
}
if (!/^[0-9a-f]{40}$/u.test(baseline) || !/^[0-9a-f]{40}$/u.test(target)) {
  throw new Error("--baseline and --target must be full Git object IDs");
}
const languages = languagesArgument.split(",").filter(Boolean);
if (languages.length === 0 || new Set(languages).size !== languages.length || languages.some((value) => !supportedLanguages.has(value))) {
  throw new Error(`languages must be a unique subset of ${[...supportedLanguages].join(",")}`);
}

const { manifest } = loadScopedGeneratedCompatManifest(manifestArgument, { baseline, target });
for (const name of ["THREADLINE_BUF", "THREADLINE_NODE", ...languages.flatMap((language) => manifest.adapterInputs.requiredEnvironmentByLanguage[language] ?? [])]) {
  if (!process.env[name]) throw new Error(`${name} is required for the selected scoped adapter set`);
}
const schemaPath = repositoryFile(manifest.targetSchema.file);
if (!readFileSync(schemaPath).equals(gitFile(target, manifest.targetSchema.file))) {
  throw new Error("working-tree RecoveryEnvelope schema differs from the pinned target schema commit");
}
const targetProtoPaths = gitOutput(["ls-tree", "-r", "--name-only", target, "--", "proto"])
  .split("\n")
  .filter((path) => path.endsWith(".proto"))
  .sort();
const workingProtoPaths = gitOutput(["ls-files", "--", "proto"])
  .split("\n")
  .filter((path) => path.endsWith(".proto"))
  .sort();
if (JSON.stringify(workingProtoPaths) !== JSON.stringify(targetProtoPaths)) {
  throw new Error("working-tree Proto file set differs from the pinned target schema commit");
}
for (const path of targetProtoPaths) {
  if (!readFileSync(resolve(repositoryRoot, path)).equals(gitFile(target, path))) {
    throw new Error(`working-tree schema differs from pinned target: ${path}`);
  }
}

run(process.execPath, [resolve(repositoryRoot, "proto/tools/verify-crypto-contracts.mjs")]);
run(process.execPath, [
  resolve(repositoryRoot, "proto/tools/verify-generated-envelope-compat.mjs"),
  `--languages=${languages.join(",")}`,
  `--baseline=${baseline}`,
  `--frame-manifest=${manifestArgument}`,
]);

console.log(`T019 scoped generated compatibility passed for ${languages.join(", ")} against ${baseline} using target schema ${target}.`);
