# Brief: 260508-chore-lightweight-epic-tickets

## Intent

Make epic tickets intentionally lightweight milestone boards. Detailed
discussion, implementation phases, and slice-specific decisions should live in
child tickets; epics should preserve scope, decomposition, cross-child
decisions, and completion state.

## Approach

- Update bundled ticket conventions and Claude compatibility convention copies.
- Update Codex `lead-write-ticket` guidance and Claude compatibility skill text
  so epic creation/editing preserves the board-level boundary.
- Update specs by stripping the planned markers once behavior is documented.
- Update mental models only where modification-relevant facts changed.

## Constraints

- Do not change ticket parser behavior or status flow unless docs require it.
- Keep epic tickets exempt from ready spec gating as they are decomposition
  artifacts.
- Preserve host-specific notation in Claude compatibility files; do not rewrite
  them into MCP notation.
- Apply the skill-authoring invariant checklist to changed skill invariant or
  constraint lines.

## Out of scope

- Do not rewrite existing epic tickets.
- Do not create child tickets for existing epics.
- Do not rename skills or change invocation names.

## Details

Expected epic contents:

- scope and non-scope;
- child ticket board;
- cross-child invariant decisions;
- done, dropped, or deferred criteria.

`lead-write-ticket` should direct detailed discussion, approaches, constraints,
and implementation phases into child tickets when they exceed board-level scope.
A single child ticket may carry multiple phases when they form one cohesive
reviewable unit.

## References

- [Must] `ai-docs/tickets/ready/260508-chore-lightweight-epic-tickets.md` - target contract.
- [Must] `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md` - bundled canonical convention source.
- [Must] `claude-plugin/infra/ticket-conventions.md` - Claude compatibility convention copy.
- [Must] `agents-plugin/skills/lead-write-ticket/SKILL.md` - Codex ticket authoring behavior.
- [Must] `claude-plugin/skills/write-ticket/SKILL.md` - Claude compatibility skill copy.
- [Must] `ai-docs/ref/skill-authoring.md` - skill/convention authoring rules.
- [Must] `ai-docs/spec/documentation-system.md` - lightweight epic convention spec.
- [Must] `ai-docs/spec/workflow-skills.md` - write-ticket epic boundary spec.
- [Must] `ai-docs/mental-model/documentation-system.md` - convention embedding and ticket coupling.
- [Must] `ai-docs/mental-model/workflow-skills.md` - Codex/Claude skill alignment rules.
- [Maybe] `agents-plugin/skills/lead-discuss/SKILL.md` - only if discussion-to-child-ticket routing needs explicit guidance.
- [Maybe] `claude-plugin/skills/discuss/SKILL.md` - only if the Codex discuss guidance changes.

## Verification

Run:

```sh
git diff --check
cd agents-plugin-tool && go test ./internal/wsdoc ./internal/mcp
cd agents-plugin-tool && go test ./...
```
