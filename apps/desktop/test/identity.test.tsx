import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { FixtureRepository } from "../src/repository.ts";
import { identityRole, identityRoles } from "../src/shell/identity.ts";
import { Shell } from "../src/shell/Shell.tsx";

afterEach(cleanup);

test("an author key always maps to the same identity role", () => {
  for (const key of ["孟青", "actor-lin-jia", "?", ""]) {
    expect(identityRoles).toContain(identityRole(key));
    expect(identityRole(key)).toBe(identityRole(key));
  }
  expect(identityRole("")).toBe("clay");
});

test("the prototype's people keep their V34 avatar colors", () => {
  expect([identityRole("孟青"), identityRole("周一鸣"), identityRole("林嘉")]).toEqual(["plum", "clay", "slate"]);
});

test("different authors spread across every identity role", () => {
  const keys = ["actor-lin-jia", "actor-meng-qing", "actor-zhou-yiming", "actor-atlas", "孟青", "周一鸣", "林嘉", "a", "b", "c"];
  expect(new Set(keys.map(identityRole))).toEqual(new Set(identityRoles));
});

test("timeline avatars carry their author's identity role", async () => {
  render(<Shell repository={new FixtureRepository({ autoCommit: false })} />);
  const messages = await screen.findAllByTestId("message");
  for (const message of messages) {
    const author = message.querySelector(".tl-message-meta strong")?.textContent ?? "";
    expect(message.querySelector(".tl-avatar")?.getAttribute("data-identity")).toBe(identityRole(author));
  }
});
