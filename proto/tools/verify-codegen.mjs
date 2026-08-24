import { createHash } from "node:crypto";
import {
  closeSync,
  cpSync,
  existsSync,
  lstatSync,
  mkdtempSync,
  mkdirSync,
  openSync,
  readFileSync,
  readdirSync,
  readlinkSync,
  realpathSync,
  renameSync,
  rmSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, delimiter, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("../../", import.meta.url));
const protoRoot = join(repositoryRoot, "proto");
const formalBufConfig = join(protoRoot, "buf.formal.yaml");

// This is a fail-closed misuse check, not a bootstrap trust root: NODE_OPTIONS or
// a compromised launcher can run code before this module executes. Formal runs
// must start this file with an approved clean-environment launcher and the
// absolute, pre-verified Node binary named by the reviewed manifest digest.
function assertCleanBootstrapEnvironment() {
  const exact = new Set([
    "NODE_OPTIONS",
    "NODE_PATH",
    "LD_PRELOAD",
    "JAVA_TOOL_OPTIONS",
    "JDK_JAVA_OPTIONS",
    "_JAVA_OPTIONS",
  ]);
  const forbidden = Object.keys(process.env).filter((name) => exact.has(name) || name.startsWith("DYLD_") || name.startsWith("GIT_"));
  if (forbidden.length > 0) {
    throw new Error(`unsafe bootstrap environment is forbidden (${forbidden.sort().join(", ")}); use the approved clean-environment launcher`);
  }
}

assertCleanBootstrapEnvironment();

const toolchain = JSON.parse(readFileSync(join(protoRoot, "toolchain.lock.json"), "utf8"));
const workspaceToolchain = JSON.parse(readFileSync(join(repositoryRoot, "toolchains.json"), "utf8"));
const generationPlan = JSON.parse(readFileSync(join(protoRoot, "buf.gen.yaml"), "utf8"));
const generatorTools = [
  "protoc-gen-go",
  "protoc-gen-connect-go",
  "protoc-gen-es",
  "protoc-gen-prost",
  "protoc-gen-prost-crate",
  "protoc-gen-swift",
  "protoc-gen-connect-swift",
  "protoc-gen-connect-kotlin",
];
const generationTools = ["buf", "protoc", ...generatorTools, "java", "javac", "node"];
const versionlessGenerators = new Set(["protoc-gen-connect-swift", "protoc-gen-connect-kotlin"]);
const modes = new Set(["verify-only", "repository", "protocol-smoke"]);

function parseMode() {
  const argumentsAfterNode = process.argv.slice(2);
  if (argumentsAfterNode.length !== 1 || !argumentsAfterNode[0].startsWith("--mode=")) {
    throw new Error("exactly one mode is required: --mode=verify-only, --mode=repository, or --mode=protocol-smoke");
  }
  const value = argumentsAfterNode[0].slice("--mode=".length);
  if (!modes.has(value)) throw new Error(`unsupported codegen mode: ${value}`);
  return value;
}

function requiredEnvironment(name) {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is required; see docs/contracts/codegen.md`);
  return value;
}

function run(command, args, options = {}) {
  if (!options.env) throw new Error(`internal error: ${basename(command)} spawn omitted the minimal verified environment`);
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? repositoryRoot,
    encoding: "utf8",
    env: options.env,
    stdio: options.capture ? "pipe" : "inherit",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    const diagnostic = options.capture ? `\n${result.stdout}${result.stderr}` : "";
    throw new Error(`${basename(command)} exited with ${result.status}${diagnostic}`);
  }
  return `${result.stdout ?? ""}${result.stderr ?? ""}`.trim();
}

function runInvocation(invocation, args, options = {}) {
  return run(invocation[0], [...invocation.slice(1), ...args], options);
}

export function formalBufArguments(command, args = []) {
  return [command, "--config", formalBufConfig, ...args];
}

function hashFile(algorithm, path) {
  return createHash(algorithm).update(readFileSync(path)).digest("hex");
}

function hashText(value) {
  return createHash("sha256").update(value).digest("hex");
}

function assertCanonicalDigest(value, length, label) {
  if (typeof value !== "string" || !new RegExp(`^[0-9a-f]{${length}}$`, "u").test(value)) {
    throw new Error(`${label} must be canonical lowercase hex (${length} characters)`);
  }
}

function assertExactKeys(label, actual, expected) {
  if (!actual || typeof actual !== "object" || Array.isArray(actual)) throw new Error(`${label} must be an object`);
  const actualKeys = Object.keys(actual).sort();
  const expectedKeys = [...expected].sort();
  if (JSON.stringify(actualKeys) !== JSON.stringify(expectedKeys)) {
    throw new Error(`${label} exact set mismatch: expected ${expectedKeys.join(", ")}; got ${actualKeys.join(", ")}`);
  }
}

function assertVersionToken(label, output, expected) {
  const tokens = output.match(/\d+(?:\.\d+){1,2}(?:\+\d+)?/gu) ?? [];
  if (!tokens.includes(expected)) throw new Error(`${label} version mismatch: expected exact token ${expected}; got ${output}`);
}

function canonicalRelativePath(value, label) {
  if (typeof value !== "string" || value.length === 0 || value.includes("\0") || value.includes(delimiter) || isAbsolute(value) || value.includes("\\")) {
    throw new Error(`${label} must be a non-empty, slash-separated relative path`);
  }
  const segments = value.split("/");
  if (segments.some((segment) => segment.length === 0 || segment === "." || segment === "..")) {
    throw new Error(`${label} contains a non-canonical path segment`);
  }
  return segments.join(sep);
}

function assertSafeIdentifier(value, label) {
  if (typeof value !== "string" || !/^[a-z0-9](?:[a-z0-9._-]{0,126}[a-z0-9])?$/u.test(value)) {
    throw new Error(`${label} must be a lowercase filesystem-safe identifier`);
  }
}

function assertBundleLocalPath(manifestDirectory, path, label, expectedType) {
  const canonicalManifestDirectory = realpathSync(manifestDirectory);
  assertNoSymlinkSegments(canonicalManifestDirectory, path, label);
  const canonicalPath = realpathSync(path);
  if (!isWithin(canonicalManifestDirectory, canonicalPath)) throw new Error(`${label} escapes the canonical manifest directory`);
  const status = lstatSync(path);
  if (status.isSymbolicLink() || (expectedType === "file" ? !status.isFile() : !status.isDirectory())) {
    throw new Error(`${label} must be a regular bundle-local ${expectedType}`);
  }
  return canonicalPath;
}

function isWithin(parent, child) {
  const fromParent = relative(parent, child);
  return fromParent === "" || (!fromParent.startsWith(`..${sep}`) && fromParent !== ".." && !isAbsolute(fromParent));
}

function walkExactTree(root, current = "") {
  return readdirSync(join(root, current), { withFileTypes: true })
    .sort((left, right) => compareCodepoints(left.name, right.name))
    .flatMap((entry) => {
      const relativePath = current ? `${current}/${entry.name}` : entry.name;
      const absolutePath = join(root, ...relativePath.split("/"));
      if (entry.isDirectory()) return walkExactTree(root, relativePath);
      if (entry.isFile()) return [{ path: relativePath, type: "file", sha256: hashFile("sha256", absolutePath) }];
      if (entry.isSymbolicLink()) return [{ path: relativePath, type: "symlink", target: readlinkSync(absolutePath) }];
      throw new Error(`runtime closure contains unsupported filesystem entry: ${absolutePath}`);
    })
    .sort((left, right) => compareCodepoints(left.path, right.path));
}

function compareCodepoints(left, right) {
  return left < right ? -1 : left > right ? 1 : 0;
}

function canonicalTreeDigest(entries) {
  const lines = entries.map((entry) => entry.type === "file"
    ? `file\0${entry.path}\0${entry.sha256}\n`
    : `symlink\0${entry.path}\0${entry.target}\n`);
  return hashText(lines.join(""));
}

function verifyBuilderAuthentication(name, source, manifestDirectory, snapshotRoot) {
  const authentication = source.authentication;
  if (!authentication || typeof authentication !== "object" || Array.isArray(authentication)) {
    throw new Error(`${name} builder-toolchain source requires authentication evidence`);
  }
  if (authentication.kind === "rust-distribution-sha256") {
    assertExactKeys(`${name} builder authentication`, authentication, ["kind", "file", "url", "sha256"]);
    if (!source.url.startsWith("https://static.rust-lang.org/dist/") || authentication.url !== `${source.url}.sha256`) {
      throw new Error(`${name} Rust builder checksum must be the matching static.rust-lang.org distribution sidecar`);
    }
    assertCanonicalDigest(authentication.sha256, 64, `${name} builder checksum metadata sha256`);
    const original = resolve(manifestDirectory, canonicalRelativePath(authentication.file, `${name} builder checksum metadata file`));
    if (!existsSync(original)) throw new Error(`${name} builder checksum metadata is missing`);
    assertBundleLocalPath(manifestDirectory, original, `${name} builder checksum metadata`, "file");
    if (hashFile("sha256", original) !== authentication.sha256) throw new Error(`${name} builder checksum metadata SHA-256 mismatch`);
    const metadata = readFileSync(original, "utf8").trim();
    if (metadata !== `${source.sha256}  ${basename(source.file)}`) {
      throw new Error(`${name} builder checksum metadata does not authenticate the source digest`);
    }
    const destination = join(snapshotRoot, "authentication", name, basename(original));
    mkdirSync(dirname(destination), { recursive: true });
    cpSync(original, destination, { errorOnExist: true });
    if (hashFile("sha256", destination) !== authentication.sha256) throw new Error(`${name} builder checksum metadata snapshot mismatch`);
    return;
  }
  if (authentication.kind === "go-distribution-json") {
    assertExactKeys(`${name} builder authentication`, authentication, ["kind", "file", "url", "sha256"]);
    if (authentication.url !== "https://go.dev/dl/?mode=json&include=all" || source.url !== `https://go.dev/dl/${basename(source.file)}`) {
      throw new Error(`${name} Go builder must use the matching go.dev archive and release metadata`);
    }
    assertCanonicalDigest(authentication.sha256, 64, `${name} Go release metadata sha256`);
    const original = resolve(manifestDirectory, canonicalRelativePath(authentication.file, `${name} Go release metadata file`));
    if (!existsSync(original)) throw new Error(`${name} Go release metadata is missing`);
    assertBundleLocalPath(manifestDirectory, original, `${name} Go release metadata`, "file");
    if (hashFile("sha256", original) !== authentication.sha256) throw new Error(`${name} Go release metadata SHA-256 mismatch`);
    const releases = JSON.parse(readFileSync(original, "utf8"));
    const expectedVersion = `go${toolchain.tools.go}`;
    const release = Array.isArray(releases) ? releases.find((item) => item?.version === expectedVersion && item?.stable === true) : undefined;
    const file = release?.files?.find((item) => item?.filename === basename(source.file));
    if (!file || file.os !== "darwin" || file.arch !== "arm64" || file.kind !== "archive" || file.sha256 !== source.sha256) {
      throw new Error(`${name} Go release metadata does not authenticate the pinned Darwin arm64 builder archive`);
    }
    const destination = join(snapshotRoot, "authentication", name, basename(original));
    mkdirSync(dirname(destination), { recursive: true });
    cpSync(original, destination, { errorOnExist: true });
    if (hashFile("sha256", destination) !== authentication.sha256) throw new Error(`${name} Go release metadata snapshot mismatch`);
    return;
  }
  if (authentication.kind === "apple-installer-signature") {
    assertExactKeys(`${name} builder authentication`, authentication, ["kind", "signer"]);
    if (process.platform !== "darwin" || !source.file.endsWith(".pkg")) {
      throw new Error(`${name} Apple installer authentication requires a Darwin .pkg source`);
    }
    if (authentication.signer !== "Developer ID Installer: Swift Open Source (V9AUD2URP3)") {
      throw new Error(`${name} Apple installer signer is not the pinned Swift Open Source identity`);
    }
    const packagePath = resolve(manifestDirectory, canonicalRelativePath(source.file, `${name} source file`));
    const trustResult = spawnSync("/usr/sbin/pkgutil", ["--check-signature", packagePath], {
      encoding: "utf8",
      env: { HOME: join(snapshotRoot, "platform-trust-home"), LANG: "C", LC_ALL: "C", PATH: "/usr/bin:/bin:/usr/sbin:/sbin", TZ: "UTC" },
      stdio: "pipe",
    });
    const evidence = `${trustResult.stdout ?? ""}${trustResult.stderr ?? ""}`;
    if (trustResult.error || trustResult.status !== 0 || !evidence.includes("Status: signed by a certificate trusted by macOS") || !evidence.includes(authentication.signer)) {
      throw new Error(`${name} Apple installer signature is not valid for the pinned Swift Open Source identity`);
    }
    return;
  }
  throw new Error(`${name} builder authentication kind is unsupported: ${authentication.kind}`);
}

function verifyHostBuilderAuthentication(name, source, snapshotRoot) {
  const authentication = source.authentication;
  assertExactKeys(`${name} host builder source`, source, ["kind", "path", "url", "authentication"]);
  assertExactKeys(`${name} host builder authentication`, authentication, [
    "kind",
    "bundleIdentifier",
    "version",
    "build",
    "swiftVersion",
    "sdkVersion",
  ]);
  if (authentication.kind !== "apple-xcode-gatekeeper") {
    throw new Error(`${name} host builder authentication kind is unsupported: ${authentication.kind}`);
  }
  if (process.platform !== "darwin" || process.arch !== "arm64") {
    throw new Error(`${name} Apple Xcode authentication requires Darwin arm64`);
  }
  if (source.path !== "/Applications/Xcode_26.6.app") {
    throw new Error(`${name} host builder must use the pinned Xcode 26.6 application path`);
  }
  if (!/^https:\/\/github\.com\/actions\/runner-images\/blob\/[0-9a-f]{40}\/images\/macos\/macos-26-arm64-Readme\.md$/u.test(source.url)) {
    throw new Error(`${name} host builder must cite an immutable GitHub runner-image inventory`);
  }
  if (authentication.bundleIdentifier !== "com.apple.dt.Xcode" || authentication.version !== "26.6" || authentication.build !== "17F113" || authentication.swiftVersion !== "6.3" || authentication.sdkVersion !== "26.5") {
    throw new Error(`${name} host builder identity does not match the pinned Xcode/Swift/SDK baseline`);
  }
  if (!existsSync(source.path) || lstatSync(source.path).isSymbolicLink() || !lstatSync(source.path).isDirectory() || realpathSync(source.path) !== source.path) {
    throw new Error(`${name} host builder must be the real pinned Xcode application directory`);
  }
  const trustEnvironment = {
    DEVELOPER_DIR: `${source.path}/Contents/Developer`,
    HOME: join(snapshotRoot, "platform-trust-home"),
    LANG: "C",
    LC_ALL: "C",
    PATH: "/usr/bin:/bin:/usr/sbin:/sbin",
    TZ: "UTC",
  };
  mkdirSync(trustEnvironment.HOME, { recursive: true, mode: 0o700 });
  const assess = spawnSync("/usr/sbin/spctl", ["--assess", "--type", "execute", "--verbose=4", source.path], { encoding: "utf8", env: trustEnvironment, stdio: "pipe" });
  const assessEvidence = `${assess.stdout ?? ""}${assess.stderr ?? ""}`;
  if (assess.error || assess.status !== 0 || !/accepted/iu.test(assessEvidence)) {
    throw new Error(`${name} Xcode application did not pass Gatekeeper assessment`);
  }
  const signature = spawnSync("/usr/bin/codesign", ["--verify", "--deep", "--strict", source.path], { encoding: "utf8", env: trustEnvironment, stdio: "pipe" });
  if (signature.error || signature.status !== 0) throw new Error(`${name} Xcode application signature verification failed`);
  const metadata = spawnSync("/usr/bin/plutil", ["-extract", "CFBundleIdentifier", "raw", "-o", "-", `${source.path}/Contents/Info.plist`], { encoding: "utf8", env: trustEnvironment, stdio: "pipe" });
  if (metadata.error || metadata.status !== 0 || metadata.stdout.trim() !== authentication.bundleIdentifier) {
    throw new Error(`${name} Xcode bundle identifier mismatch`);
  }
  const xcode = spawnSync("/usr/bin/xcodebuild", ["-version"], { encoding: "utf8", env: trustEnvironment, stdio: "pipe" });
  const xcodeEvidence = `${xcode.stdout ?? ""}${xcode.stderr ?? ""}`;
  if (xcode.error || xcode.status !== 0 || !xcodeEvidence.includes(`Xcode ${authentication.version}`) || !xcodeEvidence.includes(`Build version ${authentication.build}`)) {
    throw new Error(`${name} Xcode version/build mismatch`);
  }
  const swift = spawnSync("/usr/bin/xcrun", ["swift", "--version"], { encoding: "utf8", env: trustEnvironment, stdio: "pipe" });
  const swiftEvidence = `${swift.stdout ?? ""}${swift.stderr ?? ""}`;
  const swiftToken = authentication.swiftVersion.replaceAll(".", "\\.");
  if (swift.error || swift.status !== 0 || !new RegExp(`Apple Swift version ${swiftToken}(?:\\.\\d+)?(?:\\s|$)`, "u").test(swiftEvidence)) {
    throw new Error(`${name} Swift version mismatch`);
  }
  const sdk = spawnSync("/usr/bin/xcrun", ["--sdk", "macosx", "--show-sdk-version"], { encoding: "utf8", env: trustEnvironment, stdio: "pipe" });
  if (sdk.error || sdk.status !== 0 || sdk.stdout.trim() !== authentication.sdkVersion) {
    throw new Error(`${name} macOS SDK version mismatch`);
  }
}

function verifySources(manifest, manifestDirectory, snapshotRoot) {
  if (!manifest.sources || typeof manifest.sources !== "object" || Array.isArray(manifest.sources) || Object.keys(manifest.sources).length === 0) {
    throw new Error("Integration tool manifest must contain at least one locally verifiable source artifact");
  }
  const allowedKinds = new Set(["official-binary", "official-package", "source-archive", "builder-toolchain", "host-builder-toolchain", "protocol-fixture"]);
  const verified = {};
  for (const [name, source] of Object.entries(manifest.sources)) {
    assertSafeIdentifier(name, "source name");
    if (source?.kind === "host-builder-toolchain") {
      verifyHostBuilderAuthentication(name, source, snapshotRoot);
      verified[name] = { ...source };
      continue;
    }
    const sourceKeys = source?.kind === "builder-toolchain"
      ? ["kind", "file", "url", "sha256", "authentication"]
      : ["kind", "file", "url", "sha256"];
    assertExactKeys(`${name} source`, source, sourceKeys);
    if (!allowedKinds.has(source.kind)) throw new Error(`${name} source kind is unsupported: ${source.kind}`);
    if (typeof source.url !== "string" || !/^https:\/\//u.test(source.url)) throw new Error(`${name} source must record an HTTPS URL`);
    assertCanonicalDigest(source.sha256, 64, `${name} source sha256`);
    const original = resolve(manifestDirectory, canonicalRelativePath(source.file, `${name} source file`));
    if (!isWithin(manifestDirectory, original) || !existsSync(original)) throw new Error(`${name} source must be a regular bundle-local file`);
    assertBundleLocalPath(manifestDirectory, original, `${name} source`, "file");
    if (hashFile("sha256", original) !== source.sha256) throw new Error(`${name} source artifact SHA-256 mismatch`);
    if (source.kind === "builder-toolchain") verifyBuilderAuthentication(name, source, manifestDirectory, snapshotRoot);
    const destination = join(snapshotRoot, "sources", name, basename(original));
    mkdirSync(dirname(destination), { recursive: true });
    cpSync(original, destination, { errorOnExist: true });
    if (lstatSync(destination).isSymbolicLink() || !lstatSync(destination).isFile() || hashFile("sha256", destination) !== source.sha256) {
      throw new Error(`${name} source snapshot SHA-256 mismatch`);
    }
    verified[name] = { ...source, original, snapshot: destination };
  }
  return verified;
}

function verifyClosureTree(name, closure, root) {
  if (!existsSync(root) || lstatSync(root).isSymbolicLink() || !lstatSync(root).isDirectory()) {
    throw new Error(`${name} closure root must be a real directory: ${root}`);
  }
  assertCanonicalDigest(closure.treeSha256, 64, `${name} closure treeSha256`);
  if (!Array.isArray(closure.files) || closure.files.length === 0) throw new Error(`${name} closure must list every runtime file`);
  const expectedPaths = [];
  const expectedEntries = closure.files.map((entry, index) => {
    if (!entry || typeof entry !== "object") throw new Error(`${name} closure file ${index} is invalid`);
    const path = canonicalRelativePath(entry.path, `${name} closure file ${index}`);
    if (entry.type === "file") {
      assertExactKeys(`${name} closure file ${path}`, entry, ["path", "type", "sha256"]);
      assertCanonicalDigest(entry.sha256, 64, `${name}/${path} sha256`);
      expectedPaths.push(path.replaceAll(sep, "/"));
      return { path: path.replaceAll(sep, "/"), type: "file", sha256: entry.sha256 };
    }
    if (entry.type === "symlink") {
      assertExactKeys(`${name} closure symlink ${path}`, entry, ["path", "type", "target"]);
      if (typeof entry.target !== "string" || entry.target.length === 0 || isAbsolute(entry.target)) {
        throw new Error(`${name}/${path} symlink target must be relative`);
      }
      expectedPaths.push(path.replaceAll(sep, "/"));
      return { path: path.replaceAll(sep, "/"), type: "symlink", target: entry.target };
    }
    throw new Error(`${name}/${path} must declare type file or symlink`);
  });
  if (new Set(expectedPaths).size !== expectedPaths.length) throw new Error(`${name} closure contains duplicate paths`);
  if (JSON.stringify(expectedPaths) !== JSON.stringify([...expectedPaths].sort(compareCodepoints))) throw new Error(`${name} closure files must be sorted by canonical path`);
  const actualEntries = walkExactTree(root);
  if (JSON.stringify(actualEntries) !== JSON.stringify(expectedEntries)) throw new Error(`${name} runtime closure exact-set or digest mismatch`);
  if (canonicalTreeDigest(actualEntries) !== closure.treeSha256) throw new Error(`${name} runtime closure treeSha256 mismatch`);
  const canonicalRoot = realpathSync(root);
  for (const entry of actualEntries.filter((item) => item.type === "symlink")) {
    const resolvedTarget = realpathSync(join(root, ...entry.path.split("/")));
    // macOS exposes /tmp through the /private/tmp symlink.  Compare canonical
    // paths so an in-closure link cannot be mistaken for an escape merely
    // because the temporary root and the resolved target use those aliases.
    if (!isWithin(canonicalRoot, resolvedTarget)) throw new Error(`${name}/${entry.path} symlink escapes its verified closure`);
  }
}

function verifyRuntimeClosures(manifest, manifestDirectory, verifiedSources, snapshotRoot) {
  if (!manifest.closures || typeof manifest.closures !== "object" || Array.isArray(manifest.closures) || Object.keys(manifest.closures).length === 0) {
    throw new Error("Integration tool manifest must contain at least one exact runtime closure");
  }

  const closureRoots = {};
  for (const [name, closure] of Object.entries(manifest.closures)) {
    assertSafeIdentifier(name, "closure name");
    assertExactKeys(`${name} runtime closure`, closure, ["root", "sources", "treeSha256", "files"]);
    const root = resolve(manifestDirectory, canonicalRelativePath(closure.root, `${name} closure root`));
    if (!isWithin(manifestDirectory, root) || !existsSync(root)) throw new Error(`${name} closure root escapes the manifest directory`);
    const canonicalRoot = assertBundleLocalPath(manifestDirectory, root, `${name} closure root`, "directory");
    if (!Array.isArray(closure.sources) || closure.sources.length === 0 || new Set(closure.sources).size !== closure.sources.length) {
      throw new Error(`${name} closure sources must be a non-empty unique array`);
    }
    for (const source of closure.sources) if (typeof source !== "string" || !verifiedSources[source]) throw new Error(`${name} closure references unknown source ${source}`);
    verifyClosureTree(name, closure, root);
    const snapshot = join(snapshotRoot, "closures", name);
    mkdirSync(dirname(snapshot), { recursive: true });
    cpSync(root, snapshot, { recursive: true, verbatimSymlinks: true, errorOnExist: true });
    verifyClosureTree(name, closure, snapshot);
    closureRoots[name] = { original: root, canonicalOriginal: canonicalRoot, snapshot, sources: closure.sources };
  }

  const rootEntries = Object.entries(closureRoots).map(([name, roots]) => [name, roots.canonicalOriginal]);
  for (let leftIndex = 0; leftIndex < rootEntries.length; leftIndex += 1) {
    for (let rightIndex = leftIndex + 1; rightIndex < rootEntries.length; rightIndex += 1) {
      const [leftName, leftRoot] = rootEntries[leftIndex];
      const [rightName, rightRoot] = rootEntries[rightIndex];
      if (isWithin(leftRoot, rightRoot) || isWithin(rightRoot, leftRoot)) {
        throw new Error(`runtime closures must not overlap: ${leftName}, ${rightName}`);
      }
    }
  }
  return closureRoots;
}

function verifyProvenance(name, provenance, sources, mode) {
  if (!provenance || typeof provenance !== "object" || Array.isArray(provenance)) throw new Error(`${name} provenance must be an object`);
  if (provenance.kind === "official-binary" || provenance.kind === "official-package" || provenance.kind === "protocol-stub") {
    assertExactKeys(`${name} provenance`, provenance, ["kind", "sources"]);
    if (!Array.isArray(provenance.sources) || provenance.sources.length === 0 || new Set(provenance.sources).size !== provenance.sources.length) {
      throw new Error(`${name} provenance sources must be a non-empty unique array`);
    }
    for (const source of provenance.sources) if (!sources[source]) throw new Error(`${name} provenance references unknown source ${source}`);
    const expectedSourceKind = provenance.kind === "protocol-stub" ? "protocol-fixture" : provenance.kind;
    for (const source of provenance.sources) {
      if (sources[source].kind !== expectedSourceKind) throw new Error(`${name} ${provenance.kind} provenance cannot cite ${sources[source].kind} source ${source}`);
    }
    if (provenance.kind === "protocol-stub" && mode !== "protocol-smoke") throw new Error(`${name} protocol-stub provenance is forbidden in ${mode} mode`);
    return provenance.sources;
  }
  if (provenance.kind === "source-built") {
    assertExactKeys(`${name} provenance`, provenance, ["kind", "source", "builders", "buildCommand", "reproducibility"]);
    if (!sources[provenance.source]) throw new Error(`${name} source-built provenance references unknown source ${provenance.source}`);
    if (sources[provenance.source].kind !== "source-archive") throw new Error(`${name} source-built source must be classified source-archive`);
    if (!Array.isArray(provenance.builders) || provenance.builders.length === 0 || new Set(provenance.builders).size !== provenance.builders.length) {
      throw new Error(`${name} source-built builders must be a non-empty unique array`);
    }
    for (const source of provenance.builders) {
      if (!sources[source]) throw new Error(`${name} source-built provenance references unknown builder ${source}`);
      if (!new Set(["builder-toolchain", "host-builder-toolchain"]).has(sources[source].kind)) {
        throw new Error(`${name} source-built builder must be classified builder-toolchain or host-builder-toolchain: ${source}`);
      }
    }
    if (typeof provenance.buildCommand !== "string" || provenance.buildCommand.trim() !== provenance.buildCommand || provenance.buildCommand.length === 0) {
      throw new Error(`${name} source-built buildCommand must be a non-empty exact command record`);
    }
    if (provenance.reproducibility !== "single-build-output-verified") throw new Error(`${name} source-built reproducibility classification is invalid`);
    return [provenance.source, ...provenance.builders];
  }
  throw new Error(`${name} provenance kind is unsupported: ${provenance.kind}`);
}

function assertNoSymlinkSegments(root, path, label) {
  if (!isWithin(root, path)) throw new Error(`${label} escapes its verified closure`);
  let current = root;
  for (const segment of relative(root, path).split(sep).filter(Boolean)) {
    current = join(current, segment);
    if (lstatSync(current).isSymbolicLink()) throw new Error(`${label} may not execute through a symlink: ${current}`);
  }
}

function verifyToolManifest(manifestPath, expectedDigest, mode, snapshotRoot) {
  assertCanonicalDigest(expectedDigest, 64, "THREADLINE_PROTO_TOOL_MANIFEST_SHA256");
  if (!existsSync(manifestPath) || lstatSync(manifestPath).isSymbolicLink() || !lstatSync(manifestPath).isFile()) {
    throw new Error("Integration tool manifest must be a regular, non-symlink file");
  }
  if (hashFile("sha256", manifestPath) !== expectedDigest) throw new Error("Integration tool manifest SHA-256 mismatch");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
  if (manifest.schemaVersion !== toolchain.integrity.toolManifestSchemaVersion) throw new Error("Integration tool manifest schema mismatch");
  assertExactKeys("Integration tool manifest", manifest, ["schemaVersion", "platform", "profile", "sources", "closures", "tools"]);
  if (manifest.platform !== `${process.platform}-${process.arch}`) {
    throw new Error(`Integration tool manifest platform mismatch: expected ${process.platform}-${process.arch}`);
  }
  const expectedProfile = mode === "protocol-smoke" ? "protocol-smoke" : "release";
  if (manifest.profile !== expectedProfile) throw new Error(`${mode} requires a ${expectedProfile} manifest profile`);
  if (expectedProfile === "release" && manifest.platform !== "darwin-arm64") {
    throw new Error("formal release codegen is restricted to the protected darwin-arm64 Integration runner");
  }
  const requiredTools = mode === "repository" ? [...generationTools, "git"] : generationTools;
  assertExactKeys("Integration tool manifest tools", manifest.tools, requiredTools);

  const manifestDirectory = realpathSync(dirname(manifestPath));
  const verifiedSources = verifySources(manifest, manifestDirectory, snapshotRoot);
  const closureRoots = verifyRuntimeClosures(manifest, manifestDirectory, verifiedSources, snapshotRoot);
  const referencedClosures = new Set();
  const referencedSources = new Set();
  const tools = {};
  const invocationKinds = {};
  for (const name of requiredTools) {
    const entry = manifest.tools[name];
    assertExactKeys(`${name} tool entry`, entry, ["path", "sha256", "closure", "provenance", "invocation"]);
    assertCanonicalDigest(entry.sha256, 64, `${name} sha256`);
    if (typeof entry.closure !== "string" || !closureRoots[entry.closure]) throw new Error(`${name} references an unknown runtime closure`);
    const provenanceSources = verifyProvenance(name, entry.provenance, verifiedSources, mode);
    for (const source of provenanceSources) referencedSources.add(source);
    for (const source of closureRoots[entry.closure].sources) referencedSources.add(source);
    if (provenanceSources.some((source) => !closureRoots[entry.closure].sources.includes(source))) {
      throw new Error(`${name} closure sources must include every provenance source`);
    }
    referencedClosures.add(entry.closure);
    const originalPath = resolve(manifestDirectory, canonicalRelativePath(entry.path, `${name} path`));
    const roots = closureRoots[entry.closure];
    if (!isWithin(roots.original, originalPath)) throw new Error(`${name} is outside its declared runtime closure`);
    const path = join(roots.snapshot, relative(roots.original, originalPath));
    assertNoSymlinkSegments(roots.snapshot, path, `${name} executable`);
    const acceptedNames = entry.invocation === "verified-node"
      ? [name, `${name}.js`]
      : entry.invocation === "verified-java"
        ? [`${name}.jar`]
        : process.platform === "win32" ? [`${name}.exe`] : [name];
    if (!acceptedNames.includes(basename(path))) throw new Error(`${name} manifest path has an unexpected basename: ${basename(path)}`);
    if (!existsSync(path) || !lstatSync(path).isFile()) throw new Error(`${name} path is not a regular file`);
    if (hashFile("sha256", path) !== entry.sha256) throw new Error(`${name} executable SHA-256 mismatch`);

    const head = readFileSync(path).subarray(0, 160).toString("utf8");
    if (entry.invocation === "native") {
      if (head.startsWith("#!")) throw new Error(`${name} declares native invocation but is an interpreted script`);
    } else if (entry.invocation === "verified-node") {
      if (!head.startsWith("#!") || !/^#![^\n]*\bnode(?:\s|$)/u.test(head.split("\n", 1)[0])) {
        throw new Error(`${name} must be a Node script when invocation is verified-node`);
      }
    } else if (entry.invocation === "verified-java") {
      if (name !== "protoc-gen-connect-kotlin" || !readFileSync(path).subarray(0, 4).equals(Buffer.from([0x50, 0x4b, 0x03, 0x04]))) {
        throw new Error(`${name} must be the verified executable JAR when invocation is verified-java`);
      }
    } else {
      throw new Error(`${name} invocation must be native, verified-node, or verified-java`);
    }
    tools[name] = path;
    invocationKinds[name] = entry.invocation;
  }
  if (invocationKinds.node !== "native") throw new Error("the verified Node runtime must use native invocation");
  for (const name of requiredTools.filter((tool) => !generatorTools.includes(tool))) {
    if (invocationKinds[name] !== "native") throw new Error(`${name} must use native invocation`);
  }
  const invocations = Object.fromEntries(Object.entries(tools).map(([name, path]) => [
    name,
    invocationKinds[name] === "verified-node"
      ? [tools.node, path]
      : invocationKinds[name] === "verified-java" ? [tools.java, "-jar", path] : [path],
  ]));
  assertExactKeys("referenced runtime closures", Object.fromEntries([...referencedClosures].map((name) => [name, true])), Object.keys(closureRoots));
  assertExactKeys("referenced source artifacts", Object.fromEntries([...referencedSources].map((name) => [name, true])), Object.keys(verifiedSources));
  if (mode === "protocol-smoke" && !generatorTools.some((name) => manifest.tools[name].provenance.kind === "protocol-stub")) {
    throw new Error("protocol-smoke requires at least one generator classified as protocol-stub; use verify-only for real plugins");
  }
  return { manifest, tools, invocations };
}

function verifiedGenerationTemplate(tools, invocations) {
  return {
    ...generationPlan,
    plugins: generationPlan.plugins.map((plugin) => {
      if (typeof plugin.local === "string") return { ...plugin, local: invocations[plugin.local] };
      if (plugin.protoc_builtin === "java" || plugin.protoc_builtin === "kotlin") {
        return { ...plugin, protoc_path: tools.protoc };
      }
      throw new Error("buf.gen.yaml contains an unsupported plugin entry");
    }),
  };
}

function assertGenerationPlan() {
  const expected = {
    version: "v2",
    clean: true,
    managed: {
      enabled: true,
      override: [
        { file_option: "go_package_prefix", value: "github.com/monkeylabx/threadline/services/gen" },
        { file_option: "java_package_prefix", value: "com.threadline.proto" },
      ],
    },
    plugins: [
      { local: "protoc-gen-go", out: toolchain.outputs.go, opt: ["paths=source_relative"] },
      { local: "protoc-gen-connect-go", out: toolchain.outputs.go, opt: ["paths=source_relative"] },
      { local: "protoc-gen-es", out: toolchain.outputs.typescript, opt: ["target=ts", "import_extension=.js"] },
      { local: "protoc-gen-prost", out: toolchain.outputs.rust, opt: ["bytes=.", "compile_well_known_types=false"] },
      { local: "protoc-gen-prost-crate", out: toolchain.outputs.rust, opt: ["no_features"], strategy: "all" },
      { local: "protoc-gen-swift", out: toolchain.outputs.swift, opt: ["Visibility=Public", "FileNaming=PathToUnderscores"] },
      { local: "protoc-gen-connect-swift", out: toolchain.outputs.swift, opt: ["Visibility=Public", "FileNaming=PathToUnderscores"] },
      { protoc_builtin: "java", protoc_path: "protoc", out: toolchain.outputs.kotlin.javaMessages },
      { protoc_builtin: "kotlin", protoc_path: "protoc", out: toolchain.outputs.kotlin.kotlinDsl },
      { local: "protoc-gen-connect-kotlin", out: toolchain.outputs.kotlin.kotlinDsl },
    ],
    inputs: [{ directory: "proto" }],
  };
  if (JSON.stringify(generationPlan) !== JSON.stringify(expected)) {
    throw new Error("buf.gen.yaml must exactly match the pinned five-language generation plan");
  }
}

function verifyPinnedArtifact(path, pin, label) {
  if (lstatSync(path).isSymbolicLink() || !lstatSync(path).isFile()) throw new Error(`${label} must be a regular file, not a symlink`);
  if (hashFile("sha256", path) !== pin.sha256) throw new Error(`${label} SHA-256 mismatch`);
  if (hashFile("sha1", path) !== pin.sourceSha1) throw new Error(`${label} SHA-1 does not match the pinned Maven Central sourceSha1`);
}

function verifyKotlinArtifacts(tools, snapshotRoot, environment) {
  const compilerDirectory = resolve(requiredEnvironment("THREADLINE_KOTLIN_COMPILER_DIR"));
  if (lstatSync(compilerDirectory).isSymbolicLink() || !lstatSync(compilerDirectory).isDirectory()) {
    throw new Error("THREADLINE_KOTLIN_COMPILER_DIR must be a real directory");
  }
  const compilerPins = toolchain.integrity.kotlinCompilerClasspath;
  const compilerEntries = readdirSync(compilerDirectory, { withFileTypes: true });
  if (compilerEntries.some((entry) => !entry.isFile() || !entry.name.endsWith(".jar"))) {
    throw new Error("Kotlin compiler directory may contain only the exact pinned regular JAR files");
  }
  const compilerFiles = Object.fromEntries(compilerEntries.map((entry) => [entry.name, join(compilerDirectory, entry.name)]));
  assertExactKeys("Kotlin compiler directory", compilerFiles, Object.keys(compilerPins));
  for (const [name, path] of Object.entries(compilerFiles)) verifyPinnedArtifact(path, compilerPins[name], name);
  const compilerSnapshot = join(snapshotRoot, "kotlin", "compiler");
  mkdirSync(compilerSnapshot, { recursive: true });
  for (const [name, path] of Object.entries(compilerFiles)) cpSync(path, join(compilerSnapshot, name), { errorOnExist: true });
  const compilerSnapshotFiles = Object.fromEntries(Object.keys(compilerFiles).map((name) => [name, join(compilerSnapshot, name)]));
  for (const [name, path] of Object.entries(compilerSnapshotFiles)) verifyPinnedArtifact(path, compilerPins[name], `${name} snapshot`);
  const kotlinCompilerClasspath = Object.keys(compilerSnapshotFiles).sort().map((name) => compilerSnapshotFiles[name]).join(delimiter);
  assertVersionToken(
    "Kotlin compiler",
    run(tools.java, ["-cp", kotlinCompilerClasspath, "org.jetbrains.kotlin.cli.jvm.K2JVMCompiler", "-version"], { capture: true, env: environment }),
    toolchain.tools["kotlin-compiler"],
  );

  const runtimePins = toolchain.integrity.runtimeJars;
  const protobufJava = resolve(requiredEnvironment("THREADLINE_PROTOBUF_JAVA_JAR"));
  const protobufKotlin = resolve(requiredEnvironment("THREADLINE_PROTOBUF_KOTLIN_JAR"));
  const kotlinStdlib = resolve(requiredEnvironment("THREADLINE_KOTLIN_STDLIB_JAR"));
  const connectKotlin = resolve(requiredEnvironment("THREADLINE_CONNECT_KOTLIN_JAR"));
  const runtimeFiles = {
    [basename(connectKotlin)]: connectKotlin,
    [basename(protobufJava)]: protobufJava,
    [basename(protobufKotlin)]: protobufKotlin,
    [basename(kotlinStdlib)]: kotlinStdlib,
  };
  assertExactKeys("Kotlin runtime artifacts", runtimeFiles, Object.keys(runtimePins));
  for (const [name, path] of Object.entries(runtimeFiles)) verifyPinnedArtifact(path, runtimePins[name], name);
  const runtimeSnapshot = join(snapshotRoot, "kotlin", "runtime");
  mkdirSync(runtimeSnapshot, { recursive: true });
  for (const [name, path] of Object.entries(runtimeFiles)) cpSync(path, join(runtimeSnapshot, name), { errorOnExist: true });
  const runtimeSnapshotFiles = Object.fromEntries(Object.keys(runtimeFiles).map((name) => [name, join(runtimeSnapshot, name)]));
  for (const [name, path] of Object.entries(runtimeSnapshotFiles)) verifyPinnedArtifact(path, runtimePins[name], `${name} snapshot`);
  return {
    kotlinCompilerClasspath,
    protobufJava: runtimeSnapshotFiles[basename(protobufJava)],
    protobufKotlin: runtimeSnapshotFiles[basename(protobufKotlin)],
    kotlinStdlib: runtimeSnapshotFiles[basename(kotlinStdlib)],
    connectKotlin: runtimeSnapshotFiles[basename(connectKotlin)],
  };
}

function filesBelow(directory, suffix = "") {
  if (!existsSync(directory)) return [];
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return filesBelow(path, suffix);
    if (entry.isSymbolicLink()) throw new Error(`generated output must not contain symlinks: ${path}`);
    return path.endsWith(suffix) ? [path] : [];
  });
}

function outputDefinitions() {
  return [
    ["go", toolchain.outputs.go],
    ["typescript", toolchain.outputs.typescript],
    ["rust", toolchain.outputs.rust],
    ["swift", toolchain.outputs.swift],
    ["kotlinJava", toolchain.outputs.kotlin.javaMessages],
    ["kotlinDsl", toolchain.outputs.kotlin.kotlinDsl],
  ];
}

function verifyGeneratedOutput(generatedRoot, formal) {
  for (const [name, output] of outputDefinitions()) {
    const outputDirectory = join(generatedRoot, output);
    const allFiles = filesBelow(outputDirectory);
    if (allFiles.length === 0) throw new Error(`${name} generator produced no files`);
    const combined = allFiles.map((path) => readFileSync(path, "utf8")).join("\n");
    if (formal && combined.includes("THREADLINE_PROTOCOL_STUB")) throw new Error(`${name} output contains a protocol-stub marker in a formal run`);
    if (!formal) continue;

    const checks = toolchain.generationChecks[name];
    if (!Number.isSafeInteger(checks.fileCount) || checks.fileCount <= 0 || allFiles.length !== checks.fileCount) {
      throw new Error(`${name} generated file count mismatch: expected ${checks.fileCount}; got ${allFiles.length}`);
    }
    assertCanonicalDigest(checks.treeSha256, 64, `${name} generated treeSha256`);
    const actualTreeSha256 = canonicalTreeDigest(walkExactTree(outputDirectory));
    if (actualTreeSha256 !== checks.treeSha256) {
      throw new Error(`${name} generated exact tree mismatch: expected ${checks.treeSha256}; got ${actualTreeSha256}`);
    }
    const sourceFiles = allFiles.filter((path) => checks.extensions.some((extension) => path.endsWith(extension)));
    if (sourceFiles.length === 0) throw new Error(`${name} produced no source with an expected extension`);
    for (const path of sourceFiles) {
      const source = readFileSync(path, "utf8");
      if (!checks.signatureRegexAny.some((signature) => new RegExp(signature, "su").test(source))) {
        throw new Error(`${name} source lacks an accepted real-generator signature: ${path}`);
      }
    }
    for (const structure of checks.structureRegex) {
      if (!new RegExp(structure, "su").test(combined)) throw new Error(`${name} output lacks expected merged-contract structure: ${structure}`);
    }
  }
}

function compileKotlinOutput(generatedRoot, temporaryRoot, tools, artifacts, environment) {
  const javaClasses = join(temporaryRoot, "classes", "java");
  const kotlinClasses = join(temporaryRoot, "classes", "kotlin");
  mkdirSync(javaClasses, { recursive: true });
  mkdirSync(kotlinClasses, { recursive: true });
  const javaSources = join(generatedRoot, toolchain.outputs.kotlin.javaMessages);
  const kotlinSources = join(generatedRoot, toolchain.outputs.kotlin.kotlinDsl);
  const generatedJava = filesBelow(javaSources, ".java");
  const generatedKotlin = filesBelow(kotlinSources, ".kt");
  if (generatedJava.length === 0 || generatedKotlin.length === 0) throw new Error("Buf must generate both Java messages and Kotlin DSL wrappers");

  run(tools.javac, ["-encoding", "UTF-8", "-source", "17", "-target", "17", "-cp", artifacts.protobufJava, "-d", javaClasses, ...generatedJava], { env: environment });
  const smokeSource = join(temporaryRoot, "ThreadlineCodegenSmoke.kt");
  writeFileSync(smokeSource, [
    "import com.threadline.proto.threadline.type.v1.ErrorCode",
    "import com.threadline.proto.threadline.type.v1.errorDetail",
    "",
    "fun threadlineCodegenSmoke() {",
    "  val error = errorDetail {",
    "    code = ErrorCode.ERROR_CODE_IDEMPOTENCY_CONFLICT",
    "    reason = \"generated_contract_smoke\"",
    "    subjectId = \"subject-smoke\"",
    "    policyVersion = \"policy-smoke\"",
    "  }",
    "  check(error.code == ErrorCode.ERROR_CODE_IDEMPOTENCY_CONFLICT)",
    "  check(error.reason == \"generated_contract_smoke\")",
    "}",
    "",
  ].join("\n"));
  const compileClasspath = [javaClasses, artifacts.protobufJava, artifacts.protobufKotlin, artifacts.kotlinStdlib, artifacts.connectKotlin].join(delimiter);
  run(tools.java, ["-cp", artifacts.kotlinCompilerClasspath, "org.jetbrains.kotlin.cli.jvm.K2JVMCompiler", "-jvm-target", "17", "-no-stdlib", "-classpath", compileClasspath, "-d", kotlinClasses, ...generatedKotlin, smokeSource], { env: environment });
  return { generatedJava: generatedJava.length, generatedKotlin: generatedKotlin.length };
}

function directoryDigest(directory) {
  if (!existsSync(directory)) return "absent";
  const entries = walkExactTree(directory);
  return canonicalTreeDigest(entries);
}

function gitStatus(gitInvocation, environment) {
  return runInvocation(gitInvocation, [
    "-C", repositoryRoot,
    "-c", "core.fsmonitor=false",
    "-c", "core.hooksPath=/dev/null",
    "status", "--porcelain=v1", "--untracked-files=all",
  ], { capture: true, env: environment });
}

function assertRepositoryWriteAuthorized(gitInvocation, environment) {
  if (process.env.THREADLINE_INTEGRATION_OWNER_GENERATION !== "I_ACKNOWLEDGE_SHARED_GENERATED_SURFACES") {
    throw new Error("repository mode requires THREADLINE_INTEGRATION_OWNER_GENERATION=I_ACKNOWLEDGE_SHARED_GENERATED_SURFACES");
  }
  const status = gitStatus(gitInvocation, environment);
  if (status !== "") throw new Error(`repository mode requires a clean worktree before generation:\n${status}`);
}

export function acquireRepositoryLock() {
  const dotGit = join(repositoryRoot, ".git");
  const gitDirectory = lstatSync(dotGit).isDirectory()
    ? realpathSync(dotGit)
    : realpathSync(resolve(repositoryRoot, readFileSync(dotGit, "utf8").trim().replace(/^gitdir:\s*/u, "")));
  const path = join(gitDirectory, `threadline-codegen-${hashText(realpathSync(repositoryRoot)).slice(0, 20)}.lock`);
  let descriptor;
  try {
    descriptor = openSync(path, "wx", 0o600);
    writeFileSync(descriptor, `${process.pid}\n`, "utf8");
  } catch (error) {
    if (descriptor !== undefined) {
      closeSync(descriptor);
      if (existsSync(path)) unlinkSync(path);
    }
    throw new Error(`another cooperative Threadline codegen installation holds ${path}: ${error.message}`);
  }
  return () => {
    let closeError;
    try {
      closeSync(descriptor);
    } catch (error) {
      closeError = error;
    }
    try {
      if (existsSync(path)) unlinkSync(path);
    } catch (error) {
      closeError ??= error;
    }
    if (closeError) throw closeError;
  };
}

function assertSafeDestination(destination) {
  const repositoryReal = realpathSync(repositoryRoot);
  if (!isWithin(repositoryRoot, destination)) throw new Error(`codegen destination escapes repository: ${destination}`);
  let current = repositoryRoot;
  for (const segment of relative(repositoryRoot, destination).split(sep).filter(Boolean)) {
    current = join(current, segment);
    if (!existsSync(current)) break;
    if (lstatSync(current).isSymbolicLink()) throw new Error(`codegen destination path contains a symlink: ${current}`);
    if (!isWithin(repositoryReal, realpathSync(current))) throw new Error(`codegen destination realpath escapes repository: ${current}`);
  }
}

export function synchronizeRepositoryOutputs(generatedRoot, gitInvocation, environment, testHooks = {}) {
  const preInstallStatus = gitStatus(gitInvocation, environment);
  if (preInstallStatus !== "") throw new Error(`worktree changed during generation; refusing installation:\n${preInstallStatus}`);
  const changes = outputDefinitions().map(([name, output]) => {
    const source = join(generatedRoot, output);
    const destination = join(repositoryRoot, output);
    assertSafeDestination(destination);
    return { name, output, source, destination, before: directoryDigest(destination), after: directoryDigest(source) };
  });
  for (const change of changes) console.log(`CODEGEN_COMPARE ${change.output} ${change.before} -> ${change.after}`);

  const changed = changes.filter((change) => change.before !== change.after);
  const installed = [];
  try {
    for (const change of changed) {
      assertSafeDestination(change.destination);
      if (directoryDigest(change.destination) !== change.before) throw new Error(`${change.output} changed concurrently before installation`);
      mkdirSync(dirname(change.destination), { recursive: true });
      assertSafeDestination(change.destination);
      change.backup = `${change.destination}.threadline-codegen-backup-${process.pid}`;
      change.staging = `${change.destination}.threadline-codegen-staging-${process.pid}`;
      if (existsSync(change.backup) || existsSync(change.staging)) throw new Error(`stale codegen backup/staging path blocks generation for ${change.output}`);
      cpSync(change.source, change.staging, { recursive: true, errorOnExist: true });
      if (directoryDigest(change.staging) !== change.after) throw new Error(`${change.output} staging copy digest mismatch`);
      if (existsSync(change.destination)) renameSync(change.destination, change.backup);
      // Track the change as soon as the old destination has moved. If the
      // staging rename fails, rollback must still restore that backup.
      installed.push(change);
      testHooks.afterBackup?.(change);
      renameSync(change.staging, change.destination);
    }
  } catch (error) {
    for (const change of installed.reverse()) {
      if (existsSync(change.destination) && directoryDigest(change.destination) === change.after) {
        rmSync(change.destination, { recursive: true, force: true });
      }
      if (!existsSync(change.destination) && existsSync(change.backup)) {
        renameSync(change.backup, change.destination);
      }
    }
    const retained = changed.flatMap((change) => [change.backup, change.staging]).filter((path) => path && existsSync(path));
    const suffix = retained.length > 0 ? `; retained recovery artifacts: ${retained.join(", ")}` : "";
    throw new Error(`${error.message}${suffix}`);
  }
  for (const change of changed) if (existsSync(change.backup)) rmSync(change.backup, { recursive: true, force: true });
  console.log(`CODEGEN_SYNC synchronized ${changed.length} changed output directories using per-directory atomic renames.`);
}

function createMinimalEnvironment(tools, temporaryRoot) {
  const home = join(temporaryRoot, "home");
  const temporary = join(temporaryRoot, "tmp");
  const cache = join(temporaryRoot, "buf-cache");
  mkdirSync(home, { recursive: true, mode: 0o700 });
  mkdirSync(temporary, { recursive: true, mode: 0o700 });
  mkdirSync(cache, { recursive: true, mode: 0o700 });
  const toolDirectories = [...new Set(Object.values(tools).map(dirname))];
  if (toolDirectories.some((path) => path.includes("\0") || path.includes(delimiter))) {
    throw new Error("verified tool directories may not contain the platform PATH delimiter or NUL");
  }
  return {
    PATH: toolDirectories.join(delimiter),
    HOME: home,
    TMPDIR: temporary,
    TMP: temporary,
    TEMP: temporary,
    LANG: "C",
    LC_ALL: "C",
    TZ: "UTC",
    BUF_CACHE_DIR: cache,
    GIT_CONFIG_NOSYSTEM: "1",
    GIT_CONFIG_GLOBAL: process.platform === "win32" ? "NUL" : "/dev/null",
    GIT_OPTIONAL_LOCKS: "0",
  };
}

function main() {
  const mode = parseMode();
  assertGenerationPlan();

  const temporaryRoot = mkdtempSync(join(tmpdir(), ".threadline-codegen-private-"));
  let releaseRepositoryLock;
  try {
  const manifestPath = resolve(requiredEnvironment("THREADLINE_PROTO_TOOL_MANIFEST"));
  const expectedManifestDigest = requiredEnvironment("THREADLINE_PROTO_TOOL_MANIFEST_SHA256");
  const { manifest, tools, invocations } = verifyToolManifest(manifestPath, expectedManifestDigest, mode, join(temporaryRoot, "verified-snapshot"));
  const codegenEnvironment = createMinimalEnvironment(tools, temporaryRoot);

  if (!existsSync(process.execPath) || lstatSync(process.execPath).isSymbolicLink() || !lstatSync(process.execPath).isFile()) {
    throw new Error("approved launcher must execute a regular, non-symlink Node binary");
  }
  if (hashFile("sha256", process.execPath) !== manifest.tools.node.sha256) {
    throw new Error("running Node binary does not match the Integration manifest digest; the in-script check does not replace the approved bootstrap launcher");
  }
  assertVersionToken("Buf", run(tools.buf, ["--version"], { capture: true, env: codegenEnvironment }), toolchain.tools.buf);
  assertVersionToken("protoc", run(tools.protoc, ["--version"], { capture: true, env: codegenEnvironment }), toolchain.tools.protoc);
  for (const name of generatorTools) {
    if (versionlessGenerators.has(name)) continue;
    assertVersionToken(name, runInvocation(invocations[name], ["--version"], { capture: true, env: codegenEnvironment }), toolchain.tools[name]);
  }
  if (mode === "repository") {
    releaseRepositoryLock = acquireRepositoryLock();
    assertVersionToken("Git", runInvocation(invocations.git, ["--version"], { capture: true, env: codegenEnvironment }), toolchain.tools.git);
    assertRepositoryWriteAuthorized(invocations.git, codegenEnvironment);
  }

  if (toolchain.tools.javaRuntime !== workspaceToolchain.android.java) throw new Error("Contract JDK pin does not match toolchains.json");
  const javaSettings = run(tools.java, ["-XshowSettings:properties", "-version"], { capture: true, env: codegenEnvironment });
  const runtimeMatch = javaSettings.match(/^\s*java\.runtime\.version\s*=\s*(\S+)\s*$/mu);
  const vendorMatch = javaSettings.match(/^\s*java\.vendor\s*=\s*(.+?)\s*$/mu);
  if (runtimeMatch?.[1] !== toolchain.tools.javaRuntime) throw new Error(`Java runtime mismatch: expected ${toolchain.tools.javaRuntime}; got ${runtimeMatch?.[1] ?? "missing"}`);
  if (vendorMatch?.[1] !== toolchain.tools.javaVendor) throw new Error(`Java vendor mismatch: expected ${toolchain.tools.javaVendor}; got ${vendorMatch?.[1] ?? "missing"}`);
  assertVersionToken("javac", run(tools.javac, ["-version"], { capture: true, env: codegenEnvironment }), toolchain.tools.javaRuntime.split("+")[0]);
  if (dirname(tools.java) !== dirname(tools.javac)) throw new Error("java and javac must come from the same verified JDK bin directory");
  assertVersionToken("Node", run(tools.node, ["--version"], { capture: true, env: codegenEnvironment }), workspaceToolchain.node);
  const kotlinArtifacts = verifyKotlinArtifacts(tools, join(temporaryRoot, "verified-snapshot"), codegenEnvironment);

  const generatedRoot = join(temporaryRoot, "generated");
  run(tools.buf, formalBufArguments("lint"), { env: codegenEnvironment });
  run(tools.buf, formalBufArguments("build", ["-o", join(temporaryRoot, "descriptor.binpb")]), { env: codegenEnvironment });
  const generationTemplate = JSON.stringify(verifiedGenerationTemplate(tools, invocations));
  run(tools.buf, formalBufArguments("generate", ["--template", generationTemplate, "-o", generatedRoot]), { env: codegenEnvironment });

  const formal = mode !== "protocol-smoke";
  verifyGeneratedOutput(generatedRoot, formal);
  const compiled = compileKotlinOutput(generatedRoot, temporaryRoot, tools, kotlinArtifacts, codegenEnvironment);
  const protoCount = filesBelow(protoRoot, ".proto").length;

  if (mode === "repository") {
    synchronizeRepositoryOutputs(generatedRoot, invocations.git, codegenEnvironment);
    console.log(`Verified release codegen before repository synchronization: ${protoCount} Proto, ${compiled.generatedJava} Java, ${compiled.generatedKotlin} Kotlin files.`);
  } else if (mode === "verify-only") {
    console.log(`Verified release codegen in temporary output only: ${protoCount} Proto, ${compiled.generatedJava} Java, ${compiled.generatedKotlin} Kotlin files.`);
  } else {
    console.log(`PROTOCOL-SMOKE ONLY: plugin protocol execution and Java/Kotlin compilation passed for ${protoCount} Proto files; this is not full release-codegen evidence.`);
  }
  } finally {
    let finalizationError;
    try {
      if (releaseRepositoryLock) releaseRepositoryLock();
    } catch (error) {
      finalizationError = error;
    }
    try {
      if (process.env.THREADLINE_KEEP_CODEGEN_SMOKE !== "1") rmSync(temporaryRoot, { recursive: true, force: true });
      else console.log(`Kept codegen output at ${temporaryRoot}`);
    } catch (error) {
      finalizationError ??= error;
    }
    if (finalizationError) throw finalizationError;
  }
}

const invokedPath = process.argv[1] ? realpathSync(process.argv[1]) : "";
if (invokedPath === realpathSync(fileURLToPath(import.meta.url))) main();
