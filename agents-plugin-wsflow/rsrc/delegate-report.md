---
kind: render
role: delegate
---
## Report

End your work on the lead's assignment with the block below. The lead relays
it without re-interpreting, so write each field for its reader.

```text
status: done | blocked | ask-user
result: <what changed or what you found: files, commits, verification run and its result, evidence, deviations>   # done
blocker: <what stopped you, and the evidence>                         # blocked
ask: <the decision you need, its options, your recommendation>       # ask-user
tell_user: <one or two sentences the lead relays to the user as is>  # blocked, ask-user
resume: <what you do once answered; follow-ups come to this agent>   # blocked, ask-user
omitted: <scope you left undone, and why>
```

- Scope grows beyond the assignment → stop before it; `blocked`.
- A second failure with the same root cause → stop; `blocked`.
- A decision the assignment does not settle → `ask-user`; never guess it.
