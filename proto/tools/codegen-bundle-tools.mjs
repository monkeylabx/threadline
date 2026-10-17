export const generationToolNames = Object.freeze([
  "buf", "protoc", "protoc-gen-go", "protoc-gen-connect-go", "protoc-gen-es",
  "protoc-gen-prost", "protoc-gen-prost-crate", "protoc-gen-swift",
  "protoc-gen-connect-swift", "protoc-gen-connect-kotlin", "java", "javac", "node",
]);

// Repository generation adds exactly one reviewed executable; other modes
// retain their original exact tool set and cannot install generated surfaces.
export function bundleToolNames(tools) {
  const expected = Object.hasOwn(tools, "git") ? [...generationToolNames, "git"] : [...generationToolNames];
  if (JSON.stringify(Object.keys(tools).sort()) !== JSON.stringify([...expected].sort())) {
    throw new Error("bundle tools must contain exactly the generation tools, optionally with repository Git");
  }
  return expected;
}
