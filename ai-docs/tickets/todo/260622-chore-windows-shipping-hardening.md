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
