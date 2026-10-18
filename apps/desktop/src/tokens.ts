import { threadlineTokens } from "@threadline/design-tokens";

type ColorScheme = keyof typeof threadlineTokens.color;

/**
 * The shell never writes literal colors or spacing; every value comes from the
 * shared design tokens and is published as CSS custom properties on `:root`.
 */
export function tokenCustomProperties(scheme: ColorScheme): Record<string, string> {
  const colors = threadlineTokens.color[scheme];
  const properties: Record<string, string> = {};
  for (const [name, value] of Object.entries(colors)) {
    properties[`--tl-color-${camelToKebab(name)}`] = value;
  }
  return properties;
}

/** Applies one scheme's properties to an element (normally `document.documentElement`). */
export function applyTokens(target: HTMLElement, scheme: ColorScheme): void {
  for (const [property, value] of Object.entries(tokenCustomProperties(scheme))) {
    target.style.setProperty(property, value);
  }
  target.dataset["tlScheme"] = scheme;
}

/** Picks the scheme from the platform preferences; high contrast wins over dark. */
export function preferredScheme(matches: (query: string) => boolean): ColorScheme {
  if (matches("(prefers-contrast: more)")) {
    return "highContrastLight";
  }
  return matches("(prefers-color-scheme: dark)") ? "dark" : "light";
}

function camelToKebab(value: string): string {
  return value.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`);
}
