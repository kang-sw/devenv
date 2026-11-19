---
title: "Pi 방식 새 세션 compaction: 자체 형식 요약, 동일 ws 키 새 하네스 세션, 세그먼트 체인"
related:
  261117-feat-ws-hosted-compaction-lifecycle: "split out of it by the owner redirect of 2026-11-19 15:18; native ws/compact (Phase 1 there) is the compaction method until this is revived"
  261117-feat-ws-harness-capability-catalog: "its compaction item grade and Decision 6 name the ws-substitute"
  261117-feat-ws-single-session-holder: "reattach after daemon restart would target the newest harness-session segment"
  261114-epic-ws-common-session-foundation: "parent epic of the compaction lifecycle ticket"
---

# New-session compaction and segment chain

## Background

Idea only; nothing here is a confirmed decision. On 2026-11-19 15:18 the owner
reversed the earlier default of replacing a session on compaction: native
harness compaction is the method (its lever is `ws/compact` in
`261117-feat-ws-hosted-compaction-lifecycle`), and the Pi-style method was moved
out to this idea ticket.

The Pi-style method: the session writes a summary in our format, the daemon opens
a NEW harness session under the same ws key, injects the summary as its first
prompt, and one ws session owns a chain of harness-session segments.

## Prior decisions (background, superseded as defaults, not confirmed here)

These were owner-acknowledged on 2026-11-17 in the compaction lifecycle ticket
and are kept as the starting point if this is revived:

- (31)=a: summarize in our format, open a new harness session, inherit the ws
  session key, inject the summary as the first prompt; applied to every harness.
  Reversed as the default by the 2026-11-19 redirect.
- (34): run only at an idle turn boundary; in-flight harness tool calls and
  harness-side background shells die with the old session (daemon-owned ws/exec
  jobs survive). Still holds for native `ws/compact`.
- (35): one ws session owns a chain of harness-session segments; resume and
  holder reattach target the newest segment; the dashboard stitches transcripts.
  Rejected then: a new ws session per compaction, because agenda, whiteboard and
  child lineage are keyed by the ws key.
- (36): when the session cannot write the summary (context overflow), fall back
  to native compaction where the catalog grades it native, otherwise a cheap
  headless summarizer session over the transcript. Rejected: a daemon-side model
  API call (no model credentials).
- (37): summary format is the Pi lever prose sections plus daemon-assembled fixed
  sections (`agents-plugin-pi/src/lead-compaction.ts`, `LEAD_PROSE_SECTIONS`).

## Open questions

- Compatibility tier: the segment chain changes the session record shape (one ws
  key maps to an ordered list of segments). Settle with source-backed old/new
  writer and reopen traces; T3 needs owner approval, T4 never ships
  (`ai-docs/manuals/release-compatibility.md`).
- Whether native compaction quality is sufficient to make this unnecessary.
- Verification sketch when revived: fixture-driven per harness (Claude first):
  new segment created, ws key inherited, agenda/whiteboard/lineage intact,
  children reachable, resume after daemon restart lands on the newest segment,
  overflow fallback.
