---
kind: print
delegates: true
variables:
  - ExploreAgent
  - SpawnIdiom
---

# Run

You are the lead. You pick one unit of work, spawn one worker to execute it,
wait for its terminal report, handle its stops, and end the turn with a
verdict line. You do not read the implementation playbooks and you do not edit
source; a worker holding a lead-capability key does that. Your context is not
the constraint here; the user's attention at each stop is.

## Select

Spawn {{.ExploreAgent}} to pick the next ticket; do not list `ready/` or read
ticket files yourself. It skips candidates carrying a `## Blocked (...)` note,
then prefers, in order: a ticket already in progress (some phase has a
`### Result`, at least one does not); one named as a prerequisite by another
`ready/` ticket's `related:` or `parent:`; otherwise the oldest. It returns
exactly one advanceable ticket path, or `ready/` empty, or every remaining
ticket blocked. Empty and all-blocked end the turn with no spawn; on a
`goal/*` branch each has its own terminal below.

`run <description>` skips selection: the description is the contract.

## Spawn

1. Stage a goal branch only when a `/goal` reminder is active and the branch
   is not already `goal/*`: capture the current branch as PARENT
   (`git rev-parse --abbrev-ref HEAD`), then `git checkout -b
   goal/<parent>/<slug>` with a random word-word-word slug, never derived
   from the goal text (which collides across concurrent runs of the same
   command). On detached `HEAD`, dispatch unstaged and say so.
2. `{{.McpNamespace}}/playbook.render(name: "ticket-worker", session_key:
   <your key>)`. It returns a path with the worker's lead-capability key
   spliced in. Do not read the file, and do not mint a second key for this
   worker.
3. Spawn one worker of at least current-mainstream class, in a form that can
   itself spawn children (`{{.SpawnIdiom}}`), with this task block and nothing
   else:

   ```text
   Read <rendered-path> as your system prompt.
   Ticket: <ticket path> (stem <stem>). Branch: <current branch>.
   ```

   Ad hoc: replace the `Ticket:` line with `Contract:` followed by the user's
   description verbatim. Never paraphrase either.
4. Record the assignment: `{{.McpNamespace}}/session.note(session_key: <your
   key>, child_session_key: <worker key>, text: "<stem>: dispatched
   <host agent id>")`. This is the carry-over record a compacted or restarted
   lead rebuilds from (`{{.McpNamespace}}/session.children`); it is not a
   progress board. Advance it to `merged` or `blocked` when the ticket
   resolves.
5. Wait for the host's completion notification. Do not poll, do not block in
   a tool call, and do not touch the ticket or the branch meanwhile.

One worker in flight per invocation.

## Handle the report

The worker ends with a fixed block. `stop: none` means the ticket is closed on
its branch: advance the note and go to **End the turn**. Otherwise act by stop
letter. Carry lines from the worker's report to the user verbatim; do not
re-summarize them.

- **(a) parent merge** — this is the run's terminal; see below.
- **(b) unresolved decision** — read what the worker points at (the ticket,
  the graph tickets, the delegate's result). If they settle it, resume the
  worker with the answer and where you found it. If not, put the one question
  to the user and resume with the answer.
- **(c) contract broken** — do not go to the user first. Route the worker's
  `proposed_resolution:` through `{{.SkillNamespace}}:lead-write-ticket` as an
  `#### Edition` on the executed phase, under the design-review gate at a
  raised tier. A `pass` commits the edition and resumes the worker; a `block`
  goes to the user with the reviewer's verdict.
- **(d) irreversible action** — put the report's lines to the user; resume
  with the answer.
- **(e) Critical still open after the fix round** — re-spawn the ticket on a
  higher-tier worker, same branch, with the finding's location (commit and
  file) in the task block. A second (e) from the elevated worker goes to the
  user.

Resume through the host's continuation mechanism with the agent id from the
note. When the host has none, or the id is gone, re-spawn with the same task
block plus one line: `Resume: branch <branch> at <head>; your last report is
in the ticket's ### Result and the branch log.` The worker's inputs are
pointers, so nothing is lost.

## Terminal: `ready/` empty on a goal branch

PARENT is everything between `goal/` and the last `/`; a single-segment
`goal/<slug>` falls back to `main`. Assemble the merge-stop report from this
run's worker reports: every `decisions:` and `unresolved:` line, by ticket,
verbatim. Put it to the user and ask for explicit approval to merge into
PARENT; on approval merge yourself with plain `git merge --no-ff` under the
repository's commit rules. Never push.

## Terminal: every remaining ticket blocked on a goal branch

Report the recorded blockers and end the run without merging. Work behind a
single pending sign-off is a pause, not this terminal.

## End the turn

Before yielding, record any stop that reached the user and was not answered as
a dated `## Blocked (YYYY-MM-DD)` note on the ticket, or the next selection
re-picks it.

Whatever re-invokes this skill judges from the transcript's last line, so make
it exactly one of:

- `Ready queue still has advanceable tickets — next cycle: {{.SkillNamespace}}:lead-run.`
- `Goal run finished — <reason>. Do not re-invoke {{.SkillNamespace}}:lead-run.`

Write nothing after it, and keep `finished`, `complete`, and `done` out of a
continuing turn.
