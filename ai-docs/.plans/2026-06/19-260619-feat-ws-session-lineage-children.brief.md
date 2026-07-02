# Brief: 260619-feat-ws-session-lineage-children (Phase 1)

## Intent

Record a parent→child lineage edge on minted session keys so a lead can later
re-discover the keys it spawned. Phase 1 implements the storage half (the
`parent` field on the session record) and the delegation half (the render path
records the dispatching lead key as the child's parent). It adds no new
caller-facing tool surface — `ws.ferrule`'s optional `parent_session_key`
(Phase 2) and the `ws.session.children` tool (Phase 3) are out of scope.

## Scope Boundary

In scope (Phase 1 only):
- `sessionRecord.Parent` persisted field + `sessionEntry.parent` resolved field.
- Low-level `sessionStore.mint(root, scope, parent)` signature + writers/readers.
- Thread the already-resolved lead key through the render path so render-minted
  delegate leaves (including `root_override` worktree leaves) record it.

Out of scope (later phases): `ws.ferrule` `parent_session_key` argument (Phase
2); `ws.session.children` enumeration tool (Phase 3); any mental-model edits
beyond what Phase 1 touches.

## Caller-Visible Contract

No new caller-facing contract in Phase 1. The behavior is internal: minted
delegate-leaf keys gain a recorded `parent`, invisible until the Phase 3 tool
exposes it. Existing single-root and existing delegation flows are unchanged
because `parent` is optional and `omitempty` — legacy key files with no `parent`
field still resolve normally. The full eventual contract is specified in
`mcp-tools.md` `{#260619-session-key-lineage-children}`.

## Contract Instructions

Files / modules (all under `agents-plugin-tool/internal/mcp/`):

- `session_auth.go`:
  - `sessionRecord`: add `Parent string \`json:"parent,omitempty"\``. Keep
    `schema_version` at `1` (additive, unknown/absent fields tolerated).
  - `sessionEntry`: add `parent string`.
  - `mint(root string, scope toolRole)` → `mint(root string, scope toolRole, parent string)`;
    write `Parent: parent` into the `sessionRecord`.
  - `lookup`: populate `sessionEntry.parent` from `record.Parent`.
  - `setPreferMercenary` / `writeRecordAtomic` / `readRecord`: must preserve the
    `Parent` field across the read-modify-write (json round-trip already carries
    it; verify the prefer-mercenary update does not drop it).
- `server.go`:
  - `handleLeadLogin` (the `ws.ferrule` bootstrap, ~line 1044): pass `""` as
    parent — the primary bootstrap key has no parent in Phase 1. (Phase 2 will
    add the optional `parent_session_key` argument; do not add it now.)
  - Render handler (~lines 786-801): the lead `keyStr` is already resolved here
    to derive `mintRoot`/`rootOverride`. Pass that `keyStr` into
    `renderPlaybook` as the parent for the child mint.
- `playbook_tools.go`:
  - `renderPlaybook` and `renderPlaybookBody`: add a `parentKey string` parameter
    threaded to the child mint call (~line 397):
    `s.sessions.mint(mintRoot, childScope, parentKey)`.
  - `printPlaybook` (mintRoot=""): passes `""` parent (it never mints).

Reuse the existing mint/lookup/record machinery; do not introduce a parallel
store or a second lineage mechanism. Do not add the `parent_session_key` tool
argument or the children tool (later phases).

## Integration Test Instructions

Extend `agents-plugin-tool/internal/mcp/session_auth_test.go`:

- `mint` round-trips `parent`: a key minted with a parent resolves via `lookup`
  with that parent; a key minted with `""` resolves with empty parent.
- A render-minted delegate leaf records the calling lead key as its parent
  (exercise through the render path, not just the low-level mint, if a test seam
  exists; otherwise assert at the `renderPlaybookBody` level).
- Back-compat: a session record file written without a `parent` field (legacy
  shape) still resolves, with empty `parent` and no error.
- `setPreferMercenary` preserves `parent` across its read-modify-write.

Pass criteria: `cd agents-plugin-tool && go test ./internal/mcp/...` is green;
no new build warnings.

## Implementation Strategy Decisions

- Parent is metadata only; recording it must never change a key's scope (the
  child scope still comes from `childRoleForPlaybookRole`, unchanged).
- The lead key is already in hand at the render handler — this is pure
  value-plumbing of an existing value, not a new lookup or round-trip.
- `schema_version` stays `1`; `omitempty` keeps legacy records readable.

## Rejected Alternatives

- A separate lineage store or a parent index: rejected — the flat per-key record
  already carries everything; one optional field is sufficient.
- A new `ferrule` arg in this phase: deferred to Phase 2 (keeps Phase 1 free of
  caller-facing surface change).

## Approach

- Add the field to record + entry; widen `mint`; update its two callers.
- Thread the lead key through the render path to the child mint.
- Add tests for round-trip, render-path lineage, back-compat, and
  prefer-mercenary preservation.

## Constraints

- Single MCP package; no public Go symbols leave the package newly exported.
- Behavior-preserving for all existing flows (parent absent → identical to today).
- Concurrent mints must not clobber `parent` (existing O_EXCL + atomic rename
  path already guards this; do not weaken it).

## Out of scope

`ws.ferrule` `parent_session_key` (Phase 2); `ws.session.children` (Phase 3);
dashboard/mental-model rewrites beyond Phase 1 contact.

## Details

`mint(root, scope, parent)` writes `sessionRecord{SchemaVersion:1, Root:root,
Scope:string(scope), Parent:parent}`. `lookup` returns `sessionEntry{root, scope,
preferMercenary, parent}`. Render path: `renderPlaybook(..., parentKey)` →
`renderPlaybookBody(..., parentKey)` → `mint(mintRoot, childScope, parentKey)`.

## Verification Contract

`cd agents-plugin-tool && go test ./internal/mcp/...` green; the four test
additions above present and passing; `go build ./...` clean.

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `ai-docs/spec/mcp-tools.md` `{#260619-session-key-lineage-children}` - [Must] the contract this phase begins implementing
- `ai-docs/mental-model/mcp-runtime.md` - [Must] root-aware tool + session-key resolution invariants
- `ai-docs/mental-model/prompt-bundle.md` - [Must] `playbook.render` child-key mint + `PlaybookMeta.Role` eligibility
- `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - [Must] settled session-auth model, root_override child-key path, depth-1 containment
- `ai-docs/mental-model/named-agent-runtime.md` - [Maybe] session-key lifecycle tests context
