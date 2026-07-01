---
title: "Epic: skill playbook diet — playbook-body / MCP / unnecessary golden rule"
sage-review: required
---

## Summary

Lead playbooks carry three kinds of content that need to be separated:

1. **Playbook-body** — lead-owned judgments, non-obvious edge cases, step ordering, output formats
2. **MCP** — input contracts (schemas), routing decisions, computed verdicts
3. **Unnecessary** — restatements of MCP schemas, duplicate fact guidance, rationale prose

Currently all three are mixed. This epic defines a golden rule distinguishing them,
encodes it in `lead-skill-authoring`, and uses it to diet all remaining skills.

## Two Optimization Levers

Skill diet requires both levers applied together:

**Lever A — Text minimization.** Remove any playbook text that restates an MCP tool
schema (field names, types, enums, call format). The model loads the schema via
ToolSearch before calling; the playbook must not repeat it.

**Lever B — MCP-ification of conditional logic.** Move rule-based conditional decisions
(routing, verdict computation, allocation logic) from playbook prose into `enter.*`
or other MCP tools. Once an MCP tool computes the decision deterministically, the
playbook collapses to: gather inputs → call tool → follow output.

`lead-proceed` and `lead-implement` already have significant MCP-ification via
`enter.proceed` and `enter.implement`, so their diet is mainly Lever A.
Skills without `enter.*` support may need Lever B work before they can be fully dieted.

## Golden Rule (to encode in lead-skill-authoring)

| Content | Where it lives |
|---------|---------------|
| Input field names, types, enums, call format | MCP tool schema only — delete from playbook |
| Routing/verdict logic expressible as deterministic rules | MCP tool (`enter.*`) — delete once tool exists |
| Non-obvious edge cases not capturable in MCP schema | Playbook body |
| Lead-owned judgments (soft decision tables) | Playbook body |
| Step ordering / choreography | Playbook body |
| Rationale, background, duplication of any row above | Delete |

## Pilot / Completed

- `lead-proceed`: 180 → 63 lines via Lever A. Removed Route Facts table, Fact
  Guidance section, `enter.proceed` JSON format block. Kept scope resolution
  edge cases (non-obvious) and three lead-owned judgment tables.
- `lead-discuss`: 189 → 93 lines via Lever A.
- `lead-implement`: 281 → 225 lines via Lever A, in two passes. First pass
  removed the JSON call-format block, the Fact groups table, and tautological
  policy bullets. Second pass verified against actual `enter.implement`
  instruction-generation source (`session_state.go`,
  `implement_resolver.go`) and found Route-onward step prose was near-verbatim
  restatement of MCP-generated todo `Instruction` text — collapsed to gates
  and ownership boundaries only. This second-pass method (confirm against
  actual generated tool output, not schema-inferred guesswork) is now encoded
  in `lead-skill-authoring`'s destructive-first stance and is the required
  method for any remaining `enter.*`-backed target below.

## Phases

### Phase 1: Encode golden rule in lead-skill-authoring — Done

Encoded in `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md`:
Layer Model table, two-lever framing, destructive-first stance (generalized
to bind to any MCP post-call output, not just `Next:`), Templates/delegate-
payload exemption.

### Phase 2: Audit and classify remaining playbooks

Superseded by direct-contact evidence-checking (see `lead-implement` note
above) rather than a separate up-front audit pass. Remaining targets are
classified inline, at diet time, against actual MCP tool source.

### Phase 3/4 curated targets (2026-07-01 curation)

Merged; lever type (A vs B) is determined per-skill at diet time rather than
pre-assigned, since `lead-implement` showed the split can only be confirmed
by reading the tool's actual generated output.

- `lead-write-spec` (129 lines)
- `lead-write-ticket` (371 lines)
- `lead-add-rule` (167 lines)
- `lead-workflow-manual` (162 lines)
- `lead-sprint` (155 lines) — `enter.sprint`-backed; apply the same
  generated-instruction-text verification method used for `lead-implement`
  before assuming any section is Layer 3.

### Deletion candidate (not diet)

- `lead-verify-design` (174 lines) — its function is covered by the ticket's
  `sage-review` gate; delete the skill rather than diet it. No other skill
  or manifest entry routes to it as a handoff target (confirmed via repo
  grep). Actual deletion is a separate approval step, not covered by this
  ticket-scoping edit.

## Out of Scope

- Changing MCP tool behavior or schemas unless serving a Lever B migration.
- Removing lead-owned judgment tables.
- Removing non-obvious edge-case guardrails.
- `lead-bootstrap`, `lead-forge-mental-model`, `lead-review`, `lead-salvage` —
  rarely invoked; spec clarity outweighs diet benefit for infrequently-called
  skills. Excluded from this epic's curated target list (2026-07-01).
- `lead-forge-spec` (290 lines) — was a Phase 3 candidate in the original
  scoping; not carried into the 2026-07-01 curated target list. Deferred, not
  ruled out — revisit in a separate pass if it resurfaces as a target.
