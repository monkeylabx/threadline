import { useEffect, useState } from "react";

/** In-session channel history; navigation never reads message contents. */
export function useChannelHistory(selected: string | null, onSelect: (id: string) => void) {
  const [history, setHistory] = useState({ entries: [] as string[], index: -1 });
  useEffect(() => {
    if (selected && history.index === -1) setHistory({ entries: [selected], index: 0 });
  }, [selected, history.index]);
  const visit = (id: string) => {
    if (id === selected) return;
    const entries = [...history.entries.slice(0, history.index + 1), id];
    setHistory({ entries, index: entries.length - 1 });
    onSelect(id);
  };
  const move = (delta: number) => {
    const index = history.index + delta;
    const id = history.entries[index];
    if (id === undefined) return;
    setHistory({ ...history, index });
    onSelect(id);
  };
  return { visit, move, canBack: history.index > 0, canForward: history.index < history.entries.length - 1 };
}
