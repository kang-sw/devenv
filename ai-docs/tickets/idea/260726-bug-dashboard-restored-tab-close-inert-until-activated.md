---
title: Dashboard terminal tab close button is inert on a reload-restored tab until the tab is clicked once
related:
  260725-feat-dashboard-pty-agent-attention-notification: Phase 6 hit the mirror-image activation-only binding problem; source of the hypothesis below
  260525-bug-ws-dashboard-agent-tab-close-confirmation-sticky: earlier close-confirmation flakiness on the same tab close path
---

# Dashboard terminal tab close button is inert on a reload-restored tab until the tab is clicked once

## Background

Found while writing a dashboard e2e test on 2026-07-26. A terminal tab that
was restored by a page reload and never clicked cannot be closed: the `×`
click produces no confirmation popover and no close, and the pane stays
running. The failure a11y snapshot captured this state. Clicking the tab body
once first makes the identical close work.

## Reproduction

1. Open a dashboard terminal tab.
2. Reload the page so the tab is restored from persisted layout state.
3. Click the tab's `×` without ever clicking the tab body first.

- Expected: confirmation popover appears, then the tab closes.
- Actual: nothing happens; the pane keeps running.
- Control: clicking the tab body once before the `×` makes the same close work.

## Hypothesis (unverified — not a diagnosis)

Nobody has read the close path yet; the following is a starting point for
investigation, not a finding.

Phase 6 of `260725-feat-dashboard-pty-agent-attention-notification` fixed the
mirror image of this shape: an acknowledge action was bound only to Dockview's
`onDidActivePanelChange`, a *change* event that never fires for an
already-active tab, which left the feature's primary flow permanently stuck.
The fix was a second, change-independent trigger (the tab's own `onClick`)
handed over the layout's stable `callbacksRef`, because a raw prop would have
frozen at first paint — connected-terminal params almost never refresh.

The symptom here — close inert until the tab is clicked once — has that same
shape, so a plausible first hypothesis is that the close handler reaches a
stale closure or a params snapshot that is only refreshed on activation.
Confirm or refute against the actual close path before acting on it.

## Notes

- Scope is the dashboard frontend tab/close path; no backend behavior
  implicated by the observation.
- Captured at idea stage: the symptom is reproducible, but neither the
  mechanism nor the blast radius (terminal tabs only vs. all tab kinds,
  reload-restored only vs. any never-activated tab) has been checked.
