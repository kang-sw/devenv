---
title: "Topic summary: include the latest panel recap"
related:
  261113-feat-ws-whiteboard-topic-panels-and-log: "existing panel/log model and agent-only summary; completed contract, not edited here"
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "existing head explicitly excludes panel recaps; requested extension captured separately"
---

# Topic summary includes panel recaps

## Background

Original owner request, relayed by MAIN on 2026-11-14 at 11:34:

> 지금 "토픽 기록"에서 panel 엔트리의 recap은 상단 recap의 최신 엔트리로 등록이 안 되는데, 이것도 등록되게 해주면 좋겠어요.

English translation: panel-entry recaps in Topic History currently do not
become the latest entry in the top recap; the owner wants them included.

## Decisions

- Capture a separate feature ticket rather than rewriting completed panel/log
  or sticky-head contracts. Those tickets deliberately used the latest agent
  log line and excluded panel recaps from the topic head.
- Scheduling slot B2 is immediately **after** B (history relative time), not
  before it. This is queue order; B's exact ticket/phase reference was not
  supplied, so no fabricated `blocked-by` is added.
- **(692), confirmed:** exact owner answer `692=ok`, relayed by MAIN on
  2026-11-14 at 11:35. The topic's top summary chooses the newer of the latest
  agent log line and latest panel recap. The board page, `whiteboard.query`
  current summary and session-state topic list use the same projection.
  Compatibility tier: **T0**. Rejected: retaining the prior agent-log-only
  summary, which omits a later panel recap requested by the owner.
- MAIN authorizes implementation after B and serialized integration. This
  documentation delegate stops at capture; source edits, verification and any
  ready promotion belong to the later B2 execution path.
- **(699), MAIN technical settlement, relayed 2026-11-14 at 11:41:** when
  the latest agent log line and panel recap have equal timestamps, the larger
  durable entry ID (`eN`) wins. This reuses the board's existing entry-ID
  disambiguation for equal timestamps; it adds no new field or store format.
- Other selection/storage/API mechanisms remain unchosen; (692)/(699)
  confirm the observable projection and tie-break, not a new command.

## Prior Decisions

- `261113-feat-ws-whiteboard-topic-panels-and-log`, Decisions 2/8 and Phase 3
  Result: agent log lines supply topic summaries; panel recaps are separately
  discoverable. This request extends that behavior rather than reviving the
  retired `wb:recap` command.
- `261112-feat-ws-whiteboard-topic-tickets-sticky-head`, Implementation
  Handoff and Decision 8: the head consumes the latest agent log line, not
  panel metadata or system events. Its frozen plan/results remain unchanged.

## Phases

### Phase 1: Include panel recaps in the confirmed topic summary projection

After slot B completes, implement (692)'s confirmed projection through a
separate worker/run path. Preserve panel history/version identity and agent-log
behavior when the log is newer; no store/API mechanism is chosen here.

Future verification boundary: compare panel-only, agent-only and mixed topics.
Capture the same topic before and after posting a panel recap later than its
last agent log line, then after a newer agent log line. Assert matching
summaries on the board page, query and session-state topic list. Include
both equal-timestamp cases: panel recap with the larger `eN`, and agent log
line with the larger `eN`; each must win consistently across all three
surfaces (699). Retain the T0 boundary. Merge serially after B. Product
verification is deferred to that execution; this lane performs ticket
guardrails only.

### Result (6302d9646) - 2026-11-16

Implemented 2026-11-16 on `impl/develop/panel-recap-summary` (Phase 1).
Owner's 23:12 request and `(1221)=a` approved option A in
`ai-docs/assets/mockups/261116/panel-recap-title/mockup.html`:

- One shared display projection selects the newest agent log or panel recap;
  equal timestamps use the larger numeric `eN`, including `e9`/`e10` ties.
  Board/page/embed, query `current`, and session-state topic lists agree.
  System events cannot win; `LatestAgent` remains log-only for arrival blink.
- Panel heads use the existing 12.5px muted, four-line-clamped summary slot,
  the `P` marker with existing `wb.stream.panels` translation, panel recap
  inline text, and the exact version's `data-entry`. A later log restores `L`.
  Ctrl+Q Overview inherits the same summary and clamp. Panel history and
  payloads remain unchanged; query summary metadata excludes panel payloads.
  T0: no durable schema, store format, command, or migration change.
- Legacy-body commands already produce panels titled `Current content`;
  those now legitimately become summaries too. Unconverted legacy-recap
  fallback tests remain separate. Closed panel-only topics retain their
  summary inside the existing closed recap fold.

Verification used isolated Go 1.23.12 at
`/tmp/ws-panel-recap-go123/go/bin` (host Go is 1.26.2), with that directory
first in `PATH` and `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`:

- `go test ./internal/wswhiteboard -count=1 -v` — PASS (255.319s);
  panel-only, agent-only, mixed, timestamp recency, both numeric ties,
  immutable state, exact panel-version jump markup, legacy fallback,
  query projections/filters and panel-payload exclusion covered.
- Narrow MCP recap/session-state tests in `workflow_manual_recaps_test.go`
  (`TestRenderSessionState…` and
  `TestWorkflowStateByteIdenticalToWorkflowManualWithSummaries`) — PASS.
- `go test ./internal/wsdashboard -run '^TestCSSClassesDoNotCollideAcrossModules$' -count=1`
  — PASS; the sole new product CSS class is `wb-evi-panel`.
- `go test ./internal/wsdashboard -run '^TestPanelRecapHeadBrowser$' -count=1 -v`
  — PASS, production-rendered dashboard HTML in headless Chromium, light and
  dark: panel-only P, log → later panel P → later log L, recap jump to the
  actual panel row, Ctrl+Q summary retention and real four-line overflow clamp.
- `node internal/wswhiteboard/testdata/latest-log.browser.cjs` — PASS;
  latest-log arrival and one/four-line sticky geometry unchanged.

Browser runs used
`PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright` and
`WS_PLAYWRIGHT_MODULE=$HOME/.npm/_npx/705bc6b22212b352/node_modules/playwright`.
Required captures (`WS_PANEL_RECAP_SHOTS=/tmp/ws-panel-recap-shots`) were
visually checked against option A, outside the repository:

- `/tmp/ws-panel-recap-shots/panel-only-light.png`
- `/tmp/ws-panel-recap-shots/panel-only-dark.png`
- `/tmp/ws-panel-recap-shots/log-then-later-panel-light.png`
- `/tmp/ws-panel-recap-shots/log-then-later-panel-dark.png`
- `/tmp/ws-panel-recap-shots/overview-light.png`
- `/tmp/ws-panel-recap-shots/overview-dark.png`

No shared daemon lifecycle operation, Electron launch, merge, push, or SSH.
Integration remains with the lead; this worker leaves the ticket in `todo`.
