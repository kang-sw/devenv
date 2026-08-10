---
kind: print
---

# Scope Worktree

Target: user request

## Invariants

- Never write a `git sparse-checkout` pattern before discussing what this worktree targets with the user and getting their explicit topic/pattern decision.
- `--no-cone` is required — cone mode selects directories, not files, and `git sparse-checkout set <file>` fails with `is not a directory`.
- Scope covers `ready/`, `todo/`, and `idea/` uniformly — no status directory is exempt; before excluding `idea/*`, ensure a tracked `/ai-docs/tickets/idea/.gitkeep` exists (create and commit it if absent), because git does not track empty directories and the directory otherwise vanishes from disk (`ai-docs/ref/worktree-ticket-scope.md`).
- Verify by listing the affected status directories after every apply — mandatory, not optional. It is the only defense against the unreproduced hazard recorded in `ai-docs/ref/worktree-ticket-scope.md` (`## Unreproduced Hazard`): no pattern shape is provably safe, so the effect on disk must be confirmed by listing, never assumed from the apply command's exit code.
- Promoting a ticket into a hidden status directory (the `idea/` -> hidden `todo/` triage hot path) requires widening the pattern first (`git sparse-checkout add <path>`), then moving with `{{.McpNamespace}}/tickets.move`, which recreates the status directory the scope emptied off disk; a cross-scope move before widening refuses as an atomic no-op, and a raw `git mv` into a vanished directory fails even after widening (see `ai-docs/ref/worktree-ticket-scope.md`).
- Restore is `git sparse-checkout disable`, which fully restores the worktree.

## On: invoke

1. **Discuss.** Ask the user what this worktree's work line or topic is. Wait for their answer before writing anything — do not guess a pattern from ticket titles, branch names, or recent commits.
2. **Derive.** From the conversation, build the pattern set as a single command:
   `git sparse-checkout set --no-cone /* !/ai-docs/tickets/ready/* !/ai-docs/tickets/todo/* !/ai-docs/tickets/idea/* /ai-docs/tickets/idea/.gitkeep <topic re-includes>`,
   where `<topic re-includes>` are one or more glob re-includes such as
   `/ai-docs/tickets/ready/<topic-glob>* /ai-docs/tickets/todo/<topic-glob>*`, optionally adding
   `/ai-docs/tickets/idea/<topic-glob>*` when the topic's own captures should stay visible,
   naming the tickets that should stay visible for the discussed work line.
3. **Ensure the idea/ keep-file.** Before applying a pattern that excludes `idea/*`, confirm `ai-docs/tickets/idea/.gitkeep` exists and is tracked; if absent, create it (empty file) and commit it via `{{.McpNamespace}}/git.commit` before running `git sparse-checkout set` — otherwise the directory disappears from disk instead of narrowing to the keep-file.
4. **Apply.** Run the derived `git sparse-checkout set` command.
5. **Verify by listing (mandatory, every time).** List `ai-docs/tickets/ready/`, `ai-docs/tickets/todo/`, and `ai-docs/tickets/idea/` and report to the user exactly which tickets remain visible in each — `idea/` should show only `.gitkeep` plus any topic-matched re-includes. Do this even when the apply command reported success — success only confirms the command was accepted, not what actually stayed on disk. Skipping this step is never acceptable, regardless of how confident the derived pattern looks.
6. **Explain the remedies.** Tell the user: to promote a ticket into a currently-hidden status directory — including the `idea/` -> `todo/` triage hot path — widen the pattern first (`git sparse-checkout add <path>`), then move it with `{{.McpNamespace}}/tickets.move`, which recreates the status directory the scope emptied off disk — a raw `git mv` there fails with `No such file or directory` until the directory is recreated (`ai-docs/ref/worktree-ticket-scope.md` "Cross-Scope `git mv`" carries the detail). `git sparse-checkout disable` fully restores the worktree at any time.

## Doctrine

`{{.SkillNamespace}}:lead-scope-worktree` trades the finite resource of
session attention — tickets from unrelated work lines competing for "what
should I work on next" — against the visibility cost of a wrong scope, which
is cheap to correct (widen or disable) and never destroys data. The skill
therefore authors the scope only from an explicit user decision, never
invents scope from inference, and treats listing after every apply as
non-negotiable, because that is the only known check against the tree's one
unreproduced failure mode.
