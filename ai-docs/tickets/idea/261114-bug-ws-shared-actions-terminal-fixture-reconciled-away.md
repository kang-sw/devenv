---
title: Shared-actions terminal fixture disappears under native reconciliation
---

# Shared-actions terminal fixture disappears under native reconciliation

## Background

During the authorized 649 follow-up, both clean candidate wide-1800
shared-actions scenarios stopped before their final empty-board assertion.
The scenario's POST-stubbed terminal `bh-shared-exited` was present before
drag and absent during/after drag. It has no native daemon list entry. Both
runs then timed out finding `.bcard[data-kind="terminal"] .bc-fold`.
Native terminal lists and initial terminal-card counts were zero after closing
only the explicitly owned seeded terminals, each with HTTP 204.

The fake terminal's disappearance is recorded. Native terminal reconciliation
is a plausible cause, not a separately proven timing root cause. The earlier
fixture variant retained real server-owned exited terminals and therefore
failed its later empty-board assumption instead. Neither result clears the
full wide scenario: in the clean runs the original empty-board checkpoint was
never reached. No retry, source fix or assertion waiver was performed.

Evidence: `/tmp/compact-panel-649-700-followup/` contains
`wide-candidate-{board,adapter}/diagnostics.json`, the terminal preconditions,
full logs and screenshots. The final report is
`/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@08d59332/review-paths/1de23d1e-01-compact-header-649-followup-terminal.md`.

## Constraints

- Keep daemon terminal reconciliation and original behavior assertions intact.
- Create/close only explicitly owned fixture terminals; do not enumerate and
  close unrelated terminals or delete cards to manufacture acceptance.
- Do not call this a 649 regression or a clean wide pass without attribution.
  New runtime checks require the shared heavy slot and bounded approval.

## Phases

### Phase 1: Define a terminal fixture consistent with native authority

Trace the stubbed terminal lifecycle, list reconciliation and scenario cleanup.
Return a minimal fixture design that can exercise real terminal fold/max and
then legitimately reach the empty-board checkpoint. Distinguish any necessary
fixture setup correction from a product bug; preserve native ownership,
cleanup evidence and independent fatal guards.
