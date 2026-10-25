import type { Message } from "../repository.ts";

function startOfDay(timestampMs: number): number {
  const date = new Date(timestampMs);
  date.setHours(0, 0, 0, 0);
  return date.getTime();
}

/** Divider label for the viewer's local calendar day of `timestampMs`, relative to `nowMs`. */
export function dayLabel(timestampMs: number, nowMs: number): string {
  // Rounding absorbs 23/25-hour days around daylight-saving changes.
  const daysAgo = Math.round((startOfDay(nowMs) - startOfDay(timestampMs)) / 86_400_000);
  if (daysAgo === 0) return "今天";
  if (daysAgo === 1) return "昨天";
  const date = new Date(timestampMs);
  const monthDay = `${date.getMonth() + 1}月${date.getDate()}日`;
  return date.getFullYear() === new Date(nowMs).getFullYear() ? monthDay : `${date.getFullYear()}年${monthDay}`;
}

/**
 * Event ids of the messages that open a new local calendar day, in list order.
 * Messages without a timestamp never open a day; dates are never invented.
 */
export function dayStarts(messages: readonly Message[]): ReadonlySet<string> {
  const starts = new Set<string>();
  let currentDay: number | null = null;
  for (const message of messages) {
    if (message.sentAtMs === undefined) continue;
    const day = startOfDay(message.sentAtMs);
    if (day !== currentDay) starts.add(message.eventId);
    currentDay = day;
  }
  return starts;
}
