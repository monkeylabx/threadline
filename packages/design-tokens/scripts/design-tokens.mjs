#!/usr/bin/env node

import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const sourcePath = resolve(packageRoot, "src/tokens.json");
const schemaPath = resolve(packageRoot, "schema/design-tokens.schema.json");
const outputPaths = {
  typescript: resolve(packageRoot, "generated/typescript/tokens.ts"),
  swift: resolve(packageRoot, "generated/swift/ThreadlineTokens.swift"),
  kotlin: resolve(packageRoot, "generated/kotlin/ThreadlineTokens.kt"),
};
const examplePaths = [
  resolve(packageRoot, "examples/typescript.ts"),
  resolve(packageRoot, "examples/swift.swift"),
  resolve(packageRoot, "examples/kotlin.kt"),
];
const colorModes = ["light", "dark", "highContrastLight", "highContrastDark"];

function fail(message) {
  throw new Error(message);
}

function exactKeys(value, expected, path) {
  const actual = Object.keys(value).sort();
  const wanted = [...expected].sort();
  if (JSON.stringify(actual) !== JSON.stringify(wanted)) {
    fail(`${path} keys must be exactly ${wanted.join(", ")}; got ${actual.join(", ")}`);
  }
}

function assertNumber(value, path, minimum = 0) {
  if (typeof value !== "number" || !Number.isFinite(value) || value < minimum) {
    fail(`${path} must be a finite number >= ${minimum}`);
  }
}

function luminance(hex) {
  const channels = hex
    .slice(1)
    .match(/.{2}/g)
    .map((channel) => Number.parseInt(channel, 16) / 255)
    .map((channel) =>
      channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4,
    );
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}

function contrastRatio(foreground, background) {
  const foregroundLuminance = luminance(foreground);
  const backgroundLuminance = luminance(background);
  return (
    (Math.max(foregroundLuminance, backgroundLuminance) + 0.05) /
    (Math.min(foregroundLuminance, backgroundLuminance) + 0.05)
  );
}

function validateScale(scale, path, { monotonic = false, integer = false } = {}) {
  let previous = -Infinity;
  for (const [name, value] of Object.entries(scale)) {
    assertNumber(value, `${path}.${name}`);
    if (integer && !Number.isInteger(value)) fail(`${path}.${name} must be an integer`);
    if (monotonic && value < previous) fail(`${path} must be monotonically non-decreasing`);
    previous = value;
  }
}

function validateTokens(tokens) {
  exactKeys(
    tokens,
    ["schemaVersion", "color", "typography", "space", "radius", "layer", "motion", "accessibility"],
    "tokens",
  );
  if (tokens.schemaVersion !== 1) fail("tokens.schemaVersion must equal 1");
  exactKeys(tokens.color, colorModes, "tokens.color");

  const semanticNames = Object.keys(tokens.color.light).sort();
  if (semanticNames.length === 0) fail("tokens.color.light cannot be empty");
  for (const mode of colorModes) {
    exactKeys(tokens.color[mode], semanticNames, `tokens.color.${mode}`);
    for (const [name, value] of Object.entries(tokens.color[mode])) {
      if (!/^#[0-9A-F]{6}$/.test(value)) {
        fail(`tokens.color.${mode}.${name} must be an uppercase six-digit hex color`);
      }
    }
  }

  const swiftStyles = new Set([
    "largeTitle",
    "title1",
    "title2",
    "title3",
    "headline",
    "subheadline",
    "body",
    "callout",
    "footnote",
    "caption1",
    "caption2",
  ]);
  for (const [name, value] of Object.entries(tokens.typography)) {
    exactKeys(value, ["webRem", "lineHeight", "weight", "swiftTextStyle", "androidSp"], `typography.${name}`);
    assertNumber(value.webRem, `typography.${name}.webRem`, 0.75);
    assertNumber(value.lineHeight, `typography.${name}.lineHeight`, 1);
    assertNumber(value.androidSp, `typography.${name}.androidSp`, 12);
    if (!Number.isInteger(value.weight) || value.weight < 400 || value.weight > 800) {
      fail(`typography.${name}.weight must be an integer from 400 through 800`);
    }
    if (!swiftStyles.has(value.swiftTextStyle)) {
      fail(`typography.${name}.swiftTextStyle is not a supported Dynamic Type style`);
    }
  }
  if (Object.keys(tokens.typography).length === 0) fail("tokens.typography cannot be empty");

  validateScale(tokens.space, "tokens.space", { monotonic: true });
  validateScale(tokens.radius, "tokens.radius");
  validateScale(tokens.layer, "tokens.layer", { integer: true });
  validateScale(tokens.motion.durationMs, "tokens.motion.durationMs", { monotonic: true });
  exactKeys(tokens.motion, ["durationMs", "easing", "reducedMotion"], "tokens.motion");
  if (Object.keys(tokens.motion.easing).length === 0) fail("tokens.motion.easing cannot be empty");
  for (const [name, value] of Object.entries(tokens.motion.easing)) {
    if (typeof value !== "string" || value.length === 0) {
      fail(`tokens.motion.easing.${name} must be a non-empty string`);
    }
  }
  exactKeys(
    tokens.motion.reducedMotion,
    ["durationMultiplier", "distanceMultiplier", "allowOpacity", "essentialOnly"],
    "tokens.motion.reducedMotion",
  );
  if (
    tokens.motion.reducedMotion.durationMultiplier !== 0 ||
    tokens.motion.reducedMotion.distanceMultiplier !== 0 ||
    tokens.motion.reducedMotion.essentialOnly !== true
  ) {
    fail("reduced motion must remove duration and distance and permit essential motion only");
  }
  if (typeof tokens.motion.reducedMotion.allowOpacity !== "boolean") {
    fail("reducedMotion.allowOpacity must be a boolean");
  }
  exactKeys(
    tokens.accessibility,
    ["minimumContrast", "maximumTextScale", "contrastPairs"],
    "tokens.accessibility",
  );
  if (tokens.accessibility.minimumContrast < 4.5) fail("minimumContrast must be at least 4.5");
  if (tokens.accessibility.maximumTextScale < 2) fail("maximumTextScale must be at least 2");
  if (!Array.isArray(tokens.accessibility.contrastPairs) || tokens.accessibility.contrastPairs.length === 0) {
    fail("contrastPairs must be a non-empty array");
  }

  for (const pair of tokens.accessibility.contrastPairs) {
    exactKeys(pair, ["foreground", "background", "minimum"], "contrastPair");
    if (typeof pair.foreground !== "string" || typeof pair.background !== "string") {
      fail("contrastPair foreground and background must be strings");
    }
    assertNumber(pair.minimum, "contrastPair.minimum", 3);
    for (const mode of colorModes) {
      const foreground = tokens.color[mode][pair.foreground];
      const background = tokens.color[mode][pair.background];
      if (!foreground || !background) fail(`contrast pair references an unknown color in ${mode}`);
      const ratio = contrastRatio(foreground, background);
      if (ratio + Number.EPSILON < pair.minimum) {
        fail(
          `${mode}.${pair.foreground}/${pair.background} contrast ${ratio.toFixed(2)} is below ${pair.minimum}`,
        );
      }
    }
  }
}

function sourceHeader(comment, digest) {
  return `${comment} Generated from src/tokens.json (SHA-256: ${digest}). Do not edit.\n`;
}

function swiftString(value) {
  return JSON.stringify(value);
}

function swiftDictionary(value, valueRenderer = swiftString) {
  return `[${Object.entries(value)
    .map(([key, item]) => `${swiftString(key)}: ${valueRenderer(item)}`)
    .join(", ")}]`;
}

function kotlinString(value) {
  return JSON.stringify(value);
}

function kotlinMap(value, valueRenderer = kotlinString) {
  return `mapOf(${Object.entries(value)
    .map(([key, item]) => `${kotlinString(key)} to ${valueRenderer(item)}`)
    .join(", ")})`;
}

function kotlinDouble(value) {
  return Number.isInteger(value) ? `${value}.0` : String(value);
}

function nativeIdentifier(value) {
  const words = value.match(/[A-Za-z]+|\d+/g) ?? [];
  const identifier = words
    .map((word, index) => {
      const normalized = /^\d/.test(word) ? `_${word}` : word;
      return index === 0 ? normalized : `${normalized[0].toUpperCase()}${normalized.slice(1)}`;
    })
    .join("");
  return identifier || "token";
}

function renderTypeScript(tokens, digest) {
  return `${sourceHeader("//", digest)}export const threadlineTokens = ${JSON.stringify(tokens, null, 2)} as const;\n\nexport type ThreadlineColorMode = keyof typeof threadlineTokens.color;\nexport type ThreadlineSemanticColor = keyof typeof threadlineTokens.color.light;\nexport type ThreadlineTypographyRole = keyof typeof threadlineTokens.typography;\n`;
}

function renderSwift(tokens, digest) {
  const typography = swiftDictionary(
    tokens.typography,
    (value) =>
      `TypographyToken(webRem: ${value.webRem}, lineHeight: ${value.lineHeight}, weight: ${value.weight}, swiftTextStyle: ${swiftString(value.swiftTextStyle)}, androidSp: ${value.androidSp})`,
  );
  const semanticNames = Object.keys(tokens.color.light);
  const colorModeFields = semanticNames.map((name) => `    public let ${nativeIdentifier(name)}: String`).join("\n");
  const modes = colorModes
    .map(
      (mode) =>
        `    public static let ${mode} = ColorMode(${semanticNames
          .map((name) => `${nativeIdentifier(name)}: ${swiftString(tokens.color[mode][name])}`)
          .join(", ")})`,
    )
    .join("\n");
  const swiftNamespace = (name, values, renderer = String) => `  public enum ${name} {\n${Object.entries(values)
    .map(([key, value]) => `    public static let ${nativeIdentifier(key)} = ${renderer(value)}`)
    .join("\n")}\n  }`;
  return `${sourceHeader("//", digest)}public enum ThreadlineTokens {
  public struct TypographyToken: Sendable {
    public let webRem: Double
    public let lineHeight: Double
    public let weight: Int
    public let swiftTextStyle: String
    public let androidSp: Double
  }

  public struct ReducedMotionPolicy: Sendable {
    public let durationMultiplier: Double
    public let distanceMultiplier: Double
    public let allowOpacity: Bool
    public let essentialOnly: Bool
  }

  public struct ColorMode: Sendable {
${colorModeFields}
  }

  public enum Color {
${modes}
  }

  public static let typography: [String: TypographyToken] = ${typography}
${swiftNamespace(
  "Typography",
  tokens.typography,
  (value) =>
    `TypographyToken(webRem: ${value.webRem}, lineHeight: ${value.lineHeight}, weight: ${value.weight}, swiftTextStyle: ${swiftString(value.swiftTextStyle)}, androidSp: ${value.androidSp})`,
)}
${swiftNamespace("Space", tokens.space)}
${swiftNamespace("Radius", tokens.radius)}
${swiftNamespace("Layer", tokens.layer)}
${swiftNamespace("MotionDurationMs", tokens.motion.durationMs)}
  public static let space: [String: Double] = ${swiftDictionary(tokens.space, String)}
  public static let radius: [String: Double] = ${swiftDictionary(tokens.radius, String)}
  public static let layer: [String: Int] = ${swiftDictionary(tokens.layer, String)}
  public static let motionDurationMs: [String: Int] = ${swiftDictionary(tokens.motion.durationMs, String)}
  public static let motionEasing: [String: String] = ${swiftDictionary(tokens.motion.easing)}
  public static let reducedMotion = ReducedMotionPolicy(
    durationMultiplier: ${tokens.motion.reducedMotion.durationMultiplier},
    distanceMultiplier: ${tokens.motion.reducedMotion.distanceMultiplier},
    allowOpacity: ${tokens.motion.reducedMotion.allowOpacity},
    essentialOnly: ${tokens.motion.reducedMotion.essentialOnly}
  )
  public static let minimumContrast = ${tokens.accessibility.minimumContrast}
  public static let maximumTextScale = ${tokens.accessibility.maximumTextScale}
}
`;
}

function renderKotlin(tokens, digest) {
  const typography = kotlinMap(
    tokens.typography,
    (value) =>
      `TypographyToken(${kotlinDouble(value.webRem)}, ${kotlinDouble(value.lineHeight)}, ${value.weight}, ${kotlinString(value.swiftTextStyle)}, ${kotlinDouble(value.androidSp)})`,
  );
  const semanticNames = Object.keys(tokens.color.light);
  const colorModeFields = semanticNames
    .map((name) => `        val ${nativeIdentifier(name)}: String`)
    .join(",\n");
  const modes = colorModes
    .map(
      (mode) =>
        `        val ${mode} = ColorMode(${semanticNames
          .map((name) => `${nativeIdentifier(name)} = ${kotlinString(tokens.color[mode][name])}`)
          .join(", ")})`,
    )
    .join("\n");
  const kotlinNamespace = (name, values, renderer = String) => `    object ${name} {\n${Object.entries(values)
    .map(([key, value]) => `        const val ${nativeIdentifier(key)} = ${renderer(value)}`)
    .join("\n")}\n    }`;
  return `${sourceHeader("//", digest)}object ThreadlineTokens {
    data class TypographyToken(
        val webRem: Double,
        val lineHeight: Double,
        val weight: Int,
        val swiftTextStyle: String,
        val androidSp: Double,
    )

    data class ReducedMotionPolicy(
        val durationMultiplier: Double,
        val distanceMultiplier: Double,
        val allowOpacity: Boolean,
        val essentialOnly: Boolean,
    )

    data class ColorMode(
${colorModeFields},
    )

    object Color {
${modes}
    }

    val typography: Map<String, TypographyToken> = ${typography}
    object Typography {
${Object.entries(tokens.typography)
  .map(
    ([key, value]) =>
      `        val ${nativeIdentifier(key)} = TypographyToken(${kotlinDouble(value.webRem)}, ${kotlinDouble(value.lineHeight)}, ${value.weight}, ${kotlinString(value.swiftTextStyle)}, ${kotlinDouble(value.androidSp)})`,
  )
  .join("\n")}
    }
${kotlinNamespace("Space", tokens.space, kotlinDouble)}
${kotlinNamespace("Radius", tokens.radius, kotlinDouble)}
${kotlinNamespace("Layer", tokens.layer)}
${kotlinNamespace("MotionDurationMs", tokens.motion.durationMs)}
    val space: Map<String, Double> = ${kotlinMap(tokens.space, kotlinDouble)}
    val radius: Map<String, Double> = ${kotlinMap(tokens.radius, kotlinDouble)}
    val layer: Map<String, Int> = ${kotlinMap(tokens.layer, String)}
    val motionDurationMs: Map<String, Int> = ${kotlinMap(tokens.motion.durationMs, String)}
    val motionEasing: Map<String, String> = ${kotlinMap(tokens.motion.easing)}
    val reducedMotion = ReducedMotionPolicy(
        durationMultiplier = ${tokens.motion.reducedMotion.durationMultiplier}.0,
        distanceMultiplier = ${tokens.motion.reducedMotion.distanceMultiplier}.0,
        allowOpacity = ${tokens.motion.reducedMotion.allowOpacity},
        essentialOnly = ${tokens.motion.reducedMotion.essentialOnly},
    )
    const val minimumContrast: Double = ${tokens.accessibility.minimumContrast}
    const val maximumTextScale: Double = ${tokens.accessibility.maximumTextScale}.0
}
`;
}

async function loadInputs() {
  const [sourceBytes, schemaBytes] = await Promise.all([readFile(sourcePath), readFile(schemaPath)]);
  const tokens = JSON.parse(sourceBytes.toString("utf8"));
  JSON.parse(schemaBytes.toString("utf8"));
  validateTokens(tokens);
  const digest = createHash("sha256").update(sourceBytes).digest("hex");
  return { tokens, digest };
}

async function renderedOutputs() {
  const { tokens, digest } = await loadInputs();
  return new Map([
    [outputPaths.typescript, renderTypeScript(tokens, digest)],
    [outputPaths.swift, renderSwift(tokens, digest)],
    [outputPaths.kotlin, renderKotlin(tokens, digest)],
  ]);
}

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
