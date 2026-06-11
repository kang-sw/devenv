# Brief: ws-session-auth-phase2c

## Intent

Reshape the retained **codex/claude** runner backends plus the `agents.*`
call/lifecycle core into the first-class **mercenary** delegation surface, and
fill M1's deliberately-placed `playbook.render` seams to render-mint child
session keys. After this phase: host-native subagents are the default delegation
path; the mercenary is always available to the lead and invoked only on explicit
request or after a per-key `ws.lead.prefer_mercenary` guidance flip; both native
and mercenary delegates receive a single self-contained `playbook.render` prompt
with their child session key already spliced in; and the register-with-stems +
registration-time model-alias surface is gone.

## Scope Boundary

In scope (Phase 2c only):
1. Keyed `playbook.render` child-key mint + splice + `root_override` (fill M1 seams).
2. `ws.lead.prefer_mercenary(session_key)` lead-only render-mode flip.
3. Always-on mercenary tip fragment in every delegation-capable rendering.
4. Drop `agents.register(prompts:[stems])` + registration-time `tier`/model-alias schema.
5. Native-shaped mercenary continuation handle (agentId-shaped).
6. Mercenary scope = implementer/reviewer roles only; routing as specified.
7. Diagnostic minimization — CONSERVATIVE (see Implementation Strategy Decisions).
8. Re-triage bugs `260517-bug-ws-agent-empty-result-after-tool-use` and
   `260524-bug-ws-agent-register-stale-dir-result-hang` — explicit disposition each.

Out of scope (Phase 3): exec stateless rework; capability-scope ENFORCEMENT (the
key already carries a reserved scope from Phase 1 — do NOT add enforcement of it
here beyond the existing `roleAllowsTool` gate); dashboard build-fix /
actors-table adaptation / exec-job `owner_actor_id`. Out of scope (M4): `api.ask`
spawn/async removal.

## Caller-Visible Contract

Authoritative contract: `ai-docs/spec/mcp-tools.md`
`#260610-mercenary-delegation-surface` (read it in full — it is the binding
contract). Summary of observable behavior:

- **Default native; mercenary always available.** Default delegation is always a
  host-native subagent. Mercenary is invoked only when (a) the user explicitly
  requests it, or (b) the lead flipped its key's render mode via
  `ws.lead.prefer_mercenary(session_key)` (lead-only) — which changes ONLY the
  default delegation *guidance* `playbook.render` emits for implementer/reviewer
  playbooks, never availability.
- **Always-on tip.** Every delegation-capable rendering (`Meta.Delegates == true`)
  carries a small always-on fragment noting the mercenary path is reachable on
  request, so the on-request path works without the toggle.
- **Scope: implementer/reviewer only.** Exploration, survey (reference-discovery,
  plan-populator), and mental-model update route to native subagents — never mint
  a mercenary.
- **Single self-contained prompt; native-shaped handle.** A mercenary is invoked
  with one self-contained prompt from `playbook.render`; there is no
  `register(prompts:[stems])` step. A mercenary call returns a continuation handle
  the same shape as a native subagent id.
- **Render-minted child keys.** `playbook.render(session_key, name, context?,
  root_override?)`: when the calling `session_key`'s role == lead, mint a fresh
  child key (role taken from playbook frontmatter) and splice it into the rendered
  prompt. `root_override` rebinds BOTH the auto-include resolution root and the
  child-key binding root; render does not infer worktree shape (caller passes the
  path).
- **Containment unchanged (already landed in 2a).** The keyed `tools/call` handler
  rejects `ws.lead.*` from non-lead keys → spawn depth strictly 1. Do NOT add a
  recursion-depth counter. Schema/`tools/list` filtering stays a soft-guard only.

## Contract Instructions

All paths under `agents-plugin-tool/`.

### Unit 1 — Keyed `playbook.render`: child-key mint + splice + `root_override`
- `internal/wsrsrc/wsrsrc.go`: add `Role string` to `PlaybookMeta` (frontmatter
  `role:` — values `lead|delegate|leaf|implementer|reviewer`; see note below).
  Parse it in the existing frontmatter parser alongside `delegates`/`kind`.
  Default empty when absent.
- `internal/mcp/playbook_tools.go`: thread `root_override` through
  `renderPlaybook`/`renderPlaybookBody` (pass to `resolveRsrcRoot` AND use as the
  worktree root for the tmp file when set). Add the child-key mint+splice: a new
  function that, given the caller's session entry (role==lead) and the loaded
  `Meta.Role`, mints a child key via `s.sessions.mint(root, childScope)` and
  splices a clearly-delimited line into the body (e.g. a `Your ws session_key:
  <key>` block) so the delegate's ws calls are pre-keyed. Non-lead caller, or a
  playbook whose role is not a delegate-eligible role → no mint (render unchanged).
- `internal/mcp/server.go` `case "playbook.render"` (~800-815): accept
  `root_override` and the caller session key. `resolveToolRoot` already yields the
  root from `session_key`; ALSO look up the session entry (`s.sessions.lookup`) to
  get the caller's role for the lead-gated mint branch. Pass both into render.
  Apply `root_override` (when present) as the binding/auto-include root.
- Update the `playbook.render` MCP schema (~2268 region) to declare
  `root_override` (optional string). `session_key` is already the standard keyed
  arg (do not re-add `root`).
- Child key role: derive the child scope from `Meta.Role`. Map implementer/reviewer
  (and any delegate-intent playbook) to `roleDelegate` by default; `leaf` →
  `roleLeaf`. Keep this mapping in one helper. Reuse `toolRole` + `parseCapabilityScope`
  conventions; do NOT invent a parallel role enum.

### Unit 2 — `ws.lead.prefer_mercenary(session_key)` render-mode flip
- `internal/mcp/session_auth.go`: add `preferMercenary bool` to `sessionEntry`;
  add a registry method to set it under the write lock (read-modify-write the map
  value). Add a lookup accessor if needed.
- `internal/mcp/server.go`: new tool `ws.lead.prefer_mercenary` — schema (keyed,
  `session_key` required), dispatch case, handler. It is `ws.lead.*` so the
  EXISTING keyed-handler gate (server.go:311-324) already rejects non-lead keys —
  do not add a second check. The handler flips the caller key's `preferMercenary`.
  Add to BOTH `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json`?
  NO — mercenary is ws-only; wsflow is agentless. Add to `agents-plugin/runtime.json`
  tools+commands only; confirm wsflow contract stays agentless.
- `playbook.render` reads the caller key's `preferMercenary`: when true AND the
  playbook is an implementer/reviewer delegation playbook, the rendered default
  guidance advises the mercenary spawn idiom as primary. When false, native is the
  primary guidance. This is guidance-text selection only.

### Unit 3 — Always-on mercenary tip
- `internal/mcp/playbook_tools.go` `delegationTip` (or a sibling fragment appended
  in `renderPlaybookBody` when `Meta.Delegates`): add the small always-on
  "mercenary reachable on request" fragment. Keep it harness-aware via the
  terminology table if it names a spawn idiom. Token noise is accepted.

### Unit 4 — Drop register-with-stems + registration model-alias
- `internal/mcp/server.go` register schema (2280-2291): remove `prompts`,
  `prompt_refs`, `tier`, `model`. Keep `name`, `backend`, `system_prompt_text`.
- `case "agents.register"` (816-836): stop reading `prompts`/`prompt_refs`/`tier`/
  `model`; pass only name/backend/system_prompt_text into `RegisterOptions`.
- `internal/wsagent/agent.go`: remove the now-dead `Prompts`/`PromptRefs`/
  `ConditionalPromptRefs`/`Tier`/`Model` register inputs IF they become fully
  orphaned. CAUTION: `wsprompt.Resolve` and model-alias resolution may still be
  used by other live callers (e.g. one-shot, harness config). Remove only what is
  genuinely orphaned by the schema drop; keep harness/model resolution that the
  runner still needs to launch codex/claude. Do not break the build.
- This satisfies `spec-remove: 260508-agents-register-model-alias-field`.

### Unit 5 — Native-shaped continuation handle
- `internal/mcp/server.go` `case "agents.call"` result text (~838+): the returned
  continuation handle must be the same SHAPE as a native subagent id (per spec).
  Align the call-result text/handle so the lead reuses one continuation idiom. Do
  not invent a new handle scheme; match the native agentId shape the terminology
  table's `ContinueIdiom` already references.

### Unit 6 — Diagnostic minimization (CONSERVATIVE — see Strategy Decisions)
- Remove only diagnostics genuinely orphaned by dropping register-stems / the
  reshape. RETAIN the `agents.debug.*` / status / tail surface that
  `#260508`/`#260512` still document for the live codex mercenary lifecycle.

### Unit 7 — Bug re-triage (explicit disposition each)
- `260517-bug-ws-agent-empty-result-after-tool-use`: lives in the retained codex
  path. Investigate on the reshaped path; if reproducible, fix; else record a
  forward note. Do NOT drop it silently.
- `260524-bug-ws-agent-register-stale-dir-result-hang`: likely obsoleted by
  dropping register-with-stems. If the reshape removes the failure path, drop to
  `.dropped/` via `git mv` in the reshape commit with rationale; else keep + note.

Forbidden wiring: no temporary/mock child keys; no parallel role enum; no
recursion-depth counter; no capability-scope ENFORCEMENT beyond the existing
`roleAllowsTool` gate (that is Phase 3); no `root` re-introduction on keyed tools.

## Integration Test Instructions

- Boundary: `internal/mcp` table/integration tests (`server_test.go`,
  `playbook_tools_test.go`, `session_auth` coverage) + `internal/wsagent`
  (`agent_test.go`) + `internal/wsrsrc` for the new `Role` frontmatter field.
- New/extended coverage required:
  - `playbook.render` with a lead key + delegation playbook → response contains a
    minted child key; the key resolves in the registry to the bound root + a
    non-lead scope; a SECOND render mints a DISTINCT key.
  - `playbook.render` with a non-lead key OR a non-delegation playbook → no key
    minted (render unchanged).
  - `root_override` rebinds the child-key root (minted key resolves to the
    override root, not the caller root).
  - `ws.lead.prefer_mercenary` from a non-lead key → rejected by the existing gate;
    from a lead key → flips guidance; subsequent implementer/reviewer render shows
    mercenary-primary guidance; a non-delegation render is unaffected.
  - Always-on tip present in every `Meta.Delegates` rendering regardless of flip.
  - `agents.register` rejects/ignores removed `prompts`/`tier`/`model` per the new
    schema; a register with only `system_prompt_text` still works.
  - `agents.call` continuation handle shape matches the documented native id shape.
- Run: `go build ./... && go vet ./... && go test ./... -count=1` (from
  `agents-plugin-tool/`; `-count=1` is MANDATORY — tests read the runtime `rsrc/`
  tree and the Go cache hides regressions). Then the wsflow contract test:
  `python3 -m unittest agents-plugin-wsflow.tests.test_wsflow_runtime_contract`
  (from repo root) — must stay green (wsflow stays agentless; prefer_mercenary is
  ws-only and must NOT appear in the wsflow contract).

## Implementation Strategy Decisions (do not reopen)

- **Fill M1 seams; do not redesign playbook.render.** The session_key prepend +
  `root_override` seams were deliberately left for M3 by M1 (see the M3-forward
  comments in `playbook_tools.go`/server.go). "Coordinate with M1" means keep the
  base print/render contract intact, NOT wait for M1. Reuse `resolveRsrcRoot`,
  `renderPlaybookBody`, `s.sessions.mint`, `toolRole`.
- **Diagnostic minimization is CONSERVATIVE (lead ruling).** The contract-first
  spec (`#260508`/`#260512`) still documents `agents.debug.*`/status/tail for the
  live codex mercenary lifecycle. Removing that surface would contradict the
  binding spec. So minimize ONLY diagnostics orphaned by the reshape; retain the
  spec-documented surface. If a genuine conflict appears, ESCALATE — do not delete
  spec-documented behavior unilaterally. (Mirrors the 2b conservative decision.)
- **Child-key role source = playbook frontmatter `Role`.** Add the field; map to
  `toolRole` via one helper. Implementer/reviewer/delegate → `roleDelegate`; leaf →
  `roleLeaf`. Lead playbooks never mint (only the lead logs in).
- **prefer_mercenary changes guidance text only**, never tool availability. The
  mercenary is always reachable via the always-on tip.
- **Sequence the work as Units 1→7, committing a logical checkpoint per unit.**
  Run the build after each unit; keep the tree compiling. Unit 1 (keyed render +
  Role field) is the foundation; Unit 4 (register drop) and Unit 5 (handle) are
  the surface narrowing; Units 2-3 layer routing/guidance; Unit 6-7 are cleanup.

## Rejected Alternatives

- Building keyed `playbook.render` as separate M1 work first → rejected: seams are
  already in place and the ticket assigns the fill to Phase 2c.
- Aggressive `agents.debug.*` removal → rejected: contradicts the retained
  contract-first spec; conservative minimization instead.
- A new role enum / recursion-depth counter / capability enforcement → rejected:
  2a's keyed-handler gate already enforces depth-1; enforcement is Phase 3.
- Mock/placeholder child keys → rejected: mint real keys via the registry.

## Constraints

- AI-authored content English-only.
- Commits via `ws/git.commit` with `## AI Context`; end messages with
  `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.
- Do NOT stage `.codex`.
- `go ... -count=1` mandatory; read full output before claiming pass.
- Keep the tree compiling at every checkpoint.
- wsflow stays agentless — no mercenary surface leaks into the wsflow contract.

## Out of scope

Phase 3 (exec stateless, capability-scope enforcement, dashboard build-fix) and
M4 (`api.ask` spawn/async removal). Do not touch those surfaces beyond keeping the
build green.

## Details

- `toolRole`: `roleLead|roleDelegate|roleLeaf` (server.go:39-44).
- `sessionEntry{root, scope}` + `sessionRegistry.mint(root, scope)` /
  `lookup(key)` (session_auth.go).
- Keyed gate: server.go:311-324 (`ws.lead.*` blocked for non-lead; `roleAllowsTool`
  for non-lead scoped keys).
- `ws.lead.login` handler ~1080; `parseCapabilityScope` ~1107.
- `playbook.render` dispatch server.go:800-815; schema ~2268.
- `renderPlaybook`/`renderPlaybookBody`/`delegationTip`/`resolveRsrcRoot`:
  playbook_tools.go:151-257.
- `PlaybookMeta` wsrsrc.go:18-29 (add `Role`).
- register dispatch server.go:816-836; schema 2280-2291.
- `RegisterOptions` agent.go:~80-88.

## Verification Contract

- `go build ./... && go vet ./... && go test ./... -count=1` green from
  `agents-plugin-tool/`.
- wsflow contract test green; `prefer_mercenary` absent from wsflow contract.
- New tests above all pass.
- Residual grep: no `register(prompts` stems path remains in schema; no
  recursion-depth counter added; no `root` arg re-added to keyed tools.
- Both re-triaged bug tickets have an explicit disposition (fixed / dropped+rationale
  / kept+forward-note).

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `ai-docs/spec/mcp-tools.md` #260610-mercenary-delegation-surface — [Must] binding contract.
- `ai-docs/spec/mcp-tools.md` #260508-agents-register-model-alias-field — [Must] register drop.
- `ai-docs/mental-model/named-agent-runtime.md` — [Must] spawn/session/keyed-gate model.
- `ai-docs/mental-model/mcp-runtime.md` — [Must] tool dispatch + session-auth layer.
- `ai-docs/mental-model/prompt-bundle.md` — [Maybe] playbook render + M3 forward-compat seams.
- The lead-authored survey: `ai-docs/.plans/2026-06/11-ws-session-auth-phase2c.md` — [Must] touch-point map.
