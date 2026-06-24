---
title: "tickets.template tool + ticket-conventions diet"
related:
  260624-epic-pre-release-cleanup: sibling cleanup workset on same branch
spec:
  - mcp-tools
---

# tickets.template tool + ticket-conventions diet

## Background

`lead-write-ticket` calls `convention.read(name: "ticket-conventions")` at step 1
of every invocation, loading all 199 lines regardless of the ticket type being
created or edited. Of those 199 lines, 136 are type-specific body templates
(feat/bug/refactor/chore, research, workset, epic) that are only needed for the
current ticket type.

Adding a `tickets.template(type)` MCP tool moves the typed skeletons into the
tool layer and slims `ticket-conventions.md` to the ~63-line structural-invariant
section. Per-invocation loaded context drops from 199-line convention + procedure
to ~63-line invariants + ~30-line typed skeleton — and the skeleton itself becomes
the on-demand reference for both creation and update workflows.

## Decisions

- **Option B (separate tool)** chosen over embedding skeletons as inline comments:
  update path can call `tickets.template(type)` explicitly to recall the expected
  shape without coupling skeleton text to the file on disk.
- **ticket-conventions.md keeps the invariant section.** Structural rules (path
  format, status flow, phase numbering, stem immutability) are referenced during
  update and cascade edits and must remain in `convention.read`.

## Spec Impact

Target spec area: `ai-docs/spec/mcp-tools.md` — add `tickets.template` tool entry
(parameters, return format, capability range).
Expected caller-visible change: new `tickets.template` MCP tool exposed to
agents; `ticket-conventions.md` Templates section removed.
Contract-first spec: no (implementation clarifies the schema; spec closeout follows
Phase 1).

## Phases

### Phase 1: Add tickets.template(type) MCP tool

- New tool `tickets.template` in `agents-plugin-tool/internal/mcp/` (server.go or
  a new file under `internal/mcp/`).
- `type` param: enum `feat|bug|refactor|chore|research|workset|epic`.
- Returns the typed body template currently in
  `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md` lines 64–199,
  formatted as a ready-to-fill markdown skeleton.
- Register in both `agents-plugin/runtime.json` and
  `agents-plugin-wsflow/runtime.json` with capability range `>=<next-version>`.
- Add unit test: each type returns non-empty, well-formed markdown with expected
  section headers.

### Result

Commit `921141e4`. Implemented `agents-plugin-tool/internal/wsdoc/tickets_template.go`
(`TicketTemplate(typeStr string) (string, error)`), MCP handler in `server.go`, tests in
`tickets_template_test.go` (7-type coverage, structural headers, error cases). Both
`agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json` updated with
`"tickets.template": ">=0.30.6-dev <0.31.0"` after `tickets.create`. All review
dimensions (correctness, fit, test) passed.

### Phase 2: Slim ticket-conventions.md

- Remove lines 64–199 (Templates section) from
  `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md`.
- Keep lines 1–63 (structural invariants: path/naming, status flow, epic/workset
  rules, phase rules, stems, general rules).
- Regenerate shipped manifest:
  `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc/ -run TestRegenerateShippedManifest`.

### Phase 3: Update lead-write-ticket playbook

- In `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`:
  - Remove the upfront `convention.read(name: "ticket-conventions")` call from
    `On: invoke → 1. Resolve`.
  - In `On: Create Ticket → 1. Classify`: after determining category, call
    `{{.McpNamespace}}/tickets.template(type: "<category>")` to load the typed
    skeleton.
  - In `On: Edit Ticket → 1. Load`: call `tickets.template(type)` when the ticket
    type is known and skeleton reference is needed.
  - Keep a `convention.read(name: "ticket-conventions")` call only when structural
    invariant rules are needed (e.g., `On: invoke → 2. Route` for status flow rules,
    spec-address gate, cascade logic).
- Regenerate manifest after playbook edit.

## Verification

- `go test ./internal/mcp/...` green including new `tickets.template` tests.
- `go test ./internal/wsrsrc/...` green after manifest regen.
- `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` green after both
  runtime.json files updated.
- Invoke `write-ticket` playbook in a smoke session; confirm typed skeleton loads
  on creation and convention loads only for invariant checks.
