---
title: skill description attention policy
related:
  260508-chore-lightweight-epic-tickets: related workflow-authoring policy cleanup
related-mental-model:
  - workflow-skills
  - prompt-bundle
---

# skill description attention policy

## Background

Codex skill descriptions are part of the runtime attention surface. Descriptions
that are too weak fail to trigger important workflow entry points, while
descriptions that are too broad cause ordinary user instructions to be mistaken
for workflow commands. The current `lead-*` skill set needs clearer separation
between top-level entry skills, conditional persistence utilities, and internal
pipeline stages.

`lead-add-rule` is the key ambiguous case. It is a user-facing utility, but its
current trigger language can overmatch ordinary instructions containing words
such as "must" or "should." Rule persistence should require explicit durable
storage intent, not just prescriptive wording in the user's task.

## Decisions

- Strengthen descriptions for top-level workflow entry skills:
  `lead-workflow-manual`, `lead-discuss`, `lead-implement`, `lead-sprint`,
  `lead-ship`, `lead-proceed`, and `lead-write-ticket`.
- Treat `lead-add-rule` as conditional/explicit: trigger it only when the user
  asks to save, remember, persist, or add a rule across future sessions.
- Keep internal or derived stage descriptions lighter:
  `lead-edit`, `lead-write-code`, `lead-write-spec`, `lead-write-skeleton`,
  and `lead-update-spec`.
- Preserve the `lead-implement` hierarchy: it chooses between direct edit and
  delegated write-code; `lead-edit` is not a peer top-level entry point.
- Avoid broad keyword triggers such as "always", "never", "must", or "should"
  unless persistence intent is also explicit.

## Phases

### Phase 1: Audit description tiers

Audit all `agents-plugin/skills/*/SKILL.md` frontmatter descriptions and classify
each skill as top-level entry, conditional utility, or derived pipeline stage.

Record any Claude compatibility skill whose trigger language should remain
semantically aligned but host-specific.

### Phase 2: Update Codex skill descriptions

Rewrite Codex-facing descriptions so top-level entry skills have strong
natural-language triggers and derived stages avoid overmatching.

Use explicit persistence wording for `lead-add-rule`; do not trigger it from
ordinary prescriptive task language alone.

### Phase 3: Update compatibility docs and specs

Update workflow-skill specs, mental models, and Claude compatibility skill
descriptions where needed so the attention policy remains documented and
portable without forcing identical host notation.

