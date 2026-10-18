import { cleanup, render, screen } from "@testing-library/react";
import { threadlineTokens } from "@threadline/design-tokens";
import axe from "axe-core";
import { afterEach, describe, expect, test } from "vitest";

import { contrastRatio } from "../src/contrast.ts";
import { FixtureRepository } from "../src/repository.ts";
import { Shell } from "../src/shell/Shell.tsx";

afterEach(cleanup);

async function renderShell() {
  const repository = new FixtureRepository({ autoCommit: false });
  const { container } = render(<Shell repository={repository} />);
  await screen.findAllByTestId("message");
  return { container, repository };
}

describe("accessibility baseline", () => {
  test("axe reports no WCAG 2.1 AA violations for the rendered shell", async () => {
    const { container } = await renderShell();
    const results = await axe.run(container, {
      runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"] },
      // jsdom has no layout, so color contrast is asserted from the tokens below.
      rules: { "color-contrast": { enabled: false } },
    });
    expect(results.violations.map((violation) => `${violation.id}: ${violation.nodes.map((node) => node.target.join(" ")).join(", ")}`)).toEqual([]);
  });

  test("landmarks are present, distinct and named", async () => {
    await renderShell();
    expect(screen.getAllByRole("main")).toHaveLength(1);
    expect(screen.getByRole("main", { name: "Threadline" })).toBeTruthy();
    expect(screen.getAllByRole("navigation")).toHaveLength(1);
    expect(screen.getByRole("navigation", { name: "频道导航" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "当前会话" })).toBeTruthy();
    expect(screen.getAllByRole("status").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByRole("textbox", { name: /发消息到 .* 频道/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: "发送" })).toBeTruthy();
  });

  test("focus order runs sidebar → timeline → composer in DOM order", async () => {
    const { container } = await renderShell();
    const focusable = Array.from(
      container.querySelectorAll<HTMLElement>("button:not([disabled]), textarea, [tabindex]:not([tabindex='-1'])"),
    );
    const firstChannel = focusable.findIndex((element) => element.classList.contains("tl-channel"));
    const composer = focusable.findIndex((element) => element.id === "tl-composer-input");
    expect(firstChannel).toBeGreaterThanOrEqual(0);
    expect(composer).toBeGreaterThan(firstChannel);
    const lastChannel = focusable.map((element) => element.classList.contains("tl-channel")).lastIndexOf(true);
    expect(lastChannel).toBeLessThan(composer);
  });

  test("removing an accessible name is caught", async () => {
    const { container } = await renderShell();
    screen.getByRole("navigation", { name: "频道导航" }).removeAttribute("aria-label");
    const results = await axe.run(container, {
      runOnly: { type: "rule", values: ["landmark-unique", "region"] },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(screen.queryByRole("navigation", { name: "频道导航" })).toBeNull();
    expect(results).toBeTruthy();
  });
});

describe("token contrast used by the shell", () => {
  const pairs: Array<[keyof typeof threadlineTokens.color.light, keyof typeof threadlineTokens.color.light, number]> = [
    ["textPrimary", "canvas", 4.5],
    ["textPrimary", "surface", 4.5],
    ["textSecondary", "surface", 4.5],
    ["textMuted", "surface", 4.5],
    ["onAccent", "accent", 4.5],
    ["onRail", "rail", 4.5],
    ["warning", "warningSubtle", 4.5],
    ["accent", "canvas", 3],
    ["focusRing", "canvas", 3],
  ];
  for (const scheme of ["light", "dark", "highContrastLight"] as const) {
    test(`${scheme} pairs meet their WCAG thresholds`, () => {
      const colors = threadlineTokens.color[scheme];
      const failures = pairs
        .map(([fg, bg, minimum]) => ({ fg, bg, minimum, ratio: contrastRatio(colors[fg], colors[bg]) }))
        .filter((pair) => pair.ratio < pair.minimum)
        .map((pair) => `${pair.fg} on ${pair.bg}: ${pair.ratio.toFixed(2)} < ${pair.minimum}`);
      expect(failures).toEqual([]);
    });
  }
});
