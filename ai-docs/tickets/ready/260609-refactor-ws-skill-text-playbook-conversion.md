---
title: ws skill-text conversion — playbooks replace internal skills and subquery
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: direction, entry-skill keep-list, subquery absorption decisions
  260609-feat-ws-playbook-surface-mvp: prerequisite — playbook surface (print/render + rsrc) must exist
spec:
  - 260610-entry-skill-surface-reduction
  - 260610-subquery-explore-delegation-shift
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
- **Forward-compat (epic 260605 option-B mercenary surface).** Native delegation
  is the *default*, not the *exclusive*, path. M3 reshapes the spawn engine into a
  scoped lead-invokable "mercenary" surface (codex+claude, implementer/reviewer
  only); M2 skill/playbook text must not bake in native-only delegation language
  that M3 would have to unwind. The M1 `delegates: true` tip fragment **is** the
  always-on mercenary-tip seam — mark delegating playbooks `delegates: true`
  rather than inlining native-only delegation prose. No mercenary runtime or
  routing wiring lands in M2 (that is M3); M2 only preserves the seam.
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

### Result (d982d4fe) - 2026-06-10

Implemented on `implement/subquery-explore-playbook` (delegated; survey plan).
- New `explore` render playbook (`agents-plugin/rsrc/explore/explore.md`,
  `kind:render`, `delegates:true`, vars `ExploreAgent`/`SpawnIdiom`/`ContinueIdiom`)
  adapting `SubquerySystemPrompt`; `manifest.json` regenerated (no schema bump).
- Shifted every `ws/subquery` delegation call site across 11 full-ws skills to the
  native Explore pattern (`lead-workflow-manual` holds the canonical primitive;
  other skills reference it). `grep "ws/subquery("` over `agents-plugin/skills` is
  now empty; one retained one-line note states the tool stays callable but is not
  the shipped-skill delegation path.
- Four golden render tests (claude/codex/neutral/junk + delegation tip);
  `go test ./...` green; `TestValidateRealTree` gates the new playbook.
- Spec `260610-subquery-explore-delegation-shift` 🚧 stripped (implemented);
  mental models (prompt-bundle, workflow-skills) updated.
- Scope held: `ws/subquery` runtime tool and `agents.*` untouched (M3); no
  internal-body migration / shim reduction (Phase 2); no wsflow edits.
- Review: partitioned (correctness/fit/test) clean after one fix cycle
  (`d982d4fe`: ws/ prefix, anti-tautology junk-harness test, wording).

Forward: Phase 2 migrates the 9 internal skill bodies to `playbook.print` and
reduces the 11 entry shims; `260610-entry-skill-surface-reduction` stays 🚧.
wsflow mirroring parity for the explore playbook is deferred to follow-up ticket
`260610-chore-wsflow-explore-playbook-mirroring`.

### Phase 2: internal procedures move off the entry surface

Move the 9 internal skill bodies (`lead-implement`, `lead-write-ticket`,
`lead-write-spec`, `lead-workflow-manual`, `lead-check-blockers`,
`lead-verify-design`, `lead-verify-discussion`, `lead-write-skeleton`,
`lead-update-spec`) into `playbook.print`-served content with caller wiring so
they are no longer directly user-invoked entry points. Make `lead-write-ticket`
and `lead-write-spec` orchestration-only. Relocate `lead-skill-authoring`'s
invariant-audit target to the rsrc playbook sources and follow the audit
procedure to the new text.

Verification: internal procedures resolve through `playbook.print` with
auto-included conventions; `lead-write-ticket`/`lead-write-spec` are reachable
only as orchestration, not as direct `/ws:<name>` entry points; the invariant
audit runs against rsrc sources.

Depends on Phase 1 (the Explore playbook is one migrated body's delegation
target).

### Phase 3: entry-skill shim reduction

Reduce the 11 entry skills (`lead-discuss`, `lead-sprint`, `lead-proceed`,
`lead-review`, `lead-ship`, `lead-salvage`, `lead-bootstrap`,
`lead-skill-authoring`, `lead-add-rule`, `lead-forge-mental-model`,
`lead-forge-spec`) to thin trigger shims over their playbook bodies, preserving
direct `/ws:<name>` invocability and good trigger descriptions.

Verification: the 11 entry skills remain user-invocable with correct triggers;
their procedural bodies resolve through the playbook surface.

Depends on Phase 2 (internal procedures and shared playbook content must land
before entry shims are thinned over them).

## Spec Impact

Addressed at ready promotion by contract-first planned spec entries in
`ai-docs/spec/workflow-skills.md` (listed in `spec:`):
`260610-entry-skill-surface-reduction` (directly invocable `/ws:*` set shrinks to
11; 9 internal → `playbook.print`) and `260610-subquery-explore-delegation-shift`
(skill-facing delegation guidance moves from `ws/subquery` to a native Explore
playbook). The `ws/subquery` runtime-tool removal is **M3**, not addressed here.
Doc closeout strips the `🚧` markers when implementation lands.
