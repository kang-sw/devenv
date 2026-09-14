/**
 * Local-devenv build-and-inject module: reads and validates the Pi-adapter's
 * own `.local-devenv-runtime` marker (same JSON schema as the launcher's own
 * `bin/ws-mcp-launcher.py:read_local_devenv_contract`, but a fully
 * independent TypeScript reimplementation — read-only against the same file,
 * no shared code, no cross-language import), builds `ws-mcp` from source
 * when the marker is present, and returns an env fragment
 * (`WS_MCP_BOOTSTRAP_BINARY`) for the launcher child's own spawn.
 *
 * Deliberately the OPPOSITE validation posture from the launcher's own
 * marker read: the launcher silently treats an invalid marker as "local
 * devenv inactive" (a plugin-cache install where a broken marker should
 * never block a normal user's session). This module's caller is always a
 * developer who deliberately opted into source-build dogfood by writing this
 * exact file, so a broken marker fails loud instead, naming the offending
 * field, rather than silently falling back to a release binary the
 * developer didn't ask for.
 *
 * `readLocalDevenvMarker`'s validation and `buildLocalDevenvBootstrap`'s
 * `git rev-parse --short HEAD` read are plain synchronous IO (matching
 * `execute-gateway.ts`'s `tryGit` shape, except failures here propagate
 * instead of degrading to `undefined` — the marker is opt-in, so once it
 * exists, brokenness is a bug to surface, not a condition to paper over).
 * Only the actual `go build` invocation is an injected dependency
 * (`LocalDevenvBuildDeps.runBuild`), so this module — and its build-success
 * path — can be exercised by `npm test` with no `go` binary installed.
 */

import { accessSync, constants as fsConstants, existsSync, mkdirSync, readFileSync, renameSync, statSync, unlinkSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { isAbsolute, join } from "node:path";

export interface LocalDevenvMarker {
  source_root: string;
  tool_dir: string;
  go: string;
}

/** Carried into `bridge.ts`'s launch-error rewrap so a build/launch failure names the active dev source. */
export interface LocalDevenvContext {
  sourceRoot: string;
  sourceCommit: string;
  builtPath: string;
}

export interface LocalDevenvBuildDeps {
  /** Injected build seam — the only IO in this module that must not run for real under `npm test`. */
  runBuild: (argv: string[], opts: { cwd: string }) => void | Promise<void>;
  notify?: (message: string) => void;
  now?: () => number;
}

const MARKER_FILE_NAME = ".local-devenv-runtime";

/**
 * Reads and validates `<pluginDir>/.local-devenv-runtime`.
 *
 * - No marker file (`ENOENT`) -> `undefined`, inert, no other behavior
 *   change.
 * - Present but invalid (bad JSON, non-object payload, wrong
 *   `schema_version`, any of `source_root`/`tool_dir`/`go` missing/
 *   non-string/empty/relative, `<tool_dir>/cmd/ws-mcp` not a directory, `go`
 *   not an existing executable file) -> throws an `Error` naming the
 *   specific offending field.
 */
export function readLocalDevenvMarker(pluginDir: string): LocalDevenvMarker | undefined {
  const markerPath = join(pluginDir, MARKER_FILE_NAME);
  let raw: string;
  try {
    raw = readFileSync(markerPath, "utf8");
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === "ENOENT") return undefined;
    throw err;
  }

  let payload: unknown;
  try {
    payload = JSON.parse(raw);
  } catch (err) {
    throw new Error(`ws-pi-bridge: local-devenv marker ${markerPath} is not valid JSON: ${(err as Error).message}`);
  }
  if (typeof payload !== "object" || payload === null || Array.isArray(payload)) {
    throw new Error(`ws-pi-bridge: local-devenv marker ${markerPath} must be a JSON object`);
  }
  const obj = payload as Record<string, unknown>;
  if (obj.schema_version !== 1) {
    throw new Error(
      `ws-pi-bridge: local-devenv marker ${markerPath} has unsupported "schema_version" (expected 1, got ${JSON.stringify(obj.schema_version)})`,
    );
  }

  const fields = {} as { source_root: string; tool_dir: string; go: string };
  for (const key of ["source_root", "tool_dir", "go"] as const) {
    const value = obj[key];
    if (typeof value !== "string" || value.length === 0) {
      throw new Error(`ws-pi-bridge: local-devenv marker ${markerPath} field "${key}" is missing or not a non-empty string`);
    }
    if (!isAbsolute(value)) {
      throw new Error(`ws-pi-bridge: local-devenv marker ${markerPath} field "${key}" must be an absolute path, got "${value}"`);
    }
    fields[key] = value;
  }

  const wsMcpDir = join(fields.tool_dir, "cmd", "ws-mcp");
  let wsMcpDirIsDir: boolean;
  try {
    wsMcpDirIsDir = statSync(wsMcpDir).isDirectory();
  } catch {
    wsMcpDirIsDir = false;
  }
  if (!wsMcpDirIsDir) {
    throw new Error(`ws-pi-bridge: local-devenv marker ${markerPath} field "tool_dir" (${fields.tool_dir}) has no cmd/ws-mcp directory`);
  }

  // Mirrors the launcher's own POSIX/Windows split (ws-mcp-launcher.py
  // read_local_devenv_contract): on Windows os.access(X_OK)-equivalent
  // checks are not meaningful, so only existence-as-a-file is required
  // there; POSIX also requires the executable bit.
  let goIsFile: boolean;
  try {
    goIsFile = statSync(fields.go).isFile();
  } catch {
    goIsFile = false;
  }
  if (!goIsFile) {
    throw new Error(`ws-pi-bridge: local-devenv marker ${markerPath} field "go" (${fields.go}) does not exist or is not a file`);
  }
  if (process.platform !== "win32") {
    try {
      accessSync(fields.go, fsConstants.X_OK);
    } catch {
      throw new Error(`ws-pi-bridge: local-devenv marker ${markerPath} field "go" (${fields.go}) is not executable`);
    }
  }

  return fields;
}

/**
 * `undefined` when no marker exists (inert). Otherwise: reads `source_root`'s
 * short HEAD, composes the version/commit ldflags stamp, runs the injected
 * `deps.runBuild` to build `ws-mcp` into a pid-scoped temp path under
 * `<pluginDir>/.runtime/local-devenv/`, atomically renames it to the final
 * path on success, and returns the launcher-child env fragment plus the
 * context `bridge.ts` threads into a launch-failure rewrap.
 *
 * Fail-loud, no cache fallback: any failure (marker validation, the
 * `git rev-parse` read, or the build itself) propagates out unchanged — this
 * module never substitutes a previously-built binary or the release
 * download path once the marker is opted into.
 */
export async function buildLocalDevenvBootstrap(
  pluginDir: string,
  pluginVersion: string,
  deps: LocalDevenvBuildDeps,
): Promise<{ env: Record<string, string>; context: LocalDevenvContext } | undefined> {
  const marker = readLocalDevenvMarker(pluginDir);
  if (!marker) return undefined;

  // Direct (not injected) subprocess call, matching execute-gateway.ts's
  // tryGit shape — but a failure here propagates instead of degrading to
  // `undefined`: the marker is opt-in, so a broken source_root is a bug to
  // surface, not a condition to silently work around.
  const shortCommit = execFileSync("git", ["rev-parse", "--short", "HEAD"], {
    cwd: marker.source_root,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "ignore"],
  }).trim();

  const ldflags = `-X main.version=${pluginVersion} -X main.sourceCommit=${shortCommit}`;
  const runtimeDir = join(pluginDir, ".runtime", "local-devenv");
  mkdirSync(runtimeDir, { recursive: true });
  const tmpPath = join(runtimeDir, `ws-mcp.${process.pid}.tmp`);
  const finalPath = join(runtimeDir, "ws-mcp");

  const notify = deps.notify ?? (() => {});
  const now = deps.now ?? Date.now;

  notify(`building ws-mcp from \`${marker.source_root}\` @\`${shortCommit}\``);
  const startedAt = now();
  try {
    await deps.runBuild([marker.go, "build", "-ldflags", ldflags, "-o", tmpPath, "./cmd/ws-mcp"], { cwd: marker.tool_dir });
  } catch (err) {
    // Best-effort cleanup of a partial/failed build artifact before
    // rethrowing unchanged — the build already failed, so a cleanup error
    // here must never mask the original failure.
    try {
      if (existsSync(tmpPath)) unlinkSync(tmpPath);
    } catch {
      // best-effort only
    }
    throw err;
  }
  renameSync(tmpPath, finalPath);
  notify(`ws-mcp build finished in ${now() - startedAt}ms`);

  return {
    env: { WS_MCP_BOOTSTRAP_BINARY: finalPath },
    context: { sourceRoot: marker.source_root, sourceCommit: shortCommit, builtPath: finalPath },
  };
}
