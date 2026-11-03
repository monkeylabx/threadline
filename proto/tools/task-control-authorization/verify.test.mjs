import assert from "node:assert/strict";
import test from "node:test";
import { rules } from "./model.mjs";
import { loadFixtures, verify } from "./verify.mjs";
import { witnesses } from "./witnesses.mjs";
import "./corruption.test.mjs";
import "./aggregate.test.mjs";

const fixtures = loadFixtures();
const complete = { ...fixtures, scenarios: { ...fixtures.scenarios, cases: [...fixtures.scenarios.cases, ...witnesses] } };

test("independent reference outputs and additional denial witnesses agree", () => {
  const before = structuredClone(complete);
  assert.equal(verify(fixtures).scenarios, 190);
  assert.equal(verify(complete).scenarios, 190 + witnesses.length);
  assert.equal(new Set(rules.map((rule) => rule.id)).size, rules.length);
  assert.deepEqual(complete, before);
});

for (const guard of rules) {
  test(`removing denial guard ${guard.id} is detected`, () => {
    assert.throws(() => verify(complete, rules.filter((rule) => rule.id !== guard.id)), { name: "AssertionError" });
  });
}
