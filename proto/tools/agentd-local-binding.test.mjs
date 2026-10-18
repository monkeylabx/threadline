import assert from "node:assert/strict";
import test from "node:test";
import { authorization } from "./agentd-local-binding.mjs";

test("untrusted bootstrap is refused before any Run fact is read", () => {
  const failures = [
    { present: false }, { active: false }, { unexpired: false },
    { session: null }, { session: "" }, { peerAuthenticated: false },
  ];
  for (const failure of failures) {
    const { peerAuthenticated = true, ...principal } = failure;
    const facts = {
      principal: { present: true, active: true, unexpired: true, session: "session-a", ...principal },
      binding: { peerAuthenticated, session: "session-a" },
      get run() { assert.fail("unauthenticated caller reached Run facts"); },
    };
    assert.equal(authorization(facts, "control"), "UNAUTHENTICATED");
  }
});

test("a different host Session or window is refused before Run facts", () => {
  for (const mismatch of [{ session: "session-b" }, { window: "window-b" }]) {
    const facts = {
      principal: { present: true, active: true, unexpired: true, session: "session-a", window: "window-a" },
      binding: { peerAuthenticated: true, session: "session-a", window: "window-a", ...mismatch },
      get run() { assert.fail("mismatched host binding reached Run facts"); },
    };
    assert.equal(authorization(facts, "control"), "PERMISSION_DENIED");
  }
});
