/** Vocabulary and pure helpers shared by validation and every renderer. */

export const colorModes = ["light", "dark", "highContrastLight", "highContrastDark"];

export function fail(message) {
  throw new Error(message);
}

export function sourceHeader(comment, digest) {
  return `${comment} Generated from src/tokens.json (SHA-256: ${digest}). Do not edit.\n`;
}

/** Turns a semantic token name into a Swift/Kotlin-safe camelCase identifier. */
export function nativeIdentifier(value) {
  const words = value.match(/[A-Za-z]+|\d+/g) ?? [];
  const identifier = words
    .map((word, index) => {
      const normalized = /^\d/.test(word) ? `_${word}` : word;
      return index === 0 ? normalized : `${normalized[0].toUpperCase()}${normalized.slice(1)}`;
    })
    .join("");
  return identifier || "token";
}
