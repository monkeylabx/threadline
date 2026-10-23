import { useEffect, useState } from "react";

import type { Channel, ConnectionState, Message, Repository } from "../repository.ts";
import { useChannelDirectory, type DirectoryStatus } from "./channel-directory.ts";
import { Composer } from "./Composer.tsx";
import { OfflineBanner } from "./OfflineBanner.tsx";
import { MessagingLayout } from "./MessagingLayout.tsx";
import { Timeline } from "./Timeline.tsx";

interface ShellProps {
  readonly repository: Repository;
  readonly organization?: string;
}

/**
 * Two-pane IM shell: channel list and the selected conversation. State lives
 * here; every read and write goes through the Repository so A8 can swap the
 * fixtures for locald without touching the panes.
 */
export function Shell({ repository, organization = "北辰科技" }: ShellProps) {
  const directory = useChannelDirectory(repository);
  const channels = directory.channels;
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [messages, setMessages] = useState<readonly Message[]>([]);
  const [connection, setConnection] = useState<ConnectionState>(repository.connectionState());

  useEffect(() => repository.onConnectionChange(setConnection), [repository]);
  useEffect(() => {
    setSelectedId(channels[0]?.channelId ?? null);
  }, [channels]);
  const selected = channels.find((channel) => channel.channelId === selectedId) ?? null;
  const activeChannel = selected?.channelId ?? null;

  useEffect(() => {
    if (activeChannel === null) return;
    let cancelled = false;
    const refresh = () => {
      void repository.listMessages(activeChannel).then((list) => {
        if (!cancelled) setMessages(list);
      });
    };
    refresh();
    const unsubscribe = repository.onMessagesChange((channelId) => {
      if (channelId === activeChannel) refresh();
    });
    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [repository, activeChannel]);

  const send = (body: string) => {
    if (selected === null) return;
    void repository.enqueue(selected.channelId, body).then((pending) => {
      setMessages((current) => [...current, pending]);
      void repository.awaitCommit(pending.eventId).then((committed) => {
        setMessages((current) => current.map((item) => (item.eventId === committed.eventId ? committed : item)));
      });
    });
  };

  return (
    <MessagingLayout organization={organization} channels={channels} selectedId={selectedId} onSelect={setSelectedId} connection={directory.status === "retrying" ? "offline" : connection}>
      <section className="tl-workspace" aria-label="当前会话">
        <ConversationHeading selected={selected} />
        <DirectoryBanner status={directory.status} connection={connection} />
        <Timeline channelName={selected?.name ?? ""} messages={selected ? messages : []} />
        <Composer channelName={selected?.name ?? ""} disabled={selected === null} onSend={send} />
      </section>
    </MessagingLayout>
  );
}

function ConversationHeading({ selected }: { readonly selected: Channel | null }) {
  return (
    <header className="tl-workspace-header">
  <span aria-hidden="true">#</span>
          <div>
            <h1>{selected?.name ?? "选择一个频道"}</h1>
            {selected ? <small>{selected.memberCount > 0 ? `${selected.memberCount} 位成员` : "频道"}</small> : null}
          </div>
          <span role="status" aria-live="polite" className="tl-visually-hidden">
            {selected ? `已打开 ${selected.name}` : ""}
          </span>
        </header>
  );
}

function DirectoryBanner({ status, connection }: { readonly status: DirectoryStatus; readonly connection: ConnectionState }) {
  const labels: Partial<Record<DirectoryStatus, string>> = {
    retrying: "暂时无法载入频道，恢复连接后会自动重试。",
    denied: "没有访问频道的权限。",
    failed: "频道加载失败，请重新打开窗口后重试。",
  };
  const label = labels[status];
  return label ? <div className="tl-banner" role="status">{label}</div> : <OfflineBanner state={connection} />;
}
