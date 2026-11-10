# Plan: board rail bottom capsules overlap the reply (copy-answers) bar

## Request

User, 2026-11-10 whiteboard, with a screenshot of the dashboard board column:
the board rail's bottom jump capsules (`.wb-rail-wrap.dn` / `.wb-rail-stack`,
`internal/wswhiteboard/assets/rail.js`, styles in `page.css`) draw over the
sticky reply bar (`.reply`: the readonly `(nn) a; (mm) ok` textarea and the
copy button, `page.tmpl` "reply" template; sticky via `.full .reply` in
`page.css` ~:422 and `.r-board .reply` in `dashboard.css` ~:452), so the
bottom of the visible board is misaligned.

User direction (verbatim gist): "the copy-message area at the bottom is
better hidden in the dashboard; the file whiteboard without the dashboard
probably misrenders too - there, maybe move the message bar itself above the
badges."

## Changes

1. Dashboard (`#bcol` board): hide the reply bar visually. The dashboard's
   own composer sends answers; keep the element in the DOM (dashboard.js
   ~:815 and the whiteboard form script read `#reply-text` to build the
   composer's answer text), so hide with CSS only, never remove it. Verify
   no dashboard code path depends on the bar being visible (focus, scroll,
   the bulk-ack checkbox `#ack-all` - if the bulk "all ok" checkbox lives
   only in that bar, check whether the dashboard composer exposes the same
   action elsewhere; if not, report it instead of dropping the feature).
   With the bar hidden, the rail's bottom stack must sit flush at the board
   column's bottom edge with no leftover reserve gap.
2. File whiteboard page (`body.full`, no dashboard): keep the reply bar, and
   stack the rail's bottom capsules directly above it (capsules above, bar
   at the very bottom), so neither covers the other and the capsule fade
   still reads. Prefer measuring the bar's height in rail.js (it already
   rAF-throttles layout work) or a CSS variable over hard-coded pixels; the
   bar is `hidden` when nothing is answerable, and then the capsules sit at
   the bottom edge.
3. Embed snapshot (`body.embed`) is unaffected (rail.js returns early there).

Use only existing `--wb-*` tokens. Light and dark themes both.

## Verification

- `nice -n 19 node --check` on touched JS.
- If cheap, a headless render check (isolated dev daemon with its own
  WS_CACHE_HOME / WS_CONFIG_HOME and port, or the file page opened in a
  headless browser) with screenshots of the dashboard board bottom and the
  file page bottom with capsules visible. Never touch processes or ports
  29417 / 45743.
- Go tests only remotely: `~/.local/bin/ws-rgo -C
  /home/swkang/.ws-worktrees/devenv/aim-hydration-granite/agents-plugin-tool
  -m 30 -- test ./internal/wswhiteboard/ ./internal/wsdashboard/ -count=1
  -timeout 30m`. Never run go build/test/vet locally (it freezes the machine).
- Commit on branch `fix/rail-reply-overlap`; do not push. No `git stash`.
