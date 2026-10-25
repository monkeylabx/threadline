import { dirname, join, relative } from "node:path";

import { filesBelow, protoRoot, read, repositoryRoot } from "./support.mjs";

// Every handwritten schema file must sit at the path its package names and
// carry a stable version suffix, so generated SDKs stay predictable.
export function checkProtoSources(assert) {
  for (const path of filesBelow(join(protoRoot, "threadline"), ".proto")) {
    const source = read(path);
    const packageMatch = source.match(/^package\s+([a-z0-9_.]+);/mu);
    const expectedPackage = relative(protoRoot, dirname(path)).split(/[\\/]/u).join(".");
    assert(source.startsWith('syntax = "proto3";'), `${relative(repositoryRoot, path)} must declare proto3 syntax first`);
    assert(packageMatch?.[1] === expectedPackage, `${relative(repositoryRoot, path)} package must be ${expectedPackage}`);
    assert(/\.v[1-9][0-9]*$/u.test(expectedPackage), `${relative(repositoryRoot, path)} must use a stable version suffix`);
  }
}
