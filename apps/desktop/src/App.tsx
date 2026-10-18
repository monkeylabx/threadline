/**
 * Shell root. A7 (#265) fills the two panes; this task only establishes the
 * landmark, the token-driven surface and the test seam.
 */
export function App() {
  return (
    <main
      aria-label="Threadline"
      style={{
        minHeight: "100vh",
        background: "var(--tl-color-canvas)",
        color: "var(--tl-color-text-primary)",
      }}
    >
      <h1 style={{ fontSize: "14px", fontWeight: 700, margin: "16px" }}>Threadline</h1>
    </main>
  );
}
