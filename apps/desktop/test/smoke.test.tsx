import { cleanup, render, screen } from "@testing-library/react";
import { threadlineTokens } from "@threadline/design-tokens";
import { afterEach, expect, test } from "vitest";

afterEach(cleanup);

import { App } from "../src/App.tsx";
import { applyTokens, preferredScheme, tokenCustomProperties } from "../src/tokens.ts";

test("the app renders one labelled main landmark", async () => {
  render(<App />);
  expect(screen.getByRole("main", { name: "Threadline" })).toBeTruthy();
  expect(await screen.findByRole("heading", { level: 1 })).toBeTruthy();
});

test("token custom properties come from the shared design tokens, not literals", () => {
  const light = tokenCustomProperties("light");
  expect(light["--tl-color-canvas"]).toBe(threadlineTokens.color.light.canvas);
  expect(light["--tl-color-text-primary"]).toBe(threadlineTokens.color.light.textPrimary);
  expect(Object.keys(light).length).toBe(Object.keys(threadlineTokens.color.light).length);

  const element = document.createElement("div");
  applyTokens(element, "dark");
  expect(element.style.getPropertyValue("--tl-color-accent")).toBe(threadlineTokens.color.dark.accent);
  expect(element.dataset["tlScheme"]).toBe("dark");
  expect(element.style.getPropertyValue("--tl-space-md")).toBe(`${threadlineTokens.space.md}rem`);
  expect(element.style.getPropertyValue("--tl-font-body-size")).toBe(`${threadlineTokens.typography.body.webRem}rem`);
});

test("high contrast wins over dark when both are preferred", () => {
  expect(preferredScheme(() => false)).toBe("light");
  expect(preferredScheme((query) => query.includes("dark"))).toBe("dark");
  expect(preferredScheme((query) => query.includes("contrast"))).toBe("highContrastLight");
  expect(preferredScheme(() => true)).toBe("highContrastDark");
});
