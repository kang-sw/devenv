# Plan: split the session key record into per-purpose files

Branch `delegate/session-key-split` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/reference-outcast-afford`), based on
develop. Commit here; do not merge or push. Do not run git stash.

Read first: `ai-docs/manuals/ws-mcp.md` (edits under `internal/mcp/`),
`ai-docs/manuals/shipped-surface-boundary.md`.

## Why

Every session lives in one file, `<cacheRoot>/keys/<key>.json`
(`internal/mcp/session_auth.go` `sessionRecord`): identity (schema_version,
root, scope, parent), overrides, agenda, todos, note, nudge and mailbox
flags, and the whole whiteboard store. Every authenticated tool call runs
`lookup` -> `readRecord`, which parses the whole file; a long lead session's
file is ~490KB, mostly whiteboard. Every mutation (`mutateRecord`,
`session_state.go` ~L595) rewrites the whole record, so two processes
writing different parts (a whiteboard update and an agenda write) are
last-writer-wins over each other. The user approved the split (board (484)
a, (485) ok) and asked that downstream compatibility be handled well.

## Layout (settled, (484) a)

```
<cacheRoot>/sessions/<key>/key.json         identity + small fields:
                                            schema_version, root, scope,
                                            parent, overrides, note,
                                            review_track_nudge_shown,
                                            mailbox_reply_opened
<cacheRoot>/sessions/<key>/agenda.json      agenda map
<cacheRoot>/sessions/<key>/todos.json       todos list
<cacheRoot>/sessions/<key>/whiteboard.json  wswhiteboard.State
```

- `key.json` is the key's existence claim and activity mark: mint claims
  uniqueness with `O_CREATE|O_EXCL` on it (after `MkdirAll` of the key
  directory - make the claim race-safe: two minters of the same candidate
  must not both win; e.g. create the directory with `os.Mkdir`, which
  fails with EEXIST, as the claim, then write key.json, or keep the O_EXCL
  on key.json and treat an existing directory without key.json as taken);
  `touch` sets key.json's mtime; prune ages on key.json's mtime.
- A part file that does not exist means empty. Write each part atomically
  (temp in the same directory + rename), as today.
- Bump `sessionRecordSchemaVersion` for key.json.

## Migration and compatibility (settled (485) + lead's technical calls)

- **Read**: look up `sessions/<key>/key.json` first; when absent, fall back
  to the legacy `keys/<key>.json` and serve it in full (identity and all
  parts). A legacy record keeps authenticating with no migration.
- **Migrate lazily, on the first write only** (lead's call, refining
  (485)'s "on read"): the first mutation of a legacy session writes all
  part files and key.json into `sessions/<key>/`, then removes the legacy
  file. Read-only paths (auth lookup, `readState`, the dashboard daemon's
  page render, `children`, `recordMtime`) never migrate. Reason: the
  dashboard daemon outlives `install.sh update` and reads every session; if
  its reads migrated, a still-running older MCP server in a live session
  would lose its key mid-turn. Write order for crash safety: part files,
  then key.json (atomic), then remove legacy; a crash in between leaves
  both and the new layout wins (reads check it first), and a later write
  finishes the removal.
- **Mixed versions**: an older binary only knows `keys/`. After a
  migration it reports the key unknown. Document in
  `ai-docs/manuals/ws-mcp.md` that all ws processes (session MCP servers and
  the dashboard daemon) are restarted after updating, as already expected
  for install updates, and that downgrading loses sight of migrated
  sessions (session keys are cache state). Mint only in the new layout, but
  also refuse a candidate whose legacy `keys/<key>.json` exists.
- Do not add a bulk migration pass; legacy files age out through prune.

## Readers and writers to convert (verify the list yourself)

`internal/mcp/`: `session_auth.go` (keysDir, keyPath, mint, lookup,
recordMtime, children, touch, readRecord, writeRecordAtomic, overrides,
setReviewTrackNudgeShown), `session_state.go` (mutateRecord, readState and
their callers: agenda, todos, note), `agent_keys.go` (its own O_EXCL mint),
`dashboard_sessions.go` (~L477 reads key files directly for the daemon
page), `session_prune.go` (mtime GC: prune both layouts; a stale
`sessions/<key>/` directory is removed whole, on key.json's mtime; a
directory with no key.json and an old mtime is removed too; mailbox history
pruning unchanged), `whiteboard_tools.go` (~L675), and any other
`keysDir()`/`keyPath(` or `sessionRecord` user (grep). Check whether
`internal/execjob` and `internal/wsagent` reference the session keys dir;
they appear to have their own key paths - leave them if so.

Split the read-modify-write so a mutation touches only its part(s): e.g.
`mutateWhiteboard`, `mutateAgenda`, `mutateTodos`, `mutateKey` (identity +
small fields), or one generic helper parameterised by part. The
authenticated lookup reads key.json only. Callers that need several parts
read just those. Keep the in-process `s.mu` serialization.

A parallel delegate is changing `internal/wswhiteboard/` and
`whiteboard_tools.go` (topic header: close result, closed-topic cap). Treat
`wswhiteboard.State` as an opaque blob moved as is; keep your edits in
`whiteboard_tools.go` to the load/save seam. The lead merges both.

## Tests

- New-layout mint/lookup/children/recordMtime/touch.
- Legacy record: authenticates and serves all parts with no files created
  under `sessions/`; first agenda (or whiteboard) write migrates: parts
  present, legacy file gone, every part's content preserved (agenda, todos,
  note, overrides, flags, whiteboard).
- Crash-shaped state: both layouts present -> new wins, next write removes
  the legacy file.
- Independent parts: a whiteboard write does not rewrite agenda.json (and
  vice versa) - e.g. check file content/mtime unchanged.
- Prune: stale legacy file removed; stale `sessions/<key>/` removed whole;
  fresh ones kept; malformed key.json still pruned on mtime.
- Mint collision against an existing legacy file and an existing new dir.
- Dashboard session listing sees both layouts.
- Existing session/agenda/todo/whiteboard/dashboard tests pass unchanged in
  meaning (update fixtures that write raw key files).

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./... && go test -count=1 ./internal/mcp/ ./internal/wswhiteboard/ ./internal/wsdashboard/`
then the full `go test -count=1 ./...` once (known load flakes:
TestPiBackendStart, TestStickyFallbackPort - rerun individually if they
fail). Also a manual smoke: build `ws-mcp` and confirm a copied legacy key
file from `~/.cache/ws@kang-sw-devenv/keys/` (copy into a temp
WS_CACHE_HOME, never modify the real cache) authenticates and migrates on a
write.

## Return

Commit hashes, changed files, the final reader/writer list, test output,
anything left open (especially any reader you found outside `internal/mcp`).
