---
title: "Add lead-delegate as the session-local arbitrary executor and retire lead-prefer-subagent"
sage-review-design: completed
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260605-research-ws-native-subagent-pivot: native-subagent continuity and host-owned lifecycle anchor
sage-review-design-reviewed: b1ff2211cc19899f
sage-review-completeness: completed
sage-review-completeness-reviewed: b1ff2211cc19899f
completed: 2026-09-10
---

# Add lead-delegate as the session-local arbitrary executor and retire lead-prefer-subagent

## Background

`lead-run` now gives an implementation worker an end-to-end contract: explore,
route, edit, verify, review, record the ticket result, and commit. Its ad-hoc
path still uses that implementation procedure and fixed stop protocol, so it is
too specialized for a lead that wants to hand an arbitrary bounded task to one
native subagent, sustain that agent across exchanges, and choose its permissions
case by case.

`lead-prefer-subagent` overlaps the missing capability but owns the wrong axis.
It is a broad maximum-delegation posture, with fresh-spawn, continuation,
prompt-shaping, and return-format rules embedded in the skill. After the
refoundation, the main workflows already delegate their own payloads. The useful
remaining posture is only whether eligible general work defaults to delegation;
the lifecycle and execution rules belong to the new explicit delegate entry.

## Decisions

- **Add `lead-delegate` as a separate user entry.** It starts or continues one
  session-local native-subagent task from an arbitrary bounded prompt. The lead
  remains the mediator, chooses the model, tools, permissions, and ws capability
  scope, and may steer the same host task across turns.
- **Session-local continuity is sufficient.** Preserve a label, host task id,
  purpose, and active/released state through the current lead session and its
  compactions. A missing host handle degrades to a fresh spawn with a concise
  resume brief. No cross-session registry or durable conversation store is
  introduced.
- **Low-impact mutation is allowed.** `lead-delegate` is not read-only. The lead
  may authorize bounded, reversible, self-verifying chores, mechanical updates,
  and localized internal hotfixes. Category names such as `chore` or `bug` do
  not decide the route; impact, uncertainty, and review need do.
- **Material implementation reroutes before execution.** A ready ticket or work
  with material behavior/contract impact, cross-module or unclear scope,
  independent-review need, or unresolved design judgment goes through
  `lead-run`. If a delegate discovers that boundary after spawn, it stops before
  the expanded mutation and returns evidence plus an implementation contract.
- **Existing workflow ownership remains.** Ticket mutation routes to
  `lead-ticket`, independent review or merge adjudication to `lead-review`, and
  release/publish/deploy to `lead-ship`. Chat-only drafting may stay in
  `lead-delegate`; writing the draft into a tracked project artifact follows its
  owning workflow unless it qualifies for the bounded low-impact path above.
- **Retire the `lead-prefer-subagent` skill.** The user explicitly approved its
  deletion. Move its same-work-item continuation and fresh-spawn rules into
  `lead-delegate`; replace its durable-artifact whitelist with the owning-
  workflow/reroute gate below. Keep the `workflow.prefer_subagent` setting as a
  tuning posture whose only effect is to make eligible general work default to
  `lead-delegate`; it no longer loads a second execution procedure.
- **Keep lifecycle ownership host-native.** ws may record the lead-supplied task
  label and host id for recovery, but does not claim native status, cancellation,
  or continuation semantics. Use the host continuation mechanism for the same
  task and host-native interruption when requested.

## Exact Required Prose

Copy the following descriptions verbatim into the corresponding skill
frontmatter. Do not paraphrase or independently evolve one copy.

```yaml
lead-discuss:
  Use when the user wants to reason with the lead about direction, scope,
  risk, or trade-offs before capture or execution. The lead owns the
  conversation and decision-making; subagents may gather evidence only.
  Conversation only.

lead-delegate:
  Start or continue a session-local native subagent when the user wants a
  bounded task handed to an executor and may steer that same agent across
  turns. Use for investigation, diagnosis, drafting a requested deliverable,
  operational chores, and eligible low-impact changes. The lead chooses the
  prompt, model, tools, and permissions. Route collaborative direction-setting
  to lead-discuss and material implementation to lead-run.

lead-run:
  Drain the ready queue one ticket at a time, or execute implementation whose
  scope, behavioral impact, or review needs warrant the full worker workflow.
  The worker explores, implements, verifies, runs independent review, records
  the result, and commits.
```

Install the following procedure verbatim as the rendered `lead-delegate`
playbook. The implementation may substitute the configured skill namespace,
but must not paraphrase or independently evolve the prose.

```markdown
# Delegate

You are the lead managing a session-local native executor. Turn the user's
request into a bounded assignment, choose the executor's prompt, model,
tools, and permissions, and carry the work through follow-up exchanges.

## Routing

Route collaborative direction-setting to {{.SkillNamespace}}:lead-discuss.
Ticket authoring and status changes belong to {{.SkillNamespace}}:lead-ticket;
standalone review and merge adjudication to {{.SkillNamespace}}:lead-review;
release execution to {{.SkillNamespace}}:lead-ship.

For other work, apply this gate before dispatch and when scope changes.

Use lead-run when any of these is true:

- a ready ticket owns the work;
- the change affects public behavior, an API, protocol, schema, template,
  canonical flow, or architecture;
- the implementation scope is unclear or crosses module responsibilities;
- independent review is needed;
- the task contains an unresolved product or workflow decision.

Otherwise the lead may use lead-delegate for a bounded, reversible,
self-verifying task, including repository mutations such as housekeeping,
mechanical updates, and localized internal hotfixes.

If the delegate discovers broader scope or material impact, it stops before
that expanded change and returns an implementation contract for lead-run.

Invoke {{.SkillNamespace}}:lead-run for that handoff.

## Assignment

Give the executor the intended outcome, relevant input paths, permitted
actions, verification appropriate to the task, and the boundary at which
it should return to the lead. Choose these for the assignment; there is no
fixed executor role or read-only default. Permissions stay within existing
user authorization and the host's available capabilities.

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
```

Replace the opening prose of the rendered `lead-discuss` playbook with the
following text verbatim. Keep its Evidence, Conversation, Stops, and Output
sections unchanged.

```markdown
You are the lead in conversation. Reason with the user about direction, scope,
risk, and trade-offs before capture or execution. You own the conversation and
decision-making; subagents may gather evidence only. Edit no source and write
no document here.

What the user confirms is captured through
`{{.SkillNamespace}}:lead-ticket`. When the user moves to execution, invoke
`{{.SkillNamespace}}:lead-delegate` for a bounded task or
`{{.SkillNamespace}}:lead-run` for implementation that warrants the full
worker workflow.
```

Replace the opening prose of the rendered `lead-run` playbook with the
following text verbatim.

```markdown
You are the lead for the full worker workflow. You drain the ready queue one
ticket at a time, or accept an ad-hoc implementation contract whose scope,
behavioral impact, or review needs warrant that workflow. You select one unit
of work, spawn one worker to execute it, wait for its terminal report, handle
its stops, and end the turn with a verdict line. You do not edit source; the
worker owns implementation and verification.
```

Replace the ad-hoc paragraph under `lead-run`'s Select section with the
following text verbatim. Keep the rest of the procedure unchanged, including
the rendered-playbook read prohibition in Spawn.

```markdown
An ad-hoc implementation contract passed with the invocation skips selection.
This path is for implementation whose scope, behavioral impact, or review needs
warrant the full worker workflow. The description is the contract.
```

An explicit `lead-run` invocation remains authoritative; do not add a reverse
reroute from `lead-run` to `lead-delegate`. General-request attention comes
from the three skill descriptions, while only `lead-delegate` escalates
material implementation to `lead-run`.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Preserve host-neutral terminology and native host ownership of spawn,
  continuation, interruption, and task handles.
- Do not route full implementation through the free-form delegate merely
  because the lead can grant it broad permissions.
- Regenerate mirrors and manifests through their documented generators; do not
  hand-maintain generated copies.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/, agents-plugin-wsflow/, and agents-plugin-tool/ are named in Phase 1 |
| scope.surface | public-interface | Phase 1 adds lead-delegate to the user-invoked lead skill inventory alongside lead-run |
| scope.new_public_symbol | yes | lead-delegate |
| scope.new_type_contract | unknown | the ticket requires session-local label and host-handle recovery but names no type or signature |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/workflow_prefer_subagent_test.go, agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go, agents-plugin/tests/test_skill_dispatch_contracts.py, and agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py |
| complexity.reuse_points | confirmed | agents-plugin/skills/lead-prefer-subagent/SKILL.md#L12-L14 contains the fresh-spawn and same-work-item continuation rules |
| complexity.side_effect_risk | moderate | the entry changes delegation routing and can authorize bounded repository mutations |
| risk.correctness | high | continuation recovery and reroute boundaries determine whether material work is executed through lead-run |
| risk.fit | high | the change alters the public lead-skill inventory and delegation posture |
| risk.test | moderate | existing preference, manual-render, mirror, and manifest coverage require coordinated updates |
| risk.security_or_contract | moderate | lead-selected tools and permissions require the procedure to preserve host-native ownership |

## Phases

### Phase 1: Add the sustained delegate entry and retire the posture skill

Add the thin `lead-delegate` entry skill and install the Exact Required Prose
as its lead-facing procedure. Treat that prose as settled behavior rather than
a design prompt; implementation is limited to installing, wiring, and
verifying it.

Apply every frontmatter description, complete procedure, and replacement block
under Exact Required Prose verbatim. Implement the owning-workflow checks and
the exact reroute gate before spawn and again when the delegate reports scope
growth. The free-form path has no ticket-worker route/edit/review/commit
protocol.

Retire `lead-prefer-subagent` from the ws and wsflow skill inventories and
remove its standalone inline procedure, mirror exceptions, manifest entries,
tests, and loader hooks. Preserve `workflow.prefer_subagent` as a tuning option
that selects `lead-delegate` by default only for tasks eligible under the gate.
Move the proven fresh-spawn versus same-work-item continuation rule into the
new procedure. Update the refoundation skill inventory and canonical workflow
orientation to include `lead-delegate` without presenting it as an
implementation shortcut.

Verify explicit skill invocation, implicit trigger selection, session-local
spawn/continuation/recovery, lead-selected permission posture, every reroute
condition, scope-growth escalation, prefer-subagent tuning behavior, retired
skill absence, generated inventories, ws/wsflow mirroring, and existing
lead-run ticket execution.

### Result (84ec42c2) - 2026-09-10

Installed the exact delegate procedure and three entry descriptions in ws and
wsflow, with the specified discuss/run replacements. Both packages expose the
thin delegate entry and retire the inline posture skill. The global preference
now adds only a namespace-aware eligibility-scoped invocation hint; it no longer
loads a skill body. Updated project execution orientation and mirroring guidance;
the parent epic already declares the working six, so its settled inventory
needed no further edit. Generated manifests and rsrc mirrors through the
prescribed generators.

Verification:

- `TMPDIR=/private/tmp go test ./...` in `agents-plugin-tool`: all packages passed.
- `TMPDIR=/private/tmp scripts/smoke-ws-mcp.sh ..`: passed.
- `python3 -m unittest discover agents-plugin/tests`: 56 passed.
- `python3 -m unittest discover agents-plugin-wsflow/tests`: 10 passed.
- Final inventory-comment adjustment: focused skill-dispatch suite, 8 passed.
- Exact-prose fixture and real MCP entry tests cover both product namespaces,
  all authored routing/continuity/permission boundaries, retired entry absence,
  preference toggling without a skills root, and existing run dispatch.
- Correctness, fit, and test reviews were clean. Fresh-reader audit was clean;
  exact-prose authority accepts the possible cost of routing a small investigation
  attached to a ready ticket or unresolved implementation decision through run.

Decisions: keep all executor state and lifecycle semantics host-owned, with no
new registry or server executor contract. Repair the pre-existing Python run
assertion that still expected a fixed worker name; retain the selected-worker
behavior already present at the branch base. Update the discussion golden to
its authorized replacement opening. The initial full-suite absolute-path failure
matches the existing `260910-bug-route-ticket-absolute-path-symlink-alias` follow-up;
canonical TMPDIR passed without broadening this implementation.

Omitted: no empirical host implicit-trigger selection, native lifecycle,
compaction recovery, permission enforcement, or live ticket execution was run.
The checks establish the authored procedure and MCP integration, not host
judgment. Installed-cache verification was not performed; this change does not
alter the launcher or MCP configuration. No source implementation scope remains.
