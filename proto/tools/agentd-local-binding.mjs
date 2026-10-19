// Bind each fixture request to its authenticated principal and original stream.
function authenticated({ principal, binding }) {
  return principal.present === true && principal.active === true && principal.unexpired === true
    && binding.peerAuthenticated === true
    && typeof principal.session === "string" && principal.session.length > 0
    && typeof principal.window === "string" && principal.window.length > 0;
}

export function authorization(facts, role) {
  const { principal, binding } = facts;
  if (!authenticated(facts)) return "UNAUTHENTICATED";
  if (principal.session !== binding.session || principal.window !== binding.window) return "PERMISSION_DENIED";
  const { request, run } = facts;
  if (principal.tenant !== run.tenant) return "ERROR_CODE_TENANT_MISMATCH";
  if (principal.actor !== run.authorizedActor || principal.device !== run.authorizedDevice) return "PERMISSION_DENIED";
  if (request.run !== run.id) return "PERMISSION_DENIED";
  if (!run[role]) return "PERMISSION_DENIED";
  return null;
}

export function streamMatches(facts) {
  const { activity, principal, request } = facts;
  return request.stream === activity.stream && request.run === activity.originRun
    && principal.tenant === activity.originTenant
    && principal.actor === activity.originActor && principal.device === activity.originDevice
    && principal.session === activity.originSession
    && principal.window === activity.originWindow;
}
