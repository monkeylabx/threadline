import test from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, symlinkSync, chmodSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { narrative, classify, checkDocs, localLinks } from "./docs-only.mjs";

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "threadline-docs-ci-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
  git("init", "--quiet");
  git("config", "user.email", "ci@example.invalid");
  git("config", "user.name", "CI fixture");
  const write = (path, value) => {
    mkdirSync(dirname(join(root, path)), { recursive: true });
    writeFileSync(join(root, path), value);
  };
  const commit = () => { git("add", "."); git("commit", "--quiet", "-m", "fixture"); return git("rev-parse", "HEAD"); };
  write("README.md", "# Test\n");
  write("services/core/main.go", "package main\n");
  return { root, git, write, commit, base: commit() };
}

test("allowlist never treats source, prototype, fixture, CI or unknown files as prose", () => {
  for (const path of ["README.md", "AGENTS.md", "docs/contracts/a.md", "docs/adr/a.md", "docs/research/nested/a.md"]) assert.equal(narrative(path), true, path);
  for (const path of ["docs/prototype/a.md", "docs/spikes/a.md", "docs/contracts/a.rs", "docs/new/a.md", "Cargo.lock", ".github/workflows/build.yml", ".agents/a.md", "services/README.md"]) assert.equal(narrative(path), false, path);
});

test("docs addition, deletion and docs rename stay light; no-change and mixed changes stay full", (t) => {
  const f = fixture(t);
  assert.equal(classify(f.root, "pull_request", f.base, f.base).docsOnly, false);
  f.write("docs/contracts/a.md", "# Contract\n");
  const added = f.commit();
  assert.equal(classify(f.root, "pull_request", f.base, added).docsOnly, true);
  f.git("mv", "docs/contracts/a.md", "docs/contracts/b.md");
  const renamed = f.commit();
  assert.equal(classify(f.root, "pull_request", added, renamed).docsOnly, true);
  f.git("rm", "docs/contracts/b.md");
  assert.equal(classify(f.root, "pull_request", renamed, f.commit()).docsOnly, true);
  f.write("services/core/main.go", "package changed\n");
  assert.equal(classify(f.root, "pull_request", f.base, f.commit()).docsOnly, false);
});

test("rename of source to Markdown cannot hide its old path", (t) => {
  const f = fixture(t);
  f.git("mv", "services/core/main.go", "README-source.md");
  const result = classify(f.root, "pull_request", f.base, f.commit());
  assert.equal(result.docsOnly, false);
  assert.ok(result.paths.includes("services/core/main.go"));
});

test("symlinks and executable Markdown cannot take the lightweight path", (t) => {
  const f = fixture(t);
  symlinkSync("services/core/main.go", join(f.root, "CONTEXT.md"));
  assert.equal(classify(f.root, "pull_request", f.base, f.commit()).docsOnly, false);
  f.git("rm", "CONTEXT.md");
  chmodSync(join(f.root, "README.md"), 0o755);
  assert.equal(classify(f.root, "pull_request", f.base, f.commit()).docsOnly, false);
});

test("manual runs are full; malformed or unavailable PR evidence fails", (t) => {
  const f = fixture(t);
  assert.equal(classify(f.root, "workflow_dispatch").docsOnly, false);
  assert.throws(() => classify(f.root, "pull_request", "main", f.base));
  assert.throws(() => classify(f.root, "pull_request", "a".repeat(40), f.base));
});

test("Markdown links include images and definitions, excluding code and remote URLs", () => {
  assert.deepEqual(localLinks('[a](a.md#part) ![b](<with space.png>)\n[x]: ref.md\n[c](https://example.com) [d](#part)\n`[example](absent)`\n```md\n[x](absent)\n```\n'), ["a.md", "with space.png", "ref.md"]);
});

test("changed prose validates links and whitespace, and deleted files are safe", (t) => {
  const f = fixture(t);
  f.write("docs/contracts/a.md", "[source](../../services/core/main.go)\n");
  const good = f.commit();
  assert.doesNotThrow(() => checkDocs(f.root, f.base, good, ["docs/contracts/a.md"]));
  f.write("docs/contracts/a.md", "[missing](absent.md)\n");
  assert.throws(() => checkDocs(f.root, good, f.commit(), ["docs/contracts/a.md"]), /Missing/);
  f.write("docs/contracts/a.md", "trailing space \n");
  assert.throws(() => checkDocs(f.root, good, f.commit(), ["docs/contracts/a.md"]));
  f.git("rm", "docs/contracts/a.md");
  assert.doesNotThrow(() => checkDocs(f.root, good, f.commit(), ["docs/contracts/a.md"]));
});

test("escaping link targets fail even if the host target exists", (t) => {
  const f = fixture(t);
  f.write("README.md", "[host](../../../../etc/passwd)\n");
  assert.throws(() => checkDocs(f.root, f.base, f.commit(), ["README.md"]), /escaping/);
});

test("required job names, failure propagation and full-build gating remain wired", () => {
  const workflow = readFileSync(new URL("../../.github/workflows/build.yml", import.meta.url), "utf8");
  assert.ok(!workflow.includes("paths-ignore:"));
  assert.ok(workflow.includes("run: node --test scripts/toolchain.test.mjs scripts/ci/docs-only.test.mjs"));
  assert.ok(workflow.includes("PR_BASE_SHA: ${{ github.event.pull_request.base.sha }}"));
  const full = "needs.contracts.result == 'success' && needs.contracts.outputs.docs_only != 'true'";
  for (const id of ["workspace-linux", "postgresql", "desktop", "apple", "android"]) {
    const block = workflow.split(`\n  ${id}:\n`)[1].split(/\n  [a-z-]+:\n/)[0];
    assert.match(block, /needs: contracts\n    if: always\(\)/);
    assert.match(block, /if: needs.contracts.result != 'success'\n        run: exit 1/);
    const originalSteps = block.split(/^      - name:/m).slice(3);
    for (const step of originalSteps) assert.ok(step.includes(`        if: ${full}`), `${id}: ${step.split("\n")[0]}`);
  }
  assert.ok(workflow.includes("name: desktop-${{ matrix.name }}"));
  assert.ok(workflow.includes("matrix.os || 'ubuntu-24.04'"));
  assert.ok(workflow.includes("- os: macos-26"));
  assert.ok(workflow.includes("matrix.image || '' }}")); // No PostgreSQL service on docs/failure lane.
});

test("CLI emits skip only after validation and only for the exact checked-out commit", (t) => {
  const f = fixture(t);
  const output = join(f.root, ".git", "routing-output");
  const script = new URL("./docs-only.mjs", import.meta.url);
  f.write("docs/contracts/a.md", "[source](../../README.md)\n");
  const good = f.commit();
  const invoke = (sha) => execFileSync(process.execPath, [script.pathname], {
    cwd: f.root, stdio: "pipe", env: { ...process.env, GITHUB_EVENT_NAME: "pull_request", PR_BASE_SHA: f.base, GITHUB_SHA: sha, GITHUB_OUTPUT: output, GITHUB_STEP_SUMMARY: "" },
  });
  invoke(good);
  assert.equal(readFileSync(output, "utf8"), "docs_only=true\n");
  writeFileSync(output, "");
  f.write("docs/contracts/a.md", "[missing](missing.md)\n");
  const bad = f.commit();
  assert.throws(() => invoke(bad));
  assert.equal(readFileSync(output, "utf8"), "");
  assert.throws(() => invoke(good));
  assert.equal(readFileSync(output, "utf8"), "");
});

test("balanced and escaped parentheses resolve the exact Markdown destination", (t) => {
  assert.deepEqual(localLinks(String.raw`[a](guide(v2).md) [b](guide\(v2\).md) [c](guide(nested(v2)).md "title")`), ["guide(v2).md", "guide(v2).md", "guide(nested(v2)).md"]);
  const f = fixture(t);
  f.write("docs/contracts/guide(v2).md", "# Guide\n");
  f.write("README.md", "[guide](docs/contracts/guide(v2).md)\n");
  const head = f.commit();
  assert.doesNotThrow(() => checkDocs(f.root, f.base, head, ["README.md"]));
  f.write("README.md", "[missing](docs/contracts/missing(v2).md)\n");
  assert.throws(() => checkDocs(f.root, head, f.commit(), ["README.md"]), /Missing/);
});

test("conditional runner and service selection remains compatible with repository pin verification", async () => {
  const { verifyPins } = await import("../toolchain.mjs");
  assert.equal(verifyPins(), true);
});
