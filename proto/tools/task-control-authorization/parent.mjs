import { contains, everyTrue, rule } from "./rule.mjs";
import { reads, routeAllowed } from "./vocabulary.mjs";

export const parentRules = [
  rule("parent-read", (f) => f["parent.read"] === true && (!f.runtime || everyTrue(f, ["parent.agentRead", "parent.operatorRead"])), "parent-read-denied"),
  rule("acl", (f) => f["parent.aclMatchedDeny"] === false && ["allow", "deny"].includes(f["parent.aclDefault"]) && (f["parent.aclDefault"] === "allow" || f["parent.aclMatchedAllow"] === true), "parent-acl-denied"),
  rule("parent-state", (f) => f["parent.state"] === "active" || (f["parent.state"] === "archived" && !f.active), "parent-state-denied"),
  rule("enabled-action", (f) => contains(f["policy.enabledActions"], f.use ? "approval.request" : f.op), "action-disabled"),
  rule("audience", (f) => !reads.includes(f.op) || f["policy.readAudience"] === "channel_readers", "policy-audience-denied"),
  rule("actor-constraint", (f) => f["policy.actorDenied"] === false, "policy-actor-denied"),
  rule("parent-publish", (f) => !f.target || f["parent.publish"] === true, "parent-publish-denied"),
  rule("route", routeAllowed, "principal-route-denied"),
];
