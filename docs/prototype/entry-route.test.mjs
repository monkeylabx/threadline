import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { runInNewContext } from "node:vm";

const source = readFileSync(new URL("./entry-route.js", import.meta.url), "utf8");
const resolveEntry = runInNewContext(`${source}\nresolvePrototypeEntry;`, { URLSearchParams });

test("default desktop entries select the confirmed two-pane channel", () => {
  for (const search of ["", "?screen=channel", "?viewport=desktop", "?ui=v33"]) {
    const params = resolveEntry(search);
    assert.equal(params.get("screen"), "channel");
    assert.equal(params.get("prototype"), "im-agent-fusion");
    assert.equal(params.get("variant"), "A");
    assert.equal(params.get("ui"), "v34");
  }
});

test("explicit historical prototypes retain their route and variant", () => {
  for (const search of [
    "?prototype=artifact&variant=B&screen=task-result",
    "?prototype=private-publish&variant=C",
    "?prototype=im-agent-fusion&variant=B",
    "?prototype=classic&screen=channel",
  ]) assert.equal(resolveEntry(search).toString(), new URLSearchParams(search).toString());
});

test("domain deep links and task modal remain available", () => {
  for (const search of ["?screen=approvals", "?screen=tasks", "?screen=admin", "?screen=channel&modal=task", "?view=channel"]) {
    assert.equal(resolveEntry(search).toString(), new URLSearchParams(search).toString());
  }
});

test("default selection retains unrelated parameters and is idempotent", () => {
  const first = resolveEntry("?viewport=desktop&review=local");
  assert.equal(first.get("viewport"), "desktop");
  assert.equal(first.get("review"), "local");
  assert.equal(resolveEntry(first.toString()).toString(), first.toString());
});
