import type { Channel } from "../repository.ts";
import { Icon } from "./Icon.tsx";

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
        <span className="tl-enterprise-logo" aria-hidden="true">{organization.slice(0, 1)}</span>
        <strong>{organization}</strong><span aria-hidden="true">⌄</span>
      </div>
      <div className="tl-app-switch" role="group" aria-label="应用">
        <button type="button" aria-pressed="true"><Icon name="message" />消息</button>
        <button type="button" disabled title="Agent 工作将在任务接入后开放"><Icon name="work" />工作</button>
        <button type="button" disabled title="审批待办将在任务接入后开放"><Icon name="check" />待办</button>
      </div>
      <div className="tl-conversations-title"><strong>会话</strong><button type="button" disabled aria-label="新建会话" title="会话创建尚未接入"><Icon name="plus" /></button></div>
      <section className="tl-sidebar-section" aria-labelledby="tl-channels-title">
        <div className="tl-section-title" id="tl-channels-title">频道</div>
        {channels.length === 0 ? <p className="tl-no-channels">没有匹配的频道</p> : null}
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
      <footer className="tl-device-profile"><span className="tl-avatar" aria-hidden="true"><Icon name="device" /></span><div><strong>当前设备</strong><small>本机消息</small></div></footer>
    </nav>
  );
}
