---
title: Claude mod MCP watchdog that reconnects a dropped plugin server without the model
related:
  261018-feat-claude-mod-mcp-reconnect-lever: extends; reuses its reconnect step and shares its server-name resolution
  261018-research-claude-mod-ws-mcp-reconnect: source; its watchdog Proposal and the verified timer reconnect finding
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 26f91b05f1bf06a9
sage-review-completeness-reviewed: 26f91b05f1bf06a9
---

# Claude mod MCP watchdog that reconnects a dropped plugin server without the model

## Background

On a native-Windows host, an endpoint security agent force-terminates every
`cmd.exe` and `powershell.exe` once a night (00:00-02:00, observed over ten
nights); `python.exe` and `node.exe` survive. Claude Code on Windows launches
plugin stdio MCP servers as `claude.exe -> cmd.exe /d /s /c "python3 ...
ws-mcp-launcher.py serve --stdio" -> python3.13.exe -> ws-mcp.exe`, and the
mod's `mailbox wait` child as `cmd.exe /q /d /s /c "...python3.exe ..."`
(process-tree read on the host, 2026-10-30). The `cmd.exe` wrapper is
inserted by Claude Code, not by ws: the plugin manifest names `python3`
directly and the launcher runs the binary with `subprocess.call` on Windows.
Pi launches the same launcher without `cmd.exe`. So every Claude session
left open overnight loses its ws MCP server and its mailbox push, and stays
that way until a person or the model triggers `/mcp reconnect`.

The reconnect lever (`request_mcp_reconnect`) only runs when the model calls
it at a turn, and an idle overnight session has no turn. The research
ticket verified that `$.command.run({ command: "mcp", args: "reconnect
<server>" })` from a `$.clock.every` timer restores the server; the lever
ticket left a watchdog out of scope (user, 2026-10-18), and the user now
brings it in scope (user, 2026-10-30).

A second gap surfaced in dogfood: a session whose server never came up at
start (the launcher was still downloading the runtime binary) answered the
lever with "its server name was never resolved in this session". The server
name is learned only from a connected `$.mcp.connect($.plugin.name)`, so
neither the lever nor a watchdog can reconnect such a session.

## Decisions

- **Detect a drop by the tool list.** A tick reads `$.tool.list` and treats
  the server as dropped when none of its tools (the `mcpToolName(server, '')`
  prefix) is listed. Rejected: an MCP ping call per tick (a round-trip each
  tick; the tool list is already the lever's success judge). (user,
  2026-10-30)
- **Reuse the lever's reconnect step.** A detected drop runs the same
  bounded attempt the lever runs (`/mcp reconnect <server>` through
  `$.command.run`, then the tool-list poll, then server re-resolution), not
  a second implementation. (user, 2026-10-30)
- **Two knobs.** `claude.mcp_watchdog_enabled` (boolean; default `true` in
  `ws`, `false` in `wsflow`) and `claude.mcp_watchdog_interval_minutes`
  (positive integer; default `10` in both packages), each declared in its
  package's `config-manifest.json` and read keyless at session start like
  the other mod knobs. Rejected: a single interval knob where `0` means off
  (breaks the `claude.*_enabled` pattern); a 1-2 hour default interval (the
  check is a local read, so a short interval costs nothing and caps
  downtime). (user, 2026-10-30)
- **Success is quiet.** A successful watchdog reconnect writes one transcript
  line and submits no prompt. Rejected: submitting a resume prompt (an idle
  overnight session would spend a turn; the lever's no-`continue` success
  path already submits nothing). (user, 2026-10-30)
- **Failure is bounded.** A failed attempt writes one transcript notice per
  failure streak. After 3 consecutive failed attempts the watchdog stops
  attempting until the next successful server resolution (a lever reconnect
  or a new session start). Rejected: retrying forever (a broken launcher or
  a headless session, where `/mcp reconnect` is unavailable, would retry and
  log all night). (user, 2026-10-30)
- **One reconnect at a time.** A tick does nothing while a main-thread turn
  runs or while a lever request or lever step is pending. Rejected: queuing
  a reconnect behind them (two reconnects could race; an in-turn drop is the
  lever's case). (user, 2026-10-30)
- **Cold-start fallback from the plugin's own files.** When the server never
  resolved, the mod derives its name as `plugin:<$.plugin.name>:<key>`, where
  `<key>` is the `mcpServers` key in `$.plugin.root/.claude-plugin/plugin.json`
  (`ws` for ws, `wsflow` for wsflow today), and a watchdog knob that could not
  be read takes its value from `$.plugin.root/config-manifest.json`'s
  `default` for that key. The derived name also serves the lever, so a lever
  call in such a session reconnects instead of failing with "its server name
  was never resolved in this session". This is a deliberate exception, for
  the watchdog knobs only, to the mod's rule that a failed knob read leaves
  its feature inert (`261029-feat-claude-mod-package-parity-knob-defaults`
  D8); D8 rejected a package-default fallback because it needed per-package
  code defaults, and reading the package's own manifest file needs none.
  Update that rule's comment in `register.ts`. Rejected:
  hard-coding `plugin:<name>:<name>` (the lever ticket rejected hard-coded
  server names; the manifest key is the source of truth). (user, 2026-10-30)
- **Mailbox push recovery uses the existing watcher.** `watcher.ts` already
  backs off and re-arms on a `mailbox wait` error and retries a failed drain;
  this ticket pins that a killed wait re-arms and that a drain blocked by the
  dropped server resumes after the watchdog reconnects, with tests. Rejected:
  a new re-arm mechanism (only a failing test or live check justifies new
  code). (user, 2026-10-30)

- **Implementation details settled at review** (Sage review 2026-10-30,
  technical calls under the user's delegation):
  - *Two-way guard.* One in-flight guard in `reconnect.ts` covers both
    paths: a tick does nothing while a lever step is scheduled or running,
    and a lever step that comes due while a watchdog attempt is in flight
    awaits that attempt and reports its outcome instead of issuing a second
    `/mcp reconnect`. A lever request that is pending but not scheduled (left
    by an aborted turn) does not suppress ticks; it still runs at its next
    turn end, per the lever's "always reconnect when a request is pending".
    The main-thread turn state is the one register.ts already tracks for
    the lever (`turn.start` / `turn.complete`). Rejected: suppressing ticks
    on any pending request (an aborted turn would silence the watchdog all
    night).
  - *Derived name stays local.* The cold-start derived name feeds only the
    reconnect `server()` edge; the shared `server` cache (`useServer`) is
    still set only by `resolveServer`'s connected branch, after a successful
    reconnect. Rejected: writing the derived name into the shared cache
    (`readSetting`, the band and `recvTool` would target a server that never
    connected, and the manual retry hook would treat it as a resolution).
  - *Interval knob validation.* A read interval that is not a positive
    integer is treated as unreadable and takes the manifest default.
    Rejected: leaving the watchdog inert on a bad value (the knob exists to
    tune the period, not to switch the feature).
  - *First tick.* The first tick fires one interval after session start; a
    failed attempt then counts toward the streak like any other. Rejected: a
    special first-tick delay (a 10-minute interval already exceeds the
    30-second MCP startup timeout).

## Constraints

- Out of scope: removing `cmd.exe` from Claude Code's launch path (host-owned);
  a streamable HTTP transport for ws-mcp (`ws-mcp serve` supports only
  `--stdio`); Claude Code's 15-minute MCP failure cache; fixing orphaned
  `python3`/`ws-mcp` processes left after a `cmd.exe` kill (observe and
  report only). (user, 2026-10-30)
- The Windows `cmd.exe`-kill check is a post-merge dogfood on the Windows
  host, not a merge gate, following the lever ticket's precedent. (user,
  2026-10-30)
- Stop condition: if the mod cannot read files under `$.plugin.root`, or the
  host's plugin server name does not follow `plugin:<plugin>:<key>`, stop and
  report to the lead before choosing another fallback.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (transcript lines
  and knob descriptions ship downstream; no host- or repo-specific names),
  `ai-docs/manuals/wsflow-mirroring.md` (the wsflow `claude-mod/` copy stays
  byte-identical; only manifest defaults differ).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261018-feat-claude-mod-mcp-reconnect-lever (2026-10-18, Decisions): "A model-invoked lever, not a watchdog. The single Claude mod registers a tool the model calls when the ws MCP server is gone; the mod reconnects at the next main-thread turn end" — bearing: constrains
- 261018-feat-claude-mod-mcp-reconnect-lever (2026-10-18, Decisions): "Server name resolved at runtime, never spelled. The server argument is the `server` that `$.mcp.connect($.plugin.name)` answers, resolved while the server is up and cached" — bearing: constrains
- e02ee55a (2026-10-18, commit): "Probe (macOS, Claude Code 2.1.302): $.mcp.connect never restarts a dead server; $.command.run('/mcp reconnect') from turn.complete or a timer does, including against a copy of the real ws plugin." — bearing: supports
- 261029-feat-claude-mod-package-parity-knob-defaults (2026-10-29, Decisions): "D8. A failed knob read selects the inert state. ... Rejected: falling back to the package default - the mod would need per-package code defaults (D1)." — bearing: constrains
- 261029-feat-claude-mod-package-parity-knob-defaults (2026-10-29, Decisions): "D7. New knobs are read keyless once at session start ... from the project, global, and built-in scopes only; a session- or repo-scope tune is not read." — bearing: supports
- 3732c199 (2026-10-29, commit): "Retry hook: resolveServer's successful branch, the one place every later server resolution passes ... Rejected: a timer or a turn-end poll - it would re-read with no evidence the server came back" — bearing: constrains
- 261020-chore-retire-mcp-server-repair-skill (2026-10-20, Decisions): "wsflow mirror. Every `agents-plugin/hooks/claude-mod/` edit is mirrored into the byte-identical copy under `agents-plugin-wsflow/hooks/claude-mod/`" — bearing: constrains
- adbf5ec3 (2026-07-25, commit): "Reconnect steps written functionally ... rather than hardcoding plugin:ws-plugin:ws: that literal identifier does not survive ws:/ws//ws-cli substitution and would leave a wrong string in the wsflow mirror." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/register.ts, reconnect.ts, watcher.ts and their tests, the byte-identical agents-plugin-wsflow/hooks/claude-mod/ copy, agents-plugin/config-manifest.json, agents-plugin-wsflow/config-manifest.json |
| scope.surface | public-interface | two new shipped config keys and downstream-visible transcript lines; the lever's cold-start behavior changes |
| scope.new_public_symbol | yes | claude.mcp_watchdog_enabled, claude.mcp_watchdog_interval_minutes; absent from agents-plugin/config-manifest.json today |
| scope.new_type_contract | yes | boolean and positive-integer manifest entries for the two knobs; a watchdog edge set beside the reconnect edges in register.ts#L656-L669 |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/reconnect-engine.test.ts, watcher.test.ts, agents-plugin-tool/internal/mcp/config_manifest_parity_test.go; a new watchdog test file is possible |
| complexity.reuse_points | confirmed | reconnect step and toolsListed edge in agents-plugin/hooks/claude-mod/register.ts#L656-L669; watcher backoff in agents-plugin/hooks/claude-mod/watcher.ts#L267-L275 |
| complexity.side_effect_risk | moderate | an unattended timer runs /mcp reconnect and a successful resolution re-triggers the manual knob re-read in resolveServer |
| risk.correctness | high | timer must not race the lever request or step or a running turn, and the failure streak and cold-start name derivation are stateful |
| risk.fit | moderate | deliberate exception to the D8 failed-read-is-inert rule and the never-spelled server-name rule, inside a byte-identical mirrored mod |
| risk.test | moderate | engine tests need clock and tool-list fakes for streak and concurrency cases; the live probe needs an isolated tmux session |
| risk.security_or_contract | moderate | new shipped knobs with per-package defaults and the mod reading plugin.json and config-manifest.json under $.plugin.root |

## Phases

### Phase 1: Watchdog, cold-start name fallback, and mailbox recovery pinning

Implement the Decisions above in the single Claude mod
(`agents-plugin/hooks/claude-mod/`, mirrored to `agents-plugin-wsflow/`) and
declare the two knobs in both packages' config manifests.

Verification (merge gate):

- Engine tests, in the style of `reconnect-engine.test.ts`, covering: a tick
  with the tools listed does nothing; a tick with the tools absent runs one
  reconnect and writes one transcript line and no prompt; ticks during a
  main-thread turn or a pending lever request do nothing; 3 consecutive
  failures stop the watchdog, one notice per streak, and a later successful
  resolution restarts it; the knob off registers no timer; the interval knob
  sets the period; an unresolved server at start uses the derived name for
  both the watchdog and the lever; an unread knob falls back to the manifest
  default (on for ws, off for wsflow).
- Watcher tests: a `mailbox wait` that exits abnormally re-arms after
  backoff; a drain that fails while the server is down retries and delivers
  once the server is back.
- Live interactive probe on macOS or Linux, using an isolated tmux socket:
  kill the `ws-mcp` process of an idle session; within one interval the
  watchdog restores it with no model call (new PID, tools listed), and a
  mailbox message sent after the restore is pushed into the session. The
  probe may lower `claude.mcp_watchdog_interval_minutes` through a project-
  or global-scope tune (a session- or repo-scope tune is not read, D7).
- Existing mirror and manifest drift tests pass for both packages.

Post-merge dogfood (Windows host, not a gate): kill the `cmd.exe` parent of
an idle session's ws MCP server and of its `mailbox wait`; record whether the
tools leave the list, whether the watchdog restores the server and mail push,
whether orphaned `python3`/`ws-mcp` processes remain, and whether reconnect
attempts made inside the nightly kill window fail (evidence for revisiting
the attempt-count failure bound).
