---
title: agy 사용량 활동 기반 자동 갱신
related:
  261119-feat-ws-agy-plan-usage-display: its Decision D14 ((1413)=d, probe only on popover action) is extended by this ticket
---

# agy plan usage refreshed on agy activity

## Background

The owner (2026-11-20 21:25) asked that the agy `/usage` probe also run on
its own whenever an agy session actually produces output (a hosted agy
subagent or an agy top-level lead), under a 10-minute debounce, so the
gutter stays fresh while agy is in use and nothing is spawned while agy is
idle.

Today (`.done/261119-feat-ws-agy-plan-usage-display`, D14 (1413)=d) the probe
runs only on a usage-popover open or refresh (`/rail/usage?refresh=1`,
`plan_usage_probe.go` around 33-38). Every probe already goes through
`refreshAgy()` (`internal/wsdashboard/plan_usage_agy.go` around 189-253):
in-flight check, billed-turn latch, auth-required hold, a ~10-minute
debounce whose window starts at spawn, a 30 s timeout, cache write and
history append. The probe is a direct `exec.CommandContext` outside the
`wsagent.Runner` (around 522).

Evidence (read-only research, 2026-11-20):
- Hosted agy children: `agyMapper.response()` reaches `endTurn()` only for
  the last outstanding prompt (`internal/wsagent/agy.go` around 910, 1174);
  `conv.handle()` gets `TurnEnd`, keeps replayed events quiet
  (`ev.Replay`, `conv.replaying`), lets the agy watchdog consume retryable
  ends that continue the same logical turn, then calls `c.endTurn()`
  (`runner_events.go` around 17, 116; `agy_watchdog.go` around 335;
  `runner_conv.go` around 157).
- The runner and the plan-usage store live in the same daemon process
  (`cmd/ws-mcp/dashboard.go` around 95-112; `wsdashboard/daemon.go` around
  421, 477). The runner→daemon `TrackerChanged` callback is wired late
  through a `lateFunc` (`wsagent/runner.go` around 188; `dashboard.go`
  around 85, 168).
- Hosted agy leads (dashboard-spawned) run through the same agy backend in
  the runner (`internal/wsagent/agy.go` around 22, 149), so they produce the
  same `endTurn()` as hosted children.
- User-started (non-hosted) agy leads: the proxy hello carries `Harness` from `WS_MCP_HARNESS`
  (`wsdashboard/proxy.go` around 54; `agents-plugin/bin/ws-mcp-launcher.py`
  around 1489), but `normalizedHarness()` keeps only codex, claude and pi
  (`internal/mcp/server_harness.go` around 121), and registration stores
  only that value (`dashboard_sessions.go` around 411). Every keyed lead
  tool call reaches `dashboardTouch()` (`server_dispatch.go` around 159;
  `dashboard_sessions.go` around 323). No agy turn-end feed exists for a
  top-level lead.

## Decisions

1. **Hosted agy trigger** (owner proposal; lead technical call shown on the
   board and acknowledged with (1457); develop integrator confirmed on
   2026-11-20 that the holder work leaves this seam unchanged and adds no
   common turn-end hook). A hosted agy conversation's settled turn end, child
   or hosted lead,
   signals agy activity: the final `endTurn()` of a live (non-replay) turn of
   a `BackendAgy` conversation, not a watchdog-intercepted end that
   continues the same turn. Signal path: a runner option callback beside
   `TrackerChanged`, wired late in `cmd/ws-mcp/dashboard.go` to a daemon
   method that calls the existing `refreshAgy()` in the background. Rejected:
   output-rate `complete()` (steered prompts complete mid-turn).
2. **User-started agy lead trigger = ws tool-call proxy** (owner (1457)=a;
   sequencing per (1458), routed by the owner to the develop integrator). A
   keyed, user-started top-level lead session whose session harness is `agy`
   signals agy activity on each ws tool call it makes. It keys on the session
   harness as fixed by the in-flight agy identity fix (branch
   `impl/develop/agy-handoff-identity`, which makes `normalizedHarness()` keep
   `agy`); no separate raw-hello reader. It misses turns with no ws tool call
   and may fire a little before the turn ends; with the 10-minute debounce
   that does not change the shown values. Rejected: a private raw-hello
   reader (duplicates the identity fix); hosted only, deferring user-started
   leads to an adapter (owner chose a).
3. **Everything else in the probe stays** (lead announced default,
   acknowledged with (1457)). The popover open/refresh trigger stays; the
   activity signal adds to it. `refreshAgy()`'s in-flight check, billed-turn
   latch, auth-required hold, ~10-minute debounce from spawn, 30 s timeout,
   cache, placeholder and history behavior are unchanged; an activity signal
   inside the debounce window is a no-op. Rejected: removing the popover
   trigger (a researcher suggestion; the owner asked for an addition).
4. **No self-trigger.** The probe's own `agy -p /usage` process must never
   count as agy activity: it runs outside the runner, so it cannot produce a
   hosted turn end; and it is started with a private environment marker
   (or without `WS_MCP_HARNESS`) so that, should it ever connect to ws MCP,
   its calls are ignored by Decision 2's signal.
5. **Compatibility T0.** No MCP tool or argument, hello field, cache or
   history file shape, tracker event or manifest field changes. An old
   binary keeps probing on popover action only; the agy cache and history
   files are unchanged (`release-compatibility.md` T0).
6. **Process-spawn rule.** `performance.md` Rule 6 allows a process spawn
   only when no in-process route exists; the agy probe is the existing
   approved exception (D14 records that no harmless in-process quota route
   exists). This ticket changes only how often it may run: at most one probe
   per ~10 minutes, and only while agy is active.

## Non-goals

- An agy lead adapter feeding `dashboard.intake` turn events (as the Claude
  hook mod and the Pi extension do). It belongs to the harness
  common-interface work (`261118-refactor-ws-harness-name-branches`, epic
  `261114-epic-ws-common-session-foundation`); when it lands, Decision 2's
  proxy can switch to its turn ends.

## Constraints

- Applicable manuals: `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md` (for `internal/mcp/`).
- The activity signal must be off the interactive path: no file I/O and no
  blocking on the turn-end or tool-call path; the probe stays asynchronous.
- Tests never run the real `agy` binary; use the existing fake-agy fixtures
  in `plan_usage_agy_test.go`.
- Go tests run locally and narrowly with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 -run '<pattern>' ./internal/<pkg>`
  from `agents-plugin-tool/`; never the whole `./internal/mcp` or
  `./internal/wsdashboard` package.
- Do not start, stop, rebuild or swap the shared ws daemon. No Electron.

## Phases

### Phase 1: Hosted agy turn-end trigger

Implement Decisions 1, 3 and 4.

Verification (Go tests):
- A settled, live hosted agy turn end (child and hosted lead) triggers one
  probe; a replayed turn end, a watchdog-continued end, and a non-agy turn
  end do not.
- A second signal inside the debounce window spawns nothing; the billed-turn
  latch and auth-required hold still block activity-triggered probes.
- The popover refresh path still probes as before.
- The probe's command environment carries the self-trigger guard of
  Decision 4.

### Phase 2: User-started agy lead proxy

Starts after `impl/develop/agy-handoff-identity` (agy harness identity fix)
has merged into develop. Implement Decision 2.

Verification (Go tests):
- A user-started lead session with harness `agy` triggers a probe on a ws
  tool call; a codex, claude or pi lead's call does not.
- The signal adds no file I/O or blocking on the tool-call path.
