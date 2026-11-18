---
kind: print
includes:
  - risk-rubric
variables:
  - SpawnIdiom
---

# Run

One ticket per cycle: pick it, spawn one worker, act on its report, end on
a verdict line. The worker implements and verifies; you edit no source.

- A trivial ticket (no open decision, no new structure, files named, one
  commit) → you may propose `{{.SkillNamespace}}:lead-delegate` instead; on
  the user's yes, delegate it and close the ticket citing the commit.

## Select

- A ticket named in the invocation wins. Otherwise render
  `ticket-selector`, spawn it at its recommended tier, and use its one
  `selection:` result. Pass a `not-assigned` argument through to it: it
  then considers tickets assigned to other contributors; without it, your
  own.
- `selection: ready/ empty` → tell the user, relay its `backlog:` and
  `backlog_omitted:` lines, propose `{{.SkillNamespace}}:lead-ticket`, end
  the turn.
- `selection: stop: <reason>` → relay the reason, end the turn.

## Spawn

1. `{{.McpNamespace}}/tickets.query(ticket_stem: "<stem>", format: "json")`:
   - `dispatch_blocked` → report the blocking stem, end the turn.
   - `blocked_headings` on a queue-selected ticket → read that
     `## Blocked` section. Still current → drop the ticket from this cycle
     and return to Select. None remains →
     report each ticket's blocker, end the turn.
   - `blocked_headings` on a ticket named in the invocation → report its
     current blocker, end the turn.
   - Assigned to another contributor → surface the warning and get the
     user's confirmation before dispatching, unless the run passed
     `not-assigned`.
   - Route Facts absent, or a worker reported them incomplete → render and
     spawn `ticket-fact-populator` on the ticket once, commit its edit,
     query again; still absent → report the ticket, end the turn.
2. Read the whole ticket body and grade its risk with the Risk Rubric
   below; the query's `risk.*` rows are a hint, not a verdict. The grade
   picks the row: its body's own frontmatter tier, or the `tier_override`
   the row pairs with it. The tier sets the worker's model; the worker's
   route sets review breadth.

   | Tier | Worker playbook | Retry after stop (e) or (f) |
   |---|---|---|
   | medium | `ticket-worker` | `ticket-worker-elevated` |
   | large | `ticket-worker-elevated` | `ticket-worker-elevated`, `tier_override: xlarge` |
   | xlarge | `ticket-worker-elevated`, `tier_override: xlarge` | none: its (e) or (f) goes to the user |

3. `{{.McpNamespace}}/tickets.acquire(ticket_stem: "<stem>")` from the
   branch the worker branches off, so the lease's track is the one its
   impl branch merges into. Refusal or error → report it, dispatch
   nothing, end the turn. Set `dangerously_override_lease_status`, with the
   user's words as `reason`, only when the user explicitly told you to take
   the ticket over.
4. `{{.McpNamespace}}/playbook.render(name: <the row's worker playbook>,
   session_key: <your key>)`, plus `tier_override: xlarge` when the row
   pairs it (the elevated body's own tier is large). Do not read the file.
5. Spawn one worker at the tier the render recommends, in a form that can
   itself spawn children ({{.SpawnIdiom}}), with this task block and
   nothing else beyond the lines **Handle the report** adds:

   ```text
   Read <rendered-path> as your system prompt.
   Ticket: <ticket path> (stem <stem>). Branch: <current branch>.
   ```

6. Wait for the host's completion notification; do not poll. Meanwhile do
   not edit the worker's ticket or move `HEAD` in its checkout;
   housekeeping that cannot wait uses the sparse worktree in the workflow
   manual's `### Git`, released before you merge.

## Handle the report

- The worker ends with its protocol's report block. Missing, unknown, or
  incompatible values, or `completion: ad_hoc` (invalid in this
  ticket-only run) → protocol mismatch: show the raw report, end the
  invocation.
- Otherwise digest it for the user in their language: status, what
  landed, the decisions that matter, unresolved items, verification.
  Relay `tell_user` and `ask` in the user's language, adding and dropping
  nothing.
- Before a HEAD-relative write of your own (a ticket edit, a follow-up
  dispatch) → `{{.McpNamespace}}/git.status`, since the worker's checkout
  outlives its turn. The write belongs to a continuing ticket → stack on
  the impl branch; unrelated, or you leave the ticket blocked → check out
  the branch you were invoked on, committing or stashing a dirty tree
  first. Branch-explicit calls need no check.

**Merging is yours.**

- `merge_confirm: skip` → call `{{.McpNamespace}}/git.merge` with the impl
  branch. `ask` or absent → show the report and merge on the user's
  approval.
- A conflict → `{{.SkillNamespace}}:lead-delegate` as a bounded resolution
  task.
- `target_held_elsewhere` → the target is checked out in another worktree.
  A merge stop, not a conflict: keep the impl branch and `HEAD`, report the holder's path, end
  the turn; the next invocation merges once it is released.

**By outcome.**

- `completion: phase` → do not merge: it deletes the impl branch the next
  phase stacks on. Leave the checkout on that branch (the route continues
  only from it), and check it back out if a write of yours moved `HEAD`.
  Merge mid-ticket only when a dependent ticket needs the landing, through
  the same gate. Then **End the turn**.
- `completion: ticket` → merge the impl branch. The ticket was an epic's
  last open child → surface the epic for a close decision. Then **End the
  turn**.
- (a) merge into a `main`-class branch → ask the user for approval; on
  approval call `{{.McpNamespace}}/git.merge` with that target.
- (b) unresolved decision:
  - a branch-identity safety stop → judge ownership from cross-ticket
    context or a bounded explore. The unmerged work belongs to the target
    ticket → re-invoke its route with a vouch matching the observed branch
    and ticket; otherwise leave the stop in place.
  - any other → the ticket or its sources already answer `ask` → resume
    the worker with the answer and its source; otherwise put `ask` to the
    user and resume with the answer.
- (c) contract broken → `{{.SkillNamespace}}:lead-ticket` (operation
  `revise`) with `proposed_resolution:`, the tier, and the impl branch;
  `done` → resume the worker; `blocked` → the user.
- (d) irreversible action → put `ask` to the user; resume with the answer.
- (e) Critical open after the fix round → repeat Spawn steps 4–6 with the
  row's retry cell (its body and any `tier_override`), on the same branch,
  adding the report's open Critical `unresolved:` line to the task block.
- (f) non-convergence → the same retry, adding the failing
  `verification:` line instead.
- One retry per ticket run across (e) and (f); the next (e) or (f) goes to
  the user.

Resume through the host's continuation mechanism. It has none, or the
agent is gone → re-spawn with the same task block plus:
`Resume: branch <branch> at <head>; your last report is in the ticket's ### Result
and the branch log.`

## Parallel route (opt-in)

- This route runs the workflow manual's `#### Parallel mode` over ready
  tickets and needs one user approval per run; a user instruction to work
  in worktree or parallel mode is that approval. The approved batch is the concurrency cap.
- Render `ticket-batch-selector` at its recommended tier; it owns the
  parallel-safety read. Present its `batch:` with the approval request, or
  report it and proceed when the instruction already approved.
- Per ticket: provision by the manual's `### Worktree-based work mode`,
  render its worker with `root_override: <worktree path>`, and put the
  acquired branch on the Branch line. A route verdict other than
  `continue` is followed as reported, not re-provisioned.
- Handle each report by **Handle the report** as it arrives.

## End the turn

- A stop that reached the user and is unanswered → a dated
  `## Blocked (YYYY-MM-DD)` note on the ticket, or the next selection
  re-picks it.
- Whatever re-invokes this skill reads the transcript's last line, so make
  it exactly one of these, chosen by the queue state after that note. A
  stop that reached the user, a protocol mismatch, and a dispatch block
  end the same way:

- `Active ticket <stem> has phases remaining on its retained impl branch — next cycle: {{.SkillNamespace}}:lead-run continues it.`
- `Ready queue still has advanceable tickets — next cycle: {{.SkillNamespace}}:lead-run.`
- `Ready queue is empty — prepare a todo or idea ticket with {{.SkillNamespace}}:lead-ticket before re-invoking {{.SkillNamespace}}:lead-run.`

Write nothing after it, and keep `finished`, `complete`, and `done` out of
a continuing turn.
