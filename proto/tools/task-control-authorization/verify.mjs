import assert from "node:assert/strict";
import { evaluate, rules } from "./model.mjs";
import { actions, creates } from "./vocabulary.mjs";
import { checkExpected, compare } from "./fixture.mjs";

export { loadFixtures } from "./fixture.mjs";
const ceilingStates = ["human/OWNER/MEMBER/false", "human/APPROVER/MEMBER/false", "human/OBSERVER/MEMBER/false", "human/NONE/MEMBER/false", "human/OBSERVER/GUEST/false", "human/OWNER/MEMBER/true", "assigned-runtime/ASSIGNEE/MEMBER/false", "assigned-runtime/OWNER/MEMBER/false", "bare-agent/ASSIGNEE/MEMBER/false", "service/NONE/MEMBER/false"];

function checkMatrix({ decisions: d, scenarios: s }, guards) {
  assert.equal(d.formatVersion, 1);
  assert.equal(d.profile, "task-control-auth/1");
  assert.deepEqual(d.actions.map((row) => row.action), actions);
  for (const row of d.actions) assert.deepEqual(row.parentActions, creates.includes(row.action) ? ["channel.read", "channel.publish"] : ["channel.read"]);
  assert.equal(d.roleCeilings.length, 10);
  assert.equal(new Set(d.roleCeilings.map((row) => row.id)).size, 10);
  assert.deepEqual(d.roleCeilings.map((row) => `${row.route}/${row.taskRole}/${row.currentOrganizationAndChannelTier}/${row.devSelfException}`), ceilingStates);
  assert.equal(d.expectedCeilingCells, actions.length * d.roleCeilings.length);
  for (const row of d.roleCeilings) checkCeiling(row, s.defaults, guards);
}

function checkCeiling(row, defaults, guards) {
  assert.equal(new Set(row.allowedActions).size, row.allowedActions.length, row.id);
  assert.ok(row.allowedActions.every((action) => actions.includes(action)), row.id);
  const facts = { ...defaults, "principal.route": row.route, "task.role": row.taskRole,
    "principal.actor": row.taskRole === "OWNER" ? defaults["task.creator"] : defaults["principal.actor"],
    "organization.role": row.currentOrganizationAndChannelTier, "parent.role": row.currentOrganizationAndChannelTier,
    "runtime.verified": true, "policy.allowOwnerSelfApproval": row.devSelfException,
    "policy.configuredOwnerApprover": row.devSelfException,
    "policy.approvalRoles": row.devSelfException ? ["APPROVER", "OWNER"] : ["APPROVER"] };
  for (const action of actions) {
    const actual = evaluate({ action, when: "AuthorizeTaskControl", facts }, guards);
    assert.equal(actual.decision === "allow", row.allowedActions.includes(action), `${row.id}/${action}`);
  }
}

export function verify(fixtures, guards = rules) {
  const { scenarios: s } = fixtures;
  assert.equal(s.formatVersion, 1);
  assert.ok(s.defaults && Array.isArray(s.cases) && s.cases.length > 0);
  checkMatrix(fixtures, guards);
  assert.equal(new Set(s.cases.map((row) => row.id)).size, s.cases.length);
  for (const row of s.cases) {
    assert.match(row.id, /^[a-z][a-z0-9.-]{0,79}$/);
    checkExpected(row.then, row.id);
    const facts = { ...s.defaults, ...row.given };
    const actual = evaluate({ action: row.action, when: row.when, facts }, guards);
    compare(actual, row.then, row.id);
  }
  return { scenarios: s.cases.length, roleCells: fixtures.decisions.expectedCeilingCells, guards: guards.length };
}
