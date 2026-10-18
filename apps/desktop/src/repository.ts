/**
 * Repository is the shell's only data seam. A7 ships the fixture-backed
 * implementation; A8 replaces it with the locald bridge without touching the
 * components. Payload bodies are opaque strings here; nothing claims E2EE.
 */
export type ConnectionState = "online" | "offline";

export type MessageState = "pending" | "committed";

export interface Channel {
  readonly channelId: string;
  readonly name: string;
  readonly memberCount: number;
  readonly unread: number;
}

export interface Message {
  readonly eventId: string;
  readonly channelId: string;
  readonly authorName: string;
  readonly sentAt: string;
  readonly body: string;
  readonly state: MessageState;
  readonly channelSeq?: number;
}

export interface Repository {
  listChannels(): Promise<readonly Channel[]>;
  listMessages(channelId: string): Promise<readonly Message[]>;
  /** Queues the message durably and resolves with its pending copy. */
  enqueue(channelId: string, body: string): Promise<Message>;
  /** Resolves when the queued message is committed (or rejected). */
  awaitCommit(eventId: string): Promise<Message>;
  connectionState(): ConnectionState;
  onConnectionChange(listener: (state: ConnectionState) => void): () => void;
}

const SELF = "林嘉";

/** FixtureRepository mirrors the V34 prototype content with no network. */
export class FixtureRepository implements Repository {
  private readonly channels: Channel[];
  private readonly messages = new Map<string, Message[]>();
  private readonly commits = new Map<string, (message: Message) => void>();
  private readonly listeners = new Set<(state: ConnectionState) => void>();
  private state: ConnectionState;
  private sequence = 100;

  constructor(options: { connection?: ConnectionState; autoCommit?: boolean } = {}) {
    this.state = options.connection ?? "online";
    this.autoCommit = options.autoCommit ?? true;
    this.channels = [
      { channelId: "channel-product", name: "产品研发", memberCount: 18, unread: 0 },
      { channelId: "channel-client", name: "客户端体验", memberCount: 9, unread: 4 },
      { channelId: "channel-security", name: "安全与合规", memberCount: 6, unread: 0 },
    ];
    this.messages.set("channel-product", [
      message("evt-1", "channel-product", "孟青", "10:18", "桌面端离线恢复还有一个问题：网络回来以后，旧的同步进程可能会覆盖新状态。", 1),
      message("evt-2", "channel-product", "周一鸣", "10:21", "同意。建议把 Run Lease 和消息同步分开看。", 2),
      message("evt-3", "channel-product", SELF, "10:34", "先跑最小改动，涉及删除旧状态文件时必须让我审批。", 3),
    ]);
  }

  private readonly autoCommit: boolean;

  listChannels(): Promise<readonly Channel[]> {
    return Promise.resolve(this.channels);
  }

  listMessages(channelId: string): Promise<readonly Message[]> {
    return Promise.resolve(this.messages.get(channelId) ?? []);
  }

  enqueue(channelId: string, body: string): Promise<Message> {
    const eventId = `evt-local-${this.sequence += 1}`;
    const pending: Message = { eventId, channelId, authorName: SELF, sentAt: "现在", body, state: "pending" };
    this.messages.set(channelId, [...(this.messages.get(channelId) ?? []), pending]);
    if (this.autoCommit && this.state === "online") {
      queueMicrotask(() => this.commit(eventId));
    }
    return Promise.resolve(pending);
  }

  awaitCommit(eventId: string): Promise<Message> {
    return new Promise((resolve) => {
      this.commits.set(eventId, resolve);
    });
  }

  /** Test seam: commits one pending message at the next sequence. */
  commit(eventId: string): void {
    for (const [channelId, list] of this.messages) {
      const index = list.findIndex((item) => item.eventId === eventId);
      if (index === -1) continue;
      const committed: Message = { ...list[index]!, state: "committed", channelSeq: this.sequence };
      const next = [...list];
      next[index] = committed;
      this.messages.set(channelId, next);
      this.commits.get(eventId)?.(committed);
      this.commits.delete(eventId);
    }
  }

  connectionState(): ConnectionState {
    return this.state;
  }

  /** Test seam: flips connectivity and notifies subscribers. */
  setConnection(state: ConnectionState): void {
    this.state = state;
    for (const listener of this.listeners) listener(state);
  }

  onConnectionChange(listener: (state: ConnectionState) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }
}

function message(eventId: string, channelId: string, authorName: string, sentAt: string, body: string, channelSeq: number): Message {
  return { eventId, channelId, authorName, sentAt, body, state: "committed", channelSeq };
}
