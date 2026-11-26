import { rule } from "./rule.mjs";

export const requestRules = [
  rule("context", (f) => f.op !== "task.create" || f["request.additionalReferences"] !== true, "unsupported-context", "UNIMPLEMENTED", null),
  rule("participant-limit", (f) => f.op !== "task.create" || (Number.isInteger(f["request.participantCount"]) && f["request.participantCount"] <= 16), "payload-too-large", "RESOURCE_EXHAUSTED", "PAYLOAD_TOO_LARGE"),
  rule("participants", (f) => f.op !== "task.create" || (!f["request.duplicateActorRole"] && !f["request.extraOwner"] && !f["request.participantTimestamp"] && f["request.participantsMatchDerived"] === true), "participant-set-invalid", "INVALID_ARGUMENT", null),
  rule("delegation", (f) => !(f.target || f.runtime) || (f["delegation.current"] === true && (f.use || f["delegation.initiatorAuthorityCurrent"] === true)), "delegation-denied", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"),
  rule("requested-scope", (f) => !(f.target || f.op === "approval.request") || f["delegation.requestFits"] === true, "scope-expanded", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"),
  rule("approvers-present", (f) => f.op !== "approval.request" || f["approval.eligibleCount"] > 0, "approver-current-denied"),
];
