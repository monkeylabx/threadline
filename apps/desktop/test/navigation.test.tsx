import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { FixtureRepository } from "../src/repository.ts";
import { Shell } from "../src/shell/Shell.tsx";

afterEach(cleanup);
async function openShell() {
  render(<Shell repository={new FixtureRepository({ autoCommit: false })} />);
  await screen.findByRole("heading", { level: 1, name: "产品研发" });
}

test("channel search filters navigation without replacing the open conversation", async () => {
  await openShell();
  const search = screen.getByRole("searchbox", { name: "搜索频道" });
  fireEvent.change(search, { target: { value: " 客户端 " } });
  expect(screen.queryByRole("button", { name: "产品研发" })).toBeNull();
  expect(screen.getByRole("heading", { name: "产品研发" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /客户端体验/ }));
  await screen.findByRole("heading", { name: "客户端体验" });
  fireEvent.keyDown(search, { key: "Escape" });
  expect(screen.getByRole("button", { name: "产品研发" })).toBeTruthy();
  fireEvent.change(search, { target: { value: "不存在" } });
  expect(screen.getByText("没有匹配的频道")).toBeTruthy();
});

test("the toolbar contains search and leaves native window controls to the host", async () => {
  await openShell();
  const toolbar = screen.getByRole("group", { name: "窗口工具栏" });
  expect(toolbar.querySelectorAll("button")).toHaveLength(0);
  expect(toolbar.querySelector("input[type=search]")).toBe(screen.getByRole("searchbox", { name: "搜索频道" }));
  expect(screen.queryByRole("button", { name: /后退|前进|收起侧栏|展开侧栏/ })).toBeNull();
  expect(screen.getByRole("heading", { name: "产品研发" }).closest(".tl-topbar")).toBeNull();
});

test("application entries are named icon-only controls with honest unavailable states", async () => {
  await openShell();
  for (const name of ["消息", "私人工作", "待办"]) {
    const button = screen.getByRole("button", { name });
    expect(button.textContent).toBe("");
    expect(button.querySelector("svg")).toBeTruthy();
    expect(button.getAttribute("title")).toBeTruthy();
    expect(button.hasAttribute("disabled")).toBe(name !== "消息");
  }
  expect(screen.getByRole("button", { name: "消息" }).getAttribute("aria-pressed")).toBe("true");
});

test("sidebar resizes within V34 bounds without losing selection", async () => {
  await openShell();
  const separator = screen.getByRole("separator", { name: "调整侧栏宽度" });
  expect(separator.getAttribute("aria-valuenow")).toBe("240");
  for (let index = 0; index < 20; index += 1) fireEvent.keyDown(separator, { key: "ArrowRight" });
  expect(separator.getAttribute("aria-valuenow")).toBe("320");
  for (let index = 0; index < 20; index += 1) fireEvent.keyDown(separator, { key: "ArrowLeft" });
  expect(separator.getAttribute("aria-valuenow")).toBe("208");
  fireEvent.doubleClick(separator);
  expect(separator.getAttribute("aria-valuenow")).toBe("240");
  expect(screen.getByRole("button", { name: "产品研发" }).getAttribute("aria-current")).toBe("true");
});

test("IME confirmation does not send a draft", async () => {
  await openShell();
  const input = screen.getByRole("textbox", { name: "发消息到 产品研发 频道" });
  fireEvent.change(input, { target: { value: "中文输入" } });
  fireEvent.keyDown(input, { key: "Enter", isComposing: true });
  expect(screen.queryByText("中文输入", { selector: "p" })).toBeNull();
  expect((input as HTMLTextAreaElement).value).toBe("中文输入");
  fireEvent.keyDown(input, { key: "Enter", keyCode: 229 });
  expect(screen.queryByText("中文输入", { selector: "p" })).toBeNull();
  fireEvent.keyDown(input, { key: "Enter" });
  expect(await screen.findByText("中文输入", { selector: "p" })).toBeTruthy();
});


test("search shortcut focuses the field and unsupported resize keys preserve width", async () => {
  await openShell();
  const composer = screen.getByRole("textbox", { name: /发消息到 产品研发/ });
  fireEvent.keyDown(composer, { key: "k", metaKey: true });
  expect(document.activeElement).toBe(screen.getByRole("searchbox", { name: "搜索频道" }));
  const separator = screen.getByRole("separator");
  fireEvent.keyDown(separator, { key: "a" });
  expect(separator.getAttribute("aria-valuenow")).toBe("240");
  fireEvent.keyDown(separator, { key: "End" });
  expect(separator.getAttribute("aria-valuenow")).toBe("320");
  fireEvent.keyDown(separator, { key: "Home" });
  expect(separator.getAttribute("aria-valuenow")).toBe("208");
});

test("pointer resizing requires capture and clamps to the accepted range", async () => {
  await openShell();
  const separator = screen.getByRole("separator");
  let captured = false;
  Object.assign(separator, { setPointerCapture: () => { captured = true; }, hasPointerCapture: () => captured });
  fireEvent(separator, new MouseEvent("pointermove", { bubbles: true, clientX: 400 }));
  expect(separator.getAttribute("aria-valuenow")).toBe("240");
  fireEvent(separator, new MouseEvent("pointerdown", { bubbles: true, clientX: 240 }));
  fireEvent(separator, new MouseEvent("pointermove", { bubbles: true, clientX: 400 }));
  expect(separator.getAttribute("aria-valuenow")).toBe("320");
  fireEvent(separator, new MouseEvent("pointermove", { bubbles: true, clientX: 100 }));
  expect(separator.getAttribute("aria-valuenow")).toBe("208");
});


test("pointer movement preserves its delta when the navigation panel has outer insets", async () => {
  await openShell();
  const separator = screen.getByRole("separator");
  const pane = separator.parentElement;
  if (!pane) throw new Error("missing navigation pane");
  Object.assign(pane, { getBoundingClientRect: () => new DOMRect(16, 104, 208, 540) });
  let captured = false;
  Object.assign(separator, { setPointerCapture: () => { captured = true; }, hasPointerCapture: () => captured });
  fireEvent(separator, new MouseEvent("pointerdown", { bubbles: true, clientX: 228 }));
  fireEvent(separator, new MouseEvent("pointermove", { bubbles: true, clientX: 238 }));
  expect(separator.getAttribute("aria-valuenow")).toBe("250");
  fireEvent.lostPointerCapture(separator);
  fireEvent(separator, new MouseEvent("pointermove", { bubbles: true, clientX: 300 }));
  expect(separator.getAttribute("aria-valuenow")).toBe("250");
});

test("unread channels are emphasised and signalled on the message entry even when filtered out", async () => {
  await openShell();
  const rows = screen.getAllByRole("button").filter((button) => button.classList.contains("tl-channel"));
  expect(rows.map((row) => row.getAttribute("data-unread"))).toEqual([null, "true", null]);
  expect(screen.getByTestId("message-signal")).toBeTruthy();
  expect(screen.getByRole("button", { name: "消息" }).getAttribute("title")).toBe("消息 · 有未读");
  fireEvent.change(screen.getByRole("searchbox", { name: "搜索频道" }), { target: { value: "安全" } });
  expect(screen.queryByRole("button", { name: /客户端体验/ })).toBeNull();
  expect(screen.getByTestId("message-signal")).toBeTruthy();
});

test("the message entry carries no signal when every channel is read", async () => {
  class ReadRepository extends FixtureRepository {
    override async listChannels() {
      return (await super.listChannels()).map((channel) => ({ ...channel, unread: 0 }));
    }
  }
  render(<Shell repository={new ReadRepository({ autoCommit: false })} />);
  await screen.findByRole("heading", { level: 1, name: "产品研发" });
  expect(screen.queryByTestId("message-signal")).toBeNull();
  expect(screen.getByRole("button", { name: "消息" }).getAttribute("title")).toBe("消息");
  const rows = screen.getAllByRole("button").filter((button) => button.classList.contains("tl-channel"));
  expect(rows.every((row) => !row.hasAttribute("data-unread"))).toBe(true);
});
