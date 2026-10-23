import { invoke, isTauri } from "@tauri-apps/api/core";
import { useEffect, useState } from "react";

import { FixtureRepository, type Repository } from "./repository.ts";
import { LocaldRepository } from "./repository/locald.ts";
import { Shell } from "./shell/Shell.tsx";

const fixtures: Repository = new FixtureRepository();

/**
 * Picks the data seam once: inside the Tauri host with a configured Device
 * session the shell talks to locald; everywhere else (browser dev, tests, an
 * unconfigured host) it stays on fixtures so the IM remains usable.
 */
export function App({ repository }: { readonly repository?: Repository }) {
  const [chosen, setChosen] = useState<Repository | null>(repository ?? (isTauri() ? null : fixtures));

  useEffect(() => {
    if (chosen !== null) return;
    let cancelled = false;
    invoke<{ readonly realtime_enabled?: boolean } | null>("session_info")
      .then((info) => {
        if (!cancelled) setChosen(info ? new LocaldRepository(invoke, { realtime: info.realtime_enabled === true }) : fixtures);
      })
      .catch(() => {
        if (!cancelled) setChosen(fixtures);
      });
    return () => {
      cancelled = true;
    };
  }, [chosen]);

  useEffect(() => () => {
    if (chosen instanceof LocaldRepository) chosen.dispose();
  }, [chosen]);

  return chosen === null ? null : <Shell repository={chosen} />;
}
