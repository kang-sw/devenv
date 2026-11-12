---
title: A dashboard message sent during a Claude mod session's compaction is submitted at once as a follow-up
related:
  261018-research-claude-mod-mid-turn-steer: the ledger behind the hold-while-running and steer delivery paths
---

# A dashboard message sent during a Claude mod session's compaction is submitted at once as a follow-up

## Report

User, 2026-11-12 (board): in a Claude CLI session running the ws mod, a
message sent while the context is being compacted always goes in as a
settled follow-up ("컨텍스트 압축 중에 메시지 보내면 항상 follow up으로
확정되어서 들어가버려요").

## Evidence (unverified reading)

- `agents-plugin/hooks/claude-mod/watcher.ts`: a dashboard message is held
  (still editable, promotable to steering) only while `edges.turnRunning()`
  is true; otherwise the board drain submits it at once
  (`if (!edges.turnRunning()) void flushBoard(...)`, ~558).
- A compaction is not a main-thread turn, so during one the watcher reads
  the session as idle, submits through `$.prompt.submit`, and Claude Code
  queues that prompt behind the compaction: the page shows it as sent and
  it can no longer be withdrawn or promoted. It also lands ahead of a
  compaction continuation (the 261018 ledger's first symptom).
- `dashboard.ts` already tracks `compacting` (compactStarted/compactEnded)
  for the intake feed, so the state is available to the mod.

## Direction (not settled)

Treat a running compaction like a running turn for dashboard messages: hold
them (cancellable, promotable) and submit once the compaction settles, after
any compaction continuation. Needs a decision before it becomes actionable.
