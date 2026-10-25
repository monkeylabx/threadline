import { rule } from "./rule.mjs";

export const reconciliationRules = [
  rule("use-blocked", (f) => !f.use || f["business.runBlocked"] === false, "use-policy-restored-but-run-blocked", "FAILED_PRECONDITION", "INVALID_STATE_TRANSITION"),
  rule("use-cancel", (f) => !f.use || f["business.cancelIntent"] === false, "use-cancel-before-consume", "FAILED_PRECONDITION", "INVALID_STATE_TRANSITION"),
  rule("use-barrier", (f) => !f.use || f["business.safetyBarrier"] === false, "use-barrier-present", "FAILED_PRECONDITION", "INVALID_STATE_TRANSITION"),
  rule("use-consumption", (f) => !f.use || f["approval.alreadyConsumed"] === false, "use-once-already-consumed", "FAILED_PRECONDITION", "INVALID_STATE_TRANSITION"),
  rule("receipt-namespace", (f) => !f.receipt || f["receipt.namespaceMatches"] === true, "receipt-namespace-denied"),
  rule("receipt-bytes", (f) => !f.receipt || f["receipt.exactRequest"] === true, "idempotency-conflict", "ALREADY_EXISTS", "IDEMPOTENCY_CONFLICT"),
];
