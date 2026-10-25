/** Read-only checks of the real Composer and its stylesheet against the V34 contract. */
export function composerGeometryChecks(): readonly (readonly [boolean, string])[] {
  const form = document.querySelector<HTMLElement>(".tl-composer form");
  if (!form) return [[false, "composer fixture is not ready"]];
  const field = form.querySelector<HTMLTextAreaElement>("textarea");
  const toolbar = form.querySelector<HTMLElement>(".tl-composer-toolbar");
  const send = form.querySelector<HTMLButtonElement>(".tl-send");
  const tool = form.querySelector<HTMLButtonElement>('[aria-label="添加附件"]');
  const panel = document.querySelector<HTMLElement>(".tl-workspace");
  if (!field || !toolbar || !send || !tool || !panel) return [[false, "composer fixture is not ready"]];
  const scale = Number.parseFloat(getComputedStyle(document.documentElement).fontSize) / 16;
  const near = (actual: number, expected: number) => Math.abs(actual - expected * scale) < 0.75;
  const box = form.getBoundingClientRect(), parent = panel.getBoundingClientRect();
  const input = getComputedStyle(field), action = getComputedStyle(send);
  return [
    [input.resize === "none", "composer must not expose a native resize handle"],
    [near(box.height, 98), "composer must match the 98px reference height"],
    [near(field.getBoundingClientRect().height, 63), "composer field must retain its scalable minimum"],
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
