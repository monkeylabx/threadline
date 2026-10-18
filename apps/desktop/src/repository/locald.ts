/**
 * LocaldRepository talks to the Tauri host's window-scoped commands. The host
 * owns the encrypted store, the Core client and every secret; this class only
 * maps DTOs onto the shell's Repository and drives sync explicitly: once after
 * each send and on a bounded poll while a channel is open. Push arrives with
 * A5 (#255); until then the poll is the only way another Device's messages
 * show up.
 */
import type { Channel, ConnectionState, Message, MessageState, Repository } from "../repository.ts";

export type Invoke = <T>(command: string, args?: Record<string, unknown>) => Promise<T>;

export interface ChannelDto {
  readonly channel_id: string;
  readonly name: string;
}

export interface MessageDto {
  readonly event_id: string;
  readonly channel_id: string;
  readonly author_actor_id: string;
  readonly author_device_id: string;
  readonly text: string;
  readonly state: MessageState;
  readonly channel_seq: number | null;
  readonly sent_at_unix_ms: number | null;
  readonly committed_at_unix_ms: number | null;
}

export interface SyncReportDto {
  readonly committed: number;
  readonly deduplicated: number;
  readonly retained: number;
  readonly applied: number;
  readonly has_more: boolean;
  readonly online: boolean;
}

export interface LocaldRepositoryOptions {
  /** Poll interval while a channel is open; 0 disables polling (tests). */
  readonly pollMs?: number;
  readonly setTimer?: (callback: () => void, ms: number) => unknown;
  readonly clearTimer?: (handle: unknown) => void;
}

const UNREADABLE_BODY = "（此消息无法显示）";

export class LocaldRepository implements Repository {
  private state: ConnectionState = "online";
  private readonly connectionListeners = new Set<(state: ConnectionState) => void>();
  private readonly messageListeners = new Set<(channelId: string) => void>();
  private readonly commits = new Map<string, (message: Message) => void>();
  private readonly pollMs: number;
  private readonly setTimer: (callback: () => void, ms: number) => unknown;
  private readonly clearTimer: (handle: unknown) => void;
  private pollHandle: unknown = null;
  private polledChannel: string | null = null;

  constructor(
    private readonly invoke: Invoke,
    options: LocaldRepositoryOptions = {},
  ) {
    this.pollMs = options.pollMs ?? 4000;
    this.setTimer = options.setTimer ?? ((callback, ms) => setTimeout(callback, ms));
    this.clearTimer = options.clearTimer ?? ((handle) => clearTimeout(handle as ReturnType<typeof setTimeout>));
  }

  async listChannels(): Promise<readonly Channel[]> {
    const channels = await this.invoke<readonly ChannelDto[]>("list_channels");
    return channels.map((channel) => ({ channelId: channel.channel_id, name: channel.name, memberCount: 0, unread: 0 }));
  }

  /** Lists the channel and makes it the polled channel. */
  async listMessages(channelId: string): Promise<readonly Message[]> {
    this.watch(channelId);
    const messages = await this.invoke<readonly MessageDto[]>("list_messages", { channelId });
    return messages.map(toMessage);
  }

  async enqueue(channelId: string, body: string): Promise<Message> {
    const pending = await this.invoke<MessageDto>("send_message", { channelId, text: body });
    void this.sync(channelId);
    return toMessage(pending);
  }

  awaitCommit(eventId: string): Promise<Message> {
    return new Promise((resolve) => {
      this.commits.set(eventId, resolve);
    });
  }

  /** Replays pending sends and pulls new history; resolves pending commits. */
  async sync(channelId: string): Promise<SyncReportDto | null> {
    let report: SyncReportDto;
    try {
      report = await this.invoke<SyncReportDto>("sync_channel", { channelId });
    } catch {
      this.setConnection("offline");
      return null;
    }
    this.setConnection(report.online ? "online" : "offline");
    if (report.committed + report.deduplicated + report.applied > 0) {
      const messages = await this.invoke<readonly MessageDto[]>("list_messages", { channelId });
      for (const dto of messages) {
        const resolve = dto.state === "committed" ? this.commits.get(dto.event_id) : undefined;
        if (resolve) {
          resolve(toMessage(dto));
          this.commits.delete(dto.event_id);
        }
      }
      for (const listener of this.messageListeners) listener(channelId);
    }
    return report;
  }

  connectionState(): ConnectionState {
    return this.state;
  }

  onConnectionChange(listener: (state: ConnectionState) => void): () => void {
    this.connectionListeners.add(listener);
    return () => this.connectionListeners.delete(listener);
  }

  onMessagesChange(listener: (channelId: string) => void): () => void {
    this.messageListeners.add(listener);
    return () => this.messageListeners.delete(listener);
  }

  /** Stops the poll; call when the window closes or the repository is swapped. */
  dispose(): void {
    if (this.pollHandle !== null) this.clearTimer(this.pollHandle);
    this.pollHandle = null;
    this.polledChannel = null;
  }

  private watch(channelId: string): void {
    if (this.polledChannel === channelId || this.pollMs <= 0) return;
    this.dispose();
    this.polledChannel = channelId;
    const tick = () => {
      if (this.polledChannel !== channelId) return;
      void this.sync(channelId).finally(() => {
        if (this.polledChannel === channelId) this.pollHandle = this.setTimer(tick, this.pollMs);
      });
    };
    this.pollHandle = this.setTimer(tick, this.pollMs);
  }

  private setConnection(state: ConnectionState): void {
    if (state === this.state) return;
    this.state = state;
    for (const listener of this.connectionListeners) listener(state);
  }
}

function toMessage(dto: MessageDto): Message {
  const unreadable = dto.state === "unreadable";
  return {
    eventId: dto.event_id,
    channelId: dto.channel_id,
    authorName: unreadable ? "?" : dto.author_actor_id,
    sentAt: formatClock(dto.committed_at_unix_ms ?? dto.sent_at_unix_ms),
    body: unreadable ? UNREADABLE_BODY : dto.text,
    state: dto.state,
    ...(dto.channel_seq === null ? {} : { channelSeq: dto.channel_seq }),
  };
}

export function formatClock(unixMs: number | null): string {
  if (unixMs === null) return "";
  const date = new Date(unixMs);
  return `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}
