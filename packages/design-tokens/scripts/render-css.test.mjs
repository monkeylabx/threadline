import assert from "node:assert/strict";
import test from "node:test";
import { renderCssTokens } from "./render-css.mjs";

test("CSS publishes explicit theme roles deterministically without mutation", () => {
  const tokens = { color: { light: { onAccent: "#FFFFFF" }, dark: { onAccent: "#000000" } }, space: {}, radius: {}, typography: {} };
  const before = structuredClone(tokens);
  const result = renderCssTokens(tokens, "fixture");
  assert.equal(result, '/* Generated from src/tokens.json (fixture). Do not edit. */\n:root, [data-tl-scheme="light"] {\n  --tl-color-on-accent: #FFFFFF;\n}\n[data-tl-scheme="dark"] {\n  --tl-color-on-accent: #000000;\n}\n:root {\n\n}\n');
  assert.equal(renderCssTokens(tokens, "fixture"), result);
  assert.deepEqual(tokens, before);
});


test("CSS exposes scalable composer roles with portable names and units", () => {
  const tokens = {
    color: { light: { textPrimary: "#000000" } },
    space: { composerOuterInset: 1.25, full: 2 },
    radius: { composerSend: 0.5625, full: 9999 },
    typography: { caption: { webRem: 0.75, lineHeight: 1.4, weight: 500 } },
  };
  const before = structuredClone(tokens);
  const result = renderCssTokens(tokens, "composer");
  for (const declaration of [
    "--tl-space-composer-outer-inset: 1.25rem;",
    "--tl-space-full: 2rem;",
    "--tl-radius-composer-send: 0.5625rem;",
    "--tl-radius-full: 9999px;",
    "--tl-font-caption-size: 0.75rem;",
    "--tl-font-caption-line: 1.4;",
    "--tl-font-caption-weight: 500;",
  ]) assert.ok(result.includes(declaration), `missing ${declaration}`);
  assert.equal(renderCssTokens(tokens, "composer"), result);
  assert.deepEqual(tokens, before);
});
