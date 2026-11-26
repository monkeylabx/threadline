// Independent counterexamples; none are inferred from model output.
const rt = (given) => ({ "principal.route": "assigned-runtime", "runtime.verified": true, "task.role": "ASSIGNEE", ...given });
export const witnesses = [
  ["shape", "task.read", { "request.valid": false }, "shape-invalid", "INVALID_ARGUMENT", null],
  ["role-ceiling", "approval.decide", { "task.role": "OBSERVER", "policy.approvalRoles": ["OBSERVER"] }, "approver-role-denied"],
  ["role-policy", "approval.decide", { "task.role": "APPROVER", "policy.approvalRoles": [] }, "approver-role-denied"],
  ["operator-role", "run.report", rt({ "runtime.operatorOrganizationRole": "unknown" }), "facts-binding-invalid", "UNAVAILABLE", null],
  ["owner-binding", "task.read", { "task.role": "OWNER" }, "facts-binding-invalid", "UNAVAILABLE", null],
  ["use-role", null, rt({ "approval.decidingHumanRole": "unknown" }), "facts-binding-invalid", "UNAVAILABLE", null],
  ["clock", "run.report", rt({ "clock.now": "invalid" }), "facts-unavailable", "UNAVAILABLE", null],
  ["signature", "run.report", rt({ "grant.signatureValid": false }), "grant-signature-invalid", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"],
  ["expiry", "run.report", rt({ "clock.now": "2030-01-01T00:10:00Z" }), "grant-expired", "PERMISSION_DENIED", "GRANT_EXPIRED"],
  ["use-snapshot", null, rt({ "approval.snapshotIncludesCaller": false }), "approver-snapshot-denied"],
  ["use-role-policy", null, rt({ "policy.approvalRoles": [] }), "approver-role-denied"],
  ["use-resource", null, rt({ "approval.resourceAuthorityCurrent": false }), "scope-expanded", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"],
  ["use-history", null, rt({ "source.inCurrentInterval": false, "source.historySharingCurrent": false }), "source-history-denied", "PERMISSION_DENIED", "HISTORY_SHARING_DENIED"],
  ["use-missing-fact", null, rt({ "approval.currentEligible": undefined }), "use-approver-facts-unavailable", "UNAVAILABLE", null],
  ["request-expanded", "approval.request", rt({ "delegation.requestFits": false }), "scope-expanded", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"],
].map(([id, action, given, reason, status = "PERMISSION_DENIED", code = "PERMISSION_DENIED"]) => ({
  id, action, given, when: action === null ? "AdmitAction.current-authority" : "AuthorizeTaskControl",
  then: { decision: "deny", reason, status, ...(code && { code }), newBusinessWrites: 0, newGrants: 0, newEffects: 0 },
}));
