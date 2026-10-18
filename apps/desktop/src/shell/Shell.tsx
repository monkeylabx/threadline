import { useEffect, useState } from "react";

import type { Channel, ConnectionState, Message, Repository } from "../repository.ts";
import { Composer } from "./Composer.tsx";
import { OfflineBanner } from "./OfflineBanner.tsx";
import { Sidebar } from "./Sidebar.tsx";
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
  const [channels, setChannels] = useState<readonly Channel[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [messages, setMessages] = useState<readonly Message[]>([]);
  const [connection, setConnection] = useState<ConnectionState>(repository.connectionState());

  useEffect(() => {
    let cancelled = false;
    void repository.listChannels().then((list) => {
      if (cancelled) return;
      setChannels(list);
      setSelectedId((current) => current ?? list[0]?.channelId ?? null);
    });
    return repository.onConnectionChange((state) => {
      cancelled = cancelled || false;
      setConnection(state);
    });
  }, [repository]);

  useEffect(() => {
    if (selectedId === null) return;
    let cancelled = false;
    const refresh = () => {
      void repository.listMessages(selectedId).then((list) => {
        if (!cancelled) setMessages(list);
      });
    };
    refresh();
    const unsubscribe = repository.onMessagesChange((channelId) => {
      if (channelId === selectedId) refresh();
    });
    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [repository, selectedId]);

  const selected = channels.find((channel) => channel.channelId === selectedId) ?? null;

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
    <main className="tl-shell" aria-label="Threadline">
      <Sidebar organization={organization} channels={channels} selectedChannelId={selectedId} onSelect={setSelectedId} />
      <section className="tl-workspace" aria-label="当前会话">
        <header className="tl-workspace-header">
          <span aria-hidden="true">#</span>
          <div>
            <h1>{selected?.name ?? "选择一个频道"}</h1>
            {selected ? <small>{`${selected.memberCount} 位成员`}</small> : null}
          </div>
          <span role="status" aria-live="polite" className="tl-visually-hidden">
            {selected ? `已打开 ${selected.name}` : ""}
          </span>
        </header>
        <OfflineBanner state={connection} />
        <Timeline channelName={selected?.name ?? ""} messages={messages} />
        <Composer channelName={selected?.name ?? ""} disabled={selected === null} onSend={send} />
      </section>
    </main>
  );
}
