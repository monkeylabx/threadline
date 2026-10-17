import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { spawnSync } from "node:child_process";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { candidateEvidence, codegenPolicy, filesBelow, inspectGeneratedOutput, outputDefinitions, parseCodegenMode, reportCodegenResult } from "./codegen-output.mjs";

const lock = JSON.parse(readFileSync(new URL("../toolchain.lock.json", import.meta.url), "utf8"));
const content = "// Real generator signature\nContractStructure\n";
const fileDigest = createHash("sha256").update(content).digest("hex");
const treeDigest = createHash("sha256").update(`file\0nested/fixture.source\0${fileDigest}\n`).digest("hex");

function fixture(context) {
  const root = mkdtempSync(join(tmpdir(), "threadline-codegen-output-"));
  context.after(() => rmSync(root, { recursive: true, force: true }));
  const toolchain = structuredClone(lock);
  for (const [name, output] of outputDefinitions(toolchain)) {
    const path = join(root, output, "nested", "fixture.source");
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, content);
    toolchain.generationChecks[name] = {
      fileCount: 1, treeSha256: treeDigest, extensions: [".source"],
      signatureRegexAny: ["Real generator signature"], structureRegex: ["ContractStructure"],
    };
  }
  return { root, toolchain, file: join(root, toolchain.outputs.go, "nested", "fixture.source") };
}

test("candidate reports all six actual trees while formal modes retain locked counts and hashes", (context) => {
  const { root, toolchain, file } = fixture(context);
  const expected = Object.fromEntries(outputDefinitions(toolchain).map(([name]) => [name, { fileCount: 1, treeSha256: treeDigest }]));
  assert.deepEqual(inspectGeneratedOutput(root, toolchain, "candidate"), expected);
  for (const mode of ["verify-only", "repository"]) assert.deepEqual(inspectGeneratedOutput(root, toolchain, mode), expected);
  assert.equal(existsSync(join(root, "candidate-evidence.json")), false, "inspection must not write a report or install output");
  writeFileSync(file, `${content}changed\n`);
  assert.notDeepEqual(inspectGeneratedOutput(root, toolchain, "candidate").go, expected.go);
  for (const mode of ["verify-only", "repository"]) {
    assert.throws(() => inspectGeneratedOutput(root, toolchain, mode), /exact tree mismatch/u);
    toolchain.generationChecks.go.fileCount = 2;
    assert.throws(() => inspectGeneratedOutput(root, toolchain, mode), /file count mismatch/u);
    toolchain.generationChecks.go.fileCount = 1;
    toolchain.generationChecks.go.treeSha256 = "BAD";
    assert.throws(() => inspectGeneratedOutput(root, toolchain, mode), /canonical lowercase hex/u);
    toolchain.generationChecks.go.treeSha256 = treeDigest;
  }
  writeFileSync(file, content);
  writeFileSync(join(root, toolchain.outputs.go, "nestedZ.source"), content);
  const sortedDigest = createHash("sha256").update(`file\0nested/fixture.source\0${fileDigest}\nfile\0nestedZ.source\0${fileDigest}\n`).digest("hex");
  assert.deepEqual(inspectGeneratedOutput(root, toolchain, "candidate").go, { fileCount: 2, treeSha256: sortedDigest });
});

test("candidate rejects absent output and every invalid real-generator source", (context) => {
  const { root, toolchain, file } = fixture(context);
  for (const [source, error] of [
    ["THREADLINE_PROTOCOL_STUB", /protocol-stub marker/u],
    ["ContractStructure", /real-generator signature/u],
    ["Real generator signature", /merged-contract structure/u],
  ]) {
    writeFileSync(file, source);
    assert.throws(() => inspectGeneratedOutput(root, toolchain, "candidate"), error);
  }
  writeFileSync(file, "THREADLINE_PROTOCOL_STUB");
  assert.equal(inspectGeneratedOutput(root, toolchain, "protocol-smoke").go.fileCount, 1);
  assert.deepEqual(filesBelow(root, ".missing"), []);
  writeFileSync(file, content);
  toolchain.generationChecks.go.extensions = [".missing"];
  assert.throws(() => inspectGeneratedOutput(root, toolchain, "candidate"), /expected extension/u);
  toolchain.generationChecks.go.extensions = [".source"];
  rmSync(join(root, toolchain.outputs.kotlin.kotlinDsl), { recursive: true });
  assert.throws(() => inspectGeneratedOutput(root, toolchain, "candidate"), /kotlinDsl generator produced no files/u);
  mkdirSync(join(root, toolchain.outputs.kotlin.kotlinDsl));
  assert.throws(() => inspectGeneratedOutput(root, toolchain, "candidate"), /kotlinDsl generator produced no files/u);
});

test("output inspection rejects symlinked trees, ancestors, and nested files", (context) => {
  const { root, toolchain, file } = fixture(context);
  const outside = join(root, "outside");
  mkdirSync(outside);
  for (const target of [file, join(root, toolchain.outputs.go), join(root, "services")]) {
    rmSync(target, { recursive: true, force: true });
    symlinkSync(outside, target, "junction");
    assert.throws(() => inspectGeneratedOutput(root, toolchain, "candidate"), /must not contain symlinks/u);
    rmSync(target);
    mkdirSync(target, { recursive: true });
  }
  symlinkSync(outside, join(root, "root-link"), "junction");
  assert.throws(() => filesBelow(join(root, "root-link")), /must not contain symlinks/u);
  assert.deepEqual(filesBelow(outside), []);
});

test("mode admission distinguishes authenticated candidates from protocol stubs", (context) => {
  assert.deepEqual(codegenPolicy("candidate"), { profile: "release", allowStubs: false, lockedTrees: false, install: false });
  assert.deepEqual(codegenPolicy("protocol-smoke"), { profile: "protocol-smoke", allowStubs: true, lockedTrees: false, install: false });
  for (const mode of ["candidate", "verify-only", "repository", "protocol-smoke"]) assert.equal(parseCodegenMode([`--mode=${mode}`]), mode);
  for (const args of [[], ["candidate"], ["--mode=candidate", "--mode=repository"], ["--mode=unknown"], ["--mode=toString"]]) {
    assert.throws(() => parseCodegenMode(args));
  }
  const { root } = fixture(context);
  const manifest = join(root, "manifest.json");
  const bytes = JSON.stringify({ schemaVersion: 5, platform: `${process.platform}-${process.arch}`, profile: "protocol-smoke", sources: {}, closures: {}, tools: {} });
  writeFileSync(manifest, bytes);
  const env = {
    PATH: process.env.PATH, SystemRoot: process.env.SystemRoot,
    THREADLINE_PROTO_TOOL_MANIFEST: manifest,
    THREADLINE_PROTO_TOOL_MANIFEST_SHA256: createHash("sha256").update(bytes).digest("hex"),
  };
  const script = fileURLToPath(new URL("./verify-codegen.mjs", import.meta.url));
  const rejected = spawnSync(process.execPath, [script, "--mode=candidate"], { env, encoding: "utf8" });
  assert.notEqual(rejected.status, 0);
  assert.match(rejected.stderr, /candidate requires a release manifest profile/u);
  env.THREADLINE_PROTO_TOOL_MANIFEST_SHA256 = "0".repeat(64);
  const tampered = spawnSync(process.execPath, [script, "--mode=candidate"], { env, encoding: "utf8" });
  assert.notEqual(tampered.status, 0);
  assert.match(tampered.stderr, /manifest SHA-256 mismatch/u);
});

test("only repository completion installs; candidate evidence cannot claim a release PASS", (context) => {
  const { root, toolchain } = fixture(context);
  const summary = { protoCount: 24, compiled: { generatedJava: 1, generatedKotlin: 1 }, generationTrees: inspectGeneratedOutput(root, toolchain, "candidate") };
  const logs = [];
  context.mock.method(console, "log", (line) => logs.push(line));
  let installations = 0;
  for (const mode of ["candidate", "verify-only", "protocol-smoke"]) reportCodegenResult(mode, summary, () => { installations += 1; });
  assert.equal(installations, 0);
  assert.equal(logs[0].includes("Verified release"), false);
  const evidence = candidateEvidence(logs[0], { targetSha: "a".repeat(40), status: "passed" }, toolchain);
  assert.equal(evidence.status, "candidate-awaiting-review");
  assert.equal(evidence.installed, false);
  assert.deepEqual(evidence.generationTrees, summary.generationTrees);
  assert.equal(evidence.targetSha, "a".repeat(40));
  reportCodegenResult("repository", summary, () => { installations += 1; });
  assert.equal(installations, 1);
  assert.throws(() => reportCodegenResult("repository", summary, () => { throw new Error("installation failed"); }), /installation failed/u);
  assert.equal(logs.length, 4, "failed installation must not emit success");
  assert.throws(() => candidateEvidence("", {}, toolchain), /exactly one/u);
  assert.throws(() => candidateEvidence(`${logs[0]}\n${logs[0]}`, {}, toolchain), /exactly one/u);
  const report = JSON.parse(logs[0].slice("CODEGEN_CANDIDATE ".length));
  for (const change of [{ status: "passed" }, { installed: true }, { schemaVersion: 2 }, { targetSha: "forged" }, { generationTrees: {} }]) {
    assert.throws(() => candidateEvidence(`CODEGEN_CANDIDATE ${JSON.stringify({ ...report, ...change })}`, {}, toolchain));
  }
  for (const change of [{ fileCount: 0 }, { fileCount: 1.5 }, { treeSha256: "BAD" }]) {
    const invalid = structuredClone(report);
    Object.assign(invalid.generationTrees.go, change);
    assert.throws(() => candidateEvidence(`CODEGEN_CANDIDATE ${JSON.stringify(invalid)}`, {}, toolchain));
  }
});
