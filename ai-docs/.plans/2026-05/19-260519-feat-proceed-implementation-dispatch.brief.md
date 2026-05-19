# Brief: 260519-feat-proceed-implementation-dispatch Phase 1

## Intent

Remove stale skeleton-routing wording from active workflow skill text so normal
proceed, implement, and ticket handoff paths no longer describe skeleton
decisions as live routing branches.

## Scope Boundary

Implement only Phase 1: stale skeleton routing language removal. Phase 2
proceed dispatch precheck behavior is out of scope.

## Caller-Visible Contract

Skill readers should see that normal implementation routing proceeds through
`lead-implement`, `lead-edit`, and `lead-write-code`; generated skeleton
artifacts remain legacy compatibility material, not an active routing branch.

## Implementation Strategy Decisions

- Preserve existing behavior and routing tables except for skeleton-specific
  normal-routing wording.
- Keep spec and mental-model statements that explicitly mark skeleton artifacts
  as deprecated or legacy.
- Check wsflow mirrors for affected skills; do not add wsflow-only changes when
  no stale skeleton routing wording exists there.

## Rejected Alternatives

- Do not delete `lead-write-skeleton`; it remains a compatibility artifact.
- Do not implement Phase 2 dispatch precheck in this phase.
- Do not rewrite broader proceed phase-selection behavior.

## Approach

- Edit full ws `lead-proceed` and `lead-write-ticket` wording that presents
  skeleton decisions as normal routing.
- Audit full ws `lead-implement` and wsflow counterpart skills for matching
  stale routing language.
- Run skill and wsflow verification commands after edits.

## Constraints

- Follow skill-authoring invariants: compressed directives, no duplicated
  invariants, and no session-specific references in skill text.
- Keep wsflow distributed text free of full ws-only names and excluded skills.

## Out of scope

- Proceed-side implementation dispatch fields.
- Any change to source code or MCP behavior.
- Ticket or spec closure beyond Phase 1 result capture.

## Details

Target files:

- `agents-plugin/skills/lead-proceed/SKILL.md`
- `agents-plugin/skills/lead-write-ticket/SKILL.md`
- `agents-plugin/skills/lead-implement/SKILL.md`
- `agents-plugin-wsflow/skills/lead-proceed/SKILL.md`
- `agents-plugin-wsflow/skills/lead-write-ticket/SKILL.md`
- `agents-plugin-wsflow/skills/lead-implement/SKILL.md`

## Verification Contract

- `rg -n "skeleton|Skeleton|lead-write-skeleton|skeletons:"` over the active
  proceed, implement, and write-ticket skill files must show no normal-routing
  skeleton wording.
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `ws/spec_index.verify`

## References

- `ai-docs/mental-model/workflow-skills.md` - skeleton artifacts are deprecated
  from normal implementation routing.
- `ai-docs/spec/workflow-skills.md` - planned proceed dispatch behavior and
  legacy skeleton compatibility statements.
- `ai-docs/ref/wsflow-mirroring.md` - wsflow mirror requirements and forbidden
  full ws references.
