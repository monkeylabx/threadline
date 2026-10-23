import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { LocaldRepository, type ChannelDto } from "../src/repository/locald.ts";
import { useChannelDirectory } from "../src/shell/channel-directory.ts";
import { bridge, channels, deferred, unavailableHost } from "./channel-recovery.fixture.ts";

afterEach(() => { cleanup(); vi.useRealTimers(); });
function repository(host: ReturnType<typeof bridge>) {
  return new LocaldRepository(host.invoke, { pollMs: 0 });
}

test("unmount clears an already queued directory retry", async () => {
  vi.useFakeTimers();
  const host = unavailableHost(), instance = repository(host);
  const { unmount } = renderHook(() => useChannelDirectory(instance));
  await act(async () => {});
  expect(vi.getTimerCount()).toBe(1);
  unmount();
  host.restore();
  await act(async () => { await vi.advanceTimersByTimeAsync(8_000); });
  expect(vi.getTimerCount()).toBe(0);
  expect(host.rpc).toHaveBeenCalledTimes(1);
});

test("a late transient failure after unmount cannot schedule another attempt", async () => {
  vi.useFakeTimers();
  const pending = deferred<readonly ChannelDto[]>();
  const host = bridge(() => pending.promise), instance = repository(host);
  const { unmount } = renderHook(() => useChannelDirectory(instance));
  unmount();
  await act(async () => { pending.reject("unavailable"); });
  expect(vi.getTimerCount()).toBe(0);
  await act(async () => { await vi.advanceTimersByTimeAsync(8_000); });
  expect(host.rpc).toHaveBeenCalledTimes(1);
});

test("an in-flight discovery never overlaps and cannot publish over a replacement", async () => {
  vi.useFakeTimers();
  const pending = deferred<readonly ChannelDto[]>();
  const oldHost = bridge(() => pending.promise), oldRepository = repository(oldHost);
  const newHost = bridge(() => Promise.resolve(channels)), newRepository = repository(newHost);
  const { result, rerender } = renderHook(({ source }) => useChannelDirectory(source), { initialProps: { source: oldRepository } });
  await act(async () => { await vi.advanceTimersByTimeAsync(20_000); });
  expect(oldHost.rpc).toHaveBeenCalledTimes(1);
  rerender({ source: newRepository });
  expect(result.current.status).toBe("loading");
  expect(result.current.channels).toEqual([]);
  await act(async () => {});
  expect(result.current.status).toBe("ready");
  await act(async () => { pending.resolve([{ channel_id: "old-channel", name: "旧数据源" }]); });
  expect(result.current.channels.map((channel) => channel.name)).toEqual(["产品研发"]);
  expect(result.current.status).toBe("ready");
});

test("replacing a repository clears its scheduled retry", async () => {
  vi.useFakeTimers();
  const oldHost = unavailableHost(), oldRepository = repository(oldHost);
  const newHost = bridge(() => Promise.resolve(channels)), newRepository = repository(newHost);
  const { result, rerender } = renderHook(({ source }) => useChannelDirectory(source), { initialProps: { source: oldRepository } });
  await act(async () => {});
  expect(result.current.status).toBe("retrying");
  rerender({ source: newRepository });
  await act(async () => { await vi.advanceTimersByTimeAsync(8_000); });
  expect(result.current.status).toBe("ready");
  expect(oldHost.rpc).toHaveBeenCalledTimes(1);
  expect(vi.getTimerCount()).toBe(0);
});
