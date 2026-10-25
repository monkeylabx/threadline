import { describe, expect, test } from "vitest";

import { formatClock, LocaldRepository, type MessageDto, type SyncReportDto } from "../src/repository/locald.ts";

/** A scripted host: records invocations, commits on sync when online. */
class FakeHost {
  readonly calls: Array<{ command: string; args?: Record<string, unknown> }> = [];
  online = true;
  rejectSync = false;
  applied = 0;
  private readonly messages = new Map<string, MessageDto[]>();
  private sequence = 0;

  readonly invoke = <T>(command: string, args?: Record<string, unknown>): Promise<T> => {
    this.calls.push({ command, ...(args ? { args } : {}) });
    const channelId = String(args?.["channelId"] ?? "");
    switch (command) {
      case "list_channels":
        return Promise.resolve([{ channel_id: "channel-product", name: "产品研发" }] as T);
      case "list_messages":
        return Promise.resolve([...(this.messages.get(channelId) ?? [])] as T);
      case "send_message": {
        const pending = dto(`evt-${this.messages.size}-${this.calls.length}`, channelId, String(args?.["text"]), null);
        this.messages.set(channelId, [...(this.messages.get(channelId) ?? []), pending]);
        return Promise.resolve(pending as T);
      }
      case "sync_channel": {
        if (this.rejectSync) return Promise.reject({ code: "storage" });
        let committed = 0;
        if (this.online) {
          const list = (this.messages.get(channelId) ?? []).map((item) =>
            item.state === "pending" ? ((committed += 1), { ...item, state: "committed" as const, channel_seq: (this.sequence += 1), committed_at_unix_ms: 1_700_000_000_000 }) : item,
          );
          this.messages.set(channelId, list);
        }
        const report: SyncReportDto = { committed, deduplicated: 0, retained: this.online ? 0 : 1, applied: this.applied, has_more: false, online: this.online };
        this.applied = 0;
        return Promise.resolve(report as T);
      }
      default:
        return Promise.reject(new Error(`unexpected ${command}`));
    }
  };

  /** Another Device's message arrives on the server. */
  arrive(channelId: string, text: string): void {
    const incoming: MessageDto = { ...dto(`evt-other-${this.sequence}`, channelId, text, (this.sequence += 1)), author_actor_id: "actor-meng-qing" };
    this.messages.set(channelId, [...(this.messages.get(channelId) ?? []), incoming]);
  }

  unreadable(channelId: string): void {
    this.messages.set(channelId, [...(this.messages.get(channelId) ?? []), { ...dto("evt-x", channelId, "", 9), state: "unreadable" }]);
  }
}

function dto(eventId: string, channelId: string, text: string, channelSeq: number | null): MessageDto {
  return {
    event_id: eventId,
    channel_id: channelId,
    author_actor_id: "actor-lin-jia",
    author_device_id: "device-lin-jia-laptop",
    text,
    state: channelSeq === null ? "pending" : "committed",
    channel_seq: channelSeq,
    sent_at_unix_ms: 1_700_000_000_000,
    committed_at_unix_ms: channelSeq === null ? null : 1_700_000_000_000,
  };
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe("LocaldRepository", () => {
  test("maps channels and messages from host DTOs", async () => {
    const host = new FakeHost();
    const repository = new LocaldRepository(host.invoke, { pollMs: 0 });
    host.arrive("channel-product", "来自孟青");
    host.unreadable("channel-product");
    expect(await repository.listChannels()).toEqual([{ channelId: "channel-product", name: "产品研发", memberCount: 0, unread: 0 }]);
    const messages = await repository.listMessages("channel-product");
    expect(messages[0]).toMatchObject({ authorName: "actor-meng-qing", body: "来自孟青", state: "committed", channelSeq: 1 });
    expect(messages[0]?.sentAtMs).toBe(1_700_000_000_000);
    expect(messages[1]).toMatchObject({ authorName: "?", body: "（此消息无法显示）", state: "unreadable" });
    expect(host.calls.find((call) => call.command === "list_messages")?.args).toEqual({ channelId: "channel-product" });
  });

  test("enqueue returns the pending copy, syncs once and resolves awaitCommit", async () => {
    const host = new FakeHost();
    const repository = new LocaldRepository(host.invoke, { pollMs: 0 });
    const changed: string[] = [];
    repository.onMessagesChange((channelId) => changed.push(channelId));

    const pending = await repository.enqueue("channel-product", "先跑最小改动");
    expect(pending.state).toBe("pending");
    const committed = await repository.awaitCommit(pending.eventId);
    expect(committed).toMatchObject({ eventId: pending.eventId, state: "committed", channelSeq: 1, body: "先跑最小改动" });
    expect(host.calls.filter((call) => call.command === "sync_channel")).toHaveLength(1);
    expect(host.calls.find((call) => call.command === "send_message")?.args).toEqual({ channelId: "channel-product", text: "先跑最小改动" });
    expect(changed).toEqual(["channel-product"]);
    expect(repository.connectionState()).toBe("online");
  });

  test("an offline sync flips the connection, recovery flips it back and commits exactly once", async () => {
    const host = new FakeHost();
    const repository = new LocaldRepository(host.invoke, { pollMs: 0 });
    const states: string[] = [];
    repository.onConnectionChange((state) => states.push(state));

    host.online = false;
    const pending = await repository.enqueue("channel-product", "稍后发送");
    await flush();
    expect(repository.connectionState()).toBe("offline");
    expect((await repository.listMessages("channel-product")).map((item) => item.state)).toEqual(["pending"]);

    host.online = true;
    const commit = repository.awaitCommit(pending.eventId);
    await repository.sync("channel-product");
    expect((await commit).channelSeq).toBe(1);
    expect(await repository.listMessages("channel-product")).toHaveLength(1);
    expect(states).toEqual(["offline", "online"]);
  });

  test("a rejected sync command counts as offline without losing the queue", async () => {
    const host = new FakeHost();
    const repository = new LocaldRepository(host.invoke, { pollMs: 0 });
    host.rejectSync = true;
    await repository.enqueue("channel-product", "x");
    await flush();
    expect(repository.connectionState()).toBe("offline");
    expect(await repository.listMessages("channel-product")).toHaveLength(1);
  });

  test("polling the open channel surfaces another Device's messages and stops on dispose", async () => {
    const host = new FakeHost();
    const timers: Array<() => void> = [];
    const repository = new LocaldRepository(host.invoke, {
      pollMs: 10,
      setTimer: (callback) => (timers.push(callback), timers.length),
      clearTimer: () => timers.splice(0),
    });
    const changed: string[] = [];
    repository.onMessagesChange((channelId) => changed.push(channelId));
    await repository.listMessages("channel-product");
    expect(timers).toHaveLength(1);

    host.arrive("channel-product", "新消息");
    host.applied = 1;
    timers.shift()?.();
    await flush();
    await flush();
    expect(changed).toEqual(["channel-product"]);
    expect(timers).toHaveLength(1);

    repository.dispose();
    expect(timers).toHaveLength(0);
  });
});

test("formatClock renders local HH:MM or nothing", () => {
  const sample = new Date(2026, 9, 18, 9, 5).getTime();
  expect(formatClock(sample)).toBe("09:05");
  expect(formatClock(null)).toBe("");
});

test("messages without host timestamps carry no day", async () => {
  const invoke = (<T,>(command: string) => Promise.resolve((command === "list_messages"
    ? [{ event_id: "evt-n", channel_id: "c", author_actor_id: "a", author_device_id: "d", text: "t", state: "pending", channel_seq: null, sent_at_unix_ms: null, committed_at_unix_ms: null }]
    : { committed: 0, deduplicated: 0, retained: 0, applied: 0, has_more: false, online: true }) as T));
  const repository = new LocaldRepository(invoke, { pollMs: 0 });
  const [message] = await repository.listMessages("c");
  expect(message?.sentAt).toBe("");
  expect(message && "sentAtMs" in message).toBe(false);
  repository.dispose();
});
