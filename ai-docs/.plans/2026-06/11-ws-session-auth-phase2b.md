# Plan: ws-session-auth-phase2b (source survey)

> Authored by the lead from targeted recon, not the `plan-populator-survey`
> agent — same sanctioned fallback as Phase 2a (idea ticket
> `260611-bug-agent-context-exhaustion-opaque-failure`). The 2b source surface is
> small and fully anchored below; the EDIT remains delegated to the implementer.

## Feasibility Verdict
FEASIBLE — straightforward deletion with two concentrated chokepoints
(`runnerForBackend` for gemini, the `subquery` dispatch+gating cluster in
`server.go`). No new code paths; the `Runner` interface absorbs the gemini
removal without signature change. No `[escalate-to-lead]`.

## Touch-Point Map (file:line anchors at d638009b base)

### A. Gemini runner removal
- `internal/wsagent/gemini.go` — DELETE whole file (389 lines, `GeminiRunner`).
- `internal/wsagent/gemini_test.go` — DELETE whole file.
- `internal/wsagent/codex.go:51-52` — remove `case "gemini": return GeminiRunner{}, nil`
  from `runnerForBackend`. Keep `Runner` (16-18), `RunnerRequest` (20-34),
  `RunnerResult` (36-43), and cases `"", "codex"` (47-48) + `"claude"` (49-50).
  The `default` (53-54) already returns `unsupported agent backend %q` — that now
  covers `gemini`.
- `internal/wsagent/agent.go:1658` — backend-iteration list
  `[]string{"codex", "claude", "gemini"}` → drop `"gemini"`. Inspect the loop
  body (likely doctor/version detection) to confirm no gemini-only branch remains.
- `internal/wsconfig/config.go` — remove gemini from harness detection (215-216:
  `strings.Contains(value, "gemini")`), alias map (348-349: `case "gemini"`), and
  the error message (363: `"harness must be codex, claude, gemini, or default"` →
  drop `gemini`).
- `internal/wsconfig/config_test.go` — adapt/delete gemini harness cases.
- `cmd/ws-mcp/main.go:348` — harness flag help string drop `gemini`.

### B. Subquery runtime removal
- `internal/wsagent/subquery.go` — DELETE whole file (`SubquerySystemPrompt`).
- `internal/wsagent/agent.go:207` — DELETE `SubqueryOptions` struct.
- `internal/wsagent/agent.go:1150` — DELETE `func (m Manager) Subquery(...)`.
- `internal/wsagent/agent.go:1164` — the `SubquerySystemPrompt` ref dies with the
  method body.
- `internal/mcp/server.go`:
  - `2252-2253` — DELETE the `subquery` tool schema entry.
  - `765-775` — DELETE the `case "subquery":` dispatch (calls
    `wsagent.NewManager(...).Subquery(...)`).
  - `311-312` — DELETE the `subqueryAgentAccessAllowed` guard block.
  - `2714-2729` — DELETE `func (s *Server) subqueryAgentAccessAllowed(...)`.
  - `2730-2735` — DELETE `func isSubqueryAgentTool(...)`.
  - `2566` — DELETE the `if isSubqueryAgentTool(name) {...}` branch (inspect: the
    enclosing func returns a profile-gate decision; ensure the remaining branch
    is still correct after removal).
  - `2571` — remove `&& name != "subquery"` from the keyless-gate predicate.
  - `2642` — remove `"subquery"` from the case list.
- `cmd/ws-mcp/main.go`:
  - `43-45` — DELETE `case "subquery":` dispatch.
  - `74` — usage line: drop `subquery` from the command list.
  - `254` — drop `"subquery"` from the command slice.
  - `269` — remove `|| command == "subquery"` from the agent-command predicate.
  - `284-303` — DELETE `func subquery(args []string)`.
- `internal/mcp/server_test.go`, `internal/wsagent/agent_test.go`,
  `cmd/ws-mcp/main_test.go` — delete subquery-only tests; adapt shared helpers.

### C. Exploration coupling
- `internal/mcp/playbook_tools.go:24/29/35` — `"ExploreAgent"` template-var
  substitutions (`"the Explore agent"` / `"a search agent"` /
  `"an exploration agent"`). INSPECT: these are tiered template substitutions for
  playbook rendering, likely NOT the subquery spawn path. Remove ONLY if the
  `ExploreAgent` var is orphaned after subquery deletion (grep its template
  usage); otherwise LEAVE — playbook rendering for native subagents is not a 2b
  target. `internal/mcp/playbook_tools_test.go` follows the same rule.

## Sequence (commit per logical unit)
1. Remove gemini runner impl + dispatch case + config alias + iteration list
   (section A). `git mv 260524-bug-wsstore-ci-sqlite-busy` → `.dropped/` in this
   commit (wsstore actor records that drove the CI sqlite-busy are gone; the
   in-memory map landed in 2a). Build/vet/test.
2. Remove subquery wsagent runtime (`subquery.go`, `SubqueryOptions`, `Subquery`)
   + MCP tool/dispatch/gating + CLI subcommand (section B). `git mv` the two
   subquery bug tickets → `.dropped/` in this commit. Build/vet/test.
3. Exploration template-var cleanup if orphaned (section C). Build/vet/test.
4. Test pass: delete obsolete tests, adapt shared helpers, add the two guards
   (gemini unsupported-backend; subquery unknown-tool). Full
   `go build/vet/test ./... -count=1` green.

## LANDMINES (do NOT touch)
- The `Runner` interface, `RunnerRequest`, `RunnerResult`, `CodexRunner`,
  `ClaudeRunner` — all retained. Only the gemini concrete impl + its dispatch
  case go.
- The general `agents.status` / `agents.tail` / `agents.debug.tail` /
  `agents.debug.stdout|stderr|runtime_log|events` tools (server.go ~902/926/939/
  953 dispatch, ~2365-2420 schema) — RETAINED in 2b. Their minimization is 2c.
  Only the subquery-AGENT access gating that references them
  (`isSubqueryAgentTool` whitelist) is removed.
- `api.ask` / `api.ask_async` and their Manager methods — RETAINED; separate
  entry point from `Subquery` despite shared runtime semantics.
- The Claude runner — RETAINED.
- `wsstore` / session-key core from Phase 2a — untouched.
- When removing the `subquery` entries from the gate predicates (server.go
  2571/2642/2566), re-read each enclosing function: these are
  capability/profile-gate deciders; removing a disjunct must not flip the
  default decision for unrelated tools. Verify with the retained gate tests.

## Verification
- `go build ./... && go vet ./... && go test ./... -count=1` green.
- `runnerForBackend("gemini")` → unsupported-backend error (interface intact).
- MCP tool list omits `subquery`; `subquery` `tools/call` → unknown tool.
- No `Subquery`/`subquery` runtime, tool, gating, or CLI remains
  (`grep -rn` clean except doc refs handled in the doc pre-pass).
- Three bug tickets physically under `ai-docs/tickets/.dropped/`.
