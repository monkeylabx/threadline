/** Deterministic CSS adapter for the shared visual token contract. */
export function renderCssTokens(tokens, digest) {
  const modes = Object.entries(tokens.color).map(([mode, colors]) => renderMode(mode, colors));
  return `/* Generated from src/tokens.json (${digest}). Do not edit. */\n${modes.join("\n")}\n${renderScale(tokens)}\n`;
}

function renderScale(tokens) {
  const typography = Object.entries(tokens.typography).flatMap(([name, style]) => [
    `  --tl-font-${kebab(name)}-size: ${style.webRem}rem;`,
    `  --tl-font-${kebab(name)}-line: ${style.lineHeight};`,
    `  --tl-font-${kebab(name)}-weight: ${style.weight};`,
  ]);
  const values = [...dimensions(tokens.space, "space"), ...dimensions(tokens.radius, "radius"), ...typography];
  return `:root {\n${values.join("\n")}\n}`;
}

function dimensions(scale, family) {
  return Object.entries(scale).map(([name, value]) => {
    const unit = family === "radius" && name === "full" ? "px" : "rem";
    return `  --tl-${family}-${kebab(name)}: ${value}${unit};`;
  });
}

function renderMode(mode, colors) {
  const selector = mode === "light" ? ':root, [data-tl-scheme="light"]' : `[data-tl-scheme="${mode}"]`;
  const values = Object.entries(colors).map(([name, value]) => `  --tl-color-${kebab(name)}: ${value};`);
  return `${selector} {\n${values.join("\n")}\n}`;
}

function kebab(name) {
  return name.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`);
}
