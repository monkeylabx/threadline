import { useEffect, useState } from "react";
import type { Channel, Repository } from "../repository.ts";
import { ChannelDirectoryFailure } from "../repository/channel-directory.ts";

export type DirectoryStatus = "loading" | "ready" | "retrying" | "denied" | "failed";
interface DirectoryView {
  readonly channels: readonly Channel[];
  readonly status: DirectoryStatus;
}
const EMPTY: DirectoryView = { channels: [], status: "loading" };

/** One active startup discovery, retried only after a transient host failure. */
export function useChannelDirectory(repository: Repository): DirectoryView {
  const [view, setView] = useState({ ...EMPTY, owner: repository });
  useEffect(() => {
    let active = true;
    let retry: ReturnType<typeof setTimeout> | undefined;
    const load = async () => {
      try {
        const channels = await repository.listChannels();
        if (active) setView({ channels, status: "ready", owner: repository });
      } catch (error) {
        if (!active) return;
        const status = failureStatus(error);
        setView({ channels: EMPTY.channels, status, owner: repository });
        if (status === "retrying") retry = setTimeout(() => { void load(); }, 4_000);
      }
    };
    void load();
    return () => {
      active = false;
      clearTimeout(retry);
    };
  }, [repository]);
  return view.owner === repository ? view : EMPTY;
}

function failureStatus(error: unknown): DirectoryStatus {
  if (!(error instanceof ChannelDirectoryFailure)) return "failed";
  if (error.reason === "unavailable") return "retrying";
  return error.reason === "denied" ? "denied" : "failed";
}
