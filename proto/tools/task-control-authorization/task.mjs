import { contains, everyTrue, rule } from "./rule.mjs";

export const taskRules = [
  rule("organization-ceiling", (f) => !f.privileged || (f["organization.role"] !== "GUEST" && (!f.runtime || f["runtime.operatorOrganizationRole"] !== "GUEST")), "organization-task-ceiling"),
  rule("channel-ceiling", (f) => !f.privileged || f["parent.role"] !== "GUEST", "channel-task-ceiling"),
  rule("start-role", (f) => f.op !== "run.start" || (f["task.role"] === "OWNER" && contains(f["policy.startRoles"], "OWNER")), "task-role-denied"),
  rule("control-role", (f) => f.op !== "run.control" || (["OWNER", "OBSERVER", "APPROVER"].includes(f["task.role"]) && contains(f["policy.controlRoles"], f["task.role"])), "task-role-denied"),
  rule("source-retention", (f) => !f.source || f["source.retained"] === true, "source-erased", "FAILED_PRECONDITION", "RETENTION_EXPIRED"),
  rule("source-history", (f) => !f.source || f["source.inCurrentInterval"] === true || f["source.historySharingCurrent"] === true, "source-history-denied", "PERMISSION_DENIED", "HISTORY_SHARING_DENIED"),
  rule("target-facts", (f) => !f.target || f["target.factsAvailable"] === true, "facts-unavailable", "UNAVAILABLE", null),
  rule("target", (f) => !f.target || everyTrue(f, ["target.bindingMatches", "target.agentEnabled"]), "target-binding-denied"),
  rule("target-device", (f) => !f.target || f["target.deviceCurrent"] === true, "device-revoked", "PERMISSION_DENIED", "DEVICE_REVOKED"),
];
