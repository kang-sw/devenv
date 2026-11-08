# Plan: tracker structure - sibling order, hover insert, item refs, assignment rules and popover

Branch `delegate/tracker-structure` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`), based on
develop. Commit per logical unit here; do not merge or push. Do not run git
stash.

## Authority

`ai-docs/tickets/todo/261107-epic-ws-personal-common-store.md`
`## Cross-Child Decisions`: the **Assignment** bullet's two "Amended
2026-11-08" entries and the **Item order and references** bullet (commit
0e7474e1a). Board items: (490)-(492) assignment rules, (528) a sibling order,
(529) self-assign on create, (530) a search popover, (531) a refs with
backlinks. The mockup README `## Settled` table is the detailed design record;
add rows for these items there.

Read first: `ai-docs/manuals/shipped-surface-boundary.md`,
`ai-docs/manuals/ws-mcp.md` (you touch `internal/mcp/tracker_tools.go`),
`ai-docs/manuals/skill-authoring.md` (you touch
`agents-plugin/rsrc/tracker/tracker.md`), `ai-docs/manuals/wsflow-mirroring.md`.

## Layers, in this order

The established flow is mockup engine -> Go `wstracker` (golden/scenario
tests keep the two in step) -> renderer `tracker.js`, shared by the mockup
(through `ai-docs/ref/tracker-mockup/mock-adapter.js`) and the dashboard.

### 1. Store model (engine.js + fixtures, then Go wstracker)

- **Sibling order.** `item.create` payload gains optional `after`: a sibling
  item id (same parent) or empty/absent meaning first... decide and document
  the absent case so old events keep their current creation-order placement
  (absent = append after existing siblings in creation order, which matches
  today's view; explicit `""`/null = first, if you need it). The view's
  `itemOrder` and every per-parent child list follow the anchors. An anchor
  that names a missing, hidden, or different-parent item falls back to
  append; concurrent inserts after the same anchor order deterministically
  (by the existing display-winner tiebreak: ts, member id, seq). Reorder and
  reparent are out of scope now; keep the field shape able to take an
  `item.move` later (note it in the README, no event yet).
- **Refs.** `item.create`, `item.edit` and `comment.add`/`comment.edit` gain
  optional `refs`: item ids. Kept on edit only while `#<id or alias>` stays
  in the text (mirror the mentions rule; fix the substring weakness for refs
  by matching a token boundary). The view exposes per item `refs` and
  `backrefs` (items and comments that reference it, hidden ones excluded).
- **Assignment rules.** `item.assign` targets become a list mixing people
  and subtree rules `{root, include_root, levels?}` (levels absent = whole
  subtree). The old `levels="N"` form reads as `{root: author,
  include_root: false, levels: N}`. A rule whose root is within the author's
  reach is live/forced as today's "everyone within N" (including later
  joiners in range); a rule rooted outside the author's reach is pending to
  the root member; the root's accept makes it live with forced semantics and
  notifies the accepter's direct lead, who may return it. People targets keep
  the (273)/(279)/(280) semantics. Follow the README's existing assignment
  sections and extend them.
- Payload validation in `payload.go`, replay/state/view in Go; engine.js the
  same. Golden/scenario tests: insert between siblings, anchor fallback,
  concurrent same-anchor inserts, refs kept/dropped on edit, backrefs,
  rule assign within reach, rule rooted outside reach pending -> accept ->
  live, old levels form unchanged, mixed people+rule event.

### 2. MCP tracker surface

- `tk:item.create` gains `after?` and `refs?`; `tk:item.edit`,
  `tk:comment.add`, `tk:comment.edit` gain `refs?`; `tk:item.assign` gains a
  way to pass subtree rules alongside `people` (e.g. `rules="maya:1 ada:*"`
  or `rules="maya+:2"` for include_root; pick one compact grammar, document
  it). Use the new `<tk:store name>` group form that just landed (commit
  1a8399d0f). Update `agents-plugin/rsrc/tracker/tracker.md` (command table,
  attribute notes) and regenerate the manifest, wsflow mirror and pi rsrc
  copy with the documented commands; tests in `internal/mcp/tracker_*_test.go`.

### 3. Renderer (tracker.js / tracker.css) - WAIT for the lead's go message

Another delegate is finishing work in tracker.js's item window (mention
popup, header actions, subtask section, assignee `+` / red `−` badge). Do
layers 1-2 first and commit them. Then report "layers 1-2 done" with commit
hashes via your final message only if the lead's go message has not arrived;
when the lead messages "go", `git merge develop` into your branch, then:

- **Hover insert.** Remove the store card's top-right `+` new-item button.
  In list views, hovering near a row shows a `+` badge at the left edge
  between rows; the horizontal mouse position picks the depth (root, sibling
  of the row above, child of the row above), shown by the badge's indent and
  a guide line. Click opens the existing inline new-item row at that spot,
  creating with `parent` and `after`. Keyboard path: keep the existing
  shortcut for a new root item if any. An empty list still offers one `+`.
- **Self-assign on create (529).** A `task` or `todo` created from the
  dashboard is assigned to the creator in the same batch (`$1`). `topic`
  excluded.
- **Assign popover (530).** Replace the checkbox-grid assign flow with a
  popover anchored at its trigger (the assignee card's `+` the other delegate
  is adding; it exposes a trigger function to swap): search field; selected
  chips (people and rules); "내 팀" rule rows (L1 / L2 / whole subtree, plus
  a rule rooted at another member found by search, marked pending when
  outside reach); member rows with avatar, name, id, and a live/pending mark;
  completion mode (assignee closes / I confirm); apply. Keyboard: arrows,
  Enter toggles, Esc closes. Replace the awkward red "select at least one"
  message with a disabled apply button.
- **`#` refs.** In the composer (and body editors where the `@` popup
  exists), `#` opens the same style of popup as `@`, searching items by
  alias and title; picking inserts `#<alias> ` and records the ref. Render
  `#alias` in bodies and comments as a link that opens that item (title on
  hover). The item window shows a "참조됨" (backlinks) section listing items
  and comments referencing it, when non-empty.

### i18n

New keys in en.json and ko.json only; remove keys that become unused from
every table and list them. Regenerate the mockup `i18n-en.js` with
`WS_REGEN_TRACKER_MOCKUP_I18N=1` when keys change.

## Verification

`cd agents-plugin-tool && gofmt -l . && go vet ./internal/wstracker/ ./internal/mcp/ && go test -count=1 ./internal/wstracker/ ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wsrsrc/ ./cmd/ws-mcp/` and the tracker tests in `./internal/mcp/` (`-run 'Tracker'`), `python3 -m unittest discover agents-plugin-wsflow/tests`, `node --check` on tracker.js and engine.js. Headless Chromium on the mockup (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`), never port 29417: hover insert at three depths, assign popover with a rule, `#` popup and a backlink. Shots in `/tmp/tracker-structure/`.

## Return

Per layer: commit hashes, changed files, test output, screenshots (layer 3), i18n keys added/removed, the chosen `rules` grammar and `after` absent semantics, and anything left open.
