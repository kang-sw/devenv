---
title: "dashboard e2e: the acceptance suite's .xterm-rows assertions cannot see terminal text once the WebGL/canvas renderer is active"
related:
  260727-chore-merge-ws-dashboard-dev-into-goal-branch: surfaced-by
---

## Symptom

`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` aborts at its first
`.xterm-rows` assertion once the merged branch is in place. The assertions
read terminal content out of the DOM; the renderer the dev line brought in
draws that content to a canvas instead, so the DOM rows the locator targets
are empty or absent.

Surfaced while resolving Phase 2 of
`260727-chore-merge-ws-dashboard-dev-into-goal-branch`. This is **not** a
merge-resolution defect: the assertions are byte-identical on both sides and
at the merge base, so they never conflicted and no resolver decision touched
them. The merge is simply the first place the two halves met - our line owns
the acceptance assertions, the dev line owns the renderer change.

## Finding

- Proven by experiment during the merge, not inferred: forcing the DOM
  renderer advances the same run from the first failing assertion to a much
  later line, so the renderer is the cause and the assertions themselves are
  otherwise still valid.
- `grep -c` reports 38 `.xterm-rows` assertions in the file. Treat that as an
  upper bound on the blast radius, **not** as a count of failures: Playwright
  aborts the spec at the first failure, so only the first has actually been
  observed to fail. Nobody has run the file to completion under the WebGL
  renderer, and the remaining 37 are unexecuted.
- The failure is in the test's observation method, not in the product. There
  is no evidence the terminal renders incorrectly for a user; the evidence is
  that a DOM-reading assertion cannot observe a canvas.

## Decision needed

Two shapes, and they are not equivalent:

1. **Re-anchor the assertions** to something the WebGL renderer exposes -
   xterm's serialize addon, its accessibility tree, or a screenshot
   comparison. Keeps the tests running against the renderer users actually
   get, which is the whole point of an acceptance suite. Costs a rewrite of
   up to 38 assertions and a new way of reading terminal state.
2. **Pin the DOM renderer under test.** Cheap, and every assertion keeps
   working unchanged - but it means the acceptance suite stops exercising the
   renderer that ships, which is a real reduction in what the suite proves.
   If this is chosen it should be recorded as a deliberate coverage boundary,
   not left implicit in a config flag.

Option 2 is the tempting one and is worth choosing only with its cost stated.

## Before deciding

Run `dashboard-acceptance.spec.ts` to completion under the DOM renderer and
under the WebGL renderer, and diff the failure sets. The 38 figure is a grep
count; the real number of assertions that cannot survive the renderer change
is unknown and is the input this decision actually needs.

## Option 2 taken provisionally - 2026-07-29

Recorded here because this ticket asked that the choice not be left implicit in
a config flag.

While restoring the acceptance gate after the terminal status-bar removal
(`e946eb63`, PR #7), the `.xterm-rows` breakage blocked every downstream step and
was pinned to the DOM renderer via `addInitScript` setting
`gpuAcceleration: false`, mirroring what `altScreenRootSwitch.spec.ts` already
did. The spec carries an explaining comment at the pin site.

**This was expedient, not a considered answer to the decision above.** The cost
option 2 was flagged as carrying is now real: the acceptance suite no longer
exercises the renderer that ships. Every `.xterm-rows` assertion in the file now
proves something about a renderer users do not get by default.

Two things this does resolve:

- The blast-radius question is partly answered. With the pin in place the file
  now runs to completion, so the `.xterm-rows` assertions are no longer masked by
  an early abort. The remaining failure is unrelated (see
  `260725-bug-dashboard-fitnow-short-viewport-shrink`). The "diff the failure
  sets" experiment still has not been run under WebGL.
- The `altScreenRootSwitch.spec.ts` precedent means two specs now pin the DOM
  renderer independently. If option 1 is ever chosen, both need re-anchoring.

The ticket stays open. Option 1 remains the answer that keeps this suite an
acceptance suite.
