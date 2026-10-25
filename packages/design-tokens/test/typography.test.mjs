import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const { typography } = JSON.parse(readFileSync(new URL("../src/tokens.json", import.meta.url), "utf8"));

test("bodyCompact carries V34 13px message and company text on every platform", () => {
  assert.deepEqual(typography.bodyCompact, {
    webRem: 0.8125, lineHeight: 1.7, weight: 400, swiftTextStyle: "footnote", androidSp: 13,
  });
});

test("no role goes below the 12px / 12sp accessibility floor", () => {
  const belowFloor = Object.entries(typography)
    .filter(([, role]) => role.webRem < 0.75 || role.androidSp < 12)
    .map(([name]) => name);
  assert.deepEqual(belowFloor, []);
});
