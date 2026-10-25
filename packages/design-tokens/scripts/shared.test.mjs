import assert from "node:assert/strict";
import test from "node:test";
import { contrastRatio, luminance } from "./contrast.mjs";
import { kotlinDouble } from "./render-kotlin.mjs";
import { nativeIdentifier, sourceHeader } from "./shared.mjs";

test("native identifiers are camelCase, digit-safe and never empty", () => {
  assert.equal(nativeIdentifier("textPrimary"), "textPrimary");
  assert.equal(nativeIdentifier("on-accent"), "onAccent");
  assert.equal(nativeIdentifier("2xl"), "_2Xl");
  assert.equal(nativeIdentifier("space 2 xl"), "space_2Xl");
  assert.equal(nativeIdentifier("---"), "token");
});

test("the source header names the digest and the comment syntax it was asked for", () => {
  assert.equal(sourceHeader("//", "abc"), "// Generated from src/tokens.json (SHA-256: abc). Do not edit.\n");
});

test("Kotlin doubles always carry a fractional part", () => {
  assert.equal(kotlinDouble(4), "4.0");
  assert.equal(kotlinDouble(1.25), "1.25");
  assert.equal(kotlinDouble(0), "0.0");
});

test("contrast follows WCAG relative luminance", () => {
  assert.equal(luminance("#000000"), 0);
  assert.equal(luminance("#FFFFFF"), 1);
  assert.equal(contrastRatio("#000000", "#FFFFFF"), 21);
  assert.equal(contrastRatio("#FFFFFF", "#000000"), 21);
  assert.equal(contrastRatio("#777777", "#777777"), 1);
});
