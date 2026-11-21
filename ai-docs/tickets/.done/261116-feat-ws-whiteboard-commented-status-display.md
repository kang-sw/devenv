---
title: "whiteboard \"commented\" 상태 표시 (승인된 목업)"
completed: 2026-11-21
---

# Display the whiteboard "commented" status (approved mockup)

## Background

261113-feat-ws-whiteboard-update-refusal-cost Phase 1 added the `commented`
status: a policy item answered with a note only. That status currently
renders as the bare English text "user commented, lead's move". The mockups
(16 PNGs, develop `998cb3fe3`, under that ticket's assets) show fresh,
recent-settled, history and re-ask states.

## Decisions

- (1182)=ok, 2026-11-16: the owner approved the mockups as drawn. That
  includes state 3, where a commented item stays inside the settled list as
  shown.
- UI work is done by a Claude agent, never by a GPT/pi worker.

## Phases

### Phase 1: Implement the approved display

- Implement the mockup states, adding i18n keys for every new string.
- Fix two defects the mockup work found in the merged placeholder (see the
  `998cb3fe3` merge body):
  - the historyTransition re-ask fallback;
  - the bare `stext` span.
- Acceptance: screenshots at 1440, dark and light, match each mockup state.
  Existing whiteboard view and status tests still pass. The owner checks it
  visually.


## Resolution (2026-11-21)

Implemented whiteboard `commented` status display (`c3625b542`, merged onto `develop`) and confirmed live by owner (`(62)=ok`).
