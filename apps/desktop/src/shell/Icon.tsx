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
  smile: "M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0 M8 14s1.5 3 4 3 4-3 4-3 M8.5 9h.01 M15.5 9h.01",
  folder: "M3 6h7l2 2h9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z",
  cut: "M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0 M21 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0 M4 3l12 13 M20 3 8 16",
  textFormat: "M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0 M8 17l4-10 4 10 M9.5 13h5",
  markdown: "M9 21H4V3h15v9 M8 7h7 M8 11h4 M13 21v-6l3 3 3-3v6",
  more: "M6 12a1 1 0 1 1-2 0 1 1 0 0 1 2 0 M13 12a1 1 0 1 1-2 0 1 1 0 0 1 2 0 M20 12a1 1 0 1 1-2 0 1 1 0 0 1 2 0",
  device: "M4 4h16v12H4z M8 20h8 M12 16v4",
} as const;

export function Icon({ name }: { readonly name: keyof typeof paths }) {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d={paths[name]} /></svg>;
}
