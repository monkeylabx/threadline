import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));

// One SBOM per locked crate: the two harnesses have independent dependency
// graphs, and the OpenMLS-only graph is the one the supply-chain gate reads.
const targets = [
  { crate: "rust", name: "threadline-e2ee-interop", output: "cargo.cdx.json" },
  {
    crate: "interop-mls-rs",
    name: "threadline-e2ee-interop-mls-rs",
    output: "interop-mls-rs.cdx.json",
  },
];

function lockedComponents(crate) {
  const lock = readFileSync(`${root}/${crate}/Cargo.lock`, "utf8");
  return lock
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
    .sort((a, b) => `${a.name}@${a.version}`.localeCompare(`${b.name}@${b.version}`))
    .map(({ name, version, checksum }) => ({
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
}

mkdirSync(`${root}/sbom`, { recursive: true });

for (const { crate, name, output } of targets) {
  const components = lockedComponents(crate);
  const sbom = {
    bomFormat: "CycloneDX",
    specVersion: "1.6",
    version: 1,
    metadata: {
      component: {
        type: "application",
        name,
        version: "0.0.0",
        "bom-ref": `pkg:cargo/${name}@0.0.0`,
      },
    },
    components,
  };
  writeFileSync(`${root}/sbom/${output}`, `${JSON.stringify(sbom, null, 2)}\n`);
  console.log(`wrote ${components.length} locked components to sbom/${output}`);
}
