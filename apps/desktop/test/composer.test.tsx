import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { Composer } from "../src/shell/Composer.tsx";

// jsdom has no layout: stub content height independently of the CSS layout clamp.
const scrollHeight = Object.getOwnPropertyDescriptor(Element.prototype, "scrollHeight");
beforeEach(() => {
  Object.defineProperty(HTMLTextAreaElement.prototype, "scrollHeight", {
    configurable: true,
    get(this: HTMLTextAreaElement) { return 26 + 17 * this.value.split("\n").length; },
  });
});
afterEach(() => {
  cleanup();
  delete (HTMLTextAreaElement.prototype as { scrollHeight?: number }).scrollHeight;
  if (scrollHeight) Object.defineProperty(Element.prototype, "scrollHeight", scrollHeight);
});

function renderComposer(onSend = vi.fn()) {
  render(<Composer channelName="产品研发" disabled={false} onSend={onSend} />);
  return screen.getByRole<HTMLTextAreaElement>("textbox", { name: "发消息到 产品研发 频道" });
}

test("places the unavailable tools above the draft and sends below it", () => {
  const field = renderComposer();
  expect(field.rows).toBe(1);
  expect(field.style.height).toBe("43px");
  const toolbar = screen.getByRole("toolbar", { name: "消息工具" });
  const tools = within(toolbar).getAllByRole<HTMLButtonElement>("button");
  const send = screen.getByRole("button", { name: "发送" });
  expect(tools).toHaveLength(6);
  expect(tools.every((tool) => tool.disabled && tool.type === "button")).toBe(true);
  expect(toolbar.nextElementSibling).toBe(field);
  expect(field.nextElementSibling?.contains(send)).toBe(true);
  expect(screen.getByText("Enter 发送 · Shift+Enter 换行")).toBeTruthy();
});

test("fits the draft and clears it after sending", () => {
  const onSend = vi.fn();
  const field = renderComposer(onSend);
  fireEvent.change(field, { target: { value: "第一行\n第二行\n第三行" } });
  expect(field.style.height).toBe("77px");
  fireEvent.keyDown(field, { key: "Enter" });
  expect(onSend).toHaveBeenCalledWith("第一行\n第二行\n第三行");
  expect(field.value).toBe("");
  expect(field.style.height).toBe("43px");
});
