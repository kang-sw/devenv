---
title: Session-key parent lineage and ws.session.children enumeration
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: settled anchor — root_override child-key mint path and strict depth-1 containment this ticket grounds on
  260523-bug-implement-merge-target-discovery: merge-back is the independent action this lineage does not own
  260523-chore-implement-branch-cleanup-guidance: worktree teardown/cleanup remains a separate concern
  260503-feat-ws-mcp-worktree-orchestrator-lock: existing worktree concurrency primitive; lineage does not create worktrees
  260512-feat-llm-readable-mcp-output-defaults: output principle children() must follow
spec:
  - 260619-session-key-lineage-children
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
  - plugin-runtime
---

# Session-key parent lineage and ws.session.children enumeration

## Background

A lead session minting per-root keys has no durable record of which keys it
spawned. The key→root binding survives on disk
(`<cache-root>/keys/<key>.json`), but the lineage edge — which session minted a
given key — exists only in the rendered prompt splice and in lead context. Lead
context is compacted lossily, so after a `/compact` or session restart the lead
can no longer re-enumerate its outstanding keys to re-thread or prune them.

This ticket records a parent edge at mint time and adds a read-only enumeration
tool so the lead can recover its full subtree from one durable anchor (its own
control key).

### Two worktree scenarios

The design must keep two distinct worktree use cases separate; conflating them
caused an apparent containment conflict during discussion:

1. **dispatch-into** — the lead dispatches a child to run independent work inside
   a worktree. The child is a delegate **leaf** (restricted scope); recursion is
   not wanted. This is the original motivating case.
2. **work-in** — the lead itself decides "let's work in this worktree" and drives
   it directly. This needs **full (control) scope**. Multiple such keys in one
   conversation = one lead coordinating multiple MCP roots; it is not a tree of
   separate control agents.

These map cleanly onto the two existing mint paths and preserve the anchor's
strict depth-1 containment (see Decisions).

## Decisions

- **Lineage edge is one optional `parent` field** on the session record,
  recorded by both mint paths. `children()` labels each child by its stored
  `scope` (control vs delegate/leaf), never by comparing roots.
- **Scenario (1) reuses the anchor-settled render path**, not a new ferrule path.
  `260605-research-ws-native-subagent-pivot` settled
  `playbook.render(session_key, name, context?, root_override?)` (option (c),
  alternatives a/b rejected) as the worktree-bound child-key mint mechanism:
  `root_override` binds the minted delegate leaf to the worktree. This ticket
  only **adds `parent`** to that mint, recording the dispatching lead key. No
  competing ferrule-based leaf path is introduced.
- **Scenario (2) uses `ws.ferrule`**, which already mints a control key per root
  (a worktree is a distinct root, per the per-workroot manual clarification,
  `13eeccd9`). `ws.ferrule` gains an optional `parent_session_key` recording
  coordination lineage — the lead's primary control key as parent of the
  additional root keys it ferrules. Metadata only; no new capability.
- **Depth-1 containment is preserved and rests on the existing lead-only ferrule
  guard — this ticket invents no new constraint.** `isLeadOnlyTool`
  (`server.go:59-61`) lists `ws.ferrule`; the keyed `tools/call` gate
  (`server.go:340-345`) hard-rejects a non-lead key calling it, and the obscurity
  soft-guard (scrubbed description + manual-only teaching) covers the keyless
  self-bootstrap gap the anchor accepts as soft. Only the lead calls `ferrule`;
  dispatched children are non-recursive leaves. The lead ferruling N roots is
  already allowed (the lead key is `roleLead`, so the gate's non-lead branch is
  skipped) and already documented.
- `ws.session.children` returns child **key strings** (credentials): keys are
  lead-private, the caller already holds the parent key that minted them, and
  re-threading after compaction is the feature's purpose.
- Worktree creation/removal stays out of scope (native git tooling). Merge-back
  of parallel worktrees is an independent action owned by the lead and the
  merge/cleanup idea tickets; lineage does not perform it.
- A per-key human note is out of scope; lineage + enumeration already solves
  re-discovery.

## Constraints

- Schema growth only: add `parent` to the session record as
  `json:"parent,omitempty"`; `schema_version` stays `1` (the format ignores
  unknown/absent fields, so no migration and older readers tolerate it).
- `children()` is a read-only tool whose primary consumer is the lead LLM, so it
  must default to compact labeled text and expose `format:"json"` only as an
  escape hatch, per `mcp-tools.md` `{#260512-mcp-llm-readable-output-defaults}`.
- A caller may only enumerate the subtree under a key it presents; `children()`
  must not expose unrelated sessions.
- `parent`, when present, references a control-scope key (only control keys
  mint). Recording a parent must never widen the child's scope.

## Phases

### Phase 1: Parent lineage in the session store and the render (scenario-1) path

Add `parent` to `sessionRecord` (`json:"parent,omitempty"`) and to the low-level
`sessionStore.mint(root, scope, parent)` signature; `lookup` returns `parent` in
the resolved `sessionEntry`. Thread the lead key already resolved at the render
handler (`server.go` derives `mintRoot` — and honors `root_override` — from a
looked-up lead `session_key`) down through `renderPlaybook` →
`renderPlaybookBody` → `mint`, so render-minted delegate leaves (including
worktree-bound leaves via `root_override`) record their dispatching lead key as
`parent`. This is value-plumbing of an already-resolved key, not a new lookup.

Notes:

- Absent parent (legacy keys, non-lineage mints) stays valid: empty `parent`.
- This phase deliberately rides the existing `root_override` path (scenario 1);
  it does not add a ferrule-based worktree-leaf path.

Verification: store round-trips `parent`; a render-minted leaf carries the lead
key as parent; a `root_override` worktree leaf likewise; omitempty keeps legacy
records readable; concurrent mints do not clobber parent.

### Result (95d56b26) - 2026-06-19

Landed (impl `1f72fa3c`, manifest-drift fix `95d56b26`). All Phase 1 decisions
honored:

- `sessionRecord` gains `Parent string \`json:"parent,omitempty"\`` and
  `sessionEntry` gains `parent string`; `schema_version` stays `1`. The
  concurrently-added `Overrides` field is preserved across the json round-trip
  (`session_auth.go`).
- `sessionStore.mint(root, scope, parent)` writes `Parent`; `lookup` populates
  `entry.parent`. `setPreferMercenary`'s read-modify-write preserves `parent`
  (unmarshal/marshal round-trip; test-confirmed).
- Render path threads the lead key: `server.go` callTool sets `parentKey = keyStr`
  inside the `entry.scope == roleLead` branch (so only lead callers record a
  parent, and it is the lead's own key), passed through
  `renderPlaybook → renderPlaybookBody → mint`. `root_override` worktree leaves
  record the same lead parent (parentKey is independent of mintRoot).
- Bootstrap/print stay parentless: `handleLeadLogin` and `printPlaybook` pass `""`.
  No `parent_session_key` arg and no `ws.session.children` tool added (Phases 2/3).

Verification: `go test ./internal/mcp/... ./internal/wsrsrc/...` green; full
`go test ./...` green (one transient subprocess-timing flake, not reproduced on
re-run). Four required tests present and passing:
`TestSessionMintRoundTripsParent`, `TestRenderPathMintedChildRecordsLeadParent`,
`TestLegacySessionRecordWithoutParentResolves`,
`TestSetPreferMercenaryPreservesParent`. `go build ./...` clean.

Incidental fix: the two `lead-workflow-manual.md` golden-hash failures that prior
slices logged as "pre-existing" were stale `manifest.json` + wsflow mirror drift
left by the per-workroot manual edit (`13eeccd9`). Regenerated via the canonical
seams (`WS_REGEN_MANIFEST=1`, `WS_REGEN_WSFLOW_RSRC=1`) in `95d56b26`; those
golden tests now pass.

### Phase 2: Optional parent on ws.ferrule (scenario-2 coordination lineage)

Add an optional `parent_session_key` argument to `ws.ferrule`. When present, the
minted control key records it as `parent`. This is metadata only and grants no
new capability — `ferrule` already mints a full control key per root. The
primary root's control key is parent-less; an additional coordination root (a
worktree the lead drives directly) passes the lead's primary control key as
parent.

Verification: ferrule with `parent_session_key` records parent; without it the
key is parent-less; an invalid/unknown parent is handled without minting a
mislinked key; behavior is identical in wsflow no-agent mode; the existing
lead-only gate is unaffected (a non-lead key still cannot call ferrule).

### Result (12eb1bbe) - 2026-06-19

Landed (`12eb1bbe`). All Phase 2 decisions honored:

- `ws.ferrule` input schema gains an optional `parent_session_key` (not in
  `required`); description kept terse and consistent with the deliberately
  obscure ferrule surface (`server.go` tools list).
- `handleLeadLogin` reads `parent_session_key`, `TrimSpace`-normalizes it, and
  when non-empty validates it via `s.sessions.lookup`; an unknown parent returns
  an error and mints no key (faithful to "without minting a mislinked key"). The
  validated (or empty) parent threads into the Phase-1-widened
  `s.sessions.mint(canonical, scope, parentKey)`. Empty string behaves as absent.
- No parent scope/capability enforcement added: parent is metadata only and never
  widens scope (scope still comes from `parseCapabilityScope`, untouched). The
  lead-only ferrule gate is untouched.

Verification: `go test ./internal/mcp/...` green; `go build ./...` clean. Four
tests present and passing: `TestFerruleWithParentSessionKeyRecordsParent`,
`TestFerruleWithoutParentSessionKeyMintsParentlessKey`,
`TestFerruleUnknownParentSessionKeyErrorsWithoutMint`,
`TestFerruleEmptyParentSessionKeyBehavesAsAbsent`. Wsflow no-agent mode shares
the same code path (mode-independent plumbing), so no redundant separate test was
added.

Spec `260619-session-key-lineage-children` stays 🚧 until Phase 3 lands the
`ws.session.children` enumeration tool (the caller-facing read surface).

### Phase 3: ws.session.children enumeration tool

Add a read-only `ws.session.children(session_key, depth?, format?, include_dead?)`
tool that scans the flat `keys/` store, returns the subtree whose `parent` chain
roots at the presented key, and labels each child by stored `scope` (control
coordination keys vs delegate/leaf). Default output is compact labeled text (the
tree, including the child key strings for re-threading); `format:"json"` is the
structured escape hatch.

Settled contract decisions:

- **`depth`**: integer, default `1` (immediate children); higher = that many
  levels; `depth: 0` = full subtree. The tree is shallow in practice, so the
  bounded default is the safe choice.
- **Stale/dead keys**: the store has no eviction, so a removed worktree leaves a
  key whose `root` path no longer exists. Liveness = the key's `root` path still
  exists. Dead keys are **filtered by default**; `include_dead: true` returns
  them flagged (`live: no`) so the lead retains a prune/debug path.

Spec/docs: the caller contract is specified in
`mcp-tools.md` `{#260619-session-key-lineage-children}`. Phase 3 also updates the
`mcp-runtime` / `named-agent-runtime` mental models.

Verification: a control key returns its delegate leaves and its coordination
control keys; depth bounding works (`depth: 0` = full subtree); dead keys are
filtered unless `include_dead: true`; text default carries re-threadable keys;
json escape hatch returns stable fields; a leaf key returns an empty/flat result;
enumeration never crosses into unrelated sessions.
