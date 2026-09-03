import { existsSync } from "node:fs";
import { join } from "node:path";

/**
 * Package-local-first skills resolver. Prefers a pack-time-generated
 * `<pluginDir>/skills` (present in published/installed tarballs); falls back to
 * the monorepo canonical `<repoRoot>/agents-plugin/skills` for dev `-e` runs
 * from the source tree. `exists` is injected for unit testing.
 */
export function resolveSkillsDir(
  pluginDir: string,
  repoRoot: string,
  exists: (p: string) => boolean = existsSync,
): string {
  const local = join(pluginDir, "skills");
  if (exists(local)) return local;
  return join(repoRoot, "agents-plugin", "skills");
}
