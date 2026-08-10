import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const lock = readFileSync(`${root}/rust/Cargo.lock`, "utf8");
const packages = lock
  .split("[[package]]")
  .slice(1)
  .map((block) => {
    const field = (name) => block.match(new RegExp(`^${name} = "([^"]+)"`, "m"))?.[1];
    return {
      name: field("name"),
      version: field("version"),
      source: field("source"),
      checksum: field("checksum"),
    };
  })
  .filter((entry) => entry.name && entry.version && entry.source?.startsWith("registry+"))
  .sort((a, b) => `${a.name}@${a.version}`.localeCompare(`${b.name}@${b.version}`));

const components = packages.map(({ name, version, checksum }) => ({
  type: "library",
  "bom-ref": `pkg:cargo/${name}@${version}`,
  name,
  version,
  ...(checksum ? { hashes: [{ alg: "SHA-256", content: checksum }] } : {}),
  purl: `pkg:cargo/${name}@${version}`,
  externalReferences: [
    { type: "distribution", url: `https://crates.io/api/v1/crates/${name}/${version}/download` },
  ],
}));

const sbom = {
  bomFormat: "CycloneDX",
  specVersion: "1.6",
  version: 1,
  metadata: {
    component: {
      type: "application",
      name: "threadline-e2ee-interop",
      version: "0.0.0",
      "bom-ref": "pkg:cargo/threadline-e2ee-interop@0.0.0",
    },
  },
  components,
};

mkdirSync(`${root}/sbom`, { recursive: true });
writeFileSync(`${root}/sbom/cargo.cdx.json`, `${JSON.stringify(sbom, null, 2)}\n`);
console.log(`wrote ${components.length} locked components`);
