# Pi idle stop: keep alive while pi.spawn children run, label the stop

## Context

Owner decision (1256)=b, 2026-11-16: "a sub-agent is running, so idling is absurd".
A Pi lead (`partingsabotagevelcroimply`) idle-stopped ~10 min after spawning a
worker that kept running; the stop was logged only as `child_exited`, so it
looked like a crash.

Known facts (from investigation, verify in source):
- Default idle timeout 10 min: `agents-plugin-tool/internal/wsagent/profile.go:44`
  (commit fe28fea7f applied it to Pi leads too); `idle_timeout_seconds` 0 disables.
- `idleNow()` = `!turn && !replaying && bgTasks == 0` (`wsagent/runner.go` ~3351).
  Only `ws-execute` background shells/monitors increment bgTasks.
- Running pi.spawn children do not count; the session resumes on child report,
  mail, or owner message.

## Outcome

1. A Pi session with at least one running (not finished/stopped) pi.spawn
   child does not idle-stop. The idle timer re-arms when the last running
   child ends (same treatment as the bgTasks-reaches-zero path).
2. When an idle stop does happen, it is recorded with a distinguishable reason
   (e.g. `idle_timeout`) instead of being indistinguishable from a crash /
   plain `child_exited`, and the dashboard session row/status shows it as an
   idle (절전) stop rather than an exit. Human-facing label may be Korean or
   follow the existing dashboard string style.
3. Check whether Pi guidance (Pi lead/worker system prompt or tool
   description) already tells agents to launch long-running shell work via
   `ws-execute` `background: true` (plain bash background is not counted and
   may die at idle stop). If not, add one short sentence where the existing
   guidance about background execution lives. Do not add new docs.

## Constraints

- Simplest complete change; no new config key (a new key is a T3 manifest
  change). Keep `idle_timeout_seconds` semantics.
- Any new event/field on a tracker or shared surface must be additive (T0/T1:
  old binaries ignore it or show a default). State the tier in the commit
  `## AI Context`.
- Do not change how children report or how parents are woken.
- Read `ai-docs/manuals/release-compatibility.md` and
  `ai-docs/manuals/shipped-surface-boundary.md` before editing
  `agents-plugin-tool/internal/` (AGENTS.md Implementation Conventions).

## Verification

- Unit tests for: idle suppressed while a child runs; timer re-armed after the
  last child ends; stop reason recorded as idle.
- Go tests run ONLY on remote w5090 (`D:/sys_lock.sh remote w5090 run ...`,
  see note remote-build-w5090). NO local `go test`, no test exes, no ws/ws-mcp
  exes, no launcher on this PC. Node/TS tests (if any touched) may run locally.
- One passing focused check closes the work; then R1 review, fixes-only R2 for
  substantive findings.
</content>
</invoke>
