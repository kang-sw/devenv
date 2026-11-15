---
title: "Dashboard send latency: post-ship snapshot overlay and render fan-out"
related:
  261115-feat-ws-dashboard-board-topic-transport: "Phase 2 ships only changed rail rows but still renders the rail on every wake; item E here narrows what triggers that render"
  261115-refactor-ws-dashboard-wake-reads: "done; its W2 per-file evidence check is what fixes A/B relaxed"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e6be5428f3366229
sage-review-completeness-reviewed: e6be5428f3366229
---

# Dashboard send latency: post-ship snapshot overlay and render fan-out

## Background

Whiteboard topic `send-latency` (채팅 반영 지연). After the owner presses
send, the message took about 1.5 s to appear in the conversation, and the
delay grew over the session's life. A read-only investigation found the
cause: at investigation time, each session's `model-evidence/` directory
held one write-once file per model response and was never pruned; append-time
pruning has since landed (`7ad4a0627`; `agents-plugin-tool/internal/wslive/observed.go#L59-L62`; `agents-plugin-tool/internal/wslive/observed.go#L261-L278`). About
11,000 files were spread over 7 sessions, 3,890 of them in the lead. Every
`Snapshot()` re-stat'ed the whole directory, at about 430 snapshot reads per
second. In a 15 s CPU profile, `SnapshotCache.Snapshot` →
`ObservedCache.Load` took 45%.

Fixes A and B, plus a name-diff follow-up, went out before v0.50.0 on branch
`fix/dashboard-evidence-sweep-cache` (`882368a83`, `589de6809`):

- A: trust the directory stat between sweeps and force a full sweep every
  15 s.
- B: the send POST skips the sweep.
- Name-diff: list names and stat only the new ones.

Benchmarked on 4000 records, a steady-state Load dropped from 60-63 ms to
4.4 us. A new response still costs about 10.6 ms (the listing plus a full
`SeedObserved` rebuild).

The board deferred the rest: "화면을 덜 다시 그리기는 배포 뒤에 합니다." The
investigation ranked these remaining items:

- **C.** Session detail and the rail need the evidence overlay: detail
  renders lead/child histories, and rail hover renders the latest transition
  via `ObservedModel.Change`, which reads History
  (`agents-plugin-tool/internal/wsdashboard/pageview.go#L597-L619`;
  `agents-plugin-tool/internal/wsdashboard/rail.go#L54-L66`;
  `agents-plugin-tool/internal/wsdashboard/pageview_hover.go#L22-L34`;
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L433-L441`;
  `agents-plugin-tool/internal/wslive/observed.go#L64-L72`). A served,
  adapter-present, non-compact peer's child tree also renders observed-model
  hover through `agent`, so its snapshot lookup must retain child evidence
  (`agents-plugin-tool/internal/wsdashboard/pageview.go#L832-L855`;
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L211-L218`; `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L245-L245`; `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L435-L439`).
  Index, rail, `pushClaude` and `checkAnswers` currently take overlaid snapshots;
  index, usage collection and answer checks do not read model history
  (`agents-plugin-tool/internal/wsdashboard/pages.go#L1145-L1155`;
  `agents-plugin-tool/internal/wsdashboard/pageview.go#L1065-L1139`;
  `agents-plugin-tool/internal/wsdashboard/plan_usage.go#L72-L94`;
  `agents-plugin-tool/internal/wsdashboard/ack_check.go#L219-L237`).
- **D.** Rebuild evidence incrementally instead of calling `SeedObserved`
  over every record when a new file appears.
- **E.** The rail has nil deps, so it re-renders on every wake, and an inbox
  wake forces a full session render instead of a live one
  (`agents-plugin-tool/internal/wsdashboard/rail.go#L108-L136`;
  `agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L213-L216`;
  `agents-plugin-tool/internal/wsdashboard/live_hosted.go#L73-L75`;
  `agents-plugin-tool/internal/wsdashboard/sse.go#L172-L189`). Both triggers could be narrowed.
- **F. model-evidence retention: historical pre-ship work, (1133)=a.**
  Append-time pruning landed in `7ad4a0627`; retention and the Open Decision
  Queues of `261116-refactor-ws-model-evidence-bounded-retention` are outside
  this ticket's scope. See Decisions and the Follow-up Note.

## Decisions

- MAIN technical decision 2026-11-16: keep the evidence overlay exactly for
  views whose rendered output consumes model history; every other caller
  uses overlay-free snapshots. Source inspection identifies these consumers:
  - Session detail: lead/child model histories and agent model hovers
    (`agents-plugin-tool/internal/wsdashboard/pageview.go#L597-L619`;
    `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L211-L218`,
    `#L256-L256`).
  - Rail rows, including inert hover sources: the latest model transition
    calls `ObservedModel.Change`, which reads History
    (`agents-plugin-tool/internal/wsdashboard/pageview_hover.go#L22-L34`;
    `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L433-L439`;
    `agents-plugin-tool/internal/wslive/observed.go#L64-L71`).
  - Served, adapter-present, non-compact peers whose rendered child trees
    include agent model hovers
    (`agents-plugin-tool/internal/wsdashboard/pageview.go#L832-L855`;
    `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L245-L245`).
  Other consumers, including index rows, `pushClaude`, `checkAnswers`, command
  availability, outside-reply link extraction, compact peers and count-only
  delegate lookups, do not consume model history and stop sweeping evidence
  (`agents-plugin-tool/internal/wsdashboard/pageview.go#L402-L424`,
  `#L1065-L1139`; `agents-plugin-tool/internal/wsdashboard/plan_usage.go#L72-L94`;
  `agents-plugin-tool/internal/wsdashboard/ack_check.go#L219-L237`;
  `agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L329-L335`;
  `agents-plugin-tool/internal/wsdashboard/fs_outside.go#L346-L348`).
  Rejected: the original session-only assumption, because rail and peer-child
  hover output also reads history; blanket overlay removal would change
  visible output. Preserve existing unknown/absent evidence behavior.
- MAIN execution scope 2026-11-16: implement Phase 1 C/D/E only, T0 only,
  with unchanged rendered output. Build on landed rail row transport and
  preserve clock/liveness, layout, worktree, metadata, hover and drag behavior.
  If visible output must change, stop and report to MAIN; UI changes require
  the separate Claude/mockup-first flow.
- (1133)=a, owner: retention (F) is implemented now, before ship, on
  `fix/dashboard-evidence-sweep-cache`. The design:
  - Prune in `AppendObserved` when the count exceeds about 2256.
  - Per agent, keep the first record, every model-change point and the
    last record.
  - A 1 h age guard: delete a record only when it is more than about 1 h
    older than that agent's newest record.

## Follow-up Note: residual late-arrival risk of retention

The 1 h guard makes this unlikely but does not prove it impossible. A
record can arrive late, through the child-to-parent replay or a second
writer racing the prune, with a timestamp inside an already-pruned span.
That shifts a History change time. Example: Y@t1, Y@t3, Y@t4 prunes to
Y@t1, Y@t4. Then X@t2 arrives. Unpruned shows "Y, X@t2, Y@t3"; pruned shows
"Y, X@t2, Y@t4". In addition, after a parent restarts, the child replay can
re-add pruned records (extra writes only). Add a phase here only if the
drift is observed.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Retention, pruning and the Open Decision Queues of
  `261116-refactor-ws-model-evidence-bounded-retention` are out of scope.
- Use `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` and focused `-run` filters;
  never run the full dashboard, MCP or project Go suite. No shared-runtime
  operation, push or swap. CPU/snapshot-read measurements use an isolated
  daemon and disposable large-evidence fixtures, not the shared daemon.
- Record before/after screenshot paths and compare them when any page output
  could be affected. Existing rendered output is the baseline, not authority
  for a redesign. Cached browser environment:
  `WS_PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright`
  and `PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright`.

## Prior Decisions

- 261115-feat-ws-dashboard-board-topic-transport / af2b5b88 (2026-11-16, commit): "D4 explicitly rejects revision-keyed rail caching because clock liveness, layout and worktrees affect markup" — bearing: constrains
- 261115-refactor-ws-dashboard-wake-reads / 882368a8 (2026-11-15, commit): "adapterPresent only reads Adapter, UpdatedAt and HeartbeatMs; the overlay writes only Observed fields" — bearing: supports
- 7ad4a062 (2026-11-15, commit): "Limitation: a record arriving late (child replay into a parent, or another writer racing the prune) with an At inside a pruned span and a different model would date the change back" — bearing: constrains
- 261115-feat-ws-dashboard-observed-session-model / 4dc1e7c3 (2026-11-15, commit): "Immutable metadata sidecars survive old typed snapshots, with original producer/time deduplication." — bearing: constrains
- 1f4b3ff4 (2026-11-15, commit): "Keep last-good shared render state only for open subscribers, never after confirmed absence/mismatch; no wire event, persisted field or lock boundary changes (T0)." — bearing: constrains
- edb6f8a2 (2026-11-11, commit): "The live-only wake uses distinct broker tokens (live:<id>, ring:<id>) so the stream cache can tell a snapshot hand-off from a store change without a new wake channel" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | C/D/E span agents-plugin-tool/internal/wsdashboard/{snapshot_cache,pages,rail,sse,stream_cache,plan_usage,ack_check,inbox_routes}.go and agents-plugin-tool/internal/wslive/{observed,observed_cache}.go. |
| scope.surface | cross-module | wsdashboard.SnapshotCache consumes wslive.ObservedCache and its shared projection; rendered HTML and stream contracts must remain unchanged. |
| scope.new_public_symbol | unknown | No new exported overlay-free accessor or incremental-projection API is named; SnapshotCache.read already offers an internal overlay-free read. |
| scope.new_type_contract | unknown | Incremental projection representation/signatures are unspecified; existing ObservedEvidence is shared read-only (agents-plugin-tool/internal/wslive/observed_cache.go#L89-L101). No new disk/wire contract is authorized. |
| scope.test_surface | existing | agents-plugin-tool/internal/wslive/{observed_cache,observed}_test.go; agents-plugin-tool/internal/wsdashboard/{observed_snapshot,stream_cache,rail_rows,pageview_hover,model_history_browser}_test.go. |
| complexity.reuse_points | confirmed | SnapshotCache.read/Snapshot; ObservedCache.Load/ObservedEvidence.Overlay; summarizeObserved; pageCache.get/renderLive; broker.liveOnlySince; sessionReads and railPageSource were read. |
| complexity.side_effect_risk | high | Shared immutable evidence projections and wake-driven cached reads can stale multiple subscribers if invalidation or publication is incomplete. |
| risk.correctness | high | Rail hover reads History through Change; incremental updates must retain chronological ordering, duplicate precedence, deletions, replacement and newer-live-evidence behavior (agents-plugin-tool/internal/wslive/observed.go#L190-L245; agents-plugin-tool/internal/wslive/observed_cache.go#L180-L195). |
| risk.fit | moderate | E must preserve clock/layout/worktree invalidation and landed rail-row transport while distinguishing inbox changes from full store changes (agents-plugin-tool/internal/wsdashboard/rail.go#L108-L136; agents-plugin-tool/internal/wsdashboard/sse.go#L288-L345). |
| risk.test | high | Acceptance requires paired isolated CPU/snapshot-read measurements, a 4000-record new-response benchmark below ~10.6 ms, zero rail-invisible renders and inbox live-path/parity checks; these have not run for this ticket. |
| risk.security_or_contract | moderate | T0 only and unchanged rendered output; old typed snapshots need retained evidence overlays, while pruning, disk/wire shapes and authorization gates must stay untouched (agents-plugin-tool/internal/wslive/observed.go#L59-L62; agents-plugin-tool/internal/wslive/observed.go#L218-L245). |

## Phases

### Phase 1: Narrow snapshot overlay and render fan-out (C, D, E)

Acceptance:

- Index, rail, `pushClaude` and `checkAnswers` stop sweeping evidence unless
  a view they render needs it. Their rendered output is unchanged.
- A new evidence file updates the projection incrementally. A benchmark
  shows the per-new-response Load cost below today's ~10.6 ms at 4000
  records.
- A wake that changes no rail-visible data does not re-render the rail, and
  an inbox wake uses the live render path when the page allows it. Refresh
  the held/drained inbox reads on that path rather than reusing stale reads;
  preserve full-render fallback for mixed store changes. (1167) Auto-confirmed
  technical clarification from design review: wake classification alone loses
  fresh inbox output because `readSessionWith` reuses prior inbox reads
  (`agents-plugin-tool/internal/wsdashboard/pages.go#L1027-L1036`;
  `agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L210-L216`;
  `agents-plugin-tool/internal/wsdashboard/stream_cache.go#L176-L235`). No new
  wire or store contract is selected.
- Before/after daemon CPU and snapshot-read rate are measured on an
  isolated daemon with a large evidence fixture.
