// Select the confirmed desktop demo without overriding explicit review routes.
function resolvePrototypeEntry(search) {
  const params = new URLSearchParams(search);
  const explicitRoute = params.has("prototype") || params.has("modal") || params.has("view")
    || (params.has("screen") && params.get("screen") !== "channel");
  if (explicitRoute) return params;
  params.set("screen", "channel");
  params.set("prototype", "im-agent-fusion");
  params.set("variant", "A");
  params.set("ui", "v34");
  return params;
}
