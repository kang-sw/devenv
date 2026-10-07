---
title: agenda.set merge option applies a JSON Merge Patch to the stored blob
related:
  261007-bug-session-record-cross-process-write-race: sibling; the merge runs inside the same session-record read-modify-write that ticket locks
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2498861d01ecf26b
sage-review-completeness-reviewed: 2498861d01ecf26b
completed: 2026-10-07
---

# agenda.set merge option applies a JSON Merge Patch to the stored blob

## Background

Downstream dogfood (2026-10-07): a lead keeping work state in agenda blobs
re-sent the whole blob through `agenda.set` for every small change, because
`agenda.set` only replaces (`handleAgendaSet` ->
`sessionStore.setAgendaNote`, `agents-plugin-tool/internal/mcp/session_state.go`). Changing one field of a large blob costs the whole blob in
tool-call arguments each time.

## Decisions

- **Opt-in `merge` argument on `agenda.set`.** `merge: true` applies `value`
  to the stored blob as an RFC 7396 JSON Merge Patch instead of replacing it;
  absent or `false` keeps today's replace. Rules, per RFC 7396:
  - a field with a non-null value sets that field;
  - a field whose value is `null` removes that field;
  - an object value merges recursively into the stored object field;
  - any other value (array, string, number, boolean) replaces the field
    whole.
  Rejected: a separate `agenda.apply` tool with revisions, compare-and-swap,
  and multi-operation batches - with one writer per key, the session-record
  lock removes the race those guard against, and the merge alone removes the
  re-send cost.
- **A missing key is created from the patch.** `merge: true` on a key with no
  stored blob applies the patch to `{}`, so `null` fields drop out. Rejected:
  an error that forces a plain `agenda.set` for creation - the caller would
  have to check existence first, which is the overhead the option removes.
- **A stored non-object blob is treated as `{}`** before the patch applies,
  as RFC 7396 defines; the result replaces it.
- **With `merge: true`, `value` must be a JSON object.** A non-object or
  `null` `value` is rejected with an error and nothing is stored. The schema
  already declares `value` as an object, but `handleAgendaSet` does not
  enforce it; RFC 7396 would let a non-object patch replace the blob whole,
  and a `null` patch would store a `null` blob. Plain replace (`merge` absent
  or `false`) keeps today's acceptance unchanged.
- **Whole-key removal stays with `agenda.clear(key)`.** The merge removes
  fields inside a blob, never the agenda key itself.
- **Known RFC 7396 limit, accepted:** a field cannot be set to a literal
  `null` through the merge, because `null` means removal.

## Constraints

- The merge happens inside the session-record read-modify-write
  (`mutateRecord`), so it composes with the cross-process lock of
  261007-bug-session-record-cross-process-write-race whichever lands first;
  it adds no separate read-then-write path.
- The response text and the agenda size note stay as today.
- No `runtime.json` capability-range edit: the `agenda.set` ranges in
  `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json` track
  the plugin version through `bump-ws-version.sh`, and the property ships
  with the binary of the same version (`ai-docs/manuals/ws-mcp.md`, runtime
  compatibility).
- The `agenda.set` tool description and the `merge` property description
  state the rules above briefly, including `null` removal and whole array
  replacement.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261007-bug-session-record-cross-process-write-race (2026-10-07, Decisions): "Serialize every read-modify-write of a session record across processes with a file lock beside the record... The lock covers the whole read-modify-write. Out of scope: revisions, compare-and-swap..." — bearing: constrains
- 5ffaea72 (2026-10-02, commit): "agenda.set computes the note inside the same mutateRecord write (setAgendaNote) so the total reflects the stored state of that write; setAgenda keeps its signature as a thin wrapper for existing callers." — bearing: constrains
- 261002-feat-agenda-size-nudge-and-pi-manual-state-dedupe (2026-10-02, Decisions): "Size measure: the sum of stored blob value byte lengths (key names excluded)... `agenda.set` computes the total after the new blob is stored." — bearing: constrains
- 260702-feat-agenda-enumerate-and-clear-all (2026-07-02, commit f765a723): "agenda.list summarizes object blobs by their top-level key set (matches the compact style already used for agenda blob previews) rather than dumping full JSON" — bearing: supports
- 260625-feat-ws-session-state-machine (2026-06-25, commit 54f94a53): "Tools mirrored into the wsflow runtime contract because they are not agent-backed (not hidden in no-agent mode); the wsflow contract is exact-match." — bearing: constrains
- 8f8f7967 (2026-10-07, commit): "User directive: no mention of the subcontracted session and no session-role wording for agenda in these tickets; the agenda.set merge option (b) awaits the user." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/session_state.go handler and setAgendaNote, agents-plugin-tool/internal/mcp/server.go#L3917-L3929 agenda.set schema, agents-plugin-tool/internal/mcp/session_state_test.go |
| scope.surface | public-interface | agenda.set MCP tool input schema and description gain a merge property |
| scope.new_public_symbol | no | new merge property on the existing agenda.set tool; no new tool or exported Go symbol |
| scope.new_type_contract | yes | agenda.set inputSchema adds optional boolean merge with RFC 7396 semantics |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/session_state_test.go TestStoreAgendaRoundTrip, TestServeStdioAgendaHandler, TestServeStdioAgendaListHandler, TestServeStdioAgendaSetSizeNote |
| complexity.reuse_points | confirmed | sessionStore.mutateRecord and setAgendaNote read at agents-plugin-tool/internal/mcp/session_state.go#L604-L657 |
| complexity.side_effect_risk | low | change is confined to the merge true path inside the existing mutateRecord closure; replace path unchanged |
| risk.correctness | moderate | recursive RFC 7396 merge over json.RawMessage with null removal, missing key, and non-object stored blob cases |
| risk.fit | low | follows the setAgendaNote closure pattern and keeps response text and size note unchanged |
| risk.test | low | test plan enumerates every rule and existing stdio agenda harness tests can host them |
| risk.security_or_contract | moderate | public MCP schema change on agenda.set; agents-plugin/runtime.json#L17 and agents-plugin-wsflow/runtime.json carry agenda.set capability ranges under an exact-match wsflow contract |

## Phases

### Phase 1: Merge-patch option on agenda.set

Add the `merge` argument to the `agenda.set` schema and handler and implement
the RFC 7396 merge over the stored blob. Test: field set, field removal by
`null`, recursive object merge that keeps untouched sibling fields, array and
scalar whole replacement, a missing key created from the patch with `null`
fields dropped, a stored non-object blob replaced, `merge` absent or `false`
keeping replace behavior, a non-object or `null` `value` with
`merge: true` rejected with the stored blob unchanged, and the merged result
visible through `agenda.list` (extend `TestServeStdioAgendaListHandler`).

### Result (0bfcc4549) - 2026-10-07

- `agenda.set` takes an optional boolean `merge`. With `merge: true` the
  handler calls the new `sessionStore.mergeAgendaNote`, which applies `value`
  to the stored blob as an RFC 7396 merge patch inside `mutateRecord` and
  returns the size note from the same write. A missing key or non-object
  stored blob is patched as `{}`. Replace (`merge` absent or `false`) is
  unchanged. Commits: a71eaff02, 0bfcc4549.
- A non-object or `null` `value` with `merge: true` is rejected and nothing
  is stored. A `merge` that is present but not a boolean is also rejected
  (fixed after review), so a mistyped flag never falls back to replace.
- The tool description and the `merge` property description state the rules,
  `null` removal, whole array replacement, and the literal-`null` limit.
  No `runtime.json` edit.
- Decision: stored and patch blobs decode with `UseNumber`, so untouched
  numbers re-encode exactly as stored (pinned by
  `TestStoreMergeAgendaKeepsUntouchedNumbers`). The merged output has sorted
  keys, the same as the replace path.
- Tests: `TestServeStdioAgendaSetMerge` covers every case the phase lists,
  `TestServeStdioAgendaListHandler` now shows a merged blob through
  `agenda.list`, and `TestStoreMergeAgendaKeepsUntouchedNumbers` was added.
  `go build ./...`, `go vet ./...` and `go test ./... -count=1` in
  `agents-plugin-tool` all pass.
- Review (lite): clean, with 2 minor findings. The non-bool `merge` finding
  was fixed. Left open: no test asserts the size note on the merge path. The
  note is computed the same way as on the replace path.
