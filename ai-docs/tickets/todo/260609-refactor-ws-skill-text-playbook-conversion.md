---
title: ws skill-text conversion — playbooks replace internal skills and subquery
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: direction, entry-skill keep-list, subquery absorption decisions
  260609-feat-ws-playbook-surface-mvp: prerequisite — playbook surface (print/render + rsrc) must exist
related-mental-model:
  - workflow-skills
  - prompt-bundle
  - named-agent-runtime
---

# ws skill-text conversion — playbooks replace internal skills and subquery

## Background

Milestone M2 of the playbook-factory pivot (epic
`260605-epic-ws-playbook-factory-pivot`). With the playbook surface in place
(M1, `260609-feat-ws-playbook-surface-mvp`), this slice converts the skill
surface to the playbook model: internal skill bodies move to `playbook.print`
content, every `agents.*`/`subquery` delegation reference in skill text is
replaced with retained-native-subagent patterns, and `subquery` is absorbed into
a per-harness Explore playbook.

This is skill/prompt **text** conversion. It does NOT delete runtime code
(`agents.*`, runner backends, the `subquery` tool implementation) — that is M3.
After M2, skills no longer reference the spawn surface even though the runtime
tools still exist; M3 then deletes the now-unreferenced code.

Full direction and rejected alternatives: `260605-research-ws-native-subagent-pivot`
(see "subquery → harness Explore absorption", "skill surface reduction", and the
2026-06-08 "entry-skill keep-list" and "convention loading via playbook"
sections).

## Decisions

Binding decisions from the research ticket. A fresh implementer must not
re-derive a different skill surface:

- **Entry-skill keep-list is settled: 11 entry shims, 9 internal → playbook
  bodies** (inventory of 20 `SKILL.md` under `agents-plugin/skills`).
  - Entry shims (user types `/ws:<name>` directly; thin shims with good trigger
    descriptions): `lead-discuss`, `lead-sprint`, `lead-proceed`, `lead-review`,
    `lead-ship`, `lead-salvage`, `lead-bootstrap`, `lead-skill-authoring`,
    `lead-add-rule`, `lead-forge-mental-model`, `lead-forge-spec`.
  - Internal → `playbook.print` bodies: `lead-implement`, `lead-write-ticket`,
    `lead-write-spec`, `lead-workflow-manual`, `lead-check-blockers`,
    `lead-verify-design`, `lead-verify-discussion`, `lead-write-skeleton`,
    `lead-update-spec`.
  - `lead-write-ticket` and `lead-write-spec` become **orchestration-only**, not
    user-invoked directly; their bodies move to playbook content invoked by
    caller skills.
  - Classification axis is "is the user meant to type `/ws:<name>` directly",
    not cross-skill invocation count.
- **`subquery` is absorbed into a per-harness Explore playbook.** Claude and
  Codex both expose an Explore-style agent type; the subquery prompt-stem text
  (evidence discipline, scoping) becomes a render-kind playbook, and terminology
  (agent type name, spawn idiom) renders per harness. The async
  fire-and-forget + deferred-result pattern maps to native background subagents.
- **Delegation patterns use retained native subagents** (fast path) with a
  fresh-spawn + resume-brief recovery path. Retain/spawn continuity is tip-only
  (the `delegates: true` fragment from M1), guaranteed only within lead-context
  lifetime.
- **`lead-skill-authoring` stays an entry skill, but its invariant-audit target
  moves to the rsrc playbook sources.** The audit procedure must follow the text
  to its new home.

## Constraints

- Depends on M1: `playbook.print`/`playbook.render` and the rsrc tree must exist
  before skill bodies can move to playbook content.
- Observable workflow change (which skills the user can invoke directly, and the
  subquery → Explore terminology shift) — ask-first per repo Approval Protocol;
  ready promotion requires spec addressing for the changed workflow surface.
- No runtime deletion in this slice; `agents.*` and the `subquery` tool remain
  callable but unreferenced by shipped skill text after M2.

## Phases

### Phase 1: subquery → Explore playbook

Convert the `subquery` prompt-stem text into a render-kind Explore playbook
(harness-aware terminology), and replace `subquery` references in skill text with
the native Explore delegation pattern. Verification: skill text no longer names
`ws/subquery` for new delegation; the Explore playbook renders correctly for
claude and codex; an unknown harness gets host-neutral text.

### Phase 2: internal skill bodies → playbooks; entry-skill shim reduction

Move the 9 internal skill bodies to `playbook.print` content; reduce the 11
entry skills to thin trigger shims; make `lead-write-ticket`/`lead-write-spec`
orchestration-only. Relocate `lead-skill-authoring`'s invariant-audit target to
the rsrc playbook sources and follow the audit procedure to the new text.
Verification: the 11 entry skills remain user-invocable with correct triggers;
internal procedures resolve through `playbook.print` with auto-included
conventions; the invariant audit runs against rsrc sources.

Depends on Phase 1 (Explore playbook is one of the migrated bodies' delegation
targets).

## Spec Impact

- Target spec area: workflow-skill specs (the entry-skill surface and delegation
  model) and `mcp-tools.md` (subquery's skill-facing usage; the runtime tool
  removal itself is M3). Expected caller-visible change: the set of directly
  invocable `/ws:*` skills shrinks to 11; delegation guidance shifts from
  `ws/subquery` to native Explore.
- Contract-first spec: likely yes (observable workflow change; the keep-list must
  be stable before conversion). Resolve at ready promotion via
  `lead-write-spec`.
