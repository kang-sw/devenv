---
title: playbook.print of lead skill/manual bodies leaks gated bootstrap surface to subagents
related:
  260625-feat-ws-session-state-machine: surfaced this during Phase 3b workflow_manual hardening
related-mental-model:
  - mcp-runtime
  - prompt-bundle
  - workflow-skills
---

# playbook.print of lead skill/manual bodies leaks gated bootstrap surface to subagents

## Problem

`ws.workflow_manual` was hardened (260625 Phase 3a/3b) so a non-lead caller cannot
obtain lead self-bootstrap (`ws.ferrule`) guidance: a valid `session_key` is
required, a non-descriptive sentinel triggers fresh mode, the tool is lead-only
(`isLeadOnlyTool`), and FAIL-LOUD renders no manual body.

That gating is bypassable through a different tool: **`playbook.print` has no role
gate and no name restriction** (`internal/mcp/server.go` `case "playbook.print"`).
A delegate/leaf-scoped key — or a keyless caller — can render any playbook by
stem, and `printPlaybook` does NOT strip the `ws:fresh-only` gated region (markers
pass through as inert HTML comments, content present). Concretely:

- `playbook.print(name: "lead-workflow-manual")` returns the full manual including
  the gated `ws.ferrule(root: ...) ... mint your lead key` bootstrap line plus the
  always-shown per-root rule that also names `ws.ferrule`.
- `playbook.print(name: "lead-proceed" | "lead-discuss" | "lead-sprint" |
  "lead-salvage")` returns the repointed self-load line, which contains the
  reserved fresh-bootstrap sentinel literal (`freshBootstrapKey`) in clear text.

So both obscured secrets — the `ws.ferrule` escalation call and the
`ws.workflow_manual` fresh-mode sentinel — are readable by any subagent that calls
`playbook.print` with the right stem. `ws.ferrule` keyless mints a `roleLead` key
(`parseCapabilityScope` defaults to lead), so the chain is a full privilege
escalation.

## Threat model / severity

- The current defense is **obscurity**, matching the pre-existing posture: a pure
  MCP-surface subagent does not automatically know stems like `lead-workflow-manual`
  or `lead-proceed`; it must already know the name (e.g. from having read a lead
  skill). The tool schema does not enumerate stems. This is the same soft-gate tier
  as `ws.ferrule`'s deliberately non-descriptive name.
- Accepted (260625 user decision) as **not catastrophic**: piercing it requires
  name knowledge, and the MCP tool name (`ws/workflow_manual`) carries no `lead-`
  prefix cue. Deferred to this ticket rather than expanding the Phase 3b slice.
- Contrast: the FAIL-LOUD path closed in Phase 3b required **no** name knowledge
  (any unregistered key), which is why it was fixed inline.

## Candidate directions (not yet decided)

1. **Role-gate `playbook.print` for lead-scoped stems.** Reject (or strip the gated
   region from) `playbook.print` of `lead-*` skill/manual bodies when the caller's
   `session_key` resolves to a non-lead scope — mirroring the `ws.workflow_manual`
   lead-only gate. Must first confirm no legitimate delegate flow prints lead skill
   bodies (delegates normally receive rendered prompts via `playbook.render`, not
   `playbook.print`). Keyless `playbook.print` is the harder case (no role to gate).
2. **Strip `ws:fresh-only` in `printPlaybook` always.** Makes the backward-compat
   print path hide the gated bootstrap line for everyone; leads use
   `ws.workflow_manual(sentinel)` for fresh. Does NOT close the ungated per-root
   ferrule mention (line ~65) nor the sentinel embedded in repointed skills, so it
   is insufficient alone.
3. **Obscure internal route name for the manual playbook.** Rename the
   `lead-workflow-manual` rsrc stem to a non-descriptive internal route (e.g. a
   hidden token), referenced only via the `ws.workflow_manual` handler, so a
   subagent cannot guess the `playbook.print` stem. Low-cost preventive (user
   suggestion); raises the obscurity bar without a hard gate. Does not address the
   sentinel-in-repointed-skills exposure.

## Open questions

- Do any delegate/leaf flows legitimately call `playbook.print` on lead-scoped
  stems? (Determines whether a hard role-gate is safe.)
- Should the sentinel live somewhere other than clear text in shipped skill bodies
  (which are themselves `playbook.print`-able)?
- Is obscurity an acceptable terminal posture here, or is a hard gate warranted
  given `ws.ferrule` keyless mints a lead key?
