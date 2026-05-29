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
- Feed playbook delegate prompts to native subagents only through
  `wsflow/prompt.render`; never hand-paste full playbook prompt text.

## Prompt Render Dispatch

`wsflow/prompt.render(stem, context)` is the wsflow-only mechanism for handing a
bundled delegate prompt to a native subagent. It loads the prompt by stem,
applies render-time `ws/` -> `wsflow/` namespace substitution, injects `context`
values, writes the result to a tmp file, and returns `prompt_path`. The lead
hands `prompt_path` to a native subagent.

- Render-eligible prompt stems: `project-survey`, `plan-populator-survey`,
  `plan-populator-research`, `code-reviewer`, `mental-model-updater`. These bare
  stems are not full-ws references and may appear in distributed wsflow skill
  text.
- File-writing prompts (`plan-populator-*`, `mental-model-updater`) receive a
  caller-created output path in `context`; free-response prompts
  (`project-survey`, `code-reviewer`) return text. `prompt.render` does not mint
  an `expected_output_path`.
- The `implementer` prompt is not render-eligible in wsflow.

`prompt.render` is a wsflow-only tool: it is advertised and callable only in the
wsflow product mode and is hidden from the full ws surface. This is the mirror of
the agentless hidden-tool gate (full ws hides `agents.*`, `subquery`, and other
agent-backed tools in wsflow). The two gates are symmetric and must stay so.

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

wsflow ships no wsflow-only skills: `lead-implement` is the converged unified
spine and absorbs the former `lead-edit` primitive. The only wsflow-only runtime
surface is the `prompt.render` MCP tool (see **Prompt Render Dispatch**). Any new
wsflow-only skill or tool must be documented here and in the package test before
release.

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
