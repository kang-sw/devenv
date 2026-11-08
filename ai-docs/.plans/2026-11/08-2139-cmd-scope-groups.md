# Plan: scope groups in the shared command syntax (whiteboard topic, tracker store)

Branch `delegate/cmd-scope-groups` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`), based on develop.
Commit here; do not merge or push. Do not run git stash.

Source: user, 2026-11-08, board (525) a, (526) a, (527) ok. Today every
`wb:add`, `wb:edit`, and `wb:close` repeats `topic="<long prose name>"`, and a
`wb:add` with a mistyped name silently creates a new topic; every `tk:`
command repeats `store="..."` although a batch writes to one store.

Read first: `ai-docs/manuals/shipped-surface-boundary.md`,
`ai-docs/manuals/skill-authoring.md` (the manual and playbook text changes),
`ai-docs/manuals/wsflow-mirroring.md` (if a shipped skill or playbook text
needs a wsflow mirror), `ai-docs/manuals/ws-mcp.md` (internal/mcp).

## Decisions (settled; do not reopen)

1. **One form only** ((525) a): a scope group element; commands inside it
   take the scoped attribute from the group and may not carry it themselves
   (refused). The old flat attribute form (`topic=` on wb:add/edit/close,
   `store=` on tk: commands) is removed, not kept as an alternative.
   Stored data does not change.
2. **Tracker too** ((526) a): `<tk:store name="team"> ... </tk:store>` holds
   every tk: command except `tk:register` and `tk:sync`, which stay at the
   root and still stand alone in their batch. A batch still writes to one
   store: one `tk:store` group per batch.
3. **Tag names** ((527)): the whiteboard group reuses `wb:topic name="..."`;
   the body replacement moves into a child `wb:body` (empty clears, same
   body again is no change). Inside the group: `wb:body`, `wb:edit`
   (fields wb:old, wb:new; attr replace_all), `wb:add` (attrs id, class),
   `wb:close` (attr result). At the root: `wb:answer`, `wb:patch`,
   `wb:floor`, `wb:embed-dir`. A batch may hold several `wb:topic` groups.
   The old form (a `wb:topic` whose content is raw body text) is refused,
   and the refusal shows the new form.
4. **Shared parser rule** in `internal/wsxmlcmd`: one level of grouping, a
   group element that passes one attribute to the commands inside it.
   Groups do not nest; a scoped command outside its group, a root-only
   command inside a group, and text between commands inside a group are
   refused with the usual position/tag/line problem list. Keep the package
   generic (each Grammar declares its group tag, the attribute it passes,
   and which commands are scoped).

## Work

- `internal/wsxmlcmd` (grammar, tokenizer, Check), `internal/wswhiteboard`
  (commands.go, store.go), the tracker command grammar in
  `internal/mcp/tracker_tools.go` and whatever applies tk: commands.
- Every producer of these command strings: grep the whole tree (Go, the
  dashboard assets such as `tracker.js`/`dashboard.js` if they build tk: or
  wb: strings, the tracker mockup adapter, tests, fixtures, ai-docs/ref
  mockup, shipped skills/playbooks/rsrc examples, the workflow manual's
  shared command-syntax, whiteboard, and tracker sections, tool
  descriptions, and refusal text that repeats the rules). Update them all
  to the new form.
- Shipped text: concise, downstream-neutral; state the group rule once in
  the shared command-syntax section and show it in each tool's example.
  Remove "no root element"/"flat commands" wording that becomes false.

## Concurrency

A ticket worker on `impl/develop/judo-jab-outer` is changing the tracker
gate and the `tracker.query`/`tracker.update` descriptions (removing
"Lead-only"). Keep your edits to those descriptions limited to the syntax
example lines so the merge stays small.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./...` on touched packages;
`go test -count=1 ./internal/wsxmlcmd/ ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/wstracker/ ./internal/wsi18n/ ./cmd/ws-mcp/` and
`go test -count=1 ./internal/mcp/` (full; note flaky TestPiBackendStart /
TestStickyFallbackPort under load - rerun those alone before calling them
failures), plus any package whose tests embed these strings (grep). Tests:
parser group cases (scoped outside group, root-only inside, nested group,
attr on inner command, several groups, old wb:topic body form refused with
the new form in the message), whiteboard and tracker end-to-end batches in
the new form. `node --check` on any touched JS.

## Return

Commit hashes, changed files, test output, every producer you changed, and
anything left open.
