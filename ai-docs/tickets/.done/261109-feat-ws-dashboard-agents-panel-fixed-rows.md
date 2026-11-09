---
title: "Dashboard agents panel: fixed tool rows per running agent, agent marks distinct from tool spinners"
related:
  261109-bug-ws-dashboard-stream-wake-rerender-cpu: in flight on the same package (pageview/region render); no ordering
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d8b802b9edeb80e6
sage-review-completeness-reviewed: d8b802b9edeb80e6
completed: 2026-11-09
---

# Dashboard agents panel: fixed tool rows per running agent, agent marks distinct from tool spinners

## Background

User report (whiteboard, 2026-11-09): the session page's agents panel grows and
shrinks on every tool call, which is distracting; the pi harness widget draws a
fixed row count. The two tool rows themselves look good. The agent's own
spinner and the tool-call spinner look the same, so a sub-agent cannot be told
apart from a tool call.

Causes (read in `agents-plugin-tool/internal/wsdashboard/`):

- `assets/dashboard.tmpl` `{{define "liverows"}}` emits a `li.brow.live` only
  for each of `Permission`, `Running`, and `Generating` with `Kind == "tool"`
  that is non-nil, so a running agent shows 0 to 3 rows as the snapshot moves
  between calls (0 while thinking or replying). Commit f594e7981 chose to "drop
  the live row when it only repeats the phase pill"; that is the jump.
- The agent row mark (`.amk` in `{{define "agent"}}`), the lead root and peer
  mark (`{{define "mark"}}`), the phase pill (`{{define "phase"}}`), and the tool
  row mark (`.brow .mk`) all use the same rotating `.spin` element at nearly
  the same size: 11px for the agent and lead `.amk` marks, 10px for the phase
  pill, peer, and tool-row spinners. The agent, lead, peer, and tool-row
  spinners share the primary colour; the phase pill spinner takes the pill's
  `--wb-on-ph-*` text colour (`assets/dashboard.css#L91-L105`,
  `assets/dashboard.css#L131`, `assets/dashboard.css#L145-L151`,
  `assets/dashboard.css#L180`).
- Rendering is server-side only: the `r-tree` region is re-rendered and swapped
  by `innerHTML` (`sse.go` region events, `assets/dashboard.js` region swap), and
  `pageview.go` `splitBatches` splits an agent's `wslive.BatchView` list into
  `Finished`, `Running`, and `Generating`.

## Decisions

1. **Fixed two live rows per running agent, pi-style (user (130) a).** A running
   agent's `ol.brows.live` always holds exactly 2 rows: the latest 2 entries of
   its chronological sequence of finished batches, the running call, and a
   generating tool call (argument streaming). A finished batch renders as the
   dimmed finished row (`✓`/`✗`, the existing `batch` row look); a running or
   argument-streaming call renders bright with the tool spinner, as today. A
   pending permission prompt takes the oldest of the two slots, so it and the
   newest call stay visible. A slot with nothing to show (before the agent's
   first call) is a dim placeholder row (`-` mark, no text), keeping the row
   height. Thinking and replying still produce no row of their own; the slots
   keep showing the latest entries. Order is oldest at the top, newest at the
   bottom; placeholders fill the bottom. The "newest call" a permission prompt
   sits above is the generating tool call when there is one, else the running
   call, else the latest finished batch; a permission prompt with nothing else
   to show sits on top with a placeholder below.
   Rejected: keep today's content and only pad empty slots (user (130) b: shows
   blank slots between every call); a CSS `min-height` only (user (130) c: same
   blank look, content rule unchanged).
2. **Agent marks are a state dot; only tool rows spin (user (131) a).** A running
   agent's row mark is a primary-colour dot with a slow pulse (opacity, no
   rotation), and the phase pill in the agents panel loses its spinner (text
   pill only). The rotating spinner stays only on live tool rows. Live tool
   rows hang visibly under their agent with a thin vertical guide line in the
   indent (the pi widget's `│` lane). Applies to sub-agent rows, the lead root
   row's mark, and the peer rows that use the shared `mark` template, since all
   three are agent marks. The peer row's running-subagents count (`.prun` in
   `peerrow`) is an agent mark too: it drops its spinner and keeps the agents
   icon and count in the primary colour. The `phase` template is shared with
   the session index row (`srow`), which keeps its spinner (Decision 4), so the
   spinner is dropped only for the agents-panel callers (`r-tree`, `agent`,
   `peerrow`); the scoping mechanism (a template argument, a separate template,
   or a CSS rule scoped to the tree) is the implementer's choice.
   Rejected: keep agent spinners and change the tool row mark shape (user (131)
   b); guide line and indent only (user (131) c: the identical spinners remain).
3. **Server template, CSS, and slot computation only (user (132) ok).** The
   change lives in `assets/dashboard.tmpl` (`liverows`, `agent`, `mark`,
   `phase` within the agents panel), `assets/dashboard.css`, and a slot
   computation in `pageview.go` (or beside `splitBatches`); no client JS
   change. Rejected: JS-side slot management, because the region swap rewrites
   the tree's innerHTML on every change and server rendering already fixes the
   row count.
4. **Scope of the spinner change is the agents panel (technical).** Other
   spinners keep the rotating `.spin`: the conversation sub-agent badge
   (`subbadge`, `dashboard.js` support bar), the conversation `sysline`, the
   activity spinner, and the session index rows (`srow`). Agents-panel scope
   over a global `.spin` change - those other spinners mean "working" with no
   tool/agent ambiguity, and changing them widens the visual blast radius
   beyond the report.
5. **Finished agents keep their one-line row (technical).** A finished or
   failed agent shows no live rows, as today; its batches stay in the collapsed
   `<details>`. The panel's height therefore changes only when an agent starts
   or ends, not on each tool call. Fixed slots for finished agents over this -
   they would add rows that carry no live information and lengthen the tree.
6. **The lead root row stays one line (technical).** The lead row already folds
   its live call into its summary line (`.ldet`) and has a fixed height; only
   its mark changes per Decision 2. Adding slots to the lead row over this -
   the report concerns sub-agent rows, and the lead's live call already shows
   inline. The expanded peer tree's `{{template "liverows" .Lead}}` call (in
   `peer`, on a `LeadView`, appended after that peer lead's finished batches)
   keeps today's variable live rows under its own template name, since that
   list already grows with the batch count and slots would repeat the last
   finished batch; the peer's own sub-agents render through `agent` and get
   slots computed in `agentTree` like the session's own.

## Constraints

- Follow `ai-docs/manuals/shipped-surface-boundary.md` (shipped dashboard
  assets): no repository-specific text in shipped strings.
- New user-visible strings, if any, go through the i18n tables; every table
  must hold every `en` key (`TestOtherTablesHoldEveryEnglishKey`). A
  placeholder row needs no text.
- `prefers-reduced-motion: reduce` disables the pulse as it disables `.spin`.
- No change to the pi widget, `wslive` snapshot fields, or the SSE region
  protocol.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- f594e798 (2026-11-07, commit): "Target one line per agent: mark, label (ellipsis), caption inline as small muted text, stats + elapsed on the same row; drop the live row when it only repeats the phase pill" — bearing: constrains
- 261028-feat-pi-agent-widget-expanded-mode (2026-10-28, Decisions): "Every agent gets a fixed allotment of N rows, blank rows padding an agent with fewer batches, so the widget height changes only when an agent enters or leaves the widget." — bearing: supports
- 261028-feat-pi-agent-widget-batch-row-redesign (2026-10-28, Result): "Expanded-mode batch rows now begin with a dim │ under their agent's bullet, then a mark column (✓ finished, the spinner running, a dim - placeholder in each unfilled history slot)" — bearing: supports
- 32588da2 (2026-10-28, commit): "the user designed the row format (one tool batch per row, finished rows dimmed, running row bright with spinner, in-flight call with elapsed time and i/n)" — bearing: supports
- 45f72229 (2026-11-06, commit): "The panel template's row markup became a shared row template so the fold reuses it." — bearing: supports
- 5651e314 (2026-11-08, commit): "Running uses AgentNode.IsRunning's rule (status running) over the flat snapshot agent list, so nested running agents count and finished/failed ones do not." — bearing: constrains
- 30485943 (2026-11-07, commit): "One parsed template set per language because a parsed set's FuncMap is fixed; the entry cache key is prefixed with the language" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl, assets/dashboard.css, pageview.go, pageview_test.go |
| scope.surface | internal | AgentNode in pageview.go#L115-L126 is exported but inside the internal wsdashboard package; no SSE or wslive change |
| scope.new_public_symbol | yes | a new exported slot field on AgentNode, read by the template; package is internal |
| scope.new_type_contract | yes | a slot type or slice on AgentNode carrying finished, running, generating, permission, or placeholder rows |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/pageview_test.go |
| complexity.reuse_points | confirmed | splitBatches pageview.go#L379-L395, batch and liverows templates dashboard.tmpl#L87-L89 read |
| complexity.side_effect_risk | moderate | shared mark template also drives lead root and peer rows dashboard.tmpl#L91 and #L101, and .spin selectors are shared with srow, activity, sbadge |
| risk.correctness | moderate | slot ordering with permission taking the oldest slot and placeholders filling the bottom has several cases |
| risk.fit | low | decisions name exact templates, CSS, and pageview.go slot computation; matches server-render region swap |
| risk.test | moderate | visual height stability and pulse are only checked by headless screenshots, not unit tests |
| risk.security_or_contract | low | no snapshot field, SSE protocol, or i18n key change; html/template escaping unchanged |

## Phases

### Phase 1: Fixed live rows and distinct agent marks

- Compute a running agent's two live slots from its batches (Decision 1),
  exposing them on `AgentNode` for the template; keep `Finished` for the
  collapsed details.
- Rewrite `liverows` / the `agent` template to emit exactly two rows for a
  running agent: finished rows reuse the `batch` row look, live rows keep
  today's markup (tool name, args or "writing arguments", char count, `+N`
  others, duration), permission keeps its lock row, placeholders are dim.
- Replace the agent, lead, and peer running mark with a pulsing dot; drop the
  spinner from the agents-panel phase pill; add the guide line for live rows
  (Decision 2); leave the other spinners (Decision 4).

Verification:

- `go test ./internal/wsdashboard/...` (from `agents-plugin-tool/`) passes, with
  new render tests asserting, for a running agent: exactly 2 `li` in
  `ol.brows.live` when it has (a) no batches, (b) one finished batch, (c)
  three finished batches (the latest 2 shown, oldest first), (d) a running call
  plus finished batches, (e) a generating tool call plus a running call, (f) a
  permission prompt plus a running call plus a generating call (permission and
  the newest call shown), (g) a permission prompt alone (permission on top,
  placeholder below); a finished agent renders no `ol.brows.live`; the whole
  `r-tree` render (sub-agent, lead root, and peer rows, peer phase pills,
  `.prun`) has no `.spin` outside `ol.brows.live`, while the rendered `srow`
  and `subbadge` still carry `.spin` (Decision 4); a running agent's live list
  carries the guide-line markup or class; `dashboard.css`'s
  `prefers-reduced-motion: reduce` rule names the pulse class.
- `go vet ./internal/wsdashboard/...` and gofmt clean.
- Headless Chromium screenshots of a dev daemon session page (isolated
  `WS_CACHE_HOME`/`WS_CONFIG_HOME`, never the user's daemons on ports 29417 or
  45743) showing a running sub-agent across consecutive states with the same
  panel height, in light and dark themes; screenshots go to the scratchpad,
  not the repo.

### Result (5e42cb819) - 2026-11-09

Landed:

- `pageview.go`: `AgentNode.Slots []LiveSlot` (Permission / Finished /
  Running / Generating, none set = placeholder) from `liveSlots`, computed in
  `agentTree` for running agents only (`liveSlotCount = 2`). Entries are the
  finished batches in order, then the running call, then a tool-kind
  generating call; a permission prompt takes the top slot above the newest
  entry; placeholders fill the bottom.
- `dashboard.tmpl`: `liverows` split into `permrow` / `runrow` / `genrow`;
  new `slots` template renders a running agent's
  `<ol class="brows live bguide">` (finished slots reuse `batch`, placeholder
  `<li class="brow ph"><span class="mk">-</span></li>`). The expanded peer
  tree keeps `liverows` (Decision 6). New `tphase` (phase pill without
  spinner) is used by the lead root row, `agent`, and `peerrow`; `phase`
  is unchanged for `srow`. `mark` and the agent `.amk` emit `.pulse`; `.prun`
  dropped its spinner.
- `dashboard.css`: `.pulse` (opacity pulse, added to the reduced-motion
  rule), guide line on `.agent > .brows.bguide`, a 20px fixed row height for
  its rows, dim placeholder mark, `.tree summary.ahead { align-items: center }`;
  removed the now-unused `.ahead .phase .spin` / `.peer .spin` sizes.

Verification:

- `go test ./internal/wsdashboard/...` ok, with new
  `TestRunningAgentHasTwoFixedLiveSlots` (cases a-g plus thinking-only,
  finished-then-thinking, permission+finished; finished/failed agents have no
  live list) and `TestAgentsPanelSpinsOnlyOnToolRows` (no `spin` in `r-tree`
  outside `li.brow.live` rows across lead, same-worktree and compact peers,
  nested agents, `.prun`; `srow` and `subbadge` keep `.spin`; reduced-motion
  names `.pulse`; guide class asserted). Existing tests that asserted tree
  spinners and the empty thinking-agent live list were updated to the new
  behavior. `go test ./cmd/ws-mcp/... ./internal/wsagent/...` ok.
- `go vet ./internal/wsdashboard/...` clean; gofmt clean.
- Headless Chromium (Playwright headless shell) screenshots of the full
  session page in light and dark themes across five consecutive sub-agent
  states (thinking before any call, running, thinking after two calls,
  running + generating, permission + running): measured `.tree` height
  137.09px in all ten renders; screenshots in the session scratchpad.
- Lite review: clean.

Decisions:

- The page was rendered with `renderSessionRegions`/`renderSessionPage` from a
  throwaway test into static HTML, not a live dev daemon: same templates and
  CSS, no daemon or user ports touched.
- Guide class is `bguide`: the whiteboard's `.lane` rule reaches the dashboard
  and painted a card behind the rows.
- `.tree summary.ahead { align-items: center }` added: the whiteboard's
  `.tree summary` baseline rule made an agent row about 1.2px taller under
  the permission pill's lock icon, breaking the fixed height.
- The spinner assertion is "none outside `li.brow.live`" rather than
  "outside `ol.brows.live`", because the expanded peer tree's live tool
  rows sit in a plain `ol.brows` per Decision 6.
