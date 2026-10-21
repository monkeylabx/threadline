import { expect, test } from "vitest";
import { ChannelWatch } from "../src/repository/channel-watch.ts";
import { LocaldRepository, type Invoke, type SyncReportDto } from "../src/repository/locald.ts";

const flush = async () => { for (let i = 0; i < 8; i += 1) await Promise.resolve(); };

function clock() {
  const jobs = new Map<number, { callback: () => void; ms: number }>();
  let id = 0;
  return {
    jobs,
    setTimer: (callback: () => void, ms: number) => { jobs.set(++id, { callback, ms }); return id; },
    clearTimer: (handle: unknown) => { jobs.delete(Number(handle)); },
    run: () => { const entry = jobs.entries().next().value; if (entry) { jobs.delete(entry[0]); entry[1].callback(); } },
  };
}

test("realtime waits immediately then backs off on clean/failed closure", async () => {
  const timer = clock();
  let calls = 0;
  const watch = new ChannelWatch(async () => { calls += 1; return null; }, { ...timer, realtime: true });
  watch.select("a");
  expect([...timer.jobs.values()][0]?.ms).toBe(0);
  timer.run(); await flush();
  expect(calls).toBe(1);
  expect([...timer.jobs.values()][0]?.ms).toBe(4000);
  watch.dispose();
  expect(timer.jobs.size).toBe(0);
});

test("channel switches serialize behind the old wait and invalidate its callback", async () => {
  const timer = clock();
  const channels: string[] = [];
  let current = () => true;
  let resolve: (value: null) => void = () => {};
  const watch = new ChannelWatch((channel, valid) => {
    channels.push(channel); current = valid;
    return new Promise<null>((done) => { resolve = done; });
  }, { ...timer, realtime: true });
  watch.select("a"); timer.run();
  expect(current()).toBe(true);
  watch.select("b"); watch.select("c");
  expect(timer.jobs.size).toBe(0);
  expect(current()).toBe(false);
  resolve(null); await flush(); timer.run();
  expect(channels).toEqual(["a", "c"]);
  watch.dispose(); resolve(null); await flush();
  expect(timer.jobs.size).toBe(0);
});

test("real push refreshes the selected channel without the legacy polling command", async () => {
  const timer = clock();
  const calls: string[] = [];
  const changed: string[] = [];
  const report: SyncReportDto = { committed: 0, deduplicated: 0, retained: 0, applied: 1, has_more: false, online: true, realtime_woke: true };
  const invoke: Invoke = async <T>(command: string): Promise<T> => {
    calls.push(command);
    return (command === "wait_channel" ? report : []) as T;
  };
  const repository = new LocaldRepository(invoke, { ...timer, realtime: true });
  repository.onMessagesChange((channel) => changed.push(channel));
  await repository.listMessages("a"); timer.run(); await flush();
  expect(changed).toEqual(["a"]);
  expect(calls).toContain("wait_channel");
  expect(calls).not.toContain("sync_channel");
  expect([...timer.jobs.values()][0]?.ms).toBe(0);
  repository.dispose();
});

test("disposed or replaced-channel responses cannot change connectivity or notify old UI", async () => {
  const timer = clock();
  let reject: (reason: Error) => void = () => {};
  const invoke: Invoke = <T>(command: string): Promise<T> => command === "wait_channel"
    ? new Promise<T>((_, no) => { reject = no; }) : Promise.resolve([] as T);
  const repository = new LocaldRepository(invoke, { ...timer, realtime: true });
  const changes: string[] = [];
  repository.onMessagesChange((channel) => changes.push(channel));
  await repository.listMessages("a"); timer.run();
  await repository.listMessages("b");
  reject(new Error("old connection")); await flush();
  expect(repository.connectionState()).toBe("online");
  expect(changes).toEqual([]);
  repository.dispose();
  expect(timer.jobs.size).toBe(0);
});

test("authorization denial is distinct from a transport outage", async () => {
  const invoke: Invoke = async () => { throw "denied"; };
  const repository = new LocaldRepository(invoke, { pollMs: 0 });
  expect(await repository.sync("a")).toBeNull();
  expect(repository.connectionState()).toBe("online");
  repository.dispose();
});

test("healthy notifications with failed Core pulls back off instead of looping", async () => {
  const timer = clock();
  let calls = 0;
  const watch = new ChannelWatch(async () => {
    calls += 1;
    return { realtime_woke: true, online: false, applied: 0 };
  }, { ...timer, realtime: true });
  watch.select("a");
  for (let attempt = 0; attempt < 3; attempt += 1) {
    timer.run(); await flush();
    expect([...timer.jobs.values()][0]?.ms).toBe(4000);
  }
  expect(calls).toBe(3);
  watch.dispose();
});

test("subscriber exceptions surface without rewriting network connectivity", async () => {
  const invoke: Invoke = async <T>(command: string): Promise<T> => (command === "sync_channel"
    ? { committed: 0, deduplicated: 0, applied: 1, online: true } : []) as T;
  const repository = new LocaldRepository(invoke, { pollMs: 0 });
  repository.onMessagesChange(() => { throw new Error("subscriber bug"); });
  await expect(repository.sync("a")).rejects.toThrow("subscriber bug");
  expect(repository.connectionState()).toBe("online");
  repository.dispose();
});
