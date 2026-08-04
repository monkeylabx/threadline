import assert from "node:assert/strict";
import test from "node:test";

import { desktopSkeleton } from "../src/main.ts";

test("desktop skeleton exposes only the bridge contract version", () => {
  assert.deepEqual(desktopSkeleton, {
    bridgeContractVersion: 1,
    product: "threadline-desktop",
  });
});
