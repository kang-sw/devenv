# Plan: tracker item menu shows an ID copy button, not the raw alias

## Problem

The tracker's item menu (status popover opened from a row's status icon, and
the full item menu) renders the raw item alias, e.g. `sw-kang.c65a4bcb7634-7`,
as a header code block: `agents-plugin-tool/internal/wsdashboard/assets/tracker.js`
`menuHtml` (~:2259, `<div class="mh"><code class="al">...`). It is long and
wraps over three lines beside the title. User feedback (2026-11-09): replace
the raw ID with a copy button shaped `[copy icon] 작업 ID`, i.e. the same
affordance as the item window's `idCopyBtn` (~:1710: copy icon + "<kind> ID",
alias in the tooltip, copies on click and says so briefly).

## Change

- In `menuHtml`, replace the raw `<code class="al">` alias with the ID copy
  button. Reuse `idCopyBtn(it)` (or a shared helper) so the label, aria label,
  tooltip, and copy feedback are the same as the item window's. Make sure the
  click actually copies from inside the menu: menu clicks may go through a
  different dispatch than the item window (`data-mi`, the `act` handler
  around ~:3374, and `copyText`'s `near` argument, since the rest of the page
  may be inert while a menu is open). Keep keyboard focus behavior of the
  menu sane (the button must not steal `data-autofocus`).
- Adjust `tracker.css` (`.mh`, `.al`) so the header lays out as
  `[copy button] [title]` without the alias wrapping; drop the now-dead `.al`
  rule if nothing else uses it.
- Sweep the tracker UI for other places that show a raw item alias/id as
  plain visible text for the user to read (not tooltips, not `#ref` links,
  not data attributes). Candidate: the properties list `js.tk.prop.id`
  (~:1565, `<code>` of `it.id`). Convert those to the same copy button only
  when they are the same kind of raw-ID display; list what you found and what
  you changed or left alone, with reasons, in the report. Member ids
  (`.nid`, `.aid`, person ids) are out of scope.
- No new i18n keys unless unavoidable; if you add one, every table must hold
  it (`TestOtherTablesHoldEveryEnglishKey`).
- If the tracker mockup (engine.js / README under the mockup tree) mirrors
  this header and a test pins it, keep it consistent; otherwise leave it.

## Verification

- `cd agents-plugin-tool && go test ./internal/wsdashboard/` green.
- There is no JS test harness; at least `node --check` the edited JS and
  reason through the click path in the report (which handler receives the
  click inside the menu and that `copyText` runs).

## Boundary

Commit on `develop` in the main checkout; do not push. Never run `git stash`.
Return the commit SHA, the sweep list, and anything unverified.
