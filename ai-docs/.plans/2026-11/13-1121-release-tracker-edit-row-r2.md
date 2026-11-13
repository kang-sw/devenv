# Release gate repair round 2: tracker edit-row correctness

The ship re-review of `f01fed037..473dfd0e0` (correctness reviewer,
2026-11-13) found these defects in the tracker pencil edit row. This branch
(`fix/release-tracker-edit-row-r2`, off the ship cut 473dfd0e0) becomes the
new ship cut. Verify each finding against the code first; fix the real ones,
push back in the report on any you can show is wrong.

## Findings to fix

1. Critical, `agents-plugin-tool/internal/wsdashboard/assets/tracker.js:766-777`:
   untouched fields overwrite concurrent updates. The editor compares stale
   prefills against current state instead of recording which fields the user
   changed. Open an editor with due A, receive another writer's due B, change
   only the title: save restores due A (priority and status alike). Fix:
   compare against the values captured when the editor opened (or track
   dirty fields) and send only fields the user changed.
2. Important, `tracker.js:5035-5039` (prior must-fix 6, partial): pick L2
   and close; before that write completes, reopen, pick L3, close. Both
   payloads are built from the pre-L2 labels, so the second full-list
   replacement removes L2. Fix with per-item pending-write serialization or
   rebasing the next pick on the pending result (simplest correct).
3. Important, `tracker.js:778-779`: assignee edits use an additive event.
   Changing `[A]` to `[B]` sends only `item.assign(B)`, leaving A assigned;
   removing an assignee or changing only completion is refused because the
   selected people already hold assignments, which also rejects the other
   edits in the atomic batch. Fix: send the right assignment delta
   (add new, remove dropped, completion handling) using the engine's
   existing events (`agents-plugin-tool/internal/wstracker/`).
4. Important, `tracker.js:4439-4446`: a wrapped done/dropped assignment's
   editor offers active statuses, but saving sends `item.status` and the
   engine rejects it ("reopen it first", `wstracker/check.go:1218-1219`).
   Fix: use the existing reopen flow, or do not offer transitions that need
   it.
5. Important, `tracker.js:739-748`: unheld topics prefill the creator as
   holder (`editWho` substitutes `[it.author]` and records it as the
   original), so choosing the creator writes nothing. Fix: keep the real
   empty assignment state.
6. Important, `tracker.js:795`: an earlier save discards a newer draft. Save
   row A, open and type in row B before A's response arrives; A's success
   callback clears `ui.ned`, deleting B's draft. Fix: clear only the draft
   that belongs to that save.

Also, if cheap and in the same code: the item window's edit form
(`startEdit`/`saveEdit`) writing the whole label list (prior leftover).
Otherwise leave it.

## Out of scope

- Empty-session slash completion (fixed post-cut on develop, 1fec38d74).
- Restarting the dashboard daemon (other sessions' Pi turns would die).

## Verification

- A node-backed JS test per fixed finding in the existing tracker test style
  (behaviour, not source-string greps), run locally with node at
  `C:/Program Files/nodejs` under the local lock
  (`bash D:/sys_lock.sh run <who> <min> -- <cmd>`).
- Go tests on the remote w5090 for the touched packages
  (`D:/sys_lock.sh remote w5090 run <who> <min> --sync <worktree> -- 'export
  PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test
  ./internal/wsdashboard/... ./internal/wstracker/... -count=1'`).
- One commit per logical fix or one repair commit, each with `## AI Context`.
  State what was not checked live. Temp files only under D:/Temp/<purpose>/,
  deleted when done.
