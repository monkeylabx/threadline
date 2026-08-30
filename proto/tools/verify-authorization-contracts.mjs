import { createHash } from "node:crypto";
import { readFileSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const fixtureRoot = join(repositoryRoot, "test", "fixtures", "proto", "authorization");
const manifest = JSON.parse(readFileSync(join(fixtureRoot, "manifest.json"), "utf8"));
const sourceBytes = readFileSync(join(fixtureRoot, manifest.source.file));
const scenarios = JSON.parse(sourceBytes.toString("utf8"));

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function read(relativePath) {
  return readFileSync(join(repositoryRoot, relativePath), "utf8").replaceAll("\r\n", "\n");
}

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function merge(base, overlay) {
  if (Array.isArray(overlay) || overlay === null || typeof overlay !== "object") return overlay;
  const result = { ...(base ?? {}) };
  for (const [key, value] of Object.entries(overlay)) {
    if (key === "extends") continue;
    result[key] = merge(base?.[key], value);
  }
  return result;
}

const actions = [
  "space.discover",
  "space.channel.create",
  "channel.discover",
  "channel.read",
  "channel.publish",
  "channel.update",
  "channel.archive",
  "channel.membership.list",
  "channel.membership.add",
  "channel.membership.remove",
  "channel.acl.update",
];
const organizationRoles = ["ROLE_OWNER", "ROLE_ADMIN", "ROLE_SECURITY_ADMIN", "ROLE_MEMBER", "ROLE_GUEST"];
const channelRoles = [
  "CHANNEL_MEMBER_ROLE_NONE",
  "CHANNEL_MEMBER_ROLE_OWNER",
  "CHANNEL_MEMBER_ROLE_MODERATOR",
  "CHANNEL_MEMBER_ROLE_MEMBER",
  "CHANNEL_MEMBER_ROLE_GUEST",
];

const all = actions;
const ownerModerator = ["space.discover", "space.channel.create", "channel.discover", "channel.read", "channel.publish", "channel.update", "channel.membership.list", "channel.membership.add", "channel.membership.remove"];
const ownerMember = ["space.discover", "space.channel.create", "channel.discover", "channel.read", "channel.publish", "channel.membership.list"];
const securityOwner = ["space.discover", "channel.discover", "channel.read", "channel.publish", "channel.membership.list", "channel.membership.remove", "channel.acl.update"];
const securityModerator = ["space.discover", "channel.discover", "channel.read", "channel.publish", "channel.membership.list", "channel.membership.remove"];
const collaboration = ["space.discover", "channel.discover", "channel.read", "channel.publish", "channel.membership.list"];
const memberOwner = ["space.discover", "channel.discover", "channel.read", "channel.publish", "channel.update", "channel.archive", "channel.membership.list", "channel.membership.add", "channel.membership.remove", "channel.acl.update"];
const memberModerator = ["space.discover", "channel.discover", "channel.read", "channel.publish", "channel.update", "channel.membership.list", "channel.membership.add", "channel.membership.remove"];

const expectedMatrix = new Map([
  ["ROLE_OWNER/CHANNEL_MEMBER_ROLE_NONE", ["space.discover", "space.channel.create", "channel.discover"]],
  ["ROLE_OWNER/CHANNEL_MEMBER_ROLE_OWNER", all],
  ["ROLE_OWNER/CHANNEL_MEMBER_ROLE_MODERATOR", ownerModerator],
  ["ROLE_OWNER/CHANNEL_MEMBER_ROLE_MEMBER", ownerMember],
  ["ROLE_OWNER/CHANNEL_MEMBER_ROLE_GUEST", ownerMember],
  ["ROLE_ADMIN/CHANNEL_MEMBER_ROLE_NONE", ["space.discover", "space.channel.create", "channel.discover"]],
  ["ROLE_ADMIN/CHANNEL_MEMBER_ROLE_OWNER", all],
  ["ROLE_ADMIN/CHANNEL_MEMBER_ROLE_MODERATOR", ownerModerator],
  ["ROLE_ADMIN/CHANNEL_MEMBER_ROLE_MEMBER", ownerMember],
  ["ROLE_ADMIN/CHANNEL_MEMBER_ROLE_GUEST", ownerMember],
  ["ROLE_SECURITY_ADMIN/CHANNEL_MEMBER_ROLE_NONE", ["space.discover", "channel.discover"]],
  ["ROLE_SECURITY_ADMIN/CHANNEL_MEMBER_ROLE_OWNER", securityOwner],
  ["ROLE_SECURITY_ADMIN/CHANNEL_MEMBER_ROLE_MODERATOR", securityModerator],
  ["ROLE_SECURITY_ADMIN/CHANNEL_MEMBER_ROLE_MEMBER", collaboration],
  ["ROLE_SECURITY_ADMIN/CHANNEL_MEMBER_ROLE_GUEST", collaboration],
  ["ROLE_MEMBER/CHANNEL_MEMBER_ROLE_NONE", ["space.discover", "channel.discover"]],
  ["ROLE_MEMBER/CHANNEL_MEMBER_ROLE_OWNER", memberOwner],
  ["ROLE_MEMBER/CHANNEL_MEMBER_ROLE_MODERATOR", memberModerator],
  ["ROLE_MEMBER/CHANNEL_MEMBER_ROLE_MEMBER", collaboration],
  ["ROLE_MEMBER/CHANNEL_MEMBER_ROLE_GUEST", collaboration],
  ["ROLE_GUEST/CHANNEL_MEMBER_ROLE_NONE", ["space.discover"]],
  ["ROLE_GUEST/CHANNEL_MEMBER_ROLE_OWNER", collaboration],
  ["ROLE_GUEST/CHANNEL_MEMBER_ROLE_MODERATOR", collaboration],
  ["ROLE_GUEST/CHANNEL_MEMBER_ROLE_MEMBER", collaboration],
  ["ROLE_GUEST/CHANNEL_MEMBER_ROLE_GUEST", collaboration],
]);

function decision(effect, reason, policyVersion = "", aclVersion = "") {
  return { effect, reason, policyVersion, aclVersion };
}

function validAclEntry(entry) {
  return typeof entry.actorId === "string" && entry.actorId !== "" &&
    ["ACTOR_TYPE_HUMAN", "ACTOR_TYPE_AGENT", "ACTOR_TYPE_SERVICE"].includes(entry.actorType) &&
    actions.includes(entry.action) && ["allow", "deny"].includes(entry.effect);
}

function evaluate(testCase) {
  if (!testCase.principal) return decision("deny", "authentication-required");
  if (testCase.resource.tenantId !== testCase.principal.tenantId) return decision("deny", "tenant-mismatch");
  if (!testCase.factsAvailable) return decision("deny", "facts-unavailable");
  if (testCase.organization.tenantId !== testCase.principal.tenantId) return decision("deny", "facts-unavailable");

  const policyVersion = testCase.organization.policyVersion;
  if (testCase.organization.state !== "active") return decision("deny", "organization-unavailable", policyVersion);
  if (policyVersion === "") return decision("deny", "policy-version-invalid");
  if (testCase.member.tenantId !== testCase.principal.tenantId ||
      testCase.member.actorId !== testCase.principal.actorId ||
      testCase.member.actorType !== testCase.principal.actorType) {
    return decision("deny", "facts-unavailable", policyVersion);
  }
  if (testCase.member.state !== "active") return decision("deny", "member-inactive", policyVersion);
  if (!organizationRoles.includes(testCase.member.role)) return decision("deny", "organization-role-denied", policyVersion);
  if (!actions.includes(testCase.action)) return decision("deny", "unknown-action", policyVersion);
  if (!["space", "channel"].includes(testCase.resource.kind) || !testCase.resource.exists || testCase.resource.resourceId === "") {
    return decision("deny", "unknown-resource", policyVersion);
  }
  if (!testCase.action.startsWith(`${testCase.resource.kind}.`)) return decision("deny", "unknown-resource", policyVersion);

  let channelRole = "CHANNEL_MEMBER_ROLE_NONE";
  if (testCase.resource.kind === "channel") {
    if (testCase.channelMembership.tenantId !== testCase.principal.tenantId ||
        testCase.channelMembership.channelId !== testCase.resource.resourceId ||
        testCase.channelMembership.actorId !== testCase.principal.actorId ||
        testCase.channelMembership.actorType !== testCase.principal.actorType) {
      return decision("deny", "facts-unavailable", policyVersion);
    }
    if (!["public", "private"].includes(testCase.resource.visibility)) {
      return decision("deny", "unknown-resource", policyVersion);
    }
    if (!["active", "archived", "pending-deletion"].includes(testCase.resource.state)) {
      return decision("deny", "resource-state-denied", policyVersion);
    }
    const discoverableWithoutMembership = testCase.action === "channel.discover" && testCase.resource.visibility === "public";
    if (testCase.channelMembership.active && testCase.channelMembership.currentIntervalId === "") {
      return decision("deny", "not-a-member", policyVersion);
    }
    if (!testCase.channelMembership.active && !discoverableWithoutMembership) {
      return decision("deny", "not-a-member", policyVersion);
    }
    if (testCase.channelMembership.active) {
      channelRole = testCase.channelMembership.role;
      if (!channelRoles.includes(channelRole) || channelRole === "CHANNEL_MEMBER_ROLE_NONE") {
        return decision("deny", "channel-role-denied", policyVersion);
      }
    }
  }

  const matrixKey = `${testCase.member.role}/${channelRole}`;
  const allowed = expectedMatrix.get(matrixKey);
  assert(allowed, `${testCase.id}: missing canonical role matrix row ${matrixKey}`);
  if (!allowed.includes(testCase.action)) {
    const organizationCanEverPerform = [...expectedMatrix.entries()]
      .filter(([key]) => key.startsWith(`${testCase.member.role}/`))
      .some(([, row]) => row.includes(testCase.action));
    return decision("deny", organizationCanEverPerform ? "channel-role-denied" : "organization-role-denied", policyVersion);
  }

  const stateChangingActions = new Set([
    "channel.publish", "channel.update", "channel.archive", "channel.membership.add",
    "channel.membership.remove", "channel.acl.update",
  ]);
  if (testCase.resource.kind === "channel") {
    if (testCase.resource.state === "pending-deletion") return decision("deny", "resource-state-denied", policyVersion);
    if (testCase.resource.state === "archived" && stateChangingActions.has(testCase.action)) {
      return decision("deny", "resource-state-denied", policyVersion);
    }
  }

  const aclVersion = testCase.acl.version;
  if (aclVersion === "") return decision("deny", "acl-version-invalid", policyVersion);
  const aclResourceMatches = testCase.acl.resource.kind === testCase.resource.kind &&
    testCase.acl.resource.tenantId === testCase.resource.tenantId &&
    testCase.acl.resource.resourceId === testCase.resource.resourceId;
  if (!aclResourceMatches || !["allow", "deny"].includes(testCase.acl.defaultEffect) || !testCase.acl.entries.every(validAclEntry)) {
    return decision("deny", "acl-invalid", policyVersion, aclVersion);
  }
  const matches = testCase.acl.entries.filter((entry) =>
    entry.actorId === testCase.principal.actorId &&
    entry.actorType === testCase.principal.actorType &&
    entry.action === testCase.action);
  if (matches.some((entry) => entry.effect === "deny")) {
    return decision("deny", "acl-matched-deny", policyVersion, aclVersion);
  }
  if (!matches.some((entry) => entry.effect === "allow") && testCase.acl.defaultEffect === "deny") {
    return decision("deny", "acl-default-deny", policyVersion, aclVersion);
  }

  if (testCase.runtime.required) {
    if (!testCase.runtime.capabilityPresent) return decision("deny", "capability-required", policyVersion, aclVersion);
    if (!testCase.runtime.capabilityAllows) return decision("deny", "capability-denied", policyVersion, aclVersion);
    if (!testCase.runtime.delegationAllows) return decision("deny", "delegation-denied", policyVersion, aclVersion);
  }
  if (testCase.approval.required && !testCase.approval.present) {
    return decision("deny", "approval-required", policyVersion, aclVersion);
  }
  return decision("allow", "allowed", policyVersion, aclVersion);
}

assert(manifest.schemaVersion === 1, "fixture manifest schemaVersion must be 1");
assert(manifest.classification === "synthetic-authorization-contract-no-secrets", "fixture classification must remain synthetic and secret-free");
assert(JSON.stringify(manifest.reviewers) === JSON.stringify(["Contracts", "Product", "Security"]), "authorization fixture reviewers must remain explicit");
assert(manifest.provenance.issue === 113 && manifest.provenance.generator === "none", "fixture provenance must remain bound to Issue #113");
assert(sha256(sourceBytes) === manifest.source.sha256, "authorization scenario source SHA-256 mismatch");
assert(scenarios.schemaVersion === manifest.schemaVersion && scenarios.classification === manifest.classification, "scenario metadata differs from manifest");
assert(JSON.stringify(scenarios.actions) === JSON.stringify(actions), "authorization action order or vocabulary changed");
assert(manifest.requiredMatrix.organizationRoleCount === organizationRoles.length, "organization role count changed");
assert(manifest.requiredMatrix.channelRoleStateCount === channelRoles.length, "Channel role state count changed");
assert(manifest.requiredMatrix.actionCount === actions.length, "action count changed");
assert(manifest.requiredMatrix.cellCount === organizationRoles.length * channelRoles.length * actions.length, "matrix cell count changed");

const rows = new Map();
for (const row of scenarios.roleMatrix) {
  const key = `${row.organizationRole}/${row.channelRole}`;
  assert(!rows.has(key), `duplicate role matrix row ${key}`);
  assert(organizationRoles.includes(row.organizationRole), `${key}: unknown Organization Role`);
  assert(channelRoles.includes(row.channelRole), `${key}: unknown Channel Role state`);
  assert(new Set(row.allowedActions).size === row.allowedActions.length, `${key}: duplicate allowed action`);
  assert(row.allowedActions.every((action) => actions.includes(action)), `${key}: unknown allowed action`);
  rows.set(key, row);
}
assert(rows.size === expectedMatrix.size, "role matrix must contain all 25 Organization Role x Channel Role states");
for (const [key, expectedAllowed] of expectedMatrix) {
  const row = rows.get(key);
  assert(row, `missing role matrix row ${key}`);
  for (const action of actions) {
    assert(row.allowedActions.includes(action) === expectedAllowed.includes(action), `${key}/${action}: role matrix decision changed`);
  }
}

const authorizationProto = read("proto/threadline/authorization/v1/authorization.proto");
const identityProto = read("proto/threadline/identity/v1/identity.proto");
const channelProto = read("proto/threadline/channel/v1/channel.proto");
assert(!/\bservice\s+\w+|\brpc\s+\w+/u.test(authorizationProto), "authorization contract must remain data-only with no RPC");
assert(!/message\s+(?:Authenticated)?Principal\b/u.test(authorizationProto), "authorization contract must not define caller-supplied Principal");
assert(/oneof\s+resource\s*\{[^}]*string\s+space_id\s*=\s*2;[^}]*string\s+channel_id\s*=\s*3;/su.test(authorizationProto), "typed Space/Channel ResourceRef oneof is missing");
assert(/message\s+ResourceAcl\s*\{[^}]*string\s+acl_version\s*=\s*2;[^}]*AclEffect\s+default_effect\s*=\s*3;/su.test(authorizationProto), "versioned ResourceAcl contract is missing");
assert(/message\s+AuthorizationDecision\s*\{[^}]*AuthorizationEffect\s+effect\s*=\s*1;[^}]*AuthorizationReason\s+reason\s*=\s*2;[^}]*AuthorizationAction\s+action\s*=\s*3;[^}]*ResourceRef\s+resource\s*=\s*4;[^}]*ActorRef\s+actor\s*=\s*5;[^}]*string\s+policy_version\s*=\s*6;[^}]*string\s+acl_version\s*=\s*7;/su.test(authorizationProto), "bound, versioned AuthorizationDecision contract is missing");
for (const action of actions) {
  const enumName = `AUTHORIZATION_ACTION_${action.replaceAll(".", "_").toUpperCase()}`;
  assert(authorizationProto.includes(enumName), `missing proto action ${enumName}`);
  assert(authorizationProto.includes(`Canonical name: ${action}.`), `missing canonical action comment for ${action}`);
}
for (const role of organizationRoles) assert(identityProto.includes(role), `identity Role contract missing ${role}`);
for (const role of channelRoles.filter((role) => role !== "CHANNEL_MEMBER_ROLE_NONE")) {
  assert(channelProto.includes(role), `Channel Role contract missing ${role}`);
}
for (const contract of manifest.contracts) assert(statSync(join(repositoryRoot, "proto", contract)).isFile(), `missing contract ${contract}`);

const rawCases = new Map(scenarios.cases.map((testCase) => [testCase.id, testCase]));
assert(rawCases.size === scenarios.cases.length, "authorization scenario ids must be unique");
for (const required of manifest.requiredCases) assert(rawCases.has(required), `missing required authorization case ${required}`);

function resolveCase(id, seen = new Set()) {
  assert(!seen.has(id), `${id}: cyclic fixture inheritance`);
  const raw = rawCases.get(id);
  assert(raw, `${id}: missing fixture base`);
  if (!raw.extends) return raw;
  return merge(resolveCase(raw.extends, new Set([...seen, id])), raw);
}

for (const id of rawCases.keys()) {
  const testCase = resolveCase(id);
  const actual = evaluate(testCase);
  const expectedDecision = {
    effect: testCase.expected.effect,
    reason: testCase.expected.reason,
    policyVersion: testCase.expected.policyVersion,
    aclVersion: testCase.expected.aclVersion,
  };
  assert(JSON.stringify(actual) === JSON.stringify(expectedDecision), `${id}: expected ${JSON.stringify(expectedDecision)}, got ${JSON.stringify(actual)}`);
  if (id === "rejoin-uses-only-current-membership-interval") {
    assert(testCase.channelMembership.currentIntervalId !== "" && testCase.channelMembership.departedIntervalIds.length > 0, `${id}: current and departed intervals must both be represented`);
    assert(testCase.expected.historicalAccessGranted === false, `${id}: rejoin must not grant old history`);
  }
  if (id === "acl-version-change-invalidates-prior-allow") {
    assert(testCase.priorDecision.aclVersion !== actual.aclVersion && actual.effect === "deny", `${id}: current ACL version must replace the prior allow`);
  }
  if (id === "application-allow-does-not-create-crypto-admission") {
    assert(actual.effect === "allow" && testCase.resource.cryptographicAdmission === false, `${id}: application allow must remain separate from crypto admission`);
    assert(testCase.expected.cryptographicAdmission === false, `${id}: expected crypto admission must remain false`);
  }
}

for (const [name, value] of Object.entries(manifest.acceptance)) {
  if (name !== "physicalDevices") assert(value === true, `${name} acceptance must remain true`);
}
assert(manifest.acceptance.physicalDevices === "NOT RUN", "contract fixtures must not claim physical-device evidence");

console.log(`Threadline authorization contract is valid: ${rows.size * actions.length} role cells and ${rawCases.size} edge cases.`);
