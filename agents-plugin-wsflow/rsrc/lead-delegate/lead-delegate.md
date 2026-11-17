---
kind: print
---

# Delegate

Hand a bounded task to a native executor and carry it through follow-ups.

## Gate

Apply before dispatch, and again when scope changes.

- Outcome settled and files nameable up front → delegate.
- Any of these → capture a ticket through {{.SkillNamespace}}:lead-ticket,
  then {{.SkillNamespace}}:lead-run:
  - a decision is still open
  - new structure: a type, exported symbol, module, or external contract
  - the work spans phases or outlives this session
  - the scope cannot be named up front
- A ticket already owns the work → {{.SkillNamespace}}:lead-run.
  Exception: a trivial change (no open decision, no new structure, files
  named in the ticket, one commit) → propose delegating it instead; delegate
  on the user's yes, then close the ticket with
  `{{.McpNamespace}}/tickets.close`, citing the commit.
- A local NEEDS FIX repair from {{.SkillNamespace}}:lead-review → this gate.
- Needing review is no reason for a ticket: review follows the commit. With a
  `review-track` in the project's `AGENTS.md` `### Review Policy`, its sweep
  covers the commits; otherwise offer {{.SkillNamespace}}:lead-review once
  the executor returns.

## Assignment

- Give: outcome, input paths, permitted actions within the user's existing
  authorization, verification, and the boundary at which to return.
- Give settled constraints; keep this conversation's decisions with you.
- Tier: the workflow manual's tier rule.
- Render the executor's floor with `{{.McpNamespace}}/playbook.render(name,
  session_key: <your key>)` and give it the returned path:
  - repository mutation → `delegate-implementer`, with `context: {plan_stem:
    <short kebab-case task name>}`; when the executor commits in another
    checkout, also pass that checkout as `root_override`
  - anything else → `delegate-report`; for evidence gathering, require
    sources and material gaps in the assignment
- The render returned a `plan-path:` line → write the brief into that file
  before spawning. Never edit it after; later instructions go in messages.
  No executor commits it (not spawned, stopped, failed) → delete it.

## Continuity

- New assignment or independent judgment → fresh agent.
- Follow-up on the same assignment → same agent.
- Keep label, handle, purpose, state, last result, and next instruction
  through compaction.
- Handle lost → fresh agent with those, stated as a replacement.

## Return

The executor ends with the `delegate-report` block.

- `done` → check `result` against the assignment; corrections go to the
  same agent.
- `blocked` or `ask-user` → relay `tell_user` and `ask` to the user; send
  the answer to the same agent.
- Broader scope reported → capture a ticket carrying the brief; never widen
  the assignment.
