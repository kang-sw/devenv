---
title: "Epic: skill playbook diet — MCP schema owns input contracts"
sage-review: required
---

## Summary

Lead playbooks have accumulated JSON call-format blocks and fact-guidance prose
that duplicate information already present in MCP tool schemas. This bloat
inflates context load and maintenance surface without adding execution value.

## Principle

> MCP tool schemas own input contracts. Playbooks own decision flow only.

When a playbook step calls an MCP tool (e.g. `enter.proceed`, `enter.implement`),
the tool's JSON Schema already documents every field: name, type, enum values, and
description. The playbook must not restate this. The playbook's job is:

1. State what facts to gather and how to derive non-obvious ones.
2. Describe edge cases the MCP tool cannot compute (lead-owned judgments).
3. Call the tool. Follow its output.

Prose sections that restate MCP schema content (fact tables, field guidance,
JSON call format blocks) are filler and must be deleted.

## Pilot

`lead-proceed` was dieted in session `glazing-recapture-cedar-facecloth-50` (2026-06-30):
180 lines → 63 lines. Removed: Route Facts table, Fact Guidance section, `enter.proceed`
JSON format block. Kept: scope resolution edge cases (non-obvious), three lead-owned
judgment tables, compressed invariants.

## Phases

### Phase 1: Add principle to lead-skill-authoring

Add the MCP-schema-owns-input-contracts principle and its diet pattern as a named
section in `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md`.

Acceptance: principle is stated in one paragraph and references the pattern
(remove fact tables and JSON format blocks that duplicate MCP schema content).

### Phase 2: Diet routing playbooks

Apply the diet pattern to:
- `lead-implement` (281 lines) — enter.implement call and surrounding fact guidance
- `lead-salvage` (212 lines)
- `lead-sprint` (155 lines)

For each: read current file, identify MCP-schema-owned content, delete it, verify
regen tests pass, commit per file or as a batch.

### Phase 3: Diet write and forge playbooks

Apply the diet pattern to:
- `lead-write-ticket` (371 lines)
- `lead-forge-spec` (290 lines)
- `lead-forge-mental-model` (222 lines)
- `lead-review` (212 lines)
- `lead-verify-design` (174 lines)

These have more lead-owned logic; audit carefully before deleting.

### Phase 4: Diet remaining playbooks

Apply to:
- `lead-add-rule` (167 lines)
- `lead-workflow-manual` (162 lines)
- `lead-bootstrap` (153 lines)
- `lead-discuss` (189 lines)

## Out of Scope

- Changing MCP tool behavior or schemas.
- Removing lead-owned judgment tables (these are NOT duplicated in MCP schemas).
- Removing genuinely non-obvious edge-case rules (scope resolution guards, etc.).
