---
title: "Dashboard agent host: peer lead branch, and subagent model, effort, context tokens, and output rate in the agents panel"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-dashboard-session-rail: "sibling multi-session UX child; independent"
  261110-feat-ws-hosted-session-mode-flag: "turns the dashboard feed off for daemon-hosted sessions"
  261110-feat-ws-dashboard-headless-lead-sessions: "supplies these fields for daemon-hosted sessions from the stream"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 205b275210c3f035
sage-review-design-reviewed: 205b275210c3f035
completed: 2026-11-10
---

# Agents panel telemetry

## Background

Track A of the epic, for the sessions that exist today: with many sessions
and subagents running, the agents panel should say where a peer lead works
and what each subagent is and how fast it runs (2026-11-10 whiteboard,
topic "멀티 세션 관리 UX").

What the panel has today (Explore, 2026-11-10):

- **Peer lead rows** (`peerrow`, `assets/dashboard.tmpl`; `PeerRow`,
  `pageview.go:160-205`; built by `peerGroup`, `pageview.go:539-625`)
  already show the folder as a `main(folder)/` prefix (`Folder()`,
  `pageview.go:631`; `peerFolderLabel`) with the full path as a tooltip. No
  branch: `PeerRow` and `Registration` carry none. A served peer's snapshot
  has it (`Snapshot.Repo.Branch`, `internal/wslive/snapshot.go:140-142`,
  `:194-208`), and `peerGroup` already looks that snapshot up
  (`pageview.go:585`).
- **Subagent rows** (template `agent`; `AgentNode`, `pageview.go:114-129`;
  `wslive.Agent`, `internal/wslive/snapshot.go:250-259`) show label, reply
  ID, phase, caption, done and failed counts, and duration. No model or
  reasoning effort exists anywhere in the snapshot or the adapter event
  (`wslive.Event`, `internal/wslive/state.go:63-96`). Per-batch token usage
  arrives on the wire (`usage` in `batchIn`, `internal/wslive/state.go`) and
  the Claude mod fills it (`stepTokens`, `agents-plugin/hooks/claude-mod/dashboard.ts`),
  but `BatchView.Usage` is `json:"-"` (`internal/wslive/snapshot.go:295`):
  only the lead's tool summary reads it. Nothing computes a rate.
- **Hover**: the panel uses native `title` attributes only. Custom tooltips
  exist elsewhere: the git graph's `gltip` (`assets/dashboard.js:2545-2566`).
- **Pi reference**: the Pi subagent gutter shows `contextTokens` and
  `outputTps` (`agents-plugin-pi/src/agent-display.ts:16-18`, `:43-44`). Its
  rate comes from `createOutputRateTracker` (`agents-plugin-pi/src/output-rate.ts`):
  only text and tool-call block spans count, so request latency, gaps
  between blocks, and tool execution never enter the denominator; reasoning
  tokens and thinking spans are excluded when the provider reports a
  reasoning breakdown; token-weighted over the last 8 messages.

## Decisions

1. **Peer lead rows show the branch (user's request; technical, (31) ok).**
   A served peer's `Snapshot.Repo.Branch` is copied into `PeerRow` and shown
   as a chip after the name; a peer the dashboard does not serve shows the
   folder only. A detached or non-repository root renders the way the
   session page's own repo badge renders it. The folder is already shown. Rejected: reading an unserved
   peer's branch with git from the daemon, which would widen the current
   rule that the daemon reads git only for roots with an open stream
   (`gitbadge.go`).
2. **Subagent rows show model and effort, context tokens, and output rate
   (user's request).**
   - Model and effort come from the adapters: the Claude mod (mirrored into
     wsflow) and the Pi dashboard feed send them for each subagent, and
     `wslive.Agent` gains the fields (technical, (33) ok).
   - "Tokens" means the subagent's current context size, as the Pi gutter's
     `contextTokens` (user, (32) a and note; announced, (38) ok). Rejected:
     cumulative input plus output ((32) b); cumulative output ((32) c).
   - The rate is output tokens per second as the Pi gutter defines it
     (announced, (38) ok); it supersedes the simpler "current step output
     divided by elapsed time" first proposed in (33). Not shown for ended
     subagents.
   - The Pi side sends the values its gutter already computes. The Claude
     side matches the Pi definition as closely as its hook events allow.
   - On the Claude side the mod computes both values (technical, sage
     review): context tokens are the latest request's input plus cache-read
     and cache-write tokens (`260912-bug-ws-pi-subagent-context-meter-omits-cached-input`);
     the rate follows the Pi tracker's branch for usage without a reasoning
     breakdown (every block span and every output token count), with block
     spans estimated from the first and last chunk arrival per block index
     (`stepChunk`, `dashboard.ts`). Model and effort come from the
     `agent.spawn` hook (`register.ts`) and the lead's Agent call, effort from
     the `ws:effort-*` subagent type (`260928-feat-claude-effort-carrier-agents`).
   - A value the hooks cannot supply is left out of the row, not shown as a
     placeholder.
   - The values travel as new `wslive.Agent` and `wslive.Event` fields the
     adapters fill; `BatchView.Usage` stays `json:"-"`, as 9de28cbc decided
     (technical, (116) ok). Rejected: exposing per-batch usage in the
     snapshot; the rate needs block spans `BatchView` does not hold.
3. **Narrow panels move the stats into a hover card (technical, (34) ok).** A
   container query on the panel's own width hides the stats from the row,
   and a custom hover card, following the `gltip` pattern, shows them. The
   threshold is the panel's width, not the viewport's, so the three-column
   layout of `261110-feat-ws-dashboard-session-rail` narrows it correctly.
   The worker picks the threshold and records it in the Result.
   Rejected: native `title` tooltips (slow to appear, no line structure).

## Constraints

- Independent of the session holder, daemon lead sessions, and the terminal
  ((36) ok); it can land now.
- Scope is CLI-created sessions, through the adapters. Daemon-hosted
  sessions run with the dashboard feed off
  (`261110-feat-ws-hosted-session-mode-flag` Decision 1), so
  `261110-feat-ws-dashboard-headless-lead-sessions` supplies the same fields
  for them from the stream-json output ((41) ok).
- Verify first whether Claude Code hook events expose a subagent's actual
  model and reasoning effort. If they do not, use the spawn input's model
  parameter, then the agent definition's default ((33) ok). Likewise verify
  which Claude events can bound text and tool-call spans for the rate, and
  record in the Result how close the Claude rate comes to the Pi definition.
- The snapshot's `BatchView` shape is unchanged (Decision 2), and the
  lead's tool summary (`internal/wslive/toolsummary.go`) keeps working
  unchanged.
- The Claude mod is mirrored byte-identically into `agents-plugin-wsflow`
  (`internal/wsrsrc/wsflow_hooks_mirror_test.go`).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 9de28cbc (2026-11-07, commit): "The snapshot's BatchView keeps Calls/Usage as json:"-" so the live snapshot shape stays the same. Only the tool summary (Lead.ToolGap and the ring) exposes them." — bearing: constrains
- 9de28cbc (2026-11-07, commit): "Pi: only the dashboard feed's lead buffer retains and sends the new fields (createBatchBuffer({retainUndelivered}), dashboardBatchViews). Widget and relayed child views are unchanged." — bearing: constrains
- 12831114 (2026-11-10, commit): "Detection reads `<Path>/.git` only, never runs git ... Resolved in peerGroup (where rows are built) ... the template's Folder() stays a pure formatter over the new PeerRow.WorktreeMain field." — bearing: constrains
- 261002-feat-pi-model-output-tps-display (2026-10-02, Decisions): "D1 - Metric: visible-output decode rate. tokens = usage.output - usage.reasoning; time = sum of the message's text_* and toolcall_* block spans." — bearing: supports
- 261004-feat-pi-descendant-gutter-detail-parity (2026-10-04, Result): "model/effort, context, own cost, and the direct parent's existing output-rate value come from the owning record" — bearing: supports
- 260928-feat-claude-effort-carrier-agents (2026-09-28, Decisions): "Effort rides on the subagent type; the model rides on the call. One plugin agent per Claude effort level carries effort in frontmatter; the spawn passes the tier's model through the Agent tool's model parameter." — bearing: constrains
- 260912-bug-ws-pi-subagent-context-meter-omits-cached-input (2026-09-13, commit c103df38): "The prior input-only meter omitted cacheRead/cacheWrite tokens and could under-report live context by tens of thousands of tokens." — bearing: constrains
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Decisions): "Peer lead group. Each row shows the peer's lead phase line from the peer's own snapshot, the last exchange time, a /s/<reply-id> link, and the peer's subagent count." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/pageview.go, assets/dashboard.tmpl, assets/dashboard.js, dashboard.css, internal/wslive/snapshot.go, internal/wslive/state.go, agents-plugin/hooks/claude-mod/dashboard.ts plus wsflow mirror, agents-plugin-pi/src/dashboard-feed.ts |
| scope.surface | cross-module | wslive.Agent, wslive.Event, wsdashboard.PeerRow, Claude mod and Pi feed wire payloads |
| scope.new_public_symbol | yes | new exported fields on wslive.Agent and wslive.Event for model, effort, context tokens, rate; PeerRow branch field |
| scope.new_type_contract | yes | adapter-to-daemon intake event fields and snapshot JSON for agents |
| scope.test_surface | existing | wsdashboard/pageview_test.go, wslive/state_test.go, wslive/toolsummary_test.go, claude-mod dashboard.test.ts, agents-plugin-pi/test/dashboard-feed.test.ts, output-rate.test.ts, wsrsrc/wsflow_hooks_mirror_test.go |
| complexity.reuse_points | confirmed | Snapshot.Repo.Branch, peerGroup snapshot lookup, Pi createOutputRateTracker and agentDisplayDetail, gltip hover pattern all read in tree |
| complexity.side_effect_risk | moderate | new wslive.Agent and wslive.Event fields extend the live snapshot and intake wire; BatchView and the tool summary stay unchanged |
| risk.correctness | moderate | Claude rate must approximate Pi span semantics from hook events whose span coverage is not yet verified |
| risk.fit | moderate | spans Go daemon, two TS adapters, wsflow mirror, and a container-query layout shared with the session rail |
| risk.test | moderate | browser check for container-query hover card and Claude hook-event fidelity are not covered by unit tests |
| risk.security_or_contract | moderate | extends the adapter intake wire contract and snapshot JSON consumed across packages |

## Phases

### Phase 1: peer branch and subagent telemetry

Implement Decisions 1-3.

Verification:

- Go tests: `PeerRow` carries the branch for a served peer and none for an
  unserved one; the snapshot round-trips the new agent fields and `BatchView` JSON is unchanged; a missing value leaves its stat out of the row; a detached or non-repository peer root renders as the session page's repo badge does;
  the lead's tool summary output is unchanged.
- Adapter tests: the Claude mod and the Pi feed emit model, effort, context
  tokens, and rate for a subagent; the Pi values equal what its gutter shows
  for the same events; the wsflow mirror test passes.
- A browser check: stats inline on a panel wider than the threshold, in the
  hover card on a narrower one; the rate disappears when a subagent ends
  while model, effort, and context tokens stay.

### Result (50a0ffd2e) - 2026-11-10

Landed in 16b43fae8 (wslive), 953a29425 (Pi feed), d4d42f4eb (dashboard
UI), c2fd2c998 (Claude mod + wsflow mirror), 7e103e246 (hooks.json feed
description, both packages), 50a0ffd2e (hover-card CSS fix).

- **Wire and state.** `wslive.Event` gains `model`, `effort`,
  `contextTokens`, `outputTps` (numbers as `*float64`); `wslive.Agent`
  carries them on the snapshot (omitempty). A present field replaces the
  held value, an absent, blank, or out-of-range one keeps it, and the lead
  refuses them. The state clears the rate when a subagent leaves running,
  so an ended row keeps model, effort, and context but no rate.
  `BatchView.Usage` stays `json:"-"`, pinned by a JSON-equality test.
- **Peer branch.** `peerGroup` copies `Snapshot.Repo.Branch`/`Detached`
  for any served peer, adapter or not; the chip draws through the session
  page's `branchHandle` (goal/impl marks and last segment; a detached HEAD
  by its commit under the detached title; nothing for a non-repository
  root or an unserved peer).
- **Claude source.** Model and effort come from each subagent request's
  `turn.step` input (the actual per-request values); the spawn seeds them
  only while still empty (the spawn result's model, effort from an
  `effort-<level>` agent type). Context = latest usage input + cache read +
  cache write.
- **Threshold.** 560px of the agents panel's own width (`.r-tree` is the
  `atree` inline-size container); below it the stats hide and a hover card
  built like the git-graph tooltip shows one line per stat from the stats'
  own titles, rebuilt across tree swaps by the row's data-key.
- **Claude rate vs. Pi.** The Claude mod follows the Pi tracker's
  no-reasoning-breakdown branch (Claude reports none): per request, all
  output tokens over the summed span of every content block, excluding
  pre-first-block latency and inter-block gaps; skip null usage, span under
  250ms, or under 8 tokens; token-weighted over the last 8 eligible
  requests; window reset on a model change. The one difference is span
  measurement: Claude Code's hooks give per-index chunks but no block
  start/end, so a block spans its first to last chunk arrival. That omits
  the first chunk's decode time (a single-chunk block spans 0ms), so the
  Claude rate reads somewhat higher than Pi's, most for short blocks;
  requests made mostly of single-chunk blocks fall under the 250ms floor
  and are skipped rather than inflating the rate.

Verification:
- Go (remote): `go test ./internal/wsrsrc/ ./internal/mcp/
  ./internal/wsdashboard/ ./internal/wslive/ ./internal/wsi18n/` all ok
  (wsrsrc covers the wsflow hooks mirror); `go vet` on wslive,
  wsdashboard, mcp clean.
- Claude mod: `claude plugin test agents-plugin` 696 pass, 0 fail (an
  earlier run's engine-test timing flakes reproduce on unchanged HEAD);
  `claude plugin validate` passes for both packages; `diff -r` of the two
  claude-mod trees empty.
- Pi: `node --test` dashboard-feed and output-rate, 65 pass; the full
  suite's failures match a clean HEAD copy (environmental).
- Browser: headless Chromium over a fully rendered session page (running
  and ended subagent with telemetry, peer on an impl branch). At 900px the
  stats show inline (`claude-opus-5-5 high 48.2k 62 t/s`; the ended row
  `claude-sonnet-5 medium 9.0k`, no rate) and hovering shows no card; at
  440px the stats compute `display: none` and hovering the row shows the
  card with model, context, and output lines; the peer chip shows the
  goal/impl marks and `feature-y`; no page errors. The check found the
  card's lines flowing inline (selector specificity), fixed in 50a0ffd2e.
- Lite review: no Critical or Important; Minor 1 fixed (above); Minor 2
  left: `Event.Effort` is a JSON string, so an adapter sending a numeric
  effort would fail the event (both adapters send strings).

## Sage Review Round 1 (2026-11-10)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Unnamed reversal of 9de28cbc (BatchView.Usage kept out of the snapshot) | important | missing |
| 2 | Where the Claude-side context tokens and rate are computed is not stated | minor | autonomous |
| 3 | Which Pi rate rule the Claude rate should copy | minor | autonomous |
| 4 | Branch chip for a detached or non-repository peer root | minor | autonomous |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | BatchView usage exposure contradicts Decision 2 and an unreversed prior decision | important |
| 2 | Claude fallback when a value cannot be derived is unstated | minor |
| 3 | Narrow-panel threshold is unspecified | minor |
| 4 | Whiteboard item references are opaque | minor |
| 5 | Rate-disappears check covers only the rate | minor |
