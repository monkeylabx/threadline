import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const tokens = JSON.parse(readFileSync(new URL("../src/tokens.json", import.meta.url), "utf8"));

// Roles that carry V34 emphasis or interaction state; every mode keeps them blue.
const emphasisRoles = [
  "accent", "accentHover", "accentSubtle", "selection", "actionPressed",
  "navigationSurface", "navigationAccent", "navigationControlSurface",
  "controlHoverSurface", "controlPressedSurface", "controlSelectedSurface",
  "controlSelectedHoverSurface", "controlSelectedPressedSurface", "controlSelectedText",
];
const identityRoles = ["identitySlate", "identityPlum", "identityClay", "identityAmber"];

function hueAndSaturation(hex) {
  const [r, g, b] = [1, 3, 5].map((offset) => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255);
  const max = Math.max(r, g, b);
  const delta = max - Math.min(r, g, b);
  const lightness = (max + Math.min(r, g, b)) / 2;
  const saturation = delta === 0 ? 0 : delta / (1 - Math.abs(2 * lightness - 1));
  if (delta === 0) return { hue: 0, saturation };
  const sector = max === r ? ((g - b) / delta) % 6 : max === g ? (b - r) / delta + 2 : (r - g) / delta + 4;
  return { hue: (sector * 60 + 360) % 360, saturation };
}

for (const [mode, colors] of Object.entries(tokens.color)) {
  test(`${mode} emphasis roles stay in the V34 blue family`, () => {
    const offHue = emphasisRoles
      .map((role) => ({ role, value: colors[role], ...hueAndSaturation(colors[role]) }))
      .filter(({ hue, saturation }) => saturation > 0.08 && (hue < 200 || hue > 235))
      .map(({ role, value, hue }) => `${role} ${value} hue ${Math.round(hue)}`);
    assert.deepEqual(offHue, []);
  });

  test(`${mode} identity avatars are distinct and share one foreground`, () => {
    const values = identityRoles.map((role) => colors[role]);
    assert.equal(values.every((value) => /^#[0-9A-F]{6}$/.test(value)), true);
    assert.equal(new Set(values).size, identityRoles.length);
    for (const role of identityRoles) {
      assert.ok(
        tokens.accessibility.contrastPairs.some((pair) => pair.foreground === "onIdentity" && pair.background === role),
        `missing onIdentity contrast pair for ${role}`,
      );
    }
  });
}

test("the hue helper rejects the pre-V34 green accent", () => {
  const { hue, saturation } = hueAndSaturation("#6FC6AB");
  assert.ok(saturation > 0.08 && (hue < 200 || hue > 235));
  assert.deepEqual(hueAndSaturation("#FFFFFF"), { hue: 0, saturation: 0 });
});
