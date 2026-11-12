# Tracker: selection mode, quick-add kind toggle, priority and label chips

User feedback from the whiteboard (2026-11-12), lead-settled as whiteboard
items (325)-(328). Dashboard tracker UI only:
`agents-plugin-tool/internal/wsdashboard/assets/tracker.js`, `tracker.css`,
the i18n tables under `agents-plugin-tool/internal/wsi18n/tables/` (every
table holds every English key; Korean strings in ko.json), and their tests.

## Changes

1. **Selection mode is multi-select by default (user).** While at least one
   row is selected (`mselOn()`), a plain click on a list row toggles it (what
   Ctrl/Cmd+click does today) and Shift+click adds the range (what
   Shift+Ctrl+click does today) without clearing the rest. Outside selection
   mode a plain click keeps today's behaviour. The item window still opens
   from the menu or Enter. Update the comment block above `mselClick`.
2. **Floating selection badge ((326)).** Remove the `mselc` chip from the
   Items panel header tools. Instead, while a selection exists, show a
   floating badge at the top centre of the Items panel's scroll area:
   "N selected" plus an "x" clear button (same `msel-clear` action and the
   existing `js.tk.sel.n` / `js.tk.sel.clear` strings). It stays pinned at
   the top of the panel while the list scrolls (sticky, not page-fixed) and
   must not cover the first row's controls permanently (e.g. a small pill
   over the gap, or reserve space). Esc clears the selection when focus is in
   the list and no menu/dialog/input owns Esc.
3. **Quick-add kind toggle ((325)).** In the Items panel new row
   (`newRowHtml`/`createNew`), drop the right-hand task/topic `<select>` and
   put an icon toggle at the left edge where the leading "+" glyph is. It
   shows the current kind's icon (task: the status circle used for tasks;
   topic: the topic speech-bubble icon), toggles task <-> topic on click or
   Space/Enter, has an aria-label/tooltip naming the current kind and the
   action, and keeps every existing kind-switch rule (topic hides status and
   due chips, collapses several assignees to the viewer). The kanban
   idea-column row (`knewRowHtml`) stays title-only.
4. **Priority chip ((327)).** Add a priority chip to the quick-add row (next
   to the assignee and due chips) and to the item window header, right of the
   status button strip (after "dropped", before the + / assign buttons). The
   chip shows the level name and opens the item menu's existing priority
   step (quick-add: over the draft row, the way the status/due chips already
   do through `MN.kind 'new'` / `draftItem()`). New items default to the
   store's default level as today. Hidden for topic kind, like status and
   due. Respect the existing edit permission (`item.edit` rules) in the
   header.
5. **Label chip in quick-add ((328)).** Add a label chip to the quick-add
   row that opens a multi-select picker over the store label list
   (`VS.labels`); chosen label ids go into the create payload's `labels`
   field. For a store admin, the same picker also offers add a label,
   rename a label, and remove a label, using the existing admin label events
   the Team tab's store-settings section sends (reuse its code paths; no new
   event). Remove asks for confirmation first, because it takes the label off
   every item. A non-admin only picks. Labels are not hidden for topic kind
   unless an existing rule says otherwise (check the store-settings-and-labels
   ticket in ai-docs/tickets/.done/ and follow it).

## Constraints

- Quiet dashboard tone: `--wb-*` tokens, existing chip and icon styles.
- Update the comment above `newRowHtml` ("Priority, due, and assignees are
  set afterwards") to match.
- Keep the engine / Go side unchanged unless the create payload truly lacks
  `labels` or `priority` support; if it does, stop and report rather than
  changing the protocol.
- Out of scope: the hover-insert depth markers (whiteboard (329)) - a
  follow-up message may add it.

## Verification

- Existing JS tests (runNode cases in internal/wsdashboard) and i18n key
  tests pass through ws-rgo (never run heavy go test locally):
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 15 -- test ./internal/wsdashboard/... ./internal/wsi18n/...`
- Add JS test cases where the tracker test harness already covers selection
  and quick-add (selection toggle/range without modifiers in selection mode;
  kind toggle; priority/labels in the create payload).
- Headless Chromium captures of: selection badge (light/dark), quick-add row
  task and topic, item header with the priority chip, label picker as admin.
  Put captures in the lead's scratchpad
  /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/shots-tracker/.
