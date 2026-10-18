import { FixtureRepository, type Repository } from "./repository.ts";
import { Shell } from "./shell/Shell.tsx";

const fixtures: Repository = new FixtureRepository();

/** Shell root. A8 swaps the fixture repository for the locald bridge. */
export function App({ repository = fixtures }: { readonly repository?: Repository }) {
  return <Shell repository={repository} />;
}
