---
title: Windows shipping hardening (mercenary v1) + branch-pinned acceptance
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260620-chore-pre-shipping-windows-surface-verification: predecessor; verified the Windows surface via `go test` only, never the launcher cold-install path
  260605-research-ws-native-subagent-pivot: mercenary is the retained delegation path this ticket must make Windows-correct
related-mental-model:
  - named-agent-runtime
  - plugin-runtime
---

# Windows shipping hardening (mercenary v1) + branch-pinned acceptance

## Background

The epic `260605` branch is a fast-forwardable descendant of `main`
(`main..HEAD` = the whole playbook-factory pivot, ~353 files). Linux is well
dogfooded and mercenary is functionally verified on Linux. The only remaining
shipping surface is Windows.

Predecessor `260620` "verified" Windows, but **only via `go test ./...` against a
pre-built binary** — it never exercised the launcher's first-install / first-MCP-load
path, which is exactly where the highest build-script risk lives. Three scoped
explores (build-script / mercenary / bootstrapping) against `main..HEAD` found a
coherent cluster of Windows-only paths that Linux dogfooding can never exercise.

This ticket makes the Windows surface shipping-correct, with **mercenary-on-Windows
in v1 scope**, then proves it on a real Windows host via a branch-pinned install —
so the epic merges to `main` only after Windows passes.

## Decisions

- **mercenary-on-Windows is v1 shipping scope** (user decision). The mercenary
  quoting/path/process fixes are not deferrable.
- **Epic merge is deferred** until Windows static verification (Phase A/B) and
  branch-pinned acceptance (Phase C) pass.
- **`python3` resolvability is a user prerequisite, out of scope.** `plugin.json`
  mcpServers runs `command: "python3"`; if Python is not on PATH the MCP server
  never starts. Treated as a documented install prerequisite (Windows: Python on
  PATH), not a code fix.
- **Branch-pinned verification strategy:** test the epic build on real Windows
  without exposing it to other consumers via
  `claude plugin marketplace add kang-sw/devenv@260605-epic-ws-playbook-factory-pivot`
  then `claude plugin install ws@kang-sw-devenv` (cold install → real Go build).
  The marketplace is this repo itself (`source: "./agents-plugin"`), so pinning the
  marketplace ref pins the plugin; marketplace.json needs no change.
- **Cache invalidation:** the new AGENTS.md "version bump on dev-merge" rule
  (`bump-ws-version.sh`) is what makes the branch-pin loop pick up fresh builds —
  Claude Code keys cache on the `version` string. Without a bump, branch-pin
  reinstall serves stale cache (the dev-build staleness already hit in dogfooding).

## Constraints

- **Live-host safety (hard):** every Windows process-tree termination MUST stay
  PID/job-scoped (Toolhelp32 PPID walk or job object). Never terminate by image
  name (`taskkill /IM`) or any broad sweep — the dogfooding WSL2 host runs a live
  `claude.exe`. (Verified already honored across the cancel paths; preserve it.)
- **Contract unchanged.** These are behavior-preserving conformance fixes to the
  existing `named-agent-runtime` behavior on Windows; no new caller-visible
  interface. Spec text should need no change (confirm at closeout).

## Spec Impact

- **Target spec areas:** `named-agent-runtime` (mercenary process spawn, command
  quoting, file replacement, and process-liveness behavior) and `plugin-runtime`
  `## Windows Plugin-Managed Startup` (`#260505-windows-plugin-managed-startup`,
  launcher cold-load). The `python3`-on-PATH prerequisite is already specified
  there (lines ~266-269), matching this ticket's out-of-scope decision.
- **Expected caller-visible change:** none. Every Phase A/B item brings the
  Windows runtime into behavioral parity with the cross-platform contract these
  specs already describe (mercenary spawn/quoting correctness, atomic file
  replacement, `ERROR_ACCESS_DENIED → alive` liveness symmetry, PID-scoped
  tree-kill on the sync-runner timeout, rsrc/runtime cold-load ordering). No new
  tool, schema, or semantic surface is introduced.
- **Contract-first spec: no.** These are behavior-preserving conformance fixes;
  the existing spec text describes the intended behavior already. Confirm at
  closeout that no spec wording drifted; if any Windows item turns out to require
  a contract clarification (e.g. an observable mercenary-on-Windows nuance found
  in Phase C), capture it then.

## Findings map (evidence for the implementer)

Verified-safe already: PID-scoped tree-kill (Toolhelp32 PPID walk, no image-name
kill); liveness zombie fix `326fa74f` (zero-timeout `WaitForSingleObject` across
wsagent/execjob/wsstate); config builtin-only cold start; playbook manifest has no
dangling references; the two rsrc trees are byte-identical.

## Phases

### Phase A: Static code hardening (mercenary v1)

Goal: fix the Windows-only code defects that `go test ./...` can verify on a
Windows host (the path Phase 3 of `260620` proved works). This is the "static
verification" gate.

Items (each with a Windows unit test where `go test` can cover it):
1. `wsagent/agent.go:~2215` `shellQuote` produces POSIX `'...'` quoting baked into
   the interrupt-hook command string — meaningless to `cmd.exe`/PowerShell. Make
   the hook command Windows-correct (or skip/replace the hook mechanism on
   Windows). Add a round-trip test with a root/name containing spaces.
2. `wsagent/codex.go:152` `model_instructions_file=%q` embeds a Windows backslash
   path; codex's config parser may drop the system prompt. Apply `filepath.ToSlash`
   before `%q` (or otherwise emit a parser-safe path). Test with a backslash path.
3. `wsagent/agent.go:~260` `asyncWorkerCommandFor` cache-launcher probe does not try
   the `.exe` extension, so a Windows native launcher (`ws-mcp-launcher.exe`) is not
   found → async/mercenary spawn breaks. Add `.exe` probing. Test the Windows cache
   layout.
4. `wsagent/agent.go:~2328` (and the `wsstate` analogue) `replaceFile` uses
   remove+rename, non-atomic on Windows and failing on `ERROR_SHARING_VIOLATION`
   with a concurrent reader (dashboard/AV). Use a Windows-atomic replace
   (`MoveFileEx` `MOVEFILE_REPLACE_EXISTING`).
5. `processAlive` (wsagent / execjob / wsstate `process_*_windows.go`): `OpenProcess`
   `ERROR_ACCESS_DENIED` is treated as "not alive"; Unix treats `EPERM` as alive.
   Add the `ACCESS_DENIED → alive` symmetry across all three packages.
6. `wsagent/runner_command_windows.go` sets no `cmd.Cancel`; a synchronous-runner
   context-timeout kills only the root process, leaving children (Unix kills the
   group). Add a PID-scoped tree-kill cancel for the sync runner timeout path.
7. `agents-plugin-tool/scripts/smoke-ws-mcp.sh:53` still calls the removed
   `ws.lead.login`; rename to `ws.ferrule`. The only manual sanity tool must not
   ship broken.

Verification: `go test ./...` green on a Windows host (go1.26.x), plus the new
Windows-specific unit tests above.

### Result (8461b4cf) - 2026-06-22

All 7 items implemented (range `e910b3f6..8461b4cf`); every fix is isolated behind
a `//go:build windows` file or a `runtime.GOOS == "windows"` branch, so Unix code
paths are behaviorally unchanged and no caller-visible contract changed
(confirmed by the Fit review).

Per-item outcome:
1. Platform-aware interrupt-hook quoting: new build-tagged `quoteHookArg`
   (`hook_quote_windows.go` double-quote / `hook_quote_unix.go` → `shellQuote`);
   `interruptHookCommand` calls it.
2. `codex.go` applies `filepath.ToSlash` before `%q` for `model_instructions_file`
   (no-op on Unix; removes backslash-escape ambiguity on Windows).
3. `cacheLauncherCommand` Windows-correct probe — **refinement vs the planned
   ".exe probe"**: the real defect was that the extensionless POSIX shell shim
   (which ships) was returned before the `.py`+python branch and is unrunnable on
   Windows. Fix: on Windows skip the shell shim, try `ws-mcp-launcher.exe`, then
   fall through to `.py`+python; non-Windows order unchanged.
4. `replaceFile` (wsagent + wsstate) delegates to a build-tagged
   `atomicReplaceFile` — Windows uses `MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH)`
   with a bounded `ERROR_SHARING_VIOLATION` retry; Unix stays `os.Rename`.
5. `processAlive` (wsagent/execjob/wsstate) treats `OpenProcess`
   `ERROR_ACCESS_DENIED` as alive via `openErrorMeansAlive` (mirrors Unix `EPERM`);
   the existing `WaitForSingleObject` zombie probe is untouched.
6. `runner_command_windows.go` sets `cmd.Cancel` to the existing PID-scoped
   `cancelAsyncProcessTree` (Toolhelp32 PPID walk) — **PID-scoped hard constraint
   verified honored; no image-name termination anywhere in the diff** (Fit review
   grep-confirmed).
7. `smoke-ws-mcp.sh` tool name `ws.lead.login` → `ws.ferrule` (confirmed against
   `bootstrapToolName` in `internal/mcp/server.go`).

Verification (Linux/WSL2 host): `go build/test/vet ./...` and
`GOOS=windows GOARCH=amd64 go build/vet ./...` plus
`GOOS=windows go test -c ./internal/wsagent ./internal/execjob ./internal/wsstate`
all green (lead re-ran independently). Review: partitioned correctness/fit/test,
all clean after one cycle (test nil-case gap fixed). Mental-model invariants
recorded in `named-agent-runtime.md` (`eea18f81`).

Deferred to Phase C (require a real Windows host to execute, code is in place and
cross-compiles): item 1 cmd.exe hook-quoting empirical behavior, item 2
backslash→slash transform on native Windows paths, item 6 sync-runner subtree
reap on context timeout.

### Phase B: Launcher cold-load robustness

Goal: harden the first-install / first-load launcher paths that only a real
Windows cold install can fully exercise; land the clearly-correct fixes by review,
defer empirical confirmation to Phase C.

Items:
1. rsrc materialization race: `apply_rsrc_root_env` is a one-shot check; if `rsrc/`
   is not yet materialized when the launcher runs, `WS_RSRC_ROOT` is unset and the
   binary falls back to a wrong derived path → every `playbook.print`/`render`
   fails. Extend the existing `runtime.json` wait (the `260524` fix) to also cover
   `rsrc/`.
2. Cheap robustness only: widen/retry `wait_for_runtime_contract` against Windows
   AV scan holds; make `os.replace`-over-a-running-`.exe` recover (unlink-then-
   replace or compatible-binary fallback) for in-place upgrade while running.

Verification: code review; empirical confirmation folded into Phase C. Do not
expand scope into speculative fixes — let Phase C reveal which actually bite.

Depends on: independent of A, but C verifies both A and B.

### Result (da1047fb) - 2026-06-22

All Phase B items implemented in the canonical launcher
`agents-plugin/bin/ws-mcp-launcher.py` (commits `ab1460d4` impl, `da1047fb`
review-fix); behavior-preserving robustness, no caller-visible contract change
(Fit review confirmed; spec closeout below).

Per-item outcome:
1. rsrc materialization race: new `wait_for_rsrc_tree(plugin_dir)` does a bounded
   wait for `<plugin>/rsrc/manifest.json` (populated-tree sentinel) before the
   one-shot `apply_rsrc_root_env` seam decision in `main()`. **Refinement vs the
   planned "extend the runtime.json wait":** made it **best-effort** — on timeout
   it `note()`s and proceeds rather than `fail()`, because `apply_rsrc_root_env`
   already no-ops gracefully when `rsrc/` is absent, so a hard-fail would add a
   new unproven failure path. Happy path short-circuits (sentinel already present
   → immediate return, zero latency).
2. AV/transient tolerance: `wait_for_runtime_contract` timeout is OS-aware (10s on
   `os.name=="nt"`, 2s elsewhere); `read_runtime_contract` retries on
   `(OSError, ValueError)` before `fail()`. The `(OSError, ValueError)` width is
   required, not cosmetic — a correctness-review **critical** caught that the
   first cut narrowed to `(OSError, json.JSONDecodeError)`, which drops
   `UnicodeDecodeError` (also a `ValueError` subclass); a mid-write/byte-corrupt
   contract — exactly the cold-install window targeted — would have escaped as an
   uncaught traceback instead of the clean `fail()` path. Fixed in `da1047fb`.
3. `os.replace` contention: `install_tmp_runtime` wraps the replace in a bounded
   retry (5 attempts, ~10ms exp backoff), then falls through to the **existing**
   compatible-binary fallback, then `fail`. **Item-2b interpretation:** version-
   stamped binary names mean a true upgrade targets a *new* filename, so the only
   locked-target case is a same-version reinstall race (existing binary is
   byte-compatible → fallback reuses it). The speculative rename-aside / unlink-
   then-replace dance was rejected for Phase B (scope discipline) and deferred to
   Phase C iff empirically needed.

Scope: **canonical launcher only.** Discovered the two launcher copies already
intentionally diverge — `agents-plugin-wsflow/bin/ws-mcp-launcher.py` never
received the `260524` `wait_for_runtime_contract` fix and did not receive Phase B.
wsflow is the non-user-facing agentless derivative and is not the Windows shipping
target, so porting would expand scope. Captured as follow-up idea ticket
`260622-bug-wsflow-launcher-coldload-divergence`.

Verification (Linux/WSL2 host): `python3 -m unittest discover agents-plugin/tests`
**39 tests green** (3 new coldload tests + tightened retry-count assertions),
`python3 -m py_compile agents-plugin/bin/ws-mcp-launcher.py` clean,
`python3 -m unittest discover agents-plugin-wsflow/tests` 8 green (untouched). Lead
re-ran all three independently. Review: partitioned correctness/fit/test — Fit and
Test clean (Fit 3 style minors no-action; Test 2 minors fixed), Correctness 1
critical fixed and lead-adjudicated after the verbatim fix landed with new test
coverage. Live-host safety honored: no process-termination logic added. Mental-
model invariants recorded in `plugin-runtime.md` (`14694cc3`).

**Spec closeout:** no `ai-docs/spec/plugin-runtime.md` wording drifted. The
materialization-wait (lines ~135-138) and `WS_RSRC_ROOT`-when-present (lines
~102-103) descriptions still hold; Phase B only ensures those documented
behaviors survive Windows cold-install timing — no new interface, env var, or
contract field. Contract-first remained: no.

Deferred to Phase C (require a real Windows host): empirical confirmation that the
rsrc wait, AV-scan timeout/retry, and `os.replace` retry actually clear the cold-
install paths under a real AV/extraction race; code is in place and unit-tested.

### Phase C: Branch-pinned Windows acceptance

Goal: prove the epic build on a real Windows host without exposing it to other
consumers, then clear the merge.

Steps:
1. Ensure a version bump landed on the branch (per the AGENTS.md rule) so the
   cache invalidates.
2. On Windows: `claude plugin marketplace add kang-sw/devenv@<epic-branch>` →
   `claude plugin install ws@kang-sw-devenv`. Confirm the MCP server boots (Python
   prerequisite assumed) and the Go binary builds cold.
3. Run a real mercenary call round-trip (claude and/or codex backend): register →
   call → result, plus one cancel, confirming PID-scoped termination does not reach
   the live `claude.exe`.
4. Fix anything that breaks; bump + reinstall; iterate until clean.
5. On green: the epic is cleared for merge to `main` + ship.

Verification: real Windows cold install + at least one mercenary round-trip and one
cancel, observed clean.

Depends on: Phase A (and B) landed and version-bumped on the branch.
