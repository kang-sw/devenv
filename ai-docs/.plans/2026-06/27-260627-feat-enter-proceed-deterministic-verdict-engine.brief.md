# Brief: 260627-feat-enter-proceed-deterministic-verdict-engine

## Intent

Implement Phase 1 of `260627-feat-enter-proceed-deterministic-verdict-engine`:
move deterministic `lead-proceed` route and verdict resolution into
`ws.enter.proceed` while keeping the lead playbook responsible for artifact
reading, ambiguous judgments, and user-facing discussion.

The implementation starts with the ticket's handoff prework: `lead-proceed`
must route `NEXT: lead-implement` by printing and executing the
`lead-implement` playbook, not by calling `ws.enter.implement` or any other
implementation primitive directly.

## Selected Scope

- Ticket: `ai-docs/tickets/ready/260627-feat-enter-proceed-deterministic-verdict-engine.md`
- Phase: `Phase 1: MCP-owned proceed route verdict`
- Branch: `implement/260627-enter-proceed-verdict-engine`
- Implementation style: direct implementation in this branch after prep.
- Source edits are not part of this prep commit.

## Scope Boundary

In scope for the implementation phase:

- Hotfix the existing `lead-proceed` -> `lead-implement` handoff so the playbook
  explicitly calls `{{.McpNamespace}}/playbook.print(name: "lead-implement")`
  and executes the returned playbook inline.
- Replace the current `ws.enter.proceed` payload shape with
  `session_key + target + grouped optional/nullable facts + format`.
- Add private Go resolver logic behind `ws.enter.proceed` for normalization,
  precedence, warnings, route selection, result construction, raw verdict text,
  agenda storage, and todo replacement.
- Remove deterministic route-matrix prose from `lead-proceed` where the same
  decision is now made by MCP from normalized facts.
- Update specs/mental models at closeout for the shipped behavior.

Out of scope:

- Applying the same optimization to `lead-implement`; record compatibility hooks
  only.
- Adding a public route helper such as `ws.proceed.route`.
- Changing `playbook.print`, `playbook.render`, or product-mode rendering
  contracts.
- Redesigning ticket conventions, spec-address gates, or sage-review routing.

## Binding Decisions

[Must] `ws.enter.proceed` is the only public MCP mode-switch call for the
routing-facts-complete boundary. Reusable resolver code may be private Go only.

[Must] Deterministic `lead-proceed` rules that can be derived from normalized
facts move into MCP. The playbook keeps fact gathering, uncertain judgments,
and user interaction.

[Must] The final MCP output must tell the LLM the clear next direction. The raw
output begins with `Proceed Verdict`, `Route: ...`, and `NEXT: ...`, then
renders target, phase, reason, normalized conditions, warnings, and agenda.

[Must] Preserve current verdict-facing route vocabulary:
`target-kind`, `ticket-missing`, `has-ticket`, `status`, `migration-anchor`,
`actionable`, `discussion-needed`, `needs-ticket`, `freshness`, `category`,
`slice`, and `scope-blocked`.

[Must] Preserve current blocker specificity. Do not collapse values such as
`multiple-explicit-phases`, `too-broad`, `no-unfinished-phase`, or
`phase-already-complete` into a generic bucket.

[Must] Conflicting or inapplicable facts usually do not block. Normalize by
precedence, emit non-blocking warnings, and choose the conservative route.
Hard-block only malformed JSON/type failures, auth/session-key failures, and
enum values outside the accepted schema.

[Must] `ws.enter.proceed` remains a mode switch: it stores the `proceed` agenda
and replaces the session todo list for proceed mode.

[Must] Preserve the single-next-hop rule. The resolver emits one `NEXT`, and
the playbook executes only that next step.

[Must] The same deterministic-verdict optimization is intended for
`lead-implement` later, but this ticket only leaves the follow-up direction
documented.

## References

[Must] `ai-docs/tickets/ready/260627-feat-enter-proceed-deterministic-verdict-engine.md`

[Must] `ai-docs/spec/workflow-skills.md#260505-proceed-routing-pipeline`

[Must] `ai-docs/spec/workflow-skills.md#260519-proceed-implementation-dispatch-precheck`

[Must] `ai-docs/spec/mcp-tools.md` session-state `Enter (typed mode switches)`
section.

[Must] `ai-docs/mental-model/workflow-skills.md`

[Must] `ai-docs/mental-model/mcp-runtime.md`

[Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md`

[Must] `agents-plugin/skills/lead-skill-authoring/SKILL.md`

[Must] `ai-docs/ref/wsflow-mirroring.md`

## Likely Files

- `agents-plugin/rsrc/lead-proceed/lead-proceed.md`
- `agents-plugin/rsrc/manifest.json`
- `agents-plugin-wsflow/rsrc/lead-proceed/lead-proceed.md`
- `agents-plugin-wsflow/rsrc/manifest.json`
- `agents-plugin-tool/internal/mcp/server.go`
- `agents-plugin-tool/internal/mcp/session_state.go`
- `agents-plugin-tool/internal/mcp/session_state_test.go`
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go`
- `agents-plugin-tool/internal/wsrsrc/*_test.go`
- `ai-docs/spec/workflow-skills.md`
- `ai-docs/spec/mcp-tools.md`
- `ai-docs/mental-model/workflow-skills.md`
- `ai-docs/mental-model/mcp-runtime.md`

## Verification Expectations

- Focused Go tests for valid, partial, contradictory, inapplicable, and malformed
  `ws.enter.proceed` inputs.
- Tests for raw verdict stability, JSON result shape, warning output, agenda
  storage, and todo replacement.
- Playbook content tests proving `lead-proceed` delegates deterministic routing
  to MCP and invokes `lead-implement` through `playbook.print`.
- Manifest and wsflow rsrc mirror regeneration if shared rsrc changes.
- `go test ./internal/mcp -count=1`
- `go test ./internal/wsrsrc -count=1`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `git diff --check`
