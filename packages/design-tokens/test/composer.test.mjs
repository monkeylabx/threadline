import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const { space, typography } = JSON.parse(readFileSync(new URL("../src/tokens.json", import.meta.url), "utf8"));
const line = typography.caption.webRem * typography.caption.lineHeight;
const field = (lines) => 2 * space.composerInputInset + lines * line;

test("composer field rests at one caption line without scrolling", () => {
  assert.ok(space.composerFieldMin >= field(1), "one line must fit inside the resting field");
  assert.ok(space.composerFieldMin < field(2), "the resting field must not reserve a second blank line");
});

test("composer field grows to eight full caption lines before scrolling", () => {
  assert.ok(space.composerFieldMax >= field(8), "eight full lines must be visible at the cap");
  assert.ok(space.composerFieldMax < field(9), "a ninth line must scroll");
});

test("composer resting and largest groups keep the documented totals", () => {
  const toolbar = space.composerControl + space.sm;
  assert.equal((space.composerFieldMin + space.composerFieldGap + toolbar) * 16, 78);
  assert.equal((space.composerFieldMax + space.composerFieldGap + toolbar) * 16, 197);
});
