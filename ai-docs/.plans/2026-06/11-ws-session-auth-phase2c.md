# Survey: ws-session-auth-phase2c (lead-authored)

> Lead-authored from targeted recon, not a survey-agent run — the 2c surface is
> comparably large to 2a, where the `plan-populator-survey` agent hit the 200k
> context limit (idea `260611-bug-agent-context-exhaustion-opaque-failure`). The
> EDIT remains delegated. Verdict: **FEASIBLE** — the 2a session-auth core +
> M1 render seams are in place; 2c is additive layering + surface narrowing, no
> blocking unknowns.

## Verdict: FEASIBLE

All integration points already exist:
- Session registry + mint + lookup: `internal/mcp/session_auth.go`.
- Keyed `tools/call` role gate (handles all `ws.lead.*` containment): `server.go:311-324`.
- `toolRole` enum + `parseCapabilityScope`: `server.go:39-44`, `~1107`.
- M1 render seams (explicitly left for M3): `playbook_tools.go` + `server.go:786-815`.
- Runner backends live + pluggable (2b): `wsagent/codex.go`, `claude.go`, `runnerForBackend`.

The phase is additive (mint+splice, prefer_mercenary, tip) + subtractive
(register schema narrowing, conservative diagnostic tidy). No redesign of the
2a/M1 substrate.

## Touch-point map

### A. Keyed playbook.render (Unit 1) — the foundation
- `internal/wsrsrc/wsrsrc.go:18-29` `PlaybookMeta`: ADD `Role string`; parse
  frontmatter `role:`. Find the frontmatter parser (same file or sibling) that
  fills `Kind`/`Delegates`/`Includes`/`Variables`; add `role` there. `Extra`
  already catches unknown keys, so confirm `role` is promoted to the typed field.
- `internal/mcp/playbook_tools.go`:
  - `renderPlaybookBody(s, rsrcRoot, name, ctx, opts)` and `renderPlaybook(...)`:
    thread `rootOverride` + the caller session key/entry. The body-splice of the
    minted child key happens here (after the Pass-2 substitution, near where
    `delegationTip` is appended).
  - `resolveRsrcRoot(rootOverride)` ALREADY accepts an override (line 173) — wire
    the call site to pass it instead of `""`.
- `internal/mcp/server.go:800-815` `case "playbook.render"`: currently resolves
  `worktreeRoot` via `resolveToolRoot` and `rsrcRoot` via `resolveRsrcRoot("")`.
  CHANGE: read `root_override` arg; look up the caller session entry
  (`s.sessions.lookup(<key>)`) for the role; pass key+role+override into render.
  - The session key string: `resolveToolRoot` consumes it from
    `params.Arguments`/`params.Meta` — extract the same key for the lookup (there
    is a helper that pulls `session_key`; reuse it, see `server.go:~300-346`).
- Schema `~2268`: add `root_override` optional string to `playbook.render`.

### B. prefer_mercenary (Unit 2)
- `session_auth.go`: `sessionEntry` ADD `preferMercenary bool`; registry method
  `setPreferMercenary(key string) bool` (write-lock, read-modify-write the map
  value, return found). `lookup` already returns the entry by value.
- `server.go`: new `ws.lead.prefer_mercenary` tool — schema near the other
  `ws.lead.*`/agents schemas; dispatch case near `ws.lead.login` (`~347`);
  handler near `handleLeadLogin` (`~1080`). The `ws.lead.` prefix means the
  `server.go:311-324` gate already enforces lead-only — NO second check.
- `agents-plugin/runtime.json`: add `ws.lead.prefer_mercenary` to tools (+commands
  if it has a CLI mirror; likely tools-only). Do NOT add to
  `agents-plugin-wsflow/runtime.json` (agentless).

### C. Always-on tip (Unit 3)
- `playbook_tools.go:151-165` `delegationTip` — append the always-on mercenary
  fragment, OR add a sibling fragment appended in `renderPlaybookBody` at the two
  `Meta.Delegates` append sites (lines 201-203, 220-222). Keep one source of truth.

### D. Register narrowing (Unit 4)
- `server.go:816-836` dispatch + `2280-2291` schema: drop `prompts`, `prompt_refs`,
  `tier`, `model`. Keep `name`, `backend`, `system_prompt_text`.
- `wsagent/agent.go`: `RegisterOptions` (`~80-88`) fields `Prompts`/`PromptRefs`/
  `ConditionalPromptRefs`/`Tier`/`Model` — remove from the REGISTER path only if
  orphaned. LANDMINE: `wsprompt.Resolve(promptSpecs, SystemPromptText, Tier,
  Model)` at `agent.go:537` and model resolution at `~1113` may serve one-shot /
  harness-config callers. Trace callers before deleting struct fields; prefer
  leaving internal plumbing and only removing the MCP-schema-exposed inputs if the
  internal use is live. Build is the gate.

### E. Native-shaped handle (Unit 5)
- `server.go:838+` `case "agents.call"` result text builds
  `"%s\t%s\tpid=%d\n..."`. Align the continuation handle to the native subagent id
  shape referenced by `terminologyForHarness(...)["ContinueIdiom"]`
  (claude: `SendMessage(to: <agentId>)`; codex: `resuming the agent using its task
  id`). The handle the lead carries forward should read as an agentId, not a
  ws-internal name, so one continuation idiom covers both paths.

### F. Diagnostic minimization (Unit 6) — CONSERVATIVE
- Audit `agents.tail/status/debug.*` dispatch + schema for anything orphaned by
  the register drop. Spec `#260508`/`#260512` RETAIN debug.*/status/tail for the
  live mercenary lifecycle → do not remove those. Likely near-zero net deletion;
  the real sprawl (subquery/gemini coupling) went in 2b.

### G. Bug re-triage (Unit 7)
- `260517-bug-ws-agent-empty-result-after-tool-use`: reproduce on the reshaped
  codex path; fix or forward-note.
- `260524-bug-ws-agent-register-stale-dir-result-hang`: assess whether dropping
  register-with-stems removes the stale-dir hang path; drop+rationale or keep+note.

## Sequence

1. Unit 1 (Role field + keyed render mint/splice/root_override) — checkpoint commit.
2. Unit 4 (register schema narrowing) + Unit 5 (handle shape) — checkpoint.
3. Unit 2 (prefer_mercenary) + Unit 3 (always-on tip + guidance flip) — checkpoint.
4. Unit 6 (conservative diagnostic tidy) + Unit 7 (bug re-triage) — checkpoint.
Build + `go test -count=1` after each; wsflow contract test at the end.

## LANDMINES

- **Do NOT redesign playbook.render** — fill seams only; keep M1 print/render base intact.
- **Do NOT remove spec-documented `agents.debug.*`** — conservative minimization; escalate on conflict.
- **Do NOT add a recursion-depth counter or capability ENFORCEMENT** — 2a gate handles depth-1; enforcement is Phase 3.
- **Do NOT re-add `root` to keyed tools** — `session_key` is the only root acceptor (2a).
- **wsflow stays agentless** — prefer_mercenary must not enter the wsflow contract; the contract test asserts the agentless surface.
- **`RegisterOptions` field removal**: trace `wsprompt.Resolve`/model-alias live callers (one-shot, harness config) before deleting struct fields — remove only MCP-exposed inputs if internal use is dead. Build is the gate.
- **Child-key mint is lead-gated**: non-lead caller or non-delegation playbook → NO mint. A second render must mint a DISTINCT key (registry uniqueness).
- **`-count=1` mandatory**: tests read the runtime `rsrc/` tree; the Go cache hides regressions.
