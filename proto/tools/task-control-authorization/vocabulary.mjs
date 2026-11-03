import { contains } from "./rule.mjs";

export const actions = ["task.create", "task.read", "run.start", "run.read", "run.control", "approval.read", "approval.request", "approval.decide", "approval.revoke", "run.report"];
export const reads = ["task.read", "run.read", "approval.read"];
export const creates = ["task.create", "run.start"];
export const decisions = ["approval.decide", "approval.revoke"];
export const runtimeActions = ["run.read", "approval.request", "run.report"];
const receiptViews = { "task.create": "task.read", "run.start": "run.read", "run.control": "run.read", "run.report": "run.read", "approval.request": "approval.read", "approval.decide": "approval.read", "approval.revoke": "approval.read" };

export function normalize(action, when, facts) {
  const receipt = facts["request.purpose"] === "own_receipt_read" && facts["receipt.exists"] === true;
  const op = receipt ? receiptViews[action] : action;
  const use = when === "AdmitAction.current-authority";
  return { ...facts, action, op, receipt, use,
    runtime: facts["principal.route"] === "assigned-runtime",
    source: use || contains([...creates, "task.read", "approval.read"], op),
    target: contains(creates, op),
    approver: use || contains(decisions, op),
    approverRole: use ? facts["approval.decidingHumanRole"] : facts["task.role"],
    privileged: use || contains([...creates, ...decisions, "approval.request", "run.report"], op),
    active: use || contains([...creates, "approval.request", "run.report"], op) || (op === "approval.decide" && facts["request.approved"] === true),
  };
}

export function routeAllowed(f) {
  if (f.use) return f.runtime && f["runtime.verified"] === true;
  if (f["principal.route"] === "human") return !["approval.request", "run.report"].includes(f.op);
  return f.runtime && f["runtime.verified"] === true && contains(runtimeActions, f.op) && f["task.role"] === "ASSIGNEE";
}

export function selfApproval(f, role) {
  if (role !== "OWNER") return true;
  return f["policy.deploymentClass"] === "dedicated-dev" && f["policy.allowOwnerSelfApproval"] === true &&
    f["policy.configuredOwnerApprover"] === true && contains(f["policy.approvalRoles"], "OWNER");
}
