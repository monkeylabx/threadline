import { contains, rule } from "./rule.mjs";
import { selfApproval } from "./vocabulary.mjs";

export const approvalRules = [
  rule("approver-role", (f) => !f.approver || ["OWNER", "APPROVER"].includes(f.approverRole), "approver-role-denied"),
  rule("self-approval", (f) => !f.approver || f.use || selfApproval(f, f["task.role"]), "self-approval-denied"),
  rule("approval-role-policy", (f) => !f.approver || contains(f["policy.approvalRoles"], f.approverRole), "approver-role-denied"),
  rule("snapshot", (f) => !f.approver || f["approval.snapshotIncludesCaller"] === true, "approver-snapshot-denied"),
  rule("current-approver", (f) => !f.approver || f.use || f["approval.currentEligible"] === true, "approver-current-denied"),
  rule("approver-resources", (f) => !f.approver || f["approval.resourceAuthorityCurrent"] === true, "scope-expanded", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"),
  rule("once-mode", (f) => f.op !== "approval.decide" || f["request.approvalScope"] === "ONCE", "unsupported-mode", "UNIMPLEMENTED", null),
];
