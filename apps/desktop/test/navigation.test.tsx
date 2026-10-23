import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
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

test("history traverses visited channels and a new selection replaces forward history", async () => {
  await openShell();
  const back = screen.getByRole("button", { name: "后退" });
  const forward = screen.getByRole("button", { name: "前进" });
  expect(back.hasAttribute("disabled")).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: /客户端体验/ }));
  await screen.findByRole("heading", { name: "客户端体验" });
  fireEvent.click(back);
  await screen.findByRole("heading", { name: "产品研发" });
  fireEvent.click(forward);
  await screen.findByRole("heading", { name: "客户端体验" });
  fireEvent.click(back);
  await screen.findByRole("heading", { name: "产品研发" });
  fireEvent.click(screen.getByRole("button", { name: /安全与合规/ }));
  await screen.findByRole("heading", { name: "安全与合规" });
  expect(forward.hasAttribute("disabled")).toBe(true);
});

test("sidebar can collapse and resize within V34 bounds without losing selection", async () => {
  await openShell();
  const separator = screen.getByRole("separator", { name: "调整侧栏宽度" });
  expect(separator.getAttribute("aria-valuenow")).toBe("240");
  for (let index = 0; index < 20; index += 1) fireEvent.keyDown(separator, { key: "ArrowRight" });
  expect(separator.getAttribute("aria-valuenow")).toBe("320");
  for (let index = 0; index < 20; index += 1) fireEvent.keyDown(separator, { key: "ArrowLeft" });
  expect(separator.getAttribute("aria-valuenow")).toBe("208");
  fireEvent.doubleClick(separator);
  expect(separator.getAttribute("aria-valuenow")).toBe("240");
  fireEvent.click(screen.getByRole("button", { name: "收起侧栏" }));
  expect(screen.queryByRole("navigation", { name: "频道导航" })).toBeNull();
  const restore = screen.getByRole("button", { name: "展开侧栏" });
  await act(async () => restore.focus());
  fireEvent.click(restore);
  expect(screen.getByRole("button", { name: "产品研发" }).getAttribute("aria-current")).toBe("true");
  expect(document.activeElement).toBe(restore);
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
  fireEvent.pointerDown(separator);
  fireEvent(separator, new MouseEvent("pointermove", { bubbles: true, clientX: 400 }));
  expect(separator.getAttribute("aria-valuenow")).toBe("320");
  fireEvent(separator, new MouseEvent("pointermove", { bubbles: true, clientX: 100 }));
  expect(separator.getAttribute("aria-valuenow")).toBe("208");
});
