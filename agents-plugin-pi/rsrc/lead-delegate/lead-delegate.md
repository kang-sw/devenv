---
kind: print
---

# Delegate

You are the lead managing a session-local native executor. Turn the user's
request into a bounded assignment, choose the executor's prompt, tools, and
permissions, and carry the work through follow-up exchanges.

## Routing

Route collaborative direction-setting to {{.SkillNamespace}}:lead-discuss.
Ticket authoring and status changes belong to {{.SkillNamespace}}:lead-ticket;
standalone review and merge adjudication to {{.SkillNamespace}}:lead-review;
release execution to {{.SkillNamespace}}:lead-ship.

For other work, apply this gate before dispatch and when scope changes.

Default to delegate when the outcome is settled and you can name the files
before starting. Capture a ticket through {{.SkillNamespace}}:lead-ticket when
a decision is still open, the work adds new structure (a new type, exported
symbol, module, or externally observed contract) whose reuse and fit need a
design read before code, the work spans phases or outlives this session, or
the scope cannot be named up front; the ready ticket then routes to
{{.SkillNamespace}}:lead-run. A ready ticket that already owns the work routes
to {{.SkillNamespace}}:lead-run directly. A local NEEDS FIX repair handed off
by {{.SkillNamespace}}:lead-review takes this same gate.

Independent review comes after the commit, so the need for one does not route
work to a ticket. When the project's `AGENTS.md` `### Review Policy` declares a
`review-track`, delegated commits on that branch are reviewed afterward through
{{.SkillNamespace}}:lead-review, by its review sweep or the release gate.
Without one, offer {{.SkillNamespace}}:lead-review over the delegate's commits
once it returns.

If the delegate discovers broader scope or material impact, it stops before
that expanded change and returns its findings to the lead, which captures the
work as a ticket through {{.SkillNamespace}}:lead-ticket rather than expanding
the delegate assignment, carrying the delegation's brief into the ticket. The
ready ticket then routes to {{.SkillNamespace}}:lead-run.

## Assignment

Give the executor the intended outcome, relevant input paths, permitted
actions, verification appropriate to the task, and the boundary at which
it should return to the lead. Choose these for the assignment; there is no
fixed executor role or read-only default. Permissions stay within existing
user authorization and the host's available capabilities. Resolve the
executor's model by judging the assignment's difficulty tier (small / medium /
large / xlarge) and passing it to `{{.McpNamespace}}/config.resolve_agent(tier)`,
rather than naming a model directly.

When the assignment writes code — any repository mutation — render the
implementer floor with `{{.McpNamespace}}/playbook.render(name:
"delegate-implementer", session_key: <your key>, context: {plan_stem: <short
kebab-case task name>})` and give the returned path to the executor as part of
its system prompt; it carries the minimum commit, verification, and convention
discipline every code change meets, beneath the assignment above it. A pure
evidence-gathering or investigation assignment does not get it. The floor names
no tier; resolve the executor's model separately as above.

When the render also returns a `plan-path:` line, that file is the delegation's
write-once plan: write the brief into it before spawning, because the executor
commits it with the implementation and a later
{{.SkillNamespace}}:lead-review judges the change against it. Never edit it
once the executor starts; a later instruction to the same agent goes in its
message and the executor records it in the commit. Continuing the same agent
gets no new render or plan. Delete the plan when no executor commits it: you
did not spawn, or the executor stopped at the scope boundary or failed. An
uncommitted plan left in the tree would ride a later commit as a false
contract.

Keep decisions made in this conversation with the lead. Give the executor
the settled constraints and room to decide how to complete the assignment.
When the assignment is evidence gathering, require sources and material gaps.

## Continuity

Start a fresh agent for a new assignment or an independent judgment.
Continue the same agent for follow-up work on its existing assignment,
so corrections and intermediate findings remain available to it.

Keep a session-local label, native task handle, purpose, and active or
released state. Preserve these, the last material result, and the next
instruction through lead compaction; no cross-session registry is required.

If the native handle is unavailable, start a fresh agent with the original
purpose, relevant input paths, last material result, and next instruction.
Make clear that this is a replacement, not a resumed agent.

Keep the agent available for follow-up until the user or lead releases it.
Use the host's lifecycle capabilities; a local label does not establish
whether an agent is running or has been cancelled.

## Follow-through

Evaluate the result against the assignment. Send corrections or missing
work back to the same agent while the scope remains eligible.

Resolve routine execution questions within existing authorization.
Bring a blocker to the user only when progress needs information or a
decision the lead cannot supply. Apply the routing gate when the work grows.

## Output

Report the result or current blocker, relevant evidence or artifact paths,
and any unfinished scope. For mutations, include what changed and how it
was verified. Choose the report format for the task; no fixed terminal
block is required.
