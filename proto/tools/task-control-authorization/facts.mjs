import { everyTrue, nonempty, rule } from "./rule.mjs";

export const factRules = [
  rule("availability", (f) => f["facts.available"] === true && nonempty(f["parent.aclRevision"]), "facts-unavailable", "UNAVAILABLE", null),
  rule("binding", (f) => everyTrue(f, ["facts.tenantMatches", "facts.actorMatches", "facts.resourceMatches", "facts.transactionCurrent", "parent.decisionBindingCurrent", "policy.scopeMatches"]), "facts-binding-invalid", "UNAVAILABLE", null),
  rule("known-roles", (f) => ["OWNER", "ADMIN", "SECURITY_ADMIN", "MEMBER", "GUEST"].includes(f["organization.role"]) && ["OWNER", "MODERATOR", "MEMBER", "GUEST"].includes(f["parent.role"]), "facts-binding-invalid", "UNAVAILABLE", null),
  rule("task-role-facts", (f) => ["NONE", "OWNER", "APPROVER", "OBSERVER", "ASSIGNEE"].includes(f["task.role"]) && (f["principal.route"] !== "human" || f["task.role"] !== "ASSIGNEE"), "facts-binding-invalid", "UNAVAILABLE", null),
  rule("policy-availability", (f) => f["policy.available"] === true, "policy-unavailable", "UNAVAILABLE", null),
  rule("policy-format", (f) => f["policy.profile"] === "task-control-auth/1" && nonempty(f["policy.revision"]), "policy-invalid", "UNAVAILABLE", null),
  rule("membership", (f) => f["parent.membershipCurrent"] === true && nonempty(f["parent.intervalId"]), "not-a-member", "PERMISSION_DENIED", "NOT_A_MEMBER"),
  rule("operator-role-facts", (f) => !f.runtime || ["OWNER", "ADMIN", "SECURITY_ADMIN", "MEMBER", "GUEST"].includes(f["runtime.operatorOrganizationRole"]), "facts-binding-invalid", "UNAVAILABLE", null),
  rule("owner-binding", (f) => f["principal.route"] !== "human" || f["task.role"] !== "OWNER" || f.op === "task.create" || f["principal.actor"] === f["task.creator"], "facts-binding-invalid", "UNAVAILABLE", null),
  rule("use-role-facts", (f) => !f.use || ["OWNER", "APPROVER", "OBSERVER", "NONE"].includes(f.approverRole), "facts-binding-invalid", "UNAVAILABLE", null),
];
