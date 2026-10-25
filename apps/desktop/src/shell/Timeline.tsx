import { Fragment } from "react";
import type { Message } from "../repository.ts";
import { dayLabel, dayStarts } from "./day.ts";
import { identityRole } from "./identity.ts";

interface TimelineProps {
  readonly channelName: string;
  readonly messages: readonly Message[];
  /** Current time in Unix ms, for relative day labels. */
  readonly now: number;
}

/** Ordered message list grouped by local day; pending items are visibly provisional. */
export function Timeline({ channelName, messages, now }: TimelineProps) {
  const starts = dayStarts(messages);
  return (
    <section className="tl-timeline" aria-label={`${channelName} 频道消息`}>
      {messages.length === 0 ? <p className="tl-empty-conversation">还没有消息，从这里开始会话</p> : null}
      <ol>
        {messages.map((message) => (
          <Fragment key={message.eventId}>
            {starts.has(message.eventId) && message.sentAtMs !== undefined
              ? <li className="tl-day" data-testid="day-divider">{dayLabel(message.sentAtMs, now)}</li>
              : null}
            <MessageItem message={message} />
          </Fragment>
        ))}
      </ol>
    </section>
  );
}

function MessageItem({ message }: { readonly message: Message }) {
  return (
    <li className="tl-message" data-state={message.state} data-testid="message">
      <span className="tl-avatar" data-identity={identityRole(message.authorName)} aria-hidden="true">{message.authorName.slice(0, 1)}</span>
      <div>
        <div className="tl-message-meta">
          <strong>{message.authorName}</strong>
          <time>{message.sentAt}</time>
          {message.state === "pending" ? <span className="tl-pending-mark">发送中…</span> : null}
        </div>
        <p>{message.body}</p>
      </div>
    </li>
  );
}
