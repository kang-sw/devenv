# Plan: ws-session-auth-phase2a

> Lead-authored source survey (the delegated `plan-populator-survey` run hit the
> 200k context limit mapping this large surface — "Prompt is too long"; the lead
> mapped it with targeted greps instead). Brief: `11-ws-session-auth-phase2a.brief.md`.
> All file:line anchors are against the branch tip at plan time; re-confirm before editing.

## Feasibility verdict

The slice is feasible as one compiling unit. The resolver collapse, actor
deletion, and `AgentInternalKey` re-key have concentrated chokepoints; the
spawn-path child-credential removal compiles cleanly (a spawned child simply gets
no credential spliced until 2c → it hits the mandatory-key reject at runtime,
which is the accepted compile-not-function state). No `[escalate-to-lead]`.

## LANDMINES (do not get these wrong)

- **`RuntimeWriteAuthority` / `WriteAuthority` in `internal/wsstore/metadata_inventory.go` is the STORAGE-authority enum — UNRELATED to actor `Authority`. DO NOT delete or rename it.** Only delete `Actor.Authority`, `ChildActorAuthority`, `sessionActorAuthority`, `actorAuthority()`, `validActorAuthority()`.
- **The setup-fence `wg.Wait()` is ONLY `internal/mcp/server.go:120,143-144,167`.** The `wg.Wait()` in `internal/mcp/api_docs.go:196` and `internal/mcp/api_async.go:296` are the api-manager serialization — LEAVE THEM.
- `AgentInternalKey` has exactly ONE production caller (`internal/wsagent/agent.go:510`); the rest are tests.

## Sequence (each step is a logical commit; build between steps)

### Step 1 — Resolver collapse to key-only (`internal/mcp/server.go`)

`resolveToolRoot` (2107-2156). Keep the `session_key` branch; replace the entire
fallback tail (2119-2155) so:
- no `session_key` arg (absent/blank) → `mandatory_session_key` error naming `ws.lead.login(root)`.
- present-unknown key → existing `unknown_session` error (2113-2114, keep).
- known key → `entry.root` (2116, keep).

The `meta` param and `codexWorkspaceRoots` become unused here — drop the param or
ignore; remove now-dead `codexWorkspaceRoots` only if it has no other caller (grep
first). Distinguish the two error shapes in text so callers tell "log in" from
"re-login".

### Step 2 — Remove `ws.setup` entirely (`internal/mcp/server.go` + `cmd/ws-mcp`)

- Delete the `ws.setup` schema (≈2430-2440) and the dispatch `case "ws.setup":` (394).
- Delete `setupToolName()` (321-324) and the alias remap (355-356); replace its
  callers' error strings (1250,1252,1405,1657,2135,2142,2153) — most of those error
  paths are deleted with the actor/fallback code; for any survivor, point to
  `ws.lead.login(root)`.
- Delete the setup-fence: `isSetupFenceRequest` (171-181) and the three
  `wg.Wait()` fence sites in server.go (120,143-144,167) — leave the `wg` only if
  still used for response-write serialization; otherwise remove the WaitGroup
  wiring. (Verify no other server.go `wg` use before removing the declaration.)
- Remove the `ws.setup` special-cases at 3125, 3142, 3187, 3195, 3202-3203.
- `cmd/ws-mcp`: remove the `ws.setup` CLI mirror command and hidden `--actor-id`
  flags; keep `runtimeCapabilityCommandNames` + `runtime.json.commands` consistent.

### Step 3 — Delete actor machinery (`internal/mcp/server.go`)

- Fields: `sessionRoot` (38), `sessionActorAuthority` (41), `sessionActorID`.
- Functions: actor bootstrap (1257,1307-1316), `restoreActor` (1369), `bindActor`
  (1420), `actorAuthority` (1479), `validActorAuthority`, `validActorIDRest`.
- `s.sessionRoot` assignments (410,1273,1423,1569) and the actor-status JSON
  (1211-1224) — remove or reshape to session-registry terms.
- Child-setup wiring at 856,941,963 (`ChildActorAuthority: child.Authority`) —
  remove (spawn-path credential; replacement is 2c).

### Step 4 — `wsstore` actor removal + AgentInternalKey re-key (`internal/wsstore/`)

- `store.go`: delete the `Actor` type (57 `Authority`), `FindActor` (247), actor
  persistence (533-541), and the `ChildActorID` / `ChildActorAuthority` columns of
  `AgentDefinition` (85,605,613,649) — and the migration/schema for the actor table.
- `metadata_inventory.go:57` `AgentInternalKey(actorID, publicName)` → drop the
  `actorID` parameter; key by `publicName` within the session-root namespace.
  **Recommended:** `AgentInternalKey(publicName string)` keyed on the caller's
  resolved session root (the registration call now carries a `session_key` → root),
  so the same public name under two distinct worktree roots stays distinct. Confirm
  the exact namespace value at the call site (Step 5).

### Step 5 — `wsagent` child-actor removal + caller re-key (`internal/wsagent/agent.go`)

- Remove `ChildActorAuthority` / `ChildActorID` fields (96,111,225,236,376,463,487,651,1194,1264,1287).
- Remove or no-op `ensureAgentChildSetup` (922) and the `ChildSetupInstruction`
  splice — leave `Register`/`Call` compiling (no child credential injected; 2c
  rewires). Prefer deleting the child-setup path over stubbing fake data.
- Update the `AgentInternalKey` caller (510): pass the session-root namespace
  instead of `actorID`. Trace what feeds `actorID` at 510 and replace with the
  registration's resolved root.

### Step 6 — Strip `root` from all root-aware schemas (`internal/mcp/server.go` + `cmd/ws-mcp`)

- Strip the `"root": stringProperty("Repository root. Defaults to the server root.")`
  property from every root-aware tool schema (≈2461,2472,2485,2498,2510,2522,2599,2610
  and any siblings — grep `"root":\s*stringProperty` and remove all EXCEPT
  `ws.lead.login`'s at ≈2448). ws.setup's (2437) goes with the tool.
- Remove the matching `root` flags from root-aware CLI mirrors in `cmd/ws-mcp`.
- Confirm `ws.lead.login` is the only tool whose schema accepts `root`.

### Step 7 — Skill-text migration (read `lead-skill-authoring/SKILL.md` first)

- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md` (3 lines):
  `ws.setup` bootstrap → `ws.lead.login(root)` guidance.
- `agents-plugin/runtime.json`: remove `ws.setup` from the tool contract; keep
  `ws.lead.login`.
- `agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py`: update the contract
  expectation (ws.setup removed).
- (The actual `agents-plugin/runtime.json` ws.lead.login version range from Phase 1
  stays; only remove ws.setup.)

### Step 8 — Tests

- Extend `internal/mcp/session_auth_test.go` with the brief's required cases
  (keyless → mandatory-key; unknown → unknown_session; ws.setup → unknown tool;
  parallel distinct-root no-clobber via sentinel markers; root-omitted keyless
  agents.* → reject; registry re-key distinctness).
- Rewrite/remove actor + child-actor tests: `wsstore/store_test.go`
  (TestAgentInternalKeyScopesPublicNamesByActor at 527 and the actor cases),
  `internal/mcp/server_test.go` (2522,2545 AgentInternalKey usage + actor/setup tests).
- **Test strategy (impl-playbook):** resolver + key logic are pure → tests-first/alongside;
  the wsstore/wsagent storage changes are IO → adjust tests after the code compiles.
- Run: `cd agents-plugin-tool && go build ./... && go vet ./... && go test ./... -count=1`.
  Then the Python launcher / wsflow contract test for Step 7.

## Verification

- build/vet clean; `go test ./... -count=1` green.
- Grep proof: no `isSetupFenceRequest`, `ensureChildActor`/`ensureAgentChildSetup`
  child-credential path, `restoreActor`, `bindActor`, `FindActor`, `Actor` type,
  `sessionActorAuthority`, `setupToolName`, or `ws.setup` dispatch remains; no
  `ws.setup` bootstrap instruction in shipped skill text; `RuntimeWriteAuthority`
  still present.
- wsflow contract test green after Step 7.

## Dashboard note (Phase 3, not 2a)

The Rust dashboard (`ws-dashboard/crates/`) reads agent/actor metadata. The Go
`go build ./...` will NOT catch a Rust break. Phase 3 owns the dashboard build-fix;
if a reviewer or the implementer notices the dashboard reads the deleted actor
table, NOTE it for Phase 3 — do not fix the feature here.
