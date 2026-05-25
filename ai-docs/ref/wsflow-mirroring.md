# wsflow Mirroring

## Purpose

`agents-plugin-wsflow/` is a curated derivative of the full
`agents-plugin/` distribution. It is not a generated mirror and it is not a
user-facing ws variant.

Use this reference before editing full ws skills, plugin packaging, runtime
contracts, launcher behavior, prompt guidance, or release validation that could
affect wsflow.

## Required Check

Before editing any `agents-plugin/skills/lead-*` skill:

1. Check whether the skill is in the shipped wsflow skill set.
2. If included, update the matching `agents-plugin-wsflow/skills/lead-*`
   skill in the same logical change, or create a follow-up ticket explaining
   why it cannot be mirrored now.
3. If excluded, check whether wsflow docs, workflow manual text, static
   verification, or the exclusion rationale need updates.
4. Keep wsflow distributed text non-ws-aware. Users should see wsflow names,
   wsflow skill invocations, and wsflow MCP notation.

## Shipped wsflow Skills

Included:

- `lead-workflow-manual`
- `lead-discuss`
- `lead-write-spec`
- `lead-write-ticket`
- `lead-proceed`
- `lead-implement`
- `lead-check-blockers`
- `lead-edit`
- `lead-update-spec`
- `lead-bootstrap`
- `lead-add-rule`
- `lead-ship`
- `lead-sprint`
- `lead-verify-design`
- `lead-verify-discussion`
- `lead-forge-spec`
- `lead-forge-mental-model`
- `lead-review`

Excluded:

- `lead-write-code`
- `lead-write-skeleton`
- `lead-salvage`
- `lead-skill-authoring`

## wsflow Skill Rules

- Use `wsflow:lead-*` for plugin skill invocations.
- Use `wsflow/<tool>` for MCP tool notation.
- Do not mention `ws/`, `ws:`, `ws.`, `subquery`, or `agents.*` in
  distributed wsflow skill text.
- Do not describe wsflow as ws-lite, a ws mode, or a ws-compatible product.
- Describe subagent use by task scope, permissions, expected output, and lead
  integration responsibilities.
- Use subagents for bounded exploration, implementation, verification, audit, or
  review when useful.
- Keep workflow integration lead-owned: docs, ticket/spec changes, mental-model
  updates, commits, and final judgment stay with the lead.

## Bootstrap Template Rules

- Treat `lead-bootstrap` as a mirrored skill: behavior changes require checking
  both `agents-plugin/skills/lead-bootstrap/` and
  `agents-plugin-wsflow/skills/lead-bootstrap/`.
- Keep bootstrap template version histories package-local; matching behavior
  changes may use different version numbers in each package.
- Do not copy the full bootstrap migration backlog into the wsflow template.
- When a bootstrap baseline changes for both packages, update both templates in
  one logical change or record why one package is not applicable.

## Static Verification

The wsflow distributed skill bundle has package tests that fail when shipped
skill files contain forbidden full-ws references, excluded skills, or inventory
drift.

`lead-edit` is the only intentional wsflow-only shipped skill while wsflow keeps
source execution `lead-edit`-mediated. Any additional wsflow-only skill must be
documented here and in the package test before release.

Run:

```bash
python3 -m unittest discover agents-plugin-wsflow/tests
```

This command checks both the runtime contract and the distributed skill bundle.
It makes drift visible; it does not require wsflow skills to be text-identical
to full ws skills.

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
- `lead-salvage`
- `lead-skill-authoring`

Allow exceptions only in repository maintenance documents, tests,
compatibility comments, or hidden implementation details where the full ws name
is the precise implementation surface.

## Doctrine

wsflow mirroring optimizes for **drift visibility without generated sameness**.
The full ws distribution remains canonical, but wsflow is a curated product
with different runtime capabilities. When ambiguous, force an explicit
wsflow review instead of assuming a text-identical mirror is correct.
