import { createHash } from "node:crypto";
import { lstatSync, readFileSync, realpathSync } from "node:fs";
import { join } from "node:path";

const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");

// Called by the trusted runner launcher before the bundle's Node loads any code.
export function verifiedBootstrapNode(bundle, manifestDigest) {
  const bytes = readFileSync(join(bundle, "manifest.json"));
  if (sha256(bytes) !== manifestDigest) throw new Error("bootstrap manifest digest mismatch");
  const tool = JSON.parse(bytes).tools?.node;
  if (tool?.path !== "closures/node/bin/node" || tool.invocation !== "native") throw new Error("bootstrap requires the fixed native Node path");
  let node = realpathSync(bundle);
  for (const segment of tool.path.split("/")) {
    node = join(node, segment);
    if (lstatSync(node).isSymbolicLink()) throw new Error("bootstrap Node path must not contain symlinks");
  }
  if (!lstatSync(node).isFile() || sha256(readFileSync(node)) !== tool.sha256) throw new Error("bootstrap Node executable digest/type mismatch");
  return node;
}
