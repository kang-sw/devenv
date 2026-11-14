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
- Selection/storage/API mechanisms remain unchosen; (692) confirms the
  observable projection, not a new field, command or store format.

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
summaries on the board page, query and session-state topic list, and retain
the T0 boundary. Merge serially after B. Product verification is deferred to
that execution; this lane performs ticket guardrails only.
