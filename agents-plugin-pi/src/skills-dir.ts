import { createHash, randomUUID } from "node:crypto";
import {
  chmodSync, existsSync, lstatSync, mkdirSync, readFileSync, readdirSync,
  readlinkSync, renameSync, rmSync, symlinkSync, writeFileSync,
} from "node:fs";
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

// Kept inside the bundled, ignored skills tree; never treated as source content.
const HASH_MARKER = ".ws-skills-hash";
const HASH_VERSION = "ws-skills-v1:sha256:";

type SkillEntry =
  | { kind: "directory"; children: Map<string, SkillEntry> }
  | { kind: "file"; bytes: Buffer; mode: number }
  | { kind: "link"; target: string };

function readSkillEntry(path: string, generatedRoot = false): SkillEntry {
  const stat = lstatSync(path);
  if (stat.isSymbolicLink()) return { kind: "link", target: readlinkSync(path) };
  if (stat.isFile()) return { kind: "file", bytes: readFileSync(path), mode: stat.mode & 0o777 };
  if (!stat.isDirectory()) throw new Error(`[skills] unsupported entry: ${path}`);
  const children = new Map<string, SkillEntry>();
  for (const name of readdirSync(path).sort()) {
    if (generatedRoot && name === HASH_MARKER) continue;
    children.set(name, readSkillEntry(join(path, name)));
  }
  return { kind: "directory", children };
}

function hashSkillEntry(entry: SkillEntry): string {
  const hash = createHash("sha256");
  // JSON frames names/types unambiguously; recursion includes empty directories.
  if (entry.kind === "directory") {
    hash.update(JSON.stringify([entry.kind, [...entry.children].map(([name, child]) => [name, hashSkillEntry(child)])]));
  } else if (entry.kind === "file") {
    hash.update(JSON.stringify([entry.kind, entry.mode]));
    hash.update(entry.bytes);
  } else {
    hash.update(JSON.stringify([entry.kind, entry.target]));
  }
  return hash.digest("hex");
}

function writeAtomic(path: string, bytes: string | Buffer, mode?: number): void {
  const temporary = `${path}.${randomUUID()}.tmp`;
  try {
    writeFileSync(temporary, bytes, { flag: "wx" });
    if (mode !== undefined) chmodSync(temporary, mode);
    renameSync(temporary, path);
  } finally {
    rmSync(temporary, { force: true });
  }
}

function mirrorSkillEntry(source: SkillEntry, destination: string, generatedRoot = false): void {
  const stat = lstatSync(destination, { throwIfNoEntry: false });
  if (source.kind === "directory") {
    if (!stat?.isDirectory()) {
      if (stat) rmSync(destination, { recursive: true, force: true });
      mkdirSync(destination, { recursive: true });
    }
    for (const [name, child] of source.children) mirrorSkillEntry(child, join(destination, name));
    for (const name of readdirSync(destination)) {
      if (generatedRoot && name === HASH_MARKER) continue;
      if (!source.children.has(name)) rmSync(join(destination, name), { recursive: true, force: true });
    }
  } else if (source.kind === "file") {
    if (stat?.isFile() && (stat.mode & 0o777) === source.mode && readFileSync(destination).equals(source.bytes)) return;
    if (stat && !stat.isFile()) rmSync(destination, { recursive: true, force: true });
    writeAtomic(destination, source.bytes, source.mode);
  } else {
    if (stat?.isSymbolicLink() && readlinkSync(destination) === source.target) return;
    if (stat) rmSync(destination, { recursive: true, force: true });
    symlinkSync(source.target, destination);
  }
}

/**
 * Synchronize and validate when canonical source exists; false means absent.
 * Matching source, generated content, and marker perform no writes. Hashing the
 * generated content too repairs drift instead of trusting a stale success marker.
 * This is not a lock: simultaneous mismatch writers remain unsupported.
 */
export function syncGeneratedSkillsDir(source: string, generated: string, rsrcManifestPath: string): boolean {
  if (!existsSync(source)) return false;
  const snapshot = readSkillEntry(source);
  if (snapshot.kind !== "directory") throw new Error(`[skills] source is not a directory: ${source}`);
  if (snapshot.children.has(HASH_MARKER)) throw new Error(`[skills] reserved source entry: ${HASH_MARKER}`);
  const expectedHash = hashSkillEntry(snapshot);
  const marker = join(generated, HASH_MARKER);
  const markerBody = `${HASH_VERSION}${expectedHash}\n`;
  const generatedStat = lstatSync(generated, { throwIfNoEntry: false });
  const markerMatches = generatedStat?.isDirectory()
    && lstatSync(marker, { throwIfNoEntry: false })?.isFile()
    && readFileSync(marker, "utf8") === markerBody;
  if (markerMatches && hashSkillEntry(readSkillEntry(generated, true)) === expectedHash) {
    // The rsrc manifest may change independently of the skills content.
    validateGeneratedSkillTargets(generated, rsrcManifestPath);
    return true;
  }

  mirrorSkillEntry(snapshot, generated, true);
  validateGeneratedSkillTargets(generated, rsrcManifestPath);
  if (hashSkillEntry(readSkillEntry(generated, true)) !== expectedHash
    || hashSkillEntry(readSkillEntry(source)) !== expectedHash) {
    throw new Error("[skills] tree changed during synchronization; hash marker not published");
  }
  // Publish last. Failed copying/validation leaves the previous marker untouched.
  if (lstatSync(marker, { throwIfNoEntry: false })?.isDirectory()) rmSync(marker, { recursive: true });
  writeAtomic(marker, markerBody);
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
 * Reload-time preparation: local source checkouts synchronize on a content
 * mismatch, while installed tarballs validate the copy they already carry.
 */
export function prepareSkillsDir(pluginDir: string, repoRoot: string): string {
  const source = join(repoRoot, "agents-plugin", "skills");
  const generated = join(pluginDir, "skills");
  const manifest = join(pluginDir, "rsrc", "manifest.json");
  const synchronized = syncGeneratedSkillsDir(source, generated, manifest);
  const resolved = resolveSkillsDir(pluginDir, repoRoot);
  if (!synchronized) validateGeneratedSkillTargets(resolved, manifest);
  return resolved;
}

type SkillResourcesRegistrar = {
  on(event: "resources_discover", handler: () => { skillPaths: string[] }): unknown;
};

/** Register the startup/reload seam that prepares skills before Pi discovers them. */
export function registerSkillResources(
  pi: SkillResourcesRegistrar,
  pluginDir: string,
  repoRoot: string,
): void {
  pi.on("resources_discover", () => ({
    skillPaths: [prepareSkillsDir(pluginDir, repoRoot)],
  }));
}
