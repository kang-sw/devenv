---
title: Session-key parent lineage and ws.session.children enumeration
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: direction anchor for native/worktree work surfaces
  260523-bug-implement-merge-target-discovery: merge-back is the independent action this lineage does not own
  260523-chore-implement-branch-cleanup-guidance: worktree teardown/cleanup remains a separate concern
  260503-feat-ws-mcp-worktree-orchestrator-lock: existing worktree concurrency primitive; lineage does not create worktrees
  260512-feat-llm-readable-mcp-output-defaults: output principle children() must follow
spec:
  - mcp-tools
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
  - plugin-runtime
---

# Session-key parent lineage and ws.session.children enumeration

## Background

A lead session minting per-root keys (the `ws.ferrule` control key for each
working root, and render-minted delegate leaves) currently has no durable record
of which keys it spawned. The key→root binding survives on disk
(`<cache-root>/keys/<key>.json`), but the lineage edge — which session minted a
given key — exists only in the rendered prompt splice and in lead context.
Lead context is compacted lossily, so after a `/compact` or session restart the
lead can no longer re-enumerate its outstanding worktree control keys and
delegate leaves to re-thread or prune them.

This ticket records a parent edge at mint time and adds a read-only enumeration
tool so the lead can recover its full subtree from one durable anchor (its own
control key). It elevates a working root (including a git worktree) to a
first-class, re-discoverable work surface without a heavier workset primitive.

## Decisions

- Capability follows the minting tool, not a root-equality side condition:
  - `ws.ferrule` mints **control** keys (first-class) — one per working root the
    lead drives; a git worktree is a distinct root and gets its own control key.
  - render-minted keys are **delegate leaves** (second-class, restricted scope).
- The lineage edge is a single optional `parent` field on the session record,
  recorded by both mint paths. `children()` labels each child by its stored
  `scope`, never by comparing roots.
- `ws.session.children` returns child **key strings** (credentials). This is
  acceptable: keys are lead-private, the caller must already hold the parent key
  that minted them, and re-threading after compaction is the feature's purpose.
- Worktree creation/removal stays out of scope (done out of band via native git
  worktree tooling); this ticket only binds and enumerates keys.
- Merge-back of parallel worktrees is an independent action owned by the lead and
  by `260523-bug-implement-merge-target-discovery` /
  `260523-chore-implement-branch-cleanup-guidance`; lineage does not perform it.
- A per-key human note is out of scope; lineage + enumeration already solves
  re-discovery.

## Constraints

- Schema growth only: add `parent` to the session record as
  `json:"parent,omitempty"`; `schema_version` stays `1` (the format is designed
  to ignore unknown/absent fields, so no migration and older readers tolerate it).
- `children()` is a read-only tool whose primary consumer is the lead LLM, so it
  must default to compact labeled text and expose `format:"json"` only as an
  escape hatch, per `mcp-tools.md` `{#260512-mcp-llm-readable-output-defaults}`.
- A caller may only enumerate the subtree under a key it presents; `children()`
  must not expose unrelated sessions.
- `parent`, when present, should reference a control-scope key (only control
  keys mint). Recording a parent must never widen the child's scope.

## Phases

### Phase 1: Parent lineage in the session store and delegation path

Add `parent` to `sessionRecord` (`json:"parent,omitempty"`) and to the low-level
`sessionStore.mint(root, scope, parent)` signature. Thread the lead key already
in hand at the render handler (`server.go` derives `mintRoot` from a looked-up
lead `session_key`) down through `renderPlaybook` → `renderPlaybookBody` →
`mint`, so render-minted delegate leaves record their minting lead key as
`parent`. This is value-plumbing of an already-resolved key, not a new lookup.

Constraints/notes:

- `lookup` returns `parent` in the resolved `sessionEntry`.
- Absent parent (legacy keys, non-lineage mints) stays valid: empty `parent`.
- Reconcile with the existing render `rootOverride` path: document that
  `rootOverride` remains the delegate-leaf foreign-root mechanism (second-class)
  and is distinct from `ferrule` control keys (first-class); they must not
  compete to own the same worktree concept.

Verification: store round-trips `parent`; a render-minted leaf carries the lead
key as parent; omitempty keeps legacy records readable; concurrent mints do not
clobber parent.

### Phase 2: Optional parent on ws.ferrule (control-key lineage)

Add an optional `parent_session_key` argument to `ws.ferrule`. When present, the
minted control key records it as `parent`. This is metadata only and grants no
new capability — `ferrule` already mints a full control key per root, so
recording derivation does not change what the key can do. The primary root's
control key is parent-less; a worktree control key passes the spawning control
key as parent.

Verification: ferrule with `parent_session_key` records parent; without it the
key is parent-less; an invalid/unknown parent is handled without minting a
mislinked key; behavior is identical in wsflow no-agent mode.

### Phase 3: ws.session.children enumeration tool

Add a read-only `ws.session.children(session_key, depth?, format?)` tool that
scans the flat `keys/` store, returns the subtree whose `parent` chain roots at
the presented key, and labels each child by stored `scope` (control vs
delegate/leaf). Default output is compact labeled text (the tree, including the
child key strings for re-threading); `format:"json"` is the structured escape
hatch. `depth` defaults to immediate children (1) with an opt-in for deeper or
full-subtree traversal.

Open questions to resolve in this phase:

- `depth` maximum / full-tree sentinel shape.
- Whether to filter or flag stale/dead child keys (the store has no eviction),
  or return all and let the lead prune.

Spec/docs: add the tool contract to `mcp-tools.md`, note the `ferrule`
`parent_session_key` argument and the `sessionRecord.parent` field, and update
the `mcp-runtime` / `named-agent-runtime` mental models.

Verification: a control key returns its delegate leaves and child worktree
control keys; depth bounding works; text default carries re-threadable keys;
json escape hatch returns stable fields; a leaf key returns an empty/flat result;
enumeration never crosses into unrelated sessions.

## Spec Impact

- Target spec area: `ai-docs/spec/mcp-tools.md` — new `ws.session.children`
  read-only tool contract; `ws.ferrule` gains optional `parent_session_key`; the
  filesystem session record gains an optional `parent` field.
- Expected caller-visible change: the lead can enumerate its minted subtree
  (worktree control keys + delegate leaves) from one anchor key and re-thread
  keys after context loss; existing single-root flows are unchanged because
  `parent` is optional and omitempty.
- Contract-first spec: yes (the `children()` output contract — fields, labeling
  by scope, text-vs-json defaults, depth semantics — should be specified before
  or alongside Phase 3 implementation).
