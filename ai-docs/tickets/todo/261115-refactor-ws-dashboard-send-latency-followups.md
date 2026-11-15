---
title: "Dashboard send latency: post-ship snapshot overlay and render fan-out"
related:
  261115-feat-ws-dashboard-board-topic-transport: "Phase 2 ships only changed rail rows but still renders the rail on every wake; item E here narrows what triggers that render"
  261115-refactor-ws-dashboard-wake-reads: "done; its W2 per-file evidence check is what fixes A/B relaxed"
---

# Dashboard send latency: post-ship snapshot overlay and render fan-out

## Background

Whiteboard topic `send-latency` (채팅 반영 지연). After the owner presses
send, the message took about 1.5 s to appear in the conversation, and the
delay grew over the session's life. A read-only investigation found the
cause: each session's `model-evidence/` directory holds one write-once file
per model response and is never pruned (`wslive/observed.go:59`). About
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

- **C.** Only the session page needs the evidence overlay. Index, rail,
  `pushClaude` and `checkAnswers` all take the overlaid snapshot. Moving
  them to the overlay-free read first needs a check of which views show
  model history.
- **D.** Rebuild evidence incrementally instead of calling `SeedObserved`
  over every record when a new file appears.
- **E.** The rail has nil deps, so it re-renders on every wake, and an inbox
  wake forces a full session render instead of a live one
  (`pages.go:969`). Both triggers could be narrowed.
- **F. model-evidence retention: in progress pre-ship, (1133)=a.** This is
  not in this ticket's scope. See Decisions and the Follow-up Note.

## Decisions

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

## Open Decisions

- C scope: which views, if any, outside the session page show model history
  and must keep the overlay?

## Phases

### Phase 1: Narrow snapshot overlay and render fan-out (C, D, E)

Acceptance:

- Index, rail, `pushClaude` and `checkAnswers` stop sweeping evidence unless
  a view they render needs it. Their rendered output is unchanged.
- A new evidence file updates the projection incrementally. A benchmark
  shows the per-new-response Load cost below today's ~10.6 ms at 4000
  records.
- A wake that changes no rail-visible data does not re-render the rail, and
  an inbox wake uses the live render path when the page allows it.
- Before/after daemon CPU and snapshot-read rate are measured on an
  isolated daemon with a large evidence fixture.
