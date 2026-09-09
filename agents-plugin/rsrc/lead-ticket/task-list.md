## Included Guidance: Open Decision Queue Task List

- Use a visible task list when the harness exposes one; otherwise print a concise Markdown checklist, applying the same item rules below to it.
- One queue item equals one unresolved decision that could change persisted ticket, spec, focus, or note text.
- Write the item's visible text as the decision itself, self-describing and not a label; treat any secondary note or description field as optional detail that may not render and must never carry load-bearing content.
- Track each item as `open`, `confirmed`, `rejected`, or `deferred`.
- Before closing an item, rewrite it with `[confirmed]`, `[rejected]`, or `[deferred]`.
- Keep the list as the state record: the decisions themselves travel through the response body, and every item stays visible here rather than in hidden notes.
- Keep recommendations out of the list, where the added length invites the truncation that makes an item unreadable.
- Reflect every disposition the reconcile step assigns before continuing.
