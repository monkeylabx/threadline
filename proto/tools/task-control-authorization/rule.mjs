// Synthetic contract rules, never production authentication or permission.
export function rule(id, valid, reason = id, status = "PERMISSION_DENIED", code = "PERMISSION_DENIED") {
  return { id, valid, result: { decision: "deny", reason, status, ...(code && { code }) } };
}

export const everyTrue = (facts, keys) => keys.every((key) => facts[key] === true);
export const contains = (value, item) => Array.isArray(value) && value.includes(item);
export const nonempty = (value) => typeof value === "string" && value.length > 0;
