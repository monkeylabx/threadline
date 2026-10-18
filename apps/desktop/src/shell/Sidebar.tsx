import type { Channel } from "../repository.ts";

interface SidebarProps {
  readonly organization: string;
  readonly channels: readonly Channel[];
  readonly selectedChannelId: string | null;
  readonly onSelect: (channelId: string) => void;
}

/** Channel list pane. Selection is announced through aria-current. */
export function Sidebar({ organization, channels, selectedChannelId, onSelect }: SidebarProps) {
  return (
    <nav className="tl-sidebar" aria-label="频道导航">
      <div className="tl-sidebar-head">
        <div>
          <strong>{organization}</strong>
          <small>产品与工程</small>
        </div>
      </div>
      <section className="tl-sidebar-section" aria-labelledby="tl-channels-title">
        <div className="tl-section-title" id="tl-channels-title">频道</div>
        <ul>
          {channels.map((channel) => (
            <li key={channel.channelId}>
              <button
                type="button"
                className="tl-channel"
                aria-current={channel.channelId === selectedChannelId ? "true" : undefined}
                onClick={() => onSelect(channel.channelId)}
              >
                <span className="tl-hash" aria-hidden="true">#</span>
                <span className="tl-name">{channel.name}</span>
                {channel.unread > 0 ? (
                  <strong className="tl-unread" aria-label={`${channel.unread} 条未读`}>{channel.unread}</strong>
                ) : null}
              </button>
            </li>
          ))}
        </ul>
      </section>
    </nav>
  );
}
