import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, test } from "vitest";

afterEach(cleanup);

import { FixtureRepository } from "../src/repository.ts";
import { Shell } from "../src/shell/Shell.tsx";

async function renderShell(repository = new FixtureRepository()) {
  render(<Shell repository={repository} />);
  await screen.findByRole("heading", { level: 1, name: "产品研发" });
  await screen.findAllByTestId("message");
  return repository;
}

describe("two-pane shell", () => {
  test("renders the landmarks, the channel list and the selected conversation", async () => {
    await renderShell();
    expect(screen.getByRole("main", { name: "Threadline" })).toBeTruthy();
    const nav = screen.getByRole("navigation", { name: "频道导航" });
    const buttons = within(nav).getAllByRole("button");
    expect(buttons.map((button) => button.textContent)).toEqual(["#产品研发", "#客户端体验4", "#安全与合规"]);
    expect(buttons[0]?.getAttribute("aria-current")).toBe("true");
    expect(await screen.findAllByTestId("message")).toHaveLength(3);
    expect(screen.getByRole("textbox", { name: "发消息到 产品研发 频道" })).toBeTruthy();
  });

  test("selecting another channel switches the timeline and the composer label", async () => {
    await renderShell();
    fireEvent.click(screen.getByRole("button", { name: /客户端体验/ }));
    await screen.findByRole("heading", { level: 1, name: "客户端体验" });
    expect(screen.queryAllByTestId("message")).toHaveLength(0);
    expect(screen.getByRole("textbox", { name: "发消息到 客户端体验 频道" })).toBeTruthy();
  });

  test("sending renders an optimistic pending message that becomes committed", async () => {
    const repository = await renderShell(new FixtureRepository({ autoCommit: false }));
    const input = screen.getByRole("textbox", { name: "发消息到 产品研发 频道" });
    fireEvent.change(input, { target: { value: "先跑最小改动" } });
    fireEvent.click(screen.getByRole("button", { name: "发送" }));
    const pending = await screen.findByText("发送中…");
    const row = pending.closest("[data-testid='message']");
    expect(row?.getAttribute("data-state")).toBe("pending");
    expect((input as HTMLTextAreaElement).value).toBe("");

    const eventId = (await repository.listMessages("channel-product")).at(-1)!.eventId;
    await act(async () => {
      repository.commit(eventId);
    });
    expect(screen.queryByText("发送中…")).toBeNull();
    expect(screen.getAllByTestId("message").at(-1)?.getAttribute("data-state")).toBe("committed");
  });

  test("the offline banner appears and disappears with connectivity", async () => {
    const repository = await renderShell();
    expect(screen.queryByRole("status", { name: "" })).toBeTruthy();
    expect(screen.queryByText(/已离线/)).toBeNull();
    await act(async () => {
      repository.setConnection("offline");
    });
    expect(screen.getByText(/已离线/)).toBeTruthy();
    await act(async () => {
      repository.setConnection("online");
    });
    expect(screen.queryByText(/已离线/)).toBeNull();
  });

  test("an empty draft cannot be sent and Enter submits while Shift+Enter does not", async () => {
    const repository = await renderShell(new FixtureRepository({ autoCommit: false }));
    const input = screen.getByRole("textbox", { name: "发消息到 产品研发 频道" });
    expect((screen.getByRole("button", { name: "发送" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(input, { target: { value: "多行" } });
    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    expect((await repository.listMessages("channel-product")).length).toBe(3);
    fireEvent.keyDown(input, { key: "Enter" });
    expect((await repository.listMessages("channel-product")).length).toBe(4);
  });
});
