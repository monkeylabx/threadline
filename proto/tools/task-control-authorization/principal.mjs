import { everyTrue, rule } from "./rule.mjs";
import { actions } from "./vocabulary.mjs";

// Expiry/audience bits are synthetic route-verification facts, never a Runtime's borrowed Human Session.
export const principalRules = [
  rule("authentication", (f) => everyTrue(f, ["principal.authenticated", "principal.sessionUnexpired"]) && f["principal.credentialAudience"] === "route-correct", "authentication-required", "UNAUTHENTICATED", null),
  rule("request-shape", (f) => f["request.valid"] === true, "shape-invalid", "INVALID_ARGUMENT", null),
  rule("action", (f) => f.use || actions.includes(f.action), "unknown-action", "UNIMPLEMENTED", null),
  rule("purpose", (f) => ["operation", "own_receipt_read"].includes(f["request.purpose"]), "purpose-invalid", "INVALID_ARGUMENT", null),
  rule("parent-kind", (f) => f["request.parentKind"] === "channel", "unsupported-parent", "UNIMPLEMENTED", null),
  rule("device", (f) => f["principal.deviceCurrent"] === true, "device-revoked", "PERMISSION_DENIED", "DEVICE_REVOKED"),
  rule("organization", (f) => f["organization.active"] === true, "organization-inactive"),
  rule("member", (f) => f["organization.memberActive"] === true, "member-inactive"),
  rule("lookup", (f) => everyTrue(f, ["lookup.exists", "lookup.existsInAuthenticatedTenant", "lookup.requestedChildBindingMatches"]), "object-absent", "NOT_FOUND", null),
  rule("clock-facts", (f) => (!f.runtime && !f.use) || (Number.isFinite(Date.parse(f["clock.now"])) && Number.isFinite(Date.parse(f["grant.expiresAt"]))), "facts-unavailable", "UNAVAILABLE", null),
];
