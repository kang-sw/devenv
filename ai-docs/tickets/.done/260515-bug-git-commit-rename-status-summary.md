---
title: git.commit rename status summary mismatch
spec:
  - 260519-git-commit-add-delete-ticket-move-summary
plans:
  phase-1: 2026-05/19-1754.git-commit-rename-summary
related-mental-model:
  - git-workflow-tools
  - documentation-system
completed: 2026-05-19
---

# git.commit rename status summary mismatch

## Background

During dashboard daemon ticket promotion, `ws/git.commit` successfully committed
a `todo/` to `ready/` ticket move, and follow-up `ws/tickets.status` plus the
filesystem confirmed the ticket lived under `ready/`.

The `ws/git.commit` result payload still reported the ticket change as
`to_status: todo` for that rename. This is surprising because callers use the
commit result as workflow evidence after ticket status moves.

Temp-repo reproduction narrowed the condition: ordinary ticket renames report the
destination status correctly, including `todo -> ready` moves with light body
edits. The mismatch appears when a ticket status move also changes enough content
for native Git to report the staged change as separate `A <new-status>/<stem>.md`
and `D <old-status>/<stem>.md` records rather than an `R...` rename record. A
common trigger is adding a `### Result (...)` section while moving the ticket.

Native Git is allowed to commit that add/delete shape. The bug is only in the
workflow summary layer: `ws/git.commit` should conservatively reinterpret an
unambiguous same-stem ticket add/delete pair as a ticket status move so callers
see the destination status that matches the filesystem after commit.

## Phases

### Phase 1: Reconstruct unambiguous add/delete ticket moves

Update ticket-change detection so an add/delete-shaped ticket move is summarized
as a move only when all of these conditions hold:

- exactly one added ticket path and exactly one deleted ticket path share the
  same ticket stem;
- both paths are under recognized `ai-docs/tickets/<status>/` directories;
- the added and deleted statuses differ;
- no explicit `R...` rename record for the same stem already exists.

The reconstructed move must report `from_status` from the deleted path and
`to_status` from the added path. If a `### Result` or `#### Edition` heading is
also detected for the same stem, merge that result evidence into the reconstructed
move without letting the deleted path overwrite the destination status.

Do not make ambiguous add/delete sets look like moves. Multiple added paths,
multiple deleted paths, unmatched stems, convention-escaping paths, or same-status
delete/add pairs should remain non-move ticket changes rather than inventing a
destination. `ws/git.commit` should not reject native Git commits solely because
the ticket summary is ambiguous.

Verification should cover:

- normal `R...` ticket moves still report the destination status;
- `A ready/<stem>.md` plus `D todo/<stem>.md` reports `todo -> ready`;
- the same add/delete shape with a `### Result` addition still reports
  `todo -> ready` and preserves `result_added`;
- ambiguous same-stem add/delete sets are not reconstructed as a move.

### Result (bbd9a376) - 2026-05-19

Implemented conservative add/delete-shaped ticket move reconstruction in
`ws/git.commit` ticket-change summaries. Unambiguous same-stem ticket add/delete
pairs now report the destination status as a move, while ambiguous same-stem
sets stay non-move evidence.

Review cycle 1 found that parser-level ambiguity preservation was insufficient
because `detectTicketChanges` still merged by stem. The final implementation
preserves parsed ticket changes as separate records, merges Result/Edition
evidence by exact destination path, and keeps explicit `R...` rename records
from being overwritten by same-stem add/delete passthrough records.
