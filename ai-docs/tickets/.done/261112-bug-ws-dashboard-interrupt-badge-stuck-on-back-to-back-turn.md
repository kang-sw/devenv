---
title: "Dashboard interrupt badge stays \"Interrupting…\" when the next turn starts right after an interrupt"
related:
  261110-feat-ws-dashboard-headless-lead-sessions: source of the hosted interrupt path
completed: 2026-11-12
---

# Dashboard interrupt badge stays "Interrupting…" when the next turn starts right after an interrupt

## Background

Found while dogfooding a daemon-hosted Claude lead session (2026-11-12,
reply-id `autisticcavalierharpistdairy`). The user clicked the session page's
interrupt badge while the lead was mid-turn. The harness stopped the turn
(`[Request interrupted by user]`), and a peer's ws mail held for that session
was delivered at once as the next turn. The badge then stayed
"Interrupting…" (ko: "중단 중…") and disabled for the whole new turn. It
cleared only after that turn ended and the session went idle (user-observed).

`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` lines 1555-1622:
`interruptTurn` sets `interrupting = true`, and `syncBadge` resets it only
when it sees the convlive `data-phase` outside the working set (thinking,
replying, tool, permission). This is deliberate: "It then stays disabled
until the phase leaves the working set, so a second click cannot cut the
next turn." When held input starts the next turn before any page swap shows
a non-working phase, the flag carries over. That turn cannot be interrupted
from the page, and the label says an interrupt is still in flight.

## Direction (unconfirmed)

- Reset on a turn boundary the server reports, such as a turn counter or
  turn-start stamp on the convlive region, not on an observed phase.
- Keep the guard against a double click cutting the next turn: drop only the
  click that was in flight for the old turn.

## Verification idea

Interrupt a hosted session while it has a held message or queued mail, so
the next turn starts at once. The badge should return to "Interrupt" and
work for the new turn.


## Resolution (2026-11-12)

Fixed in 7eb0e7289 (the page records the lead turn's start stamp at the click and clears the interrupting state when the stamp changes). Verified live after a tab reload; the one stuck case seen after the merge was a tab still running the old inline script, tracked by 261112-bug-ws-dashboard-open-tab-keeps-stale-assets-after-daemon-upgrade. User board (25).
