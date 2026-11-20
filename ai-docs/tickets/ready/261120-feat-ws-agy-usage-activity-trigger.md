---
title: agy 사용량 활동 기반 자동 갱신
related:
  261119-feat-ws-agy-plan-usage-display: its Decision D14 ((1413)=d, probe only on popover action) is extended by this ticket, superseding its user-action-only rule (Decision 6)
  261116-bug-ws-exe-launch-elimination: its repeated-exe-launch concern bounds Decision 6
  261118-feat-ws-agy-tool-names-and-worker-role: its (154) keeps agy out of normalizedHarness; Phase 2 keys on the identity the merged fix exposes
  261118-refactor-ws-harness-name-branches: future agy lead adapter (Non-goals)
  261114-epic-ws-common-session-foundation: parent of the future adapter work (Non-goals)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7097099f051924ab
sage-review-completeness-reviewed: 7097099f051924ab
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
`wsagent.Runner` (`runAgyUsageCommand`, `internal/wsdashboard/plan_usage_agy.go#L529`).

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
  (`wsdashboard/proxy.go#L71-L72`; `agents-plugin/bin/ws-mcp-launcher.py#L1505`),
  but `normalizedHarness()` keeps only codex, claude and pi
  (`internal/mcp/server_harness.go#L121-L130`), and registration stores
  only that value (`dashboard_sessions.go#L416`). Every keyed lead
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
   method that hands off to the existing `refreshAgy()` in the background:
   the method starts a goroutine and returns, never waiting on the
   plan-usage store's lock, which a running probe holds across its cache
   and history writes (`internal/wsdashboard/plan_usage_agy.go` around
   240-250). Rejected:
   output-rate `complete()` (steered prompts complete mid-turn).
2. **User-started agy lead trigger = ws tool-call proxy** (owner (1457)=a;
   sequencing per (1458), routed by the owner to the develop integrator). A
   keyed, user-started top-level lead session whose session harness is `agy`
   signals agy activity on each ws tool call it makes. It keys on the agy
   session identity the in-flight identity fix (branch
   `impl/develop/agy-handoff-identity`) exposes; no separate raw-hello
   reader. The integrator expects that fix to make `normalizedHarness()` keep
   `agy` (pending impl/develop/agy-handoff-identity), but ready ticket
   `261118-feat-ws-agy-tool-names-and-worker-role` (154) and its Phase 1
   Result deliberately keep `agy` out of `normalizedHarness()` and use a
   separate `playbookHarness()`, so Phase 2 uses whatever identity the merged
   fix actually provides rather than hard-coding `currentHarness()`. Hosted
   sessions do not signal through this path (`dashboardTouch()` hands a
   hosted session to `dashboardTouchHosted()` before any user-started lead
   handling); Decision 1 covers them. It misses turns with no ws tool call
   and may fire a little before the turn ends; with the 10-minute debounce
   that does not change the shown values. Signal path (1459): the
   daemon-hosted server reaches the daemon in memory through the host it
   already holds (`mcp.ServerOptions.Live`, a `wsdashboard.LiveHost`, set in
   `cmd/ws-mcp/proxy.go` and `cmd/ws-mcp/hosted_cli.go`) and calls the same
   daemon method as Decision 1; a server with no daemon host signals nothing.
   Whether that is a new `LiveHost` method or a sibling option, and the
   names, are the implementer's call. Rejected: a new HTTP or IPC route to
   the daemon (new wire surface against Decision 5 and I/O on the tool-call
   path); a private raw-hello reader (duplicates the identity fix); hosted
   only, deferring user-started leads to an adapter (owner chose a).
3. **Everything else in the probe stays** (lead announced default,
   acknowledged with (1457)). The popover open/refresh trigger stays; the
   activity signal adds to it. `refreshAgy()`'s in-flight check, billed-turn
   latch, auth-required hold, ~10-minute debounce from spawn, 30 s timeout,
   cache, placeholder and history behavior are unchanged; an activity signal
   inside the debounce window is a no-op. Rejected: removing the popover
   trigger (a researcher suggestion; the owner asked for an addition).
4. **No self-trigger.** The probe's own `agy -p /usage` process must never
   count as agy activity: it runs outside the runner, so it cannot produce a
   hosted turn end; and (1460) `agyCommandEnv` removes `WS_MCP_HARNESS` from
   the probe's environment (today it only sets `AGY_CLI_DISABLE_AUTO_UPDATE`
   and inherits the daemon's environment,
   `internal/wsdashboard/plan_usage_agy.go#L506-L520`), so that, should the
   probe ever connect to ws MCP, it does not present the agy identity
   Decision 2 keys on, as far as that identity derives from
   `WS_MCP_HARNESS` (Phase 2 re-checks this against the merged identity
   fix). Rejected: a new private
   environment marker (it would need a new hello or env field to reach
   Decision 2's check, against Decision 5). The strip is a belt-and-braces
   guard, not the guarantee: if a user's agy MCP config sets
   `WS_MCP_HARNESS` in a static `env` block, agy passes that value on
   regardless. The real bound is that a `/usage` probe makes no keyed ws tool
   call, plus the debounce and the billed-turn latch.
5. **Compatibility T0.** No MCP tool or argument, hello field, cache or
   history file shape, tracker event or manifest field changes. An old
   binary keeps probing on popover action only; the agy cache and history
   files are unchanged (`release-compatibility.md` T0).
6. **Process-spawn rule.** `performance.md` Rule 6 allows a process spawn
   only when no in-process route exists; the agy probe is the existing
   approved exception (D14 records that no harmless in-process quota route
   exists). This ticket changes only how often it may run: at most one probe
   per ~10 minutes, and only while agy is active. This supersedes D14's
   "only on user action" rule at the owner's request (2026-11-20). The
   concern behind D14, repeated exe launches flagged by Windows antivirus
   (`261116-bug-ws-exe-launch-elimination`), is bounded by that rate and by
   spawning nothing while agy is idle.

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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/, agents-plugin/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)
- The activity signal must be off the interactive path: no file I/O and no
  blocking on the turn-end or tool-call path; the probe stays asynchronous.
- Tests never run the real `agy` binary; use the existing fake-agy fixtures
  in `plan_usage_agy_test.go`.
- Go tests run locally and narrowly with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 -run '<pattern>' ./internal/<pkg>`
  from `agents-plugin-tool/`; never the whole `./internal/mcp` or
  `./internal/wsdashboard` package.
- Do not start, stop, rebuild or swap the shared ws daemon. No Electron.

## Prior Decisions

- 261119-feat-ws-agy-plan-usage-display (2026-11-19, Decisions D14): "`agy -p /usage` runs only on user action (1413)=d. The probe runs only when the usage popover is opened or refreshed ... within the ~10-minute debounce" — bearing: contradiction-candidate
- 261116-bug-ws-exe-launch-elimination (2026-11-19, commit c62a4313): "(1413)=d: agy -p /usage runs only on user action ... because periodic subprocess launches conflict with 261116-bug-ws-exe-launch-elimination (Windows AV flags repeated exe launches)" — bearing: contradiction-candidate
- 261118-feat-ws-agy-tool-names-and-worker-role (2026-11-18, Decisions (154) and Phase 1 Result): "`normalizedHarness` and `server_harness.go` gain no agy" case; the declared harness is a separate `playbookHarness()` — bearing: constrains
- 261119-feat-ws-agy-plan-usage-display (2026-11-19, Decisions D2): "A probe runs asynchronously with a ~10-minute debounce and never on an interactive path." — bearing: constrains
- c7c231c9 (2026-11-20, commit): "Enforced Orca safety latches (billed-turn latch ..., auth-required hold ...), 30s outer timeout with process-tree kill ... and 10-minute debounce starting at spawn on forced popover refreshes only." — bearing: constrains
- e372b93b (2026-11-20, commit): "Bound planUsageStore background probes to a daemon-lifetime context cancelled by Daemon.Close() via beginClose()/waitClose(ctx) ... and guarded store/history writes against s.closed / context cancellation." — bearing: constrains
- 1a24b064 (2026-11-20, commit): "Pending state is response-only and memory-backed; cached agy files and probe behavior remain unchanged, preserving the interactive-path file-operation count." — bearing: constrains
- 261120-bug-ws-sweep-1120-agy-fixes (2026-11-20, commit 049d3eec): "agy.model_wait_retries (default 3, 0 disables) with a visible exhaustion diagnostic published exactly once on TurnEnd, EOF, or grace force-stop." — bearing: constrains
- d6c2731e (2026-11-17, commit): "Move-only same-package relocation of harness/client metadata declarations; no behavior change (T0)." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | internal/wsagent/runner.go, internal/wsagent/runner_events.go, cmd/ws-mcp/dashboard.go, internal/wsdashboard/daemon.go, internal/wsdashboard/plan_usage_agy.go, internal/mcp/dashboard_sessions.go and the daemon host seam of Decision 2, either the wsdashboard.LiveHost interface or a sibling of ServerOptions.Live in internal/mcp/process_env.go wired in cmd/ws-mcp/proxy.go (Phase 2) |
| scope.surface | cross-module | wsagent runner option callback beside Options.TrackerChanged, wsdashboard daemon method, mcp tool-call path; all under internal/, no MCP tool, hello field or file shape change (Decision 5) |
| scope.new_public_symbol | yes | exported runner option callback field and exported daemon method (names unsettled), mirroring exported Options.TrackerChanged and Daemon.TrackerChanged |
| scope.new_type_contract | yes | new func-typed callback on wsagent Options wired through a lateFunc in cmd/ws-mcp/dashboard.go; signature unsettled |
| scope.test_surface | existing | internal/wsdashboard/plan_usage_agy_test.go (fakeAgyEnv), internal/wsagent/agy_test.go, internal/mcp/dashboard_sessions_test.go; the new cases extend these, no new file named |
| complexity.reuse_points | confirmed | refreshAgy() in plan_usage_agy.go#L189-L212, TrackerChanged lateFunc wiring in cmd/ws-mcp/dashboard.go#L87-L168, dashboardTouch() in internal/mcp/server_dispatch.go#L159 |
| complexity.side_effect_risk | moderate | spawns an external agy process automatically on activity; self-trigger loop and Windows AV launch concern (261116-bug-ws-exe-launch-elimination) bound by 10-minute debounce and latches |
| risk.correctness | moderate | must separate settled live turn end from replay, watchdog-continued end and non-agy; Phase 2 may fire before the turn ends and depends on the identity fix (pending impl/develop/agy-handoff-identity) |
| risk.fit | moderate | Phase 2 needs an agy session identity, which normalizedHarness() does not give today (internal/mcp/server_harness.go#L121-L130) and 261118-feat-ws-agy-tool-names-and-worker-role (154) keeps out of it; Phase 2 reaches the daemon through the existing ServerOptions.Live host (Decision 2) |
| risk.test | moderate | fake-agy fixtures exist, but runner turn-end, replay and watchdog cases need wsagent fixtures and the debounce needs an injectable clock; narrow local test runs only |
| risk.security_or_contract | moderate | process-spawn rule 6 exception and extension of D14 user-action-only decision; T0 claim in Decision 5 unverified by measurement |

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
- The probe's command environment has no `WS_MCP_HARNESS` (Decision 4).
- The daemon method returns without waiting while a probe holds the
  plan-usage store's lock.
- The agy cache and history files an activity-triggered probe writes have
  the same shape as a popover probe's; the existing cache and history tests
  pass unchanged (Decision 5).

### Result (c946b532) - 2026-11-20

- Added a runner `AgyActivity` callback beside `TrackerChanged`, raised once
  after a live hosted agy child or lead turn settles. Replay, non-agy turns,
  compaction/non-turn endings, and watchdog-intercepted continuations do not
  signal activity.
- Late-bound the callback to `Daemon.AgyActivity`, whose goroutine handoff
  returns before taking the plan-usage mutex. The turn-end path adds no file
  I/O or process wait; `refreshAgy` remains the only probe admission path and
  retains its in-flight, debounce, billed-turn, auth-hold, cache, history, and
  shutdown guards.
- Removed `WS_MCP_HARNESS` from the probe environment while retaining
  `AGY_CLI_DISABLE_AUTO_UPDATE=true`. Activity and popover probes write the
  same schema-1 cache and history bytes, preserving the T0 decision and the
  existing popover behavior.
- Lite review found one Important test race: the duplicate-signal assertions
  used a fixed sleep before the activity goroutine necessarily ran. Commit
  `c946b532` added a package-private completion seam and waits for the handoff
  plus any admitted probe; the one-pass review has no open findings.
- Final verification passed:
  `go test -count=1 -run '^TestAgyActivityCallback' ./internal/wsagent`;
  `go test -count=1 -run '^TestPlanUsageAgy' ./internal/wsdashboard`;
  `go test -count=1 -run 'Tracker|Asset' ./internal/wsdashboard`;
  `go test -count=1 -run 'Test(LateFuncBindsAfterConstruction|AgentRunnerCatalogs|CallFallbackRegistrationAndRenderDescriptor)' ./cmd/ws-mcp`;
  native and `GOOS=windows` `go vet` on `./internal/wsagent`,
  `./internal/wsdashboard`, and `./cmd/ws-mcp`. Every Go command used
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; tests used only fake/test agy
  helpers.

### Phase 2: User-started agy lead proxy

Starts after `impl/develop/agy-handoff-identity` (agy harness identity fix)
has merged into develop: check that develop contains that branch's commits
and that a user-started agy lead's session now resolves to an agy identity.
Phase 1 does not depend on it and may land and ship alone. Implement
Decision 2.

Verification (Go tests):
- A user-started lead session carrying the agy identity the merged fix
  exposes triggers a probe on a ws
  tool call; a codex, claude or pi lead's call does not, nor does a server
  with no daemon host, nor does a hosted agy session's call.
- The signal adds no file I/O or blocking on the tool-call path, including
  while a probe holds the plan-usage store's lock.
