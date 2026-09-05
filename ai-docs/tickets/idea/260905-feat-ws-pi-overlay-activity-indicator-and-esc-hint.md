---
title: Owner overlay shows a working indicator while the respondent thinks and states what Esc does
related:
  260904-feat-ws-pi-side-thread-fork-question-surface: owns the overlay chat component
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - pi-adapter-runtime
---

# Owner overlay shows a working indicator while the respondent thinks and states what Esc does

## Background

Acceptance run 2026-09-05, scenario E3: after answering a fork-raised
question in the overlay, the owner saw no sign that the fork was processing
the answer, and could not tell whether `Esc` cancelled the exchange or only
closed the view. The spec is unambiguous (`Esc` closes the view only; the
thread stays open and the fork keeps running), but nothing on screen says
so, and the overlay renders activity only through streamed text deltas, so a
respondent that is thinking or running a tool shows nothing at all.

## Direction

- Render a one-line activity marker in the overlay while the respondent is
  streaming or running a tool and no text delta has arrived yet (reuse the
  `streaming` tail slot; clear it on settle).
- Put a fixed hint in the overlay header or footer: `Esc closes the view
  (thread stays open) · /done ends the thread`.
- Keep both behind the existing `render(width)` width bound and cover them
  in the overlay unit tests.
