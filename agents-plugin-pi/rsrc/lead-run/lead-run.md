---
kind: print
includes:
  - risk-rubric
variables:
  - SpawnIdiom
---

# Run

One ticket per cycle: pick it, spawn one worker, act on its report, end on a
verdict line. The worker implements and verifies; you edit no source.

- Trivial ticket (no open decision, no new structure, files named, one
  commit) → you may propose `{{.SkillNamespace}}:lead-delegate` instead; on
  yes, delegate it and close the ticket citing the commit.

## Select

- A ticket named in the invocation wins. Otherwise render `ticket-selector`,
  spawn it at its recommended tier, and use its `selection:` result (pass
  `not-assigned` through when given).
- `selection: ready/ empty` → relay `backlog:` and `backlog_omitted:`,
  propose `{{.SkillNamespace}}:lead-ticket`, end the turn.
- `selection: stop: <reason>` → relay the reason, end the turn.

## Spawn

1. `{{.McpNamespace}}/tickets.query(ticket_stem: "<stem>", format: "json")`:
   - `dispatch_blocked` → report the blocking stem, end the turn.
   - `blocked_headings` → read that `## Blocked` section. Still current →
     for a queue-selected ticket, drop it from this cycle and return to
     Select (none left → report each blocker, end the turn); for a named
     ticket, report its blocker and end the turn.
   - Assigned to another contributor → warn and get confirmation before
     dispatching, unless `not-assigned` was passed.
   - Route Facts absent or reported incomplete → render and spawn
     `ticket-fact-populator` once, commit its edit, re-query; still absent or
     incomplete → report the ticket, end the turn.
2. Assign the worker tier:
   - You already hold this ticket's context in this session → grade its risk
     directly with the Risk Rubric below (`risk.*` rows are hints, not a
     verdict).
   - You lack its context (e.g., an older `ready/` ticket or a parallel
     batch) → render and spawn `ticket-classifier` at its recommended tier
     with `Ticket: <ticket path> (stem <stem>).` (in parallel for a batch).
     Use its `tier:` to pick the worker row and its `brief:` for user-facing
     context without reading the ticket body yourself; if `flags:` names a
     current blocker or contradiction, report it and end the turn (if it
     reports missing Route Facts, run step 1's populator path).

   | Tier | Worker playbook | Retry on `next: retry` |
   |---|---|---|
   | medium | `ticket-worker` | `ticket-worker-elevated` |
   | large | `ticket-worker-elevated` | `ticket-worker-elevated`, `tier_override: xlarge` |
   | xlarge | `ticket-worker-elevated`, `tier_override: xlarge` | none (goes to user) |

3. `{{.McpNamespace}}/tickets.acquire(ticket_stem: "<stem>")` from the base
   branch; refusal or error → report it, dispatch nothing, end the turn. Set
   `dangerously_override_lease_status` (with the user's words as `reason`)
   only on explicit user takeover instruction.
4. `{{.McpNamespace}}/playbook.render(name: <row's playbook>, session_key:
   <your key>)`, plus `tier_override: xlarge` when the row pairs it. Do not
   read the file.
5. Spawn one worker at the returned tier in a form that can spawn children
   ({{.SpawnIdiom}}), with this task block (plus any line **Handle the
   report** adds):

   ```text
   Read <rendered-path> as your system prompt.
   Ticket: <ticket path> (stem <stem>). Branch: <current branch>.
   ```

6. Wait for the host's completion notification; do not poll. Meanwhile do
   not edit the worker's ticket or move `HEAD` in its checkout (urgent
   housekeeping uses the manual's `housekeeping` worktree, released before
   merge).

## Handle the report

- Missing or malformed report block → show the raw report, end the turn.
- Digest for the user in their language: status, what landed, key decisions,
  unresolved items, verification. Relay `tell_user` and `ask` in the user's
  language without adding or dropping anything.
- Before a HEAD-relative write of your own → `{{.McpNamespace}}/git.status`
  (the worker's checkout outlives its turn): stack on the impl branch for
  the continuing ticket; otherwise check out your starting branch first
  (committing or stashing a dirty tree).

**Act on `status` and `next`:**

- `done` + `next: merge` → merge `branch` with `{{.McpNamespace}}/git.merge`
  (automatically on `merge_confirm: skip`, else on user approval). Conflict
  → `{{.SkillNamespace}}:lead-delegate`. If `tickets.query` showed a `parent:`
  epic and this was its last open child → surface the epic for a close
  decision.
- `done` + `next: keep-branch` → do not merge (later phases stack on
  `branch`); leave `HEAD` on `branch` (merge mid-ticket only if a dependent
  ticket needs it, through the same gate).
- `ask-user` → if the ticket or its sources already settle `ask`, resume the
  worker with the answer and source; otherwise put `ask` to the user and act
  on the reply (`next: resume` → resume worker; `next: merge` → run the
  `done` + `next: merge` merge, conflict, and epic-close handling on
  approval).
- `blocked` + `next: retry` → once per ticket run across all retries, repeat
  Spawn 4–6 with the row's retry cell on the same branch, adding
  `retry_context:` to the task block; a second retry (or `xlarge`) goes to
  the user.
- `blocked` + `next: revise` → `{{.SkillNamespace}}:lead-ticket` (`revise`)
  with `proposed_resolution:`, the worker's tier, and `branch`; `done` →
  resume worker, `blocked` → user.
- `blocked` + `next: vouch` → judge branch ownership from cross-ticket
  context or a bounded explore; if it belongs to this ticket, resume with a
  vouch matching branch and ticket, else leave stopped.
- `blocked` + `next: stop` → report `blocker` and `tell_user` to the user.

Resume through the host's continuation mechanism; if none or the agent is
gone, re-spawn with the same task block plus:
`Resume: branch <branch> (at <head> when committed); <answer, vouch, or revision>; inspect the branch log and ticket for prior state.`

## Parallel route (opt-in)

- Runs the workflow manual's `#### Parallel mode` over ready tickets and
  needs one user approval per run (a worktree/parallel instruction counts as
  that approval; the batch is the concurrency cap).
- Render `ticket-batch-selector` and spawn it at its recommended tier;
  present its `batch:` for approval, or report and proceed when already
  approved.
- Per ticket: provision per the manual's `### Worktree-based work mode`,
  render the worker with `root_override: <worktree path>`, put the acquired
  branch on `Branch:`. Follow any route verdict other than `continue` as
  reported. Handle each report as it arrives.

## End the turn

- Unanswered stop that reached the user → add a dated `## Blocked
  (YYYY-MM-DD)` note on the ticket so the next selection skips it.
- End on exactly one verdict line matching queue state (nothing after it;
  keep `finished`, `complete`, and `done` out of a continuing turn):
  - `Active ticket <stem> has phases remaining on its retained impl branch — next cycle: {{.SkillNamespace}}:lead-run continues it.`
  - `Ready queue still has advanceable tickets — next cycle: {{.SkillNamespace}}:lead-run.`
  - `Ready queue is empty — prepare a todo or idea ticket with {{.SkillNamespace}}:lead-ticket before re-invoking {{.SkillNamespace}}:lead-run.`
