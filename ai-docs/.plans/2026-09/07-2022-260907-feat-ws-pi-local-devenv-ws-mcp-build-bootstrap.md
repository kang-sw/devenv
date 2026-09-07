# Plan: 260907-feat-ws-pi-local-devenv-ws-mcp-build-bootstrap — Phase 1: Local-devenv build-and-inject module on the bridge startup path

## Relevant Ticket Contract

- Add `agents-plugin-pi/src/local-devenv.ts`: read and validate the marker
  relative to `pluginDir`; compose the `go build` argv/ldflags from
  `runtime.json`'s `plugin_version` and `source_root`'s short HEAD; run an
  **injected** build function; return the env fragment (or `undefined` when
  no marker exists).
- Add optional `env` to `McpStdioClient`'s options and `spawnWsMcpClient`,
  applied only when provided; when nothing is injected the spawn call must
  stay byte-for-byte what it is today (no `env` key at all).
- In `startBridge`, invoke the module for lead/fork roles only, before
  spawning the launcher, and thread the result into `spawnWsMcpClient`.
- Marker schema is fixed and already documented/consumed by the launcher
  (read-only parity, not shared code): `{"schema_version": 1, "source_root":
  <abs>, "tool_dir": <abs>, "go": <abs>}`. Missing marker file → inert,
  return `undefined`, no other behavior change. Present-but-invalid (bad
  JSON, wrong schema, relative path, missing `cmd/ws-mcp` under `tool_dir`,
  non-executable `go`) → **fail loud**, naming the offending field — this is
  the opposite of the launcher's own silent-inactive posture for the same
  schema, and that difference is deliberate (ticket Decisions).
- Build stamp: `go build -ldflags "-X main.version=<plugin_version> -X
  main.sourceCommit=<short HEAD of source_root>" -o <tmp> ./cmd/ws-mcp`, run
  with `cwd = tool_dir`.
- Atomic output: build to `.runtime/local-devenv/ws-mcp.<pid>.tmp` under
  `pluginDir`, rename to `.runtime/local-devenv/ws-mcp` only on success.
- Build failure fails loud (propagates, no cache fallback). No fingerprint
  cache — build every lead/fork session start.
- Notify "building ws-mcp from `<source_root>` @`<commit>`" before invoking
  go, and report elapsed time after (build is synchronous inside
  `session_start`, before the launcher is spawned; no timeout budget here).
- Lead-only: only `isLeadOrFork(readSpawnRole(process.env))` roles consult
  the marker/build the binary; worker/explore never do (they reuse whatever
  the launcher already installed via the compatibility stamp).
- Marker context on any launch failure while the marker is active: the
  launcher/`initialize` failure is re-thrown with source root, short commit,
  and the built path prefixed onto the message (this is what turns the
  launcher's generic "incompatible ws-mcp runtime after repair" into
  something a developer can act on).
- Env scoping: the variable goes only on the launcher child's own `spawn`
  env (`{...process.env, WS_MCP_BOOTSTRAP_BINARY}`), never written to
  `process.env`, and the `env` spawn option is omitted entirely when there
  is nothing to inject.
- Constraints: Pi-extension code only (`src`, `test`, the manual section);
  no `bin/ws-mcp-launcher.py`, `runtime.json`, `rsrc/`, `version-check.ts`,
  or Go change. Build execution must be an injected function so `npm test`
  runs without Go installed. The marker is never created by any code path
  here — only read.
- Rewrite the "Pi adapter dogfood" subsection of `ai-docs/manuals/ws-mcp.md`
  to describe the marker instead of the hand-run `WS_MCP_BOOTSTRAP_BINARY`
  one-shot; keep the release-download path as the default.

## Out of Scope

- The launcher's own `.local-devenv-runtime` gate
  (`bin/ws-mcp-launcher.py`'s `local_devenv_cache_package` /
  `read_local_devenv_contract`) — untouched; it still never recognizes
  `agents-plugin-pi` (not a `.codex`/`.claude` plugin-cache path), which is
  exactly why the adapter needs its own dev bypass. Not shared code.
- `version-check.ts`'s exact-match `assertVersionPin` — unchanged; the
  stamped `plugin_version` is what makes the unchanged pin pass.
- Widening `local_devenv_cache_package` or relaxing the bridge pin to accept
  a `-dev` suffix — explicitly rejected in the ticket's Decisions.
- Packaging (`package.json` `files`, pack scripts): already correct today —
  `.runtime/` and `.local-devenv-runtime` are absent from `files` and
  gitignored (`.gitignore:22-23`); Phase 1 adds no new packaging surface, so
  no edit is needed there, only the owner's live `npm pack --dry-run` check
  (Verification Plan).
- Any later phase (e.g. `fork` env propagation beyond what already exists,
  fingerprint caching) — not requested; the ticket explicitly rejects a
  fingerprint cache for this dev-only path.
- `260906-bug-ws-pi-rsrc-mirror-drift` and other sibling tickets — referenced
  only as "do not touch the launcher mirror", not otherwise in scope.

## Codebase Findings

- `agents-plugin-pi/src/bridge.ts#L430-435` — `startBridge`'s current top:
  `readRuntimeContract` then an immediate, unconditional
  `spawnWsMcpClient(opts.launcherPath, opts.pluginDir, onStderr)` call with
  no `env`. This is the exact seam to insert the lead/fork-gated
  local-devenv call before, using the already-parsed
  `runtime.plugin_version`.
- `agents-plugin-pi/src/bridge.ts#L619-622` — the single existing
  `try {...} catch (err) { shutdown(); throw err; }` wrapping
  `client.initialize` + `assertVersionPin` + `listTools` + the ferrule/
  snapshot fetches. This is the one place to prefix marker context onto a
  rethrow — no need to touch multiple throw sites.
- `agents-plugin-pi/src/bridge.ts#L143-159`, `#L224-239` — established
  convention in this file: pull a role/gate predicate or IO-wrapper out of
  `startBridge`'s closure into its own exported, directly-unit-testable
  function specifically because `startBridge` itself spawns a real
  subprocess and cannot be exercised in `node --test`. Apply the same move
  for the marker-context error-wrap (e.g. `wrapLaunchErrorWithLocalDevenvContext`)
  so the "carries source root/commit/built path" test doesn't need a live
  bridge.
- `agents-plugin-pi/src/process-role.ts#L59-72` — `readSpawnRole` /
  `isLeadOrFork`, already imported into `bridge.ts` (line 31) and already
  used at `bridge.ts#L590` for the identical "lead/fork only" gate on the
  session-start snapshot fetch. Reuse verbatim for the local-devenv gate —
  no new role-detection code needed.
- `agents-plugin-pi/src/mcp-stdio-client.ts#L167-225` — `McpStdioClient`
  constructor's `options: { cwd: string; onStderr?: ... }` and its single
  `spawn(command, args, { cwd: options.cwd, stdio: [...] })` call. Needs an
  optional `env?: Record<string,string>` field, applied so that when set the
  actual node-level options become `{ cwd, stdio, env: {...process.env,
  ...options.env} }`, and when unset the options object is unchanged from
  today (no `env` key at all). Recommend extracting this into a small pure
  helper (e.g. `buildStdioSpawnOptions(cwd, env?)`) so "no marker → no env
  key" is testable without touching a real child process, matching this
  file's own testability doctrine in its header comment.
- `agents-plugin-pi/src/mcp-stdio-client.ts#L282-293` — `spawnWsMcpClient`,
  the sole caller-facing wrapper (only call site: `bridge.ts:432`). Add a
  4th optional `env?: Record<string,string>` parameter and forward it into
  the `McpStdioClient` constructor's options.
- `agents-plugin-pi/src/execute-gateway.ts#L448-465` — `scrapeWorkingContext`
  / `tryGit`: precedent for calling real `git` as a subprocess directly
  (not injected) inside this package, tested in
  `test/execute-gateway.test.ts` against a real `mkdtemp`'d git repo. Use
  the same non-injected approach for the `source_root` short-HEAD read in
  `local-devenv.ts` — only the actual `go build` invocation needs injection
  per the ticket's explicit constraint ("Build execution is an injected
  function").
- `agents-plugin-pi/src/skills-dir.ts#L1-19` — smallest example in this repo
  of the "one IO seam injected, default real implementation, rest is plain
  code" pattern (`exists` param defaulting to `existsSync`) — the shape to
  mirror for `local-devenv.ts`'s `runBuild` dependency.
- `agents-plugin-pi/bin/ws-mcp-launcher.py#L469-527` — the launcher's own
  `read_local_devenv_contract` / `local_devenv_runtime_enabled`: confirms
  the exact marker schema/validation fields (`schema_version`, `source_root`,
  `tool_dir` containing `cmd/ws-mcp`, `go` executable, all-absolute) to
  mirror for read/validate parity — but note the launcher's posture on an
  invalid marker is silent-inactive (`note(...); return None`), which this
  ticket explicitly does NOT want reused: the adapter's own read must fail
  loud instead. Do not import or share this Python logic (constraint: no
  Go/launcher edits, and this is a different language/process anyway) —
  reimplement equivalently in TypeScript.
- `agents-plugin-pi/runtime.json#L1-3` — `plugin_version: "0.45.2"`, the
  exact string to stamp via `-X main.version=`.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L23-24` — confirms the ldflags
  target symbols exist and are named exactly `main.version` and
  `main.sourceCommit` (`var version = "0.45.2-dev"`, `var sourceCommit =
  "dev"`), and line 178 confirms `runtime info` already prints
  `{"version":...,"source_commit":...}` from these same vars — no Go change
  needed, the stamp just fills in variables that already exist and are
  already surfaced.
- `agents-plugin-pi/src/index.ts#L342-348` — the only call site of
  `startBridge`, passing `{launcherPath, pluginDir, runtimeJsonPath, cwd,
  ui}`. No changes needed here — `BridgeOptions` already carries everything
  `local-devenv.ts` needs (`pluginDir`, `runtimeJsonPath`, and `opts.ui` for
  the notify callback).
- `agents-plugin-pi/.gitignore` (root `.gitignore:20-25`) — `agents-plugin-pi/.runtime/`
  and `agents-plugin-pi/.local-devenv-runtime` are already gitignored; no
  gitignore edit needed.
- `agents-plugin-pi/package.json#L11-20` — `files` whitelist already omits
  both `.runtime/` and `.local-devenv-runtime`; packaging guardrail is
  already structurally satisfied, confirming Phase 1 needs no
  `package.json` edit.
- `ai-docs/manuals/ws-mcp.md#L189-272` — the launcher-side "Local Devenv
  Repair" section already documents the exact marker JSON shape and
  validation rules in prose; the rewritten "Pi adapter dogfood" subsection
  (`#L274-309`) should point at this shape (same schema, different
  consuming code path) rather than re-deriving it, and replace the
  `go build ... -o /tmp/ws-mcp` + one-shot `WS_MCP_BOOTSTRAP_BINARY` launch
  recipe with "write the marker once, the bridge builds and injects it every
  lead session start."
- `agents-plugin-pi/test/bridge.test.ts#L1-14`, `agents-plugin-pi/test/execute-gateway.test.ts#L17-42`
  — established test-file header convention (what's covered, why, how to
  run) and the `mkdtempSync`-based real-subprocess test pattern to reuse for
  `local-devenv.test.ts`.
- **Risk signal (shortcut risk, none found):** no existing helper duplicates
  this build-and-inject responsibility, and no mock-data/fallback path is
  being substituted for the real target — the ticket's own "fail loud, no
  cache fallback" posture is a *specified* runtime behavior (per the survey
  rules, a required fallback/failure branch is not a shortcut signal). The
  only thing to watch during implementation: do not accidentally reuse or
  call into `bin/ws-mcp-launcher.py`'s Python validation (impossible cross-
  language anyway, but worth stating as a boundary) — the TS module must be
  a fully independent reimplementation, read-only against the same marker
  file.

## Implementation Plan

1. **`agents-plugin-pi/src/local-devenv.ts` (new file).** Export:
   - `LocalDevenvMarker` type: `{ source_root: string; tool_dir: string; go:
     string }`.
   - `readLocalDevenvMarker(pluginDir: string): LocalDevenvMarker |
     undefined` — reads `<pluginDir>/.local-devenv-runtime`; `ENOENT` →
     `undefined`; otherwise validate and throw `Error` naming the specific
     bad field on: invalid JSON, non-object payload, `schema_version !== 1`,
     any of `source_root`/`tool_dir`/`go` missing/non-string/empty/relative,
     `<tool_dir>/cmd/ws-mcp` not a directory, `go` not an existing
     executable file (POSIX: `fs.accessSync(go, fs.constants.X_OK)`;
     Windows: existence only, mirroring the launcher's own POSIX/Windows
     split at `ws-mcp-launcher.py#L503-509`).
   - `LocalDevenvContext` type: `{ sourceRoot: string; sourceCommit: string;
     builtPath: string }` (for bridge.ts's error-wrap).
   - `LocalDevenvBuildDeps` type: `{ runBuild: (argv: string[], opts: {
     cwd: string }) => void | Promise<void>; notify?: (message: string) =>
     void; now?: () => number }` — `runBuild` is the injected seam (real
     default in `bridge.ts` wraps `execFileSync`; tests supply a stub);
     `now` defaults to `Date.now` for elapsed-time reporting, injectable for
     deterministic tests.
   - `buildLocalDevenvBootstrap(pluginDir: string, pluginVersion: string,
     deps: LocalDevenvBuildDeps): Promise<{ env: Record<string, string>;
     context: LocalDevenvContext } | undefined>` — calls
     `readLocalDevenvMarker`; returns `undefined` immediately when no
     marker. Otherwise: read `source_root`'s short HEAD via a direct (not
     injected) `git rev-parse --short HEAD` subprocess call, mirroring
     `execute-gateway.ts#L448-465`'s `tryGit` shape but let a failure here
     propagate (this is a "marker is opted-in but broken" case, same
     fail-loud posture, not a `tryGit`-style silent-degrade); compose
     `ldflags = "-X main.version=" + pluginVersion + " -X
     main.sourceCommit=" + shortCommit`; `mkdirSync(join(pluginDir,
     ".runtime", "local-devenv"), { recursive: true })`; `tmpPath =
     .../ws-mcp.<process.pid>.tmp`, `finalPath = .../ws-mcp`; notify
     `` building ws-mcp from `${source_root}` @`${shortCommit}` `` ; record
     start time; call `await deps.runBuild([go, "build", "-ldflags",
     ldflags, "-o", tmpPath, "./cmd/ws-mcp"], { cwd: tool_dir })`
     (propagate any throw, best-effort `unlinkSync(tmpPath)` on failure
     before rethrow); on success, `renameSync(tmpPath, finalPath)`, notify
     the elapsed time, and return `{ env: { WS_MCP_BOOTSTRAP_BINARY:
     finalPath }, context: { sourceRoot: source_root, sourceCommit:
     shortCommit, builtPath: finalPath } }`.

2. **`agents-plugin-pi/src/mcp-stdio-client.ts`.**
   - Add a small pure helper (e.g. `buildStdioSpawnOptions(cwd: string, env?:
     Record<string, string>)`) returning the exact options object to pass to
     `node:child_process`'s `spawn`: `{ cwd, stdio: ["pipe","pipe","pipe"] }`
     when `env` is `undefined`, or the same plus `env: { ...process.env,
     ...env }` when provided. Export it for direct unit testing.
   - `McpStdioClient`'s constructor options (`#L173-181`) gain `env?:
     Record<string, string>`; the `spawn(command, args, {...})` call
     (`#L178-181`) uses `buildStdioSpawnOptions(options.cwd, options.env)`.
   - `spawnWsMcpClient` (`#L282-293`) gains a 4th optional parameter `env?:
     Record<string, string>` and forwards it into the `McpStdioClient`
     constructor's options alongside `cwd`/`onStderr`.

3. **`agents-plugin-pi/src/bridge.ts`.**
   - Add an exported pure helper (near the other extracted gates, e.g. after
     `sanitizeToolName`): `wrapLaunchErrorWithLocalDevenvContext(err: unknown,
     context: LocalDevenvContext | undefined): Error` — when `context` is
     `undefined`, return `err` as-is (coerced to `Error` if not already
     one); otherwise return a new `Error` whose message prefixes/embeds
     `context.sourceRoot`, `context.sourceCommit`, and `context.builtPath`
     ahead of the original message.
   - In `startBridge` (`#L430-435`): after `const runtime =
     readRuntimeContract(...)`, add `let localDevenvContext:
     LocalDevenvContext | undefined; let launcherEnv: Record<string, string>
     | undefined;` then, gated on `isLeadOrFork(readSpawnRole(process.env))`,
     `await buildLocalDevenvBootstrap(opts.pluginDir,
     runtime.plugin_version, { runBuild: runGoBuild, notify: (m) =>
     notify(opts.ui, \`ws-pi-bridge: ${m}\`) })` where `runGoBuild` is a
     small local function wrapping `execFileSync(argv[0], argv.slice(1), {
     cwd, stdio: "inherit" })` (throws on non-zero exit, matching
     `execFileSync`'s default behavior — no extra logic needed for
     "propagates on failure"). On a defined result, set both
     `launcherEnv`/`localDevenvContext`. Then change the
     `spawnWsMcpClient(opts.launcherPath, opts.pluginDir, onStderrFn)` call
     to pass `launcherEnv` as the 4th argument.
   - In the existing `catch (err) { shutdown(); throw err; }` (`#L619-622`),
     change the rethrow to `throw
     wrapLaunchErrorWithLocalDevenvContext(err, localDevenvContext);`.
   - Import `buildLocalDevenvBootstrap` and `LocalDevenvContext` from
     `./local-devenv.ts`.

4. **`ai-docs/manuals/ws-mcp.md`.** Rewrite the "Pi adapter dogfood"
   subsection (`#L274-309`): keep the Pi-track-local strip-note banner;
   replace the `go build -ldflags ... -o /tmp/ws-mcp` +
   `WS_MCP_BOOTSTRAP_BINARY=... pi -e ...` one-shot recipe with: write
   `agents-plugin-pi/.local-devenv-runtime` once (same schema as the
   launcher's own marker, already documented above at `#L199-206`, pointing
   `source_root`/`tool_dir` at this repo's `agents-plugin-tool`) and note
   that every lead/fork session start then builds and injects the binary
   automatically (no manual rebuild-and-relaunch step), fails loud on a
   build or marker error, and stays inert with the marker absent. Keep the
   release-download path described as the default/no-marker behavior.

## Verification Plan

- `cd agents-plugin-pi && npm test` — must cover (new `test/local-devenv.test.ts`
  unless a maintainer prefers folding some cases into `test/bridge.test.ts`
  for the bridge-level wrap):
  - No marker (`readLocalDevenvMarker`/`buildLocalDevenvBootstrap` on an
    empty temp dir) → `undefined`, and `buildStdioSpawnOptions(cwd,
    undefined)` carries no `env` key.
  - Each invalid-marker case (bad JSON; wrong/missing `schema_version`;
    each of `source_root`/`tool_dir`/`go` missing or relative; `tool_dir`
    without `cmd/ws-mcp`; non-executable `go`) throws an `Error` naming
    that field — build them against real `mkdtemp`'d fixture directories/
    files, matching `test/execute-gateway.test.ts`'s pattern.
  - A valid marker (real temp git repo for `source_root`, real temp
    `tool_dir/cmd/ws-mcp` dir, a real executable file for `go`) yields the
    exact ldflags string, a temp path under `.runtime/local-devenv/`
    matching `ws-mcp.<pid>.tmp`, calls the injected `runBuild` exactly once
    with `cwd` equal to `tool_dir`, and — once the stub "build" writes a
    file at the given tmp path — the final env fragment's path is the
    renamed `.runtime/local-devenv/ws-mcp`.
  - A `runBuild` that throws propagates out of `buildLocalDevenvBootstrap`
    unchanged (or wrapped only insofar as `Error` identity/message is
    preserved).
  - `wrapLaunchErrorWithLocalDevenvContext` — with a context set, the
    resulting message contains the source root, short commit, and built
    path; with no context, the original error passes through unchanged.
  - `buildStdioSpawnOptions`/`spawnWsMcpClient` forward `env` merged with
    `process.env` when given (either via the pure helper directly, or via a
    small real-subprocess check using `process.execPath` as the spawned
    command reading back an injected env var through `onStderr`, mirroring
    this file's existing IO-free-first-then-real-subprocess-if-needed
    style).
  - `isLeadOrFork`/`readSpawnRole`-gated skip: assert (at the pure-gate
    level, since `startBridge` cannot be driven in a unit test) that a
    `worker`/`explore` role never triggers the `buildLocalDevenvBootstrap`
    call path — e.g. by asserting the existing `isLeadOrFork` predicate
    against those roles returns `false`, and/or a bridge-level check that
    the gate expression used in `startBridge` is the same predicate.
  - Existing `test/version-check.test.ts` (byte-identity/version-pin) and
    all other existing suites continue to pass unchanged.
- Owner's live verification (already specified by the ticket, not part of
  this survey's job to re-derive): write the marker for
  `/Users/kang-sw/devenv`, delete the stale
  `.runtime/darwin-arm64/ws-mcp-0.45.2-*` cache entry, start a Pi lead
  session, confirm the launcher's stderr shows the bootstrap-binary install
  and `runtime info` reports `version: 0.45.2` / the develop root HEAD as
  `source_commit`; spawn one worker and confirm no second build fires;
  remove the marker and confirm the next session takes the ordinary
  release-download path; `npm pack --dry-run` lists neither the marker nor
  `.runtime/`.

## Escalations

- None.
