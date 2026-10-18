import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { decisions } from "./agentd-local-reference.mjs";

test("missing and unavailable grant expiry never admit access or effects", () => {
  const fixture = JSON.parse(readFileSync(new URL("../../test/fixtures/proto/agentd-local/scenarios.json", import.meta.url)));
  for (const expiry of [undefined, null]) {
    const facts = structuredClone(fixture.base);
    if (expiry === undefined) delete facts.run.grantExpired;
    else facts.run.grantExpired = expiry;
    assert.deepEqual(["input", "watch", "output", "approval"].map((operation) => decisions[operation](facts)), [
      "UNAVAILABLE", "UNAVAILABLE", "DROP", "DENY_PROTECTED_EFFECT",
    ]);
  }
});
