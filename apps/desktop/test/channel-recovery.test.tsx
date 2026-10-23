import { act, cleanup, render, renderHook, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { LocaldRepository } from "../src/repository/locald.ts";
import { useChannelDirectory } from "../src/shell/channel-directory.ts";
import { Shell } from "../src/shell/Shell.tsx";
import { bridge, unavailableHost } from "./channel-recovery.fixture.ts";

const repositories: LocaldRepository[] = [];
afterEach(() => {
  cleanup();
  for (const repository of repositories.splice(0)) repository.dispose();
  vi.useRealTimers();
});

function repository(host: ReturnType<typeof bridge>) {
  const result = new LocaldRepository(host.invoke, { pollMs: 0 });
  repositories.push(result);
  return result;
}

test("an offline window discovers its authorized channel when the host recovers", async () => {
  vi.useFakeTimers();
  const host = unavailableHost();
  render(<Shell repository={repository(host)} />);
  await act(async () => {});
  expect(screen.queryByRole("heading", { name: "产品研发" })).toBeNull();
  expect(screen.getByText("暂时无法载入频道，恢复连接后会自动重试。")).toBeTruthy();
  expect(screen.getByText("消息连接已离线")).toBeTruthy();
  host.restore();
  await act(async () => { await vi.advanceTimersByTimeAsync(4_000); });
  expect(screen.getByRole("heading", { name: "产品研发" })).toBeTruthy();
  expect(screen.getByRole("textbox", { name: "发消息到 产品研发 频道" }).hasAttribute("disabled")).toBe(false);
  expect(screen.getByText("消息连接正常")).toBeTruthy();
});

test("repeated unavailable results keep a bounded cadence and stop after success", async () => {
  vi.useFakeTimers();
  const host = unavailableHost();
  const instance = repository(host);
  const { result } = renderHook(() => useChannelDirectory(instance));
  await act(async () => {});
  await act(async () => { await vi.advanceTimersByTimeAsync(3_999); });
  expect(host.rpc).toHaveBeenCalledTimes(1);
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  expect(host.rpc).toHaveBeenCalledTimes(2);
  await act(async () => { await vi.advanceTimersByTimeAsync(4_000); });
  expect(result.current.status).toBe("retrying");
  expect(host.rpc).toHaveBeenCalledTimes(3);
  host.restore();
  await act(async () => { await vi.advanceTimersByTimeAsync(4_000); });
  expect(result.current.channels.map((channel) => channel.name)).toEqual(["产品研发"]);
  expect(result.current.status).toBe("ready");
  await act(async () => { await vi.advanceTimersByTimeAsync(8_000); });
  expect(host.rpc).toHaveBeenCalledTimes(4);
});

const terminal = "频道加载失败，请重新打开窗口后重试。";
test.each([
  ["denied", "没有访问频道的权限。"],
  ["no_session", terminal],
  ["storage", terminal],
  ["history_mismatch", terminal],
  ["invalid_input", terminal],
  ["synthetic-private-error", terminal],
])("%s fails closed without retries or fabricated channels", async (reason, label) => {
  vi.useFakeTimers();
  const host = bridge(() => Promise.reject(reason));
  render(<Shell repository={repository(host)} />);
  await act(async () => {});
  expect(screen.getByText(label)).toBeTruthy();
  expect(screen.queryByRole("heading", { name: "产品研发" })).toBeNull();
  expect(screen.getByRole("textbox").hasAttribute("disabled")).toBe(true);
  expect(document.body.textContent).not.toContain("synthetic-private-error");
  await act(async () => { await vi.advanceTimersByTimeAsync(12_000); });
  expect(host.rpc).toHaveBeenCalledExactlyOnceWith("list_channels");
});

test("an authorized empty directory succeeds without starting retries", async () => {
  vi.useFakeTimers();
  const host = bridge(() => Promise.resolve([]));
  const instance = repository(host);
  const { result } = renderHook(() => useChannelDirectory(instance));
  await act(async () => {});
  expect(result.current.channels).toEqual([]);
  expect(result.current.status).toBe("ready");
  await act(async () => { await vi.advanceTimersByTimeAsync(12_000); });
  expect(host.rpc).toHaveBeenCalledTimes(1);
});

test("malformed DTO conversion is visible and terminal rather than retried", async () => {
  vi.useFakeTimers();
  const host = bridge(() => Promise.resolve(null));
  render(<Shell repository={repository(host)} />);
  await act(async () => {});
  expect(screen.getByText(terminal)).toBeTruthy();
  await act(async () => { await vi.advanceTimersByTimeAsync(8_000); });
  expect(host.rpc).toHaveBeenCalledTimes(1);
});
