// Fixture-only reference decisions. Production authorization and worker
// cleanup are separate Runtime/Security admission work, not implemented here.
import { authorization, streamMatches } from "./agentd-local-binding.mjs";
import { grantAuthority, inputAuthority } from "./agentd-local-authority.mjs";

function inputReceipt(facts) {
  const { request, intake } = facts;
  if (!/^[\x21-\x7e]{1,128}$/u.test(request.inputId)) return "INVALID_ARGUMENT";
  if (Buffer.byteLength(request.text, "utf8") === 0) return "INVALID_ARGUMENT";
  if (Buffer.byteLength(request.text, "utf8") > 65536) return "ERROR_CODE_PAYLOAD_TOO_LARGE";
  if (intake.uncertain) return "ERROR_CODE_RUN_INPUT_OUTCOME_UNCERTAIN";
  if (intake.prior !== null) return intake.prior === request.text ? "DUPLICATE" : "ERROR_CODE_IDEMPOTENCY_CONFLICT";
  if (intake.activeTurn) return "ERROR_CODE_INVALID_STATE_TRANSITION";
  if (intake.acceptedInputs >= 16) return "RESOURCE_EXHAUSTED";
  return "ACCEPTED";
}

function input(facts) {
  return inputAuthority(facts) ?? inputReceipt(facts);
}

function watch(facts) {
  const denied = authorization(facts, "observe") ?? grantAuthority(facts);
  if (denied) return denied;
  const { request, activity } = facts;
  if (!request.stream) return request.after === 0 ? "WATCH" : "ERROR_CODE_CURSOR_INVALID";
  if (!streamMatches(facts)) return "ERROR_CODE_CURSOR_INVALID";
  if (request.after > activity.latest) return "ERROR_CODE_CURSOR_INVALID";
  if (request.after + 1 < activity.first) return "ERROR_CODE_SEQUENCE_GAP";
  return "WATCH";
}

function stop(facts) {
  const denied = authorization(facts, "control");
  if (denied) return denied;
  if (!facts.run.owner) return "ERROR_CODE_NOT_EXECUTION_OWNER";
  return "STOP_RECEIPT";
}

function stopOutcome(facts) {
  const { stop: state } = facts;
  if (state.cleanupFailed) return state.coreAvailable && state.ownerAcknowledged ? "RUN_FAILED" : "FAILURE_PENDING_REPORT";
  if (!state.cleanupVerified || !state.ownerAcknowledged || !state.coreAvailable) return "CANCEL_PENDING";
  return "RUN_CANCELLED";
}

function output(facts) {
  const { activity } = facts;
  if (authorization(facts, "observe") || grantAuthority(facts)) return "DROP";
  if (!streamMatches(facts) || !activity.authorized) return "DROP";
  if (activity.kind !== "agent_text" && activity.kind !== "tool_label") return "DROP";
  const limit = activity.kind === "agent_text" ? 8192 : 128;
  if (Buffer.byteLength(activity.text, "utf8") > limit) return "ERROR_CODE_PAYLOAD_TOO_LARGE";
  return activity.kind === "agent_text" ? "TEXT" : "TOOL_LABEL";
}

function approvalBinding(facts) {
  const decision = facts.approval;
  return decision.source === "core" && decision.actionMatches && decision.argumentsMatch
    && decision.scopeMatches && decision.principalMatches && decision.notExpired && decision.notRevoked;
}

function approval(facts) {
  if (!approvalBinding(facts) || inputAuthority(facts)) return "DENY_PROTECTED_EFFECT";
  if (facts.approval.runCancelled) return "DENY_PROTECTED_EFFECT";
  return "ALLOW_PROTECTED_EFFECT";
}

export const decisions = { input, watch, stop, stopOutcome, output, approval };
