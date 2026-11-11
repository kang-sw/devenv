# Tracker: subtasks under a topic

## Outcome

User report (2026-11-11): in a downstream project, putting a subtask under
a tracker topic "does not take". The paths disagree today:

- `item.create` already accepts a topic parent for the parent's creator or
  assignee (`needCreatorOrAssignee`, `agents-plugin-tool/internal/wstracker/check.go`
  create rules, ~L600).
- `item.move` rejects it: `check.go` checkMove ~L104-105
  ("parent X is a topic; a topic takes no children"), mirrored in
  `ai-docs/ref/tracker-mockup/engine.js` ~L315-329.
- The dashboard hides it: `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`
  `canSplit` (~L760) excludes `it.kind === 'topic'`, which removes the
  "add subtask" button, the menu entry, and drop/insert targets
  (`insOk`); `mvWhy` (~L779) shows `js.tk.mv.why.topic`.

User decision (306) a: open every path. A topic takes children on create,
move, and in the dashboard, under the same parent rule as any item: the
topic's creator or a member holding a live assignment on it (the existing
`needCreatorOrAssignee` / `canSplit` author-or-mineLive rule), not hidden.
This supersedes the topic-ownership ticket's "no dashboard subtask row"
rule (that ticket is in `.done/`; do not edit it - note the supersession
in your commit's AI Context).

## Changes

1. `check.go` checkMove: drop the topic rejection; keep hidden, creator or
   assignee, and subtree rules. Update the doc comment (~L70) that lists
   "a topic takes no children".
2. `engine.js`: the same in the reference engine, comment included.
3. `tracker.js`: `canSplit` no longer excludes topics; remove the topic
   branch in the move-why function and the `js.tk.mv.why.topic` key from
   all 11 tables under `agents-plugin-tool/internal/wsi18n/tables/`
   (all tables keep the same key set); regenerate
   `ai-docs/ref/tracker-mockup/i18n-en.js` (from `wsi18n.SubsetScript`; a
   test checks it is current).
4. Golden/reason cases: any case pinning the topic-parent move rejection
   flips to an accepted move; add a case where a topic's holder moves an
   item under the topic and one where a non-holder is refused (existing
   creator-or-assignee reason). Regenerate goldens with
   `node agents-plugin-tool/internal/wstracker/testdata/gen-golden.js`
   (never hand-edit), and check Go parity.
5. Check the tracker playbook `tracker.md` (agents-plugin, agents-plugin-wsflow,
   agents-plugin-pi rsrc) for any text saying topics take no children; if
   present, fix all three copies identically and regenerate manifest hashes.
6. Check topic-specific views (a topic's row, progress/rollup, the topic
   window) still render with children; fix only what breaks.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` (assets and playbooks
  ship) and, if a playbook changes, `ai-docs/manuals/wsflow-mirroring.md`
  and `ai-docs/manuals/skill-authoring.md`.
- Work only in this worktree; a peer session shares the main checkout.
- Never run go build/test/vet locally. Use
  `~/.local/bin/ws-rgo -C <worktree>/agents-plugin-tool -m 20 -- test -count=1 <pkgs>`.
  Run at least ./internal/wstracker/... ./internal/wsdashboard/...
  ./internal/wsi18n/... ./internal/mcp/... ./internal/wsrsrc/...

## Verification

- Tests above pass; new golden cases agree between engine.js and Go.
- `node --check` on tracker.js and engine.js.
- If the cached headless chromium is available, drive the tracker mockup
  (`ai-docs/ref/tracker-mockup/`) to show the "add subtask" control on a
  topic you hold and a drag of an item under it; otherwise say so.

## Return

Commit on `delegate/tracker-topic-subtasks` (this plan rides the commit).
Do not merge or push. Report commits, what changed, verification output,
and anything unverified.
