import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const policy = readFileSync(new URL("../src/interaction.css", import.meta.url), "utf8");
const tokens = JSON.parse(readFileSync(new URL("../src/tokens.json", import.meta.url), "utf8"));

// Each policy rule applies to one trait. Every selector is :where-wrapped, so all
// rules share zero specificity and source order alone decides the cascade.
const traitMatchers = [
  ["button:hover", (s) => s.hover],
  ["button:active", (s) => s.active],
  [":disabled", (s) => s.disabled],
  [".is-current", (s) => s.selected],
  ['[data-tl-control="tinted"]', (s) => s.variant === "tinted"],
  ['[data-tl-control="primary"]', (s) => s.variant === "primary"],
  ['[data-tl-control="danger"]', (s) => s.variant === "danger"],
  ['button, [role="button"]', () => true],
];

function parseRules(css) {
  const rules = [];
  for (const [, selector, body] of css.replace(/\/\*[\s\S]*?\*\//g, "").matchAll(/([^{}]+)\{([^}]*)\}/g)) {
    const declarations = Object.fromEntries(
      body.split(";").map((item) => item.split(":").map((part) => part.trim())).filter(([name]) => name),
    );
    const matcher = traitMatchers.find(([marker]) => selector.includes(marker));
    if (!matcher) throw new Error(`unclassified policy selector: ${selector.trim()}`);
    rules.push({ selector: selector.trim(), applies: matcher[1], declarations });
  }
  return rules;
}

function resolve(value, properties) {
  const reference = /^var\((--[\w-]+)\)$/.exec(value);
  if (!reference) return value;
  const name = reference[1];
  if (name.startsWith("--tl-color-")) return name.slice(11).replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
  return resolve(properties[name], properties);
}

function controlColors(scenario) {
  const properties = {};
  for (const rule of parseRules(policy)) if (rule.applies(scenario)) Object.assign(properties, rule.declarations);
  return [resolve(properties["--tl-control-background"], properties), resolve(properties["--tl-control-foreground"], properties)];
}

const rest = { variant: "neutral", hover: false, active: false, selected: false, disabled: false };
const cases = [
  ["neutral rest", {}, ["transparent", "textSecondary"]],
  ["neutral hover", { hover: true }, ["controlHoverSurface", "textPrimary"]],
  ["neutral selected", { selected: true }, ["controlSelectedSurface", "controlSelectedText"]],
  ["tinted rest", { variant: "tinted" }, ["navigationControlSurface", "textSecondary"]],
  ["primary rest", { variant: "primary" }, ["accent", "onAccent"]],
  ["neutral disabled", { disabled: true }, ["transparent", "textMuted"]],
  ["neutral disabled under pointer", { disabled: true, hover: true, active: true }, ["transparent", "textMuted"]],
  ["neutral disabled selected", { disabled: true, selected: true }, ["transparent", "textMuted"]],
  ["tinted disabled under pointer", { variant: "tinted", disabled: true, hover: true }, ["navigationControlSurface", "textMuted"]],
  ["danger disabled", { variant: "danger", disabled: true }, ["transparent", "textMuted"]],
  ["primary disabled pressed", { variant: "primary", disabled: true, active: true }, ["controlDisabledSurface", "textMuted"]],
];

for (const [name, traits, expected] of cases) {
  test(`${name} resolves to ${expected.join(" / ")}`, () => {
    assert.deepEqual(controlColors({ ...rest, ...traits }), expected);
  });
}

test("every policy selector has zero specificity", () => {
  for (const { selector } of parseRules(policy)) {
    assert.match(selector, /^:where\(\[data-tl-interaction-scope\]\) :where\([^]*\)$/);
  }
});

function contrast(foreground, background) {
  const luminance = (hex) => [1, 3, 5]
    .map((offset) => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255)
    .map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
    .reduce((sum, c, index) => sum + c * [0.2126, 0.7152, 0.0722][index], 0);
  const [high, low] = [luminance(foreground), luminance(background)].sort((a, b) => b - a);
  return (high + 0.05) / (low + 0.05);
}

// Inactive controls are exempt from WCAG 1.4.3/1.4.11; 3:1 keeps disabled icons perceivable.
for (const [mode, colors] of Object.entries(tokens.color)) {
  test(`${mode} disabled foregrounds stay perceivable on every resting surface`, () => {
    const failures = [];
    for (const variant of ["neutral", "tinted", "danger", "primary"]) {
      const [background, foreground] = controlColors({ ...rest, variant, disabled: true });
      const surfaces = background === "transparent" ? ["canvas", "surface", "navigationSurface"] : [background];
      for (const surface of surfaces) {
        const ratio = contrast(colors[foreground], colors[surface]);
        if (ratio < 3) failures.push(`${variant} ${foreground} on ${surface}: ${ratio.toFixed(2)}`);
      }
    }
    assert.deepEqual(failures, []);
  });
}
