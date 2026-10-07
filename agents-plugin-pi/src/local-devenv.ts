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
import { createHash, randomUUID } from "node:crypto";
import { isAbsolute, join } from "node:path";
import { isLeadOrFork, type SpawnRole } from "./process-role.ts";

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
 * `deps.runBuild` to build `ws-mcp` into a unique temp path under
 * `<pluginDir>/.runtime/local-devenv/`, atomically renames it to a unique
 * bootstrap path on success, and returns the launcher-child env fragment plus the
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
  const generation = `${process.pid}.${randomUUID()}`;
  const tmpPath = join(runtimeDir, `ws-mcp.${generation}.tmp`);
  const finalPath = join(runtimeDir, `ws-mcp.${generation}${process.platform === "win32" ? ".exe" : ""}`);

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
  try {
    renameSync(tmpPath, finalPath);
  } catch (error) {
    try { unlinkSync(tmpPath); } catch { /* Preserve the rename failure. */ }
    throw error;
  }
  notify(`ws-mcp build finished in ${now() - startedAt}ms`);

  return {
    env: { WS_MCP_BOOTSTRAP_BINARY: finalPath },
    context: { sourceRoot: marker.source_root, sourceCommit: shortCommit, builtPath: finalPath },
  };
}

/** The launcher's own contract identity: its binary name embeds sha256 of these exact `runtime.json` bytes. */
export function runtimeContractHash(bytes: Buffer | string): string {
  return createHash("sha256").update(bytes).digest("hex");
}

export interface ChildRuntimeEnsureDeps {
  /** Calling process's spawn role; only a lead or fork ever rebuilds. */
  role: SpawnRole | undefined;
  pluginDir: string;
  runtimeJsonPath: string;
  /** `runtimeContractHash` of the `runtime.json` the bridge's own launcher started under. */
  baselineHash: string;
  runBuild: LocalDevenvBuildDeps["runBuild"];
  /**
   * One-shot launcher run: `args` forwarded to the installed binary, `env`
   * overlaid on the caller's environment. Resolves with its stdout; rejects
   * on a non-zero exit.
   */
  runLauncher: (args: string[], env: Record<string, string>) => Promise<string>;
  notify?: (message: string) => void;
}

/**
 * The lead-side child-launch guard for a mid-session `runtime.json` change
 * (a ws version bump committed by the running lead). Children never build
 * and never receive a bootstrap override; they reuse whatever runtime the
 * launcher installed for the contract they read. A bump moves that contract
 * to a binary path nothing has installed yet, so a child's launcher would
 * fall through to a release download that does not exist until CI publishes.
 *
 * The returned function, awaited before every child launch, compares the
 * current `runtime.json` hash with the contracts already settled (the
 * bridge's startup contract included). On drift it rebuilds from source,
 * stamped with the CURRENT `plugin_version`, and runs the unchanged launcher
 * once with `WS_MCP_BOOTSTRAP_BINARY` so its own install/verify/stamp path
 * places the binary at the new contract path. `version` is forwarded because
 * a bare launcher run execs the stdio server and blocks on stdin.
 *
 * Concurrent launches after one drift share the single in-flight rebuild; a
 * settled contract is memoized so later launches do nothing; a failure is
 * not memoized, so the next launch retries. With no marker the rebuild is a
 * no-op (nothing to install, nothing to fall back to). `undefined` for a
 * worker/explore role: grandchildren stay on the release path (260907
 * "Lead-only").
 */
export function createChildRuntimeEnsurer(deps: ChildRuntimeEnsureDeps): (() => Promise<void>) | undefined {
  if (!isLeadOrFork(deps.role)) return undefined;
  const settled = new Set([deps.baselineHash]);
  const inflight = new Map<string, Promise<void>>();

  const rebootstrap = async (bytes: Buffer, hash: string): Promise<void> => {
    const contract = JSON.parse(bytes.toString("utf8")) as { plugin_version?: unknown };
    const pluginVersion = contract.plugin_version;
    if (typeof pluginVersion !== "string" || pluginVersion.length === 0) {
      throw new Error(`${deps.runtimeJsonPath} has no string "plugin_version"`);
    }
    deps.notify?.(`runtime.json changed since session start (contract ${hash.slice(0, 12)}); reinstalling ws-mcp ${pluginVersion} for child launches`);
    const bootstrap = await buildLocalDevenvBootstrap(deps.pluginDir, pluginVersion, { runBuild: deps.runBuild, notify: deps.notify });
    if (!bootstrap) return;
    try {
      const reported = (await deps.runLauncher(["version"], bootstrap.env)).trim();
      if (reported !== pluginVersion) {
        throw new Error(`installed ws-mcp reported version "${reported}", expected "${pluginVersion}"`);
      }
    } finally {
      // The launcher copied the build into the contract path (or failed);
      // either way this unique artifact has no further reader.
      try { unlinkSync(bootstrap.context.builtPath); } catch { /* best effort */ }
    }
  };

  return async () => {
    const bytes = readFileSync(deps.runtimeJsonPath);
    const hash = runtimeContractHash(bytes);
    if (settled.has(hash)) return;
    let pending = inflight.get(hash);
    if (!pending) {
      pending = rebootstrap(bytes, hash).then(
        () => { settled.add(hash); },
        (error: unknown) => {
          const message = error instanceof Error ? error.message : String(error);
          throw new Error(`ws-pi-bridge: runtime.json changed since session start; local-devenv reinstall for contract ${hash.slice(0, 12)} failed, child not launched: ${message}`);
        },
      ).finally(() => { inflight.delete(hash); });
      inflight.set(hash, pending);
    }
    await pending;
  };
}
