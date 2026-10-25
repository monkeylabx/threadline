/** The shared identity avatar roles (identitySlate … identityAmber in the design tokens). */
export const identityRoles = ["slate", "plum", "clay", "amber"] as const;
export type IdentityRole = (typeof identityRoles)[number];

// FNV-1a with a murmur3 finalizer spreads keys evenly over the four roles. The
// seed is arbitrary; this one keeps the prototype's people on their V34 colors.
const seed = 0x811c9f12;

/**
 * Maps a stable author key to one identity role so a person keeps the same
 * avatar color in every channel and on every device. Pure and total.
 */
export function identityRole(authorKey: string): IdentityRole {
  let hash = seed;
  for (const unit of authorKey) hash = Math.imul(hash ^ unit.codePointAt(0)!, 0x01000193);
  hash = Math.imul(hash ^ (hash >>> 16), 0x85ebca6b);
  hash = Math.imul(hash ^ (hash >>> 13), 0xc2b2ae35);
  return identityRoles[((hash ^ (hash >>> 16)) >>> 0) % identityRoles.length]!;
}
