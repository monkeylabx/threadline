/** Display DTO adapter; the host owns every secret and durable message fact. */
import { ChannelWatch, type WatchOptions } from "./channel-watch.ts";
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
  readonly realtime_woke?: boolean;
}

export type LocaldRepositoryOptions = WatchOptions;

const UNREADABLE_BODY = "（此消息无法显示）";

export class LocaldRepository implements Repository {
  private state: ConnectionState = "online";
  private readonly connectionListeners = new Set<(state: ConnectionState) => void>();
  private readonly messageListeners = new Set<(channelId: string) => void>();
  private readonly commits = new Map<string, (message: Message) => void>();
  private readonly watcher: ChannelWatch;

  constructor(
    private readonly invoke: Invoke,
    options: LocaldRepositoryOptions = {},
  ) {
    this.watcher = new ChannelWatch((channel, current) =>
      this.sync(channel, options.realtime ? "wait_channel" : "sync_channel", current), options);
  }

  async listChannels(): Promise<readonly Channel[]> {
    const channels = await this.invoke<readonly ChannelDto[]>("list_channels");
    return channels.map((channel) => ({ channelId: channel.channel_id, name: channel.name, memberCount: 0, unread: 0 }));
  }

  /** Lists the channel and makes it the polled channel. */
  async listMessages(channelId: string): Promise<readonly Message[]> {
    this.watcher.select(channelId);
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
  async sync(channelId: string, command = "sync_channel", current = () => true): Promise<SyncReportDto | null> {
    try {
      const report = await this.invoke<SyncReportDto>(command, { channelId });
      if (!current()) return null;
      this.setConnection(report.online ? "online" : "offline");
      if (report.committed + report.deduplicated + report.applied > 0) {
        const messages = await this.invoke<readonly MessageDto[]>("list_messages", { channelId });
        if (!current()) return null;
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
    } catch (error) {
      if (current()) this.setConnection(error === "denied" ? "online" : "offline");
      return null;
    }
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
    this.watcher.dispose();
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
