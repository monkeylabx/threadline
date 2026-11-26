import { rules } from "./rules.mjs";
import { normalize } from "./vocabulary.mjs";

export { rules } from "./rules.mjs";
const noEffects = { newBusinessWrites: 0, newGrants: 0, newEffects: 0, authoredBytesDisclosed: 0 };

// Expected fixture results and case IDs never enter this evaluator.
export function evaluate({ action, when, facts }, guards = rules) {
  if (when === "LocalStop") return { ...noEffects, localStop: true, coreConfirmedCancelled: false };
  if (when === "ReportCleanup.restricted-assignment") return cleanup(facts);
  if (!["AuthorizeTaskControl", "AdmitAction.current-authority"].includes(when)) throw new Error("Unknown reference seam");
  const f = normalize(action, when, facts);
  const denied = guards.find((guard) => !guard.valid(f));
  if (denied) return { ...noEffects, ...denied.result };
  return { ...noEffects, decision: "allow", status: "OK", ...annotations(f) };
}

function cleanup(facts) {
  if (facts["cleanup.assignmentVerified"] !== true) return { ...noEffects, status: "UNAUTHENTICATED" };
  return { ...noEffects, status: "OK", ordinaryRunReportAllowed: false, executionAllowed: false };
}

function annotations(f) {
  if (f.receipt) return { currentViewOnly: true, grantBytesReturned: 0 };
  if (f.op === "task.create") return { creatorRole: "OWNER", agentRole: "ASSIGNEE" };
  if (f.op === "run.start") return { queuesOnly: true };
  if (f.op === "approval.request") return { pendingOnly: true };
  if (f.op === "approval.decide") return { grantIssued: f["request.approved"], stopIntentRequired: !f["request.approved"], participantRoleCount: 1 };
  if (f.op === "approval.revoke") return { grantIssued: false };
  return {};
}
