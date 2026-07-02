# Brief: 260620-ticket-status-tools

## Intent

Add two new MCP tools — `tickets.close` and `tickets.move` — to the ws MCP
server. Both tools perform an atomic frontmatter-write + `git add` + `git mv`
in a single operation, removing the staging footgun that arises when a plain
`Edit` before `git mv` leaves the edit unstaged. The tools also enforce
ticket-convention guards (reject unknown stems, invalid status targets,
re-close, date-prefix mutation) at the MCP layer instead of trusting lead
memory.

## Scope Boundary

Phase 1 only: MCP tools, wsdoc helpers, CLI parity, runtime.json updates for
both full `ws` and `wsflow` contracts, and the `mcp-tools.md` spec entry.

**Out of scope for Phase 1:**
- Playbook/convention doc rewiring (Phase 2)
- `create-ticket` MCP tool (260622 ticket)
- sage-review config registration (260622 Phase 3)

## Caller-Visible Contract

### `tickets.close(session_key, stem, status, resolution?)`

- `status` ∈ `{done, dropped}` (required)
- Writes `completed: YYYY-MM-DD` (status=done) or `dropped: YYYY-MM-DD`
  (status=dropped) into the frontmatter of the ticket at its current path
- Moves the file: `.done/` for done, `.dropped/` for dropped
- If `resolution` string is non-empty, appends `\n\n## Resolution (YYYY-MM-DD)\n\n<resolution>` to the file body before the move
- Atomic: write file → `git add <old-path>` → `git mv <old-path> <new-path>`
- Returns a short text summary: `closed: <stem> → .done/` or `.dropped/`
- Guard failures return `isError: true` tool responses (not JSON-RPC errors)

Guards:
- Unknown stem → error
- Already closed (stem exists in `.done/` or `.dropped/`) → error
- `status` not in `{done, dropped}` → error
- Any input that would mutate the `YYMMDD` date prefix → error

### `tickets.move(session_key, stem, to)`

- `to` ∈ `{idea, todo, ready}` (required)
- Moves the ticket file between active status directories
- Upward (idea→todo, idea→ready, todo→ready): checked against sage-review pre-condition (see below)
- Downward (ready→todo, ready→idea, todo→idea): allowed freely; ready→todo/idea returns a tip
- Returns a short text summary: `moved: <stem> idea → todo` etc.; on downward from ready, appends: `tip: This ticket had spec entries; clear spec:, spec-remove:, and review ## Spec Impact before re-promoting.`
- Atomic: write no frontmatter change needed for plain move; just `git add <old-path>` (needed to ensure any prior working-tree edit is staged) + `git mv <old-path> <new-path>`

Guards:
- Unknown stem → error
- `to` not in `{idea, todo, ready}` → error
- Stem already at target status → error
- Any input that would mutate the `YYMMDD` date prefix → error

Sage-review pre-condition (upward moves only):
- Read `sage_review` config key via `wsconfig.Resolver.Get(sessionKey, "sage_review")`
- If resolved value is empty or `off`: no-op, allow move
- If resolved value is `auto` or `ask`: read `sage-review` frontmatter field from the ticket
  - `pending` or `blocked`: return error: `sage-review: <value>; review must complete or be skipped before promoting`
  - `completed`, `skipped`, or absent field: allow move

### CLI parity

Both tools need CLI subcommands under `tickets` in `cmd/ws-mcp/main.go`, following
the same pattern as the existing `tickets.list`, `tickets.find`, `tickets.status`
CLI mirrors. The CLI commands must appear in `runtimeCapabilityCommandNames` in
`main.go` so the launcher contract test passes.

### Runtime contract

Add both tools to `agents-plugin/runtime.json` and
`agents-plugin-wsflow/runtime.json` under the `"tools"` key with version
`">=0.30.2-dev <0.31.0"`.

No new CLI commands need to appear in the `"commands"` section of both
runtime.json files — only the MCP tools section — UNLESS the existing
`tickets.*` CLI commands already appear in the wsflow `"commands"` section.
If they do, add `tickets.close` and `tickets.move` there too.

## Contract Instructions

### New: `agents-plugin-tool/internal/wsdoc/tickets_mutate.go`

Create a new file (keep mutation helpers separate from read-only discovery):

```go
package wsdoc

type TicketCloseOptions struct {
    TicketStem string
    Status     string // "done" | "dropped"
    Resolution string // optional
    Today      string // YYYY-MM-DD, caller-supplied for testability
}

type TicketMoveOptions struct {
    TicketStem  string
    To          string // "idea" | "todo" | "ready"
    SageReview  string // resolved sage_review config value ("" | "off" | "auto" | "ask")
    Today       string // for tip text, not currently used but kept for symmetry
}

type TicketMutateResult struct {
    OldPath string
    NewPath string
    Tip     string // non-empty when a follow-up action is advised
}

func TicketsClose(root string, runner GitRunner, opts TicketCloseOptions) (TicketMutateResult, error)
func TicketsMove(root string, runner GitRunner, opts TicketMoveOptions) (TicketMutateResult, error)
```

`GitRunner` is a minimal interface that the tests can mock:
```go
type GitRunner interface {
    RunGit(ctx context.Context, root string, args ...string) ([]byte, error)
}
```
Use `context.Background()` inside these functions; callers don't need cancellation yet.

**Internal helpers (unexported) in the same file:**

- `findTicketPath(root, stem string) (path string, status string, err error)` —
  scan `idea/`, `todo/`, `ready/`, `.done/`, `.dropped/` for `<stem>.md`; return
  (path, statusDir, nil) if found, error if not.
  Re-use the existing `ticketStemRE` validation.

- `writeFrontmatterField(path string, fields map[string]string) error` —
  read file, find `---`…`---` fences, update or append each key-value pair,
  write back. If a key already exists, replace its line in-place. If it does
  not exist, insert before the closing `---`. Preserve existing indented/nested
  content under other keys verbatim. Use `os.WriteFile` (not append); this is
  the full file overwrite path. Only supports scalar string values.

- `atomicGitMove(root string, runner GitRunner, oldPath, newPath string) error` —
  calls `runner.RunGit(ctx, root, "add", oldPath)`, then
  `runner.RunGit(ctx, root, "mv", "--force", oldPath, newPath)`.
  Both paths are repo-relative (forward slash).

**No changes to `frontmatter.go`** — the read helper stays read-only.

### Modify: `agents-plugin-tool/internal/mcp/server.go`

1. In `callTool` dispatch, add cases `"tickets.close"` and `"tickets.move"`:
   - Both call `s.resolveToolRoot(params.Arguments, params.Meta)` for root
   - Both extract `session_key` from `params.Arguments` for sage-review config
   - `tickets.close`: parse `stem`, `status`, `resolution` args; call
     `wsdoc.TicketsClose(root, wsgit.ExecRunner{}, opts)`
   - `tickets.move`: before calling `wsdoc.TicketsMove`, read `sage_review`
     config value via:
     ```go
     adapter := sessionConfigAdapter{s: s.sessions}
     r := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)
     resolved, _ := r.Get(sessionKey, "sage_review")
     sageReview := resolved.Value
     ```
     Then call `wsdoc.TicketsMove(root, wsgit.ExecRunner{}, opts)`.
   - Return `toolTextResponse(req.ID, result.formatted(), err)` where
     `formatted()` produces the short summary + optional tip.

2. In `tools()` schema list, add two entries after `"tickets.status"`:
   - `tickets.close` schema
   - `tickets.move` schema
   Both require `session_key` and `stem` (align with existing tickets.* pattern).

### Modify: `agents-plugin-tool/cmd/ws-mcp/main.go`

Add CLI subcommands `tickets close` and `tickets move` (or `tickets.close` /
`tickets.move` matching the existing hyphenated-vs-dot CLI naming convention).
Look at how existing `tickets.list`/`tickets.find`/`tickets.status` CLI mirrors
are wired — follow the same pattern exactly.

Add `tickets.close` and `tickets.move` (or their CLI mirror names) to
`runtimeCapabilityCommandNames` in the same file.

### Modify: `agents-plugin/runtime.json`

Add to `"tools"`:
```json
"tickets.close": ">=0.30.2-dev <0.31.0",
"tickets.move": ">=0.30.2-dev <0.31.0"
```

Add to `"commands"` if and only if the existing `tickets.*` commands are present
there (they are: `"tickets.list"`, `"tickets.find"`, `"tickets.status"` appear in
both runtime.json files).

### Modify: `agents-plugin-wsflow/runtime.json`

Same additions as `agents-plugin/runtime.json`. This file uses
`"runtime_capabilities": {"match": "exact"}`, so the
`TestRuntimeCapabilitiesCommandReportsWsflowContractSurface` test will catch any
mismatch.

### Modify: `ai-docs/spec/mcp-tools.md`

Under `## Ticket Discovery Tools {#260505-ticket-discovery-tools}`, add a new
subsection or paragraph after the existing `tickets.find` / `tickets.status`
description:

```markdown
`tickets.close` moves a ticket to `.done/` (status=done) or `.dropped/`
(status=dropped), writing the appropriate `completed:` or `dropped:` date into
frontmatter and optionally appending a `## Resolution (YYYY-MM-DD)` body section.
The operation is atomic: frontmatter write, `git add`, and `git mv` happen as
one staged change set. {#260620-ticket-close-tool}

`tickets.move` moves a ticket along the `idea ↔ todo ↔ ready` axis. Downward
moves from `ready/` return a tip to clear spec frontmatter before re-promoting.
Upward moves check the `sage_review` config key and the ticket's `sage-review`
frontmatter field when the config is enabled; a `pending` or `blocked` field
blocks the promotion. {#260620-ticket-move-tool}
```

## Integration Test Instructions

**New file: `agents-plugin-tool/internal/wsdoc/tickets_mutate_test.go`**

This file lives in `package wsdoc` and uses `mustWrite` (from
`project_tree_test.go`) plus a `mockGitRunner` that records calls.

Required tests (TDD — write before implementing):

1. **`TestTicketsCloseMovesDoneWithCompletedDate`** — close a `todo/` ticket to
   done; verify: completed field written in file, file moved to `.done/`, git add
   + git mv called in order with correct paths.

2. **`TestTicketsCloseMovesDroppedWithDroppedDate`** — close a `ready/` ticket to
   dropped; verify: dropped field written, moved to `.dropped/`.

3. **`TestTicketsCloseAppendsResolutionSection`** — close with non-empty
   `resolution`; verify `## Resolution (YYYY-MM-DD)` appended to body.

4. **`TestTicketsCloseRejectsAlreadyClosedTicket`** — ticket is in `.done/`;
   verify error returned, no git calls.

5. **`TestTicketsCloseRejectsUnknownStem`** — stem not found; verify error.

6. **`TestTicketsCloseRejectsInvalidStatus`** — status="wip"; verify error.

7. **`TestTicketsMoveUpwardIdea→Todo`** — move `idea/` → `todo/`; verify git add
   + git mv called, result OldPath/NewPath correct.

8. **`TestTicketsMoveDownwardReady→TodoReturnsTip`** — move `ready/` → `todo/`;
   verify result Tip contains "spec".

9. **`TestTicketsMoveRejectsSameStatus`** — ticket already at target; error.

10. **`TestTicketsMoveUpwardBlockedBySageReview`** — sage_review="auto",
    frontmatter `sage-review: pending`; upward move returns error mentioning
    "sage-review".

11. **`TestTicketsMoveUpwardPassesSageReviewCompleted`** — sage_review="auto",
    frontmatter `sage-review: completed`; upward move succeeds.

12. **`TestTicketsMoveUpwardPassesSageReviewAbsent`** — sage_review="auto",
    no `sage-review` field; upward move succeeds (absent = pass).

13. **`TestTicketsMoveUpwardPassesSageReviewConfigOff`** — sage_review="off",
    frontmatter `sage-review: pending`; upward move succeeds (config off = no-op).

14. **`TestTicketsMoveUpwardPassesSageReviewConfigAbsent`** — sage_review="",
    frontmatter `sage-review: pending`; upward move succeeds (config absent = off).

Run command: `cd agents-plugin-tool && go test ./internal/wsdoc/ ./internal/mcp/ ./cmd/ws-mcp/`

The two launcher contract tests that must stay green:
- `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`
- `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`

## Implementation Strategy Decisions

- **New file `tickets_mutate.go`** rather than extending `tickets.go`: keeps
  mutation separate from the read-only discovery surface; consistent with how
  `spec_tools.go` is separate from `spec_discovery.go`.

- **`GitRunner` interface in wsdoc**: the wsdoc package must not import `wsgit`
  (it is a higher-level domain package); define a minimal local interface
  `GitRunner` with the same signature as `wsgit.Runner`. Pass `wsgit.ExecRunner{}`
  from the MCP server layer. This avoids a circular import.

- **`writeFrontmatterField` operates on the fences, not YAML parsing**: the
  existing `frontmatter()` is hand-rolled and doesn't use a YAML library. The
  write helper follows the same minimal approach: find `---`…`---`, string-scan
  for key lines, replace or append. No YAML library needed.

- **`atomicGitMove` always does `git add` then `git mv`**: even when the caller
  only does a plain directory move with no frontmatter change, `git add` is
  harmless on a clean file and preserves the invariant that any working-tree
  edit preceding the move is always staged first.

- **Today date is caller-supplied** for both options structs: makes tests
  deterministic without mocking `time.Now`.

- **sage-review config lookup inline in server.go**: construct a `wsconfig.Resolver`
  inline for the `tickets.move` dispatch case (same pattern used by `config.show`
  and `playbook.render`). Do not add a new helper or field to `Server`; the
  resolver is cheap to construct.

## Rejected Alternatives

- **Using `os/exec` directly in wsdoc** for git calls: rejected because wsdoc
  would then be impossible to test without a real git repo; the `GitRunner`
  interface pattern is already established in `wsgit`.

- **Updating `frontmatter.go`** to add write-back: rejected to keep that file
  read-only; a separate `tickets_mutate.go` is cleaner and avoids accidental
  reads turning into writes.

- **YAML library for frontmatter write-back**: rejected; the existing parser is
  hand-rolled and only needs scalar field updates; a full YAML library is
  over-engineering for two string fields.

- **Committing inside the tool**: explicitly rejected by ticket decision — the
  tool stages only, never commits. Lead commits with `ws/git.commit` to author
  the `## AI Context` / `## Ticket Updates` message.

## Approach

1. TDD: write `tickets_mutate_test.go` with all 14 tests; verify they compile but
   fail (or have no implementation to call yet).
2. Implement `tickets_mutate.go`: `findTicketPath`, `writeFrontmatterField`,
   `atomicGitMove`, `TicketsClose`, `TicketsMove`.
3. Run `go test ./internal/wsdoc/` — all 14 new tests must pass.
4. Add MCP dispatch + schema in `server.go`.
5. Add CLI mirrors + `runtimeCapabilityCommandNames` entries in `cmd/ws-mcp/main.go`.
6. Update both `runtime.json` files.
7. Update `ai-docs/spec/mcp-tools.md`.
8. Run `go test ./...` — full suite green including both contract tests.

## Constraints

- **Staging footgun must be impossible**: `atomicGitMove` always runs `git add`
  before `git mv`; never `git mv` before `git add`.
- **No commit**: the tools stage and return; they never call `git commit`.
- **Session key required**: both tools call `resolveToolRoot` which enforces the
  mandatory `session_key` contract. Do not add a `root` parameter.
- **Stem immutability**: the `YYMMDD` date prefix must not be changed by any move.
  The stem is taken from the filename; the destination filename is always
  `<same-stem>.md`. Do not rewrite the stem.
- **wsflow exact-match**: `agents-plugin-wsflow/runtime.json` uses
  `"runtime_capabilities": {"match": "exact"}` — every tool added to the full
  runtime.json must also be added to wsflow unless it is mercenary or exec.
  `tickets.close` and `tickets.move` are not mercenary or exec, so they go in
  both contracts.
- **`GitRunner` interface must not import `wsgit`**: define it locally in
  `tickets_mutate.go`; pass a compatible concrete runner from the server layer.

## Out of Scope

- Phase 2 playbook/convention doc rewiring.
- `create-ticket` MCP tool (260622).
- sage-review config key registration (260622 Phase 3); sage_review config
  absent means off — handled by treating empty Resolver.Get value as off.
- Rollback / undo tool.
- CLI `--help` text or documentation beyond the schema description field.

## Details

### `writeFrontmatterField` algorithm

```
Read file bytes.
Find first "---\n" line (line 0).
Find second "---\n" or "---" line (end fence index).
For each (key, value) in fields map:
  Scan lines[1:endFence] for a line matching `^<key>:` (no leading space).
  If found: replace that line with `<key>: <value>`.
  If not found: insert `<key>: <value>` before the end-fence line.
Write back the reassembled text.
```

For the close operation, this inserts `completed:` or `dropped:` with today's
date. For existing keys, it replaces in-place (idempotent).

### Resolution section append

After `writeFrontmatterField`, if `resolution` is non-empty, append to the file:
```
\n\n## Resolution (YYYY-MM-DD)\n\n<resolution text>\n
```
`\n\n` before the heading ensures a blank line separator even if the file does
not already end with a newline.

### Status directory mapping

```
idea/  → "idea-docs/tickets/idea/"
todo/  → "ai-docs/tickets/todo/"
ready/ → "ai-docs/tickets/ready/"
.done/ → "ai-docs/tickets/.done/"
.dropped/ → "ai-docs/tickets/.dropped/"
```

`findTicketPath` scans all five. Active move targets: idea, todo, ready.
Close targets: .done, .dropped.

### Downward-from-ready tip text

When `to ∈ {todo, idea}` and the ticket's current status is `ready`:
```
tip: This ticket had spec entries; clear spec:, spec-remove:, and review ## Spec Impact before re-promoting.
```

### MCP tool schema outlines

`tickets.close`:
```json
{
  "name": "tickets.close",
  "description": "Close a ticket to .done/ or .dropped/, writing the dated frontmatter field and optionally appending a ## Resolution section. Stages the change set atomically (frontmatter write → git add → git mv); does not commit.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "session_key": "...",
      "stem": "Ticket stem (YYMMDD-category-name).",
      "status": "Close target: done or dropped.",
      "resolution": "Optional resolution text appended as ## Resolution (today) section."
    },
    "required": ["session_key", "stem", "status"]
  }
}
```

`tickets.move`:
```json
{
  "name": "tickets.move",
  "description": "Move a ticket along the idea ↔ todo ↔ ready axis. Upward moves check the sage_review config and ticket sage-review frontmatter field. Downward moves from ready/ return a spec-cleanup tip. Stages atomically; does not commit.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "session_key": "...",
      "stem": "Ticket stem.",
      "to": "Target status: idea, todo, or ready."
    },
    "required": ["session_key", "stem", "to"]
  }
}
```

## Verification Contract

- `go test ./internal/wsdoc/` — all 14 new tests green.
- `go test ./internal/mcp/` — no regressions.
- `go test ./cmd/ws-mcp/` — both launcher contract cross-checks green:
  - `TestRuntimeCapabilitiesCommandReportsLauncherContractSurface`
  - `TestRuntimeCapabilitiesCommandReportsWsflowContractSurface`
- `go test ./...` — full suite green.
- Manual spot-check: on a test repo, call `tickets.close` on a `todo/` ticket;
  verify frontmatter updated, file in `.done/`, `git status` shows one staged
  rename (no unstaged remainder).

## References

- `[Must] agents-plugin-tool/internal/wsdoc/tickets.go` — existing read-only ticket logic; reuse `ticketStemRE`, `normalizeTicketStatus`, `TicketInfo`, `readTicket`
- `[Must] agents-plugin-tool/internal/wsdoc/frontmatter.go` — read-only parse helper; write helper is new
- `[Must] agents-plugin-tool/internal/mcp/server.go` — dispatch/schema patterns; existing `tickets.*` cases; `resolveToolRoot`, `sessionConfigAdapter` usage
- `[Must] agents-plugin/runtime.json` — add `tickets.close` + `tickets.move` to tools + commands
- `[Must] agents-plugin-wsflow/runtime.json` — add same; exact-match contract
- `[Must] agents-plugin-tool/cmd/ws-mcp/main.go` — CLI mirrors; `runtimeCapabilityCommandNames`
- `[Must] ai-docs/spec/mcp-tools.md` — add entries under ticket tools section
- `[Maybe] agents-plugin-tool/internal/wsgit/git.go` — wsgit.Runner interface signature for GitRunner definition
- `[Maybe] agents-plugin-tool/internal/wsconfig/resolver.go` — Resolver.Get signature for sage_review config read
