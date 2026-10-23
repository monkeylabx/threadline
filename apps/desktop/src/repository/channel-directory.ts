import type { Channel } from "../repository.ts";
import type { ChannelDto, Invoke } from "./locald.ts";

export type DirectoryFailureReason = "unavailable" | "denied" | "failed";

/** Stable renderer category; raw IPC errors and credentials never enter UI copy. */
export class ChannelDirectoryFailure extends Error {
  constructor(readonly reason: DirectoryFailureReason) {
    super("Channel directory could not be loaded");
  }
}

/** Only the IPC boundary is classified; DTO conversion errors remain terminal. */
export async function loadAuthorizedChannels(invoke: Invoke): Promise<readonly Channel[]> {
  let channels: readonly ChannelDto[];
  try {
    channels = await invoke<readonly ChannelDto[]>("list_channels");
  } catch (error) {
    const reason = error === "unavailable" ? "unavailable" : error === "denied" ? "denied" : "failed";
    throw new ChannelDirectoryFailure(reason);
  }
  return channels.map((channel) => ({ channelId: channel.channel_id, name: channel.name, memberCount: 0, unread: 0 }));
}
