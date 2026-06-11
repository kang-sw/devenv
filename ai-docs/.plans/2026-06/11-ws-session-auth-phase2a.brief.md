# Brief: ws-session-auth-phase2a

> Ticket: `260609-refactor-ws-spawn-runtime-deletion-session-auth`, Phase 2a.
> Branch: `implement/ws-session-auth-phase2a` (from epic `260605-epic-ws-playbook-factory-pivot`).
> Phase 1 (additive session-auth) is merged at `c917c9f0`; this slice makes that
> path mandatory and deletes the predecessor model.

## Intent

Cut the ws-mcp auth model over from the persistent actor/authority/child-actor +
`ws.setup` bootstrap to the mandatory ephemeral **session-key** model, end to end.
After this slice: `ws.lead.login(root) -> session_key` is the *sole* bootstrap and
the *only* place a `root` is accepted; every root-aware ws tool call carries a
`session_key`; there is no keyless or silent-fallback root resolution; the actor
model, `ws.setup`, and the setup-fence are gone; and the ws skill text bootstraps
with `ws.lead.login` instead of `ws.setup`.

## Scope Boundary

**In scope (Phase 2a):**

1. **Resolver → key-only mandatory.** `resolveToolRoot` resolves a root-aware
   call ONLY from `session_key`: known → its root; present-but-unknown →
   `unknown_session` reject (existing Phase 1 contract); **absent → mandatory-key
   reject** (new). Remove every silent fallback source for root-aware dispatch:
   the volatile session default-root, host-workspace metadata, `WS_MCP_PROJECT_ROOT`
   as a resolution source, and the startup-root as a resolution source.
2. **Strip the `root` parameter from all root-aware tool schemas** (and their CLI
   mirrors). `ws.lead.login(root, capability?)` is the only tool that accepts
   `root`. This strips spec `#260523-agents-root-schema-invisibility` (its
   premise — hidden explicit-root dispatch behind rootless public schemas — is
   gone when no tool but login takes a root).
3. **Delete the actor/authority/child-actor machinery:** `actor_id`-as-identity,
   the authority field, `ensureChildActor` / `childActorInstruction`,
   `restoreActor` / `bindActor` persistence, `wsstore` actor records, and the
   `actorID` dimension of the named-agent registry key
   (`wsstore.AgentInternalKey(actorID, publicName)`).
4. **Remove `ws.setup` entirely** — both `method: lead-workflow-bootstrap` (actor)
   and the bare `ws.setup(root)` compatibility root-session — plus the setup-fence
   (`isSetupFenceRequest` / `wg.Wait()`) and the `ws.setup` CLI mirror and hidden
   `--actor-id` flags.
5. **Migrate ws skill text + agent prompts** that instruct `ws.setup` bootstrap to
   `ws.lead.login`. Read `agents-plugin/skills/lead-skill-authoring/SKILL.md`
   first and apply its invariant checklist to every changed line.
6. **Keyless hard-rejection** is the union of (1) and (4): no path resolves a root
   without a valid `session_key`.

**Out of scope (later slices) — do NOT do here:**

- **2b:** delete the gemini runner impl, the `subquery` runtime, exploration spawn
  paths, diagnostic sprawl; drop the resolved-by-deletion bug tickets.
- **2c:** reshape the codex `agents.*` family into the mercenary surface — drop
  `register(prompts:[stems])`, align the continuation handle to the native agentId
  shape, add the routing gate / `ws.lead.prefer_mercenary`, and **mint+splice
  render-minted child keys** (`playbook.render`). This is the *replacement* for the
  child-actor credential that 2a deletes.
- **M1:** the keyed `playbook.render(session_key, …, root_override?)` signature.
- **M4:** `api.ask` spawn/async removal.

**Compile-not-function boundary (critical).** The retained `agents.*` / `subquery`
/ codex spawn surfaces must still **compile** against the session-keyed core after
2a; they need not still *function*. 2a removes the spawn path's child-credential
mechanism (child-actor injection); its replacement (render-minted child key) is
2c. Between 2a and 2c the spawn path compiles but cannot mint a child credential —
acceptable, because this is an intermediate state on an integration branch and no
delegate is exercised mid-refactor. Where the spawn path loses its credential
source, prefer a clear "session-key delegation not yet wired (Phase 2c)" error
over fabricated/placeholder credentials or mock data.

## Caller-Visible Contract

- `ws.lead.login(root, capability?) -> session_key`: unchanged from Phase 1, now
  the SOLE bootstrap verb and the only tool accepting `root`.
- `ws.setup`: **removed**. An explicit call returns the unknown-tool JSON-RPC error
  (not `isError` text).
- Every other root-aware tool: requires a `session_key` argument; the `root`
  parameter is gone from the schema. Absent `session_key` → mandatory-key
  `toolTextResponse` (`isError: true`) naming `ws.lead.login`. Present-but-unknown
  `session_key` → `unknown_session` `toolTextResponse` naming `ws.lead.login`
  (Phase 1 contract, unchanged).
- `agents.*` / `subquery` / `exec.*`: now keyed via `session_key`; no `root` / no
  hidden explicit-root dispatch; no actor scope.
- Skill guidance: the lead bootstraps with `ws.lead.login(root)` and threads the
  returned `session_key` through subsequent calls.

## Contract Instructions

Implement against these modules (the survey plan refines exact symbols/lines):

- `internal/mcp/server.go` — rewrite `resolveToolRoot` to the key-only mandatory
  chain; in `callTool` remove the actor gate, remove the `ws.setup` dispatch case,
  add the mandatory-key rejection for root-aware tools, and remove the setup-fence
  (`isSetupFenceRequest` / `wg.Wait()`); in `tools()` remove the `ws.setup` schema
  and strip the `root` property from every root-aware tool schema; remove
  actor-scoped dispatch wiring.
- `internal/mcp/session_auth.go` — extend with the mandatory-key error helper if a
  distinct shape from `unknown_session` is needed; reuse the Phase 1
  `sessionRegistry` and `lookup` as-is (do NOT rebuild them).
- `internal/wsstore` — remove actor records / actor table / `FindActor` / actor
  persistence; re-key `AgentInternalKey` to drop the `actorID` dimension (namespace
  agent role pointers by the session/root scope so distinct worktrees stay
  distinct — the survey confirms the exact replacement key).
- `internal/wsagent` — remove `ensureChildActor` / `childActorInstruction` /
  child-actor id persistence and actor-scoped registration/dispatch; leave the
  spawn path compiling per the compile-not-function boundary.
- `cmd/ws-mcp` — remove the `ws.setup` CLI mirror and hidden `--actor-id` flags;
  remove `root` flags from root-aware command mirrors; keep `runtimeCapabilityCommandNames`
  and `runtime.json.commands` consistent.
- `agents-plugin/**` (and `agents-plugin-wsflow/**` if it carries setup text) — skill
  text + agent prompts: `ws.setup` bootstrap → `ws.lead.login`. Read
  `lead-skill-authoring/SKILL.md` first.
- `agents-plugin/runtime.json` + `agents-plugin-wsflow/runtime.json` — remove
  `ws.setup` if present; confirm `ws.lead.login` remains; review per the
  mcp-runtime "Add an MCP tool" recipe coupling (tools() + runtime.json together).

**Reuse before adding:** the Phase 1 `sessionRegistry`, the `session_key` branch in
`resolveToolRoot`, and the keyed capability gate already exist. 2a makes them
mandatory and removes the predecessors; it does not introduce a parallel auth path.

**Forbidden:** leaving `ws.setup` as a hidden alias; a keyless compatibility
escape; fabricated/placeholder child credentials; mock auth data; advertising a
`root` param a tool ignores.

## Integration Test Instructions

- Boundary: extend `internal/mcp/session_auth_test.go` (the Phase 1 integration
  suite) and adjust `wsstore` / `wsagent` tests whose actor assumptions are deleted.
- Required cases:
  - keyless root-aware call → mandatory-key reject (not silent default).
  - present-but-unknown `session_key` → `unknown_session` reject.
  - `ws.setup` (both forms) → unknown-tool error.
  - parallel distinct-root keyed calls resolve their own root with no fence and no
    clobber (reuse the Phase 1 sentinel-marker pattern: `session-marker-root1/2.txt`).
  - a root-omitted keyless `agents.*` call → reject (no surviving actor-scope path).
  - registry re-key: same public agent name under two distinct session roots stays
    distinct after the `actorID` dimension is dropped.
- `wsstore` / `wsagent`: remove or rewrite actor-record and child-actor tests.
- Run: `cd agents-plugin-tool && go build ./... && go vet ./... && go test ./... -count=1`
  (`-count=1` because tests reading the runtime `rsrc/` tree have a cache blind
  spot). Also run the Python launcher unittest if skill-text / launcher contract is
  touched.
- Pass criteria: build/vet clean; full suite green; no actor/`ws.setup` symbol
  remains; keyless reject + `unknown_session` + parallel-no-clobber all asserted.

## Implementation Strategy Decisions

User-locked (do NOT reopen):

- **Full key-only now (Q1 = "Full key-only").** Strip the `root` param from all
  root-aware schemas in 2a; strip `#260523-agents-root-schema-invisibility` in 2a.
  `ws.lead.login(root)` is the only `root` acceptor.
- **Remove `ws.setup` entirely (Q2 = "Remove ws.setup entirely").** Both forms
  deleted; the lead and the ws skill text migrate to `ws.lead.login` in 2a.
- `session_key` is mandatory for every root-aware tool; no keyless or silent
  fallback survives.
- Registry re-key drops the `actorID` dimension of `AgentInternalKey`; namespace by
  the session root so worktrees stay distinct (survey confirms the exact key).
- Spawn-path child credential is removed in 2a; its replacement is the 2c
  render-minted child key. Leave the spawn path compiling with a clear
  not-yet-wired error, never fake credentials (compile-not-function boundary).

## Rejected Alternatives

- **Resolver-only, schema cleanup deferred to 2c** (Q1 option A) — rejected; the
  user chose the full clean cutover in 2a.
- **Keep caller-explicit `root` as a resolver** (Q1 option C) — rejected; diverges
  from the spec's "every ws call carries a session key".
- **Keep `ws.setup(root)` as a compat session-mint alias** (Q2 option A) — rejected;
  full removal chosen.
- **Keep `ws.setup(root)` as a keyless volatile default-root** (Q2 option C) —
  rejected as incoherent under mandatory-key.

## Approach

1. Rewrite `resolveToolRoot` to the key-only mandatory chain; add the mandatory-key
   error helper.
2. Remove `ws.setup` (schema, dispatch case, fence, CLI mirror, hidden flags).
3. Delete the actor machinery across `internal/mcp`, `internal/wsstore`,
   `internal/wsagent`; re-key `AgentInternalKey`.
4. Strip the `root` param from all root-aware tool schemas + CLI mirrors.
5. Sever the spawn-path child-actor injection; leave the path compiling.
6. Migrate skill text + agent prompts `ws.setup` → `ws.lead.login` (lead-skill-authoring discipline).
7. Tests (new + adjusted) and verification; the spec-remove of `#260523` is closed
   out in the doc pre-pass, not the implementer's job.

## Constraints

- Retained `agents.*` / `subquery` / codex spawn surfaces must compile after 2a.
- Do NOT reshape the `agents.*` schema/handle/routing (2c) or delete gemini/subquery
  (2b).
- `-count=1` for Go tests touching the `rsrc/` tree.
- AI-authored content is English-only.
- Skill/prompt edits follow `lead-skill-authoring/SKILL.md`.

## Out of scope

- 2b deletions, 2c codex mercenary reshape + render-minted child keys, M1 render
  signature, M4 api.ask. The dashboard build-fix is Phase 3 — but if removing
  `wsstore` actor records breaks the dashboard *build*, surface it (do not fix the
  feature; note it for Phase 3).

## Details

- Mandatory-key error vs `unknown_session`: absent key (no `session_key` arg) →
  mandatory-key reject; present arg that the registry doesn't know → `unknown_session`.
  Both are `toolTextResponse` (`isError: true`) naming `ws.lead.login`; keep them
  distinguishable in text so callers can tell "log in" from "re-login".
- `ws.lead.login` itself is NOT root-aware in the resolver sense — it takes `root`
  as its bootstrap input and must remain callable without a prior key.
- The non-lead-key `ws.lead.*` prefix block (Phase 1) stays; it is the containment
  the mercenary surface relies on.

## Verification Contract

- `cd agents-plugin-tool && go build ./... && go vet ./... && go test ./... -count=1` green.
- Python launcher unittest green if skill/launcher text changed.
- Grep proof: no `isSetupFenceRequest`, `ensureChildActor`, `childActorInstruction`,
  `restoreActor`, `bindActor`, `FindActor`, or `ws.setup` dispatch remains in the
  Go tree; no surviving `ws.setup` bootstrap instruction in shipped skill text.
- The integration cases above all assert.

## References

<!-- [Must]: read before starting. [Maybe]: consult if uncertain. -->
- `ai-docs/spec/mcp-tools.md` #260610-ephemeral-session-auth-model — [Must] the session-key contract this slice makes mandatory.
- `ai-docs/spec/mcp-tools.md` #260610-mercenary-delegation-surface — [Must] the 2c endpoint; confirms render-minted child keys are NOT 2a (compile-not-function boundary).
- `ai-docs/spec/mcp-tools.md` #260524-mcp-actor-setup-bootstrap — [Must] the actor/fence machinery being deleted.
- `ai-docs/spec/mcp-tools.md` #260505-mcp-session-default-root — [Must] the volatile default-root + fence the key map replaces.
- `ai-docs/spec/mcp-tools.md` #260523-agents-root-schema-invisibility — [Must] the hidden explicit-root contract removed in 2a.
- `ai-docs/spec/mcp-tools.md` #260505-tool-profile-gating — [Maybe] capability-scope context for keyed calls.
- `ai-docs/mental-model/mcp-runtime.md` — [Must] resolver chain, fence, keyed-call handler, wsstore actor persistence entry points.
- `ai-docs/mental-model/named-agent-runtime.md` — [Must] AgentInternalKey actor scoping, child-actor injection, actor-scoped dispatch.
- `ai-docs/mental-model/ws-web-dashboard.md` — [Maybe] Activity Console reads wsstore actor records; deletion may break the dashboard build (Phase 3 note).
- `ai-docs/mental-model/prompt-bundle.md` — [Maybe] how child keys splice into prompts (2c context).
- `ai-docs/ref/ws-mcp.md` — [Maybe] operational runbook for the machinery being deleted.
