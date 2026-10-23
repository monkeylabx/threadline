/** Opt-in browser integration fixture. No host IPC, network or native session. */
import { useState } from "react";
import { createRoot } from "react-dom/client";
import { FixtureRepository } from "../src/repository.ts";
import { Shell } from "../src/shell/Shell.tsx";
import { applyTokens } from "../src/tokens.ts";
import "../src/shell/shell.css";
import "./layout.css";

const scenario = new URLSearchParams(location.search).get("scenario") ?? "empty";
class LayoutRepository extends FixtureRepository {
  override async listMessages(channelId: string) {
    const messages = await super.listMessages(channelId);
    if (scenario === "empty") return [];
    if (scenario !== "long") return messages;
    return messages.flatMap((message, index) => Array.from({ length: 30 }, (_, copy) => ({
      ...message, eventId: `layout-${index}-${copy}`,
      body: `第 ${index * 30 + copy + 1} 条消息。` + "用于验证长历史内部滚动和消息换行。".repeat(4),
    })));
  }
}
const repository = new LayoutRepository({ connection: scenario === "offline" ? "offline" : "online" });

type Check = readonly [boolean, string];
function violations(checks: readonly Check[]) {
  return checks.filter(([valid]) => !valid).map(([, message]) => message);
}
function viewportChecks(shell: HTMLElement, composer: HTMLElement): readonly Check[] {
  const height = document.documentElement.clientHeight;
  const input = composer.getBoundingClientRect();
  const panel = composer.closest(".tl-workspace")?.getBoundingClientRect();
  return [
    [Math.abs(shell.getBoundingClientRect().height - height) <= 1, "shell must fill viewport"],
    [panel !== undefined && Math.abs(input.bottom - panel.bottom) <= 1, "composer must remain at conversation bottom"],
    [input.top >= height / 2, "composer moved into upper half"],
    [document.documentElement.scrollHeight <= height + 1, "document must not scroll"],
  ];
}
function canScroll(timeline: HTMLElement) {
  const allowed = ["auto", "scroll"].includes(getComputedStyle(timeline).overflowY);
  const previous = timeline.scrollTop;
  timeline.scrollTop = 40;
  const moved = timeline.scrollTop > 0;
  timeline.scrollTop = previous;
  return allowed && moved;
}
function timelineChecks(timeline: HTMLElement, composer: HTMLElement): readonly Check[] {
  const list = timeline.getBoundingClientRect();
  const overflow = timeline.scrollHeight > timeline.clientHeight + 1;
  const scaled = document.querySelector('[data-layout-text-scale="true"]') !== null;
  return [
    [Math.abs(list.bottom - composer.getBoundingClientRect().top) <= 1, "timeline must precede composer"],
    [list.height > 1, "timeline needs flexible space"],
    [scenario === "long" ? overflow : scaled || !overflow, "history overflow must stay in timeline"],
    [!overflow || canScroll(timeline), "long timeline must be independently scrollable"],
  ];
}
function textReflowChecks(): readonly Check[] {
  const heading = document.querySelector<HTMLElement>(".tl-workspace-header");
  const title = heading?.querySelector<HTMLElement>("div");
  const search = document.querySelector<HTMLElement>(".tl-search");
  const field = search?.querySelector<HTMLInputElement>("input");
  if (!heading || !title || !search || !field) return [[false, "text fixture is not ready"]];
  const titleBox = title.getBoundingClientRect(), headerBox = heading.getBoundingClientRect();
  const inputBox = field.getBoundingClientRect(), searchBox = search.getBoundingClientRect();
  return [
    [titleBox.top >= headerBox.top - 1 && titleBox.bottom <= headerBox.bottom + 1, "conversation title must reflow within its heading"],
    [inputBox.top >= searchBox.top - 1 && inputBox.bottom <= searchBox.bottom + 1, "search text must fit within its container"],
  ];
}

/** Checks real geometry in a browser; the mutation control proves a red result. */
function checkLayout() {
  const shell = document.querySelector<HTMLElement>(".tl-shell");
  const timeline = document.querySelector<HTMLElement>(".tl-timeline");
  const composer = document.querySelector<HTMLElement>(".tl-composer");
  if (!shell || !timeline || !composer) return "FAIL fixture is not ready";
  const checks = [...viewportChecks(shell, composer), ...timelineChecks(timeline, composer), ...textReflowChecks()];
  if (scenario === "offline") {
    const banner = document.querySelector<HTMLElement>(".tl-banner");
    checks.push([banner !== null && banner.getBoundingClientRect().bottom <= timeline.getBoundingClientRect().top + 1, "offline banner must precede timeline"]);
  }
  const errors = violations(checks);
  return `${errors.length ? "FAIL" : "PASS"} ${scenario}: ${errors.join("; ") || "composer at conversation bottom; timeline flexes; viewport contained"}`;
}

function LayoutFixture() {
  const [report, setReport] = useState("选择场景后运行布局验收");
  const [legacy, setLegacy] = useState(false);
  const [largeText, setLargeText] = useState(false);
  return <div data-layout-legacy={legacy} data-layout-text-scale={largeText}>
    <Shell repository={repository} />
    <aside className="layout-controls" aria-label="布局验收控制">
      <nav aria-label="验收场景">{["empty", "short", "long", "offline"].map((name) => <a key={name} href={`?scenario=${name}`}>{name}</a>)}</nav>
      <label><input type="checkbox" checked={legacy} onChange={(event) => setLegacy(event.target.checked)} />模拟旧版网格缺陷</label>
      <label><input type="checkbox" checked={largeText} onChange={(event) => setLargeText(event.target.checked)} />2 倍文字</label>
      <button type="button" onClick={() => setReport(checkLayout())}>运行布局验收</button>
      <output aria-live="polite">{report}</output>
    </aside>
  </div>;
}
applyTokens(document.documentElement, "light");
const root = document.getElementById("root");
if (root === null) throw new Error("layout fixture root is missing");
createRoot(root).render(<LayoutFixture />);
