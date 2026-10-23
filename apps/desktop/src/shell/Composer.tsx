import { useState, type FormEvent } from "react";
import { Icon } from "./Icon.tsx";

interface ComposerProps {
  readonly channelName: string;
  readonly disabled: boolean;
  readonly onSend: (body: string) => void;
}

/** Message composer; Enter sends, Shift+Enter inserts a newline. */
export function Composer({ channelName, disabled, onSend }: ComposerProps) {
  const [draft, setDraft] = useState("");
  const submit = (event?: FormEvent) => {
    event?.preventDefault();
    const body = draft.trim();
    if (body === "" || disabled) return;
    onSend(body);
    setDraft("");
  };
  return (
    <div className="tl-composer">
      <form onSubmit={submit}>
        <label htmlFor="tl-composer-input" className="tl-visually-hidden">{`发消息到 ${channelName} 频道`}</label>
        <textarea
          id="tl-composer-input"
          rows={2}
          value={draft}
          placeholder={`发消息到 #${channelName}`}
          disabled={disabled}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing && event.keyCode !== 229) {
              event.preventDefault();
              submit();
            }
          }}
        />
        <div className="tl-composer-toolbar">
          <button type="button" disabled aria-label="添加附件" title="附件上传尚未接入"><Icon name="clip" /></button><span>消息</span>
          <button type="submit" data-tl-control="primary" className="tl-send" aria-label="发送" disabled={disabled || draft.trim() === ""}><Icon name="send" /></button>
        </div>
      </form>
    </div>
  );
}
