// Glyphs follow the V34 prototype symbols (docs/prototype/index.html).
const paths = {
  sidebar: "M4 4h16v16H4z M9 4v16 M14 9l-3 3 3 3",
  back: "M16 5l-7 7 7 7",
  forward: "M8 5l7 7-7 7",
  search: "M18 11a7 7 0 1 1-14 0 7 7 0 0 1 14 0 M20 20l-3.4-3.4",
  message: "M21 15a4 4 0 0 1-4 4H8l-5 3V7a4 4 0 0 1 4-4h10a4 4 0 0 1 4 4z",
  work: "M6 7h12a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2z M12 3v4 M8 12h.01 M16 12h.01 M8 16h8",
  check: "M3 12l4 4L17 6 M11 16l2 2 8-10",
  plus: "M12 5v14 M5 12h14",
  clip: "M21.4 11.6l-9.2 9.2a6 6 0 0 1-8.5-8.5l9.2-9.2a4 4 0 0 1 5.7 5.7L9.4 18a2 2 0 0 1-2.8-2.8l8.5-8.5",
  send: "M22 2l-7 20-4-9-9-4z M22 2L11 13",
  device: "M4 4h16v12H4z M8 20h8 M12 16v4",
} as const;

export function Icon({ name }: { readonly name: keyof typeof paths }) {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d={paths[name]} /></svg>;
}
