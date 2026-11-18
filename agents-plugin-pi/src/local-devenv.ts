/**
 * Local-devenv build-and-inject module: reads and validates the Pi-adapter's
 * own `.local-devenv-runtime` marker (same JSON schema as the launcher's own
 * `bin/ws-mcp-launcher.py:read_local_devenv_contract`, but a fully
 * independent TypeScript reimplementation — read-only against the same file,
 * no shared marker-validation code). Ordinary startup imports the launcher's
 * read-only compatibility helpers to reuse its selected runtime, building only
 * when unusable. Explicit child contract repair still builds `ws-mcp` and
 * returns `WS_MCP_BOOTSTRAP_BINARY` for the launcher child's spawn.
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
 * Runtime inspection executes Python helpers; asset generation and `go build`
 * use an injected dependency (`LocalDevenvBuildDeps.runBuild`), so this module — and its build-success
 * path — can be exercised by `npm test` with no `go` binary installed.
 */

import { accessSync, constants as fsConstants, existsSync, mkdirSync, readFileSync, renameSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import { execFile, execFileSync } from "node:child_process";
import { promisify } from "node:util";
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
  runBuild: (argv: string[], opts: { cwd: string; timeout: number }) => void | Promise<void>;
  notify?: (message: string) => void;
  now?: () => number;
}

const MARKER_FILE_NAME = ".local-devenv-runtime";

// Read-only Python inspection keeps runtime naming, hash-bound compatibility
// and source fingerprint semantics owned by the launcher. This imports helpers
// only: it never runs main, probes a daemon, installs, or executes a ws binary.
const INSPECT_SELECTED_RUNTIME = `
import json, os, runpy, sys
from pathlib import Path
m = runpy.run_path(sys.argv[1])
plugin = Path(sys.argv[2])
contract_path = Path(sys.argv[3])
contract = m['read_runtime_contract'](contract_path)
os_name = m['host_os']()
platform = os_name + '-' + m['host_arch']()
runtime_dir = Path(os.environ.get('WS_MCP_RUNTIME_DIR', str(plugin / '.runtime' / platform)))
binary = runtime_dir / m['runtime_binary_name'](contract, contract_path, os_name)
usable = m['runtime_fully_compatible'](binary, contract, runtime_dir, allow_same_minor=True)
fingerprint = m['local_devenv_source_fingerprint'](plugin, os_name) if usable else None
print(json.dumps({'path': str(binary), 'metadata': m['read_runtime_metadata'](binary) if usable else None,
 'drift': usable and not m['compatibility_stamp_current'](binary, contract, contract_path, runtime_dir, fingerprint)}))
`;

export interface LocalDevenvStartup {
  env: Record<string, string>;
  context: LocalDevenvContext;
  /** Only a fresh unique bootstrap belongs to the bridge's cleanup path. */
  ownedBootstrap: boolean;
}

/** Ordinary lead/fork startup reuses the selected compatible runtime; drift is advisory. */
export async function prepareLocalDevenvStartup(
  pluginDir: string,
  launcherPath: string,
  runtimeJsonPath: string,
  pluginVersion: string,
  deps: LocalDevenvBuildDeps,
): Promise<LocalDevenvStartup | undefined> {
  const marker = readLocalDevenvMarker(pluginDir);
  if (!marker) return undefined;
  const { stdout } = await promisify(execFile)("python3", ["-c", INSPECT_SELECTED_RUNTIME, launcherPath, pluginDir, runtimeJsonPath], {
    cwd: pluginDir, encoding: "utf8", timeout: 30_000,
  });
  const selected = JSON.parse(stdout) as { path: string; metadata: { source_commit?: string } | null; drift: boolean };
  if (selected.metadata) {
    if (selected.drift) deps.notify?.("reusing compatible local runtime; a newer source build is available via python3 scripts/dev.py rerun daemon");
    return {
      env: {},
      context: { sourceRoot: marker.source_root, sourceCommit: selected.metadata.source_commit ?? "unknown", builtPath: selected.path },
      ownedBootstrap: false,
    };
  }
  const bootstrap = await buildLocalDevenvBootstrap(pluginDir, pluginVersion, deps);
  return bootstrap ? { ...bootstrap, ownedBootstrap: true } : undefined;
}

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
 * `deps.runBuild` to generate package assets first, then build `ws-mcp` into a unique temp path under
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
    // Match the launcher's generate_package_assets timeout and Python interpreter.
    await deps.runBuild(["python3", join(marker.tool_dir, "scripts", "generate-package-assets.py")], { cwd: marker.tool_dir, timeout: 120_000 });
    await deps.runBuild([marker.go, "build", "-ldflags", ldflags, "-o", tmpPath, "./cmd/ws-mcp"], { cwd: marker.tool_dir, timeout: 120_000 });
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
    writeFileSync(`${finalPath}.metadata.json`, JSON.stringify({ schema_version: 1, binary_sha256: createHash("sha256").update(readFileSync(finalPath)).digest("hex"), version: pluginVersion, source_commit: shortCommit }));
  } catch (error) {
    for (const path of [tmpPath, finalPath, `${finalPath}.metadata.json`]) {
      try { unlinkSync(path); } catch { /* Preserve the original failure. */ }
    }
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
   * One-shot launcher run: `args` handled by the Python launcher, `env`
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
 * places the binary at the new contract path. `version` is the launcher's
 * inspection command: with the override it installs the build and reports
 * the version from its hash-bound metadata, executing no ws binary and
 * starting no daemon (a bare launcher run would serve stdio and block).
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
      for (const path of [bootstrap.context.builtPath, `${bootstrap.context.builtPath}.metadata.json`]) {
        try { unlinkSync(path); } catch { /* best effort */ }
      }
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
