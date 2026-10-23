import assert from "node:assert/strict";
import test from "node:test";
import { renderCssColors } from "./render-css.mjs";

test("CSS publishes explicit theme roles deterministically without mutation", () => {
  const tokens = { color: { light: { onAccent: "#FFFFFF" }, dark: { onAccent: "#000000" } } };
  const before = structuredClone(tokens);
  const result = renderCssColors(tokens, "fixture");
  assert.equal(result, '/* Generated from src/tokens.json (fixture). Do not edit. */\n:root, [data-tl-scheme="light"] {\n  --tl-color-on-accent: #FFFFFF;\n}\n[data-tl-scheme="dark"] {\n  --tl-color-on-accent: #000000;\n}\n');
  assert.equal(renderCssColors(tokens, "fixture"), result);
  assert.deepEqual(tokens, before);
});
