import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const root = new URL("../../../", import.meta.url);
const fixtureRoot = new URL("test/fixtures/proto/task-control-authorization/", root);
const observations = ["decision", "status", "reason", "code", "newBusinessWrites", "newGrants", "newEffects", "authoredBytesDisclosed", "creatorRole", "agentRole", "queuesOnly", "pendingOnly", "grantIssued", "stopIntentRequired", "participantRoleCount", "currentViewOnly", "grantBytesReturned", "ordinaryRunReportAllowed", "executionAllowed", "localStop", "coreConfirmedCancelled"];
const errorSource = readFileSync(new URL("proto/threadline/type/v1/error.proto", root), "utf8");

export function loadFixtures() {
  const read = (name) => JSON.parse(readFileSync(new URL(name, fixtureRoot), "utf8"));
  return { decisions: read("decisions.json"), scenarios: read("scenarios.json") };
}

export function compare(actual, expected, label) {
  for (const key of Object.keys(actual)) assert.ok(observations.includes(key), `${label}: unexpected output key`);
  for (const key of ["decision", "status", "reason", "code"]) assert.equal(actual[key], expected[key], label);
  const projection = Object.fromEntries(Object.keys(expected).map((key) => [key, actual[key]]));
  assert.deepEqual(projection, expected, label);
  for (const key of ["newBusinessWrites", "newGrants", "newEffects"]) assert.equal(actual[key], 0, label);
}

export function checkExpected(expected, label) {
  assert.ok(expected && typeof expected === "object" && !Array.isArray(expected), label);
  for (const [key, value] of Object.entries(expected)) {
    assert.ok(observations.includes(key), `${label}: unexpected expectation key`);
    assert.ok(["string", "boolean", "number"].includes(typeof value), label);
    if (typeof value === "string") assert.match(value, /^[a-zA-Z][a-zA-Z0-9_.-]{0,79}$/, label);
  }
  if (expected.code) assert.ok(errorSource.includes(`ERROR_CODE_${expected.code} =`), label);
}
