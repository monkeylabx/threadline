type Check = readonly [boolean, string];

function textScale() {
  return Number.parseFloat(getComputedStyle(document.documentElement).fontSize) / 16;
}

function scaledNear(scale = textScale()) {
  return (actual: number, expected: number) => Math.abs(actual - expected * scale) < 0.75;
}

/** Read-only checks of the real Composer and its stylesheet against the V34 contract. */
export function composerGeometryChecks(): readonly Check[] {
  const form = document.querySelector<HTMLElement>(".tl-composer form");
  if (!form) return [[false, "composer fixture is not ready"]];
  const field = form.querySelector<HTMLTextAreaElement>("textarea");
  const toolbar = form.querySelector<HTMLElement>(".tl-composer-toolbar");
  const send = form.querySelector<HTMLButtonElement>(".tl-send");
  const tool = form.querySelector<HTMLButtonElement>('[aria-label="添加附件"]');
  const panel = document.querySelector<HTMLElement>(".tl-workspace");
  if (!field || !toolbar || !send || !tool || !panel) return [[false, "composer fixture is not ready"]];
  const near = scaledNear();
  const box = form.getBoundingClientRect(), parent = panel.getBoundingClientRect();
  const input = getComputedStyle(field), action = getComputedStyle(send);
  return [
    [input.resize === "none", "composer must not expose a native resize handle"],
    [near(box.height, 78), "composer must rest at the 78px one-line height"],
    [near(field.getBoundingClientRect().height, 43), "composer field must rest at one scalable line"],
    [near(toolbar.getBoundingClientRect().height, 32), "composer toolbar must match 32px"],
    [near(Number.parseFloat(input.paddingLeft), 13), "composer field padding must match 13px"],
    [near(box.left - parent.left, 20) && near(parent.right - box.right, 20), "composer side insets must match 20px"],
    [near(parent.bottom - box.bottom, 20), "composer bottom inset must match 20px"],
    [near(send.getBoundingClientRect().width, 26) && near(send.getBoundingClientRect().height, 26), "send control must match 26px"],
    [near(tool.getBoundingClientRect().width, 26) && near(tool.getBoundingClientRect().height, 26), "tool control must match 26px"],
    [near(Number.parseFloat(action.borderRadius), 9), "send control radius must match 9px"],
    [near(Number.parseFloat(getComputedStyle(tool).borderRadius), 5), "tool control radius must match 5px"],
    [near(Number.parseFloat(getComputedStyle(toolbar).fontSize), 12), "toolbar must preserve the shared 12px caption minimum"],
  ];
}

const setFieldValue = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;

/** Types lines into the real Composer the way React observes input, then measures after layout. */
async function typeLines(form: HTMLElement, field: HTMLTextAreaElement, lines: number) {
  setFieldValue?.call(field, Array.from({ length: lines }, (_, index) => `第 ${index + 1} 行`).join("\n"));
  field.dispatchEvent(new Event("input", { bubbles: true }));
  await new Promise((resolve) => requestAnimationFrame(resolve));
  return { height: form.getBoundingClientRect().height, scrolls: field.scrollHeight > field.clientHeight + 1 };
}

/** Drives the real Composer through eight-line growth, the scrolling cap and clearing. */
export async function composerGrowthChecks(): Promise<readonly Check[]> {
  const form = document.querySelector<HTMLElement>(".tl-composer form");
  const field = form?.querySelector<HTMLTextAreaElement>("textarea");
  if (!form || !field) return [[false, "composer fixture is not ready"]];
  const scale = textScale(), near = scaledNear(scale);
  const eight = await typeLines(form, field, 8);
  const twelve = await typeLines(form, field, 12);
  const cleared = await typeLines(form, field, 0);
  return [
    [!eight.scrolls && !near(eight.height, 78) && eight.height <= 197 * scale + 0.75, "composer field must grow to eight lines without scrolling"],
    [near(twelve.height, 197) && twelve.scrolls, "composer field must stop at the 197px cap and scroll"],
    [near(cleared.height, 78), "composer must return to its resting height when cleared"],
  ];
}
