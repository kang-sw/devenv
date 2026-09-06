---
title: Pi tier warnings repeat on every workflow_manual call and every ws-agent-list row
related:
  260906-feat-ws-pi-spawn-warns-when-tier-resolution-degrades-to-inherit: owns the per-spawn warning line and the per-tier advisory report this ticket bounds
  260906-bug-ws-pi-tier-slug-rejected-children-inherit-parent-model: root cause of the condition that makes the warnings fire; independent fix
  260906-bug-ws-pi-workflow-manual-static-body-cut-never-matches: prerequisite; rewrites dispatchMappedWorkflowManual and its two advisory call sites, so this ticket's gate lands on top of it
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 9431a5f6d6889cca
sage-review-design-reviewed: 9431a5f6d6889cca
dropped: 2026-09-06
---

# Pi tier warnings repeat on every workflow_manual call and every ws-agent-list row

## Background

Owner report, 2026-09-06, Pi dogfood: "model warnings keep reappearing".
Investigation (read-only, same day) isolated two repetition paths and one
legitimate re-emission:

- **`workflow_manual` advisory, every call.** `maybeAppendModelCatalogAdvisory`
  (`agents-plugin-pi/src/bridge.ts`) recomputes the tier report and appends
  the advisory block on every successful `workflow_manual` response, by
  design ("re-warning on every read while the condition holds", the
  `MODEL_CATALOG_ADVISORY` comment and
  `{#260903-pi-model-catalog-unset-advisory}`). A goal run calls
  `workflow_manual` at the top of every cycle, so the block reappears every
  cycle. The sessions the owner was running predate `e5e09187` and showed
  the older single-paragraph "table has no entries" copy; after `e5e09187`
  the same cadence prints one warning line per rejected tier, four lines
  per call while all four `pi` tiers are rejected (the state described in
  the related slug ticket).
- **`ws-agent-list` rows, every call, for the record's lifetime.**
  `spawnAgent` stores the spawn-time warning on the record and the list
  tool re-renders it for every registry member on every call. Records go
  dormant rather than being deleted, so a session with twenty children
  prints twenty identical warning lines on each list.
- **Per spawn, two channels.** The same line reaches the TUI (`ctx.ui.notify`)
  and the tool result. This is the related tier ticket's settled design
  (one line per spawn, both audiences) and is not changed here.

The warning content is correct in every case; only the cardinality is the
defect. The condition itself (every `pi` tier rejected because ws stores
backend-keyed slugs) is fixed separately by the slug ticket; this ticket
bounds repetition for any future rejected tier.

## Decisions

- **Advisory: once per distinct rejected set per session.** The bridge keeps
  the last emitted advisory key, a stable string built from the sorted
  `<tier>=<value>:<why>` pairs (or `unset` for the empty-table case). The
  block is appended when the key differs from the last emitted one, which
  covers the first `workflow_manual` call of a session, a tier being tuned
  mid-session, and the table becoming clean (an empty key emits nothing).
  While the key is unchanged the response carries no advisory. The four
  `config.resolve_agent` round-trips per call stay; the dedupe is on emission
  only, so a mid-session `config.tune` is still noticed on the next call.
  The key lives in a small holder object owned by the bridge and threaded
  into `maybeAppendModelCatalogAdvisory` as a parameter, so the function
  stays pure for the existing direct-call tests and each test starts from
  a fresh holder. The gate applies inside that function, so all three
  advisory call sites are covered: the two in the mapped
  `dispatchMappedWorkflowManual` branches and the raw-dispatch path in
  `startBridge` taken by unmapped roles.
- **Compaction re-arms the advisory.** The holder resets on the adapter's
  compaction boundary (the same event the goal loop observes), so the next
  `workflow_manual` after a compaction re-emits the block even though the
  rejected set is unchanged; a compacted context has lost the text and the
  advisory is the only pressure.
- **List row: separate field, `model` stays clean.** `ws-agent-list` rows
  are JSON objects; `model` keeps meaning the effective launched model
  (with its effort suffix) as the spec pins it. A rejected-tier row gains
  `tier_rejected: "<alias>: <why>"` and an accepted row has no such field.
  The full warning line with suggestions stays on the spawn/explore result
  and the one-time TUI notify; the list no longer renders the `warning`
  string. Today the record stores only the pre-formatted `warning` string
  and discards the tier alias and the `rejected` detail; `spawnAgent`
  additionally stores a structured `tierRejection?: {alias, why}` on the
  record and the list row renders `tier_rejected` from it. Like `warning`,
  the new field is in-memory only and not persisted to the sidecar. The
  TUI widget row may append `(tier <alias> rejected)` after the model.
- **The tier ticket's deferred spec pass writes the marker form.** The
  related tier ticket's Surfaces bullet says the list row repeats the spawn
  warning and its Spec Impact tells the deferred pass to write that; this
  ticket replaces that sentence. The tier ticket's frontmatter names this
  ticket so the deferred pass writes `tier_rejected` instead, and Phase 1
  amends the `ws-agent-list` bullet directly whether or not that pass has
  run.
- **No spawn-side change.** One line per spawn on both channels remains.
- **Rejected: session-wide "warn once ever".** A tier tuned mid-session to a
  different wrong value must warn again; the key comparison does that.
- **Rejected: dropping the advisory once the slug ticket lands.** A future
  rejected tier (typo, unauthenticated provider) still needs the pointer.

## Spec Impact

`pi-adapter-runtime` `{#260903-pi-model-catalog-unset-advisory}`: replace the
"recomputed and re-appended on every call while the condition holds" cadence
with the per-session key rule (emitted when the rejected set changes,
including the first call and the transition to clean). `ws-agent-list`
bullet under `{#260903-pi-delegation-spawner-tools}`: the row carries the
`tier_rejected` field and `model` stays the effective model. The tier
ticket's deferred spec pass has not yet written the "row repeats the spawn
warning" sentence there; this ticket writes the field form directly and
the tier ticket's frontmatter points the deferred pass here so that
sentence is never added.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no ws-mcp change.
- Lands after `260906-bug-ws-pi-workflow-manual-static-body-cut-never-matches`
  so the advisory gate is added to the rewritten dispatch, not merged
  against it.
- The advisory key holder is in-process state on the bridge; it is not
  persisted, resets with the session, and resets on compaction.
- `computePiAliasTableReport` and `maybeAppendModelCatalogAdvisory` stay
  IO-free; the key is derived from the report and the holder is a
  parameter.

## Phases

### Phase 1: Dedupe the advisory and compact the list row

Add the key holder to the bridge, thread it into
`maybeAppendModelCatalogAdvisory`, gate emission on a key change, and reset
the holder on the compaction boundary; store `tierRejection` on the record
in `spawnAgent`, render `tier_rejected` in `ws-agent-list` rows, and stop
rendering `warning` there. Tests: two consecutive `workflow_manual` calls
with the same rejected set append the block once; a changed set appends
again; a clean table after a rejected one appends nothing and resets the
key so a later rejection warns again; a compaction reset makes the next
call append again with the same set; the unmapped raw-dispatch path is
gated the same way; the list row carries `tier_rejected` for a
rejected-tier record and no such field for an accepted one, and `model`
is unchanged in both; the spawn result still carries the full line. Amend
the two spec passages under Spec Impact. Live check (owner-run): arm a goal
in a session with one rejected tier and confirm the advisory appears on
the first cycle only; tune the tier to a valid value and confirm no
further advisory; `ws-agent-list` after two spawns shows two
`tier_rejected` fields, not two warning lines.


## Resolution (2026-09-06)

Absorbed into 260906-bug-ws-pi-tier-slug-rejected-children-inherit-parent-model (2026-09-06). The once-per-key advisory dedupe with compaction re-arm is its Phase 3; the ws-agent-list tier_rejected marker is dropped because a rejected tier now refuses the spawn and leaves no record.
