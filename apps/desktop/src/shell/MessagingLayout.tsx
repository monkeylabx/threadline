import { useRef, useState, type CSSProperties, type ReactNode } from "react";
import type { Channel, ConnectionState } from "../repository.ts";
import { Icon } from "./Icon.tsx";
import { Sidebar } from "./Sidebar.tsx";

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
  const [width, setWidth] = useState(240);
  const [query, setQuery] = useState("");
  const search = useRef<HTMLInputElement>(null);
  const channels = props.channels.filter((channel) => channel.name.includes(query.trim()));
  return (
    <main className="tl-shell" aria-label="Threadline"
      style={{ "--tl-sidebar-width": `${width}px` } as CSSProperties}
      onKeyDown={(event) => {
        if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
          event.preventDefault();
          search.current?.focus();
        }
      }}>
      <div className="tl-topbar" role="group" aria-label="窗口工具栏" data-tauri-drag-region>
        <span className="tl-titlebar-gutter" aria-hidden="true" data-tauri-drag-region />
        <label className="tl-search"><Icon name="search" /><input ref={search} type="search" aria-label="搜索频道" placeholder="搜索频道"
          value={query} onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => { if (event.key === "Escape") setQuery(""); }} /><kbd>⌘ K</kbd></label>
        <span className="tl-titlebar-gutter" aria-hidden="true" data-tauri-drag-region />
      </div>
      <div className="tl-sidebar-pane" id="tl-sidebar-pane">
        <Sidebar organization={props.organization} channels={channels} selectedChannelId={props.selectedId}
          onSelect={props.onSelect} connection={props.connection} />
        <SidebarResize width={width} onChange={setWidth} />
      </div>
      {props.children}
    </main>
  );
}

function SidebarResize({ width, onChange }: { readonly width: number; readonly onChange: (width: number) => void }) {
  const drag = useRef<{ readonly x: number; readonly width: number } | null>(null);
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
    onPointerDown={(event) => {
      drag.current = { x: event.clientX, width };
      event.currentTarget.setPointerCapture(event.pointerId);
    }}
    onLostPointerCapture={() => { drag.current = null; }}
    onPointerMove={(event) => {
      const origin = drag.current;
      if (!origin || !event.currentTarget.hasPointerCapture(event.pointerId)) return;
      change(origin.width + event.clientX - origin.x);
    }} />;
}
