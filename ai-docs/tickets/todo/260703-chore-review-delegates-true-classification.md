---
title: "Review delegates:true classification across rsrc playbooks"
sage-review: skipped
related:
  260703-chore-prefer-subagent-verify-discussion-inline-mirror: origin of this ticket — Phase 2 review found lead-verify-discussion had carried an unconditional delegates:true continuity-tip/mercenary paragraph despite only conditional ("when investigation is useful") delegation; both paragraphs were removed from its SKILL.md rather than kept behind a marker exception
---

# Review delegates:true classification across rsrc playbooks

## Background

`delegates: true` on a `kind:print` rsrc playbook drives `delegationTip()`
(`agents-plugin-tool/internal/mcp/playbook_tools.go`), which unconditionally
appends a "Continuity tip" paragraph (all harnesses) and, in full ws mode, an
additional "Mercenary path (always available)" paragraph — regardless of
whether that specific invocation actually delegates to a subagent.

The authoring guideline in `ai-docs/mental-model/workflow-skills.md` (line
~104) says to set `delegates:true` "if it spawns subagents," without
distinguishing guaranteed/core delegation from merely-possible/conditional
delegation. This ambiguity was exposed during
`260703-chore-prefer-subagent-verify-discussion-inline-mirror` Phase 2:
`lead-verify-discussion` carried `delegates:true` even though its own
Process step explicitly delegates only "when investigation is useful"
(conditional), unlike `lead-verify-design`'s unconditional "isolate a fresh
deep reviewer" delegation (also `delegates:true`). Meanwhile
`lead-check-blockers` — described with near-identical "compact, lightweight
checkpoint" framing in the mental model — carries no `delegates:true` at all
and never spawns subagents as part of its normal flow. `lead-verify-discussion`
sat ambiguously between these two references, and its unconditional
continuity/mercenary append sat awkwardly against its own "keep the
checkpoint lightweight" instruction — resolved for that one skill by removing
the append entirely, but the underlying classification ambiguity was not
resolved for the rest of the `delegates:true` set.

Full original `kind:print, delegates:true` roster (pre-inlining):
`lead-discuss`, `lead-forge-mental-model`, `lead-forge-spec`, `lead-implement`,
`lead-salvage`, `lead-sprint`, `lead-verify-design`, `lead-verify-discussion`
(now false/inlined-without-tip per the linked ticket's Phase 2),
`lead-write-spec`. There is also a `kind:render` set (subagent-injection
prompts like `code-review-*`/`explore`/`reviewer`/`ticket-reviewer-*`) whose
`delegates:true` meaning may be entirely different and likely out of scope for
this review.

## Scope

Review each remaining `kind:print, delegates:true` playbook and decide, per
skill, whether the classification (and its unconditional continuity-tip /
mercenary-path append) fits that skill's actual delegation shape:
guaranteed-core delegation (keep as-is), conditional/optional delegation
(candidate for removal, matching the `lead-verify-discussion` precedent), or
no real delegation in current practice (candidate for removal). Consider
whether the authoring guideline itself
(`ai-docs/mental-model/workflow-skills.md` line ~104, "set `delegates:true` if
it spawns subagents") needs sharper wording distinguishing these cases.

## Out of Scope

- `kind:render` playbooks' `delegates:true` usage (different mechanism/meaning
  — separate investigation if needed).
- Re-opening the already-resolved `lead-verify-discussion` case (handled in
  `260703-chore-prefer-subagent-verify-discussion-inline-mirror` Phase 2).
