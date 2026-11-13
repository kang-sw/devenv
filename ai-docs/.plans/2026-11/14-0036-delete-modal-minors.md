# Worktree-delete modal: warn contrast and truncation tooltips (ticketless UX fix, (527)=a)

Source: UX post-swap QA of the worktree-delete modal (live build 65a917d1b).
EXEC approved minors (a) and (b) as a ticketless fix on 2026-11-13. Minor (c),
a focus-to-BODY after Esc seen once in 40 runs, is out of scope.

## (a) Light-theme warn text contrast

- `.rdeldlg .rdnote.warn, .rdeldlg .rdforce` (`dashboard.css` ~1747) draws
  `var(--wb-warn-ink)` (light `#a35f00`) on
  `color-mix(in srgb, var(--wb-warn) 14%, var(--wb-card))`. QA measured
  4.29:1, below WCAG AA 4.5:1 for 12.5px text.
- Required: at least 4.5:1 for that text in light theme, and dark theme no
  worse than today (5.7:1). Keep the warn look: amber ink on an amber tint.
- Scope the change to the delete modal (`.rdeldlg`), e.g. a darker ink or a
  lighter tint there. Do not change the global `--wb-warn-ink` or
  `--wb-warn` tokens: many other surfaces use them and a perf worker is
  editing the board assets in parallel.
- Measure, and list in the report, any other `--wb-warn-ink` on a warn-tint
  pair in `dashboard.css` that also fails 4.5:1 (e.g. `.tm-gap` ~695). Do not
  fix those; report them only.

## (b) Tooltips on ellipsised entries

- In `dashboard.js` (~6343-6360), the changed-path rows (`.rdpaths li`) and
  the holder names (`.rdholders .nm`) are ellipsised with no `title`, so
  paths that differ only past the cut-off cannot be told apart.
- Required: each such path row and holder name carries its full text as
  `title`. The "+N more" row (`li.more`) needs none.
- Follow the existing pattern next to it (`where.title = where.textContent`).

## Boundary

- Touch only `wsdashboard/assets/dashboard.css` (delete-modal rules) and
  `wsdashboard/assets/dashboard.js` (the delete-modal render). No other
  behavior changes, and no i18n changes.
- Do not touch the shared daemon:
  - No `scripts/dev.py`.
  - No restart, shutdown or kill of anything you did not start.
  - Do not contact 127.0.0.1:29417.
  - For a headless check, build `ws-mcp` to `/tmp` and run your own daemon:
    `WS_CACHE_HOME=<tmp> WS_CONFIG_HOME=<tmp> HOME=<tmp> <bin> dashboard --port 0`.
    Abort every non-GET in the browser except
    `/rail/worktrees/delete/preview`, and stop that daemon by PID when done.
  - The QA scripts in `/tmp/uxqa` (`qa1.js`-`qa5.js`) show a working setup:
    a fixture repo with linked worktrees, a mocked preview for busy/force.
- If a fix needs anything beyond these two files, stop and return to the lead
  with the reason.

## Verify

- (a): computed foreground and background colors and the contrast ratio of
  the warn note and force box, light and dark, before and after.
- (b): the `title` of a long `.rdpaths li` and a long holder `.nm` equals the
  full text. `li.more` has no title.
- No layout change: dialog width at 1400 and 420 is unchanged, with no
  horizontal overflow.
- `go test ./internal/wsdashboard/...` from `agents-plugin-tool`. The CSS
  collision guard must stay green.
