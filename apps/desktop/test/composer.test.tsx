import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { Composer } from "../src/shell/Composer.tsx";

// jsdom has no layout: model the content height as 13px insets plus one 17px row per line.
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

test("rests at one line and labels the toolbar with the keyboard hint", () => {
  const field = renderComposer();
  expect(field.rows).toBe(1);
  expect(field.style.height).toBe("43px");
  expect(screen.getByText("Enter 发送 · Shift+Enter 换行")).toBeTruthy();
  expect(screen.queryByText("消息")).toBeNull();
});

test("grows with the draft and returns to one line after sending", () => {
  const onSend = vi.fn();
  const field = renderComposer(onSend);
  fireEvent.change(field, { target: { value: "第一行\n第二行\n第三行" } });
  expect(field.style.height).toBe("77px");
  fireEvent.keyDown(field, { key: "Enter" });
  expect(onSend).toHaveBeenCalledWith("第一行\n第二行\n第三行");
  expect(field.value).toBe("");
  expect(field.style.height).toBe("43px");
});
