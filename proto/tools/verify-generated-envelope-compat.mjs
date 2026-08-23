import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, delimiter, dirname, join, resolve } from "node:path";

import {
  loadScopedGeneratedCompatManifest,
  oneField,
  parseWire,
  repositoryFile,
  repositoryRoot,
} from "./generated-envelope-compat/scoped-manifest.mjs";

const protoRoot = join(repositoryRoot, "proto");
const goldenRoot = join(protoRoot, "golden", "v1");
const goldenManifest = JSON.parse(readFileSync(join(goldenRoot, "manifest.json"), "utf8"));
const toolchainLock = JSON.parse(readFileSync(join(protoRoot, "toolchain.lock.json"), "utf8"));
const supportedLanguages = new Set(["go", "typescript", "rust", "kotlin", "swift"]);

function parseOptions() {
  const arguments_ = process.argv.slice(2);
  const singleArgument = (prefix) => {
    const matches = arguments_.filter((value) => value.startsWith(prefix));
    if (matches.length > 1) throw new Error(`${prefix} may be supplied only once`);
    return matches[0];
  };
  const argument = singleArgument("--languages=");
  const baselineArgument = singleArgument("--baseline=");
  const frameManifestArgument = singleArgument("--frame-manifest=");
  const allowed = new Set([argument, baselineArgument, frameManifestArgument].filter(Boolean));
  if (!argument || arguments_.some((value) => !allowed.has(value))) {
    throw new Error("usage: node proto/tools/verify-generated-envelope-compat.mjs --languages=go,typescript,rust,kotlin,swift [--baseline=<full-sha>] [--frame-manifest=<repository-relative-json>]");
  }
  const languages = argument.slice("--languages=".length).split(",").filter(Boolean);
  if (languages.length === 0 || new Set(languages).size !== languages.length || languages.some((value) => !supportedLanguages.has(value))) {
    throw new Error(`languages must be a unique subset of ${[...supportedLanguages].join(",")}`);
  }
  const baseline = baselineArgument?.slice("--baseline=".length);
  const frameManifest = frameManifestArgument?.slice("--frame-manifest=".length);
  if ((baseline && !frameManifest) || (!baseline && frameManifest)) {
    throw new Error("--baseline and --frame-manifest must be supplied together");
  }
  if (baseline && !/^[0-9a-f]{40}$/u.test(baseline)) throw new Error("--baseline must be a full Git object ID");
  return { languages, baseline, frameManifest };
}

function executable(name, candidates) {
  for (const candidate of candidates) {
    if (candidate && existsSync(candidate)) return resolve(candidate);
  }
  throw new Error(`${name} executable is required`);
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd ?? repositoryRoot,
    encoding: "utf8",
    env: options.env ?? process.env,
    stdio: options.capture ? "pipe" : "inherit",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(`${basename(command)} ${args.join(" ")} exited ${result.status}${options.capture ? `\n${result.stdout}${result.stderr}` : ""}`);
  }
  return `${result.stdout ?? ""}${result.stderr ?? ""}`.trim();
}

function exactVersion(name, actual, expected) {
  const tokens = actual.split(/\s+/u);
  if (![expected, `v${expected}`, `go${expected}`, `"${expected}"`].some((value) => tokens.includes(value))) {
    throw new Error(`${name} version mismatch: expected ${expected}; got ${actual}`);
  }
}

function collectFiles(root, extension) {
  const files = [];
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const path = join(root, entry.name);
    if (entry.isDirectory()) files.push(...collectFiles(path, extension));
    else if (entry.isFile() && path.endsWith(extension)) files.push(path);
  }
  return files.sort();
}

function sha256(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

function protobufRuntimeRoot(protocGenEs) {
  const candidates = [
    resolve(dirname(protocGenEs), "../../protobuf"),
    join(repositoryRoot, "node_modules", "@bufbuild", "protobuf"),
  ];
  return executable("@bufbuild/protobuf runtime", candidates);
}

function prepareFixtureRoot(frameManifestPath, work, baseline) {
  if (!frameManifestPath) return { root: goldenRoot, scoped: undefined };
  const loaded = loadScopedGeneratedCompatManifest(frameManifestPath, { baseline });
  const scoped = loaded.manifest;
  const root = join(work, "scoped-fixtures");
  mkdirSync(root, { recursive: true });
  for (const file of ["channel-event-envelope.golden.hex", "ciphertext-envelope.canary.hex"]) {
    cpSync(join(goldenRoot, file), join(root, file));
  }
  cpSync(repositoryFile(scoped.frame.file), join(root, "recovery-envelope.golden.hex"));
  cpSync(repositoryFile(scoped.frame.canary.file), join(root, "crypto-envelope.canary.hex"));
  return { legacyRoot: goldenRoot, root, scoped: { ...scoped, originalFields: loaded.fields } };
}

function verifyScopedSourceEncoding(buf, work, scoped) {
  if (!scoped) return;
  const knownOutput = join(work, "scoped-recovery-known.binpb");
  run(buf, [
    "convert",
    "proto",
    "--type",
    scoped.frame.contract,
    "--from",
    `${scoped.frame.sourceJson}#format=json`,
    "--to",
    `${knownOutput}#format=binpb`,
  ]);
  const expected = Buffer.concat([
    readFileSync(knownOutput),
    Buffer.from(scoped.frame.canary.hex, "hex"),
  ]);
  const actual = Buffer.from(readFileSync(repositoryFile(scoped.frame.file), "utf8").trim(), "hex");
  if (!expected.equals(actual)) throw new Error("scoped RecoveryEnvelope frame does not match its JSON source plus exact canary");
}

function verifyScopedRelayOutputs(work, prefix, scoped) {
  if (!scoped) return;
  const original = new Map(scoped.originalFields.map((field) => [field.number, field.raw]));
  for (const suffix of ["current-first", "previous-relay", "current-final", "previous-first", "current-relay", "previous-final"]) {
    const output = join(work, `${prefix}-${suffix}`, "recovery.bin");
    const fields = parseWire(readFileSync(output));
    for (const number of scoped.frame.preservedFields) {
      const actual = oneField(fields, number).raw;
      if (!actual.equals(original.get(number))) throw new Error(`${prefix} ${suffix} changed preserved RecoveryEnvelope field ${number}`);
    }
    for (const number of scoped.frame.currentOnlyFields) oneField(fields, number);
  }
}

function verifyLegacyRelayOutputs(work, prefix, fixtures) {
  if (!fixtures.scoped) return;
  for (const suffix of ["current-first", "previous-relay", "current-final", "previous-first", "current-relay", "previous-final"]) {
    const fields = parseWire(readFileSync(join(work, `${prefix}-legacy-${suffix}`, "recovery.bin")));
    for (const number of fixtures.scoped.frame.currentOnlyFields) {
      if (fields.some((field) => field.number === number)) {
        throw new Error(`${prefix} legacy ${suffix} invented current-only RecoveryEnvelope field ${number}`);
      }
    }
    if (oneField(fields, fixtures.scoped.frame.unknownFieldNumber).raw.toString("hex") !== fixtures.scoped.frame.canary.hex) {
      throw new Error(`${prefix} legacy ${suffix} changed the field-50000 canary`);
    }
  }
}

function kotlinJars(root) {
  const expected = {
    ...toolchainLock.integrity.runtimeJars,
    ...toolchainLock.integrity.kotlinCompilerClasspath,
  };
  const jars = {};
  for (const [name, metadata] of Object.entries(expected)) {
    const path = join(root, name);
    if (!existsSync(path) || sha256(path) !== metadata.sha256) {
      throw new Error(`Kotlin JAR integrity mismatch: ${name}`);
    }
    jars[name] = path;
  }
  return jars;
}

function extractProtoSnapshot(commit, destination) {
  const paths = run("git", ["ls-tree", "-r", "--name-only", commit, "--", "proto"], { capture: true })
    .split("\n")
    .filter((path) => path === "proto/buf.yaml" || path.endsWith(".proto"));
  if (!paths.includes("proto/buf.yaml") || paths.filter((path) => path.endsWith(".proto")).length === 0) {
    throw new Error(`N-1 commit ${commit} does not contain the expected proto module`);
  }
  for (const path of paths) {
    const output = join(destination, path);
    mkdirSync(dirname(output), { recursive: true });
    const content = spawnSync("git", ["show", `${commit}:${path}`], { cwd: repositoryRoot, encoding: null, stdio: "pipe" });
    if (content.error || content.status !== 0) throw new Error(`cannot extract ${commit}:${path}`);
    writeFileSync(output, content.stdout, { mode: 0o600 });
  }
}

function writeTemplate(path, plugin, output, options = [], managed) {
  const template = {
    version: "v2",
    ...(managed ? { managed } : {}),
    plugins: [{ local: plugin, out: output, ...(options.length > 0 ? { opt: options } : {}) }],
  };
  writeFileSync(path, `${JSON.stringify(template, null, 2)}\n`, { mode: 0o600 });
}

function generate(buf, source, template, output) {
  run(buf, ["generate", source, "--template", template, "-o", output]);
}

function tscPath() {
  const explicit = process.env.THREADLINE_TSC;
  if (explicit) return executable("TypeScript compiler", [explicit]);
  const pnpmRoot = join(repositoryRoot, "node_modules", ".pnpm");
  const candidates = existsSync(pnpmRoot)
    ? readdirSync(pnpmRoot)
      .filter((name) => name === "typescript@5.4.5")
      .map((name) => join(pnpmRoot, name, "node_modules", "typescript", "bin", "tsc"))
    : [];
  return executable("TypeScript compiler 5.4.5", candidates);
}

function compileTypescript(node, tsc, source, output, work, protobufRuntime) {
  const packageMetadata = JSON.parse(readFileSync(join(protobufRuntime, "package.json"), "utf8"));
  if (packageMetadata.name !== "@bufbuild/protobuf" || packageMetadata.version !== "2.14.0") {
    throw new Error("TypeScript compatibility runtime must be @bufbuild/protobuf 2.14.0");
  }
  for (const root of [source, output]) {
    const destination = join(root, "node_modules", "@bufbuild", "protobuf");
    mkdirSync(dirname(destination), { recursive: true });
    cpSync(protobufRuntime, destination, { recursive: true });
  }
  const config = join(work, `tsconfig-${basename(source)}.json`);
  writeFileSync(config, `${JSON.stringify({
    compilerOptions: {
      target: "ES2022",
      module: "NodeNext",
      moduleResolution: "NodeNext",
      rootDir: source,
      outDir: output,
      skipLibCheck: true,
      strict: true,
    },
    include: [`${source}/**/*.ts`],
  }, null, 2)}\n`, { mode: 0o600 });
  run(node, [tsc, "-p", config]);
}

function exchange(command, prefix, currentRoot, previousRoot, work, fixtureRoot) {
  const currentFirst = join(work, `${prefix}-current-first`);
  const previousRelay = join(work, `${prefix}-previous-relay`);
  const currentFinal = join(work, `${prefix}-current-final`);
  run(command[0], [...command.slice(1), "produce", currentRoot, fixtureRoot, fixtureRoot, currentFirst, "", `${prefix}-current-1`]);
  run(command[0], [...command.slice(1), "relay", previousRoot, fixtureRoot, currentFirst, previousRelay, `${prefix}-current-1`, `${prefix}-n-minus-one-1`]);
  run(command[0], [...command.slice(1), "relay", currentRoot, fixtureRoot, previousRelay, currentFinal, `${prefix}-n-minus-one-1`, `${prefix}-current-2`]);
  run(command[0], [...command.slice(1), "consume", previousRoot, fixtureRoot, currentFinal, "", `${prefix}-current-2`, ""]);

  const previousFirst = join(work, `${prefix}-previous-first`);
  const currentRelay = join(work, `${prefix}-current-relay`);
  const previousFinal = join(work, `${prefix}-previous-final`);
  run(command[0], [...command.slice(1), "produce", previousRoot, fixtureRoot, fixtureRoot, previousFirst, "", `${prefix}-n-minus-one-2`]);
  run(command[0], [...command.slice(1), "relay", currentRoot, fixtureRoot, previousFirst, currentRelay, `${prefix}-n-minus-one-2`, `${prefix}-current-3`]);
  run(command[0], [...command.slice(1), "relay", previousRoot, fixtureRoot, currentRelay, previousFinal, `${prefix}-current-3`, `${prefix}-n-minus-one-3`]);
  run(command[0], [...command.slice(1), "consume", currentRoot, fixtureRoot, previousFinal, "", `${prefix}-n-minus-one-3`, ""]);
}

function exchangeVariants(invoke, current, previous, prefix, work, fixtureRoot) {
  const currentFirst = join(work, `${prefix}-current-first`);
  const previousRelay = join(work, `${prefix}-previous-relay`);
  const currentFinal = join(work, `${prefix}-current-final`);
  invoke(current, ["produce", fixtureRoot, fixtureRoot, currentFirst, "", `${prefix}-current-1`]);
  invoke(previous, ["relay", fixtureRoot, currentFirst, previousRelay, `${prefix}-current-1`, `${prefix}-n-minus-one-1`]);
  invoke(current, ["relay", fixtureRoot, previousRelay, currentFinal, `${prefix}-n-minus-one-1`, `${prefix}-current-2`]);
  invoke(previous, ["consume", fixtureRoot, currentFinal, "", `${prefix}-current-2`, ""]);

  const previousFirst = join(work, `${prefix}-previous-first`);
  const currentRelay = join(work, `${prefix}-current-relay`);
  const previousFinal = join(work, `${prefix}-previous-final`);
  invoke(previous, ["produce", fixtureRoot, fixtureRoot, previousFirst, "", `${prefix}-n-minus-one-2`]);
  invoke(current, ["relay", fixtureRoot, previousFirst, currentRelay, `${prefix}-n-minus-one-2`, `${prefix}-current-3`]);
  invoke(previous, ["relay", fixtureRoot, currentRelay, previousFinal, `${prefix}-current-3`, `${prefix}-n-minus-one-3`]);
  invoke(current, ["consume", fixtureRoot, previousFinal, "", `${prefix}-n-minus-one-3`, ""]);
}

function verifyTypescript(tools, sources, work, fixtures) {
  const template = join(work, "typescript-template.json");
  writeTemplate(template, tools.protocGenEs, "typescript", ["target=ts", "import_extension=.js"]);
  const currentGenerated = join(work, "typescript-current-source");
  const previousGenerated = join(work, "typescript-previous-source");
  generate(tools.buf, sources.current, template, currentGenerated);
  generate(tools.buf, sources.previous, template, previousGenerated);
  const currentJs = join(work, "typescript-current-js");
  const previousJs = join(work, "typescript-previous-js");
  const protobufRuntime = protobufRuntimeRoot(tools.protocGenEs);
  compileTypescript(tools.node, tools.tsc, join(currentGenerated, "typescript"), currentJs, work, protobufRuntime);
  compileTypescript(tools.node, tools.tsc, join(previousGenerated, "typescript"), previousJs, work, protobufRuntime);
  const runner = join(work, "typescript-runner.mjs");
  cpSync(join(protoRoot, "tools", "generated-envelope-compat", "typescript.mjs"), runner);
  const runnerRuntime = join(work, "node_modules", "@bufbuild", "protobuf");
  mkdirSync(dirname(runnerRuntime), { recursive: true });
  cpSync(protobufRuntime, runnerRuntime, { recursive: true });
  exchange([tools.node, runner], "typescript", currentJs, previousJs, work, fixtures.root);
  verifyScopedRelayOutputs(work, "typescript", fixtures.scoped);
  if (fixtures.legacyRoot) {
    exchange([tools.node, runner], "typescript-legacy", currentJs, previousJs, work, fixtures.legacyRoot);
    verifyLegacyRelayOutputs(work, "typescript", fixtures);
  }
}

function writeGoModule(root) {
  writeFileSync(join(root, "go.mod"), [
    "module threadline.compat/generated",
    "",
    "go 1.26.0",
    "",
    "require google.golang.org/protobuf v1.36.11",
    "",
  ].join("\n"), { mode: 0o600 });
  const runner = join(protoRoot, "tools", "generated-envelope-compat", "go", "main.go");
  const destination = join(root, "cmd", "compat", "main.go");
  mkdirSync(dirname(destination), { recursive: true });
  cpSync(runner, destination);
}

function verifyGo(tools, sources, work, fixtures) {
  const template = join(work, "go-template.json");
  writeTemplate(template, tools.protocGenGo, "go", ["paths=source_relative"], {
    enabled: true,
    override: [{ file_option: "go_package_prefix", value: "threadline.compat/generated" }],
  });
  const current = join(work, "go-current");
  const previous = join(work, "go-previous");
  generate(tools.buf, sources.current, template, current);
  generate(tools.buf, sources.previous, template, previous);
  const currentModule = join(current, "go");
  const previousModule = join(previous, "go");
  writeGoModule(currentModule);
  writeGoModule(previousModule);
  const goEnvironment = {
    ...process.env,
    GOWORK: "off",
    GOTOOLCHAIN: "local",
    GOPATH: process.env.THREADLINE_GO_PATH || join(work, "gopath"),
    GOCACHE: process.env.THREADLINE_GO_CACHE || join(work, "go-cache"),
    GOMODCACHE: process.env.THREADLINE_GO_MOD_CACHE || join(work, "go-mod-cache"),
    ...(process.env.THREADLINE_ENVELOPE_COMPAT_OFFLINE === "1" ? { GOPROXY: "off", GOSUMDB: "off" } : {}),
  };
  run(tools.go, ["mod", "download"], { cwd: currentModule, env: goEnvironment });
  run(tools.go, ["mod", "tidy"], { cwd: currentModule, env: goEnvironment });
  run(tools.go, ["mod", "download"], { cwd: previousModule, env: goEnvironment });
  run(tools.go, ["mod", "tidy"], { cwd: previousModule, env: goEnvironment });
  const invoke = (module, args) => run(tools.go, ["run", "./cmd/compat", ...args], { cwd: module, env: goEnvironment });

  exchangeVariants(invoke, currentModule, previousModule, "go", work, fixtures.root);
  verifyScopedRelayOutputs(work, "go", fixtures.scoped);
  if (fixtures.legacyRoot) {
    exchangeVariants(invoke, currentModule, previousModule, "go-legacy", work, fixtures.legacyRoot);
    verifyLegacyRelayOutputs(work, "go", fixtures);
  }
}

function writeKotlinTemplate(path, protoc) {
  const template = {
    version: "v2",
    managed: {
      enabled: true,
      override: [{ file_option: "java_package_prefix", value: "com.threadline.proto" }],
    },
    plugins: [
      { protoc_builtin: "java", protoc_path: protoc, out: "java" },
      { protoc_builtin: "kotlin", protoc_path: protoc, out: "kotlin" },
    ],
  };
  writeFileSync(path, `${JSON.stringify(template, null, 2)}\n`, { mode: 0o600 });
}

function compileKotlinVariant(tools, generatedRoot, outputRoot) {
  const javaSource = join(generatedRoot, "java");
  const kotlinSource = join(generatedRoot, "kotlin");
  const javaClasses = join(outputRoot, "java-classes");
  const kotlinClasses = join(outputRoot, "kotlin-classes");
  mkdirSync(javaClasses, { recursive: true });
  mkdirSync(kotlinClasses, { recursive: true });
  const protobufJava = tools.jars["protobuf-java-4.35.1.jar"];
  const protobufKotlin = tools.jars["protobuf-kotlin-4.35.1.jar"];
  const kotlinStdlib = tools.jars["kotlin-stdlib-2.4.10.jar"];
  const runtimeClasspath = [protobufJava, protobufKotlin, kotlinStdlib].join(delimiter);
  const javaSources = collectFiles(javaSource, ".java");
  const kotlinSources = collectFiles(kotlinSource, ".kt");
  if (javaSources.length === 0 || kotlinSources.length === 0) {
    throw new Error("protoc did not generate both Java messages and Kotlin DSL sources");
  }
  run(tools.javac, ["-encoding", "UTF-8", "-cp", protobufJava, "-d", javaClasses, ...javaSources]);
  const compilerClasspath = Object.keys(toolchainLock.integrity.kotlinCompilerClasspath)
    .map((name) => tools.jars[name])
    .join(delimiter);
  const runner = join(protoRoot, "tools", "generated-envelope-compat", "kotlin", "Main.kt");
  run(tools.java, [
    "-cp", compilerClasspath,
    "org.jetbrains.kotlin.cli.jvm.K2JVMCompiler",
    "-no-stdlib",
    "-no-reflect",
    "-jvm-target", "17",
    "-classpath", [javaClasses, runtimeClasspath].join(delimiter),
    "-d", kotlinClasses,
    ...kotlinSources,
    runner,
  ]);
  return {
    command: [tools.java, "-cp", [kotlinClasses, javaClasses, runtimeClasspath].join(delimiter), "MainKt"],
  };
}

function verifyKotlin(tools, sources, work, fixtures) {
  const template = join(work, "kotlin-template.json");
  writeKotlinTemplate(template, tools.protoc);
  const currentGenerated = join(work, "kotlin-current-generated");
  const previousGenerated = join(work, "kotlin-previous-generated");
  generate(tools.buf, sources.current, template, currentGenerated);
  generate(tools.buf, sources.previous, template, previousGenerated);
  const current = compileKotlinVariant(tools, currentGenerated, join(work, "kotlin-current-build"));
  const previous = compileKotlinVariant(tools, previousGenerated, join(work, "kotlin-previous-build"));

  const invoke = (variant, args) => run(variant.command[0], [...variant.command.slice(1), ...args]);
  exchangeVariants(invoke, current, previous, "kotlin", work, fixtures.root);
  verifyScopedRelayOutputs(work, "kotlin", fixtures.scoped);
  if (fixtures.legacyRoot) {
    exchangeVariants(invoke, current, previous, "kotlin-legacy", work, fixtures.legacyRoot);
    verifyLegacyRelayOutputs(work, "kotlin", fixtures);
  }
}

function writeSwiftTemplate(path, plugin) {
  writeTemplate(path, plugin, "swift", ["Visibility=Public", "FileNaming=PathToUnderscores"]);
}

function extractSwiftProtobuf(archive, destination) {
  const expected = toolchainLock.integrity.swiftProtobufSourceArchive.sha256;
  if (sha256(archive) !== expected) throw new Error("SwiftProtobuf 1.38.1 source archive integrity mismatch");
  mkdirSync(destination, { recursive: true });
  run("/usr/bin/tar", ["-xzf", archive, "-C", destination, "--strip-components=1"]);
}

function swiftEnvironment(work) {
  const home = join(work, "swift-home");
  const moduleCache = join(work, "swift-module-cache");
  mkdirSync(home, { recursive: true });
  mkdirSync(moduleCache, { recursive: true });
  return {
    ...process.env,
    HOME: home,
    CLANG_MODULE_CACHE_PATH: moduleCache,
    SWIFTPM_MODULECACHE_OVERRIDE: moduleCache,
  };
}

function writeSwiftPackage(root, sourceRoot, currentGenerated, previousGenerated) {
  const packageFile = `// swift-tools-version: 6.2\nimport PackageDescription\n\nlet package = Package(\n  name: "ThreadlineEnvelopeCompat",\n  platforms: [.macOS(.v13)],\n  dependencies: [.package(name: "swift-protobuf", path: ${JSON.stringify(sourceRoot)})],\n  targets: [\n    .executableTarget(name: "CurrentCompat", dependencies: [.product(name: "SwiftProtobuf", package: "swift-protobuf")]),\n    .executableTarget(name: "PreviousCompat", dependencies: [.product(name: "SwiftProtobuf", package: "swift-protobuf")]),\n  ]\n)\n`;
  writeFileSync(join(root, "Package.swift"), packageFile, { mode: 0o600 });
  const runner = join(protoRoot, "tools", "generated-envelope-compat", "swift", "main.swift");
  for (const [target, generated] of [["CurrentCompat", currentGenerated], ["PreviousCompat", previousGenerated]]) {
    const destination = join(root, "Sources", target);
    mkdirSync(destination, { recursive: true });
    for (const source of collectFiles(generated, ".swift")) cpSync(source, join(destination, basename(source)));
    cpSync(runner, join(destination, "main.swift"));
  }
}

function verifySwift(tools, sources, work, fixtures) {
  const sourceRoot = join(work, "swift-protobuf-source");
  extractSwiftProtobuf(tools.sourceArchive, sourceRoot);
  const swiftEnvironmentVariables = swiftEnvironment(work);
  const pluginBuild = join(work, "swift-plugin-build");
  run(tools.swift, [
    "build", "--disable-sandbox", "--configuration", "release", "--product", "protoc-gen-swift",
    "--package-path", sourceRoot, "--scratch-path", pluginBuild, "--sdk", tools.sdk,
  ], { env: swiftEnvironmentVariables });
  const plugin = executable("protoc-gen-swift", [join(pluginBuild, "arm64-apple-macosx", "release", "protoc-gen-swift")]);
  exactVersion("protoc-gen-swift", run(plugin, ["--version"], { capture: true }), "1.38.1");
  const template = join(work, "swift-template.json");
  writeSwiftTemplate(template, plugin);
  const currentGenerated = join(work, "swift-current-generated");
  const previousGenerated = join(work, "swift-previous-generated");
  generate(tools.buf, sources.current, template, currentGenerated);
  generate(tools.buf, sources.previous, template, previousGenerated);
  const packageRoot = join(work, "swift-compat-package");
  mkdirSync(packageRoot, { recursive: true });
  writeSwiftPackage(
    packageRoot,
    sourceRoot,
    join(currentGenerated, "swift"),
    join(previousGenerated, "swift"),
  );
  const packageBuild = join(work, "swift-compat-build");
  for (const product of ["CurrentCompat", "PreviousCompat"]) {
    run(tools.swift, [
      "build", "--disable-sandbox", "--configuration", "release", "--product", product,
      "--package-path", packageRoot, "--scratch-path", packageBuild, "--sdk", tools.sdk,
    ], { env: swiftEnvironmentVariables });
  }
  const current = [join(packageBuild, "arm64-apple-macosx", "release", "CurrentCompat")];
  const previous = [join(packageBuild, "arm64-apple-macosx", "release", "PreviousCompat")];
  const invoke = (command, args) => run(command[0], args);

  exchangeVariants(invoke, current, previous, "swift", work, fixtures.root);
  verifyScopedRelayOutputs(work, "swift", fixtures.scoped);
  if (fixtures.legacyRoot) {
    exchangeVariants(invoke, current, previous, "swift-legacy", work, fixtures.legacyRoot);
    verifyLegacyRelayOutputs(work, "swift", fixtures);
  }
}

function main() {
  const options = parseOptions();
  const { languages } = options;
  const baseline = options.baseline ?? goldenManifest.compatibilityEvidence?.nMinusOneCommit;
  if (typeof baseline !== "string" || !/^[0-9a-f]{40}$/u.test(baseline)) {
    throw new Error("manifest compatibilityEvidence.nMinusOneCommit must pin a full Git commit");
  }
  const node = executable("Node", [process.env.THREADLINE_NODE, process.execPath]);
  const buf = executable("Buf", [process.env.THREADLINE_BUF, join(repositoryRoot, "node_modules", ".bin", "buf")]);
  exactVersion("Buf", run(buf, ["--version"], { capture: true }), "1.72.0");
  exactVersion("Node", run(node, ["--version"], { capture: true }), "24.19.0");
  const work = mkdtempSync(join(repositoryRoot, ".threadline-envelope-compat-"));
  try {
    const fixtures = prepareFixtureRoot(options.frameManifest, work, baseline);
    verifyScopedSourceEncoding(buf, work, fixtures.scoped);
    const previousRoot = join(work, "n-minus-one");
    extractProtoSnapshot(baseline, previousRoot);
    const sources = { current: protoRoot, previous: join(previousRoot, "proto") };
    if (languages.includes("typescript")) {
      const protocGenEs = executable("protoc-gen-es", [process.env.THREADLINE_PROTOC_GEN_ES, join(repositoryRoot, "node_modules", ".bin", "protoc-gen-es")]);
      exactVersion("protoc-gen-es", run(protocGenEs, ["--version"], { capture: true }), "2.14.0");
      verifyTypescript({ buf, node, protocGenEs, tsc: tscPath() }, sources, work, fixtures);
    }
    if (languages.includes("go")) {
      const go = executable("Go", [process.env.THREADLINE_GO]);
      const protocGenGo = executable("protoc-gen-go", [process.env.THREADLINE_PROTOC_GEN_GO]);
      exactVersion("Go", run(go, ["version"], { capture: true }), "1.26.5");
      exactVersion("protoc-gen-go", run(protocGenGo, ["--version"], { capture: true }), "1.36.11");
      verifyGo({ buf, go, protocGenGo }, sources, work, fixtures);
    }
    if (languages.includes("rust")) {
      const cargo = executable("Cargo", [process.env.THREADLINE_CARGO]);
      const rustVerifier = join(protoRoot, "tools", "verify-rust-envelope-preservation.mjs");
      const rustMode = process.env.THREADLINE_ENVELOPE_COMPAT_OFFLINE === "1" ? "--offline" : "--connected";
      run(node, [
        rustVerifier,
        rustMode,
        `--baseline=${baseline}`,
        `--frame-root=${fixtures.root}`,
        ...(fixtures.scoped ? [
          "--require-current-recovery-bindings",
          `--legacy-recovery-frame=${repositoryFile(fixtures.scoped.historicalT014RecoveryFrame.file)}`,
        ] : []),
      ], {
        env: { ...process.env, THREADLINE_BUF: buf, THREADLINE_CARGO: cargo },
      });
    }
    if (languages.includes("kotlin")) {
      const javaHome = resolve(process.env.THREADLINE_JAVA_HOME || "");
      if (!process.env.THREADLINE_JAVA_HOME) throw new Error("THREADLINE_JAVA_HOME is required for Kotlin verification");
      const java = executable("Java", [join(javaHome, "bin", "java")]);
      const javac = executable("javac", [join(javaHome, "bin", "javac")]);
      const protoc = executable("protoc", [process.env.THREADLINE_PROTOC]);
      const jarRoot = resolve(process.env.THREADLINE_KOTLIN_JARS || "");
      if (!process.env.THREADLINE_KOTLIN_JARS) throw new Error("THREADLINE_KOTLIN_JARS is required for Kotlin verification");
      exactVersion("Java", run(java, ["-version"], { capture: true }), "17.0.19");
      exactVersion("javac", run(javac, ["-version"], { capture: true }), "17.0.19");
      exactVersion("protoc", run(protoc, ["--version"], { capture: true }), "35.1");
      verifyKotlin({ buf, java, javac, protoc, jars: kotlinJars(jarRoot) }, sources, work, fixtures);
    }
    if (languages.includes("swift")) {
      const swift = executable("Swift", [process.env.THREADLINE_SWIFT]);
      const sdk = resolve(process.env.THREADLINE_SWIFT_SDK || "");
      const sourceArchive = executable("SwiftProtobuf source archive", [process.env.THREADLINE_SWIFT_PROTOBUF_ARCHIVE]);
      if (!process.env.THREADLINE_SWIFT_SDK || !existsSync(sdk)) throw new Error("THREADLINE_SWIFT_SDK is required for Swift verification");
      exactVersion("Swift", run(swift, ["--version"], { capture: true }), "6.3.2");
      verifySwift({ buf, swift, sdk, sourceArchive }, sources, work, fixtures);
    }
    console.log(`Generated-envelope compatibility passed: ${languages.join(", ")} against N-1 ${baseline}.`);
  } finally {
    if (process.env.THREADLINE_KEEP_ENVELOPE_COMPAT !== "1") rmSync(work, { recursive: true, force: true });
    else console.log(`Kept generated-envelope compatibility work at ${work}`);
  }
}

main();
