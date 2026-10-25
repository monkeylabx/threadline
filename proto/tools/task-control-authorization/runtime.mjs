import { everyTrue, rule } from "./rule.mjs";

export const runtimeRules = [
  rule("runtime-members", (f) => !f.runtime || everyTrue(f, ["runtime.agentMemberCurrent", "runtime.operatorMemberCurrent"]), "member-inactive"),
  rule("assigned-agent", (f) => !f.runtime || f["runtime.agentMatches"] === true, "target-binding-denied"),
  rule("owner", (f) => !f.runtime || everyTrue(f, ["runtime.ownerMatches", "runtime.runAssigned"]), "not-execution-owner", "FAILED_PRECONDITION", "NOT_EXECUTION_OWNER"),
  rule("lease", (f) => !f.runtime || f["runtime.leaseCurrent"] === true, "lease-lost", "FAILED_PRECONDITION", "LEASE_LOST"),
  rule("fence", (f) => !f.runtime || f["runtime.fenceCurrent"] === true, "fence-stale", "FAILED_PRECONDITION", "FENCING_TOKEN_STALE"),
  rule("execution-grant", (f) => !f.runtime || f["grant.present"] === true, "execution-grant-required", "PERMISSION_DENIED", "APPROVAL_REQUIRED"),
  rule("grant-state-expired", (f) => !f.runtime || f["grant.state"] !== "EXPIRED", "grant-expired", "PERMISSION_DENIED", "GRANT_EXPIRED"),
  rule("grant-state", (f) => !f.runtime || f["grant.state"] === "ACTIVE", "grant-revoked", "PERMISSION_DENIED", "GRANT_REVOKED"),
  rule("execution-signature", (f) => !f.runtime || f.use || f["grant.signatureValid"] === true, "grant-signature-invalid", "PERMISSION_DENIED", "GRANT_OUT_OF_SCOPE"),
  rule("execution-expiry", (f) => !f.runtime || f.use || Date.parse(f["clock.now"]) < Date.parse(f["grant.expiresAt"]), "grant-expired", "PERMISSION_DENIED", "GRANT_EXPIRED"),
];
