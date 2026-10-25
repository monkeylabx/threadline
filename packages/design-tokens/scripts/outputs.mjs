import { renderCssColors } from "./render-css.mjs";
import { renderKotlin } from "./render-kotlin.mjs";
import { renderSwift } from "./render-swift.mjs";
import { renderTypeScript } from "./render-typescript.mjs";
import { validateTokens } from "./validate-tokens.mjs";

import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

/**
 * Output registration: which inputs feed which generated files. Every entry
 * is rendered from the validated source so `generate` and `verify` always see
 * the same bytes.
 */

export const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const sourcePath = resolve(packageRoot, "src/tokens.json");
const policyPath = resolve(packageRoot, "src/interaction.css");
const prototypeRoot = resolve(packageRoot, "../../docs/prototype/generated");
const schemaPath = resolve(packageRoot, "schema/design-tokens.schema.json");
const outputPaths = {
  typescript: resolve(packageRoot, "generated/typescript/tokens.ts"),
  swift: resolve(packageRoot, "generated/swift/ThreadlineTokens.swift"),
  kotlin: resolve(packageRoot, "generated/kotlin/ThreadlineTokens.kt"),
};

/** Handwritten consumers that must bind tokens instead of raw literals. */
export const examplePaths = [
  policyPath,
  resolve(packageRoot, "examples/typescript.ts"),
  resolve(packageRoot, "examples/swift.swift"),
  resolve(packageRoot, "examples/kotlin.kt"),
];

async function loadInputs() {
  const [sourceBytes, schemaBytes] = await Promise.all([readFile(sourcePath), readFile(schemaPath)]);
  const tokens = JSON.parse(sourceBytes.toString("utf8"));
  JSON.parse(schemaBytes.toString("utf8"));
  validateTokens(tokens);
  const digest = createHash("sha256").update(sourceBytes).digest("hex");
  return { tokens, digest };
}

export async function renderedOutputs() {
  const { tokens, digest } = await loadInputs();
  const policy = await readFile(policyPath, "utf8");
  const css = renderCssColors(tokens, digest);
  return new Map([
    [resolve(packageRoot, "generated/css/colors.css"), css],
    [resolve(prototypeRoot, "colors.css"), css],
    [resolve(prototypeRoot, "interaction.css"), `/* Generated from packages/design-tokens/src/interaction.css. Do not edit. */\n${policy}`],
    [resolve(prototypeRoot, "interaction-states.html"), `<!-- Generated QA fixture. Do not edit. -->\n${await readFile(resolve(packageRoot, "test/interaction-states.html"), "utf8")}`],
    [outputPaths.typescript, renderTypeScript(tokens, digest)],
    [outputPaths.swift, renderSwift(tokens, digest)],
    [outputPaths.kotlin, renderKotlin(tokens, digest)],
  ]);
}
