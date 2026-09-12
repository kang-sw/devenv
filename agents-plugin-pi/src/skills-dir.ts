import { cpSync, existsSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { join, relative, sep } from "node:path";

interface RsrcManifest {
  files: Record<string, string>;
}

const PLAYBOOK_TARGET = /\bplaybook\.(?:read|render)\s*\(\s*name\s*:\s*["']([^"']+)["']/g;

/**
 * Package-local-first skills resolver. Prefers a generated `<pluginDir>/skills`
 * (present in published/installed tarballs); falls back to the monorepo
 * canonical `<repoRoot>/agents-plugin/skills` when the generated tree is absent.
 * `exists` is injected for unit testing.
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

/** Replace the generated tree when the canonical monorepo source is available. */
export function syncGeneratedSkillsDir(source: string, generated: string): boolean {
  if (!existsSync(source)) return false;
  rmSync(generated, { recursive: true, force: true });
  cpSync(source, generated, { recursive: true });
  return true;
}

function skillShimPaths(root: string): string[] {
  const paths: string[] = [];
  const walk = (dir: string): void => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const full = join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.isFile() && entry.name === "SKILL.md") paths.push(full);
    }
  };
  walk(root);
  return paths.sort();
}

/**
 * Fail before Pi exposes a generated shim whose static playbook target cannot
 * resolve through the package's current rsrc manifest.
 */
export function validateGeneratedSkillTargets(skillsRoot: string, rsrcManifestPath: string): void {
  if (!existsSync(skillsRoot)) return;

  const targets: Array<{ shim: string; playbook: string }> = [];
  for (const path of skillShimPaths(skillsRoot)) {
    const body = readFileSync(path, "utf8");
    for (const match of body.matchAll(PLAYBOOK_TARGET)) {
      targets.push({
        shim: relative(skillsRoot, path).split(sep).join("/"),
        playbook: match[1],
      });
    }
  }
  if (targets.length === 0) return;

  const parsed = JSON.parse(readFileSync(rsrcManifestPath, "utf8")) as Partial<RsrcManifest>;
  if (!parsed.files || typeof parsed.files !== "object" || Array.isArray(parsed.files)) {
    throw new Error(`[skills] invalid rsrc manifest: ${rsrcManifestPath}`);
  }

  const declared = new Set(Object.keys(parsed.files));
  const problems = targets
    .filter(({ playbook }) => !declared.has(`${playbook}/${playbook}.md`) && !declared.has(`${playbook}.md`))
    .map(({ shim, playbook }) => `${shim} targets playbook ${JSON.stringify(playbook)} absent from ${rsrcManifestPath}`)
    .sort();
  if (problems.length > 0) {
    throw new Error(`[skills] generated skill target validation failed:\n${problems.join("\n")}`);
  }
}

/**
 * Reload-time preparation: local source checkouts regenerate their ignored
 * package copy, while installed tarballs validate the copy they already carry.
 */
export function prepareSkillsDir(pluginDir: string, repoRoot: string): string {
  const source = join(repoRoot, "agents-plugin", "skills");
  const generated = join(pluginDir, "skills");
  syncGeneratedSkillsDir(source, generated);
  const resolved = resolveSkillsDir(pluginDir, repoRoot);
  validateGeneratedSkillTargets(resolved, join(pluginDir, "rsrc", "manifest.json"));
  return resolved;
}
