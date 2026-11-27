import { useLayoutEffect, useRef, useState, type FormEvent } from "react";
import { ComposerTools } from "./ComposerTools.tsx";

interface ComposerProps {
  readonly channelName: string;
  readonly disabled: boolean;
  readonly onSend: (body: string) => void;
}

/** Sizes the field to its content; the stylesheet's min/max-height clamp the result. */
function fitField(field: HTMLTextAreaElement) {
  field.style.height = "auto";
  field.style.height = `${field.scrollHeight}px`;
}

/** Message composer; Enter sends, Shift+Enter inserts a newline. The field grows with the draft. */
export function Composer({ channelName, disabled, onSend }: ComposerProps) {
  const [draft, setDraft] = useState("");
  const field = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => { if (field.current) fitField(field.current); }, [draft]);
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
        <ComposerTools />
        <textarea
          id="tl-composer-input"
          ref={field}
          rows={1}
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
        <div className="tl-composer-actions">
          <span className="tl-composer-hint">Enter 发送 · Shift+Enter 换行</span>
          <button type="submit" data-tl-control="primary" className="tl-send" disabled={disabled || draft.trim() === ""}>发送</button>
        </div>
      </form>
    </div>
  );
}
