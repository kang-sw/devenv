---
kind: print
---

# Scope Worktree

Target: user request

## Invariants

- Never write a `git sparse-checkout` pattern before discussing what this worktree targets with the user and getting their explicit topic/pattern decision.
- `--no-cone` is required — cone mode selects directories, not files, and `git sparse-checkout set <file>` fails with `is not a directory`.
- Scope covers `ready/` and `todo/` only; `idea/` always stays visible. `idea/` is the capture surface for surprises found mid-work, and such a capture is off-topic for the worktree by nature, so hiding `idea/` would put every new capture out of scope and make it fail at the staging step — an out-of-scope path cannot be staged under an active scope.
- Verify by listing the affected status directories after every apply — mandatory, not optional. It is the only defense against the unreproduced hazard recorded in `ai-docs/ref/worktree-ticket-scope.md` (`## Unreproduced Hazard`): no pattern shape is provably safe, so the effect on disk must be confirmed by listing, never assumed from the apply command's exit code.
- Promoting an out-of-scope ticket into a hidden status directory (e.g. `idea/` -> hidden `todo/` triage) requires widening the pattern first (`git sparse-checkout add <path>`); the bare move fails atomically otherwise.
- Restore is `git sparse-checkout disable`, which fully restores the worktree.

## On: invoke

1. **Discuss.** Ask the user what this worktree's work line or topic is. Wait for their answer before writing anything — do not guess a pattern from ticket titles, branch names, or recent commits.
2. **Derive.** From the conversation, build the pattern set as a single command:
   `git sparse-checkout set --no-cone /* !/ai-docs/tickets/ready/* !/ai-docs/tickets/todo/* <topic re-includes>`,
   where `<topic re-includes>` are one or more glob re-includes such as
   `/ai-docs/tickets/ready/<topic-glob>* /ai-docs/tickets/todo/<topic-glob>*`
   naming the tickets that should stay visible for the discussed work line.
3. **Apply.** Run the derived `git sparse-checkout set` command.
4. **Verify by listing (mandatory, every time).** List `ai-docs/tickets/ready/` and `ai-docs/tickets/todo/` and report to the user exactly which tickets remain visible in each. Do this even when the apply command reported success — success only confirms the command was accepted, not what actually stayed on disk. Skipping this step is never acceptable, regardless of how confident the derived pattern looks.
5. **Explain the remedies.** Tell the user: promoting a ticket that is currently hidden or `idea/`-only into scope requires widening first (`git sparse-checkout add <path>`) before the move will succeed; `git sparse-checkout disable` fully restores the worktree at any time.

## Doctrine

`{{.SkillNamespace}}:lead-scope-worktree` trades the finite resource of
session attention — tickets from unrelated work lines competing for "what
should I work on next" — against the visibility cost of a wrong scope, which
is cheap to correct (widen or disable) and never destroys data. The skill
therefore authors the scope only from an explicit user decision, never
invents scope from inference, and treats listing after every apply as
non-negotiable, because that is the only known check against the tree's one
unreproduced failure mode.
