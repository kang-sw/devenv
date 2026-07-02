---
title: agenda blob keys are not enumerable; add agenda_list or clear-all
sage-review: completed
completed: 2026-07-02
---

# agenda blob keys are not enumerable; add agenda_list or clear-all

## Context

Found during a v0.31.1 dogfooding pass. Each of `enter_proceed`,
`enter_implement`, `enter_sprint`, and `enter_salvage` plants an agenda blob
under its own implicit key. There is no way to enumerate which agenda keys
currently exist for a session. Clearing them required guessing the key names
(`proceed`/`implement`/`sprint`/`salvage`) by reading tool descriptions rather
than querying state directly. An agent that skims descriptions, or that
doesn't know all four `enter_*` variants exist, can easily orphan a blob with
no way to discover or clean it up later.

## Suggestion

Add an `agenda_list` tool to enumerate current agenda keys (and ideally a
short summary of each blob), and/or extend `agenda_clear` with an `all: true`
option to clear every agenda blob for the session without needing to name
each key individually.

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md`. Caller-visible change: new `agenda_list`
tool (or `agenda_clear(all: true)` mode) to enumerate/clear agenda blob keys
without guessing them from tool descriptions. Contract-first spec: no.

## Phases

### Phase 1: Add `agenda.list` and `agenda.clear(all: true)`

Implement both suggested primitives (not an either/or): a new `agenda.list`
tool to enumerate current agenda keys with a short per-blob summary, and an
`all: true` mode on the existing `agenda.clear` to clear every agenda blob for
the session without naming each key.

1. Store layer (`agents-plugin-tool/internal/mcp/session_state.go`): add
   `sessionStore.clearAllAgenda` (nils the whole `Agenda` map; no-op if
   already empty).
2. Handler layer (same file): extend `handleAgendaClear` to check an `all`
   bool arg first — when true, call `clearAllAgenda` and short-circuit before
   requiring `key`. Add `handleAgendaList`, which reads session state, and
   renders `- <key>: <summary>` lines sorted alphabetically (matching the
   existing alphabetical-key convention in `workflow_manual.go`'s session
   state rendering), or `no agenda blobs\n` when the agenda is empty. Add an
   `agendaSummary` helper: for object-shaped blobs, render `{key1, key2, ...}`
   from the top-level keys; otherwise fall back to a whitespace-collapsed raw
   preview. Both truncate to 80 chars.
3. Dispatch + schema (`agents-plugin-tool/internal/mcp/server.go`): add the
   `agenda.list` case to the tool-call switch and its tool schema (mirrors
   `agenda.set`/`agenda.clear` shape: `session_key` only, required). Extend
   the `agenda.clear` schema with an optional `all` boolean property and drop
   `key` from the required list (it is still required unless `all: true`,
   enforced in the handler).
4. Runtime contracts: add `"agenda.list": ">=0.31.1-dev <0.32.0"` next to the
   existing `agenda.set`/`agenda.clear` entries in both
   `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json` (the
   version-gated tool-availability list consumed by
   `cmd/ws-mcp` runtime-capabilities contract tests).
5. Spec (`ai-docs/spec/mcp-tools.md`, `260625-session-state-tools` anchor):
   document `agenda.clear(all: true)` and `agenda.list` alongside the existing
   `agenda.set`/`agenda.clear` freeform-primitives paragraph.
6. Tests (`agents-plugin-tool/internal/mcp/session_state_test.go`): add
   `TestStoreClearAllAgenda` (store-level clear-all, including no-op on an
   already-empty agenda) and `TestServeStdioAgendaListHandler` (handler-level:
   empty-agenda message, alphabetical multi-key listing with per-blob
   summaries, unknown-session-key error, and `agenda.clear(all: true)`
   end-to-end through the handler).

No new type/schema contract beyond the two additive tool-schema properties;
no changes to capability gating (`agenda.list` carries no
`session.`/`config.`/`mercenary.` prefix, so it is unrestricted for every role
exactly like the existing agenda tools).

### Result (f765a72) - 2026-07-02

Implemented exactly as planned above, direct-edit (no delegated plan
artifact — `enter.implement` verdict was `direct-edit`, `plan_depth: none`).

Verification:
- `cd agents-plugin-tool && go build ./...` — clean.
- `cd agents-plugin-tool && go test ./...` — all packages pass, including the
  new `TestStoreClearAllAgenda` and `TestServeStdioAgendaListHandler`, and the
  pre-existing `cmd/ws-mcp` runtime-capabilities contract tests (which needed
  the `runtime.json` updates in step 4 to stay green — they assert the tool
  surface exactly matches the contract file).
- No `gofmt` regressions introduced (a pre-existing, unrelated struct-literal
  alignment gap in `server.go` around line 2719 predates this change and was
  left untouched, since it's out of this ticket's scope).

Files changed:
- `agents-plugin-tool/internal/mcp/session_state.go`
- `agents-plugin-tool/internal/mcp/server.go`
- `agents-plugin-tool/internal/mcp/session_state_test.go`
- `agents-plugin/runtime.json`
- `agents-plugin-wsflow/runtime.json`
- `ai-docs/spec/mcp-tools.md`

No deviations from the plan.
