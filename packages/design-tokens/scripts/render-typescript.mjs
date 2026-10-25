import { sourceHeader } from "./shared.mjs";

/** Desktop/Admin Web adapter: the validated source as a readonly constant. */
export function renderTypeScript(tokens, digest) {
  return `${sourceHeader("//", digest)}export const threadlineTokens = ${JSON.stringify(tokens, null, 2)} as const;\n\nexport type ThreadlineColorMode = keyof typeof threadlineTokens.color;\nexport type ThreadlineSemanticColor = keyof typeof threadlineTokens.color.light;\nexport type ThreadlineTypographyRole = keyof typeof threadlineTokens.typography;\n`;
}
