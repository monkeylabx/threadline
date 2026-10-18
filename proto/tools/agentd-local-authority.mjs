// Synthetic Run execution admission shared by input and approval fixtures.
import { authorization } from "./agentd-local-binding.mjs";

export function grantAuthority({ run }) {
  if (!run.grant) return "ERROR_CODE_GRANT_REVOKED";
  if (typeof run.grantExpired !== "boolean") return "UNAVAILABLE";
  if (run.grantExpired) return "ERROR_CODE_GRANT_EXPIRED";
  return null;
}

function effectAuthority(facts) {
  const denied = grantAuthority(facts);
  if (denied) return denied;
  const { run } = facts;
  if (!run.route) return "UNAVAILABLE";
  if (!run.budget) return "RESOURCE_EXHAUSTED";
  if (!run.live) return "ERROR_CODE_INVALID_STATE_TRANSITION";
  return null;
}

export function inputAuthority(facts) {
  const denied = authorization(facts, "control");
  if (denied) return denied;
  const { run } = facts;
  if (!run.owner) return "ERROR_CODE_NOT_EXECUTION_OWNER";
  if (!run.lease) return "ERROR_CODE_LEASE_LOST";
  if (!run.fencing) return "ERROR_CODE_FENCING_TOKEN_STALE";
  return effectAuthority(facts);
}
