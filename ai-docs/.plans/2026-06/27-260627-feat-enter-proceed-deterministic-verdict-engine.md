# Survey Plan: 260627 enter.proceed deterministic verdict engine

This plan starts after prep. Do not loosen the brief's `[Must]` decisions.

## 1. Reconfirm Current Surfaces

Read these files first:

- `agents-plugin/rsrc/lead-proceed/lead-proceed.md`
- `agents-plugin-tool/internal/mcp/server.go`
- `agents-plugin-tool/internal/mcp/session_state.go`
- `agents-plugin-tool/internal/mcp/session_state_test.go`
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go`
- `ai-docs/spec/workflow-skills.md`
- `ai-docs/spec/mcp-tools.md`

Confirm the current implementation points already located during prep:

- `server.go` dispatches `ws.enter.proceed` to `handleEnterProceed`.
- `server.go` advertises the current `ticket`, `phase`, `next_skill`, and
  `conditions` schema.
- `session_state.go` currently stores the proceed payload through the generic
  enter handler and derives a fixed proceed checklist.
- `lead-proceed.md` currently carries the route context/matrix logic and calls
  `enter.proceed` after `NEXT` is chosen.
- `playbook_tools_test.go` already has lead-proceed render/golden assertions and
  enter-call token checks.

## 2. Handoff Prework

First source edit: in `lead-proceed.md`, make the implementation route explicit.
When `NEXT:` is `lead-implement`, the playbook must call
`{{.McpNamespace}}/playbook.print(name: "lead-implement")` and execute the
returned playbook inline before any source inspection, planning, editing, or
implementation-tool use.

Add or update a render/content test so this directive cannot regress.

Do not call `enter.implement` from `lead-proceed`.

## 3. Schema and Type Plan

Replace the public `ws.enter.proceed` schema with:

- `session_key` (required)
- `target` (required object)
- `facts` (optional object with optional `ticket`, `gates`, and `work` groups)
- `format` (`text | json`, optional/default text)

Use Go-friendly snake_case JSON fields, but render normalized conditions with
the existing hyphenated route vocabulary.

Suggested implementation shape:

- Add small internal structs for proceed target/facts/result.
- Keep parsing/validation near `handleEnterProceed`.
- Add a private pure resolver function that accepts normalized input and returns
  result plus warnings.
- Keep the session-store write path atomic through the existing mode-switch
  primitive or an equivalent single update that stores the final result agenda
  and replaces todos together.

## 4. Resolver Rules

Implement precedence rather than asking the LLM to choose route rows after facts
are complete.

Minimum normalization rules:

- Missing fact groups become `unknown` or verdict-facing `n/a`.
- `target.kind=inline` makes ticket-only facts `n/a`.
- `ticket-missing=yes` wins over supplied status/actionability.
- Container categories (`epic`, `workset`) stop with `scope-blocked=container-ticket`.
- Done/dropped/unknown statuses stop.
- Ready + actionable + no blockers routes to `lead-implement`.
- Idea/todo or stale ticket routes to `lead-write-ticket` when refresh/promotion
  is the next deterministic step.
- User-blocking discussion facts route to `lead-discuss`.
- Scope blockers route to `stop` with the specific blocker preserved.

During implementation, map every current route row from `lead-proceed.md` into
the resolver or leave it in the playbook only when it requires ambiguous human
judgment. Do not silently drop a route row.

## 5. Output Contract

Text mode returns the canonical raw verdict:

```text
Proceed Verdict
Route: ...
NEXT: ...

Target: ...
Phase: ...
Reason: ...

Conditions:
- ...

Warnings:
- none

Agenda:
- ...
```

JSON mode returns the stable result object and includes the same `raw` string.

The agenda blob written under `proceed` should contain the resolver result or a
stable subset sufficient for restoration. It must include the selected ticket,
phase/slice, next skill, normalized conditions, and warnings.

## 6. Playbook Diet

After resolver tests are in place, edit `lead-proceed.md` so it:

- Builds route facts.
- Calls `{{.McpNamespace}}/enter.proceed(...)`.
- Reads the returned `NEXT`.
- Executes only that next step.

Remove deterministic route-selection prose now owned by MCP. Keep artifact
reading, freshness checks, migration-anchor checks, ambiguous judgments, and
user-facing discussion gates in playbook prose.

Because `lead-proceed` is included in wsflow, regenerate the canonical rsrc
manifest and wsflow rsrc mirror after the rsrc edit.

## 7. Tests

Add focused tests in `agents-plugin-tool/internal/mcp`:

- Valid ready/actionable ticket path returns `NEXT: lead-implement`.
- Inline actionable/no-ticket route behavior.
- Idea/todo route to ticket writing when deterministic.
- Done/dropped/missing/unknown stop behavior.
- Container-ticket and phase/scope blockers preserve exact blocker values.
- Contradictory facts produce warnings but still choose the conservative route.
- Invalid enum/type input fails as a tool error.
- Text mode raw verdict has stable first non-empty lines.
- JSON mode includes the structured result and identical raw field.
- Agenda storage and todo replacement still occur once per call.

Update playbook tests:

- `lead-proceed` render contains `enter.proceed`.
- `lead-proceed` render contains the explicit `playbook.print(name: "lead-implement")`
  handoff directive.
- The old deterministic route matrix prose is absent or reduced to fact-gathering
  guidance only.

## 8. Docs and Generated Artifacts

Update closeout docs after behavior is finalized:

- `ai-docs/spec/workflow-skills.md`
- `ai-docs/spec/mcp-tools.md`
- `ai-docs/mental-model/workflow-skills.md`
- `ai-docs/mental-model/mcp-runtime.md`

Regenerate when rsrc changes:

```bash
cd agents-plugin-tool
WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
go test ./internal/wsrsrc -count=1
```

Run package verification:

```bash
cd agents-plugin-tool
go test ./internal/mcp -count=1
python3 -m unittest discover ../agents-plugin-wsflow/tests
git diff --check
```

## 9. Escalation Points

Escalate before implementing if:

- A current `lead-proceed` route row cannot be represented without changing the
  pinned route vocabulary.
- `ws.enter.proceed` needs a second public helper tool to stay maintainable.
- The resolver cannot preserve atomic agenda+todo replacement.
- wsflow product-mode rendering would require contract changes beyond rsrc mirror
  regeneration.
