import { existsSync, lstatSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";

const workRoot = process.env.THREADLINE_FORMAL_WORK_ROOT ? resolve(process.env.THREADLINE_FORMAL_WORK_ROOT) : "";
const output = process.env.THREADLINE_CODEGEN_BUNDLE_SPEC ? resolve(process.env.THREADLINE_CODEGEN_BUNDLE_SPEC) : "";
const runnerImagesSha = process.env.THREADLINE_RUNNER_IMAGES_SHA ?? "";
if (!workRoot || !output || !/^[0-9a-f]{40}$/u.test(runnerImagesSha)) {
  throw new Error("THREADLINE_FORMAL_WORK_ROOT, THREADLINE_CODEGEN_BUNDLE_SPEC, and a 40-hex THREADLINE_RUNNER_IMAGES_SHA are required");
}

const source = (name) => join(workRoot, "sources", name);
const closure = (name) => join(workRoot, "closures", name);
const requiredInputs = [
  source("buf-Darwin-arm64.tar.gz"),
  source("protoc-35.1-osx-aarch_64.zip"),
  source("node-v24.19.0-darwin-arm64.tar.gz"),
  source("OpenJDK17U-jdk_aarch64_mac_hotspot_17.0.19_10.tar.gz"),
  source("protoc-gen-go.v1.36.11.darwin.arm64.tar.gz"),
  source("connect-go-1.20.0.tar.gz"),
  source("go1.26.5.darwin-arm64.tar.gz"),
  source("go-release.json"),
  source("protoc-gen-es-2.14.0.tgz"),
  source("protobuf-2.14.0.tgz"),
  source("protoplugin-2.14.0.tgz"),
  source("vfs-1.6.4.tgz"),
  source("typescript-5.4.5.tgz"),
  source("debug-4.4.3.tgz"),
  source("ms-2.1.3.tgz"),
  source("protoc-gen-prost-0.5.0.tar.gz"),
  source("rust-1.97.1-aarch64-apple-darwin.tar.xz"),
  source("rust-1.97.1-aarch64-apple-darwin.tar.xz.sha256"),
  source("swift-protobuf-1.38.1.tar.gz"),
  source("protoc-gen-connect-swift.tar.gz"),
  source("protoc-gen-connect-kotlin-0.9.0.jar"),
];
for (const path of requiredInputs) {
  if (!existsSync(path) || lstatSync(path).isSymbolicLink() || !lstatSync(path).isFile()) throw new Error(`formal source input is missing: ${path}`);
}

const sources = {
  buf: { kind: "official-binary", path: source("buf-Darwin-arm64.tar.gz"), url: "https://github.com/bufbuild/buf/releases/download/v1.72.0/buf-Darwin-arm64.tar.gz" },
  protoc: { kind: "official-package", path: source("protoc-35.1-osx-aarch_64.zip"), url: "https://github.com/protocolbuffers/protobuf/releases/download/v35.1/protoc-35.1-osx-aarch_64.zip" },
  node: { kind: "official-package", path: source("node-v24.19.0-darwin-arm64.tar.gz"), url: "https://nodejs.org/dist/v24.19.0/node-v24.19.0-darwin-arm64.tar.gz" },
  jdk: { kind: "official-package", path: source("OpenJDK17U-jdk_aarch64_mac_hotspot_17.0.19_10.tar.gz"), url: "https://github.com/adoptium/temurin17-binaries/releases/download/jdk-17.0.19%2B10/OpenJDK17U-jdk_aarch64_mac_hotspot_17.0.19_10.tar.gz" },
  "protoc-gen-go": { kind: "official-binary", path: source("protoc-gen-go.v1.36.11.darwin.arm64.tar.gz"), url: "https://github.com/protocolbuffers/protobuf-go/releases/download/v1.36.11/protoc-gen-go.v1.36.11.darwin.arm64.tar.gz" },
  "connect-go-source": { kind: "source-archive", path: source("connect-go-1.20.0.tar.gz"), url: "https://github.com/connectrpc/connect-go/archive/refs/tags/v1.20.0.tar.gz" },
  "go-builder": {
    kind: "builder-toolchain",
    path: source("go1.26.5.darwin-arm64.tar.gz"),
    url: "https://go.dev/dl/go1.26.5.darwin-arm64.tar.gz",
    authentication: { kind: "go-distribution-json", file: source("go-release.json"), url: "https://go.dev/dl/?mode=json&include=all" },
  },
  "protoc-gen-es": { kind: "official-package", path: source("protoc-gen-es-2.14.0.tgz"), url: "https://registry.npmjs.org/@bufbuild/protoc-gen-es/-/protoc-gen-es-2.14.0.tgz" },
  protobuf: { kind: "official-package", path: source("protobuf-2.14.0.tgz"), url: "https://registry.npmjs.org/@bufbuild/protobuf/-/protobuf-2.14.0.tgz" },
  protoplugin: { kind: "official-package", path: source("protoplugin-2.14.0.tgz"), url: "https://registry.npmjs.org/@bufbuild/protoplugin/-/protoplugin-2.14.0.tgz" },
  vfs: { kind: "official-package", path: source("vfs-1.6.4.tgz"), url: "https://registry.npmjs.org/@typescript/vfs/-/vfs-1.6.4.tgz" },
  typescript: { kind: "official-package", path: source("typescript-5.4.5.tgz"), url: "https://registry.npmjs.org/typescript/-/typescript-5.4.5.tgz" },
  debug: { kind: "official-package", path: source("debug-4.4.3.tgz"), url: "https://registry.npmjs.org/debug/-/debug-4.4.3.tgz" },
  ms: { kind: "official-package", path: source("ms-2.1.3.tgz"), url: "https://registry.npmjs.org/ms/-/ms-2.1.3.tgz" },
  "prost-source": { kind: "source-archive", path: source("protoc-gen-prost-0.5.0.tar.gz"), url: "https://github.com/neoeinstein/protoc-gen-prost/archive/refs/tags/protoc-gen-prost-v0.5.0.tar.gz" },
  "rust-builder": {
    kind: "builder-toolchain",
    path: source("rust-1.97.1-aarch64-apple-darwin.tar.xz"),
    url: "https://static.rust-lang.org/dist/rust-1.97.1-aarch64-apple-darwin.tar.xz",
    authentication: { kind: "rust-distribution-sha256", file: source("rust-1.97.1-aarch64-apple-darwin.tar.xz.sha256"), url: "https://static.rust-lang.org/dist/rust-1.97.1-aarch64-apple-darwin.tar.xz.sha256" },
  },
  "swift-source": { kind: "source-archive", path: source("swift-protobuf-1.38.1.tar.gz"), url: "https://github.com/apple/swift-protobuf/archive/refs/tags/1.38.1.tar.gz" },
  "xcode-builder": {
    kind: "host-builder-toolchain",
    path: "/Applications/Xcode_26.6.app",
    url: `https://github.com/actions/runner-images/blob/${runnerImagesSha}/images/macos/macos-26-arm64-Readme.md`,
    authentication: { kind: "apple-xcode-gatekeeper", bundleIdentifier: "com.apple.dt.Xcode", version: "26.6", build: "17F113", swiftVersion: "6.3", sdkVersion: "26.5" },
  },
  "connect-swift": { kind: "official-binary", path: source("protoc-gen-connect-swift.tar.gz"), url: "https://github.com/connectrpc/connect-swift/releases/download/1.2.3/protoc-gen-connect-swift.tar.gz" },
  "connect-kotlin": { kind: "official-package", path: source("protoc-gen-connect-kotlin-0.9.0.jar"), url: "https://github.com/connectrpc/connect-kotlin/releases/download/v0.9.0/protoc-gen-connect-kotlin-0.9.0.jar" },
};

const esSources = ["protoc-gen-es", "protobuf", "protoplugin", "vfs", "typescript", "debug", "ms"];
const closures = {
  buf: { root: closure("buf"), sources: ["buf"] },
  protoc: { root: closure("protoc"), sources: ["protoc"] },
  node: { root: closure("node"), sources: ["node"] },
  jdk: { root: closure("jdk"), sources: ["jdk"] },
  "protoc-gen-go": { root: closure("protoc-gen-go"), sources: ["protoc-gen-go"] },
  "connect-go": { root: closure("connect-go"), sources: ["connect-go-source", "go-builder"] },
  "protoc-gen-es": { root: closure("protoc-gen-es"), sources: esSources },
  "protoc-gen-prost": { root: closure("protoc-gen-prost"), sources: ["prost-source", "rust-builder"] },
  "protoc-gen-prost-crate": { root: closure("protoc-gen-prost-crate"), sources: ["prost-source", "rust-builder"] },
  "protoc-gen-swift": { root: closure("protoc-gen-swift"), sources: ["swift-source", "xcode-builder"] },
  "connect-swift": { root: closure("connect-swift"), sources: ["connect-swift"] },
  "connect-kotlin": { root: closure("connect-kotlin"), sources: ["connect-kotlin"] },
};

const official = (path, closureName, sourceNames, invocation = "native") => ({ path, closure: closureName, provenance: { kind: "official-package", sources: sourceNames }, invocation });
const officialBinary = (path, closureName, sourceNames) => ({ path, closure: closureName, provenance: { kind: "official-binary", sources: sourceNames }, invocation: "native" });
const sourceBuilt = (path, closureName, sourceName, builders, buildCommand) => ({
  path,
  closure: closureName,
  provenance: { kind: "source-built", source: sourceName, builders, buildCommand, reproducibility: "single-build-output-verified" },
  invocation: "native",
});

const tools = {
  buf: officialBinary(join(closure("buf"), "buf"), "buf", ["buf"]),
  protoc: official(join(closure("protoc"), "protoc"), "protoc", ["protoc"]),
  "protoc-gen-go": officialBinary(join(closure("protoc-gen-go"), "protoc-gen-go"), "protoc-gen-go", ["protoc-gen-go"]),
  "protoc-gen-connect-go": sourceBuilt(join(closure("connect-go"), "protoc-gen-connect-go"), "connect-go", "connect-go-source", ["go-builder"], "GOTOOLCHAIN=local CGO_ENABLED=0 go build -trimpath -ldflags=-buildid= -o protoc-gen-connect-go ./cmd/protoc-gen-connect-go"),
  "protoc-gen-es": { ...official(join(closure("protoc-gen-es"), "node_modules", "@bufbuild", "protoc-gen-es", "bin", "protoc-gen-es"), "protoc-gen-es", esSources, "verified-node") },
  "protoc-gen-prost": sourceBuilt(join(closure("protoc-gen-prost"), "protoc-gen-prost"), "protoc-gen-prost", "prost-source", ["rust-builder"], "cargo build --release --locked -p protoc-gen-prost -p protoc-gen-prost-crate"),
  "protoc-gen-prost-crate": sourceBuilt(join(closure("protoc-gen-prost-crate"), "protoc-gen-prost-crate"), "protoc-gen-prost-crate", "prost-source", ["rust-builder"], "cargo build --release --locked -p protoc-gen-prost -p protoc-gen-prost-crate"),
  "protoc-gen-swift": sourceBuilt(join(closure("protoc-gen-swift"), "protoc-gen-swift"), "protoc-gen-swift", "swift-source", ["xcode-builder"], "DEVELOPER_DIR=/Applications/Xcode_26.6.app/Contents/Developer /usr/bin/xcrun swift build --configuration release --product protoc-gen-swift --scratch-path <private>"),
  "protoc-gen-connect-swift": officialBinary(join(closure("connect-swift"), "protoc-gen-connect-swift"), "connect-swift", ["connect-swift"]),
  "protoc-gen-connect-kotlin": official(join(closure("connect-kotlin"), "protoc-gen-connect-kotlin.jar"), "connect-kotlin", ["connect-kotlin"], "verified-java"),
  java: official(join(closure("jdk"), "Contents", "Home", "bin", "java"), "jdk", ["jdk"]),
  javac: official(join(closure("jdk"), "Contents", "Home", "bin", "javac"), "jdk", ["jdk"]),
  node: official(join(closure("node"), "bin", "node"), "node", ["node"]),
};

writeFileSync(output, `${JSON.stringify({ schemaVersion: 1, platform: "darwin-arm64", profile: "release", sources, closures, tools }, null, 2)}\n`);
console.log(output);
