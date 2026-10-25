import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { validateTokens } from "./validate-tokens.mjs";

const source = JSON.parse(readFileSync(new URL("../src/tokens.json", import.meta.url), "utf8"));
const valid = () => structuredClone(source);

test("the committed source passes every invariant and is not mutated", () => {
  const tokens = valid();
  validateTokens(tokens);
  assert.deepEqual(tokens, source);
});

test("a top-level key that is unknown or missing is rejected by name", () => {
  const extra = valid();
  extra.unexpected = 1;
  assert.throws(() => validateTokens(extra), /tokens keys must be exactly/);
  const missing = valid();
  delete missing.layer;
  assert.throws(() => validateTokens(missing), /tokens keys must be exactly/);
});

test("every color mode must declare the same semantic names as uppercase six-digit hex", () => {
  const lowercase = valid();
  lowercase.color.dark.accent = "#aabbcc";
  assert.throws(() => validateTokens(lowercase), /tokens\.color\.dark\.accent must be an uppercase six-digit hex color/);
  const drift = valid();
  delete drift.color.highContrastDark.accent;
  assert.throws(() => validateTokens(drift), /tokens\.color\.highContrastDark keys must be exactly/);
});

test("typography rejects weights and Dynamic Type styles outside the contract", () => {
  const [role] = Object.keys(source.typography);
  const heavy = valid();
  heavy.typography[role].weight = 900;
  assert.throws(() => validateTokens(heavy), new RegExp(`typography\\.${role}\\.weight must be an integer from 400 through 800`));
  const style = valid();
  style.typography[role].swiftTextStyle = "display";
  assert.throws(() => validateTokens(style), /is not a supported Dynamic Type style/);
});

test("scales enforce monotonic space, integer layers and non-negative numbers", () => {
  const space = valid();
  const spaceNames = Object.keys(space.space);
  space.space[spaceNames.at(-1)] = -1;
  assert.throws(() => validateTokens(space), /tokens\.space\..* must be a finite number >= 0/);
  const order = valid();
  order.space[spaceNames.at(-1)] = 0;
  assert.throws(() => validateTokens(order), /tokens\.space must be monotonically non-decreasing/);
  const layer = valid();
  layer.layer[Object.keys(layer.layer)[0]] = 1.5;
  assert.throws(() => validateTokens(layer), /tokens\.layer\..* must be an integer/);
});

test("reduced motion must zero duration and distance and keep essential motion only", () => {
  const motion = valid();
  motion.motion.reducedMotion.durationMultiplier = 0.5;
  assert.throws(() => validateTokens(motion), /reduced motion must remove duration and distance/);
  const opacity = valid();
  opacity.motion.reducedMotion.allowOpacity = "yes";
  assert.throws(() => validateTokens(opacity), /allowOpacity must be a boolean/);
});

test("a contrast pair below its minimum in any mode fails with the measured ratio", () => {
  const tokens = valid();
  const [pair] = tokens.accessibility.contrastPairs;
  tokens.color.dark[pair.foreground] = tokens.color.dark[pair.background];
  assert.throws(() => validateTokens(tokens), new RegExp(`dark\\.${pair.foreground}/${pair.background} contrast 1\\.00 is below ${pair.minimum}`));
  const unknown = valid();
  unknown.accessibility.contrastPairs.push({ foreground: "missingRole", background: pair.background, minimum: 4.5 });
  assert.throws(() => validateTokens(unknown), /contrast pair references an unknown color in light/);
});

test("accessibility floors cannot drop below the contract", () => {
  const contrast = valid();
  contrast.accessibility.minimumContrast = 3;
  assert.throws(() => validateTokens(contrast), /minimumContrast must be at least 4\.5/);
  const scale = valid();
  scale.accessibility.maximumTextScale = 1.5;
  assert.throws(() => validateTokens(scale), /maximumTextScale must be at least 2/);
});
