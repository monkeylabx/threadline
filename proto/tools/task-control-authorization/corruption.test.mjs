import assert from "node:assert/strict";
import test from "node:test";
import { loadFixtures, verify } from "./verify.mjs";

const corruptions = [
  (f) => f.decisions.roleCeilings[0].allowedActions.pop(),
  (f) => f.decisions.actions.push({ action: "synthetic.unknown", parentActions: [] }),
  (f) => { f.scenarios.cases[0].then.decision = "deny"; },
  (f) => { f.scenarios.cases[0].then = {}; },
  (f) => { delete f.scenarios.cases.find((row) => row.then.code).then.code; },
  (f) => { f.decisions.roleCeilings[1] = { ...f.decisions.roleCeilings[0], id: "another-id" }; },
];
test("changed ceilings, actions, expectations, omitted errors and removed role states fail", () => {
  for (const mutate of corruptions) {
    const invalid = loadFixtures();
    mutate(invalid);
    assert.throws(() => verify(invalid), { name: "AssertionError" });
  }
});
