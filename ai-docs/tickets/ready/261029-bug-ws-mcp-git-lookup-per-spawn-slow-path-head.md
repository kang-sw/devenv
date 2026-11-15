---
title: ws-mcp repeats runtime Git lookup, so a slow PATH head multiplies its cost
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0cd6dae8e825272a
sage-review-completeness-reviewed: 0cd6dae8e825272a
---

# ws-mcp repeats runtime Git lookup, so a slow PATH head multiplies its cost

## Background

Dogfood capture from the v0.48.6 pre-flight (2026-10-29, WSL2 machine).

At the original capture, git spawns passed the bare name `"git"` to
`exec.Command` / `exec.CommandContext`, causing repeated PATH searches
(recorded in 4bc5460c; this is historical, not the current inventory below). On the dogfood machine `PATH` begins with a
Windows directory injected by a desktop app
(`/mnt/c/Users/<user>/AppData/Roaming/orca/wsl-managed-cli/<hash>`); each stat
under `/mnt/c` crosses the WSL interop filesystem and costs about 22 ms
(200 × `test -e` there: 4.5 s, versus 0.009 s under `/usr/local/bin`).

Measured effect:

- `TestAcquireWarningRelaySuffix` spawns git 305 times: 6.4 s with the
  machine's `PATH`, 0.8-1.0 s with the `/mnt/` entries removed.
- The whole `internal/mcp` package (738 top-level tests, serial): 466 s versus
  66 s. At 466-581 s it sits next to the 600 s default `go test` package
  timeout, which is how the slowdown surfaced.
- The runtime pays the same per-call cost: production spawn sites that name
  `"git"` without a cached path are `internal/wsgit/git.go:28`,
  `internal/wsindex/git.go:110`, `internal/mcp/server.go:3748`,
  `internal/wsstate/paths.go:285`, `internal/wsdoc/tickets_scope.go:240`,
  `internal/wsdoc/tickets_sage_freshness.go:288`, and
  `internal/wsdoc/project_tree.go:98,111`. About 20 test files also spawn
  `"git"` directly.

Any downstream machine with a slow or network-backed directory early in
`PATH` sees the same multiplier, so this is not specific to this repository.

Prior speedups were CI-side only and did not touch local runs: e03c89961
(Windows `internal/mcp` shards) and 8c64a9414 (Windows 30 min package
timeout).

### Separate open item: plain `go test ./...` did not finish

During the same pre-flight a plain `go test ./...` in `agents-plugin-tool`
ran for 30 minutes without finishing and was killed (exit 143). Slowness alone
does not explain it: an overrun package would panic at the 10 min default
timeout and exit. Unconfirmed hypothesis: after a test binary died, a child it
had spawned (for example a hang-ssh transport) kept the stdout pipe open, so
the `| tail` consumer never saw EOF. Not reproduced yet.

## Original questions

The original capture asked whether to cache once per process, use a shared
spawn helper, or change only tests; whether direct-spawn test helpers should
share the cache without breaking PATH override tests; and whether the 30-minute
stall is related. The approved outcome below requires a runtime cache, not a
test-only workaround. It does not establish the stall's cause or choose cache
ownership/invalidation mechanics.

## Decisions

- Owner-approved FIX 6 scope (triage 1060, relayed 2026-11-15): cache the
  resolved Git path rather than looking up PATH on each runtime spawn. Keep
  this a bounded lookup-cost correction, not process/daemon lifecycle work.
  Fact population, readiness and dispatch remain with the lead after drafting.
- Preserve caller-environment selection: a hosted connection must run its own
  Git, not another connection's or the daemon's. Current per-caller environment
  semantics are explicit in
  `agents-plugin-tool/internal/mcp/process_env.go:24-44,216-220` and
  `agents-plugin-tool/internal/wsexec/lookpath.go:17-65`. Cache reuse must not
  cross semantically different executable search contexts.
- (1064) Technical settlement from fact population: own one mutex-protected,
  successful-entry Git-only cache in leaf wsexec. Retain wsgit facades and
  runner signatures; expose only the internal-package Git resolver needed by
  existing runtime callers. Key by nil/process versus explicit mode and the
  resolver's effective PATH/PATHEXT, preserving last-entry/case rules. Serialize
  cold lookup/publication; never cache failures or substitute daemon Git for
  explicit lookup errors. Alternating contexts may miss but never cross-select.
  Bypass reuse for relative searched entries, including Unix process-lookup
  empty entries and Windows drive/root-relative entries, or uncertain cwd.
  For nil/process Windows lookup, also bypass while implicit cwd search is
  enabled. Preserve returned strings, ErrDot/GODEBUG behavior and existing
  resolver semantics. Keep command environment, argv, Dir, Cancel, WaitDelay
  and streams outside the cache. Rejected: per-runner caches duplicate
  ownership; a cwd-keyed map adds capacity/eviction and Windows search policy.
- (1066) Announced settlement from fact population: successful reuse lasts
  until the search key changes, following the existing wsgit successful-entry
  model (executable.go:11-48 and the read-storm prior decision below). Under
  unchanged keys, newly installed higher-priority Git, removal, permission
  changes or symlink retargets may leave a stale success. Failed lookups always
  retry; no TTL, watcher or launch-error re-resolution is added. Rejected:
  prefix re-stat restores the lookup cost; new freshness machinery adds policy
  beyond this bounded correction. No freshness guarantee is claimed.
- INFERRED compatibility target: **T0 / bug fix**, under
  `ai-docs/manuals/release-compatibility.md`. The current runner resolves Git
  from the caller environment and executes existing argv
  (`agents-plugin-tool/internal/wsgit/git.go:35-60`,
  `agents-plugin-tool/internal/wsindex/git.go:114-139`); the new runner must
  execute the same binary/argv/environment with a cached resolution for an
  unchanged search context. No MCP argument, persisted store, Git data format,
  hello or version gate changes are needed. Old/current writers and new
  readers still use ordinary Git data; reopen needs no reconstruction. Any
  proposal requiring an old-version boundary or changing caller Git selection
  is a blocker, not approved T3 scope.

## Current source findings

- VERIFIED by source inspection: the original claim that every spawn uses a
  bare name is no longer literally current. `wsgit.Executable` already caches
  successful process-environment resolution by PATH and PATHEXT, retries failed
  lookup and has invalidation tests
  (`agents-plugin-tool/internal/wsgit/executable.go:11-48`,
  `agents-plugin-tool/internal/wsgit/executable_test.go#L29-L72`). The dashboard badge uses it
  (`internal/wsdashboard/gitbadge.go:146-150` under the same package root).
- VERIFIED: the main runtime path still repeats lookup. `s.gitRunner()` passes
  `s.environ` (`internal/mcp/process_env.go:216-220`); `ExecRunner.RunGit` calls
  `LookPathEnv` per run (`internal/wsgit/git.go:35-60`), which delegates to the
  uncached filesystem search in `internal/wsexec/lookpath.go:26-73`.
  `internal/wsindex/git.go:114-139`, `internal/wsdoc/gitenv.go:9-17` and
  `agents-plugin-tool/internal/wsstate/paths.go#L290-L301` also remain relevant lookup paths.
  `internal/wstrackerhost/runner.go:58-66,86-93` uses the cache only for nil
  env; its explicit-env path still resolves each time.
- INFERRED: repeated stats can retain the reported slow-PATH multiplier, but
  this checkout's timing and original 305-spawn/738-test counts were not
  remeasured. Original source coordinates and measurements above are historical
  evidence, not a current complete spawn inventory. The 30-minute stall remains
  unverified and must not be presented as fixed by caching.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Preserve nil versus explicit (even empty) environment semantics, PATH and
  Windows PATHEXT selection, missing-Git errors without daemon fallback, context
  cancellation, subprocess bounds, stdout/stderr behavior and test isolation.
- A permanent process-wide first-Git pin is not justified by current source:
  PATH-changing tests and multiple hosted connections already require separate
  selection. Do not broaden caching to other programs or change release/test
  timeouts as a substitute for this fix.
- Test helpers may change only where needed to exercise the same corrected
  resolution path; no wholesale test-helper migration is approved. The stall
  and transport-pipe diagnosis are outside this bounded lookup correction.
- Applicable implementation conventions: read
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md` and
  `ai-docs/manuals/release-compatibility.md` before their covered source edits.
  Preserve the existing wsdoc/wsgit dependency boundary documented in
  `internal/wsdoc/gitenv.go:9-14`; no new import cycle is acceptable.

### Checked execution inventory and reuse boundaries

All source paths below are relative to `agents-plugin-tool/`.

- Runtime literal-Git construction/selection sites: `internal/wsgit/git.go#L35-L60`
  (explicit and process branches), `internal/wsindex/git.go#L114-L145`,
  `internal/wsdoc/gitenv.go#L9-L17`, `internal/wsstate/paths.go#L290-L301`,
  `internal/mcp/git_search.go#L752-L769`, `internal/wstrackerhost/runner.go#L58-L93`,
  `internal/wsdashboard/gitbadge.go#L143-L150` (already cached),
  `internal/wsdashboard/gitdiff.go#L103-L109` and
  `internal/wsdashboard/gitpush.go#L206-L211` (uncached process PATH).
  Search: `rg -n '"git"|LookPathEnv|wsgit.Executable' agents-plugin-tool --glob '*.go' --glob '!**/*_test.go'`.
  The old wsdoc coordinates now call `gitCommand`: `internal/wsdoc/project_tree.go#L102-L115`,
  `internal/wsdoc/tickets_scope.go#L245`, `internal/wsdoc/tickets_sage_freshness.go#L287`.
  Generic executable callers also use `LookPathEnv`: `internal/mcp/worktree_hooks.go#L105`
  and `internal/execjob/execjob.go#L197`; keep non-Git resolution uncached.
- Reuse is available at the leaf `wsexec.LookPath`/`CommandContext`, not by
  importing wsgit into wsdoc or wsstate (`internal/wsdoc/gitenv.go#L9-L17`,
  `internal/wsexec/lookpath.go#L1-L4`). Keep the current `Executable` facade and
  runner signatures under decision (1064); its existing
  cache is one mutex-protected successful entry, not a per-context map
  (`internal/wsgit/executable.go#L11-L48`).
- Explicit lookup takes the last matching PATH/PATHEXT entry, folding keys only
  on Windows, skips empty PATH entries, uses default Windows extensions when
  PATHEXT is empty, and returns relative candidates unchanged
  (`internal/wsexec/lookpath.go#L26-L73`). Lookup happens before `cmd.Dir` is set
  in wsstate and git.search; Git's `-C root` is not the lookup cwd
  (`internal/wsstate/paths.go#L290-L293`, `internal/mcp/git_search.go#L758-L764`).
  Do not normalize relative results to absolute paths or merge nil and explicit
  contexts as a caching shortcut.
- The requested `processenv.Explicit` helper is not in this checkout:
  `rg -n 'package processenv|processenv\.Explicit|func Explicit' agents-plugin-tool`
  returned no matches. The current explicit-empty snapshot instead collapses
  to nil in `newProcEnv`/`environ` (`internal/mcp/process_env.go#L77-L79`,
  `internal/mcp/process_env.go#L169-L174`). Reuse the existing Explicit/nil distinction if provided by
  another landing; do not silently introduce that helper or repair the snapshot
  contract in this lookup-only ticket. Its source/landing identity is unverified.
- Existing verification artifacts: `internal/wsgit/executable_test.go#L29-L72`
  counts failed retries and successful reuse and invalidates on PATH changes
  (not a PATHEXT-specific test); `internal/wsexec/lookpath_test.go` pins nil and
  missing-program behavior; `internal/mcp/hosted_env_paths_test.go#L178-L236`
  pins hosted Git selection. Current direct-spawn test-file count is 43 using
  `rg -l 'exec\.Command(Context)?\([^\n]*"git"' agents-plugin-tool --glob '*_test.go' | wc -l`;
  the earlier "about 20" is historical, not the migration scope.

## Prior Decisions

- 10706217 (2026-11-10, commit): "P-M7 (lead decision): a caller environ whose PATH lacks the program is an exec.ErrNotFound error, never the daemon's PATH; nil environ keeps the process lookup." — bearing: constrains
- b76af960 (2026-11-10, commit): "wsexec is a new leaf package because wsdoc may not import wsgit (the {#260720-wsdoc-commit-boundary} rule) and wsstate sits below wsgit; one copy of the lookup beats two more." — bearing: constrains
- 261110-bug-ws-dashboard-daemon-read-amplification (2026-11-10, Result): "Decision 6: `wsgit.Executable()`; the badge's `badgeGitCommand` and the tracker host use it; the resolver test moved to wsgit." — bearing: supports
- 261110-bug-ws-dashboard-daemon-read-storm (2026-11-10, Result): "Decision 4: git resolved once per `PATH`+`PATHEXT` (keyed after the fake-git `PATH` tests showed a process-wide cache pins the first git); a failed lookup is not cached and falls back to bare `git`." — bearing: constrains
- 260930-feat-git-exec-audit-log (2026-09-30, Decisions): "`Runner.RunGit(ctx, root, args...)` stays unchanged." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsgit, internal/wsexec, internal/wsindex, internal/wsdoc, internal/wsstate, internal/mcp, internal/wstrackerhost and internal/wsdashboard execution sites listed above |
| scope.surface | cross-module | shared executable selection across internal packages; no MCP surface change approved |
| scope.new_public_symbol | yes | decision 1064 permits an internal-package Git resolver entry point in wsexec; existing wsgit facades and wire surface remain unchanged |
| scope.new_type_contract | no | private single-entry cache; no new wire/shared-data contract or changed runner signature |
| scope.test_surface | existing | agents-plugin-tool/internal/wsgit/executable_test.go, internal/wsexec/lookpath_test.go, internal/mcp/hosted_env_paths_test.go and runner tests |
| complexity.reuse_points | confirmed | read wsgit.Executable, wsexec.LookPath/CommandContext and hosted environment regression tests |
| complexity.side_effect_risk | high | shared cache can select the wrong caller's executable or stale successful resolution |
| risk.correctness | high | nil/explicit, relative cwd and Windows search semantics differ |
| risk.fit | moderate | shared owner must remain below wsdoc/wsstate and leave generic non-Git lookup alone |
| risk.test | high | concurrency, lookup-count assertions and native Windows selection need coverage; no implementation tests run |
| risk.security_or_contract | high | executable provenance and missing-Git refusal must not fall back to daemon PATH |

## Phases

### Phase 1: Reuse Git resolution on repeated runtime spawns

Route the relevant current runtime Git execution paths through decisions
(1064) and (1066), preserving executable selection for each search context
subject to the explicit unchanged-key stale-success window. Inspect the actual spawn inventory, including the already-cached
badge and explicit-environment tracker path; do not implement against the stale
historical line list alone.

Regression verification: count resolver/filesystem-search invocations rather
than rely on wall-clock thresholds. Repeated spawns with one unchanged search
context must reuse successful resolution; distinct hosted caller PATHs must run
their respective fake Git binaries; PATH/PATHEXT changes and missing-Git then
repaired-Git cases must retain existing behavior. Cover concurrent callers,
relative-search/cwd semantics, nil/empty env, Windows executable selection and
context/timeout preservation. Assert that relative/cwd-sensitive contexts
bypass reuse, concurrent identical cold calls share resolution, failures retry,
and an unchanged eligible key retains a successful entry without a TTL or
launch-error retry. Retain `TestExecutableResolvesOnce`, hosted-path
isolation and relevant runner tests. A bounded local cold-versus-warm benchmark
may corroborate cost without touching a network or slow mounted directory; do
not promise the historical timing or the unrelated stall is fixed. Run affected
`internal/wsgit`, `internal/wsexec`, `internal/wsindex`, `internal/wsdoc`,
`internal/wsstate`, `internal/mcp` and any changed tracker/badge tests, then
`cd agents-plugin-tool && go test ./...`. This draft ran no implementation tests.
