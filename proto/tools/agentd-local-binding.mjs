// Bind each fixture request to its authenticated principal and original stream.
export function authorization(facts, role) {
  const { principal, binding, request, run } = facts;
  if (!principal.present || !principal.active) return "UNAUTHENTICATED";
  if (principal.tenant !== run.tenant) return "ERROR_CODE_TENANT_MISMATCH";
  if (principal.actor !== run.authorizedActor || principal.device !== run.authorizedDevice) return "PERMISSION_DENIED";
  if (principal.window !== binding.window || request.run !== run.id) return "PERMISSION_DENIED";
  if (!run[role]) return "PERMISSION_DENIED";
  return null;
}

export function streamMatches(facts) {
  const { activity, principal, request } = facts;
  return request.stream === activity.stream && request.run === activity.originRun
    && principal.tenant === activity.originTenant
    && principal.actor === activity.originActor && principal.device === activity.originDevice
    && principal.window === activity.originWindow;
}
