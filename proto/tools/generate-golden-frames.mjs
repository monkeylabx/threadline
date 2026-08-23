import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const goldenRoot = join(repositoryRoot, "proto", "golden", "v1");
const manifest = JSON.parse(readFileSync(join(goldenRoot, "manifest.json"), "utf8"));
const buf = process.env.THREADLINE_BUF;
const mode = process.argv[2];

if (!buf || !isAbsolute(buf)) throw new Error("THREADLINE_BUF must name the absolute pinned Buf executable");
if (mode !== "--check" && mode !== "--write") throw new Error("usage: generate-golden-frames.mjs <--check|--write>");

const version = spawnSync(buf, ["--version"], { encoding: "utf8", env: { PATH: dirname(buf) } });
if (version.status !== 0 || version.stdout.trim() !== "1.72.0") throw new Error("THREADLINE_BUF must be pinned Buf 1.72.0");

const temporaryRoot = mkdtempSync(join(tmpdir(), "threadline-golden-"));
try {
  for (const frame of manifest.frames) {
    const knownOutput = join(temporaryRoot, `${basename(frame.file)}.known.binpb`);
    const conversion = spawnSync(
      buf,
      [
        "convert",
        "proto",
        "--type",
        frame.contract,
        "--from",
        `proto/golden/v1/${frame.sourceJson}#format=json`,
        "--to",
        `${knownOutput}#format=binpb`,
      ],
      { cwd: repositoryRoot, encoding: "utf8", env: { PATH: dirname(buf), TMPDIR: temporaryRoot } },
    );
    if (conversion.status !== 0) throw new Error(`${frame.contract} conversion failed: ${conversion.stderr}`);

    const canary = manifest.canaries.find((candidate) => candidate.contract === frame.canary);
    if (!canary) throw new Error(`${frame.contract} references unknown canary ${frame.canary}`);
    const expected = Buffer.concat([
      readFileSync(knownOutput),
      Buffer.from(readFileSync(join(goldenRoot, canary.file), "utf8").trim(), "hex"),
    ]).toString("hex");
    const destination = join(goldenRoot, frame.file);
    if (mode === "--write") writeFileSync(destination, `${expected}\n`, "utf8");
    else if (readFileSync(destination, "utf8").trim() !== expected) throw new Error(`${frame.file} is stale; rerun with --write`);
  }
} finally {
  rmSync(temporaryRoot, { recursive: true, force: true });
}

console.log(`${mode === "--write" ? "Generated" : "Verified"} representative Golden Frames with pinned Buf 1.72.0.`);
