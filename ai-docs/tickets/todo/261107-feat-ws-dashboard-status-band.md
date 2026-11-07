---
title: "Dashboard: session status band (context, branch, sync, usage) around the composer"
parent: 261107-epic-ws-dashboard-messenger-round2
related:
  261107-feat-ws-dashboard-conversation-history: touches the same session page layout and assets; merge-time collision only
---

# Dashboard: session status band (context, branch, sync, usage) around the composer

## Background

In the terminal the user watches a status line (model, session id, folder,
branch, diff counts, context tokens and cache rate, usage limits, cost). The
session page shows none of it. The user asked for a band with the folder
name, branch state, push/pull state, usage or cost when available, and above
all the current context usage (2026-11-07: "폴더 이름, 브랜치 상태, 브랜치
변경내역(push/pull 상태) 그리고 가능하면 사용량이나 금액 정보, 제일 중요한 게
지금 컨텍스트 사용량").

A static mockup drew three placements:
`ai-docs/ref/whiteboard-mockup/r5-status-band/index.html` (4bd36d1ea,
usage chips 48e9dd44d).

## Decisions

- **Variant C** (user, 2026-11-07, "(58) c, note: 저는 이게 제 취향이긴
  해요."): the context bar sits directly above the composer; the rest (folder,
  branch, sync, usage, cost) is one line below the composer. Rejected: A, a
  one-line strip under the header; B, a 2-3 row terminal-style band.
- **Usage chips** (user, 2026-11-07: "이거 week말고 5h 제약이 같이 뜨면 어떻게
  되나요? 그리고 wk 표시는 필요 없긴 해요"): the 5-hour chip first
  (`42%/18:00 (+5%)`), then the weekly chip (`17%/Sun (-72%)`); no "wk"
  label, the reset clock time or weekday tells them apart; a chip appears only
  when its limit is reported; when both do not fit on a narrow phone band,
  the higher-percent chip stays (a tie keeps the 5-hour chip), as the mockup
  draws.
- **States as drawn in the chosen mockup**: context and usage warn at 80%+ and
  turn critical at 95%+; sync shows ahead/behind arrows, synced, or no
  upstream; a host without cost or usage data (Pi) omits those chips.
- **Epic scope**: the epic's Non-Scope "usage tracking" line is narrowed to
  usage management; read-only usage display in this band is in scope (the
  user's request above).

## Constraints

- Data sources are not settled and must be before `ready/`: where Claude's
  context tokens, cache rate, usage limits, and cost reach the dashboard
  (the Claude Code status line input carries them; whether the claude-mod
  adapter can read them is unverified), what Pi exposes, and where folder,
  branch, and ahead/behind come from (daemon-side git versus the adapter).
- Page preferences stay browser-local (epic cross-child decision).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`; adapter changes also
  `ai-docs/manuals/wsflow-mirroring.md`.

## Phases

### Phase 1: Status band in variant C

Implement the band on the live session page per the decisions above, fed by
the data sources settled before promotion.

Verification: page render tests for each state (normal, warning, critical,
no upstream, ahead/behind, Pi without usage); the narrow-phone chip rule; a
live check on Claude and Pi that the context value tracks the host's own
status line.
