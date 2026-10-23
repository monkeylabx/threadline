import { vi } from "vitest";
import type { ChannelDto, Invoke, SyncReportDto } from "../src/repository/locald.ts";

export const channels: readonly ChannelDto[] = [{ channel_id: "channel-product", name: "产品研发" }];

/** A script supplies complete DTOs; the cast is only the generic IPC boundary. */
export function bridge(handler: (command: string) => Promise<unknown>) {
  const rpc = vi.fn(handler);
  const invoke: Invoke = async <T>(command: string): Promise<T> => await rpc(command) as T;
  return { invoke, rpc };
}

export function unavailableHost() {
  let online = false;
  const report: SyncReportDto = { committed: 0, deduplicated: 0, retained: 0, applied: 0, has_more: false, online: true };
  const host = bridge((command) => {
    if (command === "list_channels") return online ? Promise.resolve(channels) : Promise.reject("unavailable");
    if (command === "list_messages") return Promise.resolve([]);
    if (command === "sync_channel") return Promise.resolve(report);
    return Promise.reject(new Error("unexpected test command"));
  });
  return { ...host, restore: () => { online = true; } };
}

export function deferred<T>() {
  const actions: { resolve?: (value: T) => void; reject?: (error: unknown) => void } = {};
  const promise = new Promise<T>((resolve, reject) => { actions.resolve = resolve; actions.reject = reject; });
  return {
    promise,
    resolve(value: T) {
      if (!actions.resolve) throw new Error("deferred fixture is not initialized");
      actions.resolve(value);
    },
    reject(error: unknown) {
      if (!actions.reject) throw new Error("deferred fixture is not initialized");
      actions.reject(error);
    },
  };
}
