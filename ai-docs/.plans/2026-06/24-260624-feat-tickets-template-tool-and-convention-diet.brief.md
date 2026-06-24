# Brief: 260624-feat-tickets-template-tool-and-convention-diet (Phase 1)

## Intent

Add `tickets.template(type)` MCP tool that returns the typed ticket body skeleton
for a given ticket category. Callers use it at ticket-creation and update time to
get a focused, fill-in template rather than loading all 199 lines of
`ticket-conventions.md` and extracting the relevant section themselves.

## Scope Boundary

Phase 1 only: MCP tool handler in server.go, a `wsdoc.TicketTemplate(typeStr)`
helper in the wsdoc package, unit tests, and registration in both runtime.json
files.

Deferred:
- Removing the Templates section from `ticket-conventions.md` (Phase 2).
- Updating the `lead-write-ticket` playbook to call the new tool (Phase 3).

## Caller-Visible Contract

Tool name: `tickets.template`
Parameter: `type` (required string enum)
  Accepted values: `feat`, `bug`, `refactor`, `chore`, `research`, `workset`, `epic`
Return: plain text — the frontmatter template block followed by the matching
body template, extracted verbatim from `ticket-conventions.md` lines 64–199.

Example call: `tickets.template(type: "feat")`
Example return:
```
### Frontmatter

```yaml
---
title: <title>
...
---
```

### Body (actionable: `feat`, `bug`, `refactor`, `chore`)

```markdown
# <title>
...
```
```

## Contract Instructions

### Files to create / edit

1. **`agents-plugin-tool/internal/wsdoc/tickets_template.go`** (new file)
   - Package `wsdoc`
   - Export `TicketTemplate(typeStr string) (string, error)`
   - Maps type string → constant markdown skeleton string (inline Go constants, not
     another embed)
   - `feat`, `bug`, `refactor`, `chore` → identical "actionable" skeleton (they share
     one body template per ticket-conventions.md)
   - `research` → research skeleton
   - `workset` → workset skeleton
   - `epic` → epic skeleton
   - Unknown type → `fmt.Errorf("unknown ticket type %q; accepted: feat, bug,
     refactor, chore, research, workset, epic")`
   - Each return value includes: the shared Frontmatter block + the type-specific
     Body section, extracted verbatim from ticket-conventions.md lines 64–199.

2. **`agents-plugin-tool/internal/mcp/server.go`**
   - Add `case "tickets.template":` handler in the tool dispatch switch (near other
     `tickets.*` cases, around line 914).
   - Handler body:
     ```go
     typeStr, _ := params.Arguments["type"].(string)
     text, err := wsdoc.TicketTemplate(typeStr)
     return toolTextResponse(req.ID, text, err)
     ```
   - Add tool definition in the tools slice (near other `tickets.*` definitions):
     ```go
     {
         "name":        "tickets.template",
         "description": "Return the fill-in body skeleton for a given ticket type. Use at creation time instead of loading the full ticket-conventions document.",
         "inputSchema": map[string]any{
             "type": "object",
             "properties": map[string]any{
                 "type": stringProperty("Ticket category: feat, bug, refactor, chore, research, workset, or epic."),
             },
             "required": []string{"type"},
         },
     },
     ```

3. **`agents-plugin/runtime.json`** and **`agents-plugin-wsflow/runtime.json`**
   - Add entry in `"tools"` section:
     `"tickets.template": ">=0.30.6-dev <0.31.0"`
   - Insert after `"tickets.create"` entry.

### Skeleton content (from ticket-conventions.md lines 64–199, verbatim)

The return value for each type must include the **Frontmatter** block (lines 64–88)
followed by the type-specific **Body** block. Preserve fenced code block markers.

Type groups:
- `feat|bug|refactor|chore` → Frontmatter + "Body (actionable: feat, bug, refactor, chore)" (lines 90–116)
- `research` → Frontmatter + "Body (category = research)" (lines 125–137)
- `workset` → Frontmatter + "Workset body (category = workset)" (lines 141–168)
- `epic` → Frontmatter + "Epic body (category = epic)" (lines 170–199)

Extract these verbatim from ticket-conventions.md — do not paraphrase.

## Integration Test Instructions

New test file: `agents-plugin-tool/internal/mcp/tickets_template_test.go`

Tests to include:
- Each of the 7 accepted type values returns non-empty text and no error.
- `feat`, `bug`, `refactor`, `chore` all return identical content (same template).
- `research` return includes `## Background` and no `## Phases` heading.
- `workset` return includes `## Tickets` heading.
- `epic` return includes `## Child Tickets` heading.
- `feat` return includes `## Phases` heading.
- Unknown type (e.g. `"invalid"`) returns an error with "unknown ticket type".
- Empty type string returns an error (unknown or required).

Run command: `cd agents-plugin-tool && go test ./internal/mcp/... -run TestTicketTemplate`
Full suite: `cd agents-plugin-tool && go test ./internal/mcp/...`

## Implementation Strategy Decisions

- Templates inline as Go string constants per type in a new `wsdoc/tickets_template.go`
  file. No additional embed or rsrc file needed.
- `feat`/`bug`/`refactor`/`chore` map to one shared constant (they share a body template).
- Tool handler is minimal: extract `type`, call `wsdoc.TicketTemplate`, return text.
- Phase 1 does not remove any content from `ticket-conventions.md`.

## Rejected Alternatives

- **Separate rsrc template file**: adds parsing and embed dependency; inline constants
  are simpler and testable without filesystem.
- **Parse ticket-conventions.md at runtime**: fragile, couples tool to convention doc
  layout; inline constants are authoritative for Phase 1.

## Approach

1. Read ticket-conventions.md lines 64–199 verbatim for the four skeleton blocks.
2. Create `internal/wsdoc/tickets_template.go` with four const strings and
   `TicketTemplate(typeStr) (string, error)`.
3. Add `case "tickets.template":` in server.go dispatch + tool definition entry.
4. Add `"tickets.template"` to both runtime.json files.
5. Write test file covering all 7 types + error cases.
6. Run `go build ./... && go test ./internal/mcp/... ./internal/wsdoc/...`.
7. Run `go test ./internal/wsrsrc/...` to confirm no manifest delta (no rsrc changes).
8. Commit.

## Constraints

- Do not stage `install.sh`.
- Version range in runtime.json: `>=0.30.6-dev <0.31.0` (matches all other tools).
- Follow existing `toolTextResponse(req.ID, text, err)` return pattern.
- Do not read the ticket directly during implementation.

## Out of scope

- Phase 2: removing Templates section from `ticket-conventions.md`.
- Phase 3: updating `lead-write-ticket` playbook to call `tickets.template`.
- Adding `tickets.template` to `LeadToolNames()` or role allow-lists (not needed for
  Phase 1; can be addressed in Phase 3 if required).

## Details

Handler insertion point: `agents-plugin-tool/internal/mcp/server.go` — near line 914
(`case "tickets.create":`) for the case; near line 2673 for the tool definition.

Tool definition location in tools slice: after the `tickets.create` definition.

`TicketTemplate` signature:
```go
func TicketTemplate(typeStr string) (string, error)
```

## Verification Contract

- `go test ./internal/mcp/... -run TestTicketTemplate` — all assertions pass.
- `go test ./internal/mcp/...` — full suite green (no regressions).
- `go test ./internal/wsrsrc/...` — green (no manifest delta expected since no rsrc
  files changed in Phase 1).
- `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface` — green after both
  runtime.json files updated.

## References

- [Must] `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md` lines 64–199 — verbatim template content
- [Must] `agents-plugin-tool/internal/mcp/server.go` lines 809–914 — handler pattern; lines 2614–2680 — tool definition pattern
- [Must] `agents-plugin/runtime.json` — tool registration format
- [Must] `agents-plugin-wsflow/runtime.json` — tool registration format
- [Maybe] `agents-plugin-tool/internal/wsdoc/conventions.go` — `TicketTemplate` should follow the same package conventions
