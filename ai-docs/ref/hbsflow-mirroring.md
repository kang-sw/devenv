# hbsflow Mirroring

## Purpose

`agents-plugin-hbsflow/` is a curated derivative of the full
`agents-plugin/` distribution. It is not a generated mirror and it is not a
user-facing ws variant.

Use this reference before editing full ws skills, plugin packaging, runtime
contracts, launcher behavior, prompt guidance, or release validation that could
affect hbsflow.

## Required Check

Before editing any `agents-plugin/skills/lead-*` skill:

1. Check whether the skill is in the shipped hbsflow skill set.
2. If included, update the matching `agents-plugin-hbsflow/skills/lead-*`
   skill in the same logical change, or create a follow-up ticket explaining
   why it cannot be mirrored now.
3. If excluded, check whether hbsflow docs, workflow manual text, static
   verification, or the exclusion rationale need updates.
4. Keep hbsflow distributed text non-ws-aware. Users should see hbsflow names,
   hbsflow skill invocations, and hbsflow MCP notation.

## Shipped hbsflow Skills

Included:

- `lead-workflow-manual`
- `lead-discuss`
- `lead-write-spec`
- `lead-write-ticket`
- `lead-proceed`
- `lead-implement`
- `lead-edit`
- `lead-update-spec`
- `lead-bootstrap`
- `lead-add-rule`
- `lead-exit-session`
- `lead-ship`
- `lead-verify-discussion`
- `lead-forge-spec`
- `lead-forge-mental-model`

Excluded:

- `lead-write-code`
- `lead-write-skeleton`
- `lead-sprint`
- `lead-salvage`
- `lead-skill-authoring`

## hbsflow Skill Rules

- Use `hbsflow:lead-*` for plugin skill invocations.
- Use `hbsflow/<tool>` for MCP tool notation.
- Do not mention `ws/`, `ws:`, `ws.`, `subquery`, or `agents.*` in
  distributed hbsflow skill text.
- Do not describe hbsflow as ws-lite, a ws mode, or a ws-compatible product.
- Do not describe persistent hbsflow-managed agents, auto-resume sessions, or
  multi-turn implementer/reviewer relays.
- Use host-native one-shot subagents only for bounded read-only investigation,
  verification, audit, or review.
- Keep workflow mutations lead-owned: edits, docs, ticket/spec changes,
  mental-model updates, and commits are performed by the lead.

## Static Verification

The hbsflow distributed skill bundle should have a verification path that fails
when shipped skill files contain forbidden full-ws references, excluded skills,
or inventory drift.

Forbidden distributed-skill references include:

- `ws/`
- `ws:`
- `ws.`
- `subquery`
- `agents.register`
- `agents.call`
- `agents.result`
- `mental-model-updater`
- `lead-write-code`
- `lead-write-skeleton`
- `lead-sprint`
- `lead-salvage`

Allow exceptions only in repository maintenance documents, tests,
compatibility comments, or hidden implementation details where the full ws name
is the precise implementation surface.

## Doctrine

hbsflow mirroring optimizes for **drift visibility without generated sameness**.
The full ws distribution remains canonical, but hbsflow is a curated product
with different runtime capabilities. When ambiguous, force an explicit
hbsflow review instead of assuming a text-identical mirror is correct.
