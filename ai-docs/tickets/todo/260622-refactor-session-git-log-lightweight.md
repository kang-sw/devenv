---
title: Lightweight session git-log + commit-history-as-memory dispatch rule
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
