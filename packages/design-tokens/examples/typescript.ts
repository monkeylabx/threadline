import { threadlineTokens } from "../generated/typescript/tokens.js";

export const messageComposerStyle = {
  color: threadlineTokens.color.light.textPrimary,
  backgroundColor: threadlineTokens.color.light.surfaceElevated,
  borderColor: threadlineTokens.color.light.border,
  borderRadius: `${threadlineTokens.radius.lg}rem`,
  gap: `${threadlineTokens.space.md}rem`,
  fontSize: `${threadlineTokens.typography.body.webRem}rem`,
  lineHeight: threadlineTokens.typography.body.lineHeight,
};
