# Plan: Bug: enter.proceed status-report route is dead code — Phase 1: Remove the dead status-report route

## Relevant Ticket Contract
- Decision: Remove the `status-report` `NEXT:` enum value and its `proceedNextInstruction` case; do not implement a routing branch for it.
- Phase 1 scope: delete the `status-report` case from `proceedNextInstruction` in `agents-plugin-tool/internal/mcp/proceed_resolver.go`, and update or remove the direct assertion at `agents-plugin-tool/internal/mcp/session_state_test.go:1196-1197` that depends on it. No routing branch anywhere ever produces this value, so no other call site changes.
- Verification boundary: `go test ./...` in `agents-plugin-tool/` passes; grep for `status-report`/`status_report` across the repo turns up no remaining production or test references to the removed case.
- Explicit non-goal/out-of-scope note: the `status_report` (underscore) fact enum value on the `category` fact is unrelated caller input shape and is out of scope for this removal.
- Spec Impact: none — route was never reachable at runtime, no spec documents it as observable contract.

## Out of Scope
- Implementing a routing branch that produces `status-report` (Option 2, rejected by Decision).
- The `category` fact's `status_report` enum value in `proceed_resolver.go:273` and `server.go:2991` — distinct, unrelated caller-input shape; must remain untouched.
- Any documentation ticket about the original intent behind the reserved route (ticket says this may be a separate follow-up ticket, not part of this phase).

## Codebase Findings
- `agents-plugin-tool/internal/mcp/proceed_resolver.go#L349-L365` — `proceedNextInstruction(next string)` switch statement; the `case "status-report":` at lines 358-359 is the sole production site to delete. Confirmed via grep that no routing logic elsewhere in the file (or repo) ever assigns `next = "status-report"`.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1196-1198` — standalone `if` assertion (not part of a table-driven test case) directly calling `proceedNextInstruction("status-report")` and checking its returned string. Sits right after the table-driven test loop closes (line 1194) and before the next test function `TestProceedInputRejectsNonStringFactTypes` begins (line 1201). Safe to delete as a self-contained block; no other code depends on the `got` variable it declares.
- `agents-plugin-tool/internal/mcp/proceed_resolver.go#L273` — `category` fact enum list includes `"status_report"` (underscore). Confirmed unrelated per ticket note; do not touch.
- `agents-plugin-tool/internal/mcp/server.go#L2991` — schema property enum for `category` fact, also lists `"status_report"` (underscore); same unrelated shape, do not touch.
- Repo-wide grep (`agents-plugin-tool/`, `ai-docs/`, `agents-plugin/`) confirms the only two hyphenated `"status-report"` production/test sites are the two listed above; other hits are historical ticket docs (`ai-docs/tickets/...`) which are not code and are expected to retain historical mentions.

## Implementation Plan
1. In `agents-plugin-tool/internal/mcp/proceed_resolver.go`, delete the `case "status-report":` block (lines 358-359) from the `proceedNextInstruction` switch, leaving `lead-implement`, `lead-write-ticket`, `lead-discuss`, `stop`, and `default` cases intact.
2. In `agents-plugin-tool/internal/mcp/session_state_test.go`, delete the standalone `if got := proceedNextInstruction("status-report"); ... { t.Fatalf(...) }` block (lines 1196-1198), leaving the closing brace of `TestResolveProceed` (or equivalent enclosing test function) at line 1199 intact.
3. Run `gofmt`/build sanity via `go vet ./...` or `go build ./...` in `agents-plugin-tool/` to confirm no syntax breakage from the two deletions.

## Verification Plan
- `cd agents-plugin-tool && go test ./...` — must pass with no failures.
- `grep -rn "status-report" agents-plugin-tool/` — must return no results (production or test code) after the edit.
- `grep -rn "status_report" agents-plugin-tool/` — must still show the two unrelated `category` enum sites (`proceed_resolver.go:273`, `server.go:2991`) untouched, confirming scope discipline.

## Escalations
- None.
