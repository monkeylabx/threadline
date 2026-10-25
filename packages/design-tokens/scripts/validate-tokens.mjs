import { contrastRatio } from "./contrast.mjs";
import { colorModes, fail } from "./shared.mjs";

/**
 * Schema invariants for src/tokens.json. Checks run in a fixed order and the
 * first violation throws, so an invalid source never reaches a renderer.
 */

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

function validateScale(scale, path, { monotonic = false, integer = false } = {}) {
  let previous = -Infinity;
  for (const [name, value] of Object.entries(scale)) {
    assertNumber(value, `${path}.${name}`);
    if (integer && !Number.isInteger(value)) fail(`${path}.${name} must be an integer`);
    if (monotonic && value < previous) fail(`${path} must be monotonically non-decreasing`);
    previous = value;
  }
}

function validateColors(color) {
  exactKeys(color, colorModes, "tokens.color");
  const semanticNames = Object.keys(color.light).sort();
  if (semanticNames.length === 0) fail("tokens.color.light cannot be empty");
  for (const mode of colorModes) {
    exactKeys(color[mode], semanticNames, `tokens.color.${mode}`);
    for (const [name, value] of Object.entries(color[mode])) {
      if (!/^#[0-9A-F]{6}$/.test(value)) {
        fail(`tokens.color.${mode}.${name} must be an uppercase six-digit hex color`);
      }
    }
  }
}

function validateTypography(typography) {
  for (const [name, value] of Object.entries(typography)) {
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
  if (Object.keys(typography).length === 0) fail("tokens.typography cannot be empty");
}

function validateMotion(motion) {
  validateScale(motion.durationMs, "tokens.motion.durationMs", { monotonic: true });
  exactKeys(motion, ["durationMs", "easing", "reducedMotion"], "tokens.motion");
  if (Object.keys(motion.easing).length === 0) fail("tokens.motion.easing cannot be empty");
  for (const [name, value] of Object.entries(motion.easing)) {
    if (typeof value !== "string" || value.length === 0) {
      fail(`tokens.motion.easing.${name} must be a non-empty string`);
    }
  }
  exactKeys(
    motion.reducedMotion,
    ["durationMultiplier", "distanceMultiplier", "allowOpacity", "essentialOnly"],
    "tokens.motion.reducedMotion",
  );
  if (
    motion.reducedMotion.durationMultiplier !== 0 ||
    motion.reducedMotion.distanceMultiplier !== 0 ||
    motion.reducedMotion.essentialOnly !== true
  ) {
    fail("reduced motion must remove duration and distance and permit essential motion only");
  }
  if (typeof motion.reducedMotion.allowOpacity !== "boolean") {
    fail("reducedMotion.allowOpacity must be a boolean");
  }
}

function validateContrastPair(pair, color) {
  exactKeys(pair, ["foreground", "background", "minimum"], "contrastPair");
  if (typeof pair.foreground !== "string" || typeof pair.background !== "string") {
    fail("contrastPair foreground and background must be strings");
  }
  assertNumber(pair.minimum, "contrastPair.minimum", 3);
  for (const mode of colorModes) {
    const foreground = color[mode][pair.foreground];
    const background = color[mode][pair.background];
    if (!foreground || !background) fail(`contrast pair references an unknown color in ${mode}`);
    const ratio = contrastRatio(foreground, background);
    if (ratio + Number.EPSILON < pair.minimum) {
      fail(
        `${mode}.${pair.foreground}/${pair.background} contrast ${ratio.toFixed(2)} is below ${pair.minimum}`,
      );
    }
  }
}

function validateAccessibility(accessibility, color) {
  exactKeys(
    accessibility,
    ["minimumContrast", "maximumTextScale", "contrastPairs"],
    "tokens.accessibility",
  );
  if (accessibility.minimumContrast < 4.5) fail("minimumContrast must be at least 4.5");
  if (accessibility.maximumTextScale < 2) fail("maximumTextScale must be at least 2");
  if (!Array.isArray(accessibility.contrastPairs) || accessibility.contrastPairs.length === 0) {
    fail("contrastPairs must be a non-empty array");
  }
  for (const pair of accessibility.contrastPairs) validateContrastPair(pair, color);
}

export function validateTokens(tokens) {
  exactKeys(
    tokens,
    ["schemaVersion", "color", "typography", "space", "radius", "layer", "motion", "accessibility"],
    "tokens",
  );
  if (tokens.schemaVersion !== 1) fail("tokens.schemaVersion must equal 1");
  validateColors(tokens.color);
  validateTypography(tokens.typography);
  validateScale(tokens.space, "tokens.space", { monotonic: true });
  validateScale(tokens.radius, "tokens.radius");
  validateScale(tokens.layer, "tokens.layer", { integer: true });
  validateMotion(tokens.motion);
  validateAccessibility(tokens.accessibility, tokens.color);
}
