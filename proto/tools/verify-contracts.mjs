import { createHash } from "node:crypto";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const protoRoot = fileURLToPath(new URL("../", import.meta.url));
const repositoryRoot = fileURLToPath(new URL("../../", import.meta.url));
const errors = [];

function read(path) {
  return readFileSync(path, "utf8").replaceAll("\r\n", "\n");
}

function assert(condition, message) {
  if (!condition) errors.push(message);
}

function filesBelow(directory, suffix) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) return filesBelow(path, suffix);
    return path.endsWith(suffix) ? [path] : [];
  });
}

function decodeVarint(bytes, start) {
  let value = 0;
  let shift = 0;
  for (let index = start; index < bytes.length && shift <= 49; index += 1) {
    const byte = bytes[index];
    value += (byte & 0x7f) * 2 ** shift;
    if ((byte & 0x80) === 0) return { value, next: index + 1 };
    shift += 7;
  }
  throw new Error("invalid or oversized varint");
}

const bufYaml = read(join(repositoryRoot, "buf.yaml"));
const generationYaml = read(join(repositoryRoot, "buf.gen.yaml"));
const combinedConfig = `${bufYaml}\n${generationYaml}`;
assert(!/buf\.build|\bremote:|\bdeps:|\bmodule:/u.test(combinedConfig), "Buf configuration must not use the public BSR");
assert(/breaking:\s*\n\s+use:\s*\n\s+- FILE/u.test(bufYaml), "buf.yaml must enforce FILE breaking rules");
assert(/disallow_comment_ignores:\s*true/u.test(bufYaml), "Buf lint comment ignores must remain disabled");

const toolchain = JSON.parse(read(join(protoRoot, "toolchain.lock.json")));
assert(toolchain.schemaVersion === 1, "toolchain lock schemaVersion must be 1");
assert(Object.keys(toolchain.outputs).length === 5, "exactly five language outputs must be pinned");
for (const [language, output] of Object.entries(toolchain.outputs)) {
  assert(generationYaml.includes(`out: ${output}`), `${language} output is not wired in buf.gen.yaml: ${output}`);
}
for (const plugin of ["protoc-gen-go", "protoc-gen-es", "protoc-gen-prost", "protoc-gen-swift"]) {
  assert(generationYaml.includes(`local: ${plugin}`), `${plugin} must be a local plugin`);
  assert(typeof toolchain.tools[plugin] === "string", `${plugin} version must be pinned`);
}
assert(/protoc_builtin:\s*kotlin/u.test(generationYaml), "Kotlin must use the pinned protoc built-in generator");
assert(toolchain.commands.generate === "buf generate", "the generation command must remain canonical");

for (const path of filesBelow(join(protoRoot, "threadline"), ".proto")) {
  const source = read(path);
  const packageMatch = source.match(/^package\s+([a-z0-9_.]+);/mu);
  const expectedPackage = relative(protoRoot, dirname(path)).split("/").join(".");
  assert(source.startsWith('syntax = "proto3";'), `${relative(repositoryRoot, path)} must declare proto3 syntax first`);
  assert(packageMatch?.[1] === expectedPackage, `${relative(repositoryRoot, path)} package must be ${expectedPackage}`);
  assert(/\.v[1-9][0-9]*$/u.test(expectedPackage), `${relative(repositoryRoot, path)} must use a stable version suffix`);
}

const manifestPath = join(protoRoot, "golden", "v1", "manifest.json");
const manifest = JSON.parse(read(manifestPath));
assert(manifest.schemaVersion === 1, "Golden Frame manifest schemaVersion must be 1");
assert(manifest.canaryFieldNumber === 50000, "Golden Frame unknown-field canary must remain field 50000");
assert(manifest.fixtures.length === 2, "Ciphertext and Crypto Envelope canaries are both required");

for (const fixture of manifest.fixtures) {
  const fixturePath = join(dirname(manifestPath), fixture.file);
  assert(statSync(fixturePath).isFile(), `${fixture.contract} fixture is missing`);
  const hex = read(fixturePath).trim();
  assert(/^(?:[0-9a-f]{2})+$/u.test(hex), `${fixture.contract} fixture must be canonical lowercase hex`);
  const bytes = Buffer.from(hex, "hex");
  const digest = createHash("sha256").update(bytes).digest("hex");
  assert(digest === fixture.sha256, `${fixture.contract} SHA-256 mismatch`);
  try {
    const tag = decodeVarint(bytes, 0);
    const length = decodeVarint(bytes, tag.next);
    const payload = bytes.subarray(length.next, length.next + length.value);
    assert(Math.floor(tag.value / 8) === manifest.canaryFieldNumber, `${fixture.contract} must use field 50000`);
    assert(tag.value % 8 === 2, `${fixture.contract} canary must be length-delimited`);
    assert(length.next + length.value === bytes.length, `${fixture.contract} must contain one complete field`);
    assert(payload.toString("utf8") === fixture.payloadUtf8, `${fixture.contract} payload mismatch`);
  } catch (error) {
    errors.push(`${fixture.contract} is not a valid Golden Frame canary: ${error.message}`);
  }
}

if (errors.length > 0) {
  console.error(errors.map((error) => `- ${error}`).join("\n"));
  process.exit(1);
}

console.log("Threadline contract structure and Golden Frame canaries are valid.");
