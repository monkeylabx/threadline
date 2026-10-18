import { useState, type FormEvent } from "react";

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
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              submit();
            }
          }}
        />
        <button type="submit" className="tl-send" disabled={disabled || draft.trim() === ""}>发送</button>
      </form>
    </div>
  );
}
