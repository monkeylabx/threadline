import { Icon } from "./Icon.tsx";

const tools = [
  { icon: "smile", label: "表情" },
  { icon: "folder", label: "添加附件" },
  { icon: "cut", label: "截图" },
  { icon: "textFormat", label: "文本格式" },
  { icon: "markdown", label: "Markdown" },
  { icon: "more", label: "更多" },
] as const;

/** Presents the V34 tool positions while their capabilities are unavailable. */
export function ComposerTools() {
  return (
    <div className="tl-composer-toolbar" role="toolbar" aria-label="消息工具">
      {tools.map(({ icon, label }) => (
        <button key={icon} type="button" disabled aria-label={label} title={`${label}暂未开放`}>
          <Icon name={icon} />
        </button>
      ))}
    </div>
  );
}
