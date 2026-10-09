import { createHash } from "node:crypto";
import { existsSync, lstatSync, readFileSync, readdirSync } from "node:fs";
import { join, relative, sep } from "node:path";

const policies = Object.freeze({
  "verify-only": Object.freeze({ profile: "release", allowStubs: false, lockedTrees: true, install: false }),
  repository: Object.freeze({ profile: "release", allowStubs: false, lockedTrees: true, install: true }),
  candidate: Object.freeze({ profile: "release", allowStubs: false, lockedTrees: false, install: false }),
  "protocol-smoke": Object.freeze({ profile: "protocol-smoke", allowStubs: true, lockedTrees: false, install: false }),
});

export function codegenPolicy(mode) {
  if (!Object.hasOwn(policies, mode)) throw new Error(`unsupported codegen mode: ${mode}`);
  return policies[mode];
}

export function parseCodegenMode(args) {
  if (args.length !== 1 || !args[0].startsWith("--mode=")) {
    throw new Error("exactly one mode is required: --mode=verify-only, --mode=repository, --mode=candidate, or --mode=protocol-smoke");
  }
  const mode = args[0].slice("--mode=".length);
  codegenPolicy(mode);
  return mode;
}

export function outputDefinitions(toolchain) {
  return [
    ["go", toolchain.outputs.go],
    ["typescript", toolchain.outputs.typescript],
    ["rust", toolchain.outputs.rust],
    ["swift", toolchain.outputs.swift],
    ["kotlinJava", toolchain.outputs.kotlin.javaMessages],
    ["kotlinDsl", toolchain.outputs.kotlin.kotlinDsl],
  ];
}

export function filesBelow(directory, suffix = "") {
  if (!existsSync(directory)) return [];
  if (lstatSync(directory).isSymbolicLink()) throw new Error(`generated output must not contain symlinks: ${directory}`);
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return filesBelow(path, suffix);
    if (entry.isSymbolicLink()) throw new Error(`generated output must not contain symlinks: ${path}`);
    if (!entry.isFile()) throw new Error(`generated output contains an unsupported filesystem entry: ${path}`);
    return path.endsWith(suffix) ? [path] : [];
  });
}

function inspectOutputTree(directory, name, checks, allowStubs) {
  const files = filesBelow(directory);
  if (files.length === 0) throw new Error(`${name} generator produced no files`);
  const sources = files.map((path) => ({ path, relativePath: relative(directory, path).split(sep).join("/"), bytes: readFileSync(path) }))
    .sort((left, right) => left.relativePath < right.relativePath ? -1 : left.relativePath > right.relativePath ? 1 : 0);
  // Match the release manifest's canonical file\0path\0digest\n tree encoding.
  const lines = sources.map(({ relativePath, bytes }) => {
    const digest = createHash("sha256").update(bytes).digest("hex");
    return `file\0${relativePath}\0${digest}\n`;
  });
  const actual = { fileCount: files.length, treeSha256: createHash("sha256").update(lines.join("")).digest("hex") };
  if (allowStubs) return actual;
  const combined = sources.map(({ bytes }) => bytes.toString("utf8")).join("\n");
  if (combined.includes("THREADLINE_PROTOCOL_STUB")) throw new Error(`${name} output contains a protocol-stub marker`);
  const expectedSources = sources.filter(({ path }) => checks.extensions.some((extension) => path.endsWith(extension)));
  if (expectedSources.length === 0) throw new Error(`${name} produced no source with an expected extension`);
  for (const { path, bytes } of expectedSources) {
    if (!checks.signatureRegexAny.some((signature) => new RegExp(signature, "su").test(bytes.toString("utf8")))) {
      throw new Error(`${name} source lacks an accepted real-generator signature: ${path}`);
    }
  }
  for (const structure of checks.structureRegex) {
    if (!new RegExp(structure, "su").test(combined)) throw new Error(`${name} output lacks expected merged-contract structure: ${structure}`);
  }
  return actual;
}

function assertLockedOutput(name, actual, checks) {
  if (!Number.isSafeInteger(checks.fileCount) || checks.fileCount <= 0 || actual.fileCount !== checks.fileCount) {
    throw new Error(`${name} generated file count mismatch: expected ${checks.fileCount}; got ${actual.fileCount}`);
  }
  if (typeof checks.treeSha256 !== "string" || !/^[0-9a-f]{64}$/u.test(checks.treeSha256)) {
    throw new Error(`${name} generated treeSha256 must be canonical lowercase hex (64 characters)`);
  }
  if (actual.treeSha256 !== checks.treeSha256) {
    throw new Error(`${name} generated exact tree mismatch: expected ${checks.treeSha256}; got ${actual.treeSha256}`);
  }
}

export function inspectGeneratedOutput(generatedRoot, toolchain, mode) {
  const policy = codegenPolicy(mode);
  return Object.fromEntries(outputDefinitions(toolchain).map(([name, output]) => {
    let directory = generatedRoot;
    for (const segment of ["", ...output.split("/")]) {
      directory = join(directory, segment);
      if (existsSync(directory) && lstatSync(directory).isSymbolicLink()) throw new Error(`generated output must not contain symlinks: ${directory}`);
    }
    const actual = inspectOutputTree(directory, name, toolchain.generationChecks[name], policy.allowStubs);
    if (policy.lockedTrees) assertLockedOutput(name, actual, toolchain.generationChecks[name]);
    return [name, actual];
  }));
}

export function reportCodegenResult(mode, summary, synchronize) {
  const policy = codegenPolicy(mode);
  if (policy.install) synchronize();
  const { protoCount, compiled, generationTrees } = summary;
  if (mode === "candidate") {
    console.log(`CODEGEN_CANDIDATE ${JSON.stringify({ schemaVersion: 1, status: "candidate-awaiting-review", installed: false, protoCount, compiled, generationTrees })}`);
  } else if (policy.lockedTrees) {
    const location = policy.install ? "before repository synchronization" : "in temporary output only";
    console.log(`Verified release codegen ${location}: ${protoCount} Proto, ${compiled.generatedJava} Java, ${compiled.generatedKotlin} Kotlin files.`);
  } else {
    console.log(`PROTOCOL-SMOKE ONLY: plugin protocol execution and Java/Kotlin compilation passed for ${protoCount} Proto files; this is not full release-codegen evidence.`);
  }
}

export function candidateEvidence(log, metadata, toolchain) {
  const records = log.split("\n").filter((line) => line.startsWith("CODEGEN_CANDIDATE "));
  if (records.length !== 1) throw new Error("candidate run must emit exactly one inventory record");
  const report = JSON.parse(records[0].slice("CODEGEN_CANDIDATE ".length));
  const keys = ["compiled", "generationTrees", "installed", "protoCount", "schemaVersion", "status"];
  if (JSON.stringify(Object.keys(report).sort()) !== JSON.stringify(keys)) throw new Error("unexpected candidate inventory fields");
  if (report.schemaVersion !== 1 || report.status !== "candidate-awaiting-review" || report.installed !== false) {
    throw new Error("candidate inventory must remain provisional and uninstalled");
  }
  const names = outputDefinitions(toolchain).map(([name]) => name).sort();
  if (JSON.stringify(Object.keys(report.generationTrees).sort()) !== JSON.stringify(names)) {
    throw new Error("candidate inventory must contain exactly the six declared output trees");
  }
  for (const name of names) assertCandidateTree(name, report.generationTrees[name]);
  return { ...metadata, ...report };
}

function assertCandidateTree(name, tree) {
  if (!Number.isSafeInteger(tree.fileCount) || tree.fileCount <= 0) throw new Error(`${name} candidate file count must be positive`);
  if (typeof tree.treeSha256 !== "string" || !/^[0-9a-f]{64}$/u.test(tree.treeSha256)) {
    throw new Error(`${name} candidate treeSha256 must be canonical lowercase hex`);
  }
}
