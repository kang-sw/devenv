---
title: sage_stamp re-stamp repeats the fix-then-re-stamp instruction; move asks for a gate already resolved
---

# sage_stamp re-stamp repeats the fix-then-re-stamp instruction; move asks for a gate already resolved

## Background

Observed while promoting `261029-feat-claude-mod-package-parity-knob-defaults`
(2026-10-29):

- `tickets.sage_stamp(stage: combined)` with passing verdicts carrying
  `resolution: autonomous` issues returns "Fix the N autonomous issue(s) ...
  then call sage_stamp again with the same verdicts". After the fixes, the
  re-stamp with the same verdicts returns the identical instruction, so the
  caller has no signal that the second stamp was the terminal one; the
  instruction loops by construction.
- `tickets.move(to: "ready")` after the gate was resolved and stamped returns
  `next_instruction: Call tickets.sage_gate(stem, landing) next`, although the
  gate already ran for this promotion.

Expected: a re-stamp whose digest differs from the first stamp's (body
changed) acknowledges it as the post-fix stamp; a move whose posture is
already `completed` for both stages does not ask for the gate again.
