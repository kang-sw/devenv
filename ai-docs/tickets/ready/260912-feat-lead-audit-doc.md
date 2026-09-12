---
title: "Replace durable-rule capture with concise project-document prose auditing"
related:
  260909-research-ws-refoundation-evidence-audit: binding context for the reduced lead surface and downstream-first workflow
  260912-research-september-active-ticket-inventory-triage: inventory record for this implementation candidate
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0bec273329d7af0d
sage-review-completeness-reviewed: 0bec273329d7af0d
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

The current `lead-add-rule` description also attracts unrelated requests that
mention saving, remembering, or persisting workflow context. Removing that
high-frequency misrouting source makes this replacement release-priority work,
not optional post-release cleanup.

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
- Add a dedicated `fresh-read-doc-auditor` playbook rather than broadening the
  existing `fresh-reader-audit`, whose contract serves skill and prompt
  authoring.

## Verbatim Shipped Prose

### Skill description

```yaml
description: Draft, revise, or audit frequently reread project documents such as AGENTS.md, rules, manuals, procedures, and references. Also use after materially editing one of these documents to remove over-negation and defensive prose, and to offer an independent fresh-read audit.
```

### Lead playbook

```markdown
# Audit Doc

Draft or revise the requested project document under its existing instructions.
Preserve its meaning. Remove repeated exclusions that a positive owner or
default can state once, qualifications that change no reader action, scope, or
stop, and rationale written to defend the author rather than guide the reader.

The document keeps its own structure and style.

After writing, if the user did not already request an independent audit, ask
whether to spawn one. On acceptance, render `fresh-read-doc-auditor` through
`{{.McpNamespace}}/playbook.render` with the target path or excerpts and spawn
it with the returned bindings. Give it no conversation context.

Apply meaning-preserving findings. Return any finding that would change policy
or intent to the user.

Report the changed path and whether the independent audit ran.
```

### Fresh-read auditor playbook

```markdown
# Fresh-Read Document Audit

You are a first-time reader. Read only {{.TargetFiles}}.

Your entire scope is:

- repeated exclusions where one positive owner or default states the contract;
- disclaimers or qualifications that do not change reader action, scope, or a
  stop condition;
- rationale that defends the author or repeats a rule without narrowing it.

Preserve the intended meaning and existing project-specific instructions. For
each finding, quote the affected text, say which scope item it matches, and
propose a direct rewrite or deletion. If there are no findings, say `No findings.`
```

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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/skills/lead-add-rule/SKILL.md, agents-plugin/rsrc/lead-add-rule/lead-add-rule.md, agents-plugin-wsflow/skills/lead-add-rule/SKILL.md, agents-plugin-wsflow/rsrc/lead-add-rule/lead-add-rule.md |
| scope.surface | public-interface | agents-plugin/skills/lead-add-rule/SKILL.md#L2-L3 exposes the installed skill name and description |
| scope.new_public_symbol | yes | lead-audit-doc replaces the public lead-add-rule skill name |
| scope.new_type_contract | no | no Go type or signature change is named |
| scope.test_surface | existing | agents-plugin/tests/test_skill_dispatch_contracts.py, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py, agents-plugin-tool/internal/mcp/playbook_tools_test.go |
| complexity.reuse_points | confirmed | agents-plugin/rsrc/fresh-reader-audit/fresh-reader-audit.md provides the existing fresh-reader pattern |
| complexity.side_effect_risk | high | replaces an installed public skill and its delegated-playbook behavior |
| risk.correctness | high | full and agentless package mirrors, manifests, and inventories must remain aligned |
| risk.fit | moderate | shipped text must remain downstream-self-contained while preserving project instructions |
| risk.test | high | dispatch, user-confirmation, mirror, manifest, and retired-name behavior require coverage |
| risk.security_or_contract | high | removal of the public lead-add-rule contract and introduction of lead-audit-doc changes skill dispatch |

## Phases

### Phase 1: Replace lead-add-rule with lead-audit-doc

Remove the durable-rule entry skill and its routed playbook, introduce the
concise write-capable document-audit entry and its fresh-reader delegation
contract through the dedicated `fresh-read-doc-auditor`, and update both
package inventories, manifests, mirrors, and tests.
Verify autonomous description matching for recurring project-document edits,
the user confirmation boundary before subagent dispatch, downstream-only
resolution, the narrow prose-pathology scope, and absence of the retired skill
name and behavior.
