# Peer row collapse priority: keep a manual label visible

## Outcome

In the dashboard's peer-lead list (the "peers" group, template `peerrow` in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`, styles in
`assets/dashboard.css`), a narrow row today truncates the session name first
(e.g. `devenv/메인 ...`) while the branch chip, the harness badge (`claude`),
the tool badge (`도구`), and the tool body text (`Ba...`) keep their room.

User request (2026-11-12): when the row runs out of width, the parts give
way in this order:

1. the harness badge,
2. the tool body (the running tool's text after the tool badge),
3. the tool badge,

and the `<project>/<session name>` title is kept whole as long as possible.

This applies only when the session name is a **manual label** (a label the
user set; see the session-label work, commits 4b0a95db7 and b461aedee). When
the name shown is the automatic mailbox id, the row keeps collapsing exactly
as it does today.

## Boundaries

- Asset-only (template, CSS, and JS if a width-driven class is needed). No
  Go-side data change beyond exposing whether the name is a manual label, if
  the template does not already know it. Compatibility tier T0.
- Do not change ordering, content, or behaviour of other row parts (branch
  chip, counts, timers) beyond what the collapse order needs. Where the
  branch chip sits in the order was not specified: leave its current
  behaviour unless it blocks keeping the label whole, and say in the commit
  what you chose.
- Prefer a CSS-only solution (flex-shrink ordering, min-width, container
  queries) over JS measuring; use JS only if CSS cannot express the order.
- The same row shape may render elsewhere (rail session rows); change only
  the peer row unless the shared shape makes that impossible, and report it.

## Verification

- Existing wsdashboard tests stay green (run Go tests through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- test ./internal/wsdashboard/...`;
  never run heavy go build/test locally).
- A headless screenshot at two or three row widths, with a manual label and
  with an automatic mailbox id, showing the collapse order
  (`~/.cache/ms-playwright/chromium_headless_shell-1223/*/chrome-headless-shell
  --no-sandbox --hide-scrollbars --window-size=W,H --screenshot=<png> <url>`
  against a static fixture page is fine).
- Add a test where the package has a harness for the template output (for
  example that a manual-label row carries the class the CSS keys on).

## Return

Commit on branch `delegate/peer-row-collapse` in worktree
`/home/swkang/.ws-worktrees/devenv/mongrel-handball-profusely`. Do not merge
or push. Report the commit, the approach, screenshots' paths, and test
results.
