---
title: "화이트보드 커스텀 커맨드 슬롯: 제목·설명·실행 버튼으로 등록한 명령을 사용자가 눌러 실행"
related:
  261117-feat-ws-exec-tools: likely prerequisite (daemon-owned command execution this would sit on)
  261117-feat-ws-exec-mux-terminal: neighbouring exec surface (terminal sessions)
  261118-feat-ws-exec-intent-filter: neighbouring exec safety layer
---

# Whiteboard custom command slots

## Background

From the owner's idea memo (2026-11-21 triage, board topic
`idea-board-later`): register commands on the whiteboard as slots with a
title, a description, and a run button the user clicks. Ideas in the memo:
- show when a slot was registered;
- browse for a path with the system file explorer;
- record each run's stdout to a log;
- copy the log path so it can be handed to an agent.

## Decisions

- Still an idea; recorded only, nothing dispatched (owner, board (1487)
  note, 2026-11-21).

## Constraints

- Permission model (owner's reading, (1487) note): the dashboard already
  exposes a terminal, so a user-clicked command slot is close to exposing
  `ws/exec` itself; it adds no new class of capability, but it must reuse
  that execution path rather than invent a parallel one.
- Sequencing (owner, tentative "may need to"): this likely follows the
  `ws/exec` work (`261117-feat-ws-exec-tools`); re-check that ticket's
  status and surface before promoting this one.
- Open questions for promotion: who may register a slot (user only, or
  agents proposing slots the user confirms); where slots live (per board,
  per project) and their storage type per `ai-docs/manuals/performance.md`;
  how run output and its log path are retained and surfaced; Windows path
  and shell behaviour.

## Phases

### Phase 1: Settle the model

Settle the open questions above against the landed `ws/exec` surface, then
plan the slots as a `todo/` ticket.
