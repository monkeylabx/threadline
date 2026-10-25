#!/usr/bin/env node

import { examplePaths, renderedOutputs } from "./outputs.mjs";
import { fail } from "./shared.mjs";

import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname } from "node:path";

// Fixed commands. Validation lives in validate-tokens.mjs, platform renderers
// in render-*.mjs and output registration in outputs.mjs; this file only maps
// the CLI onto them.

async function generate() {
  for (const [path, contents] of await renderedOutputs()) {
    await mkdir(dirname(path), { recursive: true });
    await writeFile(path, contents, "utf8");
  }
  process.stdout.write("Generated TypeScript, Swift, and Kotlin design tokens.\n");
}

async function verify() {
  for (const [path, expected] of await renderedOutputs()) {
    const actual = await readFile(path, "utf8").catch(() => "");
    if (actual !== expected) fail(`${path} is stale; run npm run generate`);
  }
  for (const path of examplePaths) {
    const example = await readFile(path, "utf8");
    if (/#[0-9A-Fa-f]{3,8}\b/.test(example)) fail(`${path} contains a raw color literal`);
    if (/\b(?:padding|margin|gap|fontSize|font-size)\s*[:=]\s*\d/.test(example)) {
      fail(`${path} contains a raw spacing or typography literal`);
    }
  }
  process.stdout.write("Verified schema invariants, contrast, generated outputs, and examples.\n");
}

const command = process.argv[2];
if (command === "generate") await generate();
else if (command === "verify") await verify();
else fail("usage: node scripts/design-tokens.mjs <generate|verify>");
