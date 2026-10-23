import { useRef, useState, type CSSProperties, type ReactNode } from "react";
import type { Channel, ConnectionState } from "../repository.ts";
import { Icon } from "./Icon.tsx";
import { Sidebar } from "./Sidebar.tsx";
import { useChannelHistory } from "./navigation.ts";

interface LayoutProps {
  readonly organization: string;
  readonly channels: readonly Channel[];
  readonly selectedId: string | null;
  readonly onSelect: (id: string) => void;
  readonly connection: ConnectionState;
  readonly children: ReactNode;
}

/** V34 window frame. Channel data remains owned by the existing Repository. */
export function MessagingLayout(props: LayoutProps) {
  const [collapsed, setCollapsed] = useState(false);
  const [width, setWidth] = useState(240);
  const [query, setQuery] = useState("");
  const search = useRef<HTMLInputElement>(null);
  const history = useChannelHistory(props.selectedId, props.onSelect);
  const channels = props.channels.filter((channel) => channel.name.includes(query.trim()));
  return (
    <main className="tl-shell" aria-label="Threadline" data-collapsed={collapsed}
      style={{ "--tl-sidebar-width": `${width}px` } as CSSProperties}
      onKeyDown={(event) => {
        if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
          event.preventDefault();
          search.current?.focus();
        }
      }}>
      <div className="tl-topbar" role="group" aria-label="窗口工具栏">
        <div className="tl-window-tools">
          <button type="button" aria-label={collapsed ? "展开侧栏" : "收起侧栏"} aria-expanded={!collapsed}
            aria-controls="tl-sidebar-pane" onClick={() => setCollapsed(!collapsed)}><Icon name="sidebar" /></button>
          <button type="button" aria-label="后退" disabled={!history.canBack} onClick={() => history.move(-1)}><Icon name="back" /></button>
          <button type="button" aria-label="前进" disabled={!history.canForward} onClick={() => history.move(1)}><Icon name="forward" /></button>
        </div>
        <label className="tl-search"><Icon name="search" /><input ref={search} type="search" aria-label="搜索频道" placeholder="搜索频道"
          value={query} onChange={(event) => { setQuery(event.target.value); setCollapsed(false); }}
          onKeyDown={(event) => { if (event.key === "Escape") setQuery(""); }} /><kbd>⌘ K</kbd></label>
        <span className="tl-connection" title={props.connection === "online" ? "消息连接正常" : "消息连接已离线"}><i data-online={props.connection === "online"} /></span>
      </div>
      <div className="tl-sidebar-pane" id="tl-sidebar-pane" hidden={collapsed}>
        <Sidebar organization={props.organization} channels={channels} selectedChannelId={props.selectedId} onSelect={history.visit} />
        <SidebarResize width={width} onChange={setWidth} />
      </div>
      {props.children}
    </main>
  );
}

function SidebarResize({ width, onChange }: { readonly width: number; readonly onChange: (width: number) => void }) {
  const change = (value: number) => onChange(Math.max(208, Math.min(320, value)));
  return <div className="tl-sidebar-resizer" role="separator" tabIndex={0} aria-label="调整侧栏宽度"
    aria-orientation="vertical" aria-controls="tl-sidebar-pane" aria-valuemin={208} aria-valuemax={320} aria-valuenow={width}
    onDoubleClick={() => onChange(240)}
    onKeyDown={(event) => {
      const next = { ArrowLeft: width - 10, ArrowRight: width + 10, Home: 208, End: 320 }[event.key];
      if (next === undefined) return;
      event.preventDefault();
      change(next);
    }}
    onPointerDown={(event) => event.currentTarget.setPointerCapture(event.pointerId)}
    onPointerMove={(event) => {
      const pane = event.currentTarget.parentElement;
      if (!pane || !event.currentTarget.hasPointerCapture(event.pointerId)) return;
      change(event.clientX - pane.getBoundingClientRect().left);
    }} />;
}
