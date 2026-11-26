type Check = readonly [boolean, string];

function textScale() {
  return Number.parseFloat(getComputedStyle(document.documentElement).fontSize) / 16;
}

function frameChecks(form: HTMLElement, panel: HTMLElement): readonly Check[] {
  const scale = textScale();
  const box = form.getBoundingClientRect();
  const parent = panel.getBoundingClientRect();
  const style = getComputedStyle(form);
  return [
    [Math.abs(box.left - parent.left - 20 * scale) < 1 && Math.abs(parent.right - box.right - 20 * scale) < 1, "composer needs 20px side insets"],
    [Math.abs(parent.bottom - box.bottom - 20 * scale) < 1, "composer needs a 20px bottom inset"],
    [Math.abs(Number.parseFloat(style.borderRadius) - 12 * scale) < 1, "composer needs 12px rounded corners"],
    [style.borderWidth === "1px" && style.borderStyle === "solid", "composer needs a complete outline"],
  ];
}

function contentChecks(form: HTMLElement, field: HTMLTextAreaElement): readonly Check[] {
  const toolbar = form.querySelector(".tl-composer-toolbar")?.getBoundingClientRect();
  const actions = form.querySelector(".tl-composer-actions")?.getBoundingClientRect();
  const send = form.querySelector(".tl-send")?.getBoundingClientRect();
  if (!toolbar || !actions || !send) return [[false, "composer rows are missing"]];
  const editor = field.getBoundingClientRect();
  const scale = textScale();
  return [
    [toolbar.height > 0 && toolbar.bottom <= editor.top + 1, "toolbar must precede the editor"],
    [editor.bottom <= actions.top + 1, "actions must follow the editor"],
    [Math.abs(actions.right - send.right) < 1, "send must align with the right edge of the actions"],
    [editor.height >= Math.min(96 * scale, innerHeight * (innerHeight <= 700 ? 0.14 : 0.18)) - 1, "editor needs its viewport-bounded resting space"],
    [getComputedStyle(field).resize === "none", "editor must hide the native resize handle"],
  ];
}

/** Measures the real form and editor in the browser, including its viewport-relative geometry. */
export function composerGeometryChecks(): readonly Check[] {
  const form = document.querySelector<HTMLElement>(".tl-composer form");
  const field = form?.querySelector("textarea");
  const panel = document.querySelector<HTMLElement>(".tl-workspace");
  if (!form || !field || !panel) return [[false, "composer fixture is not ready"]];
  return [...frameChecks(form, panel), ...contentChecks(form, field)];
}

const setFieldValue = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;

/** Types lines into the real Composer the way React observes input, then measures after layout. */
async function typeLines(field: HTMLTextAreaElement, lines: number) {
  setFieldValue?.call(field, Array.from({ length: lines }, (_, index) => `第 ${index + 1} 行`).join("\n"));
  field.dispatchEvent(new Event("input", { bubbles: true }));
  await new Promise((resolve) => requestAnimationFrame(resolve));
  return { height: field.getBoundingClientRect().height, scrolls: field.scrollHeight > field.clientHeight + 1 };
}

/** Drives an editable Composer through growth, the viewport-dependent scrolling cap and clearing. */
export async function composerGrowthChecks(): Promise<readonly Check[]> {
  const field = document.querySelector<HTMLTextAreaElement>(".tl-composer textarea");
  const send = document.querySelector<HTMLButtonElement>(".tl-composer .tl-send");
  if (!field || !send) return [[false, "composer fixture is not ready"]];
  if (field.disabled) return [[send.disabled, "send must be disabled when editing is unavailable"]];
  const rest = field.getBoundingClientRect().height;
  const eight = await typeLines(field, 8);
  const capped = await typeLines(field, 24);
  const historyHeight = document.querySelector(".tl-timeline")?.clientHeight ?? 0;
  const cleared = await typeLines(field, 0);
  const cap = Math.min(162 * textScale(), innerHeight * (innerHeight <= 700 ? 0.18 : 0.24));
  return [
    [eight.height > rest + 1 && eight.height <= cap + 1, "editor must grow within the viewport-dependent cap"],
    [Math.abs(capped.height - cap) < 1 && capped.scrolls, "editor must stop growing and scroll at the cap"],
    [historyHeight >= 96, "timeline must remain visible with a full draft"],
    [Math.abs(cleared.height - rest) < 1 && !cleared.scrolls, "clearing must restore the resting height"],
  ];
}
