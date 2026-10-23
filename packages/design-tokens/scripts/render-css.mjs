/** Deterministic CSS adapter, separate from the declarative state policy. */
export function renderCssColors(tokens, digest) {
  const modes = Object.entries(tokens.color).map(([mode, colors]) => renderMode(mode, colors));
  return `/* Generated from src/tokens.json (${digest}). Do not edit. */\n${modes.join("\n")}\n`;
}

function renderMode(mode, colors) {
  const selector = mode === "light" ? ':root, [data-tl-scheme="light"]' : `[data-tl-scheme="${mode}"]`;
  const values = Object.entries(colors).map(([name, value]) => `  --tl-color-${kebab(name)}: ${value};`);
  return `${selector} {\n${values.join("\n")}\n}`;
}

function kebab(name) {
  return name.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`);
}
