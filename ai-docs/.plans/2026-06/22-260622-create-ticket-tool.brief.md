# Brief: 22-260622-create-ticket-tool

## Intent

Add a `ws/tickets.create(session_key, stem, initial_state)` MCP tool and matching CLI
mirror that creates a dated ticket stub file, writes minimal frontmatter (`title: ""`
plus `sage-review: pending` for `todo/+`), and returns `{path, tip}`. Phase 1 of
`260622-feat-sage-review-ticket-gate`.

Note: MCP tool name is `tickets.create` (dotted, consistent with `tickets.close`/
`tickets.move`). The ticket Decisions section uses `ws/create_ticket` notation
(underscore) — treat `tickets.create` as the correct name throughout this brief.

## Scope Boundary

Phase 1 only. Do not touch Phase 2 (reviewer playbooks, `lead-write-ticket` judge gate)
or Phase 3 (`sage_review*` config keys). No git staging in the create logic — the
created file is unstaged; callers commit via `git.commit`.

## Caller-Visible Contract

`ws/tickets.create(session_key, stem, initial_state)`:

- `stem`: caller-supplied semantic portion of the ticket name (e.g., `feat-foo-bar`).
- `initial_state`: `"idea"` | `"todo"` | `"ready"`. Terminal states (`"done"`,
  `"dropped"`) and empty string are rejected with a descriptive error.
- Writes `ai-docs/tickets/<status>/<YYMMDD>-<stem>.md` where `YYMMDD` is today's date.
- Returns `{path, tip}`:
  - `idea/` tip: `"promoting to 'todo/' will trigger sage review."`
  - `todo/` or `ready/` tip: `"run sage review before promoting further."`
- Error if the target path already exists.
- Error if `stem` is empty.

Stub frontmatter for `idea/`:
```yaml
---
title: ""
---
```

Stub frontmatter for `todo/` or `ready/`:
```yaml
---
title: ""
sage-review: pending
---
```

## Contract Instructions

### New file: `agents-plugin-tool/internal/wsdoc/ticket_create.go`

```go
type TicketCreateOptions struct {
    Stem         string // semantic stem (no date prefix)
    InitialState string // "idea" | "todo" | "ready"
    Today        string // YYMMDD; if empty, use time.Now().Format("060102")
}

type TicketCreateResult struct {
    Path string
    Tip  string
}

func TicketCreate(root string, opts TicketCreateOptions) (TicketCreateResult, error)
```

Implementation:
- Validate `opts.Stem` non-empty and `opts.InitialState` ∈ {idea, todo, ready}.
- Compute full stem: `<Today>-<Stem>`.
- Map `initial_state` → status directory: `idea` → `idea/`, `todo` → `todo/`,
  `ready` → `ready/`.
- Destination: `ai-docs/tickets/<status>/<full-stem>.md` (relative to `root`).
- Call `os.MkdirAll(filepath.Dir(destAbs), 0o755)` before writing.
- Reject if file already exists (use `os.Stat` check or `O_EXCL`).
- Write stub with `os.WriteFile(..., 0o644)`.
- Set tip based on `initial_state`: `idea` → "promoting to 'todo/' will trigger sage
  review."; `todo` or `ready` → "run sage review before promoting further."
- Return `{Path: rel-path-from-root, Tip: tip}`.

Existing mechanisms to reuse:
- `ticketRelPath(statusDir, stem)` in `tickets_mutate.go` (unexported): builds
  `ai-docs/tickets/<dir>/<stem>.md` with forward slashes. Reuse to compute return path;
  pass `<Today>-<Stem>` as stem.
- `statusDirs` map in `tickets_mutate.go`: `idea/todo/ready` → same dir names; reuse
  for initial_state → directory mapping.
- `wsdoc.writeFrontmatterField` is NOT used here (new file write, not a field update).
  Use `filepath.Join`, `os.MkdirAll`, `os.WriteFile`, `os.Stat` from stdlib only.

Do NOT call any git commands. Do NOT import wsgit.

### `agents-plugin-tool/internal/wsdoc/ticket_create_test.go`

Pure logic tests (TDD):
- `TestTicketCreateIdea`: creates idea stub, verifies frontmatter has `title: ""`
  and NO `sage-review` field.
- `TestTicketCreateTodo`: creates todo stub, verifies `title: ""` and
  `sage-review: pending`.
- `TestTicketCreateReady`: creates ready stub, verifies same as todo.
- `TestTicketCreateTerminalState`: `"done"` and `"dropped"` return non-nil error.
- `TestTicketCreateEmptyStem`: empty stem returns non-nil error.
- `TestTicketCreateDuplicateFile`: second call with same today+stem returns error.
- `TestTicketCreateDatePrefix`: `Today = "260101"`, verify path starts with
  `ai-docs/tickets/idea/260101-`.

Use `t.TempDir()` as root for all tests.

### `agents-plugin-tool/internal/mcp/server.go`

- Add `tickets.create` to `tools()` schema with params:
  ```
  session_key (string, required, description: "ws session key")
  stem        (string, required, description: "semantic ticket stem without date prefix")
  initial_state (string, required, description: "ticket status: idea | todo | ready")
  ```
- Add dispatch case in `callTool` switch for `"tickets.create"` following `tickets.move`
  pattern (simpler: no config lookup needed, mirror `tickets.close` dispatch shape):
  - Extract params, call `resolveToolRoot(session_key)` for root, call
    `wsdoc.TicketCreate(root, opts)`, return `toolTextResponse(req.ID, FormatTicketCreate(res), err)`.
- Add `"tickets.create"` to `rootAwareToolSchemaRequiresSessionKey` switch.

### `agents-plugin-tool/internal/mcp/format.go`

Add:
```go
func FormatTicketCreate(res wsdoc.TicketCreateResult) string
```
Returns:
```
Created <path>
Tip: <tip>
```

### `agents-plugin-tool/cmd/ws-mcp/main.go`

Under the `tickets` command group, add:
```
tickets create <stem> <initial_state>
```
- Parse args, call `wsdoc.TicketCreate(root, opts)` (root from repo root helper),
  print `FormatTicketCreate(res)`.
- Add `"tickets.create"` (dotted form, NOT argv form) to `runtimeCapabilityCommandNames`.
  Existing entries are `tickets.close`, `tickets.move`; the new entry must be `tickets.create`
  to satisfy the exact-match contract test.

### `agents-plugin/runtime.json`

Add in the `"tools"` section:
```json
"tickets.create": ">=0.30.2-dev <0.31.0"
```
Add in the `"commands"` section:
```json
"tickets.create": ">=0.30.2-dev <0.31.0"
```
Both keys use the dotted form `tickets.create` (consistent with `tickets.close`, `tickets.move`).

### `agents-plugin-wsflow/runtime.json`

Same 2 insertions (tools + commands) — this file uses `"match": "exact"`, so the
entry must match the full ws tool surface exactly. Both keys are `tickets.create`.

## Integration Test Instructions

Run from `agents-plugin-tool/`:
```bash
# 1. Unit tests
go test ./internal/wsdoc/... -v -run TestTicketCreate

# 2. Manifest regeneration (order matters)
WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -run TestGenerateRealManifest -v
WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror

# 3. Full suite
go test ./...
```

Pass criteria:
- All `TestTicketCreate*` cases pass.
- `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface` PASS (wsflow
  exact-match enforces `agents-plugin-wsflow/runtime.json` matches full surface).
- Full `go test ./...` green.

## Implementation Strategy Decisions

- New `ticket_create.go` file (not added to `tickets_mutate.go`): create is a distinct
  operation; keeping it separate preserves the mutate file's focus on close/move.
- `Today` as explicit parameter for testability (same pattern as `TicketCloseOptions`).
- No git staging: the tool creates the file only; caller commits via `ws/git.commit`.
- Minimal frontmatter: `title: ""` + `sage-review: pending` for `todo/+`. No other
  fields. Callers fill in content via `lead-write-ticket`.
- Empty string title `""` is the canonical placeholder (parseable YAML, unambiguous
  stub signal).
- No `GitRunner` interface needed (no git calls in the logic layer).
- Error on existing file: `TicketCreate` is not idempotent; callers should not
  silently overwrite existing tickets.

## Rejected Alternatives

- Adding to `tickets_mutate.go`: create is not a mutation of an existing ticket;
  separate file is cleaner.
- Auto-generating a title from stem: out of scope; callers fill in content.
- Requiring git stage: callers use `git.commit`; create is file-system only.
- Omitting `title` field from stub: leads to invalid frontmatter by convention.

## Approach

1. Write `ticket_create_test.go` (TDD: tests first for pure logic).
2. Write `ticket_create.go` implementing `TicketCreate` until tests pass.
3. Add `FormatTicketCreate` to `format.go`.
4. Add `tickets.create` to `tools()` + `callTool` + `rootAwareToolSchemaRequiresSessionKey`
   in `server.go`.
5. Add `tickets create` CLI mirror in `main.go` + `runtimeCapabilityCommandNames`.
6. Update both `runtime.json` files.
7. Run manifest regen (3-step sequence).
8. Run full `go test ./...`; verify `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`
   passes.
9. Commit.

## Constraints

- `sage-review: pending` written ONLY for `initial_state` `"todo"` or `"ready"`.
- `idea/` stub MUST NOT include `sage-review:` field.
- Terminal states (`done`, `dropped`) MUST return an error (not create files in
  `.done/` or `.dropped/`).
- Manifest regen order: `WSRSRC_REGEN` → `WS_REGEN_MANIFEST` → `WS_REGEN_WSFLOW_RSRC`.
  Skipping or reordering causes stale manifest test failures.
- wsflow `runtime.json` uses `"match": "exact"`: every tool in the full
  `agents-plugin/runtime.json` must appear in wsflow too. The `tickets.create` entry
  (in both tools and commands sections) must be added to both files.

## Out of Scope

- Phase 2: reviewer playbooks, `lead-write-ticket` sage-gate.
- Phase 3: `sage_review*` config keys in `wsconfig`.
- `parent:` field inference.
- Any body content beyond the frontmatter block.
- Saga review invocation.
- `spec:` or any other frontmatter field.

## Details

### Frontmatter format

Idea stub (`initial_state = "idea"`):
```
---
title: ""
---
```

Todo/Ready stub (`initial_state = "todo"` or `"ready"`):
```
---
title: ""
sage-review: pending
---
```

No trailing newline conventions are enforced; use standard `os.WriteFile` with a
trailing newline after the closing `---`.

### Tip strings (exact)

- `initial_state = "idea"`:
  `"promoting to 'todo/' will trigger sage review."`
- `initial_state = "todo"` or `"ready"`:
  `"run sage review before promoting further."`

### Return path convention

Return the path relative to `root`, using forward slashes (consistent with other
`TicketMutateResult.NewPath` convention in `tickets_mutate.go`).

## Verification Contract

1. `TestTicketCreateIdea`: frontmatter contains `title: ""`, does NOT contain
   `sage-review`.
2. `TestTicketCreateTodo`: frontmatter contains `sage-review: pending`.
3. `TestTicketCreateReady`: same as Todo.
4. `TestTicketCreateTerminalState`: error for `"done"` and `"dropped"`.
5. `TestTicketCreateEmptyStem`: error.
6. `TestTicketCreateDuplicateFile`: error on second call.
7. `TestTicketCreateDatePrefix`: returned path starts with `ai-docs/tickets/idea/260101-`.
8. `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`: PASS.
9. Full `go test ./...`: green, no new warnings in edited files.

## References

- `[Must] ai-docs/mental-model/mcp-runtime.md` — add-MCP-tool recipe,
  rootAwareToolSchemaRequiresSessionKey behavior, wsflow exact-match contract
- `[Must] agents-plugin-tool/internal/wsdoc/tickets_mutate.go` — existing logic
  layer pattern (options struct, result struct, GitRunner interface shape)
- `[Must] agents-plugin-tool/internal/mcp/server.go` — tools()/callTool/
  rootAwareToolSchemaRequiresSessionKey registration points
- `[Must] agents-plugin/runtime.json` — runtime capability entries pattern
- `[Must] agents-plugin-wsflow/runtime.json` — wsflow exact-match entries
- `[Maybe] agents-plugin-tool/internal/mcp/format.go` — FormatTicketMutate pattern
- `[Maybe] agents-plugin-tool/cmd/ws-mcp/main.go` — tickets group CLI mirror pattern
- `[Maybe] agents-plugin-tool/internal/wsdoc/ticket_create_test.go` — does not exist
  yet; implementer creates it
