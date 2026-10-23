const paths = {
  sidebar: "M4 4h16v16H4z M9 4v16 M14 9l-3 3 3 3",
  back: "M16 5l-7 7 7 7",
  forward: "M8 5l7 7-7 7",
  search: "M20 20l-5-5 M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0",
  message: "M4 4h16v12H9l-5 4z",
  work: "M5 8h14v11H5z M9 8V5h6v3 M5 12h14",
  check: "M4 7l2 2 4-4 M12 7h8 M4 17l2 2 4-4 M12 17h8",
  plus: "M12 5v14 M5 12h14",
  clip: "M9 12l5-5a3 3 0 0 1 4 4l-7 7a5 5 0 0 1-7-7l8-8 M7 14l7-7",
  send: "M3 11l18-8-8 18-2-8z M11 13L21 3",
  device: "M4 4h16v12H4z M8 20h8 M12 16v4",
} as const;

export function Icon({ name }: { readonly name: keyof typeof paths }) {
  return <svg viewBox="0 0 24 24" aria-hidden="true"><path d={paths[name]} /></svg>;
}
