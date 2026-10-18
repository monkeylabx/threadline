import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App.tsx";
import { applyTokens, preferredScheme } from "./tokens.ts";

const root = document.getElementById("root");
if (root === null) {
  throw new Error("desktop shell root element missing");
}
applyTokens(document.documentElement, preferredScheme((query) => window.matchMedia(query).matches));
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
