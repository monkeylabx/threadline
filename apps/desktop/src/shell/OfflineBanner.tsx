import type { ConnectionState } from "../repository.ts";

/** Visible only while offline; queued messages stay pending until reconnect. */
export function OfflineBanner({ state }: { readonly state: ConnectionState }) {
  if (state === "online") return null;
  return (
    <div className="tl-banner" role="status" aria-live="polite">
      已离线。消息会先保存在本机，恢复连接后按顺序发送。
    </div>
  );
}
