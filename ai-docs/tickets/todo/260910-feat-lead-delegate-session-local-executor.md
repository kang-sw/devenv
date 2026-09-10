---
title: "Add lead-delegate as the session-local arbitrary executor and retire lead-prefer-subagent"
sage-review-design: completed
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260605-research-ws-native-subagent-pivot: native-subagent continuity and host-owned lifecycle anchor
sage-review-design-reviewed: d961114899f52e22
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

Install the following routing text verbatim in the lead-delegate procedure.
The owning-workflow checks for `lead-ticket`, `lead-review`, and `lead-ship`
run before this block.

```text
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
```

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for
  agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/,
  agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for
  agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for
  agents-plugin-tool/internal/mcp/)
- Preserve host-neutral terminology and native host ownership of spawn,
  continuation, interruption, and task handles.
- Do not route full implementation through the free-form delegate merely
  because the lead can grant it broad permissions.
- Regenerate mirrors and manifests through their documented generators; do not
  hand-maintain generated copies.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/, agents-plugin-wsflow/, and agents-plugin-tool/ are named by the phase and constraints |
| scope.surface | public-interface | lead-delegate is a new user-invoked entry skill alongside lead-run |
| scope.new_public_symbol | yes | lead-delegate |
| scope.new_type_contract | unknown | the ticket requires session-local label and host-handle recovery but names no type or signature |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/workflow_prefer_subagent_test.go and shipped manifest tests cover the retired setting and generated surfaces |
| complexity.reuse_points | confirmed | agents-plugin/skills/lead-prefer-subagent/SKILL.md contains the existing fresh-spawn and same-work-item continuation rules |
| complexity.side_effect_risk | moderate | the entry changes delegation routing and can authorize bounded repository mutations |
| risk.correctness | high | continuation recovery and reroute boundaries determine whether material work is executed through lead-run |
| risk.fit | high | the change alters the public lead-skill inventory and delegation posture |
| risk.test | moderate | existing preference, manual-render, mirror, and manifest coverage require coordinated updates |
| risk.security_or_contract | moderate | lead-selected tools and permissions require the procedure to preserve host-native ownership |

## Phases

### Phase 1: Add the sustained delegate entry and retire the posture skill

Add the thin `lead-delegate` entry skill and its lead-facing procedure. The
procedure accepts an arbitrary bounded prompt, creates a session-local label,
spawns a native subagent with lead-selected model/tools/permissions, records
the host task handle for compaction recovery, resumes the same task for
follow-up messages, and releases it on user or lead direction. When the handle
is unavailable, create a fresh agent from the original purpose, last material
result, and next instruction.

Apply the Exact Required Prose to `lead-discuss`, `lead-delegate`, and
`lead-run`. Implement the owning-workflow checks and the exact reroute gate
before spawn and again when the delegate reports scope growth. The free-form
path has no ticket-worker route/edit/review/commit protocol and no mandatory
worker terminal block; its response shape serves the arbitrary task.

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
