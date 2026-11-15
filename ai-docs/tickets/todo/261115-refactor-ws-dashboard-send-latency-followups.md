---
title: "Dashboard send latency: post-ship snapshot fan-out and model-evidence retention"
related:
  261115-feat-ws-dashboard-board-topic-transport: "Phase 2 ships only changed rail rows but still renders the rail on every wake; item E here narrows what triggers that render"
  261115-refactor-ws-dashboard-wake-reads: "done; its W2 per-file evidence check is what fixes A/B relaxed"
---

# Dashboard send latency: post-ship snapshot fan-out and model-evidence retention

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

The board deferred the rest: "오래된 파일 정리와 화면을 덜 다시 그리기는 배포
뒤에 합니다." The investigation ranked these remaining items:

- **C.** Only the session page needs the evidence overlay. Index, rail,
  `pushClaude` and `checkAnswers` all take the overlaid snapshot. Moving
  them to the overlay-free read first needs a check of which views show
  model history.
- **D.** Rebuild evidence incrementally instead of calling `SeedObserved`
  over every record when a new file appears.
- **E.** The rail has nil deps, so it re-renders on every wake, and an inbox
  wake forces a full session render instead of a live one
  (`pages.go:969`). Both triggers could be narrowed.
- **F. model-evidence retention.** The board planned pruning before ship
  (e4926), but the implementing agent stopped before writing any code.

## Decisions

- Retention keep-set: per agent, after cleaning and dedup, keep the first
  record, every model-change point and the last record. MAIN settled this on
  the board (e4926). The investigation showed statically that this keeps the
  model summary (Current + History) identical, and that old binaries agree:
  `summarizeObserved` is unchanged since `4dc1e7c35`, and `ReadObserved`
  tolerates missing files. A dry run on the owner's machine kept 209 of
  11,541 files.
- Why retention stopped: a record that arrives late, through the child-to-
  parent replay or a second writer racing the prune, can carry a timestamp
  inside a span that was already pruned. That shifts a History change time.
  Example: Y@t1, Y@t3, Y@t4 prunes to Y@t1, Y@t4. Then X@t2 arrives. Unpruned
  shows "Y, X@t2, Y@t3"; pruned shows "Y, X@t2, Y@t4". After a parent
  restarts, the child replay also re-adds pruned records (extra writes only).

## Open Decisions

- F mitigation. The investigator proposed it; nobody has accepted it. Prune
  opportunistically in `AppendObserved` when the count exceeds about 2256.
  Delete only records whose time and mtime are more than about 1 h older than
  that agent's newest record. Tolerate ENOENT and skip locked files. Accept
  it, change it, or accept the small History drift instead?
- F compatibility tier. Retention changes the "never pruned" stored-data
  contract. Before this ticket is promoted, state the tier with source lines
  for which version writes and reads the directory.
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

### Phase 2: model-evidence retention (F)

Start only after the F open decisions are settled.

Acceptance:

- The model summary output is byte-identical before and after pruning on
  fixtures, including the late-arrival case under the chosen mitigation.
- Pruning never runs on a render path.
- The ticket's `## Decisions` records the compatibility tier with
  write/read evidence for the old and new versions.
