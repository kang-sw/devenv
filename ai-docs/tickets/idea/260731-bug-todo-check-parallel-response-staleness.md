---
title: todo.check parallel response staleness
---

# todo.check parallel response staleness

## Background

Two concurrent `ws/todo.check` calls each returned a full todo rendering that
did not consistently reflect the other completed mutation. A subsequent
`ws/todo.list` showed both items correctly completed. The final state was
correct, but callers cannot safely use either individual mutation response as
the authoritative postcondition when updates overlap.

## Phases

### Phase 1: Define concurrent todo-mutation response semantics

Determine whether todo mutations must serialize their response rendering or
return a post-commit state revision. Preserve correct final state and make each
successful response unambiguous for callers that issue overlapping mutations.

**Verification:** concurrent `todo.check` calls return responses consistent with
the final persisted todo state, or explicitly identify the returned revision.
