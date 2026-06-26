# Brief: ws-session-auth-phase3

## Intent

Land Phase 3 (final phase) of `260609-refactor-ws-spawn-runtime-deletion-session-auth`:
confirm exec is a fully stateless `exec_key` capability with no actor residue,
fold the non-functional `WS_MCP_TOOL_PROFILE` env role-gate into the
already-working capability-scoped session-key gate so keyed scope is the sole
server-side tool-permission authority, and keep the ws-dashboard compiling
against the reshaped session/lifecycle SQLite surface. Lead pre-surveyed all
three areas (three native Explore passes); their findings are folded into this
brief as the binding change set.

## Scope Boundary

In scope (Phase 3 only):
- Remove the residual `owner_actor_id` from the **exec job** record/table.
- Fold `WS_MCP_TOOL_PROFILE`: remove the process-wide env role layer; keyed
  session-key scope becomes the sole role authority.
- ws-dashboard build-fix: confirm it builds; align the stale test fixture.

Explicitly OUT of scope:
- The two Phase 2c Editions (delegate playbook asset + per-spawn tier routing) —
  append-only 2c follow-ups, NOT Phase 3.
- M4 `api.ask` corpus-routing.
- Any dashboard agent-activity feature change (port-vs-remove is a deferred
  dashboard ticket — build-fix ONLY here).
- The shared `artifacts.owner_actor_id` column (named-agent artifact
  infrastructure, not exec-specific) — see Out of scope.

## Caller-Visible Contract

1. **`WS_MCP_TOOL_PROFILE` no longer has any effect.** Setting it does not
   restrict the server's tool surface and it is not propagated to spawned
   mercenary subprocesses. Tool-permission containment for delegates flows
   entirely through the capability scope minted into a session key
   (`ws.lead.login(capability)` and render-minted child keys). This is the
   ticket's "keyed-handler check replaces it" decision; the role-gating *feature*
   is retained, only its mechanism moves from env to keyed scope.
2. **Keyed capability gate is unchanged in behavior** — a non-lead session key
   is still rejected (`-32601`) for `agents.*`/`config.*`/`session.*` (delegate)
   or additionally `api.*`/`git.commit` (leaf), plus any `ws.lead.*` for any
   non-lead key. After the fold this is the ONLY role gate.
3. **`tools/list` advertises the full lead surface** regardless of any env
   profile (already true for `runtime.capabilities` via `LeadToolNames`; now also
   true for `tools/list`). Schema visibility is advisory; enforcement is the
   keyed call gate.
4. **exec is stateless**: `exec_jobs` carries no `owner_actor_id`; exec lifecycle
   is keyed by `exec_key` + session-resolved root only. No caller-visible exec
   schema change (the column was already always empty).

## Contract Instructions

### A. exec stateless — remove `owner_actor_id` (file:line from survey)

`agents-plugin-tool/internal/wsstore/store.go`:
- Struct `ExecJob` (~line 94-125): delete field `OwnerActorID string` (~line 96).
- `UpsertExecJob` (~675-733): remove `owner_actor_id` from the INSERT column list
  (~696), from the `ON CONFLICT` update set (~699
  `owner_actor_id=excluded.owner_actor_id`), and the bound `job.OwnerActorID`
  argument (~728). Adjust placeholder/bind order consistently.
- `ExecJob` reader (~787-806): remove `owner_actor_id` from the SELECT (~788) and
  the `&job.OwnerActorID` scan target (~792). Adjust order.
- `upsertExecStreamArtifacts` (~755): the exec→artifact linkage currently passes
  `OwnerActorID: job.OwnerActorID` (always `""`). Set it to `""` explicitly (the
  shared `Artifact.OwnerActorID` field and `artifacts` table column stay — see
  Out of scope).
- `CREATE TABLE exec_jobs` DDL (~1286-1318): remove the `owner_actor_id TEXT NOT
  NULL DEFAULT ''` column line (~1288) so fresh DBs omit it.
- Add a drop-column migration following the EXISTING Phase 2a pattern
  `recreateTableWithoutColumns` (used by `migrateDropActorTablesAndColumns`,
  ~line 305-375): add a migration that recreates `exec_jobs` without
  `owner_actor_id` for existing DBs, wired into `Migrate()` in the established
  order (after the existing `execJobColumnMigrations` ADD-COLUMN step). Mirror the
  column whitelist style already used for agent tables.

Verify: `go test ./internal/wsstore/... ./internal/execjob/... -count=1` green;
an existing DB with the old column migrates without error; a fresh DB has no
`owner_actor_id` column.

### B. capability-scope fold — `WS_MCP_TOOL_PROFILE` → keyed scope

`agents-plugin-tool/internal/mcp/server.go`:
- `toolAllowed` (2548-2562): remove the `if !roleAllowsTool(s.role, name)` block
  (2555-2557). Keep the product-mode gates (NoAgentMode/wsflowOnly) and the
  `explicitAllowedTools()` allowlist (`WS_MCP_ALLOWED_TOOLS` is a separate env, NOT
  the fold target — preserve it as a visibility allowlist, now independent of
  role).
- Remove the `Server.role` field (~line 32) and its initialization
  (`role := requestedToolRole()` ~line 93) — confirm no other reader remains;
  remove `requestedToolRole()` (2564-2575) once unused.
- Keep `roleAllowsTool` (2577-2591) and the keyed gate (callTool 322-327)
  exactly — that gate is already the working enforcement and becomes the sole
  role authority. The line-308 `toolAllowed` call stays (now only enforcing
  product-mode + allowlist).
- Subprocess env propagation: in `internal/wsagent/codex.go` (~87-88) and
  `internal/wsagent/claude.go` (~102-103), remove the
  `WS_MCP_TOOL_PROFILE=<profile>` env append. Child delegate/leaf scope now flows
  via the render-minted child key spliced into the prompt (Phase 2c). The
  `RegisterOptions.ToolProfile` / `RunnerRequest.ToolProfile` plumbing
  (`agent.go` ~670/789/1082) may be removed if it becomes fully dead after the env
  append is gone; if removal widens the diff into the agent lifecycle beyond the
  env seam, leave the now-inert field and note it (cosmetic deviation) rather than
  refactor the call chain.

Safety rationale the implementer must preserve: removing the env layer is NOT a
containment regression because (a) the mental model verifies the env profile was
non-functional as a barrier, and (b) the working replacement — the keyed gate on
the render-minted child key — already exists from Phase 2c. The accepted soft-guard
hole (a delegate can keyless-`ws.lead.login` to re-escalate) is unchanged and
ticket-sanctioned.

### C. ws-dashboard build-fix

`ws-dashboard/` (Rust workspace):
- Survey result: `cargo build` ALREADY succeeds; production queries
  (`crates/daemon/src/work_root_activity_registry.rs` agent_defs/agent_instances
  SELECTs) never referenced `actor_id`. No production code change required —
  CONFIRM by running `cargo build` and reading output.
- Align the stale test fixture only: `crates/daemon/tests/routes.rs` (~4485-4512)
  still defines `actor_id TEXT` in the `agent_defs` CREATE and binds `''` in the
  INSERT. Remove the `actor_id` column from the fixture CREATE (~4487), the INSERT
  column list (~4509), and the corresponding `''` bind in VALUES (~4512). This is
  fixture hygiene so the fixture matches the reshaped Go schema.

Verify: `cd ws-dashboard && cargo build` green; `cargo test -p <daemon crate>` (or
workspace) green.

## Integration Test Instructions

Run and read full output for each (claim pass only after reading):
- `cd agents-plugin-tool && go build ./... && go vet ./... && go test ./... -count=1`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `python3 -m unittest discover agents-plugin/tests`
- `cd ws-dashboard && cargo build` (and dashboard daemon tests)

Pass criteria: all green; exec migration idempotent; capability-scope tests
exercise the KEYED scope (not the env profile) for the role gate.

## Implementation Strategy Decisions

- Keyed session-key scope is the single role authority post-fold. Do NOT invent a
  new enforcement path — `roleAllowsTool` + the callTool keyed gate already
  enforce; the work is removing the redundant non-functional env layer.
- exec column removal uses the established `recreateTableWithoutColumns`
  migration pattern, NOT raw `ALTER TABLE DROP COLUMN`.
- Dashboard is build-fix only — no Activity feature change.

## Rejected Alternatives

- Keep `WS_MCP_TOOL_PROFILE` as an advisory hint: rejected — the ticket says the
  keyed-handler check *replaces* it and the env profile is verified
  non-functional; leaving a dead env knob invites the "false safety" mistake the
  mental model warns against. Retire it.
- Raw `DROP COLUMN` for `owner_actor_id`: rejected for SQLite-portability/consistency
  with the Phase 2a actor-column removal pattern.
- Removing `artifacts.owner_actor_id` too: rejected as scope creep into
  named-agent artifact infra; left as a forward note.

## Out of scope

- `artifacts.owner_actor_id` shared column (named-agent path) — forward note only.
- `WS_MCP_ALLOWED_TOOLS` semantics (separate allowlist env, preserved).
- Any new exec or capability tool; any dashboard feature change.

## Details

- Role enum: `toolRole` = `roleLead|roleDelegate|roleLeaf`
  (`server.go:39-45`); capability string→role via `parseCapabilityScope`
  (`server.go:1149-1161`); scope stored on `sessionEntry.scope`
  (`session_auth.go:10-16`).
- Keyed gate ordering (callTool 322-327): for a known non-lead key, block
  `ws.lead.*` prefix OR `!roleAllowsTool(entry.scope, name)`. Unknown keys are not
  rejected here (surfaced as `unknown_session` by `resolveToolRoot`).

## Verification Contract

- exec: `owner_actor_id` absent from struct/DDL/queries; migration recreates
  existing tables without it; exec lifecycle tests green.
- capability: `Server.role`/`requestedToolRole` gone; a leaf/delegate **session
  key** is gated by the keyed call gate; a lead key (or keyless bootstrap) is
  unrestricted; `WS_MCP_TOOL_PROFILE` set in env has no effect on the served tool
  surface; no env propagation to codex/claude subprocesses.
- dashboard: `cargo build` green; fixture no longer references `actor_id`.
- Full suites (go, wsflow, launcher, cargo) green.

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `ai-docs/mental-model/mcp-runtime.md` [Must] — tool gate, profile semantics,
  exec job storage rules, change recipes (lines 43, 48-49, 53-58, 74-82, 89).
- `ai-docs/mental-model/named-agent-runtime.md` [Must] — WS_MCP_TOOL_PROFILE
  non-functional verdict + keyed gate as containment (lines 50, 76-77); exec is
  not actor-bound.
- `ai-docs/mental-model/ws-web-dashboard.md` [Maybe] — Activity Console reads
  `state.sqlite agent_defs`; build-fix must not touch the feature.
- Ticket `260609` `## Decisions` → "root vs cwd; exec; role-containment" and
  "Recursion containment" — the binding fold/retain decisions.
