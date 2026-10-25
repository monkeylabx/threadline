import { rule } from "./rule.mjs";
import { selfApproval } from "./vocabulary.mjs";

export const useInitialRules = [
  rule("use-facts", (f) => !f.use || (f["approval.eligibilityFactsAvailable"] !== false && ["approval.snapshotIncludesCaller", "approval.currentEligible", "approval.resourceAuthorityCurrent"].every((key) => typeof f[key] === "boolean")), "use-approver-facts-unavailable", "UNAVAILABLE", null),
  rule("use-approver", (f) => !f.use || f["approval.currentEligible"] === true, "use-approver-removed"),
  rule("use-self", (f) => !f.use || selfApproval(f, f["approval.decidingHumanRole"]), "use-owner-self-disabled"),
];

export const useRules = [
  rule("use-initiator", (f) => !f.use || f["delegation.initiatorAuthorityCurrent"] === true, "use-initiator-authority-lost", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"),
  rule("use-action", (f) => !f.use || f["approval.exactActionMatches"] === true, "use-exact-action-changed", "ALREADY_EXISTS", "IDEMPOTENCY_CONFLICT"),
  rule("use-scope", (f) => !f.use || f["approval.grantedScopeEmpty"] === false, "use-empty-resource-scope", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"),
  rule("use-grant-relation", (f) => !f.use || f["approval.grantRelationMatches"] === true, "use-grant-not-related", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"),
  rule("use-signature", (f) => !f.use || f["grant.signatureValid"] === true, "use-signature-invalid", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"),
  rule("use-expiry", (f) => !f.use || Date.parse(f["clock.now"]) < Date.parse(f["grant.expiresAt"]), "use-grant-expiry-boundary", "PERMISSION_DENIED", "GRANT_EXPIRED"),
];
