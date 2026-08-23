import { createHash } from "node:crypto";
import {
  cpSync,
  existsSync,
  lstatSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  readlinkSync,
  realpathSync,
  writeFileSync,
} from "node:fs";
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../../", import.meta.url));
const toolchain = JSON.parse(readFileSync(join(repositoryRoot, "proto", "toolchain.lock.json"), "utf8"));
const generationTools = [
  "buf",
  "protoc",
  "protoc-gen-go",
  "protoc-gen-connect-go",
  "protoc-gen-es",
  "protoc-gen-prost",
  "protoc-gen-prost-crate",
  "protoc-gen-swift",
  "protoc-gen-connect-swift",
  "protoc-gen-connect-kotlin",
  "java",
  "javac",
  "node",
];

function fail(message) {
  throw new Error(message);
}

function hashFile(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

function compareCodepoints(left, right) {
  return left < right ? -1 : left > right ? 1 : 0;
}

function walkExactTree(root, current = "") {
  return readdirSync(join(root, current), { withFileTypes: true })
    .sort((left, right) => compareCodepoints(left.name, right.name))
    .flatMap((entry) => {
      const relativePath = current ? `${current}/${entry.name}` : entry.name;
      const absolutePath = join(root, ...relativePath.split("/"));
      if (entry.isDirectory()) return walkExactTree(root, relativePath);
      if (entry.isFile()) return [{ path: relativePath, type: "file", sha256: hashFile(absolutePath) }];
      if (entry.isSymbolicLink()) {
        const target = readlinkSync(absolutePath);
        if (isAbsolute(target)) fail(`closure symlink must be relative: ${absolutePath}`);
        const resolved = realpathSync(absolutePath);
        if (!isWithin(realpathSync(root), resolved)) fail(`closure symlink escapes its root: ${absolutePath}`);
        return [{ path: relativePath, type: "symlink", target }];
      }
      fail(`unsupported closure entry: ${absolutePath}`);
    })
    .sort((left, right) => compareCodepoints(left.path, right.path));
}

function treeDigest(entries) {
  const serialized = entries.map((entry) => entry.type === "file"
    ? `file\0${entry.path}\0${entry.sha256}\n`
    : `symlink\0${entry.path}\0${entry.target}\n`).join("");
  return createHash("sha256").update(serialized).digest("hex");
}

function isWithin(parent, child) {
  const fromParent = relative(parent, child);
  return fromParent === "" || (!fromParent.startsWith(`..${sep}`) && fromParent !== ".." && !isAbsolute(fromParent));
}

function safeIdentifier(value, label) {
  if (typeof value !== "string" || !/^[a-z0-9](?:[a-z0-9._-]{0,126}[a-z0-9])?$/u.test(value)) fail(`${label} is not a safe identifier`);
}

function regularFile(path, label) {
  if (!existsSync(path) || lstatSync(path).isSymbolicLink() || !lstatSync(path).isFile()) fail(`${label} must be a regular file: ${path}`);
}

function realDirectory(path, label) {
  if (!existsSync(path) || lstatSync(path).isSymbolicLink() || !lstatSync(path).isDirectory()) fail(`${label} must be a real directory: ${path}`);
}

function exactKeys(object, expected, label) {
  const actual = Object.keys(object ?? {}).sort(compareCodepoints);
  const wanted = [...expected].sort(compareCodepoints);
  if (JSON.stringify(actual) !== JSON.stringify(wanted)) fail(`${label} keys mismatch: expected ${wanted.join(", ")}; got ${actual.join(", ")}`);
}

function main() {
  const specPath = process.env.THREADLINE_CODEGEN_BUNDLE_SPEC;
  const bundleRoot = process.env.THREADLINE_CODEGEN_BUNDLE_DIR;
  if (!specPath || !bundleRoot) fail("THREADLINE_CODEGEN_BUNDLE_SPEC and THREADLINE_CODEGEN_BUNDLE_DIR are required");
  regularFile(resolve(specPath), "bundle spec");
  const spec = JSON.parse(readFileSync(resolve(specPath), "utf8"));
  exactKeys(spec, ["schemaVersion", "platform", "profile", "sources", "closures", "tools"], "bundle spec");
  if (spec.schemaVersion !== 1 || spec.platform !== "darwin-arm64" || spec.profile !== "release") fail("bundle spec must be schema 1 darwin-arm64 release");
  exactKeys(spec.tools, generationTools, "bundle tools");

  const outputRoot = resolve(bundleRoot);
  if (outputRoot === resolve("/") || outputRoot === repositoryRoot || outputRoot.startsWith(`${repositoryRoot}${sep}`)) {
    fail("bundle output must be outside the repository");
  }
  if (existsSync(outputRoot)) {
    fail(`bundle output must be a new, non-existing directory: ${outputRoot}`);
  }
  mkdirSync(outputRoot, { mode: 0o700 });

  const sources = {};
  for (const [name, input] of Object.entries(spec.sources).sort(([left], [right]) => compareCodepoints(left, right))) {
    safeIdentifier(name, "source name");
    if (input.kind === "host-builder-toolchain") {
      exactKeys(input, ["kind", "path", "url", "authentication"], `${name} host source`);
      sources[name] = input;
      continue;
    }
    exactKeys(input, input.kind === "builder-toolchain" ? ["kind", "path", "url", "authentication"] : ["kind", "path", "url"], `${name} source`);
    const inputPath = resolve(input.path);
    regularFile(inputPath, `${name} source`);
    const destination = join(outputRoot, "sources", name, basename(inputPath));
    mkdirSync(dirname(destination), { recursive: true });
    cpSync(inputPath, destination, { errorOnExist: true });
    const relativeFile = relative(outputRoot, destination).split(sep).join("/");
    const source = { kind: input.kind, file: relativeFile, url: input.url, sha256: hashFile(destination) };
    if (input.kind === "builder-toolchain") {
      const authentication = { ...input.authentication };
      if (authentication.file) {
        const evidencePath = resolve(authentication.file);
        regularFile(evidencePath, `${name} authentication evidence`);
        const evidenceDestination = join(outputRoot, "sources", name, basename(evidencePath));
        cpSync(evidencePath, evidenceDestination, { errorOnExist: true });
        authentication.file = relative(outputRoot, evidenceDestination).split(sep).join("/");
        authentication.sha256 = hashFile(evidenceDestination);
      }
      source.authentication = authentication;
    }
    sources[name] = source;
  }

  const closureInputs = {};
  const closures = {};
  for (const [name, input] of Object.entries(spec.closures).sort(([left], [right]) => compareCodepoints(left, right))) {
    safeIdentifier(name, "closure name");
    exactKeys(input, ["root", "sources"], `${name} closure`);
    const inputRoot = resolve(input.root);
    realDirectory(inputRoot, `${name} closure`);
    if (!Array.isArray(input.sources) || input.sources.length === 0 || input.sources.some((source) => !sources[source])) fail(`${name} closure sources are invalid`);
    const destination = join(outputRoot, "closures", name);
    mkdirSync(dirname(destination), { recursive: true });
    cpSync(inputRoot, destination, { recursive: true, verbatimSymlinks: true, errorOnExist: true });
    const files = walkExactTree(destination);
    closureInputs[name] = { original: realpathSync(inputRoot), copied: destination };
    closures[name] = {
      root: relative(outputRoot, destination).split(sep).join("/"),
      sources: input.sources,
      treeSha256: treeDigest(files),
      files,
    };
  }

  const tools = {};
  for (const name of generationTools) {
    const input = spec.tools[name];
    exactKeys(input, ["path", "closure", "provenance", "invocation"], `${name} tool`);
    if (!closureInputs[input.closure]) fail(`${name} references unknown closure ${input.closure}`);
    const originalPath = realpathSync(resolve(input.path));
    const closure = closureInputs[input.closure];
    if (!isWithin(closure.original, originalPath)) fail(`${name} is outside closure ${input.closure}`);
    const copiedPath = join(closure.copied, relative(closure.original, originalPath));
    regularFile(copiedPath, `${name} copied tool`);
    tools[name] = {
      path: relative(outputRoot, copiedPath).split(sep).join("/"),
      sha256: hashFile(copiedPath),
      closure: input.closure,
      provenance: input.provenance,
      invocation: input.invocation,
    };
  }

  const manifest = {
    schemaVersion: toolchain.integrity.toolManifestSchemaVersion,
    platform: spec.platform,
    profile: spec.profile,
    sources,
    closures,
    tools,
  };
  const manifestPath = join(outputRoot, "manifest.json");
  writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, { mode: 0o600 });
  const manifestSha256 = hashFile(manifestPath);
  writeFileSync(join(outputRoot, "manifest.sha256"), `${manifestSha256}  manifest.json\n`, { mode: 0o600 });
  console.log(JSON.stringify({ bundle: outputRoot, manifest: manifestPath, manifestSha256 }, null, 2));
}

main();
