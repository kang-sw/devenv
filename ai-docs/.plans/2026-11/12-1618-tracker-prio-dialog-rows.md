# Tracker priority-levels dialog: 1-based labels and growing rows

## Outcome

The tracker settings dialog for priority levels (`case 'sprio'` in
`agents-plugin-tool/internal/wsdashboard/assets/tracker.js`, around line
3302; save handler `case 'sprio'` around line 3638) has two defects the user
reported with a screenshot (2026-11-12):

1. Row labels are 0-based. With three named levels, the fourth row's
   placeholder reads "단계 3 이름" (`js.tk.form.plevel`, argument `pi`), and
   the default radio's label/tooltip (`js.tk.form.pdefault`) is off by one the
   same way. Show 1-based numbers (pass `pi + 1` to both strings). The radio
   `value` and input `name="pname<i>"` stay 0-based; they are internal.
2. The dialog shows only `min(9, names.length + 2)` rows, so a user with three
   levels cannot enter more than five without saving and reopening, although
   the intro text says "up to 9". Make the rows grow: when the user types into
   the last row and the count is below 9, append one more blank row (same
   markup, next index). Never more than 9 rows. Keep the initial row count as
   today (`names.length + 2`, capped at 9) or at least one blank row.

## Boundaries

- Asset-only (tracker.js; i18n tables only if a string must change; it should
  not need to). Compatibility tier T0. No change to the save event shape
  (`roles.set_priority_levels {names, default}`).
- The save loop `for (var i = 0; el['pname' + i]; i++)` must still see every
  row, including appended ones (rows are contiguous by index, so it does).
- Follow the dialog's existing event wiring style in tracker.js (find how other
  dialogs react to input inside the open form) rather than adding a new
  framework.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md` before editing (AGENTS.md
  implementation-convention rows for agents-plugin-tool/internal/).

## Verification

- Go tests through the remote runner only:
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 5 -- test ./internal/wsdashboard/...`
  (never run heavy go build/test locally).
- Add a test where the package's JS harness reaches tracker.js (see
  `tracker_settings_test.go` / `tracker_quickadd_test.go` for the existing
  harness pattern): the rendered sprio rows label 1-based, and typing into the
  last row appends a row up to 9. If no harness can drive dialog input, state
  that and cover the label numbering only.
- Optional headless screenshot of the open dialog.

## Return

Commit on branch `delegate/prio-dialog` in worktree
`/home/swkang/.ws-worktrees/devenv/mongrel-handball-profusely` (this plan in
the same commit). Do not merge or push. Report the commit, approach, and test
results.
