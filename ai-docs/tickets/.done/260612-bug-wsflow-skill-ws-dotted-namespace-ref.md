---
title: wsflow lead-workflow-manual skill references forbidden `ws.` dotted namespace (wsflow/ws.lead.login)
related:
  260611-refactor-ws-tier-taxonomy-delegate-tier-routing: surfaced while running the wsflow package tests during Phase 6
completed: 2026-06-15
---

# wsflow lead-workflow-manual skill references forbidden `ws.` dotted namespace

## Background

Dogfound 2026-06-12 while running `python3 -m unittest discover
agents-plugin-wsflow/tests` during `260611` Phase 6. The wsflow skill-bundle
forbidden-reference test (`test_wsflow_skill_bundle.py`, pattern
`"full ws dotted namespace": re.compile(r"\bws\.")`) fails on:

```
skills/lead-workflow-manual/SKILL.md: full ws dotted namespace
```

Offending lines (`agents-plugin-wsflow/skills/lead-workflow-manual/SKILL.md`):

- L49: `` `wsflow/ws.lead.login` ``
- L55: ``call `wsflow/ws.lead.login(root: "<absolute-working-directory>")` ``

## Findings

- Pre-existing, not a Phase 6 regression: the lines were introduced in
  `9649a4bf` ("fix(auth): finish actor metadata removal and restore guards"),
  well before Phase 5/6. Phase 6 only touched rsrc/Go/doctrine, not wsflow skill
  text, so the failure reproduces identically at the Phase 5 tip `5c6fb71a`.
- The reference is doubly malformed: a `wsflow/` prefix glued onto the literal
  tool name `ws.lead.login`. The login tool keeps the dotted name `ws.lead.login`
  in wsflow mode (see `TestPreferMercenaryHiddenInNoAgentMode`, which asserts
  `"name":"ws.lead.login"` stays advertised), so the namespace substitution
  (`ws/`→`wsflow/`, `ws:`→`wsflow:`) never applies to the dotted form.

## Open questions (resolve before fixing)

- What is the canonical wsflow reference form for the `ws.lead.*` tools whose
  advertised names literally start with `ws.`? Options: (a) the skill says
  `lead.login` (drop the `ws.` prefix in prose) and the forbidden-`\bws\.`
  pattern stays strict; (b) the forbidden pattern gains a narrow allowance for
  `ws.lead.*` tool names; (c) the tools are re-advertised under a `wsflow.`-
  prefixed name in wsflow mode. This interacts with the keyed-handler `ws.lead.*`
  gate and the Phase 7 `ws.mercenary.*` rename.
- Whether other wsflow skills carry the same `wsflow/ws.lead.*` shape.

## Scope

Small skill-text + possibly test-pattern fix once the canonical form is decided.
Out of scope for `260611` Phase 6 (rsrc/loader convergence). Track here.

## Result (7fd40ce8) - 2026-06-15

Resolved by choosing option (a): wsflow skill prose drops the literal `ws.`
prefix for this login reference, and the forbidden `\bws\.` test remains
strict. `agents-plugin-wsflow/skills/lead-workflow-manual/SKILL.md` now lists
`wsflow/lead.login` in the runtime primitive block and describes calling the
lead login tool with a `root` argument instead of spelling the full dotted tool
identifier.

Verification ran `python3 -m unittest discover agents-plugin-wsflow/tests`; it
passed.
