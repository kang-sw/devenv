## Included Guidance: Codex Open Decision Queue

- Use Codex's visible plan/task-list surface for the Open Decision Queue.
- Create one task per unresolved decision before ticket cleanup starts.
- Write each task's visible text as the decision itself, self-describing and not a label; treat any secondary note or description field as optional detail that may not render and must never carry load-bearing content.
- Mark an item complete only after the user confirms, rejects, or defers it.
- Before marking an item complete, rewrite its text with `[confirmed]`, `[rejected]`, or `[deferred]`.
- Keep the task list as the state record: the decisions themselves travel through the response body, and every item stays visible here rather than in hidden notes.
- Keep recommendations out of the task list, where the added length invites the truncation that makes an item unreadable.
- Reflect every disposition the reconcile step assigns before continuing, and persist only items that are no longer open.
