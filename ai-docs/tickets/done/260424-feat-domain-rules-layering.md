---
title: "Domain Rules Layering: Architecture Rules Split and /add-rule Skill"
spec:
  - 260424-mental-model-directory-hierarchy
  - 260424-domain-rules-section
  - 260424-executor-ancestor-load
  - 260424-mental-model-updater-forge-authority
  - 260424-mental-model-domain-rules-promotion-only
  - 260424-mental-model-domain-rules-stale-output
  - 260424-add-rule-skill
  - 260424-bootstrap-architecture-rules-migration
related-mental-model:
  - workflow-routing
  - executor-wrapup
---

# Domain Rules Layering: Architecture Rules Split and /add-rule Skill

## Background

Downstream projects using `CLAUDE.template.md` routinely mix two distinct kinds of
rules in `## Architecture Rules`: cross-cutting structural invariants (e.g.,
"no circular dependencies") and domain-scoped architectural conventions (e.g.,
"all auth flows go through AuthService"). The current inclusion test — "does breaking
this rule make a skill produce wrong output?" — does not distinguish between them;
both pass it.

This ticket introduces a 2-layer split:

- **Architecture Rules (CLAUDE.md)** — cross-cutting invariants that apply everywhere
  in the codebase, regardless of which area is being worked in.
- **Domain Rules (mental-model docs)** — domain-scoped prescriptions that apply when
  working in a specific domain. Stored in `## Domain Rules` sections in the relevant
  `ai-docs/mental-model/<domain>.md`.

The authoring entry point is a new `/add-rule` skill that classifies incoming rules
and routes them to the right document. `mental-model-updater` gains forge-level
restructuring authority (create, split, promote) to keep the mental-model structure
aligned with code structure autonomously.

## Decisions

**Domain rules live in mental-model docs, not a new CLAUDE.md section.**
Mental-model docs are already domain-organized and loaded on-demand by executor skills.
A new CLAUDE.md section was rejected: it would require a new selective-reading
convention, while mental-model loading already handles this.

**mental-model-updater gets full restructuring authority — no user gate.**
Mental-model docs are agent artifacts consumed by other agents, not human-facing docs.
Bad restructuring is recoverable: the next updater pass can fix it. Git history
provides the audit trail. A user-approval gate was rejected as unnecessary friction.

**Domain Rules: promotion-only during splits, content modification forbidden.**
Rules only move upward in the hierarchy (sub-domain → parent index) during splits.
Downward moves require explicit user action. The updater never modifies rule content
— only flags stale rules in output. This is the asymmetry that protects user-authored
content under full updater autonomy.

**Split trigger is code-structure change, not LLM creativity.**
The updater splits a domain doc when the code diff shows the underlying module
splitting into sub-directories. It does not restructure opportunistically without
code evidence.

**Ancestor loading via path convention, not runtime lookup.**
When a skill loads `mental-model/<domain>/<sub>.md`, it also loads
`mental-model/<domain>/index.md`. The hierarchy is encoded in the file path —
no frontmatter parent-link needed.

**`/add-rule` is interactive-biased, autonomous when clear.**
When classification and target are unambiguous, the skill proposes and writes without
waiting for confirmation. Ambiguous cases (cross-cutting vs domain-scoped, or unclear
which domain) prompt the user. New domain doc creation always proposes before writing.

## Phases

### Phase 1: Mental-model conventions and directory hierarchy

Extend `claude/infra/mental-model-conventions.md` to define:
- `## Domain Rules` as a standard section in domain docs: user-authored prescriptions
  scoped to the domain; never modified by agents autonomously.
- Directory hierarchy convention: when a domain grows to multiple sub-concerns,
  the flat `<domain>.md` becomes `<domain>/index.md` + child files, mirroring
  `ai-docs/spec/<area>/` from spec v0022.
- Ancestor loading rule: any agent loading `<domain>/<sub>.md` must also load
  `<domain>/index.md` before starting work.

Update `ai-docs/mental-model.md` (index) to describe the directory hierarchy and
Domain Rules section.

Update `claude/bin/list-mental-model` to produce hierarchy-aware output: when
`<domain>/index.md` + child files exist, display the domain as a tree with the
parent on one line and children indented beneath it. Flat domains remain unchanged.

Depends on: nothing.

**Acceptance criteria:**
- `mental-model-conventions.md` defines `## Domain Rules` section with authorship
  and modification invariants.
- Conventions doc defines directory hierarchy with example layout.
- Ancestor loading rule is stated as an invariant, not a recommendation.
- `list-mental-model` output indents sub-domain docs under their parent domain.

### Result (8298a65) - 2026-04-24

Implemented as specified. `mental-model-conventions.md` extended with `## Directory Hierarchy`, ancestor loading invariant, and `## Domain Rules` sections. `ai-docs/mental-model.md` updated. `list-mental-model` rewritten with tree output (`├─`/`└─`) and ancestor `index.md` auto-emit in filtered mode. Ancestor loading bounded to 1-level hierarchies (`<domain>/<sub>.md` only) — no depth overclaim.

### Phase 2: mental-model-updater extension

Update `claude/agents/mental-model-updater.md` to:
- Grant forge-level restructuring authority: create new domain docs when code introduces
  a new module with no coverage; split existing flat docs into directories when the
  corresponding code sub-structure diverges.
- Add Domain Rules invariants: during splits, promote Domain Rules to parent `index.md`;
  never move rules downward; never modify rule content.
- Add `## Stale Rules` output section: when a Domain Rule appears inconsistent with
  current code behavior, list it with the observed inconsistency — no autonomous edit.

Depends on: Phase 1 (conventions must be defined before the agent references them).

**Acceptance criteria:**
- Agent instructions include explicit forge-authority steps and Domain Rules invariants.
- `## Stale Rules` section is defined in the output template.
- Agent instructions explicitly prohibit modifying Domain Rules content.

### Result (8298a65) - 2026-04-24

Implemented as specified. `mental-model-updater.md` updated with 3 Domain Rules Constraints, Step 5 Restructure with `/forge-mental-model` authority (create / split / no other), Step 6 Stale rule detection, and `## Stale Rules` output block. Domain Rules promotion-only (upward during splits, never downward, never content-modified). Stale rules flagged in output only — user resolves via manual edit or `/add-rule` replacement.

### Phase 3: Executor ancestor loading

Update executor skill docs (`ws:edit`, `ws:implement`, `ws:parallel-implement`) to
state the ancestor-loading contract: when the task touches a sub-domain mental-model
doc, load the parent `index.md` before starting work.

Also update `claude/infra/executor-wrapup.md` to record this as a shared executor
contract.

Depends on: Phase 1 (hierarchy must be defined first).

**Acceptance criteria:**
- Each executor skill doc references the ancestor-loading contract.
- `executor-wrapup.md` states the contract as a shared invariant.

### Result (8298a65) - 2026-04-24

Implemented as specified. `executor-wrapup.md` gains ancestor loading Invariant bullet and `## §Ancestor Loading` procedural section (3 numbered steps). `edit/SKILL.md`, `implement/SKILL.md`, `parallel-implement/SKILL.md` each have the ancestor-loading Invariant bullet and spawn-prompt propagation with "(one-level hierarchies — `<domain>/<sub>.md` only)" bound. Depth bound added during review round 2.

### Phase 4: /add-rule skill

Create `claude/skills/add-rule/SKILL.md`.

Classification logic:
- **Cross-cutting**: rule applies everywhere regardless of domain → append to
  `## Architecture Rules` in `CLAUDE.md`.
- **Domain-scoped**: rule applies in a specific domain → write to `## Domain Rules`
  in the relevant `ai-docs/mental-model/<domain>.md`.

Routing behavior:
- Clear classification + existing target doc → propose and write autonomously.
- Ambiguous classification or multiple domain candidates → list candidates, ask user.
- No matching domain doc exists → propose new doc with frontmatter, ask user to confirm.

Refusals: no modification of existing rule content; no simultaneous write to both
CLAUDE.md and a mental-model doc for the same rule.

Depends on: Phase 1 (Domain Rules section must be defined).

**Acceptance criteria:**
- Skill correctly routes a clearly cross-cutting rule to CLAUDE.md without prompting.
- Skill asks for domain selection when multiple candidates match.
- Skill proposes creating a new doc when no domain matches.
- Skill refuses to modify existing rules.

### Result (8298a65) - 2026-04-24

Implemented as specified. `claude/skills/add-rule/SKILL.md` created new. 5 Invariants (append-only, single target, no dual-write, English-only, AI Context in commit). Handler: Read → Classify → Route → Propose/Prompt → Write → Commit → Report. `judge: classification` signal table; `judge: domain-match` single/multiple/none routing. §5 Write handles both mental-model doc and CLAUDE.md missing-section cases. Doctrine: "Preserve user's original phrasing — tighten only grammar and active voice." Routing conditionals removed from Invariants in review round 2.

### Phase 5: CLAUDE.template.md — Architecture Rules refinement and migration item

Update `claude/skills/bootstrap/CLAUDE.template.md`:
- Tighten the `## Architecture Rules` inclusion test comment: explicitly state that
  domain-scoped rules do not belong here, and direct authors to mental-model Domain
  Rules via `/add-rule`.
- Add v0028 migration checklist item: prompt existing downstream project owners to
  re-evaluate current Architecture Rules and `_index.md` architectural conventions,
  reclassifying domain-scoped rules into `ai-docs/mental-model/<domain>.md` via
  `/add-rule`.

Depends on: nothing (template change is independent).

**Acceptance criteria:**
- Architecture Rules comment distinguishes cross-cutting from domain-scoped.
- v0028 migration item is present in the checklist.
- Template version tag updated to v0028.

### Result (8298a65) - 2026-04-24

Implemented as specified. `CLAUDE.template.md` inclusion test comment updated to explicitly exclude domain-scoped rules and direct authors to `/add-rule`. v0028 migration item added: re-evaluate existing Architecture Rules and move domain-scoped entries to mental-model Domain Rules via `/add-rule`. Template version tag updated to `v0028`.
