---
title: "세션 4상태 레일: 배포 후 후속 작업"
related:
  261113-feat-ws-dashboard-session-four-state-lifecycle: "source ticket; both phases carry Results, so its known deviations are captured here"
  261114-bug-ws-delegate-reviewer-render-lacks-trusted-provenance: "earlier fix for a similar trusted-provenance refusal; check whether the child-spawn report is a regression of it"
---

# Session four-state rail: post-ship follow-ups

## Background

Whiteboard topic `session-four-state` (세션 4상태(비활성 보관)). The four-state
rail shipped in two phases: backend `012d5568a` and rail UI `e2725a2d6`
(Phase 2 Result `33d872de9`, archive-500 handling `d2cafcc29`). The board's
closing line listed deviations from the approved (1127)=ok mockup and marked
one as a post-ship follow-up:

> 목업과 다른 점: 대기 메일 칩은 없습니다(백엔드에 개수가 없음). 범례 카드는
> 넣지 않았습니다. 터미널이 세션을 가져가서 생긴 409에도 "확인 중" 문구가
> 나옵니다(배포 뒤 후속).

MAIN also listed these items as post-ship follow-ups for this topic. The
first three have source evidence. The last two are MAIN-reported, and their
detail is on neither the board nor the commits.

1. **Wrong message on a takeover 409.** The rail maps archive failures by
   HTTP status only. A 409 "not owned by the dashboard" (a terminal took the
   session over) shows the same retryable "still checking" toast as the 409
   for "holder scan pending". The `d2cafcc29` AI Context accepted this as
   rare, because the next index update removes the archive control.
2. **No queued-mail chip.** The mockup shows a queued-mail count on archived
   rows. The rail/index has no per-session queued-mail count, so adding one
   needs a backend change (`33d872de9`).
3. **Prune countdown ignores the handed-off pi exemption.** `DeleteAt`
   mirrors only some of `PruneRegistrations`' exemptions (Agent rows and
   open hosted leads). A handed-off pi session that keeps its session file
   in its directory is exempt from prune, but the rail still shows it a
   "N일 후 삭제" countdown (`33d872de9`; Phase 2 Result of the source ticket).
4. **Explore the network ceiling** (MAIN-reported; scope not recorded).
5. **Child spawn lacks trusted-render provenance** (MAIN-reported). A child
   spawn during this work was refused for missing trusted-render
   provenance. If it reproduces, it is a ws tool surprise, not a rail defect.

## Decisions

- The mockup's legend card was left out on purpose and is not a follow-up
  here unless the owner asks for it.

## Open Decisions

- Item 1: should the runner give the takeover 409 a distinguishing signal,
  such as a stable code or body field, so the UI can show a separate
  message? Or should the UI re-read the row before it shows the toast? A
  new response field is a cross-version surface, so state its compatibility
  tier.
- Item 2: where does the per-session queued-mail count come from (index,
  runner state, or mailbox), and is the chip still wanted? This needs the
  owner's confirmation, because the shipped UI already left it out.
- Items 4 and 5: MAIN to record the observed symptom and scope before this
  ticket is promoted. If either one turns out to be unrelated to the four-
  state rail, split it into its own ticket.

## Phases

### Phase 1: Correct rail lifecycle signals

Fix items 1 to 3 once their open decisions are settled.

Acceptance:

- A terminal-takeover 409 shows a message distinct from "scan pending", or
  the row updates without a misleading toast. A browser test stubs both
  409 kinds.
- When the owner keeps the chip: archived rows with queued mail show the
  count, and it disappears once the mail is delivered after unarchive.
- An exempt handed-off pi row shows no prune countdown. A rail test covers
  the exemption, and the test agrees with `PruneRegistrations` on the same
  fixture.
- Items 4 and 5 are either resolved with evidence or moved out to their own
  tickets.
