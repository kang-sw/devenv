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

## Pilot

`lead-proceed` dieted in this session (2026-06-30): 180 → 63 lines via Lever A.
Removed: Route Facts table, Fact Guidance section, `enter.proceed` JSON format block.
Kept: scope resolution edge cases (non-obvious), three lead-owned judgment tables.

## Phases

### Phase 1: Encode golden rule in lead-skill-authoring

Edit `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md`:
- Add the two-lever model (Lever A: text minimization; Lever B: MCP-ification)
- Add the golden rule table
- Name the section so later authoring passes can apply it without reading this epic

Acceptance: a new authoring pass on any playbook can apply the rule without
reading this ticket.

### Phase 2: Audit and classify remaining playbooks

For each un-dieted playbook, identify:
- Lever A removable content (MCP schema restatements)
- Lever B candidates (conditional logic that could move to MCP)
- Which MCP tools would need to be created or extended for Lever B

Produce a per-playbook classification. Research output; no edits in this phase.

### Phase 3: MCP-ification (Lever B) where needed

Implement new or extended `enter.*` / MCP tools identified in Phase 2.
Diet the corresponding playbooks immediately after each tool lands.

Likely candidates (confirm in Phase 2):
- `lead-write-ticket` (371 lines) — complex routing and phase-consistency logic
- `lead-forge-spec` (290 lines)
- `lead-review` (212 lines)

### Phase 4: Lever A diet for remaining playbooks

Apply text-minimization diet to playbooks that already have `enter.*` support
or have no significant conditional logic to MCP-ify:

- `lead-implement` (281 lines) — `enter.implement` exists
- `lead-salvage` (212 lines)
- `lead-forge-mental-model` (222 lines)
- `lead-discuss` (189 lines)
- `lead-verify-design` (174 lines)
- `lead-add-rule` (167 lines)
- `lead-workflow-manual` (162 lines)
- `lead-bootstrap` (153 lines)
- `lead-sprint` (155 lines)

## Out of Scope

- Changing MCP tool behavior or schemas unless serving a Lever B migration.
- Removing lead-owned judgment tables.
- Removing non-obvious edge-case guardrails.
