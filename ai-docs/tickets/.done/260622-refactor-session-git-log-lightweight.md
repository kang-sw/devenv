---
title: Lightweight session git-log + commit-history-as-memory dispatch rule
completed: 2026-06-24
---

# Lightweight session git-log + commit-history-as-memory dispatch rule

## Background

Session bootstrap step 4 (`git log -10`) loads full commit bodies including
`## AI Context` sections at every session start, regardless of whether the
current session needs that depth. This bloats the initial context window with
dense decision rationale that is better fetched on demand.

Two coordinated changes address this:

1. Replace the full-body `git log -10` in AGENTS.md step 4 with
   `git log --oneline -20` — lighter arc, same recency coverage.
2. Add an Evidence invariant to the lead-discuss playbook establishing commit
   history as an explicit project memory tier: `## AI Context` bodies hold
   decision rationale that docs may not yet reflect, and access should go
   through an Explore-type subagent rather than inline reads.

## Spec Impact

Target spec area: `workflow-skills.md` — lead-bootstrap Project Memory section (step 4) and lead-discuss Evidence invariant block.
Expected caller-visible change: bootstrap sessions receive `git log --oneline -20` output instead of full `git log -10` bodies; lead-discuss Evidence invariant gains a recommendation to route commit-history access through Explore-type subagent dispatch rather than inline reads.
Contract-first spec: no

## Phases

### Phase 1: Oneline step 4 + commit-history-as-memory rule

Change `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`:
- Step 4 in `## Project Memory`: `git log -10` → `git log --oneline -20`,
  description updated to "recent commit stems" (drop the "for `## AI Context`
  rationale" phrase, which no longer applies).
- Add migration entry: `- v0037: Replace step 4 in ## Project Memory from
  \`git log -10\` to \`git log --oneline -20\` with description "recent commit
  stems".`

Change `AGENTS.md` (this repo):
- Same step 4 update as the template.

Change `agents-plugin/ws/rsrc/lead-discuss/lead-discuss.md`:
- In the `## Invariants > Evidence` block, after the mental-model staleness
  line, add:

  > Commit history is a project memory tier: `## AI Context` bodies carry
  > decision rationale that docs may not yet reflect. Access this memory
  > through Explore-type subagent dispatch rather than inline git log reads.

Constraint: the new rule is recommendation-level ("Access" not "Must access");
mandatory enforcement is intentionally deferred.

Rejected alternative: remove step 4 entirely — step 3 already covers
`--oneline --graph -50`, but removal requires bootstrap migration coordination
beyond a template text update and was deemed out of scope.

### Result (5a79a5d2) - 2026-06-24

Phase 1 implemented as specified. Deviation: ticket specified migration entry v0037,
but v0037 was already taken by the `.deps/` gitignore entry; used v0042 (next
available after v0041) and bumped Template Version tag accordingly.
Spec closeout: `workflow-skills.md` cascade paragraph updated with commit-history
tier description; mental-model was already current.
