---
title: "Replace durable-rule capture with concise project-document prose auditing"
related:
  260909-research-ws-refoundation-evidence-audit: binding context for the reduced lead surface and downstream-first workflow
  260912-research-september-active-ticket-inventory-triage: inventory record for this implementation candidate
---

# Replace durable-rule capture with concise project-document prose auditing

## Background

`lead-add-rule` occupies a lead-skill entry for classifying durable rules and
routing them to `AGENTS.md` or a path-scoped manual. After the document-layer
retirement, the owner no longer considers that dedicated capture workflow worth
its lead-surface cost. The broader recurring problem is prose written by agents
in documents that future agents repeatedly read: over-negation, defensive
qualification that does not serve the document's purpose, and rationale that
defends the author without changing reader action.

The repository's `skill-authoring` manual is not installed downstream and
cannot supply this behavior to shipped skills. The replacement must be
self-contained in the plugin surface available to an ordinary downstream
project.

## Decisions

- Replace `lead-add-rule` with a write-capable `lead-audit-doc` skill. Retire
  the old skill name, playbook, and durable-rule classification and routing
  behavior rather than preserving a compatibility alias.
- Make the skill description attention-bearing for drafting, revising, or
  auditing frequently reread project guidance, especially rules, `AGENTS.md`,
  manuals, procedures, and references. It also applies after an agent
  materially edits such a document so the agent can invoke it without the user
  naming the skill.
- Keep the entry skill and lead procedure short because they are expected to be
  read frequently.
- The skill may write and revise the target document under the project's normal
  approval rules. It preserves the document's intended meaning and
  project-specific instructions.
- After a material write or revision, propose spawning a separate subagent for
  a fresh-read audit. Do not spawn the auditor until the user accepts the
  proposal.
- Give the fresh reader only the target document or excerpts needed for the
  audit. It identifies the affected prose and proposes a direct rewrite or
  deletion; the lead applies meaning-preserving fixes and returns semantic or
  policy changes to the user.

## Constraints

- The audit is limited to agent-writing pathologies: several exclusions where
  one positive owner or default carries the contract, disclaimers or defensive
  qualifications that do not change reader action, scope, or stop conditions,
  and rationale that repeats or defends a rule without narrowing it.
- Document headings, templates, formatting, tone, correctness, completeness,
  and policy remain outside the audit boundary; the skill introduces no general
  style system.
- Keep the shipped contract downstream-self-contained. Do not refer to this
  repository's `skill-authoring` manual or other files that bootstrap does not
  install.
- Preserve the full and agentless package mirrors, resource manifests, and
  skill-inventory contracts while replacing the public skill name.
- Apply the shipped-surface, skill-authoring, and wsflow-mirroring manuals to
  every matching source change in this repository; these are implementation
  constraints, not downstream dependencies.

## Phases

### Phase 1: Replace lead-add-rule with lead-audit-doc

Remove the durable-rule entry skill and its routed playbook, introduce the
concise write-capable document-audit entry and its fresh-reader delegation
contract, and update both package inventories, manifests, mirrors, and tests.
Verify autonomous description matching for recurring project-document edits,
the user confirmation boundary before subagent dispatch, downstream-only
resolution, the narrow prose-pathology scope, and absence of the retired skill
name and behavior.
