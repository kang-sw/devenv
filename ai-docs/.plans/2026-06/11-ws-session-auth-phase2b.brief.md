# Brief: ws-session-auth-phase2b

## Intent
Delete the genuinely-retired spawn surfaces now that Phase 2a severed the actor
dependency: the Gemini runner implementation, the `subquery` tool runtime
(exploration → native-subagent path), and the diagnostic/gating coupling that
exists only to serve those retired paths. Keep the harness-neutral runner-backend
interface so Gemini/antigravity/custom harnesses remain a deferred plug, not a
structural exclusion. Drop three resolved-by-deletion bug tickets to `.dropped/`
in the removing commits.

## Scope Boundary
**In scope (Phase 2b):**
- Delete the Gemini runner *implementation* (`internal/wsagent/gemini.go`,
  `gemini_test.go`) and its dispatch case in `runnerForBackend`.
- Remove Gemini coupling in config harness aliasing/detection and any
  backend-iteration lists, so Gemini is a deferred plug (re-adding it requires a
  new `GeminiRunner` + re-registration), not a live backend.
- Delete the `subquery` tool runtime end to end: `wsagent` `Subquery` method +
  `SubqueryOptions` + `SubquerySystemPrompt` (`subquery.go`), the MCP `subquery`
  tool (schema + dispatch + subquery-agent access gating), and the CLI `subquery`
  subcommand.
- Remove exploration-purpose spawn coupling that exists only for the deleted
  subquery/exploration path (verify the `ExploreAgent` playbook template vars:
  remove only if orphaned after subquery deletion; otherwise leave).
- Drop to `.dropped/` via `git mv` **in the removing commits**:
  `260524-bug-wsstore-ci-sqlite-busy`,
  `260524-bug-subquery-non-head-history-evidence`,
  `260524-bug-subquery-working-directory-stderr`.

**Out of scope (deferred):**
- Phase 2c: the codex `agents.*` call/lifecycle reshape, dropping
  `register(prompts:[stems])`, native-handle parity, the mercenary routing gate.
  The Codex runner stays live and callable through 2b via the same `Runner`
  interface.
- **General diagnostic minimization is deferred to Phase 2c.** The ticket's
  "minimize `agents.tail/status/debug` to what the retained mercenary lifecycle
  needs" cannot be done correctly before 2c defines that lifecycle. 2b removes
  only diagnostics/gating *coupled to the deleted paths*; the general
  `agents.status` / `agents.tail` / `agents.debug.*` surface (which serves the
  live Codex runner) stays intact for 2c to minimize against the concrete
  mercenary lifecycle. This avoids speculative delete-then-re-add churn.
- The Claude runner stays (not a Phase 2b target).
- `api.ask` / async API jobs stay; they share named-agent runtime *semantics*
  but are a separate `Manager` entry point from `Subquery`.

## Caller-Visible Contract
- The MCP `subquery` tool is **removed** from the advertised tool surface and its
  dispatch returns unknown-tool for `subquery`.
- The CLI `ws-mcp subquery` subcommand is **removed** (usage line updated).
- `gemini` is no longer a usable agent backend: `runnerForBackend("gemini")`
  returns the existing `unsupported agent backend` error; config harness
  validation/aliasing no longer advertises `gemini`.
- The `Runner` interface (`RunnerRequest`/`RunnerResult`) is **unchanged** and
  still pluggable; Codex and Claude still attach through it and stay live.
- The general `agents.*` lifecycle + diagnostic tool surface is **unchanged** in
  2b (status/tail/debug.* remain).

## Contract Instructions
- Files / modules:
  - `internal/wsagent/gemini.go`, `gemini_test.go` — delete.
  - `internal/wsagent/codex.go` — remove the `case "gemini"` from
    `runnerForBackend` (keep `Runner`, `RunnerRequest`, `RunnerResult`,
    `CodexRunner`, `ClaudeRunner`, and `case "", "codex"` / `"claude"`).
  - `internal/wsagent/subquery.go` — delete (the `SubquerySystemPrompt` const).
  - `internal/wsagent/agent.go` — remove `SubqueryOptions` (~207), the `Subquery`
    method (~1150) and its `SubquerySystemPrompt` use (~1164); remove `gemini`
    from the backend-iteration list (~1658). Leave the rest of the manager
    lifecycle intact.
  - `internal/mcp/server.go` — remove the `subquery` tool schema (~2252), the
    `case "subquery"` dispatch (~765), and the subquery-agent access gating
    (`subqueryAgentAccessAllowed` ~311/2714, `isSubqueryAgentTool` ~2730, and the
    `subquery` references in the keyless/profile gate lists ~2566/2571/2642).
    **Do not** remove the general `agents.status`/`agents.tail`/`agents.debug.*`
    tools.
  - `internal/wsconfig/config.go` — remove `gemini` from harness
    detection/aliasing (~215/348) and from the harness error message (~363:
    "harness must be codex, claude, gemini, or default" → drop `gemini`).
  - `internal/mcp/playbook_tools.go` — inspect the `ExploreAgent` template vars
    (~24/29/35); remove only if orphaned by subquery deletion.
  - `cmd/ws-mcp/main.go` — remove the `subquery` CLI subcommand (~43/284), its
    usage-line entry (~74), the gate-list entries (~254/269), and drop `gemini`
    from the harness flag help (~348).
- Reuse the existing `unsupported agent backend %q` error path for removed
  backends; do not add a new bespoke error.
- Forbidden: do NOT stub/mock a "gemini removed" placeholder runner; delete the
  impl outright. Do NOT introduce a fallback that silently routes `gemini` →
  codex. Do NOT delete the `Runner` interface or collapse it into Codex.

## Integration Test Instructions
- Boundary: Go package tests across `internal/wsagent`, `internal/mcp`,
  `internal/wsconfig`, `cmd/ws-mcp`.
- Delete tests that exercise only the removed surfaces (gemini runner, subquery
  runtime/CLI/tool, subquery-agent gating). **Do not** delete tests for retained
  surfaces (codex/claude runners, general `agents.*` lifecycle + diagnostics,
  config harness for codex/claude/default, api.ask). If a shared test helper
  references a removed surface, adapt the helper rather than deleting the
  retained-surface test (Phase 2a regression lesson).
- Add/keep a guard asserting `runnerForBackend("gemini")` now returns the
  unsupported-backend error (proves the interface stayed but the impl is gone).
- Add/keep a guard asserting the MCP tool list no longer advertises `subquery`
  and a `subquery` `tools/call` is rejected as unknown.
- Pass criteria: `go build ./... && go vet ./... && go test ./... -count=1` all
  green (tests reading the runtime `rsrc/` tree need `-count=1`).

## Implementation Strategy Decisions
- Delete-impl-keep-interface: the `Runner` interface is the harness-neutral plug
  point; only the Gemini concrete impl + its dispatch case are removed.
- Gemini becomes a fully deferred plug (no config alias, no runner); the
  interface is the re-entry point.
- General `agents.*` diagnostic minimization is **deferred to 2c** (see Scope
  Boundary); 2b only removes diagnostics/gating coupled to deleted paths.
- Bug-ticket drops happen via `git mv` inside the same commits that remove the
  corresponding runtime, so the drop rationale is co-located with the deletion.

## Rejected Alternatives
- Minimizing the general `agents.debug.*`/`status`/`tail` surface now: rejected —
  the target ("mercenary lifecycle needs") is undefined until 2c; speculative
  minimization risks delete-then-re-add churn and could strand 2c.
- Keeping `gemini` as a config alias pointing at a deleted runner: rejected —
  leaves config validating a backend that fails at runtime (inconsistent
  surface).
- Replacing `GeminiRunner` with a stub returning "unsupported": rejected — the
  `runnerForBackend` default already yields the unsupported-backend error;
  a stub is dead code.

## Approach
- Work backend-by-surface: (1) gemini runner + dispatch + config + iteration
  list; (2) subquery runtime (wsagent) + MCP tool + gating + CLI; (3) exploration
  template-var cleanup if orphaned; (4) test adaptation/deletion; (5) bug-ticket
  `git mv` co-located with each deletion commit.
- Verify build/vet/test green after each logical commit.

## Constraints
- Codex runner stays live and callable through the unchanged `Runner` interface.
- No new public symbols; this is deletion + dispatch pruning only.
- AI-authored content English-only; commits via `ws/git.commit` with
  `## AI Context` ending in the required Co-Authored-By trailer.
- Do not stage `.codex`.

## Out of scope
- Codex `agents.*` reshape, `register(prompts:[stems])` removal, native-handle
  parity, mercenary routing gate (all Phase 2c).
- General `agents.*` diagnostic minimization (Phase 2c).
- exec stateless / dashboard build-fix (Phase 3).

## Details
- `Runner` interface (keep verbatim): `Call(RunnerRequest) (RunnerResult, error)`
  in `internal/wsagent/codex.go`.
- `runnerForBackend` after edit: cases `"", "codex"` → `CodexRunner{}`,
  `"claude"` → `ClaudeRunner{}`, default → `unsupported agent backend %q`.
- Three bug tickets to `git mv` into `ai-docs/tickets/.dropped/`:
  `260524-bug-wsstore-ci-sqlite-busy.md`,
  `260524-bug-subquery-non-head-history-evidence.md`,
  `260524-bug-subquery-working-directory-stderr.md`.

## Verification Contract
- `go build ./... && go vet ./... && go test ./... -count=1` green.
- `grep -rn "gemini\|Gemini\|GeminiRunner" internal cmd` shows no runner impl or
  dispatch (mental-model/spec doc refs handled in the doc pre-pass, not here).
- `grep -rn "subquery\|Subquery"` shows no runtime, tool, gating, or CLI.
- `runnerForBackend("gemini")` returns the unsupported-backend error.
- MCP tool list omits `subquery`; `subquery` `tools/call` → unknown tool.
- The three bug tickets are physically under `ai-docs/tickets/.dropped/`.

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/mental-model/named-agent-runtime.md` - [Must] runner-backend interface
  contract ("Add a backend" recipe), subquery + diagnostic surface, api.ask
  separation. The Gemini contracts here are deleted-by-this-phase; do not treat
  them as constraints to preserve.
- `ai-docs/mental-model/mcp-runtime.md` - [Must] subquery + agents.* + api.ask
  share named-agent runtime; tool dispatch + profile gating shape.
- `ai-docs/mental-model/prompt-bundle.md` - [Maybe] `SubquerySystemPrompt` is
  inline; explore rsrc playbook relationship.
