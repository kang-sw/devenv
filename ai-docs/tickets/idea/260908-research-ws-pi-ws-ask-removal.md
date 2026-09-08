---
title: Consider removing ws-ask after costly discussion-fork dogfooding
---

# Consider removing ws-ask after costly discussion-fork dogfooding

## Background

On 2026-09-08 the owner reported that discussion forks opened for `ws-ask`
questions rarely reuse the expected prefix cache and can cause abrupt cost
spikes when the owner opens them. The owner considers the current utility low
relative to that expense. These are owner-reported observations, not an
independently established cache-hit measurement or a proven universal provider
behavior.

## Immediate mitigation versus permanent removal

The owner authorized an urgent, reversible implementation slice to hide
`ws-ask` from the model-visible tool/schema list, preventing new model-issued
questions. That implementation is separate from this idea; this capture does
not claim the mitigation has already landed or that it removes every possible
fork cost.

The permanent fate of `ws-ask` is explicitly undecided. Record eventual tool
removal here for a later owner decision; do not treat temporary hiding as
approval to delete the implementation, existing question/thread state, or the
separate `ws-fork` capability.

## Recovery context

The inspected lifecycle separates question registration from fork creation:
`ws-ask` records a pending question without spawning; opening it through
`/answer` creates the discussion respondent from the then-current lead context.
Therefore hiding the tool prevents new model-created questions but does not
itself establish that previously pending questions are cost-free to open.
The implementation should make the exposure boundary and reload requirements
explicit rather than promise a retroactive removal of already-issued schemas.

## Deferred decision

Reassess whether to remove the tool after considering the reported cost and
observed usefulness. Before any permanent removal, explicitly settle its scope,
existing-thread handling and the affected guidance/tool contracts. No deletion,
replacement interaction design, automatic re-enable condition, or wider fork
policy is chosen in this ticket. Do not expand this idea into implementation
until the owner decides its fate.
